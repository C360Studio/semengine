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

// The thirteen tests from here to TestWatermark_Concurrent are SemStreams'
// pkg/revlag/watermark_test.go at the pin 8b99efe9c66a4faa4fa509f9f62cc6bad8392128, unchanged
// except that each read of the dropped Indexed() takes the floor from IndexedAt() (design D7)
// and each carries the spec requirement it proves. Their messages still say Indexed.

// The watermark's whole reason to exist is honesty under SPARSE, out-of-order,
// multi-path completion. These tests feed sparse ascending revision streams (e.g.
// {50, 72, 100}) — NOT dense 1..N — because History=1 + DeliverLastPerSubject never
// delivers a dense set, and the rejected "advance-from-Indexed+1" algorithm would
// stall on the very first gap.

// base is a fixed origin for synthetic KV commit timestamps: every test timestamp is
// base.Add(rev * time.Second), so a commit time is trivially attributable to the
// revision that carried it and no test depends on the wall clock.
var base = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

// commitAt is the synthetic commit time for revision r (ascending with revision, as
// real KV commit times are).
func commitAt(r uint64) time.Time { return base.Add(time.Duration(r) * time.Second) }

// spec: revision-watermark / The floor is the low-water mark of in-flight revisions
// spec: revision-watermark / IndexedAt pairs the floor with a covered commit time
func TestWatermark_ColdStart(t *testing.T) {
	w := New()
	if got, _ := w.IndexedAt(); got != 0 {
		t.Fatalf("cold start Indexed = %d, want 0", got)
	}
	// Nothing observed → no covered revision → the commit time is ZERO, which the
	// contract says callers must read as "staleness not computable", never as
	// "zero staleness" (a fabricated 0 would read as a perfectly fresh view).
	indexed, at := w.IndexedAt()
	if indexed != 0 || !at.IsZero() {
		t.Fatalf("cold start IndexedAt = (%d, %v), want (0, zero time)", indexed, at)
	}
}

// spec: revision-watermark / The floor is the low-water mark of in-flight revisions
// spec: revision-watermark / IndexedAt pairs the floor with a covered commit time
func TestWatermark_SparseMidBuild_LowWaterOfPending(t *testing.T) {
	w := New()
	// Sparse ascending delivery: distinct keys at purged-gap revisions.
	w.Observe(50, "A", commitAt(50))
	w.Observe(72, "B", commitAt(72))
	w.Observe(100, "C", commitAt(100))

	// Nothing completed: Indexed is minPending-1, NOT stuck at 0 waiting for rev 1.
	if got, _ := w.IndexedAt(); got != 49 {
		t.Fatalf("all pending: Indexed = %d, want 49 (minPending-1)", got)
	}
	// Floor 49 covers no DELIVERED revision (50 is the lowest ever delivered), so the
	// covered commit time is still unknown.
	if _, at := w.IndexedAt(); !at.IsZero() {
		t.Fatalf("floor below every delivered revision: commit time = %v, want zero", at)
	}

	// Out-of-order completion: a fast LATE worker (rev 72) cannot report the gap done.
	w.Complete("B", 72)
	if got, _ := w.IndexedAt(); got != 49 {
		t.Fatalf("after completing 72 (50 still pending): Indexed = %d, want 49", got)
	}

	// Complete the low revision: watermark advances past it to the next gap.
	w.Complete("A", 50)
	if got, _ := w.IndexedAt(); got != 99 {
		t.Fatalf("after completing 50 (100 still pending): Indexed = %d, want 99", got)
	}
	// Documented conservatism: 72 completed while it sat ABOVE the then-floor (49),
	// so its timestamp was dropped rather than retained. The covered time is 50's —
	// OLDER than the true newest-covered (72), i.e. staleness over-reports, never
	// under-reports. Retaining it would mean a map that grows with completions.
	if _, at := w.IndexedAt(); !at.Equal(commitAt(50)) {
		t.Fatalf("covered commit time = %v, want %v (conservative: 72's was dropped)", at, commitAt(50))
	}

	// Complete the last: pending empties → Indexed jumps to observedHigh (=100),
	// honestly skipping the purged 51..71, 73..99 that were never delivered.
	w.Complete("C", 100)
	if got, _ := w.IndexedAt(); got != 100 {
		t.Fatalf("caught up: Indexed = %d, want 100 (observedHigh)", got)
	}
	// Caught up is the EXACT case: the floor is observedHigh, which was itself
	// observed, so its own commit time is the answer.
	indexed, at := w.IndexedAt()
	if indexed != 100 || !at.Equal(commitAt(100)) {
		t.Fatalf("caught up IndexedAt = (%d, %v), want (100, %v)", indexed, at, commitAt(100))
	}
}

