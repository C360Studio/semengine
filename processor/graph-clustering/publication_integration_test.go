//go:build integration

package graphclustering

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/natsclient"
)

// mark is what a COMMUNITY_INDEX record says about its group: how many members, and the revision
// every write at or below which it reflects.
type mark struct {
	members  int
	revision uint64
}

// readRecords reads every COMMUNITY_INDEX record through a reader of the test's own, keyed
// "<level>.<group ID>". It returns an error rather than failing, so a wait can poll it while the
// bucket is gone.
func readRecords(ctx context.Context, client *natsclient.Client) (map[string]mark, error) {
	reader, err := kvcatalog.OpenCatalogReader(ctx, client, graph.BucketCommunityIndex)
	if err != nil {
		return nil, err
	}
	keys, err := natsclient.FilteredKeys(ctx, reader, ">")
	if err != nil {
		return nil, err
	}
	marks := map[string]mark{}
	for _, k := range keys {
		entry, err := reader.Get(ctx, k)
		if err != nil {
			return nil, err
		}
		record, err := clustering.DecodeCommunity(k, entry.Value())
		if err != nil {
			return nil, err
		}
		marks[fmt.Sprintf("%d.%s", record.Level, record.ID)] = mark{record.MemberCount, record.EntityRevision}
	}
	return marks, nil
}

func records(t *testing.T, client *natsclient.Client) map[string]mark {
	t.Helper()
	marks, err := readRecords(t.Context(), client)
	if err != nil {
		t.Fatalf("read %s: %v", graph.BucketCommunityIndex, err)
	}
	return marks
}

// lastRevision is the last revision written to kv, read from its backing stream.
func lastRevision(t *testing.T, kv jetstream.KeyValue) uint64 {
	t.Helper()
	status, err := kv.Status(t.Context())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	return status.(*jetstream.KeyValueBucketStatus).StreamInfo().State.LastSeq
}

// editedEntity is storedEntity for id with test.state.value set to value.
func editedEntity(id string, value int) []byte {
	return []byte(`{"id":"` + id + `","triples":[{"subject":"` + id +
		`","predicate":"test.state.value","object":` + fmt.Sprint(value) + `,"source":"test","timestamp":"2026-11-10T12:00:00Z"}]}`)
}

// awaitIndexedInEnvelope waits for graph-clustering's GRAPH_STATUS envelope to carry an
// indexed_revision of at least rev; the follower writes it on its heartbeat.
func awaitIndexedInEnvelope(t *testing.T, client *natsclient.Client, rev uint64) {
	t.Helper()
	await(t, fmt.Sprintf("an envelope with indexed_revision %d", rev),
		func(ctx context.Context) (graph.IndexStatusResponse, error) {
			status, _, err := envelope(ctx, client)
			return status, err
		}, func(s graph.IndexStatusResponse) bool { return s.IndexedRevision >= rev })
}

// TestRecordsFollowChanges: an entity written, edited and deleted, with a tick after each: its
// groups' records follow each write with entity_revision at that write's revision, and the group
// its delete empties is removed while the groups it shares with another entity stay. Before each
// tick, the graph-clustering key's indexed_revision already reaches the write while the record
// still carries the earlier mark (graph-clustering spec, "Upsert, edit, delete" and "Applied before
// the tick"). The expected records are worked out by hand from the writes.
func TestRecordsFollowChanges(t *testing.T) {
	client := fixtureClient(t)
	c, states := startClustering(t, client, tickByHand)
	const a1, b1 = "acme.ops.test.alpha.doc.a1", "acme.ops.test.beta.doc.b1"

	r1 := put(t, states, a1, storedEntity(a1))
	awaitApplied(t, c, r1)
	awaitIndexedInEnvelope(t, client, r1)
	if got := records(t, client); len(got) != 0 {
		t.Fatalf("records before the first tick = %v; want none", got)
	}
	c.tick(t.Context())
	want := map[string]mark{"0.acme.ops.test.alpha": {1, r1}, "1.acme.ops.test": {1, r1}, "2.acme.ops": {1, r1}}
	if got := records(t, client); !maps.Equal(got, want) {
		t.Fatalf("records after the write = %v; want %v", got, want)
	}

	r2 := put(t, states, a1, editedEntity(a1, 2))
	awaitApplied(t, c, r2)
	awaitIndexedInEnvelope(t, client, r2)
	if got := records(t, client); !maps.Equal(got, want) {
		t.Fatalf("records after the edit, before its tick = %v; want still %v", got, want)
	}
	c.tick(t.Context())
	want = map[string]mark{"0.acme.ops.test.alpha": {1, r2}, "1.acme.ops.test": {1, r2}, "2.acme.ops": {1, r2}}
	if got := records(t, client); !maps.Equal(got, want) {
		t.Fatalf("records after the edit = %v; want %v", got, want)
	}

	r3 := put(t, states, b1, storedEntity(b1))
	awaitApplied(t, c, r3)
	c.tick(t.Context())
	if err := states.Delete(t.Context(), a1); err != nil {
		t.Fatalf("delete %s: %v", a1, err)
	}
	r4 := lastRevision(t, states) // the delete marker's: nothing else writes the bucket
	awaitApplied(t, c, r4)
	c.tick(t.Context())
	want = map[string]mark{"0.acme.ops.test.beta": {1, r3}, "1.acme.ops.test": {1, r4}, "2.acme.ops": {1, r4}}
	if got := records(t, client); !maps.Equal(got, want) {
		t.Fatalf("records after the delete = %v; want %v, the alpha group removed", got, want)
	}
}

