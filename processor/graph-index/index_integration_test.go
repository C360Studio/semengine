//go:build integration

package graphindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

// waitBound is the real-clock failure bound of every wait on the server.
const waitBound = 60 * time.Second

func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(15*time.Second),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

// harness is a running write path: the component built from JSON through CreateGraphIndex, its
// follower, and the ENTITY_STATES write handle the test uses in graph-ingest's place.
type harness struct {
	c      *Component
	f      *follower.Follower
	states jetstream.KeyValue
	client *natsclient.Client
	logs   *logBuffer // everything the component logs
}

// logBuffer is the component's log output, safe to write from the lanes while a test reads it.
type logBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// startIndex builds graph-index from raw and wires the write path in the order task 4.3's Start
// will: the follower, the index buckets, the pool, then the follower's Run, the coalescer and the
// repair loop. The cleanup stops them in Stop's order: the feed first, then the pool's Shutdown
// while the pool's context is still live, so accepted work is written, then the pool's context.
func startIndex(t *testing.T, client *natsclient.Client, raw string) *harness {
	t.Helper()
	states, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketEntityStates, err)
	}
	logs := &logBuffer{}
	built, err := CreateGraphIndex(json.RawMessage(raw), component.Dependencies{
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatalf("CreateGraphIndex: %v", err)
	}
	c := built.(*Component)
	f, err := follower.Open(t.Context(), follower.Config{
		Client:          client,
		Key:             key,
		MetricsRegistry: c.metricsRegistry,
		Logger:          c.logger,
		StartupAttempts: c.config.StartupAttempts,
		StartupInterval: time.Duration(c.config.StartupInterval) * time.Millisecond,
		Deliver:         c.deliver,
		Failed:          c.failedTotal,
	})
	if err != nil {
		t.Fatalf("follower.Open: %v", err)
	}
	if err := c.openIndexBuckets(t.Context()); err != nil {
		t.Fatalf("openIndexBuckets: %v", err)
	}
	poolCtx, poolCancel := context.WithCancel(context.Background())
	if err := c.startIndexPool(poolCtx, f); err != nil {
		poolCancel()
		t.Fatalf("startIndexPool: %v", err)
	}
	feedCtx, feedCancel := context.WithCancel(t.Context())
	var feeds sync.WaitGroup
	feeds.Go(func() {
		if err := f.Run(feedCtx); err != nil {
			t.Errorf("follower Run: %v", err)
		}
	})
	if c.coalescer != nil {
		feeds.Go(func() { c.coalescer.run(feedCtx) })
	}
	feeds.Go(func() { c.repairLoop(feedCtx) })
	t.Cleanup(func() {
		feedCancel()
		joined := make(chan struct{})
		go func() { feeds.Wait(); close(joined) }()
		select {
		case <-joined:
		case <-time.After(waitBound):
			t.Errorf("feeds did not return within %s", waitBound)
		}
		ctx, cancel := context.WithTimeout(context.Background(), waitBound)
		defer cancel()
		if err := c.pool.Shutdown(ctx); err != nil {
			t.Errorf("pool Shutdown: %v", err)
		}
		poolCancel()
	})
	return &harness{c: c, f: f, states: states, client: client, logs: logs}
}

// stmt is one statement of a stored entity.
type stmt struct {
	predicate string
	object    any
}

// entityValue is the ENTITY_STATES value for id holding stmts, each with its source and timestamp.
func entityValue(t *testing.T, id string, stmts ...stmt) []byte {
	t.Helper()
	type triple struct {
		Subject   string    `json:"subject"`
		Predicate string    `json:"predicate"`
		Object    any       `json:"object"`
		Source    string    `json:"source"`
		Timestamp time.Time `json:"timestamp"`
	}
	triples := make([]triple, 0, len(stmts))
	for _, s := range stmts {
		triples = append(triples, triple{id, s.predicate, s.object, "test", time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)})
	}
	data, err := json.Marshal(map[string]any{"id": id, "triples": triples})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func entityID(n int) string { return fmt.Sprintf("acme.ops.test.system.widget.%03d", n) }

func (h *harness) put(t *testing.T, id string, value []byte) uint64 {
	t.Helper()
	rev, err := h.states.Put(t.Context(), id, value)
	if err != nil {
		t.Fatalf("put %s: %v", id, err)
	}
	return rev
}

