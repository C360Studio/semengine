//go:build integration

package graphembedding

import (
	"context"
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
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/storage/storeregistry"
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

// logBuffer is the component's log output, safe to write from its goroutines while a test reads it.
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

// harness is a running graph-embedding built from JSON through CreateGraphEmbedding, with the
// ENTITY_STATES write handle the test uses in graph-ingest's place.
type harness struct {
	c       *Component
	f       *follower.Follower
	states  jetstream.KeyValue
	client  *natsclient.Client
	logs    *logBuffer
	metrics *metric.MetricsRegistry
}

func ensureEntityStates(t *testing.T, client *natsclient.Client) jetstream.KeyValue {
	t.Helper()
	states, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketEntityStates, err)
	}
	return states
}

// newEmbedding builds an initialized, unstarted graph-embedding from raw, on a metrics registry of
// its own, logging to the returned buffer. stores is the store registry, nil for none.
func newEmbedding(t *testing.T, client *natsclient.Client, raw string, stores *storeregistry.Registry) *harness {
	t.Helper()
	logs := &logBuffer{}
	metrics := metric.NewMetricsRegistry()
	built, err := CreateGraphEmbedding(json.RawMessage(raw), component.Dependencies{
		NATSClient: client, MetricsRegistry: metrics, StoreRegistry: stores,
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatalf("CreateGraphEmbedding: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return &harness{c: c, client: client, logs: logs, metrics: metrics}
}

// startEmbedding ensures ENTITY_STATES, builds graph-embedding from raw and starts it; the test's
// cleanup stops it.
func startEmbedding(t *testing.T, client *natsclient.Client, raw string, stores *storeregistry.Registry) *harness {
	t.Helper()
	states := ensureEntityStates(t, client)
	h := newEmbedding(t, client, raw, stores)
	if err := h.c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, h.c) })
	h.states, h.f = states, h.c.follower.Load()
	return h
}

// stopBound is out of reach: go test -timeout is the deadline that fails a Stop that never returns.
const stopBound = time.Hour

func stopComponent(t *testing.T, c *Component) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stopBound)
	defer cancel()
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func entityID(n int) string { return fmt.Sprintf("acme.ops.test.system.doc.%03d", n) }

