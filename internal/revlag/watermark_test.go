package revlag

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// The time expectations of the tests from here to TestWatermark_Concurrent follow ADR-083
// decision 3: IndexedAt's time is the commit time of the lowest in-flight revision, or of
// Observed's revision when none is in flight. Each carries the spec requirement it proves.

// The watermark's whole reason to exist is honesty under SPARSE, out-of-order,
// multi-path completion. These tests feed sparse ascending revision streams (e.g.
// {50, 72, 100}) — NOT dense 1..N — because History=1 + DeliverLastPerSubject never
// delivers a dense set, and the rejected "advance-from-Indexed+1" algorithm would
// stall on the very first gap.

// base is a fixed origin for synthetic KV commit timestamps: every test timestamp is
// base.Add(rev * time.Second), so a commit time is trivially attributable to the
// revision that carried it and no test depends on the wall clock.
var base = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

// commitAt is the synthetic commit time for revision r.
func commitAt(r uint64) time.Time { return base.Add(time.Duration(r) * time.Second) }

// observe calls Observe on the test goroutine and fails the test if it refuses r.
func observe(t *testing.T, w *Watermark, r uint64, key string, at time.Time) {
	t.Helper()
	if err := w.Observe(r, key, at); err != nil {
		t.Fatalf("Observe(%d, %q) refused: %v", r, key, err)
	}
}

// wantIndexedAt fails the test unless IndexedAt returns floor and the commit time at.
func wantIndexedAt(t *testing.T, w *Watermark, floor uint64, at time.Time, why string) {
	t.Helper()
	gotFloor, gotAt := w.IndexedAt()
	if gotFloor != floor || !gotAt.Equal(at) {
		t.Fatalf("%s: IndexedAt = (%d, %v), want (%d, %v)", why, gotFloor, gotAt, floor, at)
	}
}

// spec: revision-watermark / The floor is one below the lowest in-flight revision
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_ColdStart(t *testing.T) {
	// Nothing observed: the commit time is ZERO, which the contract says callers must read as
	// "staleness not computable", never as "zero staleness".
	wantIndexedAt(t, New(), 0, time.Time{}, "cold start")
}

// spec: revision-watermark / The floor is one below the lowest in-flight revision
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_SparseMidBuild_LowWaterOfPending(t *testing.T) {
	w := New()
	// Sparse ascending delivery: distinct keys at purged-gap revisions.
	observe(t, w, 50, "A", commitAt(50))
	observe(t, w, 72, "B", commitAt(72))
	observe(t, w, 100, "C", commitAt(100))

	// Nothing completed: the floor is minPending-1, NOT stuck at 0 waiting for rev 1, and the
	// time is that of 50, the oldest delivered write the index lacks.
	wantIndexedAt(t, w, 49, commitAt(50), "all pending")

	// Out-of-order completion: a fast LATE worker (rev 72) cannot report the gap done, nor
	// move the time while 50 is still unapplied.
	w.Complete("B", 72)
	wantIndexedAt(t, w, 49, commitAt(50), "after completing 72 (50 still pending)")

	// Complete the low revision: the floor advances past it to the next gap, and the time
	// becomes that of 100, now the oldest unapplied write.
	w.Complete("A", 50)
	wantIndexedAt(t, w, 99, commitAt(100), "after completing 50 (100 still pending)")

	// Complete the last: pending empties, so the floor jumps to Observed (100), honestly
	// skipping the purged 51..71, 73..99 that were never delivered, with 100's own time.
	w.Complete("C", 100)
	wantIndexedAt(t, w, 100, commitAt(100), "caught up")
}

// spec: revision-watermark / Completion is key-scoped and drains lower revisions
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_CoalescerCollapse_KeyScopedDrainsLowerRevisions(t *testing.T) {
	w := New()
	// One key delivered three times (the coalescer will re-Get only the latest, 30).
	observe(t, w, 10, "K", commitAt(10))
	observe(t, w, 20, "K", commitAt(20))
	observe(t, w, 30, "K", commitAt(30))
	wantIndexedAt(t, w, 9, commitAt(10), "three pending revs of K")

	// The single completion at the re-Got latest revision drains ALL collapsed lower
	// revisions of K that no worker ever sees individually. Exact-revision completion
	// would strand 10 and 20 forever. Caught up, the time is 30's.
	w.Complete("K", 30)
	wantIndexedAt(t, w, 30, commitAt(30), "after key-scoped Complete(K,30)")
}

