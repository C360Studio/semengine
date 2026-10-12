// Package lifecycleguard is the one-shot owner-lifecycle state a SemEngine component composes
// (SemStreams issue 1411; design D13 of setup-04a-02-ingest-kernel). It admits one Start, refuses
// a second, makes Stop safe before, during and after Start, and keeps a cleanup that failed, in a
// rollback or in a Stop, pending for the next Stop. It owns that state and nothing else: what the
// component acquires and releases stays in the functions it passes in, and a failed Start is rolled
// back through pkg/lifecyclecleanup.
package lifecycleguard

import (
	"context"
	"errors"
	"sync"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/lifecyclecleanup"
)

// Guard is one owner's lifecycle state. The zero value is an owner that has not started. A Guard
// is single use: once Start has been attempted past its refusals, or Stop has been called, a later
// Start is refused with errs.ErrAlreadyStarted. From an admitted Start until a cleanup succeeds,
// whether Start's rollback or a Stop's, the guard holds something to release and each Stop runs
// cleanup again.
type Guard struct {
	mu sync.Mutex
	// used is set by the first admitted Start or the first Stop.
	used bool
	// terminal is set once nothing is left to release: by a Stop before any Start, or by a
	// rollback or cleanup that succeeded. Stop then returns nil and calls nothing.
	terminal bool
	// stopping is set while a Stop runs cleanup.
	stopping bool
	// startDone is non-nil while a Start is in flight and closed when it finishes.
	startDone chan struct{}
}

// Start admits one Start and runs start, which acquires the owner's resources. A nil context, a
// context already ended, and a second Start are refused before start runs. When start returns an
// error or panics, cleanup runs through lifecyclecleanup.RollbackFailedStart with ctx, and its
// error is joined into Start's; if that rollback fails, the cleanup stays pending and the next
// Stop runs it again.
func (g *Guard) Start(ctx context.Context, start, cleanup func(context.Context) error) (startErr error) {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "lifecycleguard", "Start", "context cannot be nil")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "lifecycleguard", "Start", "context already ended")
	}
	g.mu.Lock()
	if g.used {
		g.mu.Unlock()
		return errs.WrapFatal(errs.ErrAlreadyStarted, "lifecycleguard", "Start", "lifecycle already used")
	}
	done := make(chan struct{})
	g.used, g.startDone = true, done
	g.mu.Unlock()

	committed := false
	defer func() {
		var rollbackErr error
		if !committed {
			rollbackErr = lifecyclecleanup.RollbackFailedStart(ctx, cleanup)
			startErr = errors.Join(startErr, rollbackErr)
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if !committed && rollbackErr == nil {
			g.terminal = true
		}
		close(done)
		g.startDone = nil
	}()
	if err := start(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// Stop runs cleanup and returns its result. A nil context is refused, and a context already ended
// returns its error. Stop before any Start runs nothing, returns nil and uses up the guard. Stop
// during a Start waits for that Start to finish, or returns ctx's error first. When cleanup fails,
// it stays pending and the next Stop runs it again. Once a cleanup or a failed Start's rollback
// has succeeded, Stop returns nil and runs nothing. A Stop while another Stop runs cleanup is
// refused.
func (g *Guard) Stop(ctx context.Context, cleanup func(context.Context) error) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "lifecycleguard", "Stop", "nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for {
		g.mu.Lock()
		if !g.used {
			g.used, g.terminal = true, true
			g.mu.Unlock()
			return nil
		}
		if g.terminal {
			g.mu.Unlock()
			return nil
		}
		if done := g.startDone; done != nil {
			g.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if g.stopping {
			g.mu.Unlock()
			return errs.WrapTransient(errors.New("stop already in progress"), "lifecycleguard", "Stop", "concurrent Stop is unsupported")
		}
		g.stopping = true
		g.mu.Unlock()

		err := cleanup(ctx)
		g.mu.Lock()
		g.stopping, g.terminal = false, err == nil
		g.mu.Unlock()
		return err
	}
}