// spec: revision-watermark / Completion is key-scoped and drains lower revisions
// spec: revision-watermark / IndexedAt pairs the floor with a covered commit time
func TestWatermark_CoalescerCollapse_KeyScopedDrainsLowerRevisions(t *testing.T) {
	w := New()
	// One key delivered three times (the coalescer will re-Get only the latest, 30).
	w.Observe(10, "K", commitAt(10))
	w.Observe(20, "K", commitAt(20))
	w.Observe(30, "K", commitAt(30))
	if got, _ := w.IndexedAt(); got != 9 {
		t.Fatalf("three pending revs of K: Indexed = %d, want 9", got)
	}

	// The single completion at the re-Got latest revision drains ALL collapsed lower
	// revisions of K that no worker ever sees individually. Exact-revision completion
	// would strand 10 and 20 forever.
	w.Complete("K", 30)
	if got, _ := w.IndexedAt(); got != 30 {
		t.Fatalf("after key-scoped Complete(K,30): Indexed = %d, want 30", got)
	}
	// A collapsed multi-revision drain must carry the NEWEST collapsed revision's
	// commit time — the view really does reflect the world as of rev 30, and taking
	// 10's time here would over-report staleness by the whole coalesce window.
	if _, at := w.IndexedAt(); !at.Equal(commitAt(30)) {
		t.Fatalf("coalesced drain commit time = %v, want %v", at, commitAt(30))
	}
}

// TestWatermark_CoalescerCollapse_MidBuildTakesNewestCovered pins the collapsed-drain
// timestamp while the pipeline is NOT caught up (a higher key is still pending), so
// the answer comes from the drained set rather than from the pending-empty shortcut.
//
// spec: revision-watermark / IndexedAt pairs the floor with a covered commit time
func TestWatermark_CoalescerCollapse_MidBuildTakesNewestCovered(t *testing.T) {
	w := New()
	w.Observe(10, "K", commitAt(10))
	w.Observe(20, "K", commitAt(20))
	w.Observe(30, "K", commitAt(30))
	w.Observe(40, "J", commitAt(40)) // still in flight; pins the floor at 39

	w.Complete("K", 30)
	if got, _ := w.IndexedAt(); got != 39 {
		t.Fatalf("Indexed = %d, want 39 (J@40 pending)", got)
	}
	if _, at := w.IndexedAt(); !at.Equal(commitAt(30)) {
		t.Fatalf("commit time = %v, want %v (newest drained revision <= floor)", at, commitAt(30))
	}
}

// spec: revision-watermark / Completion is key-scoped and drains lower revisions
func TestWatermark_DeleteSupersedesPendingUpdate(t *testing.T) {
	w := New()
	// Update K@10 is delivered and in flight...
	w.Observe(10, "K", commitAt(10))
	// ...then a delete K@11 arrives. The delete's key-scoped completion drains the
	// tombstone AND the earlier still-pending update it supersedes.
	w.Observe(11, "K", commitAt(11))
	w.Complete("K", 11)
	if got, _ := w.IndexedAt(); got != 11 {
		t.Fatalf("after delete supersedes pending update: Indexed = %d, want 11", got)
	}
	if _, at := w.IndexedAt(); !at.Equal(commitAt(11)) {
		t.Fatalf("commit time = %v, want %v (the tombstone's own commit)", at, commitAt(11))
	}
}

// spec: revision-watermark / Completion is key-scoped and drains lower revisions
// spec: revision-watermark / IndexedAt pairs the floor with a covered commit time
func TestWatermark_CompletionIsKeyScoped(t *testing.T) {
	w := New()
	w.Observe(10, "A", commitAt(10))
	w.Observe(11, "B", commitAt(11)) // a delete of a DIFFERENT key
	// Completing B must NOT drain A's pending revision even though 10 <= 11.
	w.Complete("B", 11)
	if got, _ := w.IndexedAt(); got != 9 {
		t.Fatalf("key-scoped: completing B left A@10 pending, Indexed = %d, want 9", got)
	}
	w.Complete("A", 10)
	if got, _ := w.IndexedAt(); got != 11 {
		t.Fatalf("after A completes too: Indexed = %d, want 11 (observedHigh)", got)
	}
	if _, at := w.IndexedAt(); !at.Equal(commitAt(11)) {
		t.Fatalf("commit time = %v, want %v (caught up → observedHigh's commit)", at, commitAt(11))
	}
}