// TestNothingPublishedBeforeReplay: graph-clustering started on a populated ENTITY_STATES with a
// ticker every millisecond publishes nothing until the replay is applied, so the first record
// written for every group already counts all its members (graph-clustering spec, "During the
// replay"). A test watch on COMMUNITY_INDEX, opened before Start on the empty bucket, sees every
// write in order; a tick that published during the replay would write a group before all its
// members were applied. The replay of 2,000 entities spans many ticks.
func TestNothingPublishedBeforeReplay(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	const groups, perGroup = 40, 50
	want := map[string]int{"1.acme.ops.test": groups * perGroup, "2.acme.ops": groups * perGroup}
	for g := range groups {
		want[fmt.Sprintf("0.acme.ops.test.g%02d", g)] = perGroup
		for e := range perGroup {
			id := fmt.Sprintf("acme.ops.test.g%02d.doc.e%02d", g, e)
			put(t, states, id, storedEntity(id))
		}
	}
	index, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketCommunityIndex)
	if err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketCommunityIndex, err)
	}
	watcher, err := index.WatchAll(t.Context())
	if err != nil {
		t.Fatalf("watch %s: %v", graph.BucketCommunityIndex, err)
	}
	t.Cleanup(func() { _ = watcher.Stop() })

	c := newClustering(t, client, `{"detection_interval": "1ms"}`)
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, c) })

	first := map[string]int{}
	for len(first) < len(want) {
		entry := <-watcher.Updates()
		if entry == nil {
			continue // the end of the empty bucket's snapshot
		}
		if entry.Operation() != jetstream.KeyValuePut {
			t.Fatalf("COMMUNITY_INDEX %s removed (%v) during the build; want only puts", entry.Key(), entry.Operation())
		}
		record, err := clustering.DecodeCommunity(entry.Key(), entry.Value())
		if err != nil {
			t.Fatalf("decode %s: %v", entry.Key(), err)
		}
		key := fmt.Sprintf("%d.%s", record.Level, record.ID)
		if _, seen := first[key]; !seen {
			first[key] = record.MemberCount
		}
	}
	partial := map[string]string{}
	for group, members := range first {
		if members != want[group] {
			partial[group] = fmt.Sprintf("%d of %d", members, want[group])
		}
	}
	if len(partial) > 0 {
		t.Fatalf("groups first written with members missing: %v; want every member applied before any publication", partial)
	}
}

// TestLostFeedStopsPublication: once the follower declares the feed lost, a tick that starts
// afterwards writes nothing, though the maintainer holds a change no tick has published (graph-
// clustering spec, "A revision out of order"). The trigger here is the one an operator causes,
// ENTITY_STATES deleted under the running component; a revision delivered out of order is declared
// lost by the same latch, which the follower's own test drives
// (TestFollowerDeclaresLostFeedOnOutOfOrderDelivery).
func TestLostFeedStopsPublication(t *testing.T) {
	client := fixtureClient(t)
	c, states := startClustering(t, client, tickByHand)
	const a1, a2 = "acme.ops.test.alpha.doc.a1", "acme.ops.test.alpha.doc.a2"

	r1 := put(t, states, a1, storedEntity(a1))
	awaitApplied(t, c, r1)
	c.tick(t.Context())
	before := records(t, client)
	if before["0.acme.ops.test.alpha"].members != 1 {
		t.Fatalf("records after the first tick = %v; want the alpha group with one member", before)
	}
	r2 := put(t, states, a2, storedEntity(a2))
	awaitApplied(t, c, r2) // the maintainer holds a2; no tick has published it

	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketEntityStates); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketEntityStates, err)
	}
	// The watch was observed to close within two of nats.go's activity checks (18 s at the pinned
	// nats.go); nats.go does not promise it, so the wait has no deadline of its own.
	await(t, "Health naming the lost feed",
		func(context.Context) (component.HealthStatus, error) { return c.Health(), nil },
		func(s component.HealthStatus) bool { return strings.Contains(s.LastError, "ENTITY_STATES feed lost") })

	c.tick(t.Context())
	if got := records(t, client); !maps.Equal(got, before) {
		t.Fatalf("records after a tick on the lost feed = %v; want them unchanged, %v", got, before)
	}
}