// TestWatermark_CoalescerCollapse_MidBuildTakesOldestInFlight drains a collapsed key while
// the pipeline is NOT caught up (a higher key is still pending): the drained revisions do not
// set the time; the oldest still in flight does.
//
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_CoalescerCollapse_MidBuildTakesOldestInFlight(t *testing.T) {
	w := New()
	observe(t, w, 10, "K", commitAt(10))
	observe(t, w, 20, "K", commitAt(20))
	observe(t, w, 30, "K", commitAt(30))
	observe(t, w, 40, "J", commitAt(40)) // still in flight; pins the floor at 39

	w.Complete("K", 30)
	wantIndexedAt(t, w, 39, commitAt(40), "J@40 pending")
}

// spec: revision-watermark / Completion is key-scoped and drains lower revisions
func TestWatermark_DeleteSupersedesPendingUpdate(t *testing.T) {
	w := New()
	// Update K@10 is delivered and in flight...
	observe(t, w, 10, "K", commitAt(10))
	// ...then a delete K@11 arrives. The delete's key-scoped completion drains the
	// tombstone AND the earlier still-pending update it supersedes.
	observe(t, w, 11, "K", commitAt(11))
	w.Complete("K", 11)
	wantIndexedAt(t, w, 11, commitAt(11), "after delete supersedes pending update")
}

// spec: revision-watermark / Completion is key-scoped and drains lower revisions
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_CompletionIsKeyScoped(t *testing.T) {
	w := New()
	observe(t, w, 10, "A", commitAt(10))
	observe(t, w, 11, "B", commitAt(11)) // a delete of a DIFFERENT key
	// Completing B must NOT drain A's pending revision even though 10 <= 11.
	w.Complete("B", 11)
	wantIndexedAt(t, w, 9, commitAt(10), "key-scoped: completing B left A@10 pending")
	w.Complete("A", 10)
	wantIndexedAt(t, w, 11, commitAt(11), "after A completes too")
}

// spec: revision-watermark / The floor is one below the lowest in-flight revision
func TestWatermark_TrailingDeleteOnlyTail(t *testing.T) {
	w := New()
	observe(t, w, 10, "A", commitAt(10))
	w.Complete("A", 10)
	wantIndexedAt(t, w, 10, commitAt(10), "after A")
	// A tail of pure deletes must still advance the watermark — their sequences
	// participate, or lag wedges forever.
	observe(t, w, 11, "B", commitAt(11))
	w.Complete("B", 11)
	wantIndexedAt(t, w, 11, commitAt(11), "trailing delete")
}

// TestWatermark_NewDeliveryAfterCatchUp is the seam between a caught-up view and the next
// delivery: the floor rises from Observed to the new revision minus one, and the time becomes
// the new revision's, the write the index now lacks.
//
// spec: revision-watermark / Observe accepts only a revision above Observed
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_NewDeliveryAfterCatchUp(t *testing.T) {
	w := New()
	observe(t, w, 10, "A", commitAt(10))
	w.Complete("A", 10)
	wantIndexedAt(t, w, 10, commitAt(10), "caught up at 10")

	observe(t, w, 50, "B", commitAt(50))
	wantIndexedAt(t, w, 49, commitAt(50), "50 in flight")
}

