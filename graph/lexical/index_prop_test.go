package lexical

import (
	"errors"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// The properties draw words from a closed vocabulary that tokenizing leaves
// as it is: lower case, no s or x, never ending in e, no stopword. So the
// reference model can split text on white space and needs no tokenizer.
var (
	heldVocab   = []string{"alpha", "bravo", "delta", "golf", "hotel", "india", "kilo", "lima", "papa", "quebec"}
	absentVocab = []string{"romeo", "tango", "victor", "zulu"}
	allVocab    = slices.Concat(heldVocab, absentVocab)
	propIDs     = []string{
		"c360.lex.prop.doc.item.a", "c360.lex.prop.doc.item.b", "c360.lex.prop.doc.item.c",
		"c360.lex.prop.note.item.a", "c360.lex.prop.note.item.b", "c360.lex.prop.note.item.c",
	}
	propScopes = [][]string{
		nil,
		{"c360.lex.prop.doc"},
		{"c360.lex.prop.note.item.b"},
		{"c360.lex.prop.doc", "c360.lex.prop.note.item.a"},
		{""},
	}
)

// failer is what the shared checks need from *testing.T and *rapid.T.
type failer interface {
	Fatalf(format string, args ...any)
}

// op is one write: a Put of doc, or a Delete.
type op struct {
	id     string
	delete bool
	doc    Document
}

// model is the reference: the documents the index should hold, keyed by ID,
// with BM25F worked out naively from the requirement.
type model map[string]Document

func (m model) apply(o op) {
	if o.delete || len(strings.Fields(o.doc.Name))+len(strings.Fields(o.doc.Body)) == 0 {
		delete(m, o.id)
		return
	}
	m[o.id] = o.doc
}

func count(words []string, w string) int {
	n := 0
	for _, x := range words {
		if x == w {
			n++
		}
	}
	return n
}

func modelLength(d Document) float64 {
	return float64(NameWeight*len(strings.Fields(d.Name)) + len(strings.Fields(d.Body)))
}

func modelTF(d Document, w string) float64 {
	return float64(NameWeight*count(strings.Fields(d.Name), w) + count(strings.Fields(d.Body), w))
}

func inModelScope(id string, scope []string) bool {
	if len(scope) == 0 {
		return true
	}
	for _, p := range scope {
		if p == "" || id == p || strings.HasPrefix(id, p+".") {
			return true
		}
	}
	return false
}

// modelQuery is a generated query in the model's terms: its plain words, its
// phrases and its excluded terms, each a list of vocabulary words.
type modelQuery struct {
	plain             []string
	phrases, excluded [][]string
}

func plainOnly(words ...string) modelQuery { return modelQuery{plain: words} }

// text writes the query out, its parts in the given order (nil: plain words,
// phrases, then excluded terms): plain words bare, phrases in quotes,
// excluded terms after "-", quoted when they hold more than one word.
func (mq modelQuery) text(order []int) string {
	parts := slices.Clone(mq.plain)
	for _, p := range mq.phrases {
		parts = append(parts, `"`+strings.Join(p, " ")+`"`)
	}
	for _, x := range mq.excluded {
		if len(x) == 1 {
			parts = append(parts, "-"+x[0])
		} else {
			parts = append(parts, `-"`+strings.Join(x, " ")+`"`)
		}
	}
	if order == nil {
		return strings.Join(parts, " ")
	}
	out := make([]string, 0, len(parts))
	for _, i := range order {
		out = append(out, parts[i])
	}
	return strings.Join(out, " ")
}

func (mq modelQuery) parts() int { return len(mq.plain) + len(mq.phrases) + len(mq.excluded) }

// scored is the distinct words the requirement scores: plain and phrase words.
func (mq modelQuery) scored() []string {
	return distinct(slices.Concat(mq.plain, slices.Concat(mq.phrases...)))
}

// fieldHolds reports run standing word after word in field, compared as
// space-joined text.
func fieldHolds(field string, run []string) bool {
	return strings.Contains(" "+strings.Join(strings.Fields(field), " ")+" ", " "+strings.Join(run, " ")+" ")
}

func docHolds(d Document, run []string) bool {
	return fieldHolds(d.Name, run) || fieldHolds(d.Body, run)
}

// admits is the requirement's filter: every phrase held, no excluded term.
func (mq modelQuery) admits(d Document) bool {
	for _, p := range mq.phrases {
		if !docHolds(d, p) {
			return false
		}
	}
	for _, x := range mq.excluded {
		if docHolds(d, x) {
			return false
		}
	}
	return true
}

// scores gives every entity in scope that holds a query word its BM25F score.
func (m model) scores(words []string, scope []string) map[string]float64 {
	out := map[string]float64{}
	if len(m) == 0 {
		return out
	}
	n := float64(len(m))
	total := 0.0
	for _, d := range m {
		total += modelLength(d)
	}
	avg := total / n
	for _, w := range words {
		df := 0.0
		for _, d := range m {
			if modelTF(d, w) > 0 {
				df++
			}
		}
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for id, d := range m {
			tf := modelTF(d, w)
			if tf == 0 || !inModelScope(id, scope) {
				continue
			}
			out[id] += idf * tf * 2.2 / (tf + 1.2*(0.25+0.75*modelLength(d)/avg))
		}
	}
	return out
}

// ranked orders the model's scores by score, then ID.
func ranked(scores map[string]float64) []Hit {
	hits := make([]Hit, 0, len(scores))
	for id, s := range scores {
		hits = append(hits, Hit{id, s})
	}
	slices.SortFunc(hits, func(a, b Hit) int {
		if a.Score != b.Score {
			if a.Score > b.Score {
				return -1
			}
			return 1
		}
		return strings.Compare(a.EntityID, b.EntityID)
	})
	return hits
}

func distinct(words []string) []string {
	var out []string
	for _, w := range words {
		if !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

func applyAll(t failer, ix *Index, m model, ops []op) {
	for _, o := range ops {
		if o.delete {
			ix.Delete(o.id)
		} else if err := ix.Put(o.id, o.doc); err != nil {
			t.Fatalf("Put(%q, %+v): %v", o.id, o.doc, err)
		}
		if m != nil {
			m.apply(o)
		}
	}
}

// checkAgainstModel runs one query on the index and compares it with the
// model: a refusal exactly when the query has no plain word and no phrase,
// the same count of matches, the model's score for each hit within 1e-9, and
// at each position the model's score at that position, so the order is the
// model's wherever two scores differ by more than that.
func checkAgainstModel(t failer, ix *Index, m model, mq modelQuery, order []int, opts Options) {
	text := mq.text(order)
	if ix.Len() != len(m) {
		t.Fatalf("Len() = %d, model holds %d", ix.Len(), len(m))
	}
	q, err := ParseQuery(text)
	if len(mq.plain) == 0 && len(mq.phrases) == 0 {
		if !errors.Is(err, ErrEmptyQuery) {
			t.Fatalf("ParseQuery(%q) = %+v, %v; want ErrEmptyQuery", text, q, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", text, err)
	}
	res, err := ix.Search(q, opts)
	if err != nil {
		t.Fatalf("Search(%q, %+v): %v", text, opts, err)
	}
	scores := m.scores(mq.scored(), opts.Scope)
	for id := range scores {
		if !mq.admits(m[id]) {
			delete(scores, id)
		}
	}
	want := ranked(scores)
	if res.Matched != len(want) {
		t.Fatalf("Search(%q, %+v): Matched = %d, model %d (%v)", text, opts, res.Matched, len(want), want)
	}
	if len(res.Hits) != min(opts.Limit, len(want)) {
		t.Fatalf("Search(%q, %+v): %d hits, model %v", text, opts, len(res.Hits), want)
	}
	seen := map[string]bool{}
	for i, h := range res.Hits {
		modelScore, ok := scores[h.EntityID]
		if !ok || seen[h.EntityID] {
			t.Fatalf("Search(%q, %+v): hit %d %v is not a model match once (model %v)", text, opts, i, h, want)
		}
		seen[h.EntityID] = true
		if math.Abs(h.Score-modelScore) > 1e-9 || math.Abs(h.Score-want[i].Score) > 1e-9 {
			t.Fatalf("Search(%q, %+v): hit %d = %v, model ranks %v there and scores it %v",
				text, opts, i, h, want[i], modelScore)
		}
	}
}

func genDoc(t *rapid.T, label string) Document {
	name := rapid.SliceOfN(rapid.SampledFrom(heldVocab), 0, 3).Draw(t, label+" name")
	body := rapid.SliceOfN(rapid.SampledFrom(heldVocab), 0, 8).Draw(t, label+" body")
	return Document{Name: strings.Join(name, " "), Body: strings.Join(body, " ")}
}

func genOps(t *rapid.T, ids []string, label string) []op {
	return rapid.SliceOfN(rapid.Custom(func(t *rapid.T) op {
		o := op{id: rapid.SampledFrom(ids).Draw(t, "id")}
		if rapid.IntRange(0, 4).Draw(t, "kind") == 0 {
			o.delete = true
		} else {
			o.doc = genDoc(t, "doc")
		}
		return o
	}), 0, 20).Draw(t, label)
}

func genOptions(t *rapid.T) Options {
	return Options{
		Scope: rapid.SampledFrom(propScopes).Draw(t, "scope"),
		Limit: rapid.IntRange(1, 8).Draw(t, "limit"),
	}
}

// genRun draws 1 to maxLen words: half the time a run that stands in a held
// field, so phrases and exclusions often match, otherwise any vocabulary
// words.
func genRun(t *rapid.T, m model, maxLen int, label string) []string {
	if len(m) > 0 && rapid.Bool().Draw(t, label+" from a held field") {
		d := m[rapid.SampledFrom(slices.Sorted(maps.Keys(m))).Draw(t, label+" entity")]
		field := strings.Fields(d.Body)
		if rapid.Bool().Draw(t, label+" in name") || len(field) == 0 {
			field = strings.Fields(d.Name)
		}
		if len(field) > 0 {
			start := rapid.IntRange(0, len(field)-1).Draw(t, label+" start")
			n := rapid.IntRange(1, min(maxLen, len(field)-start)).Draw(t, label+" length")
			return field[start : start+n]
		}
	}
	return rapid.SliceOfN(rapid.SampledFrom(allVocab), 1, maxLen).Draw(t, label+" words")
}

func genQuery(t *rapid.T, m model) (modelQuery, []int) {
	var mq modelQuery
	for range rapid.IntRange(0, 4).Draw(t, "plain words") {
		mq.plain = append(mq.plain, rapid.SampledFrom(allVocab).Draw(t, "plain"))
	}
	for range rapid.IntRange(0, 2).Draw(t, "phrases") {
		mq.phrases = append(mq.phrases, genRun(t, m, 3, "phrase"))
	}
	for range rapid.IntRange(0, 2).Draw(t, "exclusions") {
		mq.excluded = append(mq.excluded, genRun(t, m, 2, "excluded"))
	}
	order := make([]int, mq.parts())
	for i := range order {
		order[i] = i
	}
	return mq, rapid.Permutation(order).Draw(t, "order")
}

func TestPropIndexMatchesModel(t *testing.T) {
	a, b, c := propIDs[0], propIDs[1], propIDs[3]
	witnesses := []struct {
		name string
		ops  []op
		mq   modelQuery
		opts Options
	}{
		{"replaced document", []op{
			{id: a, doc: Document{Name: "alpha", Body: "bravo bravo"}},
			{id: a, doc: Document{Body: "delta"}},
			{id: b, doc: Document{Body: "alpha delta golf"}},
		}, plainOnly("alpha", "delta"), Options{Limit: 5}},
		{"empty document and delete", []op{
			{id: a, doc: Document{Body: "alpha"}},
			{id: b, doc: Document{Name: "alpha", Body: "bravo"}},
			{id: c, doc: Document{Body: "alpha alpha"}},
			{id: a, doc: Document{}},
			{id: b, delete: true},
		}, plainOnly("alpha", "bravo"), Options{Limit: 5}},
		{"scope and limit", []op{
			{id: a, doc: Document{Body: "alpha"}},
			{id: b, doc: Document{Body: "alpha alpha bravo"}},
			{id: c, doc: Document{Name: "alpha", Body: "alpha"}},
		}, plainOnly("alpha"), Options{Scope: []string{"c360.lex.prop.doc"}, Limit: 1}},
		{"tie broken by ID", []op{
			{id: b, doc: Document{Body: "golf hotel"}},
			{id: a, doc: Document{Body: "golf hotel"}},
		}, plainOnly("golf", "zulu"), Options{Limit: 1}},
		{"phrase together, apart and across fields", []op{
			{id: a, doc: Document{Body: "kilo alpha bravo"}},
			{id: b, doc: Document{Body: "bravo alpha kilo"}},
			{id: c, doc: Document{Name: "golf alpha", Body: "bravo"}},
		}, modelQuery{plain: []string{"kilo"}, phrases: [][]string{{"alpha", "bravo"}}}, Options{Limit: 5}},
		{"two phrases, one held", []op{
			{id: a, doc: Document{Name: "delta golf", Body: "kilo lima"}},
			{id: b, doc: Document{Body: "delta golf lima kilo"}},
		}, modelQuery{phrases: [][]string{{"delta", "golf"}, {"kilo", "lima"}}}, Options{Limit: 5}},
		{"excluded word and phrase", []op{
			{id: a, doc: Document{Body: "alpha hotel"}},
			{id: b, doc: Document{Name: "alpha", Body: "india kilo"}},
			{id: c, doc: Document{Name: "india", Body: "kilo alpha"}},
		}, modelQuery{plain: []string{"alpha"}, excluded: [][]string{{"hotel"}, {"india", "kilo"}}}, Options{Limit: 1}},
		{"phrase no entity holds", []op{
			{id: a, doc: Document{Body: "alpha bravo"}},
		}, modelQuery{plain: []string{"alpha"}, phrases: [][]string{{"bravo", "alpha"}}}, Options{Limit: 5}},
		{"word in plain and phrase scores once", []op{
			{id: a, doc: Document{Body: "alpha bravo"}},
			{id: b, doc: Document{Body: "alpha alpha bravo delta"}},
		}, modelQuery{plain: []string{"alpha"}, phrases: [][]string{{"alpha", "bravo"}}}, Options{Limit: 5}},
		{"only exclusions", []op{
			{id: a, doc: Document{Body: "alpha"}},
		}, modelQuery{excluded: [][]string{{"alpha"}}}, Options{Limit: 5}},
	}
	for _, w := range witnesses {
		t.Run(w.name, func(t *testing.T) {
			ix, m := NewIndex(), model{}
			applyAll(t, ix, m, w.ops)
			checkAgainstModel(t, ix, m, w.mq, nil, w.opts)
		})
	}

	rapid.Check(t, func(t *rapid.T) {
		ix, m := NewIndex(), model{}
		applyAll(t, ix, m, genOps(t, propIDs, "history"))
		for range rapid.IntRange(1, 4).Draw(t, "queries") {
			mq, order := genQuery(t, m)
			checkAgainstModel(t, ix, m, mq, order, genOptions(t))
		}
	})
}

// checkSameResults asks two indexes the same query and requires equal
// answers, scores compared exactly.
func checkSameResults(t failer, first, second *Index, words []string, opts Options) {
	q, err := ParseQuery(strings.Join(words, " "))
	if err != nil {
		t.Fatalf("ParseQuery(%v): %v", words, err)
	}
	r1, err1 := first.Search(q, opts)
	r2, err2 := second.Search(q, opts)
	if err1 != nil || err2 != nil {
		t.Fatalf("Search(%v, %+v): errors %v and %v", words, opts, err1, err2)
	}
	if !equalResults(r1, r2) {
		t.Fatalf("Search(%v, %+v) after two histories with the same end:\n%+v\n%+v", words, opts, r1, r2)
	}
}

// writeDirect puts final in ID order. writeRoundabout reaches the same
// entities another way: extra entities written first and deleted last, and
// each final entity written first with junk, then with its own document.
func writeDirect(t failer, final map[string]Document) *Index {
	ix := NewIndex()
	for _, id := range slices.Sorted(maps.Keys(final)) {
		applyAll(t, ix, nil, []op{{id: id, doc: final[id]}})
	}
	return ix
}

func writeRoundabout(t failer, final map[string]Document, order []string, extras, junk []Document) *Index {
	ix := NewIndex()
	for i, d := range extras {
		applyAll(t, ix, nil, []op{{id: extraID(i), doc: d}})
	}
	for i, id := range order {
		applyAll(t, ix, nil, []op{{id: id, doc: junk[i]}, {id: id, doc: final[id]}})
	}
	for i := range extras {
		applyAll(t, ix, nil, []op{{id: extraID(i), delete: true}})
	}
	return ix
}

func extraID(i int) string { return "c360.lex.prop.extra.item.x" + string(rune('a'+i)) }

func TestPropRankingIgnoresHistory(t *testing.T) {
	final := map[string]Document{
		propIDs[0]: {Name: "alpha", Body: "bravo delta"},
		propIDs[1]: {Body: "alpha alpha golf"},
		propIDs[4]: {Name: "delta golf", Body: "alpha"},
	}
	extras := []Document{{Body: "alpha alpha alpha alpha hotel"}, {Name: "golf golf", Body: "india kilo lima"}}
	junk := []Document{{Body: "papa quebec"}, {Name: "alpha"}, {Body: "delta delta delta delta"}}
	order := []string{propIDs[4], propIDs[0], propIDs[1]}
	direct, roundabout := writeDirect(t, final), writeRoundabout(t, final, order, extras, junk)
	for _, words := range [][]string{{"alpha"}, {"golf", "delta"}, {"bravo", "alpha", "golf"}} {
		checkSameResults(t, direct, roundabout, words, Options{Limit: 10})
	}

	rapid.Check(t, func(t *rapid.T) {
		ids := rapid.SliceOfNDistinct(rapid.SampledFrom(propIDs), 1, len(propIDs), rapid.ID[string]).Draw(t, "ids")
		final := map[string]Document{}
		for _, id := range ids {
			d := genDoc(t, "final")
			if d.Name == "" && d.Body == "" {
				d.Body = "alpha"
			}
			final[id] = d
		}
		order := rapid.Permutation(ids).Draw(t, "order")
		var extras, junk []Document
		for range rapid.IntRange(0, 4).Draw(t, "extras") {
			extras = append(extras, genDoc(t, "extra"))
		}
		for range order {
			junk = append(junk, genDoc(t, "junk"))
		}
		direct, roundabout := writeDirect(t, final), writeRoundabout(t, final, order, extras, junk)
		for range rapid.IntRange(1, 4).Draw(t, "queries") {
			words := rapid.SliceOfN(rapid.SampledFrom(heldVocab), 1, 4).Draw(t, "words")
			checkSameResults(t, direct, roundabout, words, genOptions(t))
		}
	})
}

// checkAbsentFindsNothing queries words no held entity holds, per the
// model, plus words never written at all.
func checkAbsentFindsNothing(t failer, ix *Index, m model, words []string) {
	q, err := ParseQuery(strings.Join(words, " "))
	if err != nil {
		t.Fatalf("ParseQuery(%v): %v", words, err)
	}
	res, err := ix.Search(q, Options{Limit: 10})
	if err != nil {
		t.Fatalf("Search(%v): %v", words, err)
	}
	if len(res.Hits) != 0 || res.Matched != 0 {
		t.Fatalf("Search(%v) over %v = %+v, want no hit", words, m, res)
	}
}

func absentFrom(m model) []string {
	var out []string
	for _, w := range allVocab {
		held := false
		for _, d := range m {
			if modelTF(d, w) > 0 {
				held = true
			}
		}
		if !held {
			out = append(out, w)
		}
	}
	return out
}

func TestPropAbsentWordsMatchNothing(t *testing.T) {
	a, b := propIDs[0], propIDs[1]
	for _, w := range []struct {
		name  string
		ops   []op
		words []string
	}{
		{"never written", []op{{id: a, doc: Document{Body: "alpha"}}}, []string{"zulu", "tango"}},
		{"deleted", []op{{id: a, doc: Document{Body: "alpha"}}, {id: a, delete: true}}, []string{"alpha"}},
		{"replaced", []op{{id: a, doc: Document{Name: "alpha"}}, {id: a, doc: Document{Body: "bravo"}}}, []string{"alpha"}},
		{"emptied", []op{
			{id: a, doc: Document{Body: "alpha"}}, {id: b, doc: Document{Body: "bravo"}}, {id: a, doc: Document{}},
		}, []string{"alpha"}},
	} {
		t.Run(w.name, func(t *testing.T) {
			ix, m := NewIndex(), model{}
			applyAll(t, ix, m, w.ops)
			checkAbsentFindsNothing(t, ix, m, w.words)
		})
	}

	rapid.Check(t, func(t *rapid.T) {
		ix, m := NewIndex(), model{}
		applyAll(t, ix, m, genOps(t, propIDs, "history"))
		// absentVocab is never written, so there is always a word to draw.
		words := rapid.SliceOfN(rapid.SampledFrom(absentFrom(m)), 1, 4).Draw(t, "words")
		checkAbsentFindsNothing(t, ix, m, words)
	})
}