// TestResetStopsPublication: a stored entity the replay refuses latches the follower's reset, and a
// tick that starts afterwards writes nothing, though the maintainer holds the valid entity beside
// it (graph-clustering spec, "Publication waits for the replay and stops on a broken feed").
func TestResetStopsPublication(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	const a1, b1 = "acme.ops.test.alpha.doc.a1", "acme.ops.test.beta.doc.b1"
	put(t, states, a1, storedEntity(a1))
	rev := put(t, states, b1, []byte("not an entity state"))

	c := newClustering(t, client, tickByHand)
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, c) })
	// The follower latches the reset before it delivers the refused entity, and delivers in
	// order, so once both revisions are applied the reset is latched and a1 is held.
	await(t, "both stored entities applied",
		func(context.Context) (uint64, error) { return c.follower.Load().Indexed(), nil },
		func(indexed uint64) bool { return indexed >= rev })
	if health := c.Health(); !strings.Contains(health.LastError, "graph state reset required") {
		t.Fatalf("Health = %+v; want the latched reset named", health)
	}

	c.tick(t.Context())
	if got := records(t, client); len(got) != 0 {
		t.Fatalf("records after a tick on a latched reset = %v; want none", got)
	}
}

// heldLines holds every log record with message inside the component's logger, one at a time,
// until the test releases it: the goroutine that logs it waits there. Each hold sends on entered
// and then waits for a receive from release; after done closes, nothing more is held.
type heldLines struct {
	message string
	entered chan struct{}
	release chan struct{}
	done    chan struct{}
}

type repeatHoldingHandler struct {
	inner slog.Handler
	held  *heldLines
}

func (h repeatHoldingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h repeatHoldingHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == h.held.message {
		select {
		case h.held.entered <- struct{}{}:
			select {
			case <-h.held.release:
			case <-h.held.done:
			}
		case <-h.held.done:
		}
	}
	return h.inner.Handle(ctx, r)
}

func (h repeatHoldingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return repeatHoldingHandler{inner: h.inner.WithAttrs(attrs), held: h.held}
}

func (h repeatHoldingHandler) WithGroup(name string) slog.Handler {
	return repeatHoldingHandler{inner: h.inner.WithGroup(name), held: h.held}
}

