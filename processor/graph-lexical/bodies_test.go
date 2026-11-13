package graphlexical

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/internal/revlag"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/storage"
)

// readDeadline is graph/entitytext's bound on one body read.
const readDeadline = 10 * time.Second

// errStoreTimeout is what the NATS object store returns when a read's deadline passes with the
// read in flight: its own timeout, which wraps no context error.
var errStoreTimeout = errors.New("nats: timeout")

// fakeStore is a streamable store whose Open each test supplies; it counts the opens of each key.
type fakeStore struct {
	open func(ctx context.Context, key string) (io.ReadCloser, error)

	mu     sync.Mutex
	opened map[string]int
}

func newFakeStore(open func(ctx context.Context, key string) (io.ReadCloser, error)) *fakeStore {
	return &fakeStore{open: open, opened: map[string]int{}}
}

func (*fakeStore) Put(context.Context, string, []byte) error      { return nil }
func (*fakeStore) Get(context.Context, string) ([]byte, error)    { return nil, errors.New("not read") }
func (*fakeStore) List(context.Context, string) ([]string, error) { return nil, nil }
func (*fakeStore) Delete(context.Context, string) error           { return nil }
func (s *fakeStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	s.opened[key]++
	s.mu.Unlock()
	return s.open(ctx, key)
}

// opens is the number of opens of key, or of every key when key is "".
func (s *fakeStore) opens(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "" {
		return s.opened[key]
	}
	n := 0
	for _, c := range s.opened {
		n += c
	}
	return n
}

// serving answers every key with body(key) at once.
func serving(body func(key string) string) *fakeStore {
	return newFakeStore(func(_ context.Context, key string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(body(key))), nil
	})
}

// yielding is a read that gives way to other goroutines before each chunk.
type yielding struct{ r io.Reader }

func (y yielding) Read(p []byte) (int, error) {
	runtime.Gosched()
	return y.r.Read(p)
}

func (yielding) Close() error { return nil }

// blockedReader is a read in flight that blocks until the context its stream was opened under
// ends, then fails with the store's own timeout, as the NATS object store does.
type blockedReader struct{ ctx context.Context }

func (r blockedReader) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, errStoreTimeout
}

func (blockedReader) Close() error { return nil }

// fakeStores is a store registry that counts the lookups of each instance.
type fakeStores struct {
	stores map[string]storage.StreamableStore

	mu     sync.Mutex
	lookup map[string]int
}

func newFakeStores(stores map[string]storage.StreamableStore) *fakeStores {
	return &fakeStores{stores: stores, lookup: map[string]int{}}
}

func (s *fakeStores) Streamable(instance string) (storage.StreamableStore, bool) {
	s.mu.Lock()
	s.lookup[instance]++
	s.mu.Unlock()
	store, ok := s.stores[instance]
	return store, ok
}

func (s *fakeStores) lookups(instance string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookup[instance]
}

// syncBuffer is a log destination the body goroutine writes and the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// rig is graph-lexical's apply side without NATS: the applier, the body path running on a
// goroutine of its own, and a revlag.Watermark, the follower's own, as the floor.
type rig struct {
	t      *testing.T
	floor  *revlag.Watermark
	apply  *applier
	bodies *bodyPath
	logs   *syncBuffer

	mu      sync.Mutex
	target  uint64
	reached chan struct{} // closed when the floor reaches target
}

// newRig starts a rig whose bodies are read from stores, at most maxLen characters; the test's
// cleanup stops the body path and joins it.
func newRig(t *testing.T, stores entitytext.Stores, maxLen int) *rig {
	t.Helper()
	metrics, err := newLexicalMetrics(metric.NewMetricsRegistry())
	if err != nil {
		t.Fatalf("newLexicalMetrics: %v", err)
	}
	r := &rig{t: t, floor: revlag.New(), logs: &syncBuffer{}}
	logger := slog.New(slog.NewTextHandler(r.logs, nil))
	r.apply = newApplier(entitytext.New(nil, maxLen), maxLen, r.complete, metrics, logger)
	r.bodies = newBodyPath(r.apply, stores, maxLen, logger)
	ctx, cancel := context.WithCancel(t.Context())
	var running sync.WaitGroup
	running.Go(func() { r.bodies.run(ctx) })
	t.Cleanup(func() {
		cancel()
		running.Wait()
	})
	return r
}

