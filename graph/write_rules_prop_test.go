package graph_test

import (
	"reflect"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// Requirement: graph-entity-writes/One rule per write mode and Timestamp orders a replace (design D15, with "within
// one write, statements equal in every field count once")
//
// TestPropReplaceBySourceMatchesModel draws a stored entity and an arrival from one small pool of statements, so one
// write often repeats a statement, carries two sources or predicates at once, ties a timestamp, or arrives older than
// what is stored, in any order. ReplaceBySource's result must equal replaceModel's. Both assertions run on every
// case; the boundaries themselves (an equal timestamp, a repeat, a stale set) are also named examples in
// write_rules_test.go, so no boundary depends on a random draw reaching it.
func TestPropReplaceBySourceMatchesModel(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		pool := rapid.SliceOfN(statementGen(), 1, 5).Draw(rt, "pool")
		pick := func(label string, maxLen int) []message.Triple {
			indexes := rapid.SliceOfN(rapid.IntRange(0, len(pool)-1), 0, maxLen).Draw(rt, label)
			out := make([]message.Triple, len(indexes))
			for i, n := range indexes {
				out[i] = pool[n]
			}
			return out
		}
		stored, arrival := pick("stored", 4), pick("arrival", 6)

		got := graph.ReplaceBySource(stored, arrival)
		wantTriples, wantStale := replaceModel(stored, arrival)
		requireSameStatements(rt, got.Triples, wantTriples)
		if len(got.Stale) != 0 || len(wantStale) != 0 {
			if !reflect.DeepEqual(got.Stale, wantStale) {
				rt.Fatalf("Stale = %+v, want %+v", got.Stale, wantStale)
			}
		}
	})
}

// statementGen draws one statement from a domain small enough that draws share a key and a timestamp often. The
// object may be a slice, which == cannot compare.
func statementGen() *rapid.Generator[message.Triple] {
	expiry := t0.Add(time.Hour)
	return rapid.Custom(func(rt *rapid.T) message.Triple {
		s := message.Triple{
			Subject:    rapid.SampledFrom([]string{subject, subject + "b"}).Draw(rt, "subject"),
			Predicate:  rapid.SampledFrom([]string{predP, predQ}).Draw(rt, "predicate"),
			Source:     rapid.SampledFrom([]string{"A", "B"}).Draw(rt, "source"),
			Timestamp:  rapid.SampledFrom([]time.Time{t0.Add(-time.Second), t0, t0.Add(time.Second)}).Draw(rt, "at"),
			Object:     rapid.SampledFrom([]any{10.0, 11.0, []any{1.0, "two"}}).Draw(rt, "object"),
			Confidence: rapid.SampledFrom([]float64{1, 0.5}).Draw(rt, "confidence"),
		}
		if rapid.Bool().Draw(rt, "expires") {
			s.ExpiresAt = &expiry
		}
		return s
	})
}

// replaceModel is the replace rule as the requirement states it, written without ReplaceBySource's code. The
// statements of one (subject, predicate, source) are one set. A set the arrival carries is stale, and named in
// arrival order, when the stored set of its key has a later latest timestamp; the stored set then stays. Otherwise
// the arrival's set takes the stored set's place, each statement equal in every field kept once (reflect.DeepEqual is
// exact for this domain: float64 or []any objects, UTC times). A stored set the arrival does not carry stays as it is,
// repeats included.
func replaceModel(stored, arrival []message.Triple) ([]message.Triple, []graph.StaleSet) {
	type key struct{ subject, predicate, source string }
	keyOf := func(s message.Triple) key { return key{s.Subject, s.Predicate, s.Source} }
	latest := func(set []message.Triple) time.Time {
		var at time.Time
		for _, s := range set {
			if s.Timestamp.After(at) {
				at = s.Timestamp
			}
		}
		return at
	}

	storedSets := map[key][]message.Triple{}
	for _, s := range stored {
		storedSets[keyOf(s)] = append(storedSets[keyOf(s)], s)
	}
	arrivalSets := map[key][]message.Triple{}
	var carried []key
	for _, s := range arrival {
		k := keyOf(s)
		if _, ok := arrivalSets[k]; !ok {
			carried = append(carried, k)
		}
		arrivalSets[k] = append(arrivalSets[k], s)
	}

	var want []message.Triple
	var stale []graph.StaleSet
	for _, k := range carried {
		if old, ok := storedSets[k]; ok && latest(old).After(latest(arrivalSets[k])) {
			stale = append(stale, graph.StaleSet{Subject: k.subject, Predicate: k.predicate, Source: k.source})
			want = append(want, old...)
			continue
		}
		var set []message.Triple
		for _, s := range arrivalSets[k] {
			if !containsEqual(set, s) {
				set = append(set, s)
			}
		}
		want = append(want, set...)
	}
	for k, old := range storedSets {
		if _, ok := arrivalSets[k]; !ok {
			want = append(want, old...)
		}
	}
	return want, stale
}

func containsEqual(set []message.Triple, s message.Triple) bool {
	for _, have := range set {
		if reflect.DeepEqual(have, s) {
			return true
		}
	}
	return false
}
