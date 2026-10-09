package graphingest

// query_contract_guard_test.go — per-entity poison scoping at the query
// surface (poison-response-scoping). The surface-global sticky latch is gone:
// the boot snapshot sweep validates the resident snapshot into the per-entity
// poison inventory and then STOPS its watcher (snapshot-then-stop, design D1);
// refusal is per-entity and derives solely from each read's validating decode
// of the stored bytes. These tests pin the sweep lifecycle, the per-entity
// refusal scope, and the inversion of the old whole-surface latch behavior.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

const (
	guardTestPoisonID = "acme.ops.robotics.gcs.sensor.001"
	guardTestValidID  = "acme.ops.robotics.gcs.sensor.002"
)

// TestBootSweepInventoriesResidentPoisonAndBoots: resident poison from before
// this boot is inventoried (ERROR + gauge + Health degraded), ingest still
// boots, queries of the poisoned entity refuse per-entity, and queries of a
// valid entity serve (spec: "resident poison from before this boot is
// inventoried" + "one poisoned entity does not take down the query surface").
// Not parallel: asserts on the component-local slog capture.
func TestBootSweepInventoriesResidentPoisonAndBoots(t *testing.T) {
	watcher := newIngestGuardWatcher(4)
	watcher.updates <- &mockKVEntry{key: guardTestPoisonID, revision: 3, data: guardTestPoisonBytes(guardTestPoisonID)}
	watcher.updates <- &mockKVEntry{key: guardTestValidID, revision: 4, data: guardTestValidBytes(guardTestValidID)}
	watcher.updates <- nil
	bucket := newMockKVBucket()
	bucket.watchAllFactory = func() (jetstream.KeyWatcher, error) { return watcher, nil }
	bucket.data[guardTestPoisonID] = mockKVData{value: guardTestPoisonBytes(guardTestPoisonID), revision: 3}
	bucket.data[guardTestValidID] = mockKVData{value: guardTestValidBytes(guardTestValidID), revision: 4}
	logs := newIngestGuardLogBuffer()
	c := newIngestGuardTestComponent(bucket)
	c.logger = slog.New(slog.NewTextHandler(logs, nil))

	// The sweep completes: a poisoned entity does not fail it.
	if err := c.startEntityStateGuard(context.Background(), c.entityBucket); err != nil {
		t.Fatalf("boot sweep: %v", err)
	}

	rec, ok := poisonInventoryEntry(c, guardTestPoisonID)
	if !ok {
		t.Fatal("resident poison not inventoried by boot sweep")
	}
	if rec.revision != 3 {
		t.Fatalf("inventory revision = %d, want 3", rec.revision)
	}
	if rec.contractErr.EntityID != guardTestPoisonID {
		t.Fatalf("stamped EntityID = %q, want %q", rec.contractErr.EntityID, guardTestPoisonID)
	}
	if got := testutil.ToFloat64(c.poisonedEntities); got != 1 {
		t.Fatalf("poisoned-entities gauge = %v, want 1", got)
	}
	if n := countLogOccurrences(logs, "code="+graph.ErrorCodeGraphStateResetRequired); n != 1 {
		t.Fatalf("structured ERROR count = %d, want exactly 1 (once per entity)", n)
	}

	health := c.Health()
	if health.Healthy {
		t.Fatal("Health must be unhealthy while the inventory is non-empty")
	}
	if health.Status != graph.IndexStateDegraded {
		t.Fatalf("Health status = %q, want %q (NOT reset_required)", health.Status, graph.IndexStateDegraded)
	}
	if !containsAll(health.LastError, "1 poisoned entity", guardTestPoisonID) {
		t.Fatalf("Health message missing count/ID sample: %q", health.LastError)
	}

	// Per-entity refusal: A refuses typed, B serves.
	result, err := c.handleQueryEntityNATS(context.Background(), []byte(`{"id":"`+guardTestPoisonID+`"}`))
	if result != nil {
		t.Fatalf("poisoned read result = %s, want nil", result)
	}
	assertIngestResetRequired(t, err)

	result, err = c.handleQueryEntityNATS(context.Background(), []byte(`{"id":"`+guardTestValidID+`"}`))
	if err != nil {
		t.Fatalf("valid entity read failed during poison incident: %v", err)
	}
	assertGuardExactEntity(t, result, guardTestValidID, 4)
}

