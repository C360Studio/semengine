package clustering

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/lexical"
	"github.com/c360studio/semengine/message"
)

// The keyword scorecard's floors: the first run of the distinctive-word rule (7.9% common,
// 75.7% precision) given one point, each better than the most-counted rule it replaced
// (27.9% common, 66.7% precision on this corpus and grouping). A change that crosses one is
// a change in keyword quality.
const (
	// commonCeiling is the largest share of keywords that may be common words: words held
	// by commonShare or more of all sections.
	commonCeiling = 0.089
	// precisionFloor is the least mean share, over groups of at least precisionMinMembers
	// members, of the first ten word-index hits for a group's keywords that are the group's
	// own sections.
	precisionFloor = 0.747
)

// commonShare is the share of all sections that makes a word common.
const commonShare = 0.20

// precisionMinMembers is the smallest group whose keyword search is measured: a group of
// fewer than ten cannot fill ten hits.
const precisionMinMembers = 10

// scorecardSection is one entity of the word index's frozen scorecard corpus.
type scorecardSection struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	Name string `json:"name"`
	Body string `json:"body"`
}

func loadScorecardCorpus(t *testing.T) []scorecardSection {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "lexical", "testdata", "scorecard", "corpus.jsonl.gz"))
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
	if len(corpus) != 2000 {
		t.Fatalf("corpus holds %d sections, want 2000", len(corpus))
	}
	return corpus
}

var notIDLetter = regexp.MustCompile(`[^a-z0-9]+`)

// directoryGroup names a section's group after its file's parent directory, at most its
// last element when the directory is deeper than three, so each spec and each archived
// change is a group of its own. The name is one entity-ID position.
func directoryGroup(path string) string {
	parts := strings.Split(filepath.Dir(path), "/")
	if len(parts) > 3 {
		parts = parts[len(parts)-1:]
	}
	if g := notIDLetter.ReplaceAllString(strings.ToLower(strings.Join(parts, "-")), ""); g != "" {
		return g
	}
	return "root"
}

// referenceKeywords is a plain TF-IDF over groups, written here from the textbook score,
// not from terms.go: every distinct word of every section counts, a word's score in a group
// is the share of the group's sections holding it times the log of all sections over the
// sections holding it, ties lexical, and a word needs two holders in a group of two or more.
func referenceKeywords(groupOf []string, words []map[string]bool) map[string][]string {
	df := map[string]int{}
	inGroup := map[string]map[string]int{}
	size := map[string]int{}
	for i, ws := range words {
		g := groupOf[i]
		size[g]++
		if inGroup[g] == nil {
			inGroup[g] = map[string]int{}
		}
		for w := range ws {
			df[w]++
			inGroup[g][w]++
		}
	}
	n := float64(len(words))
	out := map[string][]string{}
	for g, counts := range inGroup {
		type scored struct {
			w string
			s float64
		}
		var all []scored
		for w, held := range counts {
			if size[g] >= 2 && held < 2 {
				continue
			}
			all = append(all, scored{w, float64(held) / float64(size[g]) * math.Log(n/float64(df[w]))})
		}
		slices.SortFunc(all, func(a, b scored) int {
			if a.s != b.s {
				if a.s > b.s {
					return -1
				}
				return 1
			}
			return strings.Compare(a.w, b.w)
		})
		for _, sw := range all[:min(len(all), MaxKeywords)] {
			out[g] = append(out[g], sw.w)
		}
	}
	return out
}

// keywordQuality is what the scorecard measures of one set of group keywords.
type keywordQuality struct {
	common    float64 // share of all keywords that are common words
	precision float64 // mean share of a group's keyword-search top ten inside the group
	measured  int     // groups whose search was measured
}