// TestWatermark_ObserveRefusesNonAscending: a revision at or below Observed, revision 0
// included, is refused with an error and changes no result. Each case completes under the key
// the refused call named, to show the refused revision was not filed under it.
//
// spec: revision-watermark / Observe accepts only a revision above Observed
func TestWatermark_ObserveRefusesNonAscending(t *testing.T) {
	refuse := func(t *testing.T, w *Watermark, r uint64, key string, at time.Time) {
		t.Helper()
		if err := w.Observe(r, key, at); err == nil {
			t.Fatalf("Observe(%d, %q) returned no error, want a refusal (Observed is %d)", r, key, w.Observed())
		}
	}
	wantObserved := func(t *testing.T, w *Watermark, want uint64) {
		t.Helper()
		if got := w.Observed(); got != want {
			t.Fatalf("Observed = %d, want %d", got, want)
		}
	}

	t.Run("observed again", func(t *testing.T) {
		w := New()
		observe(t, w, 10, "A", commitAt(10))
		refuse(t, w, 10, "B", commitAt(10).Add(time.Millisecond))
		wantIndexedAt(t, w, 9, commitAt(10), "after the refused re-observation")
		wantObserved(t, w, 10)
		w.Complete("B", 10)
		wantIndexedAt(t, w, 9, commitAt(10), "Complete(B, 10): 10 stays A's")
		w.Complete("A", 10)
		wantIndexedAt(t, w, 10, commitAt(10), "Complete(A, 10)")
	})
	t.Run("lower", func(t *testing.T) {
		w := New()
		observe(t, w, 50, "A", commitAt(50))
		refuse(t, w, 20, "B", commitAt(20))
		wantIndexedAt(t, w, 49, commitAt(50), "after the refused lower revision")
		wantObserved(t, w, 50)
		w.Complete("A", 50)
		wantIndexedAt(t, w, 50, commitAt(50), "Complete(A, 50): 20 never went in flight")
	})
	t.Run("zero", func(t *testing.T) {
		w := New()
		refuse(t, w, 0, "X", commitAt(1))
		wantIndexedAt(t, w, 0, time.Time{}, "after the refused revision 0")
		wantObserved(t, w, 0)
	})
}

// TestWatermark_ZeroCommitTimeStaysUnknown covers the no-timestamp path: a caller that
// cannot supply a commit time leaves the time zero rather than fabricating one, so downstream
// reports "not computable" instead of "0 ms stale" — caught up, and while the oldest in-flight
// revision is the one without a time.
//
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_ZeroCommitTimeStaysUnknown(t *testing.T) {
	w := New()
	observe(t, w, 7, "A", time.Time{})
	w.Complete("A", 7)
	wantIndexedAt(t, w, 7, time.Time{}, "caught up at a revision observed with no time")

	w = New()
	observe(t, w, 7, "A", time.Time{})
	observe(t, w, 8, "B", commitAt(8))
	wantIndexedAt(t, w, 6, time.Time{}, "oldest in flight observed with no time")
}

// spec: revision-watermark / Completion is key-scoped and drains lower revisions
func TestWatermark_CompleteUnobservedRevisionDrainsLower(t *testing.T) {
	// A coalescer re-Get can return a revision NEWER than any yet delivered to the
	// watch (a write between snapshot and Get). Completing at that not-yet-observed
	// revision must still drain the key's lower pending, and must NOT advance
	// Observed (only Observe does that).
	w := New()
	observe(t, w, 10, "K", commitAt(10))
	w.Complete("K", 40) // 40 never observed
	// The commit time is 10's — never 40's, whose entry was never delivered and whose
	// commit time the watermark has never seen.
	wantIndexedAt(t, w, 10, commitAt(10), "Observed unchanged; 10 drained")
}

// TestWatermark_Concurrent runs Observe/Complete across many goroutines to prove the
// mutex holds and that draining every observed revision leaves the floor at
// Observed (run under -race). Mirrors the real wiring: the watch goroutine
// Observes ascending; N workers Complete out of order.
//
// spec: revision-watermark / The Watermark is safe for concurrent use
func TestWatermark_Concurrent(t *testing.T) {
	w := New()
	const n = 2000

	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = string(rune('a'+i%26)) + "-" + time.Duration(i).String()
		observe(t, w, uint64(i+1), keys[i], commitAt(uint64(i+1)))
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(rev int) {
			defer wg.Done()
			w.Complete(keys[rev], uint64(rev+1))
		}(i)
	}
	wg.Wait()

	// Fully drained → the exact caught-up answer, regardless of completion order.
	wantIndexedAt(t, w, n, commitAt(n), "after draining all")
}

