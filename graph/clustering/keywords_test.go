package clustering

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// TestKeywordsScoredWhenWritten: a group's keywords are its words scored by the share of
// its members holding them times ln(graph members with words / graph members holding the
// word), ties lexical, scored when the record is written. The expected lists are worked by
// hand from that rule in each step's comment.
func TestKeywordsScoredWhenWritten(t *testing.T) {
	t.Run("common word, lone holder, untouched group", func(t *testing.T) {
		m, kv := newTestMaintainer(t)
		a0, a1, a2 := "acme.dep1.s1.docs.note.a0", "acme.dep1.s1.docs.note.a1", "acme.dep1.s1.docs.note.a2"
		b0, b1, b2 := "acme.dep1.s2.docs.note.b0", "acme.dep1.s2.docs.note.b1", "acme.dep1.s2.docs.note.b2"
		mustUpsert(t, m, a0, 1, "common special shared lone")
		mustUpsert(t, m, a1, 1, "common special shared")
		mustUpsert(t, m, a2, 1, "common special")
		mustUpsert(t, m, b0, 1, "common other shared")
		mustUpsert(t, m, b1, 1, "common other")
		mustUpsert(t, m, b2, 1, "common other")
		m.BootstrapDone()
		mustTick(t, m)
		kv.takeOps()

		// Six members with words. s1: special 3/3*ln(6/3) = 0.69, shared 2/3*ln(6/3) = 0.46;
		// common 3/3*ln(6/6) = 0 says nothing; lone has one holder. s2: other 0.69; shared
		// has one holder there.
		s1, s2 := "0.acme.dep1.s1.docs", "0.acme.dep1.s2.docs"
		wantKeywords(t, kv, s1, "special", "shared")
		wantKeywords(t, kv, s2, "other")

		// A member outside s1 raises special to four holders of seven (common stays held by
		// every member). Scored now, s1 would be
		// shared 2/3*ln(7/3) = 0.565 before special ln(7/4) = 0.560; s1 is not touched, so the
		// tick writes only b3's groups and s1 keeps the keywords it was written with.
		b3 := "acme.dep1.s2.docs.note.b3"
		mustUpsert(t, m, b3, 1, "common special other")
		mustTick(t, m)
		ops := kv.takeOps()
		slices.Sort(ops)
		if want := puts(groupKeys(b3)...); !slices.Equal(ops, want) {
			t.Fatalf("operations = %q, want %q", ops, want)
		}
		wantKeywords(t, kv, s1, "special", "shared")

		// Touching s1 writes it again, scored against the graph as it is now.
		mustUpsert(t, m, a2, 2, "common special")
		mustTick(t, m)
		wantKeywords(t, kv, s1, "shared", "special")
	})

	t.Run("only a member's most frequent words are held", func(t *testing.T) {
		m, kv := newTestMaintainer(t)
		// Each of c0 and c1 holds w00..w31 twice and aaa once, so aaa is outside both
		// members' MaxEntityTerms most frequent words. Held, it would tie with every wNN on
		// 2/2*ln(3/2) and sort first; it is not held, so the ten are w00..w09.
		var text []string
		var want []string
		for i := range MaxEntityTerms {
			w := fmt.Sprintf("w%02d", i)
			text = append(text, w, w)
			if i < MaxKeywords {
				want = append(want, w)
			}
		}
		text = append(text, "aaa")
		mustUpsert(t, m, "acme.dep1.s3.docs.note.c0", 1, strings.Join(text, " "))
		mustUpsert(t, m, "acme.dep1.s3.docs.note.c1", 1, strings.Join(text, " "))
		// A group of one: its one member is enough to hold a word, ln(3/1).
		mustUpsert(t, m, "acme.dep1.s4.docs.note.d0", 1, "zzz")
		m.BootstrapDone()
		mustTick(t, m)
		wantKeywords(t, kv, "0.acme.dep1.s3.docs", want...)
		wantKeywords(t, kv, "0.acme.dep1.s4.docs", "zzz")
	})
}