// entityValue is the ENTITY_STATES value for id with one string statement per predicate and value
// pair, and ref as its storage reference when not nil.
func entityValue(t *testing.T, id string, ref map[string]string, pairs ...string) []byte {
	t.Helper()
	type triple struct {
		Subject   string    `json:"subject"`
		Predicate string    `json:"predicate"`
		Object    any       `json:"object"`
		Source    string    `json:"source"`
		Timestamp time.Time `json:"timestamp"`
	}
	triples := []triple{}
	for i := 0; i+1 < len(pairs); i += 2 {
		triples = append(triples, triple{id, pairs[i], pairs[i+1], "test", time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)})
	}
	value := map[string]any{"id": id, "triples": triples}
	if ref != nil {
		value["storage_ref"] = ref
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

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

// record reads id's EMBEDDING_INDEX record through a reader of its own; nil when there is none.
func (h *harness) record(ctx context.Context, id string) (*embedding.Record, error) {
	reader, err := kvcatalog.OpenCatalogReader(ctx, h.client, graph.BucketEmbeddingIndex)
	if err != nil {
		return nil, err
	}
	entry, err := reader.Get(ctx, id)
	if natsclient.IsKVNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record embedding.Record
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// awaitPending waits until id's record is pending at revision rev.
func (h *harness) awaitPending(t *testing.T, id string, rev uint64) *embedding.Record {
	t.Helper()
	return await(t, fmt.Sprintf("%s pending at revision %d", id, rev),
		func(ctx context.Context) (*embedding.Record, error) { return h.record(ctx, id) },
		func(r *embedding.Record) bool {
			return r != nil && r.Status == embedding.StatusPending && r.SourceRevision == rev
		})
}

// TestReadinessWaitsForHopTwo: a revision whose pending record is written is not caught up until
// the Worker reports its terminal outcome (graph-embedding spec, "A pending record is not caught
// up"; design D1). Hop 1 runs serially on the follower's goroutine, so B's record being stored
// means hop 1 has returned for A: whatever it would complete for A is completed by then.
func TestReadinessWaitsForHopTwo(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	a, b := entityID(1), entityID(2)
	revA := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first document"))
	revB := h.put(t, b, entityValue(t, b, nil, "doc.meta.title", "second document"))
	h.awaitPending(t, b, revB)
	if record := h.awaitPending(t, a, revA); record.SourceText != "first document" {
		t.Errorf("A's pending text = %q, want the title", record.SourceText)
	}
	if got := h.f.Indexed(); got >= revA {
		t.Fatalf("indexed %d with A's record pending at %d; want below it", got, revA)
	}

	// The Worker's terminal outcome for A completes it; B still holds the floor.
	h.c.onTerminal(t.Context(), a, revA, embedding.OutcomeGenerated, "")
	if got := h.f.Indexed(); got < revA || got >= revB {
		t.Fatalf("indexed %d after A's terminal outcome; want at least %d and below %d", got, revA, revB)
	}
	h.c.onTerminal(t.Context(), b, revB, embedding.OutcomeFailed, "timeout")
	if got := h.f.Indexed(); got < revB {
		t.Errorf("indexed %d after B's terminal outcome; want at least %d", got, revB)
	}
}

// TestHopOneEndsTextlessAndDeletedRevisions: a revision with no text and a tombstone end at hop 1,
// each removing the entity's record, so the floor reaches them with no terminal outcome from the
// Worker (design D1).
func TestHopOneEndsTextlessAndDeletedRevisions(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	a := entityID(1)
	rev := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first document"))
	h.awaitPending(t, a, rev)
	h.c.onTerminal(t.Context(), a, rev, embedding.OutcomeGenerated, "")

	textless := h.put(t, a, entityValue(t, a, nil, "doc.meta.count", "7"))
	await(t, "the text-less revision indexed",
		func(context.Context) (uint64, error) { return h.f.Indexed(), nil },
		func(got uint64) bool { return got >= textless })
	if record, err := h.record(t.Context(), a); err != nil || record != nil {
		t.Fatalf("record after a text-less revision = %+v, %v; want none", record, err)
	}

	again := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first document again"))
	h.awaitPending(t, a, again)
	if err := h.states.Delete(t.Context(), a); err != nil {
		t.Fatalf("delete %s: %v", a, err)
	}
	await(t, "the tombstone removes the record",
		func(ctx context.Context) (*embedding.Record, error) { return h.record(ctx, a) },
		func(r *embedding.Record) bool { return r == nil })
	if got := h.f.Indexed(); got <= again {
		t.Errorf("indexed %d after the tombstone; want above the pending revision %d", got, again)
	}
}

// TestCoalescedRevisionsComplete: with coalesce_ms above zero, revisions of one entity collapse
// into one pending record at the revision read, and completing it drains every lower delivered
// revision of the entity (design D1; pin
// derived_state_convergence_integration_test.go:TestIntegration_CoalescedLane_TombstoneConverges).
// The coalescer applies batches one after another, so B's record, written after A's last
// revision was read, orders the floor read after A's batch.
func TestCoalescedRevisionsComplete(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{"coalesce_ms": 20}`, nil)
	a, b := entityID(1), entityID(2)
	first := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "version one"))
	second := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "version two"))
	if record := h.awaitPending(t, a, second); record.SourceText != "version two" {
		t.Errorf("A's pending text = %q, want the second version", record.SourceText)
	}
	revB := h.put(t, b, entityValue(t, b, nil, "doc.meta.title", "other"))
	h.awaitPending(t, b, revB)
	if got := h.f.Indexed(); got >= first {
		t.Fatalf("indexed %d before any terminal outcome; want below %d", got, first)
	}

	h.c.onTerminal(t.Context(), a, second, embedding.OutcomeGenerated, "")
	if got := h.f.Indexed(); got < second || got >= revB {
		t.Fatalf("indexed %d after A's outcome at %d; want both of A's revisions drained, B pending", got, second)
	}

	t.Run("a tombstone drops the pending update and converges", func(t *testing.T) {
		h.c.onTerminal(t.Context(), b, revB, embedding.OutcomeGenerated, "")
		update := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "version three"))
		if err := h.states.Delete(t.Context(), a); err != nil {
			t.Fatalf("delete %s: %v", a, err)
		}
		await(t, "the tombstone indexed",
			func(context.Context) (uint64, error) { return h.f.Indexed(), nil },
			func(got uint64) bool { return got > update })
		if record, err := h.record(t.Context(), a); err != nil || record != nil {
			t.Errorf("record after the tombstone = %+v, %v; want none", record, err)
		}
	})
}

// bodyStore is a storage.StreamableStore registered only so its instance resolves; hop 1 never
// reads it.
type bodyStore struct{ storage.StreamableStore }

// TestStorageRefLaneEmbedsIdentityThenBody: an entity whose body is offloaded to a registered
// store is queued on the offloaded lane, its inline identity text in the record for hop 2 to embed
// ahead of the fetched body and no source text (design D5; ADR-063; pin
// offloaded_identity_integration_test.go:TestIntegration_OffloadedEntityEmbedsIdentityAlongsideBody).
// Hop 2's join of the two is graph/embedding's (offloaded_identity_test.go). An entity whose store
// is not registered is embedded from its inline text, and the excluded body is declared: counted
// for each entity and warned about once.
func TestStorageRefLaneEmbedsIdentityThenBody(t *testing.T) {
	stores := storeregistry.New()
	if err := stores.Register("objstore", bodyStore{}); err != nil {
		t.Fatal(err)
	}
	h := startEmbedding(t, fixtureClient(t), `{}`, stores)

	offloaded, bodyOnly := entityID(1), entityID(2)
	rev := h.put(t, offloaded, entityValue(t, offloaded, map[string]string{"storage_instance": "objstore", "key": "body/1"},
		"doc.meta.title", "Quarterly report", "doc.meta.summary", "revenue up"))
	record := h.awaitPending(t, offloaded, rev)
	if record.IdentityText != "Quarterly report revenue up" || record.SourceText != "" ||
		record.StorageRef == nil || *record.StorageRef != (embedding.StorageRef{StorageInstance: "objstore", Key: "body/1"}) {
		t.Errorf("offloaded record = identity %q, source %q, ref %+v; want the identity text, no source, the reference",
			record.IdentityText, record.SourceText, record.StorageRef)
	}
	rev = h.put(t, bodyOnly, entityValue(t, bodyOnly, map[string]string{"storage_instance": "objstore", "key": "body/2"},
		"doc.meta.count", "3"))
	if record := h.awaitPending(t, bodyOnly, rev); record.IdentityText != "" || record.StorageRef == nil {
		t.Errorf("body-only record = identity %q, ref %+v; want no identity text and the reference",
			record.IdentityText, record.StorageRef)
	}
	if n := testutil.ToFloat64(h.c.metrics.contentUnresolved); n != 0 {
		t.Errorf("content_unresolved_total = %v with every store registered, want 0", n)
	}

	t.Run("an unregistered store excludes only the body", func(t *testing.T) {
		var last uint64
		for i := 3; i <= 4; i++ {
			id := entityID(i)
			last = h.put(t, id, entityValue(t, id, map[string]string{"storage_instance": "elsewhere", "key": "body"},
				"doc.meta.title", fmt.Sprintf("title %d", i)))
		}
		record := h.awaitPending(t, entityID(4), last)
		if record.SourceText != "title 4" || record.StorageRef != nil || record.IdentityText != "" {
			t.Errorf("record with an unregistered store = source %q, ref %+v, identity %q; want the inline lane",
				record.SourceText, record.StorageRef, record.IdentityText)
		}
		if n := testutil.ToFloat64(h.c.metrics.contentUnresolved); n != 2 {
			t.Errorf("content_unresolved_total = %v, want 2", n)
		}
		if n := strings.Count(h.logs.String(), "offloaded body excluded from embeddings"); n != 1 {
			t.Errorf("exclusion warned %d times, want once", n)
		}
	})
}

// suiteOwner is graph-embedding as the lifecycle suite's owner. Observe lists what the component
// holds; the follower's watch is private to graph/follower.
type suiteOwner struct{ c *Component }

func (o suiteOwner) Start(ctx context.Context) error { return o.c.Start(ctx) }
func (o suiteOwner) Stop(ctx context.Context) error  { return o.c.Stop(ctx) }

// Observe reads the component's handles without a lock: the suite observes after a Start or Stop
// has returned to it.
func (o suiteOwner) Observe() lifecycletest.Observation {
	var held []string
	if o.c.runCancel != nil {
		held = append(held, "run cancel")
	}
	if o.c.coalescer != nil {
		held = append(held, "coalescer")
	}
	if o.c.feeds != nil {
		held = append(held, "feeds")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// TestGraphEmbeddingLifecycleSuite runs the lifecycle suite on graph-embedding, with coalesce_ms
// above zero so the coalescer is among the handles. The must-fail owner's environment has no
// ENTITY_STATES, so its Start fails in the follower's Open before it acquires anything. The
// component is one-shot, so no restart is promised.
func TestGraphEmbeddingLifecycleSuite(t *testing.T) {
	client := fixtureClient(t)
	ensureEntityStates(t, client)
	factory := func() lifecycletest.Owner {
		return suiteOwner{newEmbedding(t, client, `{"coalesce_ms": 50}`, nil).c}
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
		return suiteOwner{newEmbedding(t, client, `{"startup_attempts": 1, "startup_interval_ms": 1}`, nil).c}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		ensureEntityStates(t, client) // the must-fail owner deleted it
		owner := factory()
		if err := owner.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		want := []string{"run cancel", "coalescer", "feeds"}
		if held := owner.Observe().Unresolved; !slices.Equal(held, want) {
			t.Errorf("held while started = %v, want %v", held, want)
		}
		stopComponent(t, owner.(suiteOwner).c)
		if held := owner.Observe().Unresolved; len(held) != 0 {
			t.Errorf("held after a nil Stop = %v, want nothing", held)
		}
	})
}

// TestStartContextEndStopsNothing: ending the context given to Start after Start returned stops
// nothing; hop 1 still writes what is stored (ADR-113).
func TestStartContextEndStopsNothing(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	h := newEmbedding(t, client, `{}`, nil)
	startCtx, cancelStart := context.WithCancel(t.Context())
	if err := h.c.Start(startCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopComponent(t, h.c) })
	cancelStart()
	h.states = states
	id := entityID(1)
	h.awaitPending(t, id, h.put(t, id, entityValue(t, id, nil, "doc.meta.title", "after the start context ended")))
	if health := h.c.Health(); !health.Healthy || health.Status != "running" {
		t.Errorf("Health = %+v, want running and healthy", health)
	}
}