// TestWatermark_ObservedIsHighestAccepted pins Observed apart from the floor: it is the
// highest revision accepted whether or not that revision has completed, and a completion
// at a revision never delivered does not move it.
//
// spec: revision-watermark / Observed is the highest revision accepted
func TestWatermark_ObservedIsHighestAccepted(t *testing.T) {
	w := New()
	if got := w.Observed(); got != 0 {
		t.Fatalf("new watermark: Observed = %d, want 0", got)
	}
	observe(t, w, 50, "A", commitAt(50))
	observe(t, w, 72, "B", commitAt(72))
	if got := w.Observed(); got != 72 {
		t.Fatalf("50 and 72 in flight: Observed = %d, want 72", got)
	}
	if floor, _ := w.IndexedAt(); floor != 49 {
		t.Fatalf("50 and 72 in flight: floor = %d, want 49", floor)
	}
	w.Complete("B", 72)
	if got := w.Observed(); got != 72 {
		t.Fatalf("after Complete(B, 72): Observed = %d, want 72", got)
	}
	w.Complete("A", 100) // 100 was never delivered
	if got := w.Observed(); got != 72 {
		t.Fatalf("after Complete(A, 100) ahead of delivery: Observed = %d, want 72", got)
	}
	if floor, _ := w.IndexedAt(); floor != 72 {
		t.Fatalf("caught up: floor = %d, want 72", floor)
	}
}

// TestWatermark_DrainAtFloor: the drained revision 10 sits exactly at the new floor while 11
// is still in flight, so the floor is 10 and the time is 11's, the oldest unapplied write.
//
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_DrainAtFloor(t *testing.T) {
	w := New()
	observe(t, w, 10, "A", commitAt(10))
	observe(t, w, 11, "B", commitAt(11))
	w.Complete("A", 10)
	wantIndexedAt(t, w, 10, commitAt(11), "a drain at the floor")
}

// TestWatermark_TimeIsOldestInFlight: while behind, the time is that of the lowest in-flight
// revision — chosen by revision, not by commit time, since commit times are not promised to
// rise with revision.
//
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_TimeIsOldestInFlight(t *testing.T) {
	w := New()
	observe(t, w, 50, "A", commitAt(50))
	observe(t, w, 72, "B", commitAt(72))
	observe(t, w, 100, "C", commitAt(100))
	wantIndexedAt(t, w, 49, commitAt(50), "none complete")

	w = New()
	observe(t, w, 1, "A", commitAt(9))
	observe(t, w, 2, "B", commitAt(3))
	wantIndexedAt(t, w, 0, commitAt(9), "commit times falling with revision")
}

// TestWatermark_CompletionAboveInFlightKeepsTime: a completion that leaves the lowest
// in-flight revision in flight changes neither floor nor time; the time moves when the
// lowest drains.
//
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestWatermark_CompletionAboveInFlightKeepsTime(t *testing.T) {
	w := New()
	observe(t, w, 50, "A", commitAt(50))
	observe(t, w, 72, "B", commitAt(72))
	observe(t, w, 100, "C", commitAt(100))
	w.Complete("B", 72)
	wantIndexedAt(t, w, 49, commitAt(50), "72 completes above 50")
	w.Complete("A", 50)
	wantIndexedAt(t, w, 99, commitAt(100), "then 50 completes")
}

// TestWatermark_CompleteWithNothingInFlight: a Complete that drains nothing changes no
// result, on a new watermark, while another key holds the floor, and once caught up.
//
// spec: revision-watermark / Completion is key-scoped and drains lower revisions
func TestWatermark_CompleteWithNothingInFlight(t *testing.T) {
	assertUnchanged := func(t *testing.T, w *Watermark, key string, rev uint64) {
		t.Helper()
		floor, at := w.IndexedAt()
		observed := w.Observed()
		w.Complete(key, rev)
		gotFloor, gotAt := w.IndexedAt()
		if gotFloor != floor || !gotAt.Equal(at) || w.Observed() != observed {
			t.Fatalf("Complete(%q, %d) with nothing in flight changed (%d, %v, %d) to (%d, %v, %d)",
				key, rev, floor, at, observed, gotFloor, gotAt, w.Observed())
		}
	}

	w := New()
	assertUnchanged(t, w, "A", 5)

	observe(t, w, 10, "A", commitAt(10))
	observe(t, w, 20, "B", commitAt(20))
	w.Complete("A", 10)
	// B@20 holds the floor at 19 and the time at 20's. Neither A nor Z has anything in
	// flight, below or above the floor.
	wantIndexedAt(t, w, 19, commitAt(20), "setup")
	assertUnchanged(t, w, "A", 30)
	assertUnchanged(t, w, "Z", 5)
	assertUnchanged(t, w, "B", 19) // B is in flight only above 19

	w.Complete("B", 20)
	assertUnchanged(t, w, "B", 20)
	assertUnchanged(t, w, "A", 1000)
}

