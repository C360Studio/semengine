package graphingest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
)

type graphIngestTestOwner struct {
	component   *Component
	cancelStart context.CancelFunc
	attempted   bool
	transferred bool
}

func newGraphIngestTestOwner(c *Component) *graphIngestTestOwner {
	return &graphIngestTestOwner{component: c}
}

func (o *graphIngestTestOwner) startContext(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)
	o.cancelStart = cancel
	return ctx
}

func (o *graphIngestTestOwner) stop(operationCtx context.Context) error {
	if o.attempted {
		return errors.New("graph-ingest fixture Stop already attempted")
	}
	o.attempted = true // A returned error or panic never grants an implicit second attempt.
	if o.cancelStart != nil {
		defer o.cancelStart() // Accepted Start authority stays live through concrete Stop.
	}
	stopCtx, cancelStop := context.WithTimeout(context.WithoutCancel(operationCtx), 5*time.Second)
	defer cancelStop()
	stopErr := o.component.Stop(stopCtx)
	if err := stopCtx.Err(); err != nil {
		stopErr = errors.Join(stopErr, fmt.Errorf("graph-ingest terminal context ended: %w", err))
	}
	if err := operationCtx.Err(); err != nil {
		stopErr = errors.Join(stopErr, fmt.Errorf("graph-ingest operation authority ended before controlled Stop: %w", err))
	}
	return stopErr
}

func (o *graphIngestTestOwner) finish(operationCtx context.Context, t *testing.T) {
	t.Helper()
	if o.attempted {
		return
	}
	if err := o.stop(operationCtx); err != nil {
		t.Errorf("graph-ingest terminal cleanup: %v", err)
	}
}

func (o *graphIngestTestOwner) provisionalFinish(operationCtx context.Context, t *testing.T) {
	t.Helper()
	if !o.transferred {
		o.finish(operationCtx, t)
	}
}

func (o *graphIngestTestOwner) transfer() {
	o.transferred = true
}

// startGuardAcquiringNothing puts c's lifecycle guard where the pin's struct literal
// put a running component (lifecycleUsed: true, running: true): one Start admitted,
// holding something to release, so the next Stop runs c's cleanup. The guard keeps
// its state to itself (design D22), so the state is reached through its Start, with a
// start function that acquires nothing.
func startGuardAcquiringNothing(tb testing.TB, c *Component) {
	tb.Helper()
	if err := c.lifecycle.Start(tb.Context(), func(context.Context) error { return nil }, c.release); err != nil {
		tb.Fatalf("guard Start that acquires nothing: %v", err)
	}
}

// leaveGuardCleanupPending puts c's lifecycle guard where the pin's struct literal put
// a component whose Start failed and whose rollback failed (lifecycleUsed: true,
// cleanupPending: true): the guard is used, and the next Stop runs c's cleanup (design
// D22, guard.go's Start).
func leaveGuardCleanupPending(tb testing.TB, c *Component) {
	tb.Helper()
	errStart := errors.New("setup: start fails")
	errRollback := errors.New("setup: rollback fails")
	err := c.lifecycle.Start(tb.Context(),
		func(context.Context) error { return errStart },
		func(context.Context) error { return errRollback })
	if !errors.Is(err, errStart) || !errors.Is(err, errRollback) {
		tb.Fatalf("guard Start that fails with a failing rollback = %v, want both causes", err)
	}
}

// assertGuardUnused fails the test unless c's lifecycle guard is still unused: a
// following Start of the guard, with a start function that acquires nothing, is not
// refused with errs.ErrAlreadyStarted. This is design D22's stand-in for the pin's read
// of lifecycleUsed; it uses the guard up, so it is a test's last step.
func assertGuardUnused(tb testing.TB, c *Component) {
	tb.Helper()
	err := c.lifecycle.Start(tb.Context(), func(context.Context) error { return nil }, c.release)
	if errors.Is(err, errs.ErrAlreadyStarted) {
		tb.Fatalf("lifecycle guard already used: a following Start = %v", err)
	}
	if err != nil {
		tb.Fatalf("following Start of the guard: %v", err)
	}
}

// graphIngestLifecycleConsumeContext stands in for a JetStream consume context. At the
// pin it is in lifecycle_owner_test.go:15-31; test_owner_test.go reads it too.
type graphIngestLifecycleConsumeContext struct {
	closed    chan struct{}
	drainSeen chan struct{}
	drainOnce sync.Once
	drains    atomic.Int32
}

func (*graphIngestLifecycleConsumeContext) Stop() { panic("unexpected force Stop") }

func (c *graphIngestLifecycleConsumeContext) Drain() {
	c.drainOnce.Do(func() {
		c.drains.Add(1)
		close(c.drainSeen)
	})
}

func (c *graphIngestLifecycleConsumeContext) Closed() <-chan struct{} { return c.closed }