// TestFailedTickRebuildsUnchangedGroups: a group a failed tick found unchanged is built again by
// the next tick, against the graph-wide counts of that moment. Here those counts move between
// the failed tick and the good one, so s1's keyword alpha, held by every member with words by
// then, scores ln(4/4) = 0 and is no keyword any more.
func TestFailedTickRebuildsUnchangedGroups(t *testing.T) {
	m, kv := newTestMaintainer(t)
	a0, a1 := "acme.dep1.s1.docs.note.a0", "acme.dep1.s1.docs.note.a1"
	b0, c0 := "acme.dep1.s2.docs.note.b0", "acme.dep1.s3.docs.note.c0"
	mustUpsert(t, m, a0, 1, "alpha")
	mustUpsert(t, m, a1, 1, "alpha")
	mustUpsert(t, m, b0, 1, "beta")
	mustUpsert(t, m, c0, 1, "gamma")
	m.BootstrapDone()
	mustTick(t, m)
	s1 := "0.acme.dep1.s1.docs"
	wantKeywords(t, kv, s1, "alpha") // 2/2*ln(4/2)

	// The same value at a newer revision changes s1's records. The first failing tick writes s1
	// (the first key) and fails on the next; the second fails on its first write, after finding
	// s1's record equal to the one written.
	mustUpsert(t, m, a1, 2, "alpha")
	for _, allowed := range []int{1, 0} {
		n := 0
		kv.set(func(f *fakeKV) {
			f.beforePut = func(context.Context, string) error {
				if n++; n > allowed {
					return errors.New("injected put failure")
				}
				return nil
			}
		})
		if err := m.Tick(t.Context()); err == nil {
			t.Fatalf("Tick failing after %d writes = nil, want an error", allowed)
		}
	}
	kv.set(func(f *fakeKV) { f.beforePut = nil })

	mustUpsert(t, m, b0, 2, "alpha beta")
	mustUpsert(t, m, c0, 2, "alpha gamma")
	mustTick(t, m)
	wantKeywords(t, kv, s1)
}

// TestTelemetryRevisionKeepsKeywords: revisions that change only a value no text rule reads
// (a size) change no graph-wide count, so every record a tick publishes equals the record a
// restart fed the same values would publish, and a group none of whose members changed is
// never written.
func TestTelemetryRevisionKeepsKeywords(t *testing.T) {
	state := func(id, text string, size float64) *graph.EntityState {
		return &graph.EntityState{ID: id, Triples: []message.Triple{
			{Subject: id, Predicate: "docs.note.body", Object: text},
			{Subject: id, Predicate: "docs.note.size", Object: size},
		}}
	}
	type value struct {
		revision uint64
		text     string
		size     float64
	}
	values := map[string]*value{
		"acme.dep1.s1.docs.note.a0": {1, "alpha shared", 1},
		"acme.dep1.s1.docs.note.a1": {1, "alpha shared", 1},
		"acme.dep2.s1.docs.note.b0": {1, "beta shared", 1},
		"acme.dep2.s1.docs.note.b1": {1, "beta shared", 1},
	}
	feed := func(m *Maintainer) {
		for _, id := range slices.Sorted(maps.Keys(values)) {
			v := values[id]
			if err := m.Upsert(id, v.revision, state(id, v.text, v.size)); err != nil {
				t.Fatal(err)
			}
		}
		m.BootstrapDone()
	}
	restartRecords := func() map[string]*Community {
		fresh, kv := newTestMaintainer(t)
		feed(fresh)
		mustTick(t, fresh)
		return kv.records(t)
	}

	m, kv := newTestMaintainer(t)
	feed(m)
	mustTick(t, m)
	wantKeywords(t, kv, "0.acme.dep1.s1.docs", "alpha") // 2/2*ln(4/2); shared, ln(4/4), is none
	wantKeywords(t, kv, "0.acme.dep2.s1.docs", "beta")
	kv.takeOps()

	for i := range 20 {
		id := []string{"acme.dep1.s1.docs.note.a0", "acme.dep1.s1.docs.note.a1"}[i%2]
		values[id].revision++
		values[id].size++
		v := values[id]
		if err := m.Upsert(id, v.revision, state(id, v.text, v.size)); err != nil {
			t.Fatal(err)
		}
		mustTick(t, m)
		for _, op := range kv.takeOps() {
			if strings.Contains(op, "acme.dep2") {
				t.Fatalf("revision %d of %s: %s, a group with no changed member was written", v.revision, id, op)
			}
		}
		got, want := kv.records(t), restartRecords()
		if len(got) != len(want) {
			t.Fatalf("revision %d of %s: records %q, a restart publishes %q", v.revision, id,
				slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
		}
		for key, w := range want {
			if !reflect.DeepEqual(got[key], w) {
				t.Fatalf("revision %d of %s: record %s = %+v, a restart publishes %+v", v.revision, id, key, got[key], w)
			}
		}
	}
}

func wantKeywords(t *testing.T, kv *fakeKV, key string, want ...string) {
	t.Helper()
	r := kv.records(t)[key]
	if r == nil {
		t.Fatalf("no record %s", key)
	}
	if !slices.Equal(r.Keywords, want) {
		t.Fatalf("%s keywords = %q, want %q", key, r.Keywords, want)
	}
}
