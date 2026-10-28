package objectstore

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
)

// suiteOwner adapts Component to the lifecycle suite (lifecycle-suite, "Observe adapter
// contract"). Unresolved lists, by a stable name each, what a started Component holds and what a
// cleanup that succeeded clears: the run context's cancel, the Store, the core-NATS
// subscriptions, the JetStream consumers, and the input check while its goroutine runs.
//
// Calls is empty: the Component has no acquisition seam to count through (design D8), so the
// suite's "a no-op made no call" checks compare empty maps, and what a call left held is read
// from Unresolved.
type suiteOwner struct{ c *Component }

func (o suiteOwner) Start(ctx context.Context) error { return o.c.Start(ctx) }
func (o suiteOwner) Stop(ctx context.Context) error  { return o.c.Stop(ctx) }

// Observe reads the handles start records. The suite observes after a Start or Stop returned to
// it, and the input check touches none of these fields, so the reads need no lock.
func (o suiteOwner) Observe() lifecycletest.Observation {
	c := o.c
	var held []string
	if c.cancel != nil {
		held = append(held, "run cancel")
	}
	if c.store.Load() != nil {
		held = append(held, "store")
	}
	if len(c.subscriptions) > 0 {
		held = append(held, "core subscriptions")
	}
	if len(c.consumers) > 0 {
		held = append(held, "consumers")
	}
	// Listed while its goroutine runs: a closed checkDone means the check has returned, which
	// is not held work, so a check that ended with the Start context is reported gone.
	if c.checkDone != nil && !isClosed(c.checkDone) {
		held = append(held, "input check")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// stubSubscription is a core-NATS subscription whose Drain behaves as natsclient's does when
// the drain itself is slow: it returns the context's error when that context has ended, and a
// later call with a live context completes the same drain.
type stubSubscription struct{ drains int }

func (s *stubSubscription) Drain(ctx context.Context) error {
	s.drains++
	return ctx.Err()
}

// stubConsumeContext is a JetStream consume loop whose Closed fires when the test closes it.
type stubConsumeContext struct {
	closed chan struct{}
	drains int
}

func (c *stubConsumeContext) Drain()                  { c.drains++ }
func (c *stubConsumeContext) Stop()                   { panic("cleanup must drain, never force-stop") }
func (c *stubConsumeContext) Closed() <-chan struct{} { return c.closed }

// TestFailedCleanupRetainsInputsForLaterStop proves the Component's branch of lifecycle-suite,
// "Failed start whose own cleanup fails" (design D8), which the suite's failed-start check does not
// judge: a Start that fails after binding its inputs, and whose own cleanup then fails too,
// returns both failures and keeps the inputs on record, and a later Stop with a fresh finite
// context returns nil and leaves nothing.
//
// The guard is reached through its Start, as graph-ingest's
// TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop does: a start function that
// binds and fails, and a rollback that is the Component's own release under a context that has
// already ended, as a rollback budget that expires while an input still finishes a write leaves
// it. The guard runs the one cleanup Start was given, so that cleanup fails on its first call,
// the rollback, and runs release with the later Stop's context after that.
func TestFailedCleanupRetainsInputsForLaterStop(t *testing.T) {
	c, err := buildFromJSON(t, `{}`) // no NATS client: Start is reached through the shell below
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	owner := suiteOwner{c}
	subscription := &stubSubscription{}
	consumer := &stubConsumeContext{closed: make(chan struct{})}
	runCtx, runCancel := context.WithCancel(t.Context())

	errStart := errors.New("start fails after binding its inputs")
	expired, expire := context.WithCancel(t.Context())
	expire()
	rolledBack := false // the guard runs the cleanup one call at a time
	err = c.lifecycle.Start(t.Context(),
		func(context.Context) error {
			c.cancel = runCancel
			c.subscriptions = append(c.subscriptions, subscription)
			c.consumers = append(c.consumers, consumerBinding{handle: consumer})
			return errStart
		},
		func(ctx context.Context) error {
			if !rolledBack { // the rollback: release under the ended context fails
				rolledBack = true
				return c.release(expired)
			}
			return c.release(ctx)
		})
	if !errors.Is(err, errStart) || !errors.Is(err, context.Canceled) {
		t.Fatalf("failed Start whose cleanup fails = %v, want the start failure and context.Canceled", err)
	}
	if got, want := owner.Observe().Unresolved, []string{"run cancel", "core subscriptions", "consumers"}; !slices.Equal(got, want) {
		t.Fatalf("held after the failed cleanup = %v, want %v", got, want)
	}
	if consumer.drains != 1 || subscription.drains != 1 {
		t.Fatalf("drains after the failed cleanup: consumer %d, subscription %d; want 1 each",
			consumer.drains, subscription.drains)
	}
	if !errors.Is(runCtx.Err(), context.Canceled) {
		t.Fatalf("run context after the failed cleanup = %v, want canceled", runCtx.Err())
	}

	close(consumer.closed)
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("later Stop: %v", err)
	}
	if got := owner.Observe().Unresolved; len(got) != 0 {
		t.Fatalf("held after the later Stop = %v, want nothing", got)
	}
	if consumer.drains != 1 {
		t.Fatalf("consumer Drain issued again: %d calls", consumer.drains)
	}
	if err := owner.Stop(t.Context()); err != nil {
		t.Fatalf("repeated Stop: %v", err)
	}
}
