// Package revlag provides a revision-lag "caught up" watermark for async,
// write-driven, lagging pipelines fed by a NATS KV watch (ADR-066).
//
// A component that watches a KV bucket and processes each entry through a pool
// (with optional coalescing and inline deletes) needs to answer "have I caught up
// to the latest committed write?" honestly — not "have I started?". The Watermark
// tracks that as revision lag: Observe every delivered revision, Complete it when
// its processing reaches a terminal outcome, and read the floor from IndexedAt —
// the highest revision such that every delivered revision <= it is done and
// nothing <= it is still in flight.
//
// It also answers "how OLD is the view I am serving?" (ADR-083): IndexedAt pairs
// the watermark with the KV COMMIT timestamp of the newest observed revision it
// covers, so a consumer can bound staleness in wall time instead of in revisions
// (a revision count is load- and coalesce-dependent; see gh#590).
package revlag

import (
	"container/list"
	"slices"
	"sync"
	"time"
)

// Watermark is the low-water-of-pending "caught up" tracker. It answers "every
// DELIVERED revision <= the floor has completed and nothing <= it is still in
// flight." It is deliberately defined only over DELIVERED revisions, so it is
// correct even when the delivered revision set is SPARSE — as it is for a NATS KV
// bucket at History=1, where WatchAll delivers via OrderedConsumer +
// DeliverLastPerSubject and superseded revisions are purged and never delivered.
// The rejected "advance from Indexed+1 past every contiguous completion" would
// stall forever on the first purged gap.
//
// Correctness rests on ONE property, which OrderedConsumer guarantees at the
// nats-server storage layer for both the bootstrap replay and live updates:
// delivery is monotonic ascending in the revision passed to Observe. Any
// un-observed revision below observedHigh is therefore purged (permanently absent)
// and correctly skipped. That same property makes the floor monotonic
// non-decreasing, which is what lets the covered COMMIT TIME be carried forward as
// a single scalar (see coveredRev/coveredAt below) instead of a growing map.
//
// All methods are safe for concurrent use: the watch goroutine Observes; N pool
// workers, a coalescer callback, and/or an inline delete handler Complete; a status
// handler reads IndexedAt().
type Watermark struct {
	mu             sync.Mutex
	observedHigh   uint64    // highest revision ever delivered
	observedHighAt time.Time // commit time of observedHigh
	// order lists the in-flight revisions in ascending order, so the floor is read
	// from its front; pending maps each in-flight revision to its element there;
	// byKey lists each key's in-flight revisions, so Complete visits only that key's.
	// A drained revision leaves all three at once: they are the ONLY per-revision
	// state, they drain to empty when the pipeline catches up, and memory tracks
	// in-flight work, never cumulative throughput.
	pending map[uint64]*list.Element
	order   list.List
	byKey   map[string][]uint64
	// coveredRev is the newest revision known to be BOTH observed and covered by
	// the current floor; coveredAt is its commit time. Carried forward as
	// one scalar pair rather than retaining timestamps for every completed revision
	// (which would grow without bound while one slow key pins the floor).
	//
	// It is advanced only from evidence in hand at drain time, so it can lag the
	// true newest-covered revision when completions land out of order ABOVE the
	// floor: those timestamps are dropped rather than retained. The resulting
	// commit time is therefore <= the true one, i.e. staleness computed from it is
	// never UNDER-reported — the fail-closed direction. The error is bounded by the
	// in-flight window and collapses to exact whenever the pipeline catches up
	// (pending empties → coveredRev = observedHigh).
	coveredRev uint64
	coveredAt  time.Time
}

// inflight is the per-pending-revision record: the revision, the key it belongs to
// (completion is key-scoped) and the KV entry's commit time (entry.Created()).
type inflight struct {
	rev      uint64
	key      string
	commitAt time.Time
}

// New returns an empty Watermark (its floor is 0 until the first Observe).
func New() *Watermark {
	return &Watermark{pending: make(map[uint64]*list.Element), byKey: make(map[string][]uint64)}
}

// Observe records revision r (for key, committed at commitAt) as
// delivered-and-in-flight. Call it from the watch goroutine for EVERY delivered
// entry — updates AND deletes — before dispatch, passing the KV entry's COMMIT
// timestamp (entry.Created()), never the local arrival time: commit time is what
// makes staleness mean "the view reflects the world as of T" and correctly includes
// server-side delivery backlog once an entry lands. A zero commitAt is allowed (it
// simply leaves the covered timestamp unknown, which reads as "staleness not
// computable" downstream) so tests and legacy paths need not fabricate one.
//
// observedHigh is monotonic; ascending delivery means r exceeds any prior, but max()
// is kept for defensiveness. Revision 0 ("no revision") is ignored — KV stream
// sequences start at 1. A revision observed again while in flight takes the newer
// key and commit time. Ascending delivery appends in constant time; a lower revision
// walks back from the highest in-flight revision to its place.
func (w *Watermark) Observe(r uint64, key string, commitAt time.Time) {
	if r == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if r > w.observedHigh {
		w.observedHigh = r
		w.observedHighAt = commitAt
	}
	in := &inflight{rev: r}
	if e, ok := w.pending[r]; ok {
		in = e.Value.(*inflight)
		w.dropFromKeyLocked(in.key, r)
	} else {
		w.pending[r] = w.insertLocked(in)
	}
	in.key, in.commitAt = key, commitAt
	w.byKey[key] = append(w.byKey[key], r)
}

