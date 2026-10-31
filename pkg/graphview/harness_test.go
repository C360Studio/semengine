package graphview

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// Every test that runs a view does so inside a synctest.Test bubble, so time is the bubble's
// fake clock and nothing waits on the wall clock. The fake watch's update channel is
// unbuffered: a send returns once Run has received the entry, and the synctest.Wait after it
// returns once Run has applied it, fired its hooks and blocked again. A tick is
// <-time.After(testTick) followed by synctest.Wait: the bubble's clock crosses exactly one tick
// of Run's ticker, and Wait returns once Run has fanned that window out and every delivery
// goroutine has blocked again.

// testTick is the tick interval of every test view.
const testTick = 100 * time.Millisecond

// fakeEntry implements jetstream.KeyValueEntry deterministically.
type fakeEntry struct {
	key     string
	value   []byte
	rev     uint64
	op      jetstream.KeyValueOp
	created time.Time
}

func (e fakeEntry) Bucket() string                  { return "FAKE" }
func (e fakeEntry) Key() string                     { return e.key }
func (e fakeEntry) Value() []byte                   { return e.value }
func (e fakeEntry) Revision() uint64                { return e.rev }
func (e fakeEntry) Created() time.Time              { return e.created }
func (e fakeEntry) Delta() uint64                   { return 0 }
func (e fakeEntry) Operation() jetstream.KeyValueOp { return e.op }

// fakeWatcher is a KeyWatcher the test drives: it sends entries (nil is the end-of-replay
// marker) and calls lose to close the update channel while Run's context is live. Its update
// channel also closes when the watch's context ends, as nats.go's does (probe P11, PR #458
// comment 6472856197); the pin's fake ignored the context (pin harness_test.go:56). Stop only
// counts calls: at nats.go v1.55.0 Stop unsubscribes, and the channel closes when the delivery
// ends (internal/kvwatch).
type fakeWatcher struct {
	updates   chan jetstream.KeyValueEntry
	closeOnce sync.Once
	stops     atomic.Int32
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{updates: make(chan jetstream.KeyValueEntry)}
}

func (w *fakeWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *fakeWatcher) Stop() error                             { w.stops.Add(1); return nil }

// lose closes the update channel once, whoever calls it first: the test or the watch's
// context ending.
func (w *fakeWatcher) lose() { w.closeOnce.Do(func() { close(w.updates) }) }

// fakeSource hands out queued fake watchers, one per WatchAll, and fails WatchAll when the
// queue is empty.
type fakeSource struct {
	mu     sync.Mutex
	queue  []*fakeWatcher
	opened int
}

func (s *fakeSource) WatchAll(ctx context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opened++
	if len(s.queue) == 0 {
		return nil, errors.New("fake source: no watcher queued")
	}
	w := s.queue[0]
	s.queue = s.queue[1:]
	context.AfterFunc(ctx, w.lose)
	return w, nil
}

// decodeTest is the injected validating decode for unit tests:
// "SKIP" -> keep=false; "POISON..." -> contract error; else the string value.
func decodeTest(_ string, value []byte, _ EntryMeta) (string, bool, error) {
	s := string(value)
	switch {
	case strings.HasPrefix(s, "POISON"):
		return "", false, errors.New("test contract violation: " + s)
	case s == "SKIP":
		return "", false, nil
	default:
		return s, true, nil
	}
}

// recorder keeps every hook call a harness view makes. A test reads it after synctest.Wait,
// which orders the read after every hook call made so far.
type recorder struct {
	mu          sync.Mutex
	lost        []error
	subscribers []int
	fanOuts     [][2]int // {overwritten, maxPending}
}

func (r *recorder) hooks() Hooks {
	return Hooks{
		OnWatcherLost: func(err error) { r.mu.Lock(); r.lost = append(r.lost, err); r.mu.Unlock() },
		OnSubscribers: func(n int) { r.mu.Lock(); r.subscribers = append(r.subscribers, n); r.mu.Unlock() },
		OnFanOut: func(overwritten, maxPending int) {
			r.mu.Lock()
			r.fanOuts = append(r.fanOuts, [2]int{overwritten, maxPending})
			r.mu.Unlock()
		},
	}
}