// reading is one read of a watermark: IndexedAt's floor and time, then Observed.
type reading struct {
	floor    uint64
	at       time.Time
	observed uint64
}

func (r reading) equal(o reading) bool {
	return r.floor == o.floor && r.at.Equal(o.at) && r.observed == o.observed
}

// TestWatermark_ConcurrentReadersAndWriters is the real wiring: one goroutine observes revisions
// in ascending order and hands each to a pool of workers that complete it, while a reader calls
// IndexedAt and Observed throughout. Keys repeat (r%16), so one completion can drain a lower
// revision of its key that another worker is about to complete; 1,500 and 2,000 have keys of
// their own, so only their own completions drain them. At checkpoints A and B the test asks the
// reader for a read begun after the writes it waited for, and the exact value read is the proof
// that a read reflects every call that returned before it. Run under -race.
//
// spec: revision-watermark / The Watermark is safe for concurrent use
// spec: revision-watermark / Observe accepts only a revision above Observed
func TestWatermark_ConcurrentReadersAndWriters(t *testing.T) {
	const workers = 8
	withheld := map[uint64]string{1500: "solo-1500", 2000: "solo-2000"}
	key := func(r uint64) string {
		if k, ok := withheld[r]; ok {
			return k
		}
		return fmt.Sprintf("r%d", r%16)
	}
	w := New()

	// completed counts the current phase's Complete calls; its Wait returns once each has
	// returned, and each worker completes a revision only after the observer's Observe of it
	// returned (the send on revs orders the two).
	var completed sync.WaitGroup
	revs := make(chan uint64)
	var writers sync.WaitGroup
	for range workers {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for r := range revs {
				w.Complete(key(r), r)
				completed.Done()
			}
		}()
	}

	toB := make(chan struct{})       // the test lets the observer go on to 1,001..2,000
	observedB := make(chan struct{}) // the observer has observed 2,000
	release := make(chan struct{})   // the test releases the completions of 1,500 and 2,000
	completed.Add(1000)
	writers.Add(1)
	go func() {
		defer writers.Done()
		defer close(revs)
		observeRange := func(from, to uint64) {
			for r := from; r <= to; r++ {
				if err := w.Observe(r, key(r), commitAt(r)); err != nil {
					t.Errorf("Observe(%d) refused: %v", r, err)
				}
				if _, ok := withheld[r]; !ok {
					revs <- r
				}
			}
		}
		observeRange(1, 1000)
		<-toB
		observeRange(1001, 2000)
		close(observedB)
		<-release
		revs <- 1500
		revs <- 2000
	}()

	reader, readAt, reads, fell := startReader(w)

	completed.Wait() // every Observe and Complete of 1..1,000 has returned
	if got, want := readAt(), (reading{1000, commitAt(1000), 1000}); !got.equal(want) {
		t.Errorf("checkpoint A: read %+v, want %+v", got, want)
	}

	completed.Add(998)
	close(toB)
	<-observedB      // every Observe of 1,001..2,000 has returned
	completed.Wait() // every Complete but 1,500's and 2,000's has returned
	if got, want := readAt(), (reading{1499, commitAt(1500), 2000}); !got.equal(want) {
		t.Errorf("checkpoint B: read %+v, want %+v", got, want)
	}

	completed.Add(2)
	close(release)
	writers.Wait()
	reader.stop()
	t.Logf("the reader read %d times", *reads)
	if len(*fell) > 0 {
		t.Fatalf("the floor fell %d times: %v", len(*fell), *fell)
	}
	// writers.Wait above ordered this read after every Observe and Complete.
	wantIndexedAt(t, w, 2000, commitAt(2000), "after all writers returned")
	if got := w.Observed(); got != 2000 {
		t.Fatalf("Observed = %d, want 2000", got)
	}
}

// freeReader is a goroutine reading a watermark in a loop until stopped.
type freeReader struct {
	done chan struct{}
	wg   sync.WaitGroup
}

// stop ends the reader and returns once it has returned, which orders the test's reads of the
// reader's counters after its last write.
func (r *freeReader) stop() {
	close(r.done)
	r.wg.Wait()
}