func measureKeywords(t *testing.T, keywords map[string][]string, groupOf []string, words []map[string]bool,
	size map[string]int, ix *lexical.Index, corpusIDs map[string]int) keywordQuality {
	t.Helper()
	df := map[string]int{}
	for _, ws := range words {
		for w := range ws {
			df[w]++
		}
	}
	var q keywordQuality
	total, common := 0, 0
	var precisionSum float64
	for _, g := range slices.Sorted(maps.Keys(keywords)) {
		for _, w := range keywords[g] {
			total++
			if float64(df[w]) >= commonShare*float64(len(words)) {
				common++
			}
		}
		if size[g] < precisionMinMembers || len(keywords[g]) == 0 {
			continue
		}
		query, err := lexical.ParseQuery(strings.Join(keywords[g], " "))
		if err != nil {
			t.Fatalf("group %s keywords %q: %v", g, keywords[g], err)
		}
		res, err := ix.Search(query, lexical.Options{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Hits) == 0 {
			t.Fatalf("group %s keywords %q find nothing", g, keywords[g])
		}
		in := 0
		for _, h := range res.Hits {
			if groupOf[corpusIDs[h.EntityID]] == g {
				in++
			}
		}
		precisionSum += float64(in) / float64(len(res.Hits))
		q.measured++
	}
	q.common = float64(common) / float64(max(total, 1))
	q.precision = precisionSum / float64(max(q.measured, 1))
	return q
}

// TestKeywordsScorecard publishes the word index's scorecard corpus, each section an entity
// whose level-0 group is its file's parent directory, and measures the published keywords
// against a plain TF-IDF over all of every section's words: the share of keywords that are
// common words, and how many of the first ten hits a search for a group's keywords finds
// inside the group. Each must hold its floor.
func TestKeywordsScorecard(t *testing.T) {
	corpus := loadScorecardCorpus(t)
	m, kv := newTestMaintainer(t)
	ix := lexical.NewIndex()
	groupOf := make([]string, len(corpus))
	words := make([]map[string]bool, len(corpus))
	corpusIDs := map[string]int{}
	size := map[string]int{}
	for i, s := range corpus {
		instance := s.ID[strings.LastIndexByte(s.ID, '.')+1:]
		g := directoryGroup(s.Path)
		id := "c360.semengine." + g + ".markdown.section." + instance
		groupOf[i], corpusIDs[id] = g, i
		size[g]++
		words[i] = map[string]bool{}
		for _, w := range extractWords(s.Name + " " + s.Body) {
			words[i][w] = true
		}
		state := &graph.EntityState{ID: id, Triples: []message.Triple{
			{Subject: id, Predicate: "doc.section.title", Object: s.Name},
			{Subject: id, Predicate: "doc.section.content", Object: s.Body},
		}}
		if err := m.Upsert(id, uint64(i+1), state); err != nil {
			t.Fatal(err)
		}
		if err := ix.Put(id, lexical.Document{Name: s.Name, Body: s.Body}); err != nil {
			t.Fatal(err)
		}
	}
	m.BootstrapDone()
	mustTick(t, m)

	published := map[string][]string{}
	for key, r := range kv.records(t) {
		if g, ok := strings.CutPrefix(key, "0.c360.semengine."); ok {
			published[strings.TrimSuffix(g, ".markdown")] = r.Keywords
		}
	}
	if len(published) != len(size) {
		t.Fatalf("%d level-0 records, want one per group (%d)", len(published), len(size))
	}

	got := measureKeywords(t, published, groupOf, words, size, ix, corpusIDs)
	ref := measureKeywords(t, referenceKeywords(groupOf, words), groupOf, words, size, ix, corpusIDs)
	t.Logf("%d sections, %d groups, %d searched", len(corpus), len(size), got.measured)
	t.Logf("published keywords: common %.1f%%, precision %.1f%%", 100*got.common, 100*got.precision)
	t.Logf("reference TF-IDF:   common %.1f%%, precision %.1f%%", 100*ref.common, 100*ref.precision)
	if got.common > commonCeiling {
		t.Errorf("common keywords = %.1f%%, above the ceiling of %.1f%%", 100*got.common, 100*commonCeiling)
	}
	if got.precision < precisionFloor {
		t.Errorf("keyword-search precision = %.1f%%, below the floor of %.1f%%", 100*got.precision, 100*precisionFloor)
	}
}