// spec: revision-watermark / The floor is the low-water mark of in-flight revisions
func TestWatermark_TrailingDeleteOnlyTail(t *testing.T) {
	w := New()
	w.Observe(10, "A", commitAt(10))
	w.Complete("A", 10)
	if got, _ := w.IndexedAt(); got != 10 {
		t.Fatalf("after A: Indexed = %d, want 10", got)
	}
	// A tail of pure deletes must still advance the watermark — their sequences
	// participate, or lag wedges forever.
	w.Observe(11, "B", commitAt(11))
	w.Complete("B", 11)
	if got, _ := w.IndexedAt(); got != 11 {
		t.Fatalf("trailing delete: Indexed = %d, want 11", got)
	}
	if _, at := w.IndexedAt(); !at.Equal(commitAt(11)) {
		t.Fatalf("commit time = %v, want %v", at, commitAt(11))
	}
}

// TestWatermark_ObserveDoesNotRegressCoveredTime pins the seam between a caught-up
// view and the next delivery: observing a NEW revision moves the floor from
// observedHigh down to r-1, which still covers the previously-covered revision, so
// the covered commit time must stay put (it must not reset to zero and read as
// "staleness not computable" on every single write).
//
// spec: revision-watermark / The floor never falls under ascending observation
// spec: revision-watermark / IndexedAt pairs the floor with a covered commit time
func TestWatermark_ObserveDoesNotRegressCoveredTime(t *testing.T) {
	w := New()
	w.Observe(10, "A", commitAt(10))
	w.Complete("A", 10)

	w.Observe(50, "B", commitAt(50)) // in flight; floor drops to 49
	indexed, at := w.IndexedAt()
	if indexed != 49 {
		t.Fatalf("Indexed = %d, want 49", indexed)
	}
	if !at.Equal(commitAt(10)) {
		t.Fatalf("commit time = %v, want %v (rev 10 is still covered by floor 49)", at, commitAt(10))
	}
}

// spec: revision-watermark / Observed is the highest revision delivered
func TestWatermark_RevisionZeroIgnored(t *testing.T) {
	w := New()
	w.Observe(0, "X", commitAt(1)) // "no revision" — stream sequences start at 1
	if got, _ := w.IndexedAt(); got != 0 {
		t.Fatalf("Observe(0) should be a no-op, Indexed = %d, want 0", got)
	}
	if _, at := w.IndexedAt(); !at.IsZero() {
		t.Fatalf("Observe(0) must not record a commit time, got %v", at)
	}
	w.Observe(5, "A", commitAt(5))
	if got, _ := w.IndexedAt(); got != 4 {
		t.Fatalf("after Observe(5): Indexed = %d, want 4", got)
	}
}

// TestWatermark_ZeroCommitTimeStaysUnknown covers the legacy/no-timestamp path: a
// caller that cannot supply a commit time leaves the covered time zero rather than
// fabricating one, so downstream reports "not computable" instead of "0 ms stale".
//
// spec: revision-watermark / IndexedAt pairs the floor with a covered commit time
func TestWatermark_ZeroCommitTimeStaysUnknown(t *testing.T) {
	w := New()
	w.Observe(7, "A", time.Time{})
	w.Complete("A", 7)
	indexed, at := w.IndexedAt()
	if indexed != 7 {
		t.Fatalf("Indexed = %d, want 7", indexed)
	}
	if !at.IsZero() {
		t.Fatalf("commit time = %v, want zero (unknown, not fabricated)", at)
	}
}

