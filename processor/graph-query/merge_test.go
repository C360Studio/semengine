package graphquery

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
)

// words and vectors are hit lists naming ids, best first, for the merge's input.
func words(ids ...string) []graph.LexicalHit {
	hits := make([]graph.LexicalHit, 0, len(ids))
	for i, id := range ids {
		hits = append(hits, graph.LexicalHit{EntityID: id, Score: float64(100 - i)})
	}
	return hits
}

func vectors(ids ...string) []graph.SimilarityHit {
	hits := make([]graph.SimilarityHit, 0, len(ids))
	for i, id := range ids {
		hits = append(hits, graph.SimilarityHit{EntityID: id, Similarity: 1 - float64(i)/100})
	}
	return hits
}

// row is a merged hit as the tests compare it: its ID, its score, and its place in each list, 0 when
// it is not in that list.
type row struct {
	id                   string
	score                float64
	wordRank, vectorRank int
}

func rows(merged []mergedHit) []row {
	out := make([]row, 0, len(merged))
	for _, hit := range merged {
		out = append(out, row{hit.entityID, hit.score, hit.wordRank, hit.vectorRank})
	}
	return out
}

// referenceMerge is the merge as the requirement states it, kept in the test: each entity's score is
// the sum of 1 / (60 + place) over the lists holding it, place from 1, an entity's first place in a
// list being its place; ordered by score, highest first, then ID ascending. The word term is added
// first, as a sum of two terms is the same either way round.
func referenceMerge(wordIDs, vectorIDs []string) []row {
	byID := map[string]*row{}
	place := func(ids []string, set func(*row, int)) {
		for i, id := range ids {
			r, ok := byID[id]
			if !ok {
				r = &row{id: id}
				byID[id] = r
			}
			set(r, i+1)
		}
	}
	place(wordIDs, func(r *row, p int) {
		if r.wordRank == 0 {
			r.wordRank = p
		}
	})
	place(vectorIDs, func(r *row, p int) {
		if r.vectorRank == 0 {
			r.vectorRank = p
		}
	})
	out := make([]row, 0, len(byID))
	for _, r := range byID {
		if r.wordRank > 0 {
			r.score += 1 / float64(60+r.wordRank)
		}
		if r.vectorRank > 0 {
			r.score += 1 / float64(60+r.vectorRank)
		}
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b row) int { return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.id, b.id)) })
	return out
}

// TestMergeRanks: the merge of a word list and an embedding list, against a hand table, then
// against referenceMerge over generated lists. Generated checks are used because the merge is a
// transformation with stated laws (each entity once, order by score then ID, ranks equal places)
// over two inputs that interact through their overlap; the table's cases are the witnesses for the
// boundaries the laws name: no overlap, full overlap, one list empty, both empty, an equal score
// broken by ID, and an ID listed twice in one list.
func TestMergeRanks(t *testing.T) {
	const a, b, c = "a", "b", "c"
	// term is one list's share of a score, worked out at run time as the sum is, since a constant
	// expression is rounded once and a run-time sum at each step.
	term := func(place int) float64 { return 1 / float64(60+place) }
	for _, tc := range []struct {
		name          string
		words, vector []string
		want          []row
	}{
		{"both lists", []string{a, b}, []string{c, a}, []row{
			{a, term(1) + term(2), 1, 2}, {c, term(1), 0, 1}, {b, term(2), 2, 0},
		}},
		{"no embedding hits keep the word order", []string{c, a, b}, nil, []row{
			{c, term(1), 1, 0}, {a, term(2), 2, 0}, {b, term(3), 3, 0},
		}},
		{"no word hits keep the embedding order", nil, []string{b, c}, []row{
			{b, term(1), 0, 1}, {c, term(2), 0, 2},
		}},
		{"both empty", nil, nil, []row{}},
		{"an equal score goes by ID", []string{b}, []string{a}, []row{
			{a, term(1), 0, 1}, {b, term(1), 1, 0},
		}},
		{"the same list both ways", []string{a, b}, []string{a, b}, []row{
			{a, term(1) + term(1), 1, 1}, {b, term(2) + term(2), 2, 2},
		}},
		{"an ID listed twice keeps its first place", []string{a, b, a}, nil, []row{
			{a, term(1), 1, 0}, {b, term(2), 2, 0},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rows(mergeRanks(words(tc.words...), vectors(tc.vector...))); !slices.Equal(got, tc.want) {
				t.Errorf("merge = %+v\nwant    %+v", got, tc.want)
			}
		})
	}

	pool := make([]string, 12)
	for i := range pool {
		pool[i] = fmt.Sprintf("e%02d", i)
	}
	t.Run("against the reference", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			wordIDs := rapid.SliceOfNDistinct(rapid.SampledFrom(pool), 0, len(pool), rapid.ID).Draw(t, "words")
			vectorIDs := rapid.SliceOfNDistinct(rapid.SampledFrom(pool), 0, len(pool), rapid.ID).Draw(t, "vectors")
			got := rows(mergeRanks(words(wordIDs...), vectors(vectorIDs...)))
			if want := referenceMerge(wordIDs, vectorIDs); !slices.Equal(got, want) {
				t.Fatalf("merge of %q and %q = %+v\nreference %+v", wordIDs, vectorIDs, got, want)
			}
		})
	})
}

// TestStoppedSearchEndsWithoutItsReply: a search started beside the word search and then stopped is
// cancelled, and stop returns only once it has returned, without its reply: the search below never
// answers, and ends only when its context does. The bubble fails the test if any goroutine is left
// blocked when it ends. A search awaited answers its value, and a stop after it waits on nothing.
func TestStoppedSearchEndsWithoutItsReply(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ended := false
		_, stop := goSearch(t.Context(), func(ctx context.Context) int {
			<-ctx.Done()
			ended = true
			return 0
		})
		stop()
		// stop's return is ordered after the search's: it received the search's result.
		if !ended {
			t.Error("stop returned before the search ended")
		}
	})
	synctest.Test(t, func(t *testing.T) {
		await, stop := goSearch(t.Context(), func(context.Context) int { return 7 })
		if got := await(); got != 7 {
			t.Errorf("await = %d, want 7", got)
		}
		if got := await(); got != 7 {
			t.Errorf("a second await = %d, want 7", got)
		}
		stop()
	})
}