// await waits, bounded by waitBound, until done accepts what observe returns.
func await[T any](t *testing.T, what string, observe func(context.Context) (T, error), done func(T) bool) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), waitBound)
	defer cancel()
	v, err := probe.Await(ctx, observe, done)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	return v
}

// awaitIndexed waits until the follower's floor reaches rev: every delivered revision at or below
// it has been applied by a lane.
func (h *harness) awaitIndexed(t *testing.T, rev uint64) {
	t.Helper()
	await(t, fmt.Sprintf("indexed revision %d", rev),
		func(context.Context) (uint64, error) { return h.f.Indexed(), nil },
		func(got uint64) bool { return got >= rev })
}

// keys is every key stored in bucket, sorted, read through its own reader.
func (h *harness) keys(t *testing.T, bucket string) []string {
	t.Helper()
	reader, err := kvcatalog.OpenCatalogReader(t.Context(), h.client, bucket)
	if err != nil {
		t.Fatalf("open %s: %v", bucket, err)
	}
	keys, err := reader.Keys(t.Context())
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return []string{}
	}
	if err != nil {
		t.Fatalf("keys of %s: %v", bucket, err)
	}
	slices.Sort(keys)
	return keys
}

func (h *harness) value(t *testing.T, bucket, k string) string {
	t.Helper()
	reader, err := kvcatalog.OpenCatalogReader(t.Context(), h.client, bucket)
	if err != nil {
		t.Fatalf("open %s: %v", bucket, err)
	}
	entry, err := reader.Get(t.Context(), k)
	if err != nil {
		t.Fatalf("get %s/%s: %v", bucket, k, err)
	}
	return string(entry.Value())
}

// The expected keys below are built from the layouts the design states (target.source.hex(predicate),
// predicate.entity, sha256(folded name).entity.hex(predicate)), not from the package's own builders.
func hexOf(predicate string) string { return hex.EncodeToString([]byte(predicate)) }

func nameHash(name string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name))))
	return hex.EncodeToString(sum[:])
}

func sorted(keys ...string) []string {
	slices.Sort(keys)
	return keys
}

func assertKeys(t *testing.T, h *harness, bucket string, want []string) {
	t.Helper()
	if got := h.keys(t, bucket); !slices.Equal(got, want) {
		t.Errorf("%s keys:\n got %q\nwant %q", bucket, got, want)
	}
}

const (
	linkTo   = "test.link.to"
	armed    = "test.state.armed"
	disarmed = "test.state.disarmed"
	alias    = "core.identity.alias"
)