// spec: revision-watermark / Completion is key-scoped and drains lower revisions
func TestWatermark_CompleteUnobservedRevisionDrainsLower(t *testing.T) {
	// A coalescer re-Get can return a revision NEWER than any yet delivered to the
	// watch (a write between snapshot and Get). Completing at that not-yet-observed
	// revision must still drain the key's lower pending, and must NOT advance
	// observedHigh (only Observe does that).
	w := New()
	w.Observe(10, "K", commitAt(10))
	w.Complete("K", 40) // 40 never observed
	if got, _ := w.IndexedAt(); got != 10 {
		t.Fatalf("Indexed = %d, want 10 (observedHigh unchanged; 10 drained)", got)
	}
	// The commit time is 10's — never 40's, whose entry was never delivered and whose
	// commit time the watermark has never seen.
	if _, at := w.IndexedAt(); !at.Equal(commitAt(10)) {
		t.Fatalf("commit time = %v, want %v", at, commitAt(10))
	}
}

// TestWatermark_MemoryDoesNotGrowWithCompletions is the structural guard behind the
// "carry forward one scalar, not a map of completed revisions" decision: after N
// observe/complete cycles the ONLY per-revision map must be empty, so retained
// memory tracks in-flight work rather than cumulative throughput. It asserts on the
// internal map directly (same package) because that is the thing that would grow.
//
// spec: revision-watermark / Retained state tracks in-flight work
func TestWatermark_MemoryDoesNotGrowWithCompletions(t *testing.T) {
	w := New()
	const n = 20000
	for i := uint64(1); i <= n; i++ {
		w.Observe(i, "K", commitAt(i))
		w.Complete("K", i)
	}
	w.mu.Lock()
	pendingLen := len(w.pending)
	w.mu.Unlock()
	if pendingLen != 0 {
		t.Fatalf("pending retained %d entries after draining every revision, want 0", pendingLen)
	}
	indexed, at := w.IndexedAt()
	if indexed != n || !at.Equal(commitAt(n)) {
		t.Fatalf("IndexedAt = (%d, %v), want (%d, %v)", indexed, at, n, commitAt(n))
	}

	// Same guard with a slow key pinning the floor: the completions that pile up
	// ABOVE it must not be retained either (that is the case a "completed revisions"
	// map would grow without bound).
	w2 := New()
	w2.Observe(1, "SLOW", commitAt(1))
	for i := uint64(2); i <= n; i++ {
		w2.Observe(i, "K", commitAt(i))
		w2.Complete("K", i)
	}
	w2.mu.Lock()
	pendingLen = len(w2.pending)
	w2.mu.Unlock()
	if pendingLen != 1 {
		t.Fatalf("pending retained %d entries with one key stuck, want 1 (the stuck key only)", pendingLen)
	}
	if got, _ := w2.IndexedAt(); got != 0 {
		t.Fatalf("Indexed = %d, want 0 (SLOW@1 pins the floor)", got)
	}
}

// TestWatermark_Concurrent runs Observe/Complete across many goroutines to prove the
// mutex holds and that draining every observed revision leaves Indexed at
// observedHigh (run under -race). Mirrors the real wiring: the watch goroutine
// Observes ascending; N workers Complete out of order.
//
// spec: revision-watermark / The Watermark is safe for concurrent use
func TestWatermark_Concurrent(t *testing.T) {
	w := New()
	const n = 2000

	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = string(rune('a'+i%26)) + "-" + time.Duration(i).String()
		w.Observe(uint64(i+1), keys[i], commitAt(uint64(i+1)))
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

	indexed, at := w.IndexedAt()
	if indexed != uint64(n) {
		t.Fatalf("after draining all: Indexed = %d, want %d", indexed, n)
	}
	// Fully drained → the exact caught-up answer, regardless of completion order.
	if !at.Equal(commitAt(n)) {
		t.Fatalf("after draining all: commit time = %v, want %v", at, commitAt(n))
	}
}

// The tests below are new in SemEngine (design D7 and D8 of the OpenSpec change
// setup-04a-04-graph-roots-revlag); the ones above are the SemStreams pin's.

