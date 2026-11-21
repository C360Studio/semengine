package damage_test

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"sync"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/internal/damage"
)

var (
	errFirst  = errors.New("first refusal")
	errSecond = errors.New("second refusal")
)

func wantEntry(t *testing.T, r *damage.Record, key string, revision uint64, refusal error) {
	t.Helper()
	got, ok := r.Get(key)
	if !ok {
		t.Fatalf("Get(%q) = not recorded, want recorded at %d", key, revision)
	}
	if got.Key != key || got.Revision != revision || got.Refusal != refusal {
		t.Fatalf("Get(%q) = %+v, want {%s %d %v}", key, got, key, revision, refusal)
	}
}

func wantAbsent(t *testing.T, r *damage.Record, key string) {
	t.Helper()
	if got, ok := r.Get(key); ok {
		t.Fatalf("Get(%q) = %+v, want not recorded", key, got)
	}
}

// spec: entity-state-follower / A damaged entity is recorded per key and cleared by its repair
// spec: graph-ingest-recovery / A proof of repair at or above the recorded revision clears it
func TestClearAboveRecordedRevisionDrops(t *testing.T) {
	var r damage.Record
	if !r.Report("a", 5, errFirst) {
		t.Fatal("first Report of a key returned false, want true")
	}
	wantEntry(t, &r, "a", 5, errFirst)
	if !r.Clear("a", 6) {
		t.Fatal("Clear(a, 6) over a report at 5 returned false, want true")
	}
	wantAbsent(t, &r, "a")
	if n := r.Len(); n != 0 {
		t.Fatalf("Len after the clear = %d, want 0", n)
	}
	if !r.Report("a", 5, errSecond) {
		t.Fatal("Report after a clear returned false, want true: the key was not recorded")
	}
}

// spec: entity-state-follower / A damaged entity is recorded per key and cleared by its repair
func TestClearAtRecordedRevisionKeeps(t *testing.T) {
	var r damage.Record
	r.Report("a", 5, errFirst)
	if r.Clear("a", 5) {
		t.Fatal("Clear(a, 5) over a report at 5 returned true, want false")
	}
	wantEntry(t, &r, "a", 5, errFirst)
	if r.Clear("b", 9) {
		t.Fatal("Clear of a key never reported returned true, want false")
	}
	if n := r.Len(); n != 1 {
		t.Fatalf("Len = %d, want 1", n)
	}
}

// spec: entity-state-follower / A damaged entity is recorded per key and cleared by its repair
func TestLowerReportIsIgnored(t *testing.T) {
	var r damage.Record
	r.Report("a", 5, errFirst)
	if r.Report("a", 4, errSecond) {
		t.Fatal("Report(a, 4) under a report at 5 returned true, want false")
	}
	wantEntry(t, &r, "a", 5, errFirst)
	// An equal or higher report replaces the entry but is not new.
	if r.Report("a", 5, errSecond) {
		t.Fatal("Report(a, 5) of a recorded key returned true, want false")
	}
	wantEntry(t, &r, "a", 5, errSecond)
	if r.Report("a", 7, errFirst) {
		t.Fatal("Report(a, 7) of a recorded key returned true, want false")
	}
	wantEntry(t, &r, "a", 7, errFirst)
}

func TestClearBelowZeroDropsNothing(t *testing.T) {
	var r damage.Record
	r.Report("a", 0, errFirst)
	if r.Clear("a", 0) {
		t.Fatal("Clear(a, 0) returned true, want false: a bound of 0 drops nothing")
	}
	wantEntry(t, &r, "a", 0, errFirst)
}