// complete is the applier's completion: the floor's, and the signal once it reaches the target.
func (r *rig) complete(key string, revision uint64) {
	r.floor.Complete(key, revision)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reached != nil && r.indexed() >= r.target {
		close(r.reached)
		r.reached = nil
	}
}

func (r *rig) indexed() uint64 {
	indexed, _ := r.floor.IndexedAt()
	return indexed
}

// deliver hands e to the applier as the follower and component.deliver do: observed by the
// floor, applied, and its body read queued on the body path.
func (r *rig) deliver(e follower.Entry) {
	r.t.Helper()
	if err := r.floor.Observe(e.Revision, e.Key, time.Time{}); err != nil {
		r.t.Fatalf("observe: %v", err)
	}
	if read, ok := mustDeliver(r.t, r.apply, e); ok {
		r.bodies.enqueue(read)
	}
}

// awaitFloor returns once the floor reaches revision: every delivered revision at or below it is
// complete.
func (r *rig) awaitFloor(revision uint64) {
	r.mu.Lock()
	if r.indexed() >= revision {
		r.mu.Unlock()
		return
	}
	reached := make(chan struct{})
	r.target, r.reached = revision, reached
	r.mu.Unlock()
	<-reached
}

// requireBodies fails unless the bodies_not_indexed gauge reads want by state, and the follower's
// Failed reports the unread ones under body_unread.
func (r *rig) requireBodies(want map[string]int) {
	r.t.Helper()
	for _, name := range bodyStateNames[1:] {
		if got := testutil.ToFloat64(r.apply.metrics.bodies.WithLabelValues(name)); got != float64(want[name]) {
			r.t.Errorf("bodies_not_indexed{state=%q} = %v; want %d", name, got, want[name])
		}
	}
	n, reasons := r.apply.failed()
	wantReasons := map[string]uint64(nil)
	if want["unread"] > 0 {
		wantReasons = map[string]uint64{"body_unread": uint64(want["unread"])}
	}
	if n != int64(want["unread"]) || !maps.Equal(reasons, wantReasons) {
		r.t.Errorf("Failed = %d %v; want %d %v", n, reasons, want["unread"], wantReasons)
	}
}

// TestBlockingStoreBoundsFirstBuild: 10,000 entities have bodies on a store whose reads block
// until their context ends and then fail with the store's own timeout, which wraps no context
// error. The first build completes one read deadline after the deliveries, every body counted
// unread, and the store is opened once.
func TestBlockingStoreBoundsFirstBuild(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocking := newFakeStore(func(ctx context.Context, _ string) (io.ReadCloser, error) {
			return blockedReader{ctx: ctx}, nil
		})
		r := newRig(t, newFakeStores(map[string]storage.StreamableStore{"objectstore": blocking}), defaultMaxTextLen)

		const entities = 10_000
		start := time.Now()
		for i := range entities {
			key := fmt.Sprintf("c360.lex.test.doc.pump.p%d", i)
			r.deliver(stateEntry(t, key, uint64(i+1), text{name: "pump", offloaded: true}))
		}
		r.awaitFloor(entities)
		if elapsed := time.Since(start); elapsed > readDeadline {
			t.Fatalf("the first build took %v on the bubble's clock; want at most one read deadline, %v",
				elapsed, readDeadline)
		}
		if n := blocking.opens(""); n != 1 {
			t.Errorf("the blocking store was opened %d times; want once, then skipped", n)
		}
		r.requireBodies(map[string]int{"unread": entities})
	})
}