// TestWatermark_ObservedIsHighestDelivered pins Observed apart from the floor: it is the
// highest revision delivered whether or not that revision has completed, and a completion
// at a revision never delivered does not move it.
//
// spec: revision-watermark / Observed is the highest revision delivered
func TestWatermark_ObservedIsHighestDelivered(t *testing.T) {
	w := New()
	if got := w.Observed(); got != 0 {
		t.Fatalf("new watermark: Observed = %d, want 0", got)
	}
	w.Observe(50, "A", commitAt(50))
	w.Observe(72, "B", commitAt(72))
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

// TestWatermark_DrainAtFloorIsCovered is the boundary the pin's tests miss: the drained
// revision 10 sits exactly at the new floor (11 is still in flight), so the floor covers it
// and IndexedAt carries its commit time rather than the zero time.
//
// spec: revision-watermark / IndexedAt pairs the floor with a covered commit time
func TestWatermark_DrainAtFloorIsCovered(t *testing.T) {
	w := New()
	w.Observe(10, "A", commitAt(10))
	w.Observe(11, "B", commitAt(11))
	w.Complete("A", 10)
	floor, at := w.IndexedAt()
	if floor != 10 || !at.Equal(commitAt(10)) {
		t.Fatalf("IndexedAt = (%d, %v), want (10, %v): a drain at the floor is covered", floor, at, commitAt(10))
	}
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

	w.Observe(10, "A", commitAt(10))
	w.Observe(20, "B", commitAt(20))
	w.Complete("A", 10)
	// B@20 holds the floor at 19 and the covered time at 10's. Neither A nor Z has
	// anything in flight, below or above the floor.
	if floor, at := w.IndexedAt(); floor != 19 || !at.Equal(commitAt(10)) {
		t.Fatalf("setup: IndexedAt = (%d, %v), want (19, %v)", floor, at, commitAt(10))
	}
	assertUnchanged(t, w, "A", 30)
	assertUnchanged(t, w, "Z", 5)
	assertUnchanged(t, w, "B", 19) // B is in flight only above 19

	w.Complete("B", 20)
	assertUnchanged(t, w, "B", 20)
	assertUnchanged(t, w, "A", 1000)
}

// TestWatermark_ConcurrentReadersAndWriters is the real wiring: one goroutine observes
// revisions in ascending order and hands each to a pool of workers that complete it, while a
// reader calls IndexedAt and Observed throughout. Keys repeat, so one completion can drain a
// lower revision of its key that another worker is about to complete. Run under -race.
//
// spec: revision-watermark / The Watermark is safe for concurrent use
// spec: revision-watermark / The floor never falls under ascending observation
func TestWatermark_ConcurrentReadersAndWriters(t *testing.T) {
	const n = 2000
	const workers = 8
	key := func(r uint64) string { return fmt.Sprintf("k%d", r%16) }
	w := New()

	revs := make(chan uint64)
	var writers sync.WaitGroup
	for range workers {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for r := range revs {
				w.Complete(key(r), r)
			}
		}()
	}
	writers.Add(1)
	go func() {
		defer writers.Done()
		defer close(revs)
		for r := uint64(1); r <= n; r++ {
			w.Observe(r, key(r), commitAt(r))
			revs <- r
		}
	}()

	stop := make(chan struct{})
	var reader sync.WaitGroup
	var fell []string // written only by the reader goroutine
	reads := 0
	reader.Add(1)
	go func() {
		defer reader.Done()
		var last uint64
		for {
			floor, _ := w.IndexedAt()
			_ = w.Observed()
			reads++
			if floor < last {
				fell = append(fell, fmt.Sprintf("%d after %d", floor, last))
			}
			last = floor
			select {
			case <-stop:
				return
			default:
			}
		}
	}()

	writers.Wait()
	close(stop)
	// reader.Wait orders the reads of fell and reads below after the reader's last write.
	reader.Wait()
	if len(fell) > 0 {
		t.Fatalf("the floor fell %d times in %d reads under ascending observation: %v", len(fell), reads, fell)
	}
	// writers.Wait above ordered this read after every Observe and Complete.
	floor, at := w.IndexedAt()
	if floor != n || !at.Equal(commitAt(n)) {
		t.Fatalf("after all writers returned: IndexedAt = (%d, %v), want (%d, %v)", floor, at, n, commitAt(n))
	}
	if got := w.Observed(); got != n {
		t.Fatalf("Observed = %d, want %d", got, n)
	}
}