// TestIndexReplacementThroughEveryIndex drives one entity's statements from A to B to none and then
// deletes it, and after each step reads every index bucket's complete key set. Each owned index
// holds exactly the current state; another source's edge to the entity survives its delete, since
// the source owns it; and the alias index keeps every alias ever written, the pin's behaviour kept
// until #415 (design D12).
func TestIndexReplacementThroughEveryIndex(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{"workers": 4}`)
	src, other, targetA, targetB := entityID(1), entityID(2), entityID(10), entityID(11)
	title := vocabulary.DCTermsTitle

	h.put(t, other, entityValue(t, other, stmt{linkTo, src}))
	revA := h.put(t, src, entityValue(t, src,
		stmt{linkTo, targetA}, stmt{armed, true}, stmt{title, "Alpha"}, stmt{alias, "alpha-1"}))
	h.awaitIndexed(t, revA)
	assertKeys(t, h, graph.BucketOutgoingIndex, sorted(src, other))
	if got, want := h.value(t, graph.BucketOutgoingIndex, src), `[{"to_entity_id":"`+targetA+`","predicate":"`+linkTo+`"}]`; got != want {
		t.Errorf("outgoing %s after A = %s, want %s", src, got, want)
	}
	assertKeys(t, h, graph.BucketIncomingIndex, sorted(
		targetA+"."+src+"."+hexOf(linkTo), src+"."+other+"."+hexOf(linkTo)))
	assertKeys(t, h, graph.BucketPredicateIndex, sorted(
		linkTo+"."+src, armed+"."+src, title+"."+src, alias+"."+src, linkTo+"."+other))
	assertKeys(t, h, graph.BucketNameIndex, sorted(nameHash("Alpha")+"."+src+"."+hexOf(title)))
	assertKeys(t, h, graph.BucketAliasIndex, sorted("alpha-1"))

	revB := h.put(t, src, entityValue(t, src,
		stmt{linkTo, targetB}, stmt{disarmed, true}, stmt{title, "Beta"}, stmt{alias, "beta-1"}))
	h.awaitIndexed(t, revB)
	if got, want := h.value(t, graph.BucketOutgoingIndex, src), `[{"to_entity_id":"`+targetB+`","predicate":"`+linkTo+`"}]`; got != want {
		t.Errorf("outgoing %s after B = %s, want %s", src, got, want)
	}
	assertKeys(t, h, graph.BucketIncomingIndex, sorted(
		targetB+"."+src+"."+hexOf(linkTo), src+"."+other+"."+hexOf(linkTo)))
	assertKeys(t, h, graph.BucketPredicateIndex, sorted(
		linkTo+"."+src, disarmed+"."+src, title+"."+src, alias+"."+src, linkTo+"."+other))
	assertKeys(t, h, graph.BucketNameIndex, sorted(nameHash("Beta")+"."+src+"."+hexOf(title)))
	assertKeys(t, h, graph.BucketAliasIndex, sorted("alpha-1", "beta-1")) // #415: a dropped alias stays

	revNone := h.put(t, src, entityValue(t, src))
	h.awaitIndexed(t, revNone)
	if got := h.value(t, graph.BucketOutgoingIndex, src); got != `[]` {
		t.Errorf("outgoing %s after none = %s, want []", src, got)
	}
	assertKeys(t, h, graph.BucketIncomingIndex, sorted(src+"."+other+"."+hexOf(linkTo)))
	assertKeys(t, h, graph.BucketPredicateIndex, sorted(linkTo+"."+other))
	assertKeys(t, h, graph.BucketNameIndex, []string{})

	revB2 := h.put(t, src, entityValue(t, src, stmt{linkTo, targetB}, stmt{title, "Beta"}))
	h.awaitIndexed(t, revB2)
	if err := h.states.Delete(t.Context(), src); err != nil {
		t.Fatalf("delete %s: %v", src, err)
	}
	// A later write's revision is above the delete's: the floor reaching it orders the reads below
	// after the delete's apply.
	h.awaitIndexed(t, h.put(t, entityID(50), entityValue(t, entityID(50))))
	assertKeys(t, h, graph.BucketOutgoingIndex, sorted(other, entityID(50)))
	assertKeys(t, h, graph.BucketIncomingIndex, sorted(src+"."+other+"."+hexOf(linkTo)))
	assertKeys(t, h, graph.BucketPredicateIndex, sorted(linkTo+"."+other))
	assertKeys(t, h, graph.BucketNameIndex, []string{})
	if n := h.c.failedTotal(); n != 0 {
		t.Errorf("failed entities = %d, want 0", n)
	}
}

// TestDataFlowCountsEachDeliveryBeforeCoalescing writes one entity three times with values of
// different sizes, waiting before each next write until DataFlow has counted the last one, so each
// write is its own delivery and none is superseded in the stream before the follower reads it.
// DataFlow counts all three and the sum of their lengths, though the coalescer may apply fewer.
func TestDataFlowCountsEachDeliveryBeforeCoalescing(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{"coalesce_ms": 200}`)
	id, target := entityID(1), entityID(9)
	values := [][]byte{
		entityValue(t, id, stmt{armed, true}),
		entityValue(t, id, stmt{armed, true}, stmt{disarmed, false}),
		entityValue(t, id, stmt{linkTo, target}, stmt{armed, true}, stmt{disarmed, false}),
	}
	before := h.c.DataFlow()
	var wantBytes int64
	var last uint64
	for i, value := range values {
		last = h.put(t, id, value)
		wantBytes += int64(len(value))
		await(t, fmt.Sprintf("delivery %d counted", i+1),
			func(context.Context) (component.FlowMetrics, error) { return h.c.DataFlow(), nil },
			func(got component.FlowMetrics) bool { return got.Messages >= before.Messages+int64(i+1) })
	}
	h.awaitIndexed(t, last)
	after := h.c.DataFlow()
	if got := after.Messages - before.Messages; got != 3 {
		t.Errorf("DataFlow messages rose by %d, want 3", got)
	}
	if got := after.Bytes - before.Bytes; got != wantBytes {
		t.Errorf("DataFlow bytes rose by %d, want %d", got, wantBytes)
	}
	if got, want := h.value(t, graph.BucketOutgoingIndex, id), `[{"to_entity_id":"`+target+`","predicate":"`+linkTo+`"}]`; got != want {
		t.Errorf("outgoing %s = %s, want the last value's %s", id, got, want)
	}
}