// publicationFailures reads graph_clustering_publication_failures_total from the registry the
// component was built with.
func publicationFailures(t *testing.T, c *Component) float64 {
	t.Helper()
	const name = "semengine_graph_clustering_publication_failures_total"
	families, err := c.metricsRegistry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() == name && len(family.GetMetric()) == 1 {
			return family.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatalf("the component's registry does not gather %s", name)
	return 0
}

// TestPersistentPublicationFailureIsStuck: while every tick's write to COMMUNITY_INDEX fails,
// Health is not healthy and names publication_failed, the envelope reads degraded with
// publication_failed 1, and the counter rises by one per tick; once a tick succeeds, Health is
// healthy, the record is written and the envelope is ready again (graph-clustering spec, "Every
// write fails"; ADR-109). The writes fail because COMMUNITY_INDEX is deleted under the running
// component, and succeed once it is created again. Each failed tick is held in its own log line,
// which the component writes after counting the failure and keeping it, so every reading below is
// taken with that tick's outcome recorded and no later tick run.
func TestPersistentPublicationFailureIsStuck(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	held := &heldLines{
		message: "community publication failed; the changed groups are retried on the next tick",
		entered: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
	c := newClusteringLogging(t, client, `{"detection_interval": "20ms"}`,
		repeatHoldingHandler{inner: slog.NewTextHandler(io.Discard, nil), held: held})
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, c) })
	t.Cleanup(func() { close(held.done) }) // runs first: a failed test never leaves the Stop above waiting on a hold
	awaitApplied(t, c, 0)                  // the write below is live (#584)

	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketCommunityIndex); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketCommunityIndex, err)
	}
	const a1 = "acme.ops.test.alpha.doc.a1"
	awaitApplied(t, c, put(t, states, a1, storedEntity(a1)))

	for failed := 1; failed <= 3; failed++ {
		<-held.entered // a failed tick is held in its log line
		if n := publicationFailures(t, c); n != float64(failed) {
			t.Fatalf("publication_failures_total = %v at failed tick %d; want %d", n, failed, failed)
		}
		if health := c.Health(); health.Healthy || !strings.Contains(health.LastError, "publication_failed 1") {
			t.Fatalf("Health at failed tick %d = %+v; want unhealthy naming publication_failed 1", failed, health)
		}
		if failed == 1 {
			status := await(t, "an envelope naming publication_failed",
				func(ctx context.Context) (graph.IndexStatusResponse, error) {
					status, _, err := envelope(ctx, client)
					return status, err
				}, func(s graph.IndexStatusResponse) bool { return s.FailedCount > 0 })
			if status.State != graph.IndexStateDegraded ||
				!maps.Equal(status.FailedReasons, map[string]uint64{"publication_failed": 1}) {
				t.Fatalf("envelope = %+v; want degraded, failed_reasons {publication_failed: 1}", status)
			}
		}
		if failed == 3 {
			if _, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketCommunityIndex); err != nil {
				t.Fatalf("create %s again: %v", graph.BucketCommunityIndex, err)
			}
		}
		held.release <- struct{}{}
	}

	await(t, "a successful tick: Health healthy and the alpha record written",
		func(ctx context.Context) (bool, error) {
			marks, err := readRecords(ctx, client)
			return c.Health().Healthy && marks["0.acme.ops.test.alpha"].members == 1, err
		}, func(ok bool) bool { return ok })
	if n := publicationFailures(t, c); n != 3 {
		t.Fatalf("publication_failures_total = %v after the success; want 3", n)
	}
	await(t, "an envelope ready again",
		func(ctx context.Context) (graph.IndexStatusResponse, error) {
			status, _, err := envelope(ctx, client)
			return status, err
		}, func(s graph.IndexStatusResponse) bool { return s.FailedCount == 0 && s.State == graph.IndexStateReady })
}

// TestStopWaitsForTick: Stop joins the ticker, so it does not return nil while a tick is still
// running. A failed tick is held in its log line, on the ticker's goroutine; a Stop whose context
// ends meanwhile returns that context's error and keeps its handles; once the tick is released, the
// next Stop returns nil and nothing of the component is left running (graph-clustering spec, "Stop
// leaves nothing").
func TestStopWaitsForTick(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	held := &heldLines{
		message: "community publication failed; the changed groups are retried on the next tick",
		entered: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
	c := newClusteringLogging(t, client, `{"detection_interval": "20ms"}`,
		repeatHoldingHandler{inner: slog.NewTextHandler(io.Discard, nil), held: held})
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, c) })
	unhold := sync.OnceFunc(func() { close(held.done) })
	t.Cleanup(unhold) // runs first: a failed test never leaves the Stop above waiting on the hold
	awaitApplied(t, c, 0)

	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketCommunityIndex); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketCommunityIndex, err)
	}
	put(t, states, "acme.ops.test.alpha.doc.a1", storedEntity("acme.ops.test.alpha.doc.a1"))
	<-held.entered // a failed tick is held in its log line, on the ticker's goroutine

	// The deadline is the behaviour under test: Stop must outlast it while the tick is held.
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if err := c.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop while a tick is held = %v; want context.DeadlineExceeded, Stop waiting for the ticker", err)
	}
	if held := (suiteOwner{c}).Observe().Unresolved; len(held) == 0 {
		t.Fatal("Stop that ended early released its handles; want them kept for the next Stop")
	}

	unhold() // releases the held tick, and holds no later one
	stopComponent(t, c)
	if held := (suiteOwner{c}).Observe().Unresolved; len(held) != 0 {
		t.Fatalf("held after a nil Stop = %v, want nothing", held)
	}
	await(t, "no goroutine in a Component or Follower method after Stop",
		func(context.Context) (string, error) { return componentStacks(), nil },
		func(stacks string) bool { return stacks == "" })
}
