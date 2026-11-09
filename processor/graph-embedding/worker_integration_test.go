//go:build integration

package graphembedding

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// bodyInstance is the storage instance the gated store is registered under.
const bodyInstance = "objstore"

// gatedStore is a body store whose Open holds the Worker inside hop 2 until the test releases the
// key, as a slow storage backend would; only Open is called. opened receives each key Open is
// called with. An Open whose context ends reports it on cancelled and returns once letGo is
// closed, at once unless the store is slow to cancel; returned closes once any Open has returned.
type gatedStore struct {
	storage.StreamableStore

	opened    chan string
	cancelled chan struct{}
	letGo     chan struct{}
	returned  chan struct{}

	mu           sync.Mutex
	gates        map[string]chan struct{}
	returnedOnce sync.Once
}

func newGatedStore() *gatedStore {
	s := newSlowCancelStore()
	close(s.letGo)
	return s
}

// newSlowCancelStore is a gatedStore whose cancelled Open waits until the test closes letGo, as
// a backend slow to notice cancellation would.
func newSlowCancelStore() *gatedStore {
	return &gatedStore{
		opened:    make(chan string, 16),
		cancelled: make(chan struct{}, 16),
		letGo:     make(chan struct{}),
		returned:  make(chan struct{}),
		gates:     map[string]chan struct{}{},
	}
}

// stores returns a store registry holding s under bodyInstance.
func (s *gatedStore) stores(t *testing.T) *storeregistry.Registry {
	t.Helper()
	stores := storeregistry.New()
	if err := stores.Register(bodyInstance, s); err != nil {
		t.Fatal(err)
	}
	return stores
}

func (s *gatedStore) gate(key string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gates[key] == nil {
		s.gates[key] = make(chan struct{})
	}
	return s.gates[key]
}

// release lets every Open of key, waiting or to come, return its body.
func (s *gatedStore) release(key string) { close(s.gate(key)) }

func (s *gatedStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	defer s.returnedOnce.Do(func() { close(s.returned) })
	select {
	case s.opened <- key:
	default: // nobody is counting Opens beyond the buffer
	}
	select {
	case <-s.gate(key):
		return io.NopCloser(strings.NewReader("body of " + key)), nil
	case <-ctx.Done():
		select {
		case s.cancelled <- struct{}{}:
		default:
		}
		<-s.letGo
		return nil, ctx.Err()
	}
}

// bodyRef is the ENTITY_STATES storage reference of a body under key in the gated store.
func bodyRef(key string) map[string]string {
	return map[string]string{"storage_instance": bodyInstance, "key": key}
}

// awaitGenerated waits until id's record is generated at revision rev, written by the Worker.
func (h *harness) awaitGenerated(t *testing.T, id string, rev uint64) *embedding.Record {
	t.Helper()
	return await(t, fmt.Sprintf("%s generated at revision %d", id, rev),
		func(ctx context.Context) (*embedding.Record, error) { return h.record(ctx, id) },
		func(r *embedding.Record) bool {
			return r != nil && r.Status == embedding.StatusGenerated && r.SourceRevision == rev
		})
}

// awaitIndexed waits until the follower's floor reaches rev and returns it.
func (h *harness) awaitIndexed(t *testing.T, what string, rev uint64) uint64 {
	t.Helper()
	return await(t, what, func(context.Context) (uint64, error) { return h.f.Indexed(), nil },
		func(got uint64) bool { return got >= rev })
}

// stacksIn returns the stacks of every goroutine with frame in its stack.
func stacksIn(frame string) string {
	buf := make([]byte, 1<<20)
	var found []string
	for _, stack := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
		if strings.Contains(stack, frame) {
			found = append(found, stack)
		}
	}
	return strings.Join(found, "\n\n")
}

// followerFrame is in every stack of the follower's goroutines.
const followerFrame = "follower.(*Follower)"

// stackBound is the real-clock failure bound on a goroutine leaving the stack dump after it told
// its joiner it was done (#490).
const stackBound = time.Minute

// TestStopJoinsWorker: Start runs the Worker (hop 2) under the run context, and Stop returns
// only after its Run has (design D2; ADR-113). The Worker is held inside a body read that waits,
// after Stop ends the run context, for the test to let it go. Stop has not returned once the
// follower's goroutines, which Stop joins first, are gone; it returns nil once the read is let
// go, and by then the read has returned. A Run that ends because Stop ended the run context is
// not a lost watch: no lost-watch line is logged before Stop returns.
func TestStopJoinsWorker(t *testing.T) {
	store := newSlowCancelStore()
	h := startEmbedding(t, fixtureClient(t), `{}`, store.stores(t))
	id := entityID(1)
	h.put(t, id, entityValue(t, id, bodyRef("held"), "doc.meta.title", "held at hop two"))
	if key := <-store.opened; key != "held" {
		t.Fatalf("the Worker opened %q; want held", key)
	}
	if stacksIn(followerFrame) == "" {
		t.Fatalf("no goroutine has %s while started; the frame no longer matches the follower", followerFrame)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- h.c.Stop(t.Context()) }()
	<-store.cancelled // Stop ended the run context; the Worker is still inside Open
	if err := natsfixture.AwaitNoStacks(t.Context(), stackBound, func() string { return stacksIn(followerFrame) }); err != nil {
		t.Fatalf("the follower's goroutines are still running during Stop: %v", err)
	}
	select {
	case err := <-stopped:
		t.Fatalf("Stop returned %v while the Worker was still inside its body read", err)
	default:
	}
	close(store.letGo)
	if err := <-stopped; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-store.returned:
	default:
		t.Error("Stop returned before the Worker's body read had")
	}
	if strings.Contains(h.logs.String(), "embedding worker watch lost") {
		t.Errorf("a Stop logged a lost Worker watch:\n%s", h.logs.String())
	}
}

