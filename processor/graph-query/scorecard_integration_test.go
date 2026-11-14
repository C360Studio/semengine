//go:build integration

package graphquery

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/graph/lexical"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/types"
)

// The scorecard bars. A verb may have the answer in its first ten at most one point less often than
// the plain reference does, and never less often than its floor. The floors sit just under the
// reference's R@10 on this corpus, 56% unscoped and 60% within the group, so a change that lowered
// the reference too could not carry the verbs down with it.
const (
	globalScorecardFloor = 0.55
	localScorecardFloor  = 0.58
	// scorecardLimit is the most matches either verb returns; a rank past it counts as not found.
	// The full hundred, not ten: matches put in a wrong order inside the first hundred then move the
	// answer out of the first ten, which R@10 sees.
	scorecardLimit = graph.MaxSimilarityLimit
)

// scorecardDir is the word index's frozen corpus and questions, shared so both scorecards ask the
// same questions of the same text.
var scorecardDir = filepath.Join("..", "..", "graph", "lexical", "testdata", "scorecard")

// The predicates a section is written with: graph-lexical's default rule reads a .title value as the
// entity's name and a .content value as its body.
const (
	sectionTitle   = "doc.section.title"
	sectionContent = "doc.section.content"
)

type scorecardSection struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
	Body string `json:"body"`
}

// scorecardQuestion is a section heading asked as a question; the answer is the section under it,
// whose text does not hold the heading.
type scorecardQuestion struct {
	Query string `json:"query"`
	ID    string `json:"id"`
}

func loadScorecard(t *testing.T) ([]scorecardSection, []scorecardQuestion) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(scorecardDir, "corpus.jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	var corpus []scorecardSection
	sc := bufio.NewScanner(zr)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var s scorecardSection
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			t.Fatalf("corpus line %d: %v", len(corpus)+1, err)
		}
		corpus = append(corpus, s)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(scorecardDir, "known.json"))
	if err != nil {
		t.Fatal(err)
	}
	var questions []scorecardQuestion
	if err := json.Unmarshal(raw, &questions); err != nil {
		t.Fatal(err)
	}
	if len(corpus) != 2000 || len(questions) != 500 {
		t.Fatalf("scorecard holds %d sections and %d questions, want 2000 and 500", len(corpus), len(questions))
	}
	return corpus, questions
}

// levelZeroGroup is the group a corpus section belongs to at level 0: its ID without the type and
// instance parts, c360.semengine.<top directory>.markdown.
func levelZeroGroup(id string) string {
	parts := strings.Split(id, ".")
	return strings.Join(parts[:4], ".")
}

// bm25 is the plain reference: Okapi BM25 with Lucene's idf, k1 1.2 and b 0.75, over one field
// holding all of a section's words, with nothing the word index adds (no name weight).
type bm25 struct {
	postings  map[string][]bm25Posting
	length    []int
	avgLength float64
}

type bm25Posting struct{ doc, count int }

func newBM25(docs [][]string) *bm25 {
	r := &bm25{postings: map[string][]bm25Posting{}, length: make([]int, len(docs))}
	total := 0
	for d, words := range docs {
		r.length[d] = len(words)
		total += len(words)
		counts := map[string]int{}
		for _, w := range words {
			counts[w]++
		}
		for w, n := range counts {
			r.postings[w] = append(r.postings[w], bm25Posting{d, n})
		}
	}
	r.avgLength = float64(total) / float64(len(docs))
	return r
}

// scores is every document's score for the query's words, each word counted once.
func (r *bm25) scores(query []string) []float64 {
	const k1, b = 1.2, 0.75
	scores := make([]float64, len(r.length))
	n := float64(len(r.length))
	seen := map[string]bool{}
	for _, w := range query {
		if seen[w] {
			continue
		}
		seen[w] = true
		postings := r.postings[w]
		df := float64(len(postings))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for _, p := range postings {
			tf := float64(p.count)
			norm := k1 * (1 - b + b*float64(r.length[p.doc])/r.avgLength)
			scores[p.doc] += idf * tf * (k1 + 1) / (tf + norm)
		}
	}
	return scores
}

// referenceRank is target's 1-based rank among the documents that score above zero and that keep
// accepts, by score and then by ID, or 0 when it scores nothing.
func referenceRank(scores []float64, ids []string, target int, keep func(int) bool) int {
	if scores[target] <= 0 {
		return 0
	}
	rank := 1
	for d, s := range scores {
		if s <= 0 || !keep(d) {
			continue
		}
		if s > scores[target] || (s == scores[target] && ids[d] < ids[target]) {
			rank++
		}
	}
	return rank
}

// tally counts where the answers ranked: R@1, R@10 and MRR, counting a rank past scorecardLimit as
// not found so the reference is measured as deep as the verbs answer.
type tally struct {
	name                   string
	questions, top1, top10 int
	reciprocal             float64
}

func (s *tally) add(rank int) {
	s.questions++
	if rank <= 0 || rank > scorecardLimit {
		return
	}
	s.reciprocal += 1 / float64(rank)
	if rank == 1 {
		s.top1++
	}
	if rank <= 10 {
		s.top10++
	}
}

