// Package lifecycleguard is the one-shot owner-lifecycle state a SemEngine component composes. It
// admits one Start, refuses a second, makes Stop safe before, during and after Start, and keeps a
// cleanup that failed, in a rollback or in a Stop, pending for the next Stop. It owns that state
// and nothing else: what the component acquires and releases stays in the functions it passes in,
// and a failed Start is rolled back through pkg/lifecyclecleanup.
//
// The owner gives its cleanup once, to Start. A failed Start's rollback and every Stop run that
// one function.
//
// Start and Stop refuse a nil context the same way, as invalid data. They answer a context that
// has already ended differently: Start refuses it as invalid, since a Start given an ended context
// cannot begin, while Stop on a started unstopped owner still runs cleanup so teardown always
// initiates and ctx bounds only waiting inside cleanup.
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
// the recorded cleanup again.
type Guard struct {
	mu sync.Mutex
	// used is set by the first admitted Start or the first Stop.
	used bool
	// cleanup is the function the admitted Start was given, set together with used. A Stop
	// before any Start sets used and terminal at once, so Stop never runs an unset cleanup.
	cleanup func(context.Context) error
	// terminal is set once nothing is left to release: by a Stop before any Start, or by a
	// rollback or cleanup that succeeded. Stop then returns nil and calls nothing.
	terminal bool
	// stopping is set while a Stop runs cleanup.
	stopping bool
	// startDone is non-nil while a Start is in flight and closed when it finishes.
	startDone chan struct{}
}

// Start admits one Start, records cleanup as the owner's one cleanup, and runs start, which
// acquires the owner's resources. Before start runs it refuses a nil context as invalid data, a
// context already ended as invalid (the error wraps the context's own), and a second Start with
// errs.ErrAlreadyStarted; a refused Start runs nothing, records nothing and leaves the guard as it
// was, so the admitted Start's cleanup stays the one every Stop runs. When start returns an error
// or panics, cleanup runs through lifecyclecleanup.RollbackFailedStart with ctx, and its error is
// joined into Start's; if that rollback fails, the cleanup stays pending and the next Stop runs it
// again.
func (g *Guard) Start(ctx context.Context, start, cleanup func(context.Context) error) (startErr error) {
	if ctx == nil {
		return nilContextError("Start")
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
	g.used, g.cleanup, g.startDone = true, cleanup, done
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

// Stop runs the cleanup the admitted Start recorded even when ctx has already ended, so teardown
// always initiates and ctx bounds only waiting inside cleanup. A nil context is refused as invalid
// data, as Start refuses it. Stop before any Start runs nothing, returns nil and uses up the guard.
// Stop during a Start waits for that Start to finish, or returns ctx's error first if Start is
// still running when ctx ends. When cleanup returns an error, it stays pending and the next Stop
// runs it again. When cleanup returns nil, the guard becomes terminal; if ctx had already ended
// when cleanup began, Stop still returns that context error unwrapped while later Stops return nil
// and run nothing. A Stop while another Stop runs cleanup is refused as transient.
//
// A cleanup that panics is not recovered, and it leaves the guard stuck: after a panic in a Stop's
// cleanup every later Stop is refused as concurrent, and after a panic in a failed Start's rollback
// every later Stop waits until its own context ends. A cleanup must not panic. The guard does not
// recover one because no caller's cleanup can panic and nothing recovers a panic around Stop, so a
// panicking cleanup ends the process before a later Stop could see the stuck state.
func (g *Guard) Stop(ctx context.Context) error {
	if ctx == nil {
		return nilContextError("Stop")
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
				g.mu.Lock()
				stillStarting := g.startDone != nil
				g.mu.Unlock()
				if stillStarting {
					return ctx.Err()
				}
				continue
			}
		}
		if g.stopping {
			g.mu.Unlock()
			return errs.WrapTransient(errors.New("stop already in progress"), "lifecycleguard", "Stop", "concurrent Stop is unsupported")
		}
		g.stopping = true
		cleanup := g.cleanup
		g.mu.Unlock()

		ended := ctx.Err()
		err := cleanup(ctx)
		g.mu.Lock()
		g.stopping, g.terminal = false, err == nil
		g.mu.Unlock()
		if err != nil {
			return err
		}
		return ended
	}
}

// nilContextError is the refusal Start and Stop give a nil context. method only names the refusing
// method in the message.
func nilContextError(method string) error {
	return errs.WrapInvalid(errs.ErrInvalidData, "lifecycleguard", method, "nil context")
}