// harness runs a View[string] over one fake watcher on its own goroutine. Build it inside a
// synctest bubble.
type harness struct {
	t    *testing.T
	view *View[string]
	src  *fakeSource
	w    *fakeWatcher
	rec  *recorder

	cancel   context.CancelFunc
	done     chan error
	returned bool
	runErr   error
}

// newHarness starts Run on a view with the recorder's hooks and waits until Run has opened its
// watch. The bubble's end cancels t.Context, which ends Run.
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessDecoding(t, decodeTest)
}

// newHarnessDecoding is newHarness with the given decode function.
func newHarnessDecoding(t *testing.T, decode DecodeFunc[string]) *harness {
	t.Helper()
	h := &harness{t: t, w: newFakeWatcher(), rec: &recorder{}}
	h.src = &fakeSource{queue: []*fakeWatcher{h.w}}
	view, err := New[string](h.src, decode, WithHooks(h.rec.hooks()), WithTickInterval(testTick))
	require.NoError(t, err)
	h.view = view
	ctx, cancel := context.WithCancel(t.Context())
	h.cancel = cancel
	h.done = make(chan error, 1)
	go func() { h.done <- view.Run(ctx) }()
	synctest.Wait() // Run has opened the watch and waits for its first entry
	return h
}

// runResult returns Run's error, failing the test when Run has not returned once every
// goroutine in the bubble is blocked.
func (h *harness) runResult() error {
	h.t.Helper()
	synctest.Wait() // Run has returned, or is blocked
	if !h.returned {
		select {
		case h.runErr = <-h.done:
			h.returned = true
		default:
			h.t.Fatal("Run has not returned")
		}
	}
	return h.runErr
}

// stop ends Run's context and returns Run's error.
func (h *harness) stop() error {
	h.t.Helper()
	h.cancel()
	return h.runResult()
}

// send hands one entry to Run and waits until Run has applied it.
func (h *harness) send(e jetstream.KeyValueEntry) {
	h.t.Helper()
	h.w.updates <- e
	synctest.Wait() // Run has applied the entry and fired its hooks
}

// put sends a Put entry and waits until the view applied it.
func (h *harness) put(key, val string, rev uint64) {
	h.t.Helper()
	h.send(fakeEntry{key: key, value: []byte(val), rev: rev, op: jetstream.KeyValuePut})
}

// del sends a Delete tombstone and waits until the view applied it.
func (h *harness) del(key string, rev uint64) {
	h.t.Helper()
	h.send(fakeEntry{key: key, rev: rev, op: jetstream.KeyValueDelete})
}

// purge sends a Purge tombstone and waits until the view applied it.
func (h *harness) purge(key string, rev uint64) {
	h.t.Helper()
	h.send(fakeEntry{key: key, rev: rev, op: jetstream.KeyValuePurge})
}

// endReplay sends the end-of-replay marker and requires the view to be caught up.
func (h *harness) endReplay() {
	h.t.Helper()
	h.send(nil)
	require.NoError(h.t, h.view.WaitCaughtUp(h.t.Context()))
}

// tickNow lets one tick window pass and waits until the view has fanned it out and every
// delivery goroutine has blocked again.
func (h *harness) tickNow() {
	h.t.Helper()
	<-time.After(testTick)
	synctest.Wait() // Run has fanned the window out; delivery goroutines are blocked
}

// lostCalls returns the OnWatcherLost calls so far.
func (h *harness) lostCalls() []error {
	h.t.Helper()
	synctest.Wait() // every hook call made so far has returned
	h.rec.mu.Lock()
	defer h.rec.mu.Unlock()
	return append([]error(nil), h.rec.lost...)
}