// TestWatermark_RetentionTracksInFlight checks retention without reading any field of the
// watermark: with one revision stuck in flight, a million others observed and completed above
// it must not grow the live heap. One retained 8-byte entry per completion would grow it by
// about 8 MB; the bound is 1 MiB (PR #407 probe, issuecomment-6388227813: the pin's code grows
// it by about 0 bytes, 1,864 under -race).
//
// spec: revision-watermark / Retained state tracks in-flight work
func TestWatermark_RetentionTracksInFlight(t *testing.T) {
	const n = 1_000_000
	const bound = 1 << 20
	w := New()
	w.Observe(1, "STUCK", commitAt(1))

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for r := uint64(2); r <= n+1; r++ {
		w.Observe(r, "K", commitAt(r))
		w.Complete("K", r)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)

	// These reads also keep w reachable past the second measurement.
	if floor, at := w.IndexedAt(); floor != 0 || !at.IsZero() {
		t.Fatalf("IndexedAt = (%d, %v), want (0, zero time): STUCK@1 pins the floor", floor, at)
	}
	if got := w.Observed(); got != n+1 {
		t.Fatalf("Observed = %d, want %d", got, n+1)
	}
	grew := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("live heap grew by %d bytes over %d completions with one revision in flight", grew, n)
	if grew >= bound {
		t.Fatalf("live heap grew by %d bytes over %d completions, want under %d: completed revisions are retained",
			grew, n, bound)
	}
}

// TestPropWatermarkHistory is the generated check of design D8: a history of Observe and
// Complete calls goes to the port and to the pin's code (pinref_test.go) alike, and after every
// step their IndexedAt (floor and time) and Observed must be equal. Two checks come from the
// spec, not from either implementation, through a record the test keeps of what is in flight:
//
//   - the floor is the highest observed revision when nothing is in flight, else the lowest
//     in-flight revision minus 1 ("The floor is the low-water mark of in-flight revisions";
//     the record drains as "Completion is key-scoped and drains lower revisions" says);
//   - while the history has only observed revisions higher than any before, the floor never
//     falls ("The floor never falls under ascending observation").
//
// Each history starts with the fixed prefix Observe(10, A), Observe(11, B), Complete(A, 10), so
// the drain exactly at the floor runs in every case, and ends by completing every key at the
// highest revision, so the caught-up path runs in every case: both must then report nothing in
// flight (floor equals Observed) with equal times. The model is sequential; the two concurrent
// tests cover concurrency under -race.
//
// spec: revision-watermark / The floor is the low-water mark of in-flight revisions
// spec: revision-watermark / Completion is key-scoped and drains lower revisions
// spec: revision-watermark / The floor never falls under ascending observation
func TestPropWatermarkHistory(t *testing.T) {
	var c propCounts
	rapid.Check(t, func(rt *rapid.T) {
		h := newPropHistory(&c)
		h.observe(rt, 10, "A", commitAt(10))
		h.observe(rt, 11, "B", commitAt(11))
		h.complete(rt, "A", 10)
		steps := rapid.IntRange(0, 60).Draw(rt, "steps")
		for range steps {
			h.step(rt)
		}
		for _, k := range propKeys {
			h.complete(rt, k, h.high)
		}
		h.checkCaughtUp(rt)
		c.histories++
		if !h.ascending {
			c.nonAscending++
		}
	})
	t.Logf("rapid.seed flag %s (0: Rapid drew a fresh seed, printed only on failure); histories completed %d "+
		"(%d not ascending); step checks %d (in flight %d, caught up %d); floor-never-falls checks %d; "+
		"final drains %d; observe next %d, observe zero time %d, observe lower %d, re-observe %d, complete %d",
		flag.Lookup("rapid.seed").Value.String(), c.histories, c.nonAscending, c.checks, c.inFlight,
		c.caughtUp, c.monotonic, c.drains, c.ops["next"], c.ops["zeroTime"], c.ops["lower"],
		c.ops["reobserve"], c.ops["complete"])
}

var propKeys = []string{"A", "B", "C"}

type propCounts struct {
	histories, nonAscending, checks, inFlight, caughtUp, monotonic, drains int
	ops                                                                    map[string]int
}

// propHistory runs one history against the port and the pin's code, and keeps the test's own
// record of it: the highest observed revision, each in-flight revision's key, and every
// revision observed (in first-observed order, so draws from it replay).
type propHistory struct {
	port      *Watermark
	ref       *pinWatermark
	high      uint64
	inFlight  map[uint64]string
	observed  []uint64
	seen      map[uint64]bool
	ascending bool
	lastFloor uint64
	c         *propCounts
}

func newPropHistory(c *propCounts) *propHistory {
	if c.ops == nil {
		c.ops = map[string]int{}
	}
	return &propHistory{
		port: New(), ref: newPinWatermark(), inFlight: map[uint64]string{}, seen: map[uint64]bool{},
		ascending: true, c: c,
	}
}