// TestFailedWriteWithholdsReadinessAndRepairClearsIt deletes INCOMING_INDEX under the running
// index, as an operator wiping it would, and writes an entity with an edge. Its required write
// fails: the entity is counted unresolved, the follower names it and refuses queries, and the
// revision still completes, so the watermark does not pin (design D11, M6). Once the bucket is back,
// one repair pass indexes the entity and readiness returns.
func TestFailedWriteWithholdsReadinessAndRepairClearsIt(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	src, target := entityID(1), entityID(2)
	js, err := h.client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketIncomingIndex); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketIncomingIndex, err)
	}

	rev := h.put(t, src, entityValue(t, src, stmt{linkTo, target}))
	h.awaitIndexed(t, rev)
	// The lane marks the entity failed before it completes the revision, and Indexed reached rev.
	if n := h.c.failedTotal(); n != 1 {
		t.Fatalf("failed entities = %d, want 1", n)
	}
	if stuck := h.f.Stuck(); !slices.Contains(stuck, "1 index writes unresolved") {
		t.Errorf("Stuck() = %q, want it to name the unresolved write", stuck)
	}
	if err := h.f.QueryReady(t.Context()); err == nil {
		t.Errorf("QueryReady = nil while a required write is unresolved")
	}

	if _, err := kvcatalog.EnsureCatalogBucket(t.Context(), h.client, graph.BucketIncomingIndex); err != nil {
		t.Fatalf("recreate %s: %v", graph.BucketIncomingIndex, err)
	}
	h.c.repairFailedEntities(t.Context())
	await(t, "repair resolves the write",
		func(context.Context) (int64, error) { return h.c.failedTotal(), nil },
		func(n int64) bool { return n == 0 })
	assertKeys(t, h, graph.BucketIncomingIndex, sorted(target+"."+src+"."+hexOf(linkTo)))
	if stuck := h.f.Stuck(); len(stuck) != 0 {
		t.Errorf("Stuck() after repair = %q, want none", stuck)
	}
	if err := h.f.QueryReady(t.Context()); err != nil {
		t.Errorf("QueryReady after repair: %v", err)
	}
}

