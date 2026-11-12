package lexical

import (
	"errors"
	"math"
	"slices"
	"sync"
	"testing"
)

// plainQuery is a query of the distinct words of text.
func plainQuery(text string) Query {
	var q Query
	for _, w := range Tokens(text) {
		if !slices.Contains(q.words, w) {
			q.words = append(q.words, w)
		}
	}
	return q
}

func mustPut(t *testing.T, ix *Index, id string, doc Document) {
	t.Helper()
	if err := ix.Put(id, doc); err != nil {
		t.Fatalf("Put(%q): %v", id, err)
	}
}

func mustSearch(t *testing.T, ix *Index, query string, opts Options) Result {
	t.Helper()
	res, err := ix.Search(plainQuery(query), opts)
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	return res
}

func hitIDs(res Result) []string {
	ids := make([]string, 0, len(res.Hits))
	for _, h := range res.Hits {
		ids = append(ids, h.EntityID)
	}
	return ids
}

const (
	idOne = "c360.lex.test.doc.item.one"
	idTwo = "c360.lex.test.doc.item.two"
)

func TestPutReplacesAndDeletes(t *testing.T) {
	ix := NewIndex()
	words := []string{"alpha", "beta", "gamma"}
	steps := []struct {
		name   string
		write  func()
		holds  string // the word the entity's last write holds, if any
		length int
	}{
		{"alpha", func() { mustPut(t, ix, idOne, Document{Body: "alpha"}) }, "alpha", 1},
		{"replaced by beta", func() { mustPut(t, ix, idOne, Document{Name: "beta"}) }, "beta", 1},
		{"empty document", func() { mustPut(t, ix, idOne, Document{Name: "the", Body: "  "}) }, "", 0},
		{"gamma", func() { mustPut(t, ix, idOne, Document{Body: "gamma"}) }, "gamma", 1},
		{"deleted", func() { ix.Delete(idOne) }, "", 0},
		{"deleted again", func() { ix.Delete(idOne) }, "", 0},
	}
	for _, step := range steps {
		step.write()
		for _, w := range words {
			got := hitIDs(mustSearch(t, ix, w, Options{Limit: 10}))
			var want []string
			if w == step.holds {
				want = []string{idOne}
			}
			if !slices.Equal(got, want) {
				t.Errorf("after %s: search %q = %v, want %v", step.name, w, got, want)
			}
		}
		if got := ix.Len(); got != step.length {
			t.Errorf("after %s: Len() = %d, want %d", step.name, got, step.length)
		}
	}

	err := ix.Put("not an id", Document{Body: "alpha"})
	if !errors.Is(err, ErrInvalidEntityID) {
		t.Fatalf("Put(%q) = %v, want ErrInvalidEntityID", "not an id", err)
	}
	if got := ix.Len(); got != 0 {
		t.Errorf("after a refused Put: Len() = %d, want 0", got)
	}
}

func TestSearchRanksByBM25(t *testing.T) {
	ix := NewIndex()
	docs := map[string]string{
		"c360.lex.test.doc.rank.e1": "alpha beta",
		"c360.lex.test.doc.rank.e2": "alpha alpha gamma",
		"c360.lex.test.doc.rank.e3": "alpha gamma gamma gamma",
		"c360.lex.test.doc.rank.e4": "beta beta beta gamma delta delta",
		"c360.lex.test.doc.rank.e5": "alpha gamma",
	}
	for id, body := range docs {
		mustPut(t, ix, id, Document{Body: body})
	}
	// Worked by hand. N = 5, lengths 2 3 4 6 2, average 17/5 = 3.4.
	// alpha: df 4, IDF ln(1 + 1.5/4.5) = ln(4/3) = 0.2876820724517809.
	// beta:  df 2, IDF ln(1 + 3.5/2.5) = ln(2.4) = 0.8754687373538999.
	// Each term adds IDF * tf * 2.2 / (tf + 1.2 * (0.25 + 0.75 * len/3.4)):
	// e1 = ln(4/3)*2.2/(1+1.2*(0.25+0.75*2/3.4)) + ln(2.4)*2.2/(same)
	// e4 = ln(2.4)*3*2.2/(3+1.2*(0.25+0.75*6/3.4))
	// e2 = ln(4/3)*2*2.2/(2+1.2*(0.25+0.75*3/3.4))
	// e5 = ln(4/3)*2.2/(1+1.2*(0.25+0.75*2/3.4))
	// e3 = ln(4/3)*2.2/(1+1.2*(0.25+0.75*4/3.4))
	want := []Hit{
		{"c360.lex.test.doc.rank.e1", 1.398772999573391},
		{"c360.lex.test.doc.rank.e4", 1.1820408222756627},
		{"c360.lex.test.doc.rank.e2", 0.4090992209010116},
		{"c360.lex.test.doc.rank.e5", 0.34595850513493903},
		{"c360.lex.test.doc.rank.e3", 0.26831195784779566},
	}
	for _, limit := range []int{10, 2} {
		res := mustSearch(t, ix, "alpha beta", Options{Limit: limit})
		w := want[:min(limit, len(want))]
		if res.Matched != 5 {
			t.Errorf("limit %d: Matched = %d, want 5", limit, res.Matched)
		}
		if len(res.Hits) != len(w) {
			t.Fatalf("limit %d: hits = %v, want %v", limit, res.Hits, w)
		}
		for i := range w {
			if res.Hits[i].EntityID != w[i].EntityID || math.Abs(res.Hits[i].Score-w[i].Score) > 1e-12 {
				t.Errorf("limit %d: hit %d = %v, want %v", limit, i, res.Hits[i], w[i])
			}
		}
	}
}

