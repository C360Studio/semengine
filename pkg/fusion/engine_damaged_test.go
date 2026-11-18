package fusion_test

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semengine/pkg/fusion"
)

// TestResponseCarriesDamagedCount holds that a response carries the largest
// damaged_entities any graph read behind it reported, never their sum, in every
// resolve mode and on every kind of response built after the resolve answered;
// that it leaves truncated alone; and that the key is absent when no read
// reported one.
func TestResponseCarriesDamagedCount(t *testing.T) {
	seed := entity("S", "S", "s.go")
	near := entity("A", "A", "a.go")
	entities := map[string]*fusion.Entity{"S": seed, "A": near}
	in := map[string][]fusion.Edge{"S": {edge("A")}}

	for name, tc := range map[string]struct {
		query string
		want  []fusion.Want
		g     *fakeGraph
		count int
	}{
		"a relations read reports 1": {"q", []fusion.Want{fusion.WantRelations},
			&fakeGraph{seeds: map[string][]string{"q": {"S"}}, entities: entities, in: in,
				neighborsDamaged: map[string]int{"S": 1}}, 1},
		"resolve at 2, relations at 3": {"q", []fusion.Want{fusion.WantRelations},
			&fakeGraph{seeds: map[string][]string{"q": {"S"}}, entities: entities, in: in,
				resolveDamaged: 2, neighborsDamaged: map[string]int{"S": 3}}, 3},
		"resolve at 3, relations at 2": {"q", []fusion.Want{fusion.WantRelations},
			&fakeGraph{seeds: map[string][]string{"q": {"S"}}, entities: entities, in: in,
				resolveDamaged: 3, neighborsDamaged: map[string]int{"S": 2}}, 3},
		"an impact walk read": {"q", []fusion.Want{fusion.WantImpact},
			&fakeGraph{seeds: map[string][]string{"q": {"S"}}, entities: entities, in: in,
				neighborsDamaged: map[string]int{"S": 2}}, 2},
		"a paths walk read": {"q", []fusion.Want{fusion.WantPaths},
			&fakeGraph{seeds: map[string][]string{"q": {"S"}}, entities: entities,
				out: map[string][]fusion.Edge{"S": {edge("A")}}, neighborsDamaged: map[string]int{"S": 2}}, 2},
		"a graph walk read": {"q", []fusion.Want{fusion.WantGraph},
			&fakeGraph{seeds: map[string][]string{"q": {"S"}}, entities: entities, in: in,
				neighborsDamaged: map[string]int{"S": 2}}, 2},
		"a natural-language resolve": {nlQuery, nil,
			&fakeGraph{seeds: map[string][]string{nlQuery: {"S"}}, entities: entities, resolveDamaged: 2}, 2},
		"a miss's name suggestions": {"q", nil,
			&fakeGraph{names: []string{"S"}, namesDamaged: 2}, 2},
		"resolve at 3, a miss's names at 2": {"q", nil,
			&fakeGraph{names: []string{"S"}, resolveDamaged: 3, namesDamaged: 2}, 3},
		"a deferral after the resolve answered": {"q", nil,
			&fakeGraph{seeds: map[string][]string{"q": {"S"}}, resolveDamaged: 2}, 2},
		"no read reports one": {"q", []fusion.Want{fusion.WantRelations, fusion.WantImpact},
			&fakeGraph{seeds: map[string][]string{"q": {"S"}}, entities: entities, in: in}, 0},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := mustEngine(t, tc.g, nil).Fuse(t.Context(), fusion.Request{Query: tc.query, Want: tc.want}, refLens{})
			if err != nil {
				t.Fatalf("Fuse: %v", err)
			}
			if resp.Truncated {
				t.Errorf("Truncated = true, want false: damage is not the budget cutting nodes off")
			}
			raw, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var keys map[string]json.RawMessage
			if err := json.Unmarshal(raw, &keys); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got, present := keys["damaged_entities"]
			switch {
			case tc.count == 0 && present:
				t.Errorf("damaged_entities = %s when no read reported one, want no key", got)
			case tc.count > 0 && (!present || resp.DamagedEntities != tc.count):
				t.Errorf("damaged_entities = %d (key present %v), want %d", resp.DamagedEntities, present, tc.count)
			}
		})
	}
}