// TestNoncanonicalValueLatchesResetAndHoldsItsRevision writes, after the replay, a value naming
// another entity than its key. The lane latches reset, counts the entity unresolved, and does not
// complete the revision, so the watermark never reports it applied.
func TestNoncanonicalValueLatchesResetAndHoldsItsRevision(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`) // one lane: work applies in submit order
	bad, good := entityID(1), entityID(2)
	badRev := h.put(t, bad, entityValue(t, entityID(3), stmt{armed, true}))
	h.put(t, good, entityValue(t, good, stmt{linkTo, bad}))
	// One lane applies good after bad's work has returned, completion included: good's row is the
	// event that orders the reads below after bad's apply.
	await(t, "the later entity indexed",
		func(ctx context.Context) ([]string, error) { return h.keys(t, graph.BucketOutgoingIndex), nil },
		func(keys []string) bool { return slices.Contains(keys, good) })
	if got := h.f.Indexed(); got >= badRev {
		t.Errorf("Indexed() = %d, want below the noncanonical revision %d", got, badRev)
	}
	if n := h.c.failedTotal(); n != 1 {
		t.Errorf("failed entities = %d, want 1", n)
	}
	want := "graph state reset required: " + string(graph.GraphStateReasonNoncanonicalEntityID)
	if stuck := h.f.Stuck(); !slices.Contains(stuck, want) {
		t.Errorf("Stuck() = %q, want %q", stuck, want)
	}
}

// TestErrorCountCountsOutcomesNotOperations: ErrorCount counts each entity whose required writes
// ultimately failed and each reset, once each (design D4, #425). Failed KV operations, retries
// included, are the reconciliation metrics' to count, not ErrorCount's.
func TestErrorCountCountsOutcomesNotOperations(t *testing.T) {
	t.Run("one entity whose writes fail on every attempt", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		js, err := h.client.JetStream()
		if err != nil {
			t.Fatal(err)
		}
		if err := js.DeleteKeyValue(t.Context(), graph.BucketIncomingIndex); err != nil {
			t.Fatalf("delete %s: %v", graph.BucketIncomingIndex, err)
		}
		src := entityID(1)
		rev := h.put(t, src, entityValue(t, src, stmt{linkTo, entityID(2)}))
		// The lane completes the revision after its last attempt has failed and the entity is marked.
		h.awaitIndexed(t, rev)
		if got := h.c.Health().ErrorCount; got != 1 {
			t.Fatalf("ErrorCount = %d after one entity's writes failed on all %d attempts, want 1",
				got, indexWriteMaxAttempts)
		}
	})
	t.Run("one reset that holds the entity", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`) // one lane: work applies in submit order
		good := entityID(2)
		h.put(t, entityID(1), entityValue(t, entityID(3), stmt{armed, true}))
		h.put(t, good, entityValue(t, good))
		// The noncanonical revision never completes; good's row, applied on the same lane after it,
		// is the event that orders the read below after the reset.
		await(t, "the later entity indexed",
			func(context.Context) ([]string, error) { return h.keys(t, graph.BucketOutgoingIndex), nil },
			func(keys []string) bool { return slices.Contains(keys, good) })
		if got := h.c.Health().ErrorCount; got != 1 {
			t.Fatalf("ErrorCount = %d after one noncanonical value latched reset, want 1", got)
		}
	})
	t.Run("one reset that completes", func(t *testing.T) {
		h := startIndex(t, fixtureClient(t), `{}`)
		rev := h.put(t, entityID(1), []byte("not an entity state"))
		h.awaitIndexed(t, rev)
		if got := h.c.Health().ErrorCount; got != 1 {
			t.Fatalf("ErrorCount = %d after one unreadable value latched reset, want 1", got)
		}
	})
}

// TestUnsafeAliasSkipIsLoggedAndCounted: an alias that is not a valid NATS KV key is skipped, as
// at the pin, and the skip is declared: a warning names the entity and predicate, and
// aliases_skipped_total counts it, while the entity's other rows, its safe alias among them, are
// written (#426; the alias index otherwise keeps the pin's behaviour, #415).
func TestUnsafeAliasSkipIsLoggedAndCounted(t *testing.T) {
	h := startIndex(t, fixtureClient(t), `{}`)
	src, target := entityID(1), entityID(2)
	rev := h.put(t, src, entityValue(t, src, stmt{linkTo, target}, stmt{alias, "two words"}, stmt{alias, "alpha-1"}))
	// The lane logs and counts the skip while it plans, before it completes the revision.
	h.awaitIndexed(t, rev)

	assertKeys(t, h, graph.BucketAliasIndex, sorted("alpha-1"))
	assertKeys(t, h, graph.BucketOutgoingIndex, sorted(src))
	assertKeys(t, h, graph.BucketPredicateIndex, sorted(linkTo+"."+src, alias+"."+src))
	if n := h.c.failedTotal(); n != 0 {
		t.Errorf("failed entities = %d, want 0: a skipped alias is not a failed write", n)
	}
	logs := h.logs.String()
	if !strings.Contains(logs, "KV-unsafe alias skipped") || !strings.Contains(logs, "entity_id="+src) ||
		!strings.Contains(logs, "predicate="+alias) {
		t.Errorf("no warning naming the skipped alias's entity and predicate; logs:\n%s", logs)
	}
	families, err := h.c.metricsRegistry.PrometheusRegistry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var skipped float64
	for _, family := range families {
		if family.GetName() == "semengine_graph_index_aliases_skipped_total" {
			for _, m := range family.GetMetric() {
				skipped += m.GetCounter().GetValue()
			}
		}
	}
	if skipped != 1 {
		t.Errorf("semengine_graph_index_aliases_skipped_total = %v, want 1", skipped)
	}
}