// TestHealthNamesLostWorkerWatch: when EMBEDDING_INDEX is deleted under a running component,
// nats.go closes the Worker's watch, its Run returns while the run context is live, and the loss
// is logged as an error and named by Health, not healthy (design D2). Nothing rebinds the watch:
// with the bucket recreated and a new pending record written to it, Health still names it.
func TestHealthNamesLostWorkerWatch(t *testing.T) {
	client := fixtureClient(t)
	h := startEmbedding(t, client, `{}`, nil)
	a, b := entityID(1), entityID(2)
	h.awaitGenerated(t, a, h.put(t, a, entityValue(t, a, nil, "doc.meta.title", "before the loss")))
	if health := h.c.Health(); !health.Healthy {
		t.Fatalf("Health with the Worker running = %+v, want healthy", health)
	}

	js, err := client.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketEmbeddingIndex); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketEmbeddingIndex, err)
	}
	// How soon nats.go closes a deleted bucket's watch is its timing (graph/follower's limits);
	// go test -timeout is the deadline. The component latches the loss before it logs the line,
	// and the log buffer's lock orders this read after that write, so Health reads the latch.
	await(t, "the lost Worker watch logged",
		func(context.Context) (string, error) { return h.logs.String(), nil },
		func(logs string) bool { return strings.Contains(logs, "embedding worker watch lost") })
	if health := h.c.Health(); health.Healthy || health.Status != "degraded" ||
		!strings.Contains(health.LastError, workerWatchLost) {
		t.Fatalf("Health after the loss = %+v, want degraded, not healthy, naming %q", health, workerWatchLost)
	}

	if _, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingIndex); err != nil {
		t.Fatalf("recreate %s: %v", graph.BucketEmbeddingIndex, err)
	}
	h.awaitPending(t, b, h.put(t, b, entityValue(t, b, nil, "doc.meta.title", "after the loss")))
	if health := h.c.Health(); health.Healthy || !strings.Contains(health.LastError, workerWatchLost) {
		t.Errorf("Health after the bucket came back = %+v, want the lost watch still named", health)
	}
}

// TestPendingRecordBeforeStartIsEmbedded: a pending record left in EMBEDDING_INDEX by an earlier
// process, before Start, is embedded once the Worker runs, and its revision completes.
func TestPendingRecordBeforeStartIsEmbedded(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	a := entityID(1)
	rev, err := states.Put(t.Context(), a, entityValue(t, a, nil, "doc.meta.title", "written before Start"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingIndex)
	if err != nil {
		t.Fatal(err)
	}
	dedup, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingDedup)
	if err != nil {
		t.Fatal(err)
	}
	earlier := embedding.NewStorage(client.NewKVStore(index), client.NewKVStore(dedup))
	if _, err := earlier.SavePendingGuarded(t.Context(), &embedding.Record{
		EntityID: a, SourceText: "written before Start", SourceRevision: rev,
	}); err != nil {
		t.Fatalf("SavePendingGuarded: %v", err)
	}

	h := startEmbedding(t, client, `{}`, nil)
	h.awaitGenerated(t, a, rev)
	h.awaitIndexed(t, "the pre-Start revision indexed", rev)
}

// TestWorkerExclusionCountedAfterRestart: review finding R2 at stop 2. A pending record an
// earlier process left on the offloaded lane names a store this process has not registered, as
// after a restart without that storage component. The Worker embeds the identity text alone and
// counts the excluded body on the component's content_unresolved_total, as hop 1 counts its own.
// A coalescing window longer than the test keeps hop 1 from reading the entity, so the only
// exclusion is the Worker's.
func TestWorkerExclusionCountedAfterRestart(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	a := entityID(1)
	rev, err := states.Put(t.Context(), a, entityValue(t, a, bodyRef("old-body"), "doc.meta.title", "retained title"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingIndex)
	if err != nil {
		t.Fatal(err)
	}
	dedup, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEmbeddingDedup)
	if err != nil {
		t.Fatal(err)
	}
	earlier := embedding.NewStorage(client.NewKVStore(index), client.NewKVStore(dedup))
	if _, err := earlier.SavePendingGuarded(t.Context(), &embedding.Record{
		EntityID: a, SourceRevision: rev, IdentityText: "retained title",
		StorageRef: &embedding.StorageRef{StorageInstance: bodyInstance, Key: "old-body"},
	}); err != nil {
		t.Fatal(err)
	}

	h := startEmbedding(t, client, `{"coalesce_ms":3600000}`, nil)
	// The Worker counts the exclusion before it writes the generated record awaited here.
	h.awaitGenerated(t, a, rev)
	if logs := h.logs.String(); !strings.Contains(logs, "no live store is registered") {
		t.Fatalf("the Worker logged no exclusion; logs:\n%s", logs)
	}
	if n := testutil.ToFloat64(h.c.metrics.contentUnresolved); n != 1 {
		t.Errorf("content_unresolved_total = %v after the Worker excluded the body, want 1", n)
	}
}