// TestUnreadBodyCountedThenRepaired: a store fails at once with an error of its own, which is
// no context error. The first read on it fails, the second body on it is not read, both are
// counted unread and reported as failures; once the store serves them, the next repair pass reads
// both and every count is 0.
func TestUnreadBodyCountedThenRepaired(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var failing atomic.Bool
		failing.Store(true)
		flaky := newFakeStore(func(_ context.Context, key string) (io.ReadCloser, error) {
			if failing.Load() {
				return nil, errors.New("nats: stream not found")
			}
			return io.NopCloser(strings.NewReader(key)), nil
		})
		r := newRig(t, newFakeStores(map[string]storage.StreamableStore{"objectstore": flaky}), defaultMaxTextLen)

		r.deliver(stateEntry(t, pump, 1, text{name: "pumpname", offloaded: true, bodyKey: "pumpbody"}))
		r.deliver(stateEntry(t, valve, 2, text{name: "valvename", offloaded: true, bodyKey: "valvebody"}))
		r.awaitFloor(2)
		requireHeld(t, r.apply.index, []string{pump}, "pumpname")
		requireHeld(t, r.apply.index, nil, "pumpbody", "valvebody")
		r.requireBodies(map[string]int{"unread": 2})
		if n := flaky.opens(""); n != 1 {
			t.Fatalf("the failing store was opened %d times before the repair pass; want once, then skipped", n)
		}

		failing.Store(false)
		<-time.After(repairInterval)
		synctest.Wait() // the repair pass the interval started has read both bodies
		requireHeld(t, r.apply.index, []string{pump}, "pumpname", "pumpbody")
		requireHeld(t, r.apply.index, []string{valve}, "valvebody")
		r.requireBodies(map[string]int{})
	})
}

// TestMissingAndUnresolvedBodies: one body is missing from its store and another names an
// instance no store is registered under. Neither is a failure; the store keeps serving the next
// body; a repair pass looks the instance up again and never reads the missing body again.
func TestMissingAndUnresolvedBodies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newFakeStore(func(_ context.Context, key string) (io.ReadCloser, error) {
			if key == "gone" {
				return nil, fmt.Errorf("Store.Open %q: %w", key, storage.ErrObjectNotFound)
			}
			return io.NopCloser(strings.NewReader(key)), nil
		})
		stores := newFakeStores(map[string]storage.StreamableStore{"objectstore": store})
		r := newRig(t, stores, defaultMaxTextLen)

		const drain = "c360.lex.test.doc.drain.c"
		r.deliver(stateEntry(t, pump, 1, text{name: "pumpname", offloaded: true, bodyKey: "gone"}))
		r.deliver(stateEntry(t, valve, 2, text{name: "valvename", offloaded: true, instance: "elsewhere"}))
		r.deliver(stateEntry(t, drain, 3, text{name: "drainname", offloaded: true, bodyKey: "drainbody"}))
		r.awaitFloor(3)
		requireHeld(t, r.apply.index, []string{pump}, "pumpname")
		requireHeld(t, r.apply.index, []string{valve}, "valvename")
		requireHeld(t, r.apply.index, []string{drain}, "drainname", "drainbody")
		r.requireBodies(map[string]int{"missing": 1, "no_store": 1})

		<-time.After(repairInterval)
		synctest.Wait() // the repair pass the interval started has ended
		if n := store.opens("gone"); n != 1 {
			t.Errorf("the missing body was read %d times; want once, not retried until the entity changes", n)
		}
		if n := stores.lookups("elsewhere"); n != 2 {
			t.Errorf("the unregistered instance was looked up %d times; want 2, again at the repair pass", n)
		}
		r.requireBodies(map[string]int{"missing": 1, "no_store": 1})
		for _, line := range []string{"offloaded body missing", "no store registered"} {
			if n := strings.Count(r.logs.String(), line); n != 1 {
				t.Errorf("%q logged %d times; want once:\n%s", line, n, r.logs.String())
			}
		}
	})
}

