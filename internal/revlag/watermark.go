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
// It also answers "how OLD is the view I am serving?" (ADR-083 decision 3, as
// amended for #408): IndexedAt pairs the floor with the KV COMMIT time of the
// oldest delivered write not yet applied (the lowest in-flight revision), or, when
// none is in flight, of the newest delivered write. A consumer can then bound
// staleness in wall time instead of in revisions (a revision count is load- and
// coalesce-dependent; see gh#590).
package revlag

import (
	"container/list"
	"fmt"
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
// It accepts revisions only in ascending order: Observe refuses a revision at or
// below Observed. That ascending delivery holds for a KV watch is not promised by
// nats.go or nats-server (ADR-066), so the watermark checks it rather than trusting
// its caller, and the caller declares the feed lost on a refusal. Because accepted
// revisions ascend, any revision below Observed that was never observed was purged
// and is correctly skipped, and the floor never decreases.
//
// All methods are safe for concurrent use: the watch goroutine Observes; N pool
// workers, a coalescer callback, and/or an inline delete handler Complete; a status
// handler reads IndexedAt().
type Watermark struct {
	mu         sync.Mutex
	observed   uint64    // highest revision accepted
	observedAt time.Time // commit time accepted with observed
	// order holds the in-flight revisions (each an *inflight) in ascending order:
	// Observe only appends, since it accepts only a revision above every other, so
	// the floor and its time are read from the front. byKey holds each key's
	// in-flight elements of order, also ascending, so Complete visits only that
	// key's. A drained revision leaves both at once: they are the only per-revision
	// state, they drain to empty when the pipeline catches up, and memory tracks
	// in-flight work, never cumulative throughput.
	order list.List
	byKey map[string][]*list.Element
}

// inflight is the record of one in-flight revision: the revision and the KV
// entry's commit time (entry.Created()).
type inflight struct {
	rev      uint64
	commitAt time.Time
}

// New returns an empty Watermark (its floor is 0 until the first Observe).
func New() *Watermark {
	return &Watermark{byKey: make(map[string][]*list.Element)}
}

// Observe records revision r (for key, committed at commitAt) as
// delivered-and-in-flight. Call it from the watch goroutine for EVERY delivered
// entry — updates AND deletes — before dispatch, passing the KV entry's COMMIT
// timestamp (entry.Created()), never the local arrival time: commit time is what
// makes staleness mean "the view reflects the world as of T" and correctly includes
// server-side delivery backlog once an entry lands. A zero commitAt is allowed (it
// leaves the time IndexedAt returns unknown while r is the revision it reports,
// which reads as "staleness not computable" downstream).
//
// Observe accepts r only when it is above Observed, and returns an error for any
// other, revision 0 ("no revision": KV stream sequences start at 1) included. A
// refused call changes nothing: a revision observed again keeps the key and commit
// time it had. A refusal means the feed broke the ascending order the floor rests
// on, so the caller must stop trusting the floor.
func (w *Watermark) Observe(r uint64, key string, commitAt time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if r <= w.observed {
		return fmt.Errorf("revlag: revision %d for key %q is not above the highest observed revision %d",
			r, key, w.observed)
	}
	w.observed, w.observedAt = r, commitAt
	e := w.order.PushBack(&inflight{rev: r, commitAt: commitAt})
	w.byKey[key] = append(w.byKey[key], e)
	return nil
}

// Complete drains every in-flight revision for key with revision <= rev — the single
// key-scoped completion rule. A pool worker calls it with the entry it processed,
// which drains any coalescer-collapsed lower revisions of that key that no worker
// sees individually; an inline delete handler calls it with the tombstone's
// revision, which drains an earlier pending update the delete supersedes. Key-scoped
// (never global-<=-rev) so one key's completion cannot drop a different key's pending
// revision. It visits only key's own in-flight revisions. A rev never observed (a
// coalescer re-Get can return a newer revision than the watch has delivered) drains
// the key's lower revisions and moves neither Observed nor the time IndexedAt
// returns to that revision.
func (w *Watermark) Complete(key string, rev uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	elems := w.byKey[key]
	// elems ascends, so the revisions to drain are a prefix of it.
	n := 0
	for n < len(elems) && elems[n].Value.(*inflight).rev <= rev {
		w.order.Remove(elems[n])
		n++
	}
	if n == len(elems) {
		delete(w.byKey, key)
		return
	}
	// slices.Delete clears the vacated tail, so the backing array holds no drained element.
	w.byKey[key] = slices.Delete(elems, 0, n)
}

// Observed returns the highest revision an Observe call accepted, whether or not it
// has completed, and 0 before any. It is the enumeration-time target a bootstrap
// latch needs: when the initial-sync sentinel fires, every pre-existing entity has
// been delivered, so their revisions are all <= this value, and the initial build is
// complete once the floor IndexedAt returns reaches it.
//
// Distinct from that floor, which is the low-water mark of what is APPLIED. Comparing
// against a freshly-read stream LastSeq instead would make a bootstrap latch unreachable
// under continuous write, because that target advances as fast as the index does.
func (w *Watermark) Observed() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.observed
}

// IndexedAt returns the low-water-of-pending floor: Observed when nothing is in
// flight, else the lowest in-flight revision minus 1. Every delivered revision <=
// the floor is complete and nothing <= it is still pending.
//
// It returns the floor together with the COMMIT time Observe accepted for the lowest
// in-flight revision, or, when none is in flight, for Observed's revision: the oldest
// delivered write the index lacks, or the newest it has. That is the input to the
// ADR-083 staleness_ms field, so now minus it is how long that write has waited; with
// nothing in flight it is an upper bound on the view's age. A completion that leaves
// the lowest in-flight revision in flight changes neither result.
//
// The two are read under one lock so a caller cannot pair a floor with a time from a
// different instant. commitAt is the ZERO time when nothing has been observed or the
// chosen revision was observed with the zero time — callers MUST treat zero as
// "staleness not computable", never as "zero staleness".
func (w *Watermark) IndexedAt() (indexed uint64, commitAt time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	lowest := w.order.Front()
	if lowest == nil {
		return w.observed, w.observedAt
	}
	in := lowest.Value.(*inflight)
	return in.rev - 1, in.commitAt
}