// TestBootSweepLastRevisionWins: a key whose poisoned snapshot revision is
// superseded by a valid (or tombstoned) pre-marker revision ends with NO
// inventory entry (spec: "a key repaired mid-drain is not inventoried").
func TestBootSweepLastRevisionWins(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		supersede jetstream.KeyValueEntry
	}{
		{
			name:      "valid revision supersedes",
			supersede: &mockKVEntry{key: guardTestPoisonID, revision: 4, data: guardTestValidBytes(guardTestPoisonID)},
		},
		{
			name:      "delete tombstone supersedes",
			supersede: &mockKVEntry{key: guardTestPoisonID, revision: 4, op: jetstream.KeyValueDelete},
		},
		{
			name:      "purge tombstone supersedes",
			supersede: &mockKVEntry{key: guardTestPoisonID, revision: 4, op: jetstream.KeyValuePurge},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			watcher := newIngestGuardWatcher(4)
			watcher.updates <- &mockKVEntry{key: guardTestPoisonID, revision: 3, data: guardTestPoisonBytes(guardTestPoisonID)}
			watcher.updates <- tc.supersede
			watcher.updates <- nil
			bucket := newMockKVBucket()
			bucket.watchAllFactory = func() (jetstream.KeyWatcher, error) { return watcher, nil }
			c := newIngestGuardTestComponent(bucket)

			if err := c.startEntityStateGuard(context.Background(), c.entityBucket); err != nil {
				t.Fatalf("boot sweep: %v", err)
			}

			if _, ok := poisonInventoryEntry(c, guardTestPoisonID); ok {
				t.Fatal("superseded poisoned revision must leave no inventory entry")
			}
			if got := testutil.ToFloat64(c.poisonedEntities); got != 0 {
				t.Fatalf("poisoned-entities gauge = %v, want 0", got)
			}
		})
	}
}

// TestEntityStateGuardFailureIsReturned: a sweep that cannot reach the end
// of the snapshot returns a transient error, which fails Start, at each of its
// three exits; the sweep stops any watcher it opened and records no poison
// from the failure.
func TestEntityStateGuardFailureIsReturned(t *testing.T) {
	t.Parallel()

	refused := errors.New("watch refused")
	cases := []struct {
		name string
		// watch is the bucket's Watch; it closes opened once called.
		watch  func(opened chan<- struct{}, watcher *ingestGuardWatcher) (jetstream.KeyWatcher, error)
		cancel bool // cancel the sweep's context once the watcher is open
		want   error
	}{
		{
			name: "watch refused",
			watch: func(opened chan<- struct{}, _ *ingestGuardWatcher) (jetstream.KeyWatcher, error) {
				close(opened)
				return nil, refused
			},
			want: refused,
		},
		{
			name: "updates closed before the end of the snapshot",
			watch: func(opened chan<- struct{}, watcher *ingestGuardWatcher) (jetstream.KeyWatcher, error) {
				watcher.updates <- &mockKVEntry{key: guardTestPoisonID, revision: 3, data: guardTestPoisonBytes(guardTestPoisonID)}
				watcher.closeUpdates()
				close(opened)
				return watcher, nil
			},
		},
		{
			name: "context cancelled during the sweep",
			watch: func(opened chan<- struct{}, watcher *ingestGuardWatcher) (jetstream.KeyWatcher, error) {
				close(opened)
				return watcher, nil
			},
			cancel: true,
			want:   context.Canceled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			watcher := newIngestGuardWatcher(1)
			opened := make(chan struct{})
			bucket := newMockKVBucket()
			bucket.watchAllFactory = func() (jetstream.KeyWatcher, error) { return tc.watch(opened, watcher) }
			c := newIngestGuardTestComponent(bucket)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- c.startEntityStateGuard(ctx, c.entityBucket) }()
			<-opened
			if tc.cancel {
				cancel()
			}
			err := <-result

			var classified *errs.ClassifiedError
			if !errors.As(err, &classified) || classified.Class != errs.ErrorTransient {
				t.Fatalf("sweep error = %T %v, want a transient classified error", err, err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("sweep error = %v, want it to wrap %v", err, tc.want)
			}
			// Every case but the refused watch hands the sweep a watcher, which it must stop.
			if tc.want != refused && !watcher.stopped() {
				t.Fatal("the sweep's watcher was not stopped")
			}
			if c.entityPoisonSize.Load() != 0 {
				t.Fatal("a failed sweep must not record poison")
			}
		})
	}
}