func TestSearchBreaksTiesByID(t *testing.T) {
	ix := NewIndex()
	for _, id := range []string{"c360.lex.test.doc.tie.c", "c360.lex.test.doc.tie.a", "c360.lex.test.doc.tie.b"} {
		mustPut(t, ix, id, Document{Body: "alpha beta"})
	}
	got := hitIDs(mustSearch(t, ix, "alpha", Options{Limit: 10}))
	want := []string{"c360.lex.test.doc.tie.a", "c360.lex.test.doc.tie.b", "c360.lex.test.doc.tie.c"}
	if !slices.Equal(got, want) {
		t.Errorf("hits = %v, want %v", got, want)
	}
}

func TestNameOutranksBody(t *testing.T) {
	ix := NewIndex()
	// Equal weighted length: one Name word and two Body words each.
	mustPut(t, ix, idOne, Document{Name: "pump", Body: "valve motor"})
	mustPut(t, ix, idTwo, Document{Name: "valve", Body: "pump motor"})
	res := mustSearch(t, ix, "pump", Options{Limit: 10})
	if got, want := hitIDs(res), []string{idOne, idTwo}; !slices.Equal(got, want) {
		t.Fatalf("hits = %v, want %v", res.Hits, want)
	}
	if res.Hits[0].Score <= res.Hits[1].Score {
		t.Errorf("name match scores %v, body match %v; want the name match higher", res.Hits[0].Score, res.Hits[1].Score)
	}
}

func TestScopeBeforeLimit(t *testing.T) {
	ix := NewIndex()
	const inside, outside = "c360.lex.test.inside", "c360.lex.test.outside"
	// Outside entities hold the query word three times, inside ones once or
	// twice, so every outside entity outscores every inside one.
	docs := map[string]string{
		outside + ".item.a": "pump pump pump",
		outside + ".item.b": "pump pump pump",
		outside + ".item.c": "pump pump pump",
		inside + ".item.a":  "pump motor valve",
		inside + ".item.b":  "pump pump valve",
		inside + ".item.c":  "pump motor motor",
		inside + ".item.d":  "motor valve",
	}
	for id, body := range docs {
		mustPut(t, ix, id, Document{Body: body})
	}
	all := mustSearch(t, ix, "pump", Options{Limit: 100})
	unscoped := make(map[string]float64, len(all.Hits))
	for _, h := range all.Hits {
		unscoped[h.EntityID] = h.Score
	}

	res := mustSearch(t, ix, "pump", Options{Scope: []string{inside}, Limit: 2})
	if res.Matched != 3 {
		t.Errorf("Matched = %d, want 3", res.Matched)
	}
	// inside.item.b holds the word twice; a and c tie on tf and length, so
	// the ID breaks the tie.
	want := []string{inside + ".item.b", inside + ".item.a"}
	if got := hitIDs(res); !slices.Equal(got, want) {
		t.Fatalf("hits = %v, want %v", res.Hits, want)
	}
	for _, h := range res.Hits {
		if h.Score != unscoped[h.EntityID] {
			t.Errorf("%s scores %v in scope, %v without", h.EntityID, h.Score, unscoped[h.EntityID])
		}
	}

	if _, err := ix.Search(plainQuery("pump"), Options{Scope: []string{"c360."}, Limit: 2}); !errors.Is(err, ErrInvalidScope) {
		t.Errorf("scope %q: err = %v, want ErrInvalidScope", "c360.", err)
	}
	if _, err := ix.Search(plainQuery("pump"), Options{Scope: []string{inside}}); !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("limit 0: err = %v, want ErrInvalidLimit", err)
	}
}