// insertLocked places in among the in-flight revisions in ascending order and
// returns its element; r is not already in flight. Caller holds w.mu.
func (w *Watermark) insertLocked(in *inflight) *list.Element {
	for e := w.order.Back(); e != nil; e = e.Prev() {
		if e.Value.(*inflight).rev < in.rev {
			return w.order.InsertAfter(in, e)
		}
	}
	return w.order.PushFront(in)
}

// dropFromKeyLocked removes r from key's in-flight revisions, and the key once it
// has none. Caller holds w.mu.
func (w *Watermark) dropFromKeyLocked(key string, r uint64) {
	revs := slices.DeleteFunc(w.byKey[key], func(x uint64) bool { return x == r })
	if len(revs) == 0 {
		delete(w.byKey, key)
		return
	}
	w.byKey[key] = revs
}

// Complete drains every in-flight revision for key with revision <= rev — the single
// key-scoped completion rule. A pool worker calls it with the entry it processed,
// which drains any coalescer-collapsed lower revisions of that key that no worker
// sees individually; an inline delete handler calls it with the tombstone's
// revision, which drains an earlier pending update the delete supersedes. Key-scoped
// (never global-<=-rev) so one key's completion cannot drop a different key's pending
// revision. It visits only key's own in-flight revisions.
func (w *Watermark) Complete(key string, rev uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var drained []*inflight
	var kept []uint64
	for _, r := range w.byKey[key] {
		if r > rev {
			kept = append(kept, r)
			continue
		}
		e := w.pending[r]
		w.order.Remove(e)
		delete(w.pending, r)
		drained = append(drained, e.Value.(*inflight))
	}
	if len(drained) == 0 {
		return
	}
	if len(kept) == 0 {
		delete(w.byKey, key)
	} else {
		w.byKey[key] = kept
	}
	// Every drained revision has left the in-flight set, so the covered floor
	// advances over the post-drain state.
	w.advanceCoveredLocked(drained)
}

// advanceCoveredLocked moves the covered (rev, commit time) pair forward using the
// revisions just drained, each with its commit time. Caller holds w.mu.
//
// Two cases, mirroring indexedLocked:
//   - pending empty: the floor IS observedHigh, which was itself observed, so the
//     covered timestamp is exactly observedHighAt. This is the caught-up case an
//     operator sees most, and it is exact.
//   - pending non-empty: the floor is one less than the lowest in-flight revision
//     (the front of order) and the only newly-covered revisions are those just
//     drained at or below it. A revision drained EARLIER while it sat above the
//     then-floor is not reconsidered — that is the deliberate memory/precision
//     trade documented on coveredRev.
func (w *Watermark) advanceCoveredLocked(drained []*inflight) {
	if len(w.pending) == 0 {
		if w.observedHigh > w.coveredRev {
			w.coveredRev = w.observedHigh
			w.coveredAt = w.observedHighAt
		}
		return
	}
	floor := w.indexedLocked()
	for _, in := range drained {
		if in.rev <= floor && in.rev > w.coveredRev {
			w.coveredRev = in.rev
			w.coveredAt = in.commitAt
		}
	}
}

// Observed returns the highest revision ever DELIVERED, regardless of whether it has
// been applied. It is the enumeration-time target a bootstrap latch needs: when the
// initial-sync sentinel fires, every pre-existing entity has been delivered, so their
// revisions are all <= this value, and the initial build is complete once the floor
// IndexedAt returns reaches it.
//
// Distinct from that floor, which is the low-water mark of what is APPLIED. Comparing
// against a freshly-read stream LastSeq instead would make a bootstrap latch unreachable
// under continuous write, because that target advances as fast as the index does.
func (w *Watermark) Observed() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.observedHigh
}

// IndexedAt returns the low-water-of-pending floor: observedHigh when nothing is in
// flight, else minPending-1. Every delivered revision <= the floor is complete and
// nothing <= it is still pending.
//
// It returns the floor together with the COMMIT time of the newest observed
// revision that floor covers — the "the view reflects the world as of T"
// input to the ADR-083 staleness_ms field.
//
// The two are read under one lock so a caller cannot pair a floor with a timestamp
// from a different instant. commitAt is the ZERO time when the floor covers no
// observed revision (cold start, or a floor that sits below every delivered
// revision because the lowest one is still in flight) — callers MUST treat zero as
// "staleness not computable", never as "zero staleness". commitAt may be older than
// the floor's true coverage but never newer (see coveredRev).
func (w *Watermark) IndexedAt() (indexed uint64, commitAt time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.indexedLocked(), w.coveredAt
}

// indexedLocked computes the floor IndexedAt returns from the lowest in-flight
// revision, the front of order, without a scan; caller holds w.mu.
func (w *Watermark) indexedLocked() uint64 {
	lowest := w.order.Front()
	if lowest == nil {
		return w.observedHigh
	}
	return lowest.Value.(*inflight).rev - 1
}