// TestEntityStateGuardDeliberateStopDrainsToCloseWithoutMisclassification:
// after the end-of-snapshot marker the watcher is stopped and its channel
// consumed until close, discarding post-marker entries; the deliberate stop
// is not a sweep failure and Health is not degraded by it.
func TestEntityStateGuardDeliberateStopDrainsToCloseWithoutMisclassification(t *testing.T) {
	t.Parallel()

	watcher := newIngestGuardWatcher(8)
	watcher.updates <- &mockKVEntry{key: guardTestValidID, revision: 1, data: guardTestValidBytes(guardTestValidID)}
	watcher.updates <- nil
	// Post-marker entries: concurrent writers kept publishing. These must be
	// discarded by the drain-to-close, NOT validated, NOT inventoried, and the
	// channel closure they precede must not be misclassified.
	watcher.updates <- &mockKVEntry{key: guardTestPoisonID, revision: 2, data: guardTestPoisonBytes(guardTestPoisonID)}
	watcher.updates <- &mockKVEntry{key: guardTestValidID, revision: 3, data: guardTestValidBytes(guardTestValidID)}
	bucket := newMockKVBucket()
	bucket.watchAllFactory = func() (jetstream.KeyWatcher, error) { return watcher, nil }
	c := newIngestGuardTestComponent(bucket)

	done := make(chan error, 1)
	go func() { done <- c.startEntityStateGuard(context.Background(), c.entityBucket) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("deliberate stop reported as a sweep failure: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("guard did not drain to close after the deliberate stop")
	}

	if !watcher.stopped() {
		t.Fatal("watcher was not stopped after the snapshot marker")
	}
	if c.entityPoisonSize.Load() != 0 {
		t.Fatal("post-marker discarded entries must not be inventoried")
	}
	health := c.Health()
	if !health.Healthy {
		t.Fatalf("Health degraded by the stopped watcher: %+v", health)
	}
}

// TestQueryDiscoveredPoisonServesConcurrentReadyResponse is the review-named
// INVERSION of TestQueryDiscoveredPoisonBlocksConcurrentReadyResponse: with
// the surface-global latch gone, a valid response in flight while another
// query discovers poison now SERVES (design D2 — enforcement is per-read
// byte-authoritative; the inventory gates nothing).
func TestQueryDiscoveredPoisonServesConcurrentReadyResponse(t *testing.T) {
	t.Parallel()

	valid := guardTestValidBytes(guardTestValidID)
	poison := guardTestPoisonBytes(guardTestPoisonID)
	validAtGetBarrier := make(chan struct{})
	releaseValid := make(chan struct{})
	var barrierOnce sync.Once
	bucket := newMockKVBucket()
	bucket.getFunc = func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
		switch key {
		case guardTestValidID:
			barrierOnce.Do(func() {
				close(validAtGetBarrier)
				<-releaseValid
			})
			return &mockKVEntry{key: key, revision: 2, data: valid}, nil
		case guardTestPoisonID:
			return &mockKVEntry{key: key, revision: 3, data: poison}, nil
		default:
			return nil, jetstream.ErrKeyNotFound
		}
	}
	c := newIngestGuardTestComponent(bucket)

	type result struct {
		data []byte
		err  error
	}
	validResult := make(chan result, 1)
	go func() {
		data, err := c.handleQueryEntityNATS(context.Background(), []byte(`{"id":"`+guardTestValidID+`"}`))
		validResult <- result{data: data, err: err}
	}()
	<-validAtGetBarrier

	// While the valid query is parked mid-read, another query discovers and
	// inventories poison.
	data, err := c.handleQueryEntityNATS(context.Background(), []byte(`{"id":"`+guardTestPoisonID+`"}`))
	if data != nil {
		t.Fatalf("poison query result = %s, want nil", data)
	}
	assertIngestResetRequired(t, err)
	if _, ok := poisonInventoryEntry(c, guardTestPoisonID); !ok {
		t.Fatal("query-discovered poison not inventoried")
	}

	close(releaseValid)
	concurrent := <-validResult
	if concurrent.err != nil {
		t.Fatalf("concurrent valid response refused after poison discovery: %v", concurrent.err)
	}
	assertGuardExactEntity(t, concurrent.data, guardTestValidID, 2)
}

