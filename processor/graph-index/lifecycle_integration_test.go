//go:build integration

package graphindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// suiteOwner is graph-index as the lifecycle suite's owner. Observe lists what graph-index itself
// holds; the follower's watch is private to graph/follower, so TestGraphIndexLifecycleSuite reads
// it from the server instead (design D11, no test-only export, #186).
type suiteOwner struct{ c *Component }

func (o suiteOwner) Start(ctx context.Context) error { return o.c.Start(ctx) }
func (o suiteOwner) Stop(ctx context.Context) error  { return o.c.Stop(ctx) }

// Observe reads the component's handles without a lock: the suite observes after a Start or Stop
// has returned to it, and only a call that overran its bound is observed while it runs, a check
// that has already failed.
func (o suiteOwner) Observe() lifecycletest.Observation {
	c := o.c
	var held []string
	if c.runCancel != nil {
		held = append(held, "run cancel")
	}
	if c.poolCancel != nil {
		held = append(held, "pool cancel")
	}
	if c.pool != nil {
		held = append(held, "pool lanes")
	}
	if c.coalescer != nil {
		held = append(held, "coalescer")
	}
	if c.feeds != nil {
		held = append(held, "feeds")
	}
	if len(c.subscriptions) > 0 {
		held = append(held, "query subscriptions")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// watchConsumers is the number of consumers on ENTITY_STATES' backing stream: the follower's
// watch is one while graph-index runs.
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

// TestGraphIndexLifecycleSuite runs the lifecycle suite on graph-index. Every owner is a fresh
// component on one client of the package's shared server; coalesce_ms is above zero so the
// coalescer is among the handles. The must-fail owner's environment has no ENTITY_STATES, so its
// Start fails in the follower's Open, before it acquires anything, and the cleanup the failed
// Start runs succeeds: lifecycle-suite's "Cleanup succeeds after a failed start". graph-index's
// start acquires nothing it can fail to release (notes/4.3.md), so the branch where that cleanup
// fails has no case here. graph-index is one-shot (a Start after Stop is refused), so no restart
// is promised.
func TestGraphIndexLifecycleSuite(t *testing.T) {
	client := fixtureClient(t)
	ensureEntityStates(t, client)
	factory := func() lifecycletest.Owner {
		c, _ := newIndex(t, client, `{"coalesce_ms": 50}`)
		return suiteOwner{c}
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
		c, _ := newIndex(t, client, `{"startup_attempts": 1, "startup_interval_ms": 1}`)
		return suiteOwner{c}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that omitted a kind would pass it. This names every kind a
	// started graph-index holds, and reads the follower's watch from the server.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		ensureEntityStates(t, client) // the must-fail owner deleted it
		owner := factory()
		if err := owner.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		want := []string{"run cancel", "pool cancel", "pool lanes", "coalescer", "feeds", "query subscriptions"}
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

// TestStopWritesAcceptedWork: work the keyed pool accepted before Stop is written before Stop
// returns nil, with no error counted and no failure logged (spec "Stop writes accepted work";
// design D4). The entities are stored before Start, so the replay hands them to the one lane far
// faster than it writes them, and Stop runs with a backlog.
func TestStopWritesAcceptedWork(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	const accepted = 500
	target := entityID(999)
	for n := 1; n <= accepted+1; n++ {
		id := entityID(n)
		if _, err := states.Put(t.Context(), id, entityValue(t, id, stmt{linkTo, target})); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
	c, logs := newIndex(t, client, `{}`) // one lane
	startComponent(t, c)
	h := &harness{c: c, client: client}

	// The follower calls Deliver for one entry at a time, and deliver counts an entry before it
	// submits it. The last entry counted therefore means every earlier entry's submit has
	// returned: the first 500 entries are accepted work.
	await(t, "every entry delivered",
		func(context.Context) (int64, error) { return c.DataFlow().Messages, nil },
		func(n int64) bool { return n >= accepted+1 })
	if written := len(h.keys(t, graph.BucketOutgoingIndex)); written >= accepted {
		t.Fatalf("%d of %d entities already written before Stop; the test needs a backlog", written, accepted)
	}
	errorsBefore := c.Health().ErrorCount

	stopComponent(t, c)

	written := h.keys(t, graph.BucketOutgoingIndex)
	var missing []string
	for n := 1; n <= accepted; n++ {
		if !slices.Contains(written, entityID(n)) {
			missing = append(missing, entityID(n))
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d accepted entities not written when Stop returned nil, first %s", len(missing), missing[0])
	}
	if got := c.Health().ErrorCount; got != errorsBefore {
		t.Errorf("ErrorCount = %d after Stop, want %d", got, errorsBefore)
	}
	for _, level := range []string{"level=WARN", "level=ERROR"} {
		if out := logs.String(); strings.Contains(out, level) {
			t.Errorf("a clean Stop logged at %s:\n%s", level, out)
		}
	}
}

// TestStopInterruptedThenRetriedWritesAcceptedWork: a Stop whose context ends while the pool is
// still draining returns that context's error and keeps its handles, and a second Stop finishes
// the drain, so every accepted entity is written (spec "Stop writes accepted work"; ADR-113).
//
// The test depends on the drain outlasting the cancel: the backlog is observed before Stop, as in
// TestStopWritesAcceptedWork, and the cancel follows Stop's entry into cleanup by one observation.
// If the drain ever wins, the first Stop returns nil and the test fails as "no backlog", never
// passing without having interrupted a drain.
func TestStopInterruptedThenRetriedWritesAcceptedWork(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	const accepted = 500
	target := entityID(999)
	for n := 1; n <= accepted+1; n++ {
		id := entityID(n)
		if _, err := states.Put(t.Context(), id, entityValue(t, id, stmt{linkTo, target})); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
	c, _ := newIndex(t, client, `{}`) // one lane
	startComponent(t, c)
	h := &harness{c: c, client: client}

	// As in TestStopWritesAcceptedWork: the last entry counted means the first 500 are accepted.
	await(t, "every entry delivered",
		func(context.Context) (int64, error) { return c.DataFlow().Messages, nil },
		func(n int64) bool { return n >= accepted+1 })
	if written := len(h.keys(t, graph.BucketOutgoingIndex)); written >= accepted {
		t.Fatalf("%d of %d entities already written before Stop; the test needs a backlog", written, accepted)
	}

	stopCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := make(chan error, 1)
	go func() { first <- c.Stop(stopCtx) }()
	// component.Lifecycle reports "stopped" before it runs cleanup, so this orders the cancel after
	// Stop has been entered, not after the drain has begun.
	await(t, "Stop entered",
		func(context.Context) (string, error) { return c.Health().Status, nil },
		func(status string) bool { return status == "stopped" })
	cancel()
	select {
	case err := <-first:
		if err == nil {
			t.Fatal("the first Stop returned nil: the drain finished before the cancel, no backlog interrupted")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first Stop = %v, want context.Canceled", err)
		}
	case <-time.After(waitBound):
		t.Fatal("the first Stop did not return after its context ended")
	}

	stopComponent(t, c)

	written := h.keys(t, graph.BucketOutgoingIndex)
	for n := 1; n <= accepted; n++ {
		if !slices.Contains(written, entityID(n)) {
			t.Fatalf("accepted entity %s not written after the retried Stop", entityID(n))
		}
	}
}

// TestSecondStartIsRefused: a second Start on a running graph-index returns errs.ErrAlreadyStarted
// and the component keeps running: it still indexes what is written (spec "One Start per
// instance"; ADR-108 D3(a)). The pin returned nil.
func TestSecondStartIsRefused(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	if err := h.c.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("second Start = %v, want ErrAlreadyStarted", err)
	}
	requireStillIndexing(t, h)
}

// TestStartContextEndStopsNothing: ending the context given to Start after Start returned stops
// nothing; graph-index still indexes what is written (ADR-113; design D4).
func TestStartContextEndStopsNothing(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	c, logs := newIndex(t, client, `{}`)
	startCtx, cancelStart := context.WithCancel(t.Context())
	if err := c.Start(startCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, c) })
	cancelStart()
	requireStillIndexing(t, &harness{c: c, states: states, client: client, logs: logs})
}

// requireStillIndexing writes an entity with an edge and waits until its outgoing row is stored,
// then checks the component reads running and healthy.
func requireStillIndexing(t *testing.T, h *harness) {
	t.Helper()
	src, target := entityID(1), entityID(2)
	h.put(t, src, entityValue(t, src, stmt{linkTo, target}))
	await(t, "the entity indexed",
		func(context.Context) ([]string, error) { return h.keys(t, graph.BucketOutgoingIndex), nil },
		func(keys []string) bool { return slices.Contains(keys, src) })
	if health := h.c.Health(); !health.Healthy || health.Status != "running" {
		t.Errorf("Health = %+v, want running and healthy", health)
	}
}

// TestHealthIgnoresErrorCountAndTracksStuckStates: Healthy is false exactly while a stuck state
// holds, and ErrorCount never lowers it (spec "Health reads named stuck states only"; ADR-109 D2).
// A required write that fails lowers Healthy, naming it; its repair raises Healthy again while
// ErrorCount stays at one. A latched reset lowers Healthy, naming it.
func TestHealthIgnoresErrorCountAndTracksStuckStates(t *testing.T) {
	t.Run("a failed write and its repair", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		js, err := h.client.JetStream()
		if err != nil {
			t.Fatal(err)
		}
		if err := js.DeleteKeyValue(t.Context(), graph.BucketIncomingIndex); err != nil {
			t.Fatalf("delete %s: %v", graph.BucketIncomingIndex, err)
		}
		src := entityID(1)
		h.put(t, src, entityValue(t, src, stmt{linkTo, entityID(2)}))
		health := await(t, "Health reports the unresolved write",
			func(context.Context) (component.HealthStatus, error) { return h.c.Health(), nil },
			func(got component.HealthStatus) bool { return !got.Healthy })
		if health.Status != "degraded" || !strings.Contains(health.LastError, "1 index writes unresolved") {
			t.Errorf("Health with a failed write = %+v, want degraded naming the unresolved write", health)
		}

		if _, err := kvcatalog.EnsureCatalogBucket(t.Context(), h.client, graph.BucketIncomingIndex); err != nil {
			t.Fatalf("recreate %s: %v", graph.BucketIncomingIndex, err)
		}
		h.c.repairFailedEntities(t.Context())
		health = await(t, "Health after the repair",
			func(context.Context) (component.HealthStatus, error) { return h.c.Health(), nil },
			func(got component.HealthStatus) bool { return got.Healthy })
		if health.Status != "running" || health.ErrorCount != 1 || h.c.failedTotal() != 0 {
			t.Errorf("Health after the repair = %+v with %d unresolved, want running, ErrorCount 1, none unresolved",
				health, h.c.failedTotal())
		}
	})
	t.Run("a latched reset", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		h.put(t, entityID(1), entityValue(t, entityID(3), stmt{armed, true}))
		want := "graph state reset required: " + string(graph.GraphStateReasonNoncanonicalEntityID)
		await(t, "Health names the reset",
			func(context.Context) (component.HealthStatus, error) { return h.c.Health(), nil },
			func(got component.HealthStatus) bool { return !got.Healthy && strings.Contains(got.LastError, want) })
	})
}

// TestLatchBootstrap_ContinuousWrite (pin:processor/graph-index/watermark_test.go:271): the
// bootstrap latch sets at the target fixed when the startup replay ended, while writes continue
// and the live target stays ahead of the indexed floor, so the envelope never reads ready (ADR-109
// D4). The pin set the follower's fields by hand; here graph-index replays 200 stored entities
// while a writer keeps writing faster than one lane indexes, and the envelope graph-index
// publishes is read from GRAPH_STATUS.
func TestLatchBootstrap_ContinuousWrite(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	for n := 1; n <= 200; n++ {
		id := entityID(n)
		if _, err := states.Put(t.Context(), id, entityValue(t, id, stmt{armed, true})); err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
	}
	c, _ := newIndex(t, client, `{}`) // one lane
	startComponent(t, c)

	writeCtx, stopWriting := context.WithCancel(t.Context())
	var writer sync.WaitGroup
	defer writer.Wait()
	defer stopWriting()
	writer.Go(func() {
		for n := 0; writeCtx.Err() == nil; n++ {
			id := entityID(n % 100)
			if _, err := states.Put(writeCtx, id, entityValue(t, id, stmt{armed, n%2 == 0})); err != nil && writeCtx.Err() == nil {
				t.Errorf("put %s: %v", id, err)
				return
			}
		}
	})

	status := await(t, "an envelope reporting bootstrap complete",
		func(ctx context.Context) (graph.IndexStatusResponse, error) { return publishedEnvelope(ctx, client) },
		func(s graph.IndexStatusResponse) bool { return s.BootstrapComplete })
	if status.Ready || status.IndexedRevision >= status.TargetRevision {
		t.Fatalf("fixture no longer models continuous write: the latching envelope %+v is caught up", status)
	}
}

// publishedEnvelope reads graph-index's readiness envelope from GRAPH_STATUS, as a remote reader
// does.
func publishedEnvelope(ctx context.Context, client *natsclient.Client) (graph.IndexStatusResponse, error) {
	js, err := client.JetStream()
	if err != nil {
		return graph.IndexStatusResponse{}, err
	}
	kv, err := js.KeyValue(ctx, graph.BucketGraphStatus)
	if err != nil {
		return graph.IndexStatusResponse{}, err
	}
	entry, err := kv.Get(ctx, key)
	if err != nil {
		return graph.IndexStatusResponse{}, err
	}
	var s graph.IndexStatusResponse
	if err := json.Unmarshal(entry.Value(), &s); err != nil {
		return graph.IndexStatusResponse{}, fmt.Errorf("decode envelope %q: %w", entry.Value(), err)
	}
	return s, nil
}