func TestSortedOrderAndLimit(t *testing.T) {
	var r damage.Record
	if got := r.Sorted(0); len(got) != 0 {
		t.Fatalf("Sorted(0) of an empty record = %v, want none", got)
	}
	for i, k := range []string{"d", "b", "e", "a", "c"} {
		r.Report(k, uint64(i+1), errFirst)
	}
	keys := func(es []damage.Entry) []string {
		out := make([]string, 0, len(es))
		for _, e := range es {
			out = append(out, e.Key)
		}
		return out
	}
	all := []string{"a", "b", "c", "d", "e"}
	for _, tc := range []struct {
		limit int
		want  []string
	}{
		{0, all}, {-1, all}, {1, all[:1]}, {3, all[:3]}, {5, all}, {6, all},
	} {
		if got := keys(r.Sorted(tc.limit)); !slices.Equal(got, tc.want) {
			t.Fatalf("Sorted(%d) = %v, want %v", tc.limit, got, tc.want)
		}
	}
	if e := r.Sorted(1)[0]; e.Revision != 4 || e.Refusal != errFirst {
		t.Fatalf("Sorted(1)[0] = %+v, want a at revision 4", e)
	}
	if n := r.Count(func(id string) bool { return id < "c" }); n != 2 {
		t.Fatalf("Count(id < c) = %d, want 2", n)
	}
	if n := r.Count(nil); n != 5 {
		t.Fatalf("Count(nil) = %d, want 5", n)
	}
}

// TestConcurrentReportsAndClears runs reports and clears of distinct keys from many
// goroutines under -race; after the join, Len and Sorted name exactly the keys whose last
// event was a report.
func TestConcurrentReportsAndClears(t *testing.T) {
	var r damage.Record
	const workers, perWorker = 8, 50
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range perWorker {
				key := fmt.Sprintf("w%d-%03d", w, i)
				r.Report(key, 1, errFirst)
				_ = r.Len()
				if i%2 == 1 {
					r.Clear(key, 2)
				}
			}
		})
	}
	wg.Wait() // every Report and Clear has returned
	var want []string
	for w := range workers {
		for i := 0; i < perWorker; i += 2 {
			want = append(want, fmt.Sprintf("w%d-%03d", w, i))
		}
	}
	slices.Sort(want)
	if n := r.Len(); n != len(want) {
		t.Fatalf("Len = %d, want %d", n, len(want))
	}
	got := make([]string, 0, len(want))
	for _, e := range r.Sorted(0) {
		got = append(got, e.Key)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Sorted keys = %v, want %v", got, want)
	}
}

// propEvent is one operation of a generated history, as the reference model keeps it.
type propEvent struct {
	clear    bool
	revision uint64 // the report's revision, or the clear's bound
	refusal  error
}

// propModel keeps every event per key and answers from the stated invariant, not from a
// running state: a key is recorded exactly when some report of it is at or above every later
// clear's bound. Its entry is the surviving report with the highest revision, the latest of
// those at that revision.
type propModel map[string][]propEvent

func (m propModel) entry(key string) (damage.Entry, bool) {
	events := m[key]
	var best damage.Entry
	found := false
	for i, ev := range events {
		if ev.clear {
			continue
		}
		survives := true
		for _, later := range events[i+1:] {
			if later.clear && ev.revision < later.revision {
				survives = false
				break
			}
		}
		if survives && (!found || ev.revision >= best.Revision) {
			best = damage.Entry{Key: key, Revision: ev.revision, Refusal: ev.refusal}
			found = true
		}
	}
	return best, found
}