// step draws one operation of design D8 and applies it.
func (h *propHistory) step(rt *rapid.T) {
	key := rapid.SampledFrom(propKeys).Draw(rt, "key")
	op := rapid.SampledFrom([]string{"next", "next", "zeroTime", "lower", "reobserve", "complete", "complete"}).
		Draw(rt, "op")
	h.c.ops[op]++
	switch op {
	case "next", "zeroTime":
		r := h.high + rapid.Uint64Range(1, 3).Draw(rt, "gap")
		at := commitAt(r)
		if op == "zeroTime" {
			at = time.Time{}
		}
		h.observe(rt, r, key, at)
	case "lower":
		r := rapid.Uint64Range(1, h.high-1).Draw(rt, "lowerRev")
		h.observe(rt, r, key, commitAt(r))
	case "reobserve":
		// Under any key, and with a commit time not given before, so a time kept from the
		// earlier observation is told apart from the one last given.
		r := rapid.SampledFrom(h.observed).Draw(rt, "reobserveRev")
		offset := time.Duration(rapid.IntRange(1, 999).Draw(rt, "offsetMs")) * time.Millisecond
		h.observe(rt, r, key, commitAt(r).Add(offset))
	case "complete":
		r := rapid.OneOf(rapid.SampledFrom(h.observed), rapid.Uint64Range(0, h.high+3)).Draw(rt, "completeRev")
		h.complete(rt, key, r)
	}
}

func (h *propHistory) observe(rt *rapid.T, r uint64, key string, at time.Time) {
	if r <= h.high {
		h.ascending = false
	} else {
		h.high = r
	}
	if !h.seen[r] {
		h.seen[r] = true
		h.observed = append(h.observed, r)
	}
	h.inFlight[r] = key
	h.port.Observe(r, key, at)
	h.ref.Observe(r, key, at)
	h.check(rt, fmt.Sprintf("Observe(%d, %q, %v)", r, key, at))
}

func (h *propHistory) complete(rt *rapid.T, key string, rev uint64) {
	for r, k := range h.inFlight {
		if k == key && r <= rev {
			delete(h.inFlight, r)
		}
	}
	h.port.Complete(key, rev)
	h.ref.Complete(key, rev)
	h.check(rt, fmt.Sprintf("Complete(%q, %d)", key, rev))
}

// check compares the port with the pin's code and with the record after the operation op.
func (h *propHistory) check(rt *rapid.T, op string) {
	floor, at := h.port.IndexedAt()
	refFloor, refAt := h.ref.IndexedAt()
	if floor != refFloor || !at.Equal(refAt) {
		rt.Fatalf("after %s: port IndexedAt = (%d, %v), pin's code = (%d, %v)", op, floor, at, refFloor, refAt)
	}
	if got, want := h.port.Observed(), h.ref.Observed(); got != want {
		rt.Fatalf("after %s: port Observed = %d, pin's code = %d", op, got, want)
	}
	want := h.high
	if len(h.inFlight) > 0 {
		h.c.inFlight++
		low := ^uint64(0)
		for r := range h.inFlight {
			low = min(low, r)
		}
		want = low - 1
	} else {
		h.c.caughtUp++
	}
	if floor != want {
		rt.Fatalf("after %s: floor = %d, want %d (in flight: %v, highest observed %d)", op, floor, want, h.inFlight, h.high)
	}
	if h.ascending {
		h.c.monotonic++
		if floor < h.lastFloor {
			rt.Fatalf("after %s: floor fell from %d to %d under ascending observation", op, h.lastFloor, floor)
		}
	}
	h.lastFloor = floor
	h.c.checks++
}

// checkCaughtUp runs after every key has completed at the highest revision.
func (h *propHistory) checkCaughtUp(rt *rapid.T) {
	if len(h.inFlight) != 0 {
		rt.Fatalf("record still has %v in flight after the final drain", h.inFlight)
	}
	floor, at := h.port.IndexedAt()
	refFloor, refAt := h.ref.IndexedAt()
	if floor != h.port.Observed() || refFloor != h.ref.Observed() {
		rt.Fatalf("after the final drain: floors (%d, pin %d) differ from Observed (%d, pin %d)",
			floor, refFloor, h.port.Observed(), h.ref.Observed())
	}
	if !at.Equal(refAt) {
		rt.Fatalf("after the final drain: port time %v, pin's code %v", at, refAt)
	}
	h.c.drains++
}
