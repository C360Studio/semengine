//go:build integration

package objectstore

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
)

// stubSubscription is a core-NATS subscription whose Drain behaves as natsclient's does when
// the drain itself is slow: it returns the context's error when that context has ended, and a
// later call with a live context completes the same drain.
type stubSubscription struct{ drains int }

func (s *stubSubscription) Drain(ctx context.Context) error {
	s.drains++
	return ctx.Err()
}

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
//
// The JetStream input is a real consumer, bound as start binds it: cleanup waits for its handlers
// through natsclient's WaitConsumerHandlers, which knows only handles its client returned (design
// D11), so a stub consume loop would be refused.
func TestFailedCleanupRetainsInputsForLaterStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
	createInputStream(ctx, t, clientJetStream(t, client), jetstream.MemoryStorage)
	c, err := newStartable(client, jetStreamConfig)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	owner := suiteOwner{c}
	subscription := &stubSubscription{}
	runCtx, runCancel := context.WithCancel(t.Context())

	errStart := errors.New("start fails after binding its inputs")
	expired, expire := context.WithCancel(t.Context())
	expire()
	rolledBack := false // the guard runs the cleanup one call at a time
	err = c.lifecycle.Start(ctx,
		func(ctx context.Context) error {
			c.cancel = runCancel
			c.subscriptions = append(c.subscriptions, subscription)
			// Nothing is published to the input, so its handler never runs and needs no Store.
			handle, err := c.bindJetStream(ctx, c.bindings[0], nil)
			if err != nil {
				return err
			}
			c.consumers = append(c.consumers, consumerBinding{handle: handle})
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
	if !c.consumers[0].drainIssued || subscription.drains != 1 {
		t.Fatalf("after the failed cleanup: consumer drain issued %t, subscription drains %d; want true and 1",
			c.consumers[0].drainIssued, subscription.drains)
	}
	if !errors.Is(runCtx.Err(), context.Canceled) {
		t.Fatalf("run context after the failed cleanup = %v, want canceled", runCtx.Err())
	}

	stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
	defer stopCancel()
	if err := owner.Stop(stopCtx); err != nil {
		t.Fatalf("later Stop: %v", err)
	}
	if got := owner.Observe().Unresolved; len(got) != 0 {
		t.Fatalf("held after the later Stop = %v, want nothing", got)
	}
	if err := owner.Stop(stopCtx); err != nil {
		t.Fatalf("repeated Stop: %v", err)
	}
}