func TestSingularFindsPlural(t *testing.T) {
	ix := NewIndex()
	mustPut(t, ix, idOne, Document{Body: "services processes"})
	for _, q := range []string{"service", "process"} {
		if got := hitIDs(mustSearch(t, ix, q, Options{Limit: 10})); !slices.Equal(got, []string{idOne}) {
			t.Errorf("search %q = %v, want [%s]", q, got, idOne)
		}
	}
}

func TestJoinedIdentifierIsNotHeld(t *testing.T) {
	ix := NewIndex()
	mustPut(t, ix, idOne, Document{Body: "withRetry"})
	for _, q := range []string{"withRetry", "retry"} {
		if got := hitIDs(mustSearch(t, ix, q, Options{Limit: 10})); !slices.Equal(got, []string{idOne}) {
			t.Errorf("search %q = %v, want [%s]", q, got, idOne)
		}
	}
	if got := hitIDs(mustSearch(t, ix, "withretry", Options{Limit: 10})); len(got) != 0 {
		t.Errorf("search %q = %v; the joined form must not be held", "withretry", got)
	}
}

func TestSearchDuringWrites(t *testing.T) {
	const switching = "c360.lex.test.doc.item.switch"
	fixed := map[string]Document{
		"c360.lex.test.doc.item.a": {Name: "pump", Body: "valve motor"},
		"c360.lex.test.doc.item.b": {Body: "motor motor relay"},
		"c360.lex.test.doc.item.c": {Body: "valve relay"},
	}
	// Both documents hold pump and motor, with different counts and lengths,
	// so a search that saw the entity missing or half written gets an
	// answer equal to neither.
	docA := Document{Name: "pump motor", Body: "relay"}
	docB := Document{Body: "pump pump motor valve valve valve"}
	queries := []string{"pump", "motor valve", "relay pump"}

	answers := func(doc Document) []Result {
		ix := NewIndex()
		for id, d := range fixed {
			mustPut(t, ix, id, d)
		}
		mustPut(t, ix, switching, doc)
		out := make([]Result, len(queries))
		for i, q := range queries {
			out[i] = mustSearch(t, ix, q, Options{Limit: 10})
		}
		return out
	}
	beforeA, beforeB := answers(docA), answers(docB)
	for i := range queries {
		if equalResults(beforeA[i], beforeB[i]) {
			t.Fatalf("query %q answers the same for both documents; it cannot tell them apart", queries[i])
		}
	}

	ix := NewIndex()
	for id, d := range fixed {
		mustPut(t, ix, id, d)
	}
	mustPut(t, ix, switching, docA)

	// The writer runs until every searcher has finished, and searchers start
	// only after its first write, so every search runs while writes go on.
	searchersDone := make(chan struct{})
	firstWrite := make(chan struct{})
	var writer sync.WaitGroup
	writer.Go(func() {
		for i := 0; ; i++ {
			doc := docA
			if i%2 == 0 {
				doc = docB
			}
			if err := ix.Put(switching, doc); err != nil {
				t.Errorf("Put: %v", err)
			}
			if i == 0 {
				close(firstWrite)
			}
			select {
			case <-searchersDone:
				return
			default:
			}
		}
	})
	<-firstWrite
	var searchers sync.WaitGroup
	for range 4 {
		searchers.Go(func() {
			for i := range 5000 {
				q := i % len(queries)
				res, err := ix.Search(plainQuery(queries[q]), Options{Limit: 10})
				if err != nil {
					t.Errorf("Search(%q): %v", queries[q], err)
					return
				}
				if !equalResults(res, beforeA[q]) && !equalResults(res, beforeB[q]) {
					t.Errorf("Search(%q) = %+v during writes; want %+v or %+v", queries[q], res, beforeA[q], beforeB[q])
					return
				}
			}
		})
	}
	searchers.Wait()
	close(searchersDone)
	writer.Wait()
}

func equalResults(a, b Result) bool {
	return a.Matched == b.Matched && slices.Equal(a.Hits, b.Hits)
}