// startReader starts a goroutine that reads w in a loop and records every floor lower than the
// one it read before. readAt asks it for a read begun after the ask and returns that read. The
// counters it returns are written only by the reader; read them after stop.
func startReader(w *Watermark) (r *freeReader, readAt func() reading, reads *int, fell *[]string) {
	r = &freeReader{done: make(chan struct{})}
	ask := make(chan chan reading)
	reads, fell = new(int), new([]string)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		var last uint64
		read := func() reading {
			floor, at := w.IndexedAt()
			got := reading{floor: floor, at: at, observed: w.Observed()}
			*reads++
			if floor < last {
				*fell = append(*fell, fmt.Sprintf("%d after %d", floor, last))
			}
			last = floor
			return got
		}
		for {
			read()
			select {
			case reply := <-ask:
				reply <- read()
			case <-r.done:
				return
			default:
			}
		}
	}()
	readAt = func() reading {
		reply := make(chan reading)
		ask <- reply
		return <-reply
	}
	return r, readAt, reads, fell
}

// TestWatermark_RetentionTracksInFlight checks retention without reading any field of the
// watermark: a million revisions observed and completed must not grow the live heap, with
// nothing else in flight and with one revision stuck below them. One retained 8-byte entry per
// completion would grow it by about 8 MB; the bound is 1 MiB.
//
// spec: revision-watermark / Retained state tracks in-flight work
func TestWatermark_RetentionTracksInFlight(t *testing.T) {
	const n = 1_000_000
	// observeAndComplete observes and completes from..to one after another under one key.
	observeAndComplete := func(t *testing.T, w *Watermark, from, to uint64) {
		for r := from; r <= to; r++ {
			if err := w.Observe(r, "K", commitAt(r)); err != nil {
				t.Fatalf("Observe(%d) refused: %v", r, err)
			}
			w.Complete("K", r)
		}
	}

	t.Run("none stuck", func(t *testing.T) {
		w := New()
		grew := liveHeapGrowth(func() { observeAndComplete(t, w, 1, n) })
		// These reads also keep w reachable past the second measurement.
		wantIndexedAt(t, w, n, commitAt(n), "caught up")
		wantHeapBound(t, grew, n, "nothing else in flight")
	})
	t.Run("one stuck", func(t *testing.T) {
		w := New()
		observe(t, w, 1, "STUCK", commitAt(1))
		grew := liveHeapGrowth(func() { observeAndComplete(t, w, 2, n+1) })
		// These reads also keep w reachable past the second measurement.
		wantIndexedAt(t, w, 0, commitAt(1), "STUCK@1 pins the floor and the time")
		if got := w.Observed(); got != n+1 {
			t.Fatalf("Observed = %d, want %d", got, n+1)
		}
		wantHeapBound(t, grew, n, "one revision stuck in flight")
	})
}

// liveHeapGrowth returns how far the live heap grew across fill, measured after a collection
// on each side.
func liveHeapGrowth(fill func()) int64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fill()
	runtime.GC()
	runtime.ReadMemStats(&after)
	return int64(after.HeapAlloc) - int64(before.HeapAlloc)
}

func wantHeapBound(t *testing.T, grew int64, n int, what string) {
	t.Helper()
	const bound = 1 << 20
	t.Logf("live heap grew by %d bytes over %d completions, %s", grew, n, what)
	if grew >= bound {
		t.Fatalf("live heap grew by %d bytes over %d completions (%s), want under %d: completed revisions are retained",
			grew, n, what, bound)
	}
}

