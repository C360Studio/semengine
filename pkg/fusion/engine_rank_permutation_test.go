package fusion_test

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/fusion"
)

// The generated check for ranking (spec "Ranking is a stable permutation"): Fuse
// orders the hydrated entities by its score, keeps resolve order among entities it
// cannot tell apart, and holds each hydrated entity exactly once. The oracle is
// built by construction, never from the ranker: entities in one group share a name
// that matches no query, carry no class and no weighted predicate, so nothing but
// their resolve position separates them, and their expected relative order is
// resolve order. Other entities get names that match the query exactly, by prefix
// or by substring, and classes and weighted predicates the signals score, so the
// ranker reorders around the groups. Every group has at least two members and every
// generated case has at least one group, so the order assertion runs on every case.
//
// What the examples still own: which signal outranks which (the pin's
// TestEngine_Signals_* tests) and hydration order as the ranking prior
// (TestEngine_RankingFollowsHydrationOrder).

const (
	rankBoostPredicate  = "rank.weight.boost"
	rankDemotePredicate = "rank.weight.demote"
	rankClass           = "http://example.org/Specific"
)

// rankEntity is one generated entity: its group, -1 for none, and its triples.
type rankEntity struct {
	id    string
	group int
	ent   *fusion.Entity
}

// fuseRanked runs one Fuse over entities in the given resolve order and returns the
// handles of the nodes in response order.
func fuseRanked(ctx context.Context, query string, resolved []rankEntity) ([]string, error) {
	byID := make(map[string]*fusion.Entity, len(resolved))
	ids := make([]string, len(resolved))
	for i, r := range resolved {
		byID[r.id] = r.ent
		ids[i] = r.id
	}
	g := &fakeGraph{seeds: map[string][]string{query: ids}, entities: byID}
	signals := fakeSignals{
		depth:    map[string]float64{rankClass: 3},
		salience: map[string]float64{rankBoostPredicate: 1.5, rankDemotePredicate: -1.5},
	}
	eng, err := fusion.NewEngine(g, nil, slog.New(slog.DiscardHandler), fusion.WithSignals(signals))
	if err != nil {
		return nil, fmt.Errorf("NewEngine: %w", err)
	}
	resp, err := eng.Fuse(ctx, fusion.Request{
		Query:  query,
		Want:   []fusion.Want{fusion.WantBody},
		Budget: fusion.Budget{MaxNodes: len(resolved) + 1},
	}, refLens{})
	if err != nil {
		return nil, fmt.Errorf("Fuse: %w", err)
	}
	handles := make([]string, len(resp.Nodes))
	for i, n := range resp.Nodes {
		handles[i] = n.Handle
	}
	return handles, nil
}

// checkStablePermutation reports the first way handles breaks the law for resolved.
func checkStablePermutation(resolved []rankEntity, handles []string) error {
	want := make([]string, len(resolved))
	for i, r := range resolved {
		want[i] = r.id
	}
	got := slices.Clone(handles)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return fmt.Errorf("ranking is not the input's multiset: got %v, want %v", handles, want)
	}
	position := make(map[string]int, len(handles))
	for i, h := range handles {
		position[h] = i
	}
	last := map[int]rankEntity{}
	for _, r := range resolved { // resolve order
		if r.group < 0 {
			continue
		}
		if prev, ok := last[r.group]; ok && position[prev.id] > position[r.id] {
			return fmt.Errorf("group %d lost resolve order: %s resolved before %s but ranks after it (%v)",
				r.group, prev.id, r.id, handles)
		}
		last[r.group] = r
	}
	return nil
}

func TestRankIsStablePermutation(t *testing.T) {
	// Named witness, run every time before the generated cases: an exact score tie
	// between different names. With 3 entities the resolve base is 12, 8, 4 by
	// position; the third's name contains the query (+8), so the first and the third
	// both score 12 and resolve order must decide between them.
	t.Run("exact tie keeps resolve order", func(t *testing.T) {
		first := rankEntity{id: "acme.ops.code.repo.symbol.first", group: 0,
			ent: entity("acme.ops.code.repo.symbol.first", "grp-0", "a.go")}
		second := rankEntity{id: "acme.ops.code.repo.symbol.second", group: -1,
			ent: entity("acme.ops.code.repo.symbol.second", "other", "b.go")}
		third := rankEntity{id: "acme.ops.code.repo.symbol.third", group: 0,
			ent: entity("acme.ops.code.repo.symbol.third", "xalphax", "c.go")}
		resolved := []rankEntity{first, second, third}
		handles, err := fuseRanked(t.Context(), "alpha", resolved)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{first.id, third.id, second.id}; !slices.Equal(handles, want) {
			t.Fatalf("ranking = %v, want %v", handles, want)
		}
		if err := checkStablePermutation(resolved, handles); err != nil {
			t.Fatal(err)
		}
	})

	queries := []string{"alpha", "beta", "gamma"}
	var cases, groupMembers, others int
	t.Run("generated", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			query := rapid.SampledFrom(queries).Draw(rt, "query")
			var pool []rankEntity
			groups := rapid.IntRange(1, 3).Draw(rt, "groups")
			for g := range groups {
				size := rapid.IntRange(2, 5).Draw(rt, fmt.Sprintf("group%dSize", g))
				for range size {
					id := fmt.Sprintf("acme.ops.code.repo.symbol.e%d", len(pool))
					// grp-N contains no query word, and the entity carries no class
					// and no weighted predicate: nothing scores it but its position.
					pool = append(pool, rankEntity{id: id, group: g,
						ent: entity(id, fmt.Sprintf("grp-%d", g), "g.go")})
				}
			}
			for range rapid.IntRange(0, 10).Draw(rt, "others") {
				id := fmt.Sprintf("acme.ops.code.repo.symbol.e%d", len(pool))
				name := rapid.SampledFrom([]string{query, query + "x", "x" + query + "x", "plain"}).Draw(rt, "name")
				var extra []message.Triple
				if rapid.Bool().Draw(rt, "classed") {
					extra = append(extra, message.Triple{Predicate: "entity.ontology.class", Object: rankClass})
				}
				switch rapid.IntRange(0, 2).Draw(rt, "weight") {
				case 1:
					extra = append(extra, message.Triple{Predicate: rankBoostPredicate, Object: true})
				case 2:
					extra = append(extra, message.Triple{Predicate: rankDemotePredicate, Object: true})
				}
				pool = append(pool, rankEntity{id: id, group: -1, ent: entity(id, name, "o.go", extra...)})
			}
			resolved := rapid.Permutation(pool).Draw(rt, "resolveOrder")
			handles, err := fuseRanked(t.Context(), query, resolved)
			if err != nil {
				rt.Fatal(err)
			}
			if err := checkStablePermutation(resolved, handles); err != nil {
				rt.Fatal(err)
			}
			cases++
			for _, r := range resolved {
				if r.group >= 0 {
					groupMembers++
				} else {
					others++
				}
			}
		})
	})
	t.Logf("cases checked %d, group members %d, other entities %d", cases, groupMembers, others)
}