// takeSubscribers returns the OnSubscribers counts reported since the last call.
func (h *harness) takeSubscribers() []int {
	h.t.Helper()
	synctest.Wait() // every hook call made so far has returned
	h.rec.mu.Lock()
	defer h.rec.mu.Unlock()
	out := h.rec.subscribers
	h.rec.subscribers = nil
	return out
}

// takeFanOuts returns the OnFanOut reports since the last call.
func (h *harness) takeFanOuts() [][2]int {
	h.t.Helper()
	synctest.Wait() // every hook call made so far has returned
	h.rec.mu.Lock()
	defer h.rec.mu.Unlock()
	out := h.rec.fanOuts
	h.rec.fanOuts = nil
	return out
}

// recvBatch takes the batch the subscription's delivery goroutine has ready, failing the test
// when there is none once every goroutine in the bubble is blocked.
func recvBatch(t *testing.T, sub *Subscription[string]) []Delta[string] {
	t.Helper()
	synctest.Wait() // the delivery goroutine has sent its batch, or has nothing to send
	select {
	case b, ok := <-sub.Deltas():
		require.True(t, ok, "deltas channel closed while expecting a batch (Err: %v)", sub.Err())
		return b
	default:
		t.Fatal("no delta batch ready")
		return nil
	}
}

// requireNoBatch fails the test when the subscription has a batch, or has closed, once every
// goroutine in the bubble is blocked.
func requireNoBatch(t *testing.T, sub *Subscription[string]) {
	t.Helper()
	synctest.Wait() // the delivery goroutine has sent anything it had
	select {
	case b, ok := <-sub.Deltas():
		t.Fatalf("unexpected delivery: batch %v, open %v", b, ok)
	default:
	}
}

// recvClosed drains the subscription until its channel closes, returning every batch received
// on the way, and fails the test when the channel is still open once the bubble is blocked.
func recvClosed(t *testing.T, sub *Subscription[string]) [][]Delta[string] {
	t.Helper()
	var out [][]Delta[string]
	for {
		synctest.Wait() // the delivery goroutine has sent or closed, or is blocked
		select {
		case b, ok := <-sub.Deltas():
			if !ok {
				return out
			}
			out = append(out, b)
		default:
			t.Fatal("deltas channel not closed")
			return nil
		}
	}
}

// tracker materializes a subscriber's snapshot+delta stream and asserts the
// per-key revision high-water NEVER regresses — the coherence invariant every
// delivery in this suite is held to (G2/G3/G4).
type tracker struct {
	t      *testing.T
	values map[string]string
	revs   map[string]uint64
}

func newTracker(t *testing.T) *tracker {
	return &tracker{t: t, values: make(map[string]string), revs: make(map[string]uint64)}
}

func (tr *tracker) loadSnapshot(snap Snapshot[string]) {
	tr.t.Helper()
	for k, e := range snap.Entries {
		tr.values[k] = e.Value
		tr.revs[k] = e.Revision
	}
	for k, p := range snap.Poisoned {
		delete(tr.values, k)
		tr.revs[k] = p.Revision
	}
}

// apply applies a batch on the test goroutine, failing the test on a regression.
func (tr *tracker) apply(batch []Delta[string]) {
	tr.t.Helper()
	require.NoError(tr.t, tr.applyBatch(batch))
}

// applyBatch applies a batch and returns the first regression as an error, so a goroutine other
// than the test's can report it to the test goroutine (FailNow is valid only on the test's own).
func (tr *tracker) applyBatch(batch []Delta[string]) error {
	for _, d := range batch {
		if d.Revision <= tr.revs[d.Key] {
			return fmt.Errorf("delta %s %s@%d regresses the subscriber's high-water %d — stale delivery",
				d.Op, d.Key, d.Revision, tr.revs[d.Key])
		}
		tr.revs[d.Key] = d.Revision
		switch d.Op {
		case DeltaUpsert:
			tr.values[d.Key] = d.Value
		case DeltaDelete, DeltaPoison:
			delete(tr.values, d.Key)
		}
	}
	return nil
}