// TestPropWatermarkHistory is the generated check of the watermark: a history of Observe and
// Complete calls goes to the watermark and to the test's own model of the requirements, and
// after every call the watermark's result must equal the model's. The model keeps each
// in-flight revision with its key and commit time, and the highest accepted revision with its
// time. From the spec, it predicts:
//
//   - every refusal: Observe accepts only a revision above Observed, and a refused call changes
//     no result ("Observe accepts only a revision above Observed");
//   - the floor: Observed when nothing is in flight, else the lowest in-flight revision minus 1
//     ("The floor is one below the lowest in-flight revision"; the model drains as "Completion
//     is key-scoped and drains lower revisions" says);
//   - the time: the commit time accepted with the lowest in-flight revision, else with Observed's
//     revision ("IndexedAt pairs the floor with the oldest unapplied commit time");
//   - that the floor never decreases from one call to the next.
//
// Commit times are drawn apart from revisions, so they need not rise with them. Each history
// starts with the fixed prefix Observe(10, A), Observe(11, B), Complete(A, 10), which must read
// (10, 11's time), then a refused Observe(11, C), so the drain at the floor and the refusal of a
// re-observation run in every case; it ends by completing every key at the highest revision,
// so the caught-up row runs in every case. The model is sequential; the two concurrent tests
// cover concurrency under -race.
//
// spec: revision-watermark / Observe accepts only a revision above Observed
// spec: revision-watermark / The floor is one below the lowest in-flight revision
// spec: revision-watermark / Completion is key-scoped and drains lower revisions
// spec: revision-watermark / IndexedAt pairs the floor with the oldest unapplied commit time
func TestPropWatermarkHistory(t *testing.T) {
	c := propCounts{ops: map[string]int{}}
	rapid.Check(t, func(rt *rapid.T) {
		h := newPropHistory(&c)
		h.observe(rt, 10, "A", commitAt(10))
		h.observe(rt, 11, "B", commitAt(11))
		h.complete(rt, "A", 10)
		if floor, at := h.w.IndexedAt(); floor != 10 || !at.Equal(commitAt(11)) {
			rt.Fatalf("prefix: IndexedAt = (%d, %v), want (10, %v)", floor, at, commitAt(11))
		}
		h.observe(rt, 11, "C", commitAt(11).Add(time.Millisecond))
		steps := rapid.IntRange(0, 60).Draw(rt, "steps")
		for range steps {
			h.step(rt)
		}
		for _, k := range propKeys {
			h.complete(rt, k, h.high)
		}
		if len(h.inFlight) != 0 {
			rt.Fatalf("model still has %v in flight after the final drain", h.inFlight)
		}
		h.c.drains++
		c.histories++
	})
	t.Logf("rapid.seed flag %s (0: Rapid drew a fresh seed, printed only on failure); histories completed %d; "+
		"step checks %d (in flight %d, caught up %d); refusals %d; final drains %d; "+
		"ops: next %d, zero time %d, lower %d, re-observe %d, zero %d, complete %d",
		flag.Lookup("rapid.seed").Value.String(), c.histories, c.checks, c.inFlight, c.caughtUp, c.refusals,
		c.drains, c.ops["next"], c.ops["zeroTime"], c.ops["lower"], c.ops["reobserve"], c.ops["zero"],
		c.ops["complete"])
}

var propKeys = []string{"A", "B", "C"}

type propCounts struct {
	histories, checks, inFlight, caughtUp, refusals, drains int
	ops                                                     map[string]int
}

// propEntry is the model's record of one in-flight revision.
type propEntry struct {
	key string
	at  time.Time
}

// propHistory runs one history against the watermark and keeps the model of it.
type propHistory struct {
	w         *Watermark
	high      uint64    // highest accepted revision
	highAt    time.Time // commit time accepted with high
	inFlight  map[uint64]propEntry
	accepted  []uint64 // every accepted revision, in order, so draws from it replay
	lastFloor uint64
	c         *propCounts
}

func newPropHistory(c *propCounts) *propHistory {
	return &propHistory{w: New(), inFlight: map[uint64]propEntry{}, c: c}
}

// step draws one Observe or Complete operation of the history and applies it.
func (h *propHistory) step(rt *rapid.T) {
	key := rapid.SampledFrom(propKeys).Draw(rt, "key")
	op := rapid.SampledFrom([]string{"next", "next", "zeroTime", "lower", "reobserve", "zero", "complete", "complete"}).
		Draw(rt, "op")
	h.c.ops[op]++
	switch op {
	case "next", "zeroTime":
		r := h.high + rapid.Uint64Range(1, 3).Draw(rt, "gap")
		at := time.Time{}
		if op == "next" {
			at = base.Add(time.Duration(rapid.IntRange(0, 10_000).Draw(rt, "atMs")) * time.Millisecond)
		}
		h.observe(rt, r, key, at)
	case "lower":
		r := rapid.Uint64Range(1, h.high-1).Draw(rt, "lowerRev")
		h.observe(rt, r, key, commitAt(r))
	case "reobserve":
		// Under any key, and with a commit time not given before, so a time kept from the
		// earlier observation is told apart from the one the refused call gave.
		r := rapid.SampledFrom(h.accepted).Draw(rt, "reobserveRev")
		offset := time.Duration(rapid.IntRange(1, 999).Draw(rt, "offsetMs")) * time.Millisecond
		h.observe(rt, r, key, commitAt(r).Add(offset))
	case "zero":
		h.observe(rt, 0, key, commitAt(1))
	case "complete":
		r := rapid.OneOf(rapid.SampledFrom(h.accepted), rapid.Uint64Range(0, h.high+3)).Draw(rt, "completeRev")
		h.complete(rt, key, r)
	}
}

