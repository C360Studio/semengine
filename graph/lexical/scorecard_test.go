package lexical

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// scorecardFloor is the least share of known-item questions whose answer
// must be in the first ten hits: the R@10 measured when NameWeight was set,
// minus one point.
const scorecardFloor = 0.55

var scorecardDir = filepath.Join("testdata", "scorecard")

// section is one entity of the frozen corpus.
type section struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
	Body string `json:"body"`
}

func loadCorpus(t *testing.T) []section {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scorecardDir, "corpus.jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var corpus []section
	sc := bufio.NewScanner(zr)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var s section
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			t.Fatalf("corpus line %d: %v", len(corpus)+1, err)
		}
		corpus = append(corpus, s)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(corpus) != 2000 {
		t.Fatalf("corpus holds %d sections, want 2000", len(corpus))
	}
	return corpus
}

func corpusIndex(t *testing.T, corpus []section) *Index {
	t.Helper()
	ix := NewIndex()
	for _, s := range corpus {
		mustPut(t, ix, s.ID, Document{Name: s.Name, Body: s.Body})
	}
	if ix.Len() != len(corpus) {
		t.Fatalf("index holds %d entities, want %d", ix.Len(), len(corpus))
	}
	return ix
}

func readJSON(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scorecardDir, name))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// knownItem is a section heading asked as a question; the answer is the
// section under it, whose text does not hold the heading.
type knownItem struct {
	Query string `json:"query"`
	ID    string `json:"id"`
}

type recall struct {
	questions, top1, top10 int
	reciprocal             float64
}

func (r recall) at1() float64  { return float64(r.top1) / float64(r.questions) }
func (r recall) at10() float64 { return float64(r.top10) / float64(r.questions) }
func (r recall) mrr() float64  { return r.reciprocal / float64(r.questions) }

// measure asks every question of ix and ranks its answer among every hit.
func measure(t *testing.T, ix *Index, items []knownItem) recall {
	t.Helper()
	r := recall{questions: len(items)}
	for _, it := range items {
		res := mustSearch(t, ix, it.Query, Options{Limit: ix.Len()})
		rank := slices.IndexFunc(res.Hits, func(h Hit) bool { return h.EntityID == it.ID })
		if rank < 0 {
			continue
		}
		r.reciprocal += 1 / float64(rank+1)
		if rank == 0 {
			r.top1++
		}
		if rank < 10 {
			r.top10++
		}
	}
	return r
}

func TestScorecardHoldsItsFloor(t *testing.T) {
	ix := corpusIndex(t, loadCorpus(t))
	var items []knownItem
	readJSON(t, "known.json", &items)
	if len(items) != 500 {
		t.Fatalf("known.json holds %d questions, want 500", len(items))
	}
	r := measure(t, ix, items)
	t.Logf("NameWeight %d: R@1 %.1f%%  R@10 %.1f%%  MRR %.3f over %d questions",
		NameWeight, 100*r.at1(), 100*r.at10(), r.mrr(), r.questions)
	if r.at10() < scorecardFloor {
		t.Errorf("R@10 = %.1f%%, below the floor of %.1f%%", 100*r.at10(), 100*scorecardFloor)
	}
}

// behaviourQuestion is one entry of questions.json; README.md says how each
// expectation is graded.
type behaviourQuestion struct {
	ID   string `json:"id"`
	Band string `json:"band"`
	Twin string `json:"twin"`
	Args struct {
		Query string   `json:"query"`
		Scope []string `json:"scope"`
		Limit *int     `json:"limit"`
	} `json:"args"`
	ExpectAll       []string `json:"expect_all"`
	ExpectAny       []string `json:"expect_any"`
	ExpectNone      []string `json:"expect_none"`
	ExpectTopAll    []string `json:"expect_top_all"`
	ExpectIDs       []string `json:"expect_ids"`
	ExpectNoIDs     []string `json:"expect_no_ids"`
	ExpectNoHits    bool     `json:"expect_no_hits"`
	ExpectHitsMatch string   `json:"expect_hits_match"`
	ExpectError     string   `json:"expect_error"`
	Why             string   `json:"why"`
}

func (q behaviourQuestion) expects() bool {
	return len(q.ExpectAll)+len(q.ExpectAny)+len(q.ExpectNone)+len(q.ExpectTopAll)+
		len(q.ExpectIDs)+len(q.ExpectNoIDs) > 0 ||
		q.ExpectNoHits || q.ExpectHitsMatch != "" || q.ExpectError != ""
}

var parseErrors = map[string]error{
	"empty":    ErrEmptyQuery,
	"reserved": ErrReservedSyntax,
	"too_long": ErrQueryTooLong,
}

