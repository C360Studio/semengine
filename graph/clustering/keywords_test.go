package clustering

import (
	"fmt"
	"slices"
	"strings"
	"testing"
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
