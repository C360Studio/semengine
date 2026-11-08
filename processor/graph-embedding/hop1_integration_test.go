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

// await waits until done accepts what observe returns; go test -timeout is the one deadline.
func await[T any](t *testing.T, what string, observe func(context.Context) (T, error), done func(T) bool) T {
	t.Helper()
	v, err := probe.Await(t.Context(), observe, done)
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
// up"; design D1). A's body is offloaded to a store that holds the Worker's read until the test
// releases it, so A stays pending at hop 2 while B, inline, is embedded. Hop 1 runs serially on
// the follower's goroutine, so B's record being generated means hop 1 has returned for A:
// whatever it would complete for A is completed by then.
func TestReadinessWaitsForHopTwo(t *testing.T) {
	store := newGatedStore()
	h := startEmbedding(t, fixtureClient(t), `{}`, store.stores(t))
	a, b := entityID(1), entityID(2)
	revA := h.put(t, a, entityValue(t, a, bodyRef("a"), "doc.meta.title", "first document"))
	revB := h.put(t, b, entityValue(t, b, nil, "doc.meta.title", "second document"))
	if record := h.awaitPending(t, a, revA); record.IdentityText != "first document" {
		t.Errorf("A's pending identity text = %q, want the title", record.IdentityText)
	}
	h.awaitGenerated(t, b, revB)
	if got := h.f.Indexed(); got >= revA {
		t.Fatalf("indexed %d with A's record pending at %d; want below it", got, revA)
	}

	// The Worker's terminal outcome for A completes it, and B's already has.
	store.release("a")
	h.awaitIndexed(t, "A's and B's revisions indexed", revB)
	h.awaitGenerated(t, a, revA)
}

// TestConfiguredTextSuffixesSelectText: hop 1 embeds the values of the predicates text_suffixes
// names, and only those: a predicate only the defaults name (".title") is left out (design D4;
// Codex review C2 on PR #550). The Worker has one lane, held on an earlier entity's body read, so
// the record stays pending for the test to read.
func TestConfiguredTextSuffixesSelectText(t *testing.T) {
	store := newGatedStore()
	h := startEmbedding(t, fixtureClient(t), `{"text_suffixes": [".signature"], "workers": 1}`, store.stores(t))
	holdWorker(t, h, store)
	a := entityID(1)
	rev := h.put(t, a, entityValue(t, a, nil,
		"code.function.signature", "func main()", "doc.meta.title", "default-only title"))
	if record := h.awaitPending(t, a, rev); record.SourceText != "func main()" {
		t.Errorf("pending text = %q, want only the .signature value", record.SourceText)
	}
}

// holdWorker holds a one-lane Worker inside hop 2: it writes an entity whose offloaded body the
// gated store never releases and waits for the Worker to open it, so every pending record written
// after stays pending until Stop.
func holdWorker(t *testing.T, h *harness, store *gatedStore) {
	t.Helper()
	const id = "acme.ops.test.system.doc.hold"
	h.put(t, id, entityValue(t, id, bodyRef("hold"), "doc.meta.title", "holds the lane"))
	if key := <-store.opened; key != "hold" {
		t.Fatalf("the Worker opened %q; want hold", key)
	}
}

// TestHopOneEndsTextlessAndDeletedRevisions: a revision with no text and a tombstone end at hop 1,
// each removing the entity's record, so the floor reaches them with no terminal outcome from the
// Worker (design D1).
func TestHopOneEndsTextlessAndDeletedRevisions(t *testing.T) {
	h := startEmbedding(t, fixtureClient(t), `{}`, nil)
	a := entityID(1)
	h.awaitIndexed(t, "the first revision indexed", h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first document")))

	textless := h.put(t, a, entityValue(t, a, nil, "doc.meta.count", "7"))
	await(t, "the text-less revision indexed",
		func(context.Context) (uint64, error) { return h.f.Indexed(), nil },
		func(got uint64) bool { return got >= textless })
	if record, err := h.record(t.Context(), a); err != nil || record != nil {
		t.Fatalf("record after a text-less revision = %+v, %v; want none", record, err)
	}

	again := h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first document again"))
	await(t, "a record at the revision before the tombstone",
		func(ctx context.Context) (*embedding.Record, error) { return h.record(ctx, a) },
		func(r *embedding.Record) bool { return r != nil && r.SourceRevision == again })
	if err := h.states.Delete(t.Context(), a); err != nil {
		t.Fatalf("delete %s: %v", a, err)
	}
	// applyTombstone deletes the record before it completes the tombstone's revision, so the
	// floor passing the pending revision orders the record read after the delete.
	await(t, "the tombstone indexed",
		func(context.Context) (uint64, error) { return h.f.Indexed(), nil },
		func(got uint64) bool { return got > again })
	if record, err := h.record(t.Context(), a); err != nil || record != nil {
		t.Errorf("record after the tombstone = %+v, %v; want none", record, err)
	}
}

// TestRestartCompletesGeneratedRevision: after a restart the follower replays ENTITY_STATES, and a
// revision whose generated record already stands ends at hop 1, since SavePendingGuarded keeps
// the generated record (pin #722 B2), so the floor reaches it with no terminal outcome from the
// Worker; otherwise the replayed revision would hold the floor, and bootstrap, below it for good.
// Hop 1 runs serially on the follower's goroutine and the watch delivers in revision order, so
// B's record, written after the restart, being stored means hop 1 has returned for A's replay.
// B's body is offloaded to a store that never releases it, so B holds the floor at hop 2.
func TestRestartCompletesGeneratedRevision(t *testing.T) {
	client := fixtureClient(t)
	store := newGatedStore()
	first := startEmbedding(t, client, `{}`, store.stores(t))
	a, b := entityID(1), entityID(2)
	rev := first.put(t, a, entityValue(t, a, nil, "doc.meta.title", "first document"))
	first.awaitGenerated(t, a, rev)
	stopComponent(t, first.c)

	second := startEmbedding(t, client, `{}`, store.stores(t))
	revB := second.put(t, b, entityValue(t, b, bodyRef("b"), "doc.meta.title", "second document"))
	second.awaitPending(t, b, revB)
	if got := second.f.Indexed(); got < rev || got >= revB {
		t.Errorf("indexed %d after the restart; want A's replayed generated revision %d complete, B's %d pending",
			got, rev, revB)
	}
	if record, err := second.record(t.Context(), a); err != nil || record == nil ||
		record.Status != embedding.StatusGenerated || record.SourceRevision != rev {
		t.Errorf("A's record after the replay = %+v, %v; want generated at revision %d", record, err, rev)
	}
}

// TestCoalescedRevisionsComplete: with coalesce_ms above zero, revisions of one entity collapse
// into one pending record at the revision read, and completing it drains every lower delivered
// revision of the entity (design D1; pin
// derived_state_convergence_integration_test.go:TestIntegration_CoalescedLane_TombstoneConverges).
// The coalescer applies batches one after another, so B's record, written after A's last
// revision was read, orders the floor read after A's batch. A watch that opens after both of A's
// writes delivers only the second, so a first entity's revision is awaited before them: the
// watch is then live and delivers each of A's revisions. A's and B's bodies are offloaded to a
// store that holds the Worker's read of each until the test releases it.
func TestCoalescedRevisionsComplete(t *testing.T) {
	store := newGatedStore()
	h := startEmbedding(t, fixtureClient(t), `{"coalesce_ms": 20}`, store.stores(t))
	a, b, live := entityID(1), entityID(2), entityID(3)
	h.awaitIndexed(t, "the first entity indexed", h.put(t, live, entityValue(t, live, nil, "doc.meta.title", "the watch is live")))
	first := h.put(t, a, entityValue(t, a, bodyRef("a"), "doc.meta.title", "version one"))
	second := h.put(t, a, entityValue(t, a, bodyRef("a"), "doc.meta.title", "version two"))
	if record := h.awaitPending(t, a, second); record.IdentityText != "version two" {
		t.Errorf("A's pending identity text = %q, want the second version", record.IdentityText)
	}
	revB := h.put(t, b, entityValue(t, b, bodyRef("b"), "doc.meta.title", "other"))
	h.awaitPending(t, b, revB)
	if got := h.f.Indexed(); got >= first {
		t.Fatalf("indexed %d before any terminal outcome; want below %d", got, first)
	}

	store.release("a")
	if got := h.awaitIndexed(t, "A's revisions indexed", second); got >= revB {
		t.Fatalf("indexed %d after A's outcome at %d; want both of A's revisions drained, B pending", got, second)
	}

	t.Run("a tombstone drops the pending update and converges", func(t *testing.T) {
		store.release("b")
		h.awaitIndexed(t, "B indexed", revB)
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

// TestStorageRefLaneEmbedsIdentityThenBody: an entity whose body is offloaded to a registered
// store is queued on the offloaded lane, its inline identity text in the record for hop 2 to embed
// ahead of the fetched body and no source text (design D5; ADR-063; pin
// offloaded_identity_integration_test.go:TestIntegration_OffloadedEntityEmbedsIdentityAlongsideBody).
// Hop 2's join of the two is graph/embedding's (offloaded_identity_test.go). An entity whose store
// is not registered is embedded from its inline text, and the excluded body is declared: counted
// for each entity and warned about once. The Worker has one lane, held on an earlier entity's
// body read, so every record stays pending for the test to read.
func TestStorageRefLaneEmbedsIdentityThenBody(t *testing.T) {
	store := newGatedStore()
	h := startEmbedding(t, fixtureClient(t), `{"workers": 1}`, store.stores(t))
	holdWorker(t, h, store)

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
	if o.c.repairs != nil {
		held = append(held, "repair lane")
	}
	if o.c.worked != nil {
		held = append(held, "worker")
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
		want := []string{"run cancel", "coalescer", "feeds", "repair lane", "worker"}
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
// nothing; hop 1 still writes what is stored and the Worker embeds it (ADR-113).
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
	h.awaitGenerated(t, id, h.put(t, id, entityValue(t, id, nil, "doc.meta.title", "after the start context ended")))
	if health := h.c.Health(); !health.Healthy || health.Status != "running" {
		t.Errorf("Health = %+v, want running and healthy", health)
	}
}

// TestRecreatedRevisionWaitsForWorker (#555; Codex's final review of PR #550, F1): on the
// coalesced lane a batch that reads an entity absent completes only the revisions delivered before
// its read, so a re-creation the follower observes during that read stays outstanding until its
// own terminal outcome. The test runs a batch's reconcileEntity with its ENTITY_STATES read held
// once the read has found the entity absent, and re-creates the entity meanwhile: the await on
// DataFlow's message count, which deliver counts after the watermark observes the revision,
// orders the release, and so the completion, after that observation. The Worker is held on the
// re-created entity's body, so its record stays pending and the floor must stay below it.
func TestRecreatedRevisionWaitsForWorker(t *testing.T) {
	store := newGatedStore()
	// The coalescer never fires within the test, so the reconciles below are the only readers.
	h := startEmbedding(t, fixtureClient(t), `{"coalesce_ms":3600000}`, store.stores(t))
	a := entityID(99)
	h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "before delete"))
	await(t, "old delivery", func(context.Context) (int64, error) {
		return h.c.DataFlow().Messages, nil
	}, func(n int64) bool { return n >= 1 })
	if err := h.states.Delete(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	h.awaitIndexed(t, "tombstone", lastRevision(t, h.client, graph.BucketEntityStates))

	read := &heldReader{CatalogReader: h.c.entities, read: make(chan error, 1), release: make(chan struct{})}
	h.c.entities = read // no reader runs yet: the coalescer has not fired and nothing is failed
	reconciled := make(chan struct{})
	go func() {
		defer close(reconciled)
		h.c.reconcileEntity(t.Context(), a)
	}()
	if err := <-read.read; !natsclient.IsKVNotFoundError(err) {
		close(read.release)
		<-reconciled
		t.Fatalf("held read = %v, want the entity absent", err)
	}
	before := h.c.DataFlow().Messages
	rev := h.put(t, a, entityValue(t, a, bodyRef("held"), "doc.meta.title", "recreated"))
	await(t, "recreation observed", func(context.Context) (int64, error) {
		return h.c.DataFlow().Messages, nil
	}, func(n int64) bool { return n > before })
	close(read.release)
	<-reconciled // the absent read's completion has run

	h.c.reconcileEntity(t.Context(), a) // the re-creation's own batch
	h.awaitPending(t, a, rev)
	if got := h.f.Indexed(); got >= rev {
		t.Fatalf("recreated revision %d is still pending, but indexed revision is %d", rev, got)
	}
}

// heldReader holds its first Get after the read, reporting the read's error on read until
// release closes; later Gets pass through.
type heldReader struct {
	kvcatalog.CatalogReader
	once    sync.Once
	read    chan error
	release chan struct{}
}

func (r *heldReader) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	entry, err := r.CatalogReader.Get(ctx, key)
	r.once.Do(func() {
		r.read <- err
		<-r.release
	})
	return entry, err
}