func TestScorecardBehaviour(t *testing.T) {
	corpus := loadCorpus(t)
	ix := corpusIndex(t, corpus)
	byID := make(map[string]section, len(corpus))
	for _, s := range corpus {
		byID[s.ID] = s
	}
	var file struct {
		Version   int                 `json:"version"`
		Corpus    string              `json:"corpus"`
		Grading   string              `json:"grading"`
		Questions []behaviourQuestion `json:"questions"`
	}
	readJSON(t, "questions.json", &file)
	ids := make(map[string]bool)
	for _, q := range file.Questions {
		if ids[q.ID] {
			t.Fatalf("question %s appears twice", q.ID)
		}
		ids[q.ID] = true
	}

	for _, q := range file.Questions {
		t.Run(q.ID, func(t *testing.T) {
			if !q.expects() {
				t.Fatalf("question %s expects nothing", q.ID)
			}
			if q.Twin != "" && !ids[q.Twin] {
				t.Fatalf("twin %s is not a question", q.Twin)
			}
			for _, id := range slices.Concat(q.ExpectIDs, q.ExpectNoIDs) {
				if _, ok := byID[id]; !ok {
					t.Fatalf("%s is not in the corpus", id)
				}
			}
			parsed, err := ParseQuery(q.Args.Query)
			if q.ExpectError != "" {
				want, ok := parseErrors[q.ExpectError]
				if !ok {
					t.Fatalf("unknown expect_error %q", q.ExpectError)
				}
				if !errors.Is(err, want) {
					t.Fatalf("ParseQuery(%q) error = %v, want %v", q.Args.Query, err, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", q.Args.Query, err)
			}
			limit := 10
			if l := q.Args.Limit; l != nil {
				limit = *l
				if limit == 0 {
					limit = ix.Len()
				}
			}
			res, err := ix.Search(parsed, Options{Scope: q.Args.Scope, Limit: limit})
			if err != nil {
				t.Fatalf("Search(%q): %v", q.Args.Query, err)
			}
			gradeBehaviour(t, q, res, byID, corpus)
		})
	}
}

func holds(text, s string) bool { return strings.Contains(strings.ToLower(text), strings.ToLower(s)) }

func gradeBehaviour(t *testing.T, q behaviourQuestion, res Result, byID map[string]section, corpus []section) {
	t.Helper()
	hits := hitIDs(res)
	texts := make([]string, len(hits))
	for i, id := range hits {
		texts[i] = byID[id].Name + "\n" + byID[id].Body
	}
	anyHolds := func(s string) bool { return slices.ContainsFunc(texts, func(x string) bool { return holds(x, s) }) }

	if q.ExpectNoHits && res.Matched != 0 {
		t.Errorf("%q matched %d, want none: %v", q.Args.Query, res.Matched, hits)
	}
	for _, s := range q.ExpectAll {
		if !anyHolds(s) {
			t.Errorf("%q: no hit holds %q", q.Args.Query, s)
		}
	}
	if len(q.ExpectAny) > 0 && !slices.ContainsFunc(q.ExpectAny, anyHolds) {
		t.Errorf("%q: no hit holds any of %q", q.Args.Query, q.ExpectAny)
	}
	for _, s := range q.ExpectNone {
		for i, x := range texts {
			if holds(x, s) {
				t.Errorf("%q: hit %s holds %q", q.Args.Query, hits[i], s)
			}
		}
	}
	for _, s := range q.ExpectTopAll {
		if len(texts) == 0 || !holds(texts[0], s) {
			t.Errorf("%q: top hit of %v does not hold %q", q.Args.Query, hits, s)
		}
	}
	for _, id := range q.ExpectIDs {
		if !slices.Contains(hits, id) {
			t.Errorf("%q: %s not among %d hits", q.Args.Query, id, len(hits))
		}
	}
	for _, id := range q.ExpectNoIDs {
		if slices.Contains(hits, id) {
			t.Errorf("%q: %s among the hits", q.Args.Query, id)
		}
	}
	if q.ExpectHitsMatch != "" {
		re := regexp.MustCompile(q.ExpectHitsMatch)
		var want []string
		for _, s := range corpus {
			if re.MatchString(s.Name) || re.MatchString(s.Body) {
				want = append(want, s.ID)
			}
		}
		slices.Sort(want)
		got := slices.Sorted(slices.Values(hits))
		if !slices.Equal(got, want) {
			t.Errorf("%q: hits differ from the sections matching %q\n  only in hits: %v\n  only matching: %v",
				q.Args.Query, q.ExpectHitsMatch, minus(got, want), minus(want, got))
		}
	}
}

// minus returns the members of a that b lacks.
func minus(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}