func (m propModel) recorded() []string {
	var keys []string
	for k := range m {
		if _, ok := m.entry(k); ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

var propKeys = []string{"k3", "k1", "k4", "k2"}

type propCounts struct {
	histories, reports, newReports, clears, drops, sortedChecks int
}

type propHistory struct {
	r      damage.Record
	model  propModel
	c      *propCounts
	nextID int
}

func (h *propHistory) report(rt *rapid.T, key string, revision uint64) {
	_, before := h.model.entry(key)
	h.nextID++
	refusal := fmt.Errorf("refusal %d", h.nextID)
	h.model[key] = append(h.model[key], propEvent{revision: revision, refusal: refusal})
	got := h.r.Report(key, revision, refusal)
	if got != !before {
		rt.Fatalf("Report(%s, %d) = %v, want %v (recorded before: %v)", key, revision, got, !before, before)
	}
	h.c.reports++
	if got {
		h.c.newReports++
	}
	h.check(rt)
}

func (h *propHistory) clear(rt *rapid.T, key string, below uint64) {
	_, before := h.model.entry(key)
	h.model[key] = append(h.model[key], propEvent{clear: true, revision: below})
	_, after := h.model.entry(key)
	want := before && !after
	if got := h.r.Clear(key, below); got != want {
		rt.Fatalf("Clear(%s, %d) = %v, want %v", key, below, got, want)
	}
	h.c.clears++
	if want {
		h.c.drops++
	}
	h.check(rt)
}

// check compares every read of the record with the model after each event.
func (h *propHistory) check(rt *rapid.T) {
	recorded := h.model.recorded()
	if n := h.r.Len(); n != len(recorded) {
		rt.Fatalf("Len = %d, want %d (%v)", n, len(recorded), recorded)
	}
	if n := h.r.Count(nil); n != len(recorded) {
		rt.Fatalf("Count(nil) = %d, want %d", n, len(recorded))
	}
	odd := 0
	for _, k := range recorded {
		if k == "k1" || k == "k3" {
			odd++
		}
	}
	if n := h.r.Count(func(id string) bool { return id == "k1" || id == "k3" }); n != odd {
		rt.Fatalf("Count(k1|k3) = %d, want %d", n, odd)
	}
	for _, k := range propKeys {
		want, wantOK := h.model.entry(k)
		got, ok := h.r.Get(k)
		if ok != wantOK || got != want {
			rt.Fatalf("Get(%s) = (%+v, %v), want (%+v, %v)", k, got, ok, want, wantOK)
		}
	}
	limit := rapid.IntRange(-1, len(propKeys)+1).Draw(rt, "limit")
	wantN := len(recorded)
	if limit > 0 && limit < wantN {
		wantN = limit
	}
	sorted := h.r.Sorted(limit)
	if len(sorted) != wantN {
		rt.Fatalf("Sorted(%d) has %d entries, want %d", limit, len(sorted), wantN)
	}
	for i, e := range sorted {
		want, _ := h.model.entry(recorded[i])
		if e != want {
			rt.Fatalf("Sorted(%d)[%d] = %+v, want %+v", limit, i, e, want)
		}
	}
	h.c.sortedChecks++
}

// TestPropRecordFollowsLatestEvent generates sequential histories of reports and clears over
// four keys and compares every read after every event with a reference model that keeps the
// whole history and applies the invariant directly: a key is recorded exactly when some report
// of it is at or above every later clear's bound; Len and Count equal the recorded keys;
// Sorted(n) is the first n of them in key order; Report returns true only for a key not
// recorded before; Clear returns whether it dropped an entry.
//
// Revisions and bounds are drawn from 0 to 6, so equal revisions, a bound of 0, and a
// clear exactly at a recorded revision are reached often. Every history starts with a fixed
// prefix that runs each boundary the rule names: a new report, a clear at the recorded
// revision (kept), a bound of 0 (kept), a lower report (ignored, false), a clear above (drops).
// The model is sequential; TestConcurrentReportsAndClears covers concurrent use under -race.
//
// spec: entity-state-follower / A damaged entity is recorded per key and cleared by its repair
// spec: graph-ingest-recovery / A proof of repair at or above the recorded revision clears it
func TestPropRecordFollowsLatestEvent(t *testing.T) {
	var c propCounts
	rapid.Check(t, func(rt *rapid.T) {
		h := &propHistory{model: propModel{}, c: &c}
		h.report(rt, "k2", 3)
		h.clear(rt, "k2", 3)
		h.clear(rt, "k2", 0)
		h.report(rt, "k2", 2)
		h.clear(rt, "k2", 4)
		steps := rapid.IntRange(0, 40).Draw(rt, "steps")
		for range steps {
			key := rapid.SampledFrom(propKeys).Draw(rt, "key")
			revision := rapid.Uint64Range(0, 6).Draw(rt, "revision")
			if rapid.Bool().Draw(rt, "clear") {
				h.clear(rt, key, revision)
			} else {
				h.report(rt, key, revision)
			}
		}
		c.histories++
	})
	t.Logf("rapid.seed flag %s (0: Rapid drew a fresh seed, printed only on failure); histories completed %d; "+
		"reports %d (new %d); clears %d (dropped %d); read checks %d",
		flag.Lookup("rapid.seed").Value.String(), c.histories, c.reports, c.newReports, c.clears, c.drops,
		c.sortedChecks)
}
