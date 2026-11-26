//go:build integration

package graphindex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// suiteOwner is graph-index as the lifecycle suite's owner. Observe lists what graph-index itself
// holds; the follower's watch is private to graph/follower, so TestGraphIndexLifecycleSuite reads
// it from the server instead of through a test-only export.
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
// start acquires nothing it can fail to release, so the branch where that cleanup fails has no
// case here. graph-index is one-shot (a Start after Stop is refused), so no restart
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

// laneHold holds the one lane inside the first entity it indexes: holdFirstIndexed sends that
// entity on entered once its index rows are written, then waits for release.
type laneHold struct {
	once    sync.Once
	entered chan string
	release chan struct{}
}

// holdFirstIndexed is the component's log handler. The lane logs "indexed entity" after an
// entity's writes and before its revision completes, so blocking that record holds the lane
// with one entity in flight and the rest queued behind it.
type holdFirstIndexed struct {
	slog.Handler
	hold *laneHold
}

func (h holdFirstIndexed) Handle(ctx context.Context, r slog.Record) error {
	err := h.Handler.Handle(ctx, r)
	if r.Message != "indexed entity" {
		return err
	}
	first := false
	h.hold.once.Do(func() { first = true })
	if first {
		var id string
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "entity" {
				id = a.Value.String()
			}
			return true
		})
		h.hold.entered <- id
		<-h.hold.release
	}
	return err
}

func (h holdFirstIndexed) WithAttrs(attrs []slog.Attr) slog.Handler {
	return holdFirstIndexed{h.Handler.WithAttrs(attrs), h.hold}
}

func (h holdFirstIndexed) WithGroup(name string) slog.Handler {
	return holdFirstIndexed{h.Handler.WithGroup(name), h.hold}
}

// heldBacklog is graph-index on one lane, held inside the first of stored entities with the
// rest queued behind it.
type heldBacklog struct {
	c            *Component
	h            *harness
	logs         *logBuffer
	inFlight     string            // the entity the lane is held inside, its rows written
	revisions    map[string]uint64 // each stored entity's revision
	errorsBefore int
	releaseLane  func()
}

// startHeldBacklog stores the entities, starts graph-index and returns once the lane is held
// inside the first and every entry is counted. The follower calls Deliver for one entry at a
// time, and deliver counts an entry before it submits it, so the last entry counted means every
// earlier entry's submit has returned: at least stored-2 entities wait behind the one in flight.
func startHeldBacklog(t *testing.T, client *natsclient.Client, stored int) *heldBacklog {
	t.Helper()
	states := ensureEntityStates(t, client)
	target := entityID(999)
	revisions := make(map[string]uint64, stored)
	for n := 1; n <= stored; n++ {
		id := entityID(n)
		rev, err := states.Put(t.Context(), id, entityValue(t, id, stmt{linkTo, target}))
		if err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
		revisions[id] = rev
	}

	hold := &laneHold{entered: make(chan string, 1), release: make(chan struct{})}
	releaseLane := sync.OnceFunc(func() { close(hold.release) })
	logs := &logBuffer{}
	built, err := CreateGraphIndex(json.RawMessage(`{}`), component.Dependencies{ // one lane
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
		Logger: slog.New(holdFirstIndexed{
			Handler: slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}),
			hold:    hold,
		}),
	})
	if err != nil {
		t.Fatalf("CreateGraphIndex: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	startComponent(t, c)
	t.Cleanup(releaseLane) // runs before startComponent's Stop

	inFlight := <-hold.entered
	await(t, "every entry delivered",
		func(context.Context) (int64, error) { return c.DataFlow().Messages, nil },
		func(n int64) bool { return n >= int64(stored) })
	return &heldBacklog{
		c: c, h: &harness{c: c, f: c.follower.Load(), client: client}, logs: logs,
		inFlight: inFlight, revisions: revisions, errorsBefore: c.Health().ErrorCount,
		releaseLane: releaseLane,
	}
}

// requireQueuedDropped checks, after a Stop that returned nil, so with every lane joined, that
// only the entity in flight was written: no queued entity has rows, none is marked failed, no
// queued revision is completed, and nothing is logged as a warning.
func (b *heldBacklog) requireQueuedDropped(t *testing.T) {
	t.Helper()
	if written := b.h.keys(t, graph.BucketOutgoingIndex); !slices.Equal(written, []string{b.inFlight}) {
		t.Errorf("outgoing rows after Stop = %d entities, want only the one in flight (%s)", len(written), b.inFlight)
	}
	if got := b.c.failedTotal(); got != 0 {
		t.Errorf("%d entities marked failed after Stop, want 0", got)
	}
	lowestQueued := uint64(0)
	for id, rev := range b.revisions {
		if id != b.inFlight && (lowestQueued == 0 || rev < lowestQueued) {
			lowestQueued = rev
		}
	}
	if got := b.h.f.Indexed(); got >= lowestQueued {
		t.Errorf("indexed floor = %d after Stop, want below %d: a queued revision was completed", got, lowestQueued)
	}
	if got := b.c.Health().ErrorCount; got != b.errorsBefore {
		t.Errorf("ErrorCount = %d after Stop, want %d", got, b.errorsBefore)
	}
	out := b.logs.String()
	if strings.Contains(out, "readiness withheld") {
		t.Errorf("Stop logged readiness withheld:\n%s", out)
	}
	for _, level := range []string{"level=WARN", "level=ERROR"} {
		if strings.Contains(out, level) {
			t.Errorf("Stop logged at %s:\n%s", level, out)
		}
	}
}

// TestStopDropsQueuedWork: a Stop with a backlog writes the entity already in flight and
// leaves the queued ones alone. They are not read, not written, not marked failed, and their
// revisions are not completed, and nothing is logged as a warning. The next boot replays every
// stored entity, so nothing is lost.
//
// The one lane is held inside the first entity until the first Stop, under a context that has
// already ended, has run its cleanup: that Stop returns the context's error and has begun the
// pool's shutdown. The lane is then let go, and a second Stop, under a short context, returns
// nil, so a Stop its context interrupted is finished by the next one.
func TestStopDropsQueuedWork(t *testing.T) {
	b := startHeldBacklog(t, fixtureClient(t), 50)

	ended, cancelEnded := context.WithDeadline(t.Context(), time.Unix(1, 0))
	defer cancelEnded()
	if err := b.c.Stop(ended); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop(ended) = %v, want context.DeadlineExceeded with the lane held", err)
	}
	b.releaseLane()
	// The deadline is the behaviour under test: with the queue dropped, Stop waits for one
	// entity's writes, not for the backlog.
	short, cancelShort := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelShort()
	if err := b.c.Stop(short); err != nil {
		t.Fatalf("retried Stop under a short context = %v, want nil", err)
	}
	b.requireQueuedDropped(t)
}

