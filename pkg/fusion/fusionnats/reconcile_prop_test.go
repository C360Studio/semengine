package fusionnats

import (
	"fmt"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/fusion"
)

// reconcileCase is one batch: the IDs asked for and the reply's two lists.
type reconcileCase struct {
	requested []string
	entities  []string
	missing   []graph.MissingEntity
}

// reconcileModel is the requirement ("The batch read accounts for every
// requested ID"), written without reconcileHydration: each distinct requested ID
// once, in request order; held if the reply returned it, else the reason the
// reply gave in fusion's closed set, else unknown.
func reconcileModel(tc reconcileCase) (held []string, unhydrated []fusion.Unhydrated) {
	for i, id := range tc.requested {
		if slices.Contains(tc.requested[:i], id) {
			continue
		}
		if slices.Contains(tc.entities, id) {
			held = append(held, id)
			continue
		}
		// A reply that lists an ID missing twice is read by its last entry.
		reason := fusion.UnhydratedUnknown
		for _, m := range tc.missing {
			if m.ID != id {
				continue
			}
			reason = fusion.UnhydratedUnknown
			switch m.Reason {
			case "not_found":
				reason = fusion.UnhydratedNotFound
			case "error":
				reason = fusion.UnhydratedError
			}
		}
		unhydrated = append(unhydrated, fusion.Unhydrated{Handle: id, Reason: reason})
	}
	return held, unhydrated
}

// checkReconcile reports the first way reconcileHydration departs from the model.
func checkReconcile(tc reconcileCase) error {
	reply := graph.EntityBatchResponse{Missing: tc.missing}
	for _, id := range tc.entities {
		reply.Entities = append(reply.Entities, graph.EntityState{ID: id})
	}
	got := reconcileHydration(tc.requested, reply)
	gotHeld := make([]string, 0, len(got.Entities))
	for _, e := range got.Entities {
		gotHeld = append(gotHeld, e.ID)
	}
	wantHeld, wantUnhydrated := reconcileModel(tc)
	if !slices.Equal(gotHeld, wantHeld) {
		return fmt.Errorf("entities = %v, want %v", gotHeld, wantHeld)
	}
	if !slices.Equal(got.Unhydrated, wantUnhydrated) {
		return fmt.Errorf("unhydrated = %v, want %v", got.Unhydrated, wantUnhydrated)
	}
	return nil
}

// TestReconcileTotal is the generated check of the batch accounting law. IDs come
// from a pool of four, so requests repeat IDs and the reply returns IDs nobody
// asked for; each pool ID is independently in the reply's entities, its missing
// list (with a reason in or out of the set) once or twice, both, or neither, and the entities
// arrive in any order. Each case compares the whole result with the model, so
// the assertion runs on every case. The named witnesses below run first, so the
// duplicate, both-lists, neither-list and missing-twice cases are checked on every run.
func TestReconcileTotal(t *testing.T) {
	a, b, c := "acme.ops.d.s.t.a", "acme.ops.d.s.t.b", "acme.ops.d.s.t.c"
	witnesses := map[string]reconcileCase{
		"duplicate request":    {requested: []string{a, b, a}, entities: []string{b, a}},
		"in both lists":        {requested: []string{a}, entities: []string{a}, missing: []graph.MissingEntity{{ID: a, Reason: "not_found"}}},
		"in neither list":      {requested: []string{a, b}, entities: []string{a}},
		"reply order reversed": {requested: []string{a, b, c}, entities: []string{c, b, a}},
		"missing twice, last wins": {requested: []string{a}, missing: []graph.MissingEntity{
			{ID: a, Reason: "not_found"}, {ID: a, Reason: "later"},
		}},
	}
	for name, tc := range witnesses {
		t.Run(name, func(t *testing.T) {
			if err := checkReconcile(tc); err != nil {
				t.Fatal(err)
			}
		})
	}

	pool := []string{a, b, c, "acme.ops.d.s.t.d"}
	reasons := []graph.MissingReason{"not_found", "error", "", "later"}
	t.Run("generated", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			tc := reconcileCase{requested: rapid.SliceOfN(rapid.SampledFrom(pool), 0, 8).Draw(rt, "requested")}
			for _, id := range pool {
				switch rapid.IntRange(0, 4).Draw(rt, "lists "+id) {
				case 1:
					tc.entities = append(tc.entities, id)
				case 2:
					tc.missing = append(tc.missing, graph.MissingEntity{ID: id, Reason: rapid.SampledFrom(reasons).Draw(rt, "reason "+id)})
				case 3:
					tc.entities = append(tc.entities, id)
					tc.missing = append(tc.missing, graph.MissingEntity{ID: id, Reason: rapid.SampledFrom(reasons).Draw(rt, "reason "+id)})
				case 4:
					tc.missing = append(tc.missing,
						graph.MissingEntity{ID: id, Reason: rapid.SampledFrom(reasons).Draw(rt, "first reason "+id)},
						graph.MissingEntity{ID: id, Reason: rapid.SampledFrom(reasons).Draw(rt, "last reason "+id)})
				}
			}
			tc.entities = rapid.Permutation(tc.entities).Draw(rt, "entity order")
			if err := checkReconcile(tc); err != nil {
				rt.Fatal(err)
			}
		})
	})
}
