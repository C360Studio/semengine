//go:build integration

package graphlexical

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
)

// suiteOwner is graph-lexical as the lifecycle suite's owner. Observe lists what graph-lexical
// itself holds; the follower's watch is private to graph/follower, so
// TestGraphLexicalLifecycleSuite reads it from the server instead of through a test-only export.
type suiteOwner struct{ c *Component }

func (o suiteOwner) Start(ctx context.Context) error { return o.c.Start(ctx) }
func (o suiteOwner) Stop(ctx context.Context) error  { return o.c.Stop(ctx) }

// Observe reads the component's handles without a lock: the suite observes after a Start or Stop
// has returned to it, and only a call that overran its bound is observed while it runs, a check
// that has already failed.
func (o suiteOwner) Observe() lifecycletest.Observation {
	var held []string
	if o.c.runCancel != nil {
		held = append(held, "run cancel")
	}
	if o.c.stopped != nil {
		held = append(held, "stopped")
	}
	if len(o.c.subscriptions) > 0 {
		held = append(held, "query subscriptions")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// watchConsumers is the number of consumers on ENTITY_STATES' backing stream: the follower's
// watch is one while graph-lexical runs.
func watchConsumers(ctx context.Context, client *natsclient.Client) (int, error) {
	js, err := client.JetStream()
	if err != nil {
		return -1, err
	}
	stream, err := js.Stream(ctx, "KV_"+graph.BucketEntityStates)
	if err != nil {
		return -1, err
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return -1, err
	}
	return info.State.Consumers, nil
}

// TestGraphLexicalLifecycleSuite runs the lifecycle suite on graph-lexical. Every owner is a fresh
// component on one client of the package's shared server. The must-fail owner's environment has
// no ENTITY_STATES, so its Start fails in the follower's Open, before it acquires anything, and
// the cleanup the failed Start runs succeeds; graph-lexical's start acquires nothing it can fail
// to release, so the branch where that cleanup fails has no case here. graph-lexical is one-shot
// (a Start after Stop is refused), so no restart is promised.
func TestGraphLexicalLifecycleSuite(t *testing.T) {
	client := fixtureClient(t)
	ensureEntityStates(t, client)
	factory := func() lifecycletest.Owner {
		return suiteOwner{newLexical(t, client, `{}`, metric.NewMetricsRegistry(), nil)}
	}
	mustFail := func() lifecycletest.Owner {
		js, err := client.JetStream()
		if err != nil {
			t.Fatalf("JetStream: %v", err)
		}
		if err := js.DeleteKeyValue(t.Context(), graph.BucketEntityStates); err != nil &&
			!errors.Is(err, jetstream.ErrBucketNotFound) {
			t.Fatalf("delete %s: %v", graph.BucketEntityStates, err)
		}
		return suiteOwner{newLexical(t, client, `{"startup_attempts": 1, "startup_interval_ms": 1}`,
			metric.NewMetricsRegistry(), nil)}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// The suite judges only what is left after a Stop, so an adapter that omitted a kind would
	// pass it. This names every kind a started graph-lexical holds, and reads the follower's watch
	// from the server.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		ensureEntityStates(t, client) // the must-fail owner deleted it
		owner := factory()
		if err := owner.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		want := []string{"run cancel", "stopped", "query subscriptions"}
		if held := owner.Observe().Unresolved; !slices.Equal(held, want) {
			t.Errorf("held while started = %v, want %v", held, want)
		}
		await(t, "the follower's watch on the server",
			func(ctx context.Context) (int, error) { return watchConsumers(ctx, client) },
			func(n int) bool { return n == 1 })
		stopComponent(t, owner.(suiteOwner).c)
		if held := owner.Observe().Unresolved; len(held) != 0 {
			t.Errorf("held after a nil Stop = %v, want nothing", held)
		}
		// nats.go deletes the watch's consumer asynchronously after Stop's unsubscribe.
		await(t, "no consumer left on ENTITY_STATES after Stop",
			func(ctx context.Context) (int, error) { return watchConsumers(ctx, client) },
			func(n int) bool { return n == 0 })
	})
}