// TestBodyCutAtCap: an offloaded body longer than max_text_len has its words past the cap left
// out, its name's words found, the cut gauge counts it, and the first cut since start is logged
// once at warning level, naming the entity and the cap.
func TestBodyCutAtCap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const maxLen = 40
		bodies := map[string]string{
			"pump":  strings.Repeat("early ", 8) + "lateword",
			"valve": strings.Repeat("first ", 8) + "tailword",
		}
		store := serving(func(key string) string { return bodies[key] })
		r := newRig(t, newFakeStores(map[string]storage.StreamableStore{"objectstore": store}), maxLen)

		r.deliver(stateEntry(t, pump, 1, text{name: "pumpname", offloaded: true, bodyKey: "pump"}))
		r.deliver(stateEntry(t, valve, 2, text{name: "valvename", offloaded: true, bodyKey: "valve"}))
		r.awaitFloor(2)
		requireHeld(t, r.apply.index, []string{pump}, "pumpname", "early")
		requireHeld(t, r.apply.index, []string{valve}, "valvename", "first")
		requireHeld(t, r.apply.index, nil, "lateword", "tailword")
		if got := testutil.ToFloat64(r.apply.metrics.cut); got != 2 {
			t.Errorf("bodies_cut = %v; want 2", got)
		}
		logs := r.logs.String()
		want := fmt.Sprintf("level=WARN msg=\"entity text cut at max_text_len; words past it are not indexed\" entity=%s max_text_len=%d", pump, maxLen)
		if !strings.Contains(logs, want) || strings.Count(logs, "text cut") != 1 {
			t.Errorf("logs hold no single warning %q:\n%s", want, logs)
		}
	})
}

// TestDeliveriesAndBodyReadsConcurrent: deliveries arrive on one goroutine while the body path
// reads on another and a third reads the failure count, as the follower's heartbeat does. Once
// the floor reaches the last revision, every key holds its last revision's name and body and no
// earlier revision's words. Run under -race. Each read, and each delivery, gives way to the other
// goroutines, as a read or a watch waiting on the network does, so deliveries land while reads
// are out, even on one CPU.
func TestDeliveriesAndBodyReadsConcurrent(t *testing.T) {
	store := newFakeStore(func(_ context.Context, key string) (io.ReadCloser, error) {
		return yielding{strings.NewReader(key)}, nil
	})
	r := newRig(t, newFakeStores(map[string]storage.StreamableStore{"objectstore": store}), defaultMaxTextLen)

	keys := []string{pump, valve, "c360.lex.test.doc.drain.c"}
	const rounds = 300
	heartbeat, stop := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(heartbeat)
		for {
			select {
			case <-stop:
				return
			default:
				r.apply.failed()
				runtime.Gosched()
			}
		}
	}()
	// Every fifth revision has no offloaded body, the last of each key among the rest.
	revision := uint64(0)
	want := map[string][]string{}
	for round := range rounds {
		for k, key := range keys {
			revision++
			tx := text{name: fmt.Sprintf("k%dr%dn", k, revision), offloaded: round%5 != 0,
				bodyKey: fmt.Sprintf("k%dr%db", k, revision)}
			want[key] = []string{tx.name}
			if tx.offloaded {
				want[key] = append(want[key], tx.bodyKey)
			}
			r.deliver(stateEntry(t, key, revision, tx))
			runtime.Gosched()
		}
	}
	r.awaitFloor(revision)
	close(stop)
	<-heartbeat

	for k, key := range keys {
		requireHeld(t, r.apply.index, []string{key}, want[key]...)
		for rev := uint64(1); rev <= revision; rev++ {
			if name := fmt.Sprintf("k%dr%dn", k, rev); name != want[key][0] {
				requireHeld(t, r.apply.index, nil, name, fmt.Sprintf("k%dr%db", k, rev))
			}
		}
	}
	r.requireBodies(map[string]int{})
}