func assertGuardExactEntity(t *testing.T, data []byte, entityID string, revision uint64) {
	t.Helper()
	var exact graph.ExactEntity
	if err := json.Unmarshal(data, &exact); err != nil {
		t.Fatalf("decode exact entity response: %v", err)
	}
	if exact.Entity == nil || exact.Entity.ID != entityID || exact.KVRevision != revision {
		t.Fatalf("exact entity = %#v, want id=%q revision=%d", exact, entityID, revision)
	}
}

// TestBatchBackendFailureReturnsNoPartialEntities: a non-poison backend
// failure inside a batch still fails the whole read transiently (unchanged
// aggregate fail-the-batch contract for operational errors).
func TestBatchBackendFailureReturnsNoPartialEntities(t *testing.T) {
	t.Parallel()

	bucket := newMockKVBucket()
	bucket.getFunc = func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
		if key == guardTestValidID {
			return &mockKVEntry{key: key, data: guardTestValidBytes(guardTestValidID)}, nil
		}
		return nil, errors.New("backend unavailable")
	}
	c := newIngestGuardTestComponent(bucket)

	result, err := c.handleQueryBatchNATS(context.Background(), []byte(
		`{"ids":["`+guardTestValidID+`","`+guardTestPoisonID+`"]}`,
	))
	if result != nil {
		t.Fatalf("partial result = %s, want nil", result)
	}
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorTransient {
		t.Fatalf("error = %T %v, want transient backend failure", err, err)
	}
}

// ----------------------------------------------------------------------------
// Shared test scaffolding for the guard/poison suites.
// ----------------------------------------------------------------------------

func newIngestGuardTestComponent(bucket *mockKVBucket) *Component {
	c := &Component{
		entityBucket:    bucketStoreForTest(bucket),
		payloadRegistry: mustTestPayloadRegistry(),
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		running:         true,
		// Fresh, unregistered gauge per component so tests can assert values
		// without cross-test interference on the process-global metric.
		poisonedEntities: prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_poisoned_entities"}),
	}
	c.lastActivity.Store(time.Now())
	return c
}

// bucketStoreForTest uses the package's existing test client helper.
func bucketStoreForTest(bucket jetstream.KeyValue) *natsclient.KVStore {
	client, err := natsclient.NewClient("") // never connected; the pin named a fixed address
	if err != nil {
		panic(err)
	}
	return client.NewKVStore(bucket)
}

// ingestGuardWatcher is the mock snapshot watcher. Its Stop() CLOSES the
// updates channel exactly once — matching real nats.go semantics, which the
// production drain-to-close depends on (a Stop that left the channel open
// would hang the drain forever).
type ingestGuardWatcher struct {
	updates    chan jetstream.KeyValueEntry
	closeOnce  sync.Once
	wasStopped sync.Once
	stopFlag   chan struct{}
}

func newIngestGuardWatcher(capacity int) *ingestGuardWatcher {
	return &ingestGuardWatcher{
		updates:  make(chan jetstream.KeyValueEntry, capacity),
		stopFlag: make(chan struct{}),
	}
}

func (w *ingestGuardWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }

func (w *ingestGuardWatcher) Stop() error {
	w.wasStopped.Do(func() { close(w.stopFlag) })
	w.closeUpdates()
	return nil
}

// closeUpdates closes the channel once — shared by Stop and by tests that
// simulate a transport failure (server-side closure without Stop).
func (w *ingestGuardWatcher) closeUpdates() {
	w.closeOnce.Do(func() { close(w.updates) })
}

func (w *ingestGuardWatcher) stopped() bool {
	select {
	case <-w.stopFlag:
		return true
	default:
		return false
	}
}

// ingestGuardLogBuffer is a mutex-guarded log sink safe for cross-goroutine
// capture-then-assert.
type ingestGuardLogBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func newIngestGuardLogBuffer() *ingestGuardLogBuffer { return &ingestGuardLogBuffer{} }

func (b *ingestGuardLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *ingestGuardLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func countLogOccurrences(b *ingestGuardLogBuffer, needle string) int {
	return strings.Count(b.String(), needle)
}

func containsAll(s string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(s, needle) {
			return false
		}
	}
	return true
}