func (h *propHistory) observe(rt *rapid.T, r uint64, key string, at time.Time) {
	op := fmt.Sprintf("Observe(%d, %q, %v)", r, key, at)
	err := h.w.Observe(r, key, at)
	if r <= h.high {
		h.c.refusals++
		if err == nil {
			rt.Fatalf("%s returned no error, want a refusal (highest accepted %d)", op, h.high)
		}
	} else {
		if err != nil {
			rt.Fatalf("%s refused: %v", op, err)
		}
		h.high, h.highAt = r, at
		h.inFlight[r] = propEntry{key: key, at: at}
		h.accepted = append(h.accepted, r)
	}
	h.check(rt, op)
}

func (h *propHistory) complete(rt *rapid.T, key string, rev uint64) {
	for r, e := range h.inFlight {
		if e.key == key && r <= rev {
			delete(h.inFlight, r)
		}
	}
	h.w.Complete(key, rev)
	h.check(rt, fmt.Sprintf("Complete(%q, %d)", key, rev))
}

// check compares the watermark with the model after the operation op.
func (h *propHistory) check(rt *rapid.T, op string) {
	wantFloor, wantAt := h.high, h.highAt
	if len(h.inFlight) > 0 {
		h.c.inFlight++
		low := ^uint64(0)
		for r := range h.inFlight {
			low = min(low, r)
		}
		wantFloor, wantAt = low-1, h.inFlight[low].at
	} else {
		h.c.caughtUp++
	}
	floor, at := h.w.IndexedAt()
	if floor != wantFloor || !at.Equal(wantAt) {
		rt.Fatalf("after %s: IndexedAt = (%d, %v), want (%d, %v) (in flight: %v, highest accepted %d)",
			op, floor, at, wantFloor, wantAt, h.inFlight, h.high)
	}
	if got := h.w.Observed(); got != h.high {
		rt.Fatalf("after %s: Observed = %d, want %d", op, got, h.high)
	}
	if floor < h.lastFloor {
		rt.Fatalf("after %s: floor fell from %d to %d", op, h.lastFloor, floor)
	}
	h.lastFloor = floor
	h.c.checks++
}

// BenchmarkWatermarkInFlight measures per-call cost against the in-flight set's size: with n
// revisions in flight under keys of their own, each iteration observes one new revision and
// completes the oldest, so the set stays at n and every completion drains the lowest in-flight
// revision; the read benchmark reads with n in flight. Per-call cost that does not grow from
// 10,000 to 40,000 shows no call scans the in-flight set.
func BenchmarkWatermarkInFlight(b *testing.B) {
	for _, n := range []uint64{10_000, 40_000} {
		// keys[r%(n+1)] is distinct for any n+1 consecutive revisions.
		keys := make([]string, n+1)
		for i := range keys {
			keys[i] = fmt.Sprintf("k%d", i)
		}
		fill := func(b *testing.B) *Watermark {
			w := New()
			for r := uint64(1); r <= n; r++ {
				if err := w.Observe(r, keys[r%(n+1)], commitAt(r)); err != nil {
					b.Fatal(err)
				}
			}
			return w
		}
		b.Run(fmt.Sprintf("observe+complete/inflight=%d", n), func(b *testing.B) {
			w := fill(b)
			b.ResetTimer()
			for i := uint64(1); i <= uint64(b.N); i++ {
				r := n + i
				if err := w.Observe(r, keys[r%(n+1)], commitAt(r)); err != nil {
					b.Fatal(err)
				}
				w.Complete(keys[i%(n+1)], i)
			}
		})
		b.Run(fmt.Sprintf("indexedAt/inflight=%d", n), func(b *testing.B) {
			w := fill(b)
			b.ResetTimer()
			for range b.N {
				w.IndexedAt()
			}
		})
	}
}