// TestStopDropsQueuedWorkFromItsStart: the cutoff is where Stop begins, not where the pool
// shuts down. A query handler that is still running holds Stop in its subscription drain; the
// lane, let go during that drain, must still leave the queued entities alone.
func TestStopDropsQueuedWorkFromItsStart(t *testing.T) {
	client := fixtureClient(t)
	b := startHeldBacklog(t, client, 50)

	entered := make(chan struct{})
	enteredOnce := sync.OnceFunc(func() { close(entered) })
	release := make(chan struct{})
	releaseHandler := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseHandler)
	const stuckSubject = "test.graph-index.stop-cutoff"
	stuck, err := client.SubscribeForRequests(t.Context(), stuckSubject,
		func(context.Context, []byte) ([]byte, error) {
			enteredOnce()
			<-release
			return nil, nil
		})
	if err != nil {
		t.Fatalf("subscribe %s: %v", stuckSubject, err)
	}
	// First, so its drain is the first thing Stop waits on.
	b.c.subscriptions = append([]*natsclient.Subscription{stuck}, b.c.subscriptions...)
	if err := client.GetConnection().PublishRequest(stuckSubject, "test.graph-index.reply", nil); err != nil {
		t.Fatalf("publish %s: %v", stuckSubject, err)
	}
	<-entered

	stopCtx, cancelStop := context.WithTimeout(t.Context(), stopBound)
	defer cancelStop()
	stopped := make(chan error, 1)
	go func() { stopped <- b.c.Stop(stopCtx) }()
	// The flag is the event that orders the lane's release after Stop has begun: component.Lifecycle
	// reports "stopped" before it calls cleanup, so Health cannot order it. The held handler keeps
	// Stop in its first drain until releaseHandler.
	if _, err := probe.Await(t.Context(),
		func(context.Context) (bool, error) { return b.c.stopping.Load(), nil },
		func(set bool) bool { return set }); err != nil {
		t.Fatalf("Stop began dropping queued work: %v", err)
	}
	b.releaseLane()
	releaseHandler()
	if err := <-stopped; err != nil {
		t.Fatalf("Stop = %v, want nil", err)
	}
	b.requireQueuedDropped(t)
}