func (s *tally) String() string {
	share := func(n int) float64 { return 100 * float64(n) / float64(s.questions) }
	return fmt.Sprintf("%-28s R@1 %5.1f%%  R@10 %5.1f%%  MRR %.3f  over %d questions",
		s.name, share(s.top1), share(s.top10), s.reciprocal/float64(s.questions), s.questions)
}

// requireBar fails unless got has the answer in its first ten at most one point less often than ref,
// and at least floor of the time. Counts, not shares: one point of 500 questions is 5 answers.
func requireBar(t *testing.T, got, ref *tally, floor float64) {
	t.Helper()
	if slack := got.questions / 100; got.top10 < ref.top10-slack {
		t.Errorf("%s: R@10 %d of %d, more than one point below the reference's %d", got.name, got.top10, got.questions, ref.top10)
	}
	if float64(got.top10) < floor*float64(got.questions) {
		t.Errorf("%s: R@10 %d of %d, below the floor of %.0f%%", got.name, got.top10, got.questions, 100*floor)
	}
}

// matchRank is id's 1-based place among matches, or 0 when it is not there.
func matchRank(matches []graph.TextSearchMatch, id string) int {
	return slices.IndexFunc(matches, func(m graph.TextSearchMatch) bool { return m.ID == id }) + 1
}

// TestSearchScorecard: the word index's frozen corpus is written through graph-ingest and indexed by
// graph-lexical, and each of its 500 known-item questions is asked through globalSearch and through
// localSearch scoped to the answer's level-0 group. Each verb's R@10 must be at most one point below
// a plain BM25 computed here over the same text, unscoped for global and within the group for local,
// and at or above its floor (spec, "Search quality holds against a plain reference").
func TestSearchScorecard(t *testing.T) {
	corpus, questions := loadScorecard(t)
	stack := startIngestAndIndexAs(t, types.PlatformMeta{Org: "c360", Platform: "semengine"})
	startWordIndex(t, stack.client)
	startComponent(t, stack.client, `{}`)

	// No cap: every corpus body is shorter than graph-lexical's, so it indexes each whole.
	rule := entitytext.New(nil, 0)
	ids := make([]string, len(corpus))
	place := make(map[string]int, len(corpus))
	groups := make([]string, len(corpus))
	words := make([][]string, len(corpus))
	var last uint64
	now := time.Now()
	for i, s := range corpus {
		ids[i], place[s.ID], groups[i] = s.ID, i, levelZeroGroup(s.ID)
		triples := []message.Triple{
			{Subject: s.ID, Predicate: sectionTitle, Object: s.Name, Source: "test", Timestamp: now, Confidence: 1},
			{Subject: s.ID, Predicate: sectionContent, Object: s.Body, Source: "test", Timestamp: now, Confidence: 1},
		}
		state := &graph.EntityState{ID: s.ID, MessageType: testEntityType, UpdatedAt: now}
		created, err := stack.mutations.Create(t.Context(), graph.CreateEntityRequest{Entity: state, Triples: triples})
		if err != nil {
			t.Fatalf("create %s: %v", s.ID, err)
		}
		last = created.KVRevision
		// The reference reads the text graph-lexical's default rule takes from these triples.
		doc := rule.Document(&graph.EntityState{ID: s.ID, Triples: triples}, "")
		words[i] = lexical.Tokens(doc.Name + " " + doc.Body)
	}
	stack.awaitSearchable(t, last)
	reference := newBM25(words)

	globalRef := &tally{name: "global, plain BM25"}
	global := &tally{name: "global, globalSearch"}
	localRef := &tally{name: "local, plain BM25 in group"}
	local := &tally{name: "local, localSearch"}
	for _, q := range questions {
		target, ok := place[q.ID]
		if !ok {
			t.Fatalf("answer %s of %q is not in the corpus", q.ID, q.Query)
		}
		scores := reference.scores(lexical.Tokens(q.Query))
		globalRef.add(referenceRank(scores, ids, target, func(int) bool { return true }))
		localRef.add(referenceRank(scores, ids, target, func(d int) bool { return groups[d] == groups[target] }))

		request, err := json.Marshal(graph.GlobalSearchRequest{Query: q.Query, Limit: scorecardLimit})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := stack.askGlobal(t, string(request))
		if err != nil {
			t.Fatalf("globalSearch %q: %v", q.Query, err)
		}
		_, globalData := decodeGlobal(t, raw)
		global.add(matchRank(globalData.Text.Matches, q.ID))

		request, err = json.Marshal(graph.LocalSearchRequest{EntityID: q.ID, Query: q.Query, Limit: scorecardLimit})
		if err != nil {
			t.Fatal(err)
		}
		raw, err = search(t, stack.client, string(request))
		_, localData := searchSuccess(t, raw, err)
		if localData.CommunityID != groups[target] {
			t.Fatalf("localSearch %q from %s: community %q, want %q", q.Query, q.ID, localData.CommunityID, groups[target])
		}
		local.add(matchRank(localData.Matches, q.ID))
	}

	for _, s := range []*tally{globalRef, global, localRef, local} {
		t.Log(s)
	}
	requireBar(t, global, globalRef, globalScorecardFloor)
	requireBar(t, local, localRef, localScorecardFloor)
}