// TestStopWithAStuckHandlerStopsEveryOtherVerb: a handler that outlasts Stop's context does not
// leave the other verbs answering. The stuck subscription is put first, so its drain is the first
// to fail. A later Stop, once the handler has returned, finishes.
func TestStopWithAStuckHandlerStopsEveryOtherVerb(t *testing.T) {
	client := fixtureClient(t)
	h := startIndex(t, client, `{}`)

	entered := make(chan struct{})
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce)
	const stuckSubject = "test.graph-index.stuck"
	stuck, err := client.SubscribeForRequests(t.Context(), stuckSubject,
		func(context.Context, []byte) ([]byte, error) {
			close(entered)
			<-release
			return nil, nil
		})
	if err != nil {
		t.Fatalf("subscribe %s: %v", stuckSubject, err)
	}
	h.c.subscriptions = append([]*natsclient.Subscription{stuck}, h.c.subscriptions...)

	if err := client.GetConnection().PublishRequest(stuckSubject, "test.graph-index.reply", nil); err != nil {
		t.Fatalf("publish %s: %v", stuckSubject, err)
	}
	<-entered

	stopCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- h.c.Stop(stopCtx) }()
	// component.Lifecycle reports "stopped" as soon as cleanup begins, ordering cancel after Stop
	// entered cleanup while the stuck handler holds the first subscription drain.
	if _, err := probe.Await(t.Context(),
		func(context.Context) (string, error) { return h.c.Health().Status, nil },
		func(status string) bool { return status == "stopped" }); err != nil {
		t.Fatalf("Stop entered: %v", err)
	}
	cancel()
	if err := <-stopped; !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop with a stuck handler = %v, want context.Canceled", err)
	}
	if err := h.c.pool.SubmitBlocking(t.Context(), "probe", indexWork{}); !errors.Is(err, dispatch.ErrStopped) {
		t.Fatalf("pool SubmitBlocking after stuck-handler Stop = %v, want dispatch.ErrStopped", err)
	}
	feedBound, cancelFeedBound := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancelFeedBound()
	select {
	case <-h.c.feeds:
	case <-feedBound.Done():
		t.Fatal("Stop with a stuck handler did not cancel feeds' runCtx")
	}

	// Every drain has begun when Stop returns, but the server drops a subscription only once it has
	// read the unsubscribe, so this waits for the verbs to go quiet.
	var verbs []string
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphIndex {
			verbs = append(verbs, verb.Subject)
		}
	}
	if _, err := probe.Await(t.Context(),
		func(context.Context) ([]string, error) {
			var answering []string
			for _, subject := range verbs {
				_, err := client.GetConnection().Request(subject, []byte(`{}`), requestTimeout)
				if !errors.Is(err, nats.ErrNoResponders) {
					answering = append(answering, subject)
				}
			}
			return answering, nil
		},
		func(answering []string) bool { return len(answering) == 0 }); err != nil {
		t.Fatalf("no graph-index verb answering: %v", err)
	}

	releaseOnce()
	stopComponent(t, h.c)
}

// TestSecondStartIsRefused: a second Start on a running graph-index returns errs.ErrAlreadyStarted
// and the component keeps running: it still indexes what is written (spec "One Start per
// instance"; ADR-108 D3(a)).
func TestSecondStartIsRefused(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	if err := h.c.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("second Start = %v, want ErrAlreadyStarted", err)
	}
	requireStillIndexing(t, h)
}

// TestStartContextEndStopsNothing: ending the context given to Start after Start returned stops
// nothing; graph-index still indexes what is written (ADR-113).
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
// ErrorCount stays at one. A damaged entity lowers Healthy, naming it.
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
		if health.Status != "degraded" || !strings.Contains(health.LastError, "1 unresolved failures") {
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
	t.Run("a damaged entity", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		h.put(t, entityID(1), entityValue(t, entityID(3), stmt{armed, true}))
		want := "1 damaged entity: " + entityID(1) + " (" + string(graph.GraphStateReasonNoncanonicalEntityID) + ")"
		await(t, "Health names the damaged entity",
			func(context.Context) (component.HealthStatus, error) { return h.c.Health(), nil },
			func(got component.HealthStatus) bool { return !got.Healthy && strings.Contains(got.LastError, want) })
	})
}

// TestLatchBootstrap_ContinuousWrite: the bootstrap latch sets at the target fixed when the
// startup replay ended, while writes continue and the live target stays ahead of the indexed
// floor, so the envelope never reads ready (ADR-109 D4). graph-index replays 200 stored entities
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
