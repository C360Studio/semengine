package clustering

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/message"
)

// defaultText is the text rule a Maintainer given no suffixes reads under.
var defaultText = entitytext.New(nil, 0)

// TestExtractTerms checks the word definition on each kind of input: separators, stop
// words, short and long runs, case and repeats.
func TestExtractTerms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"simple text", "autonomous navigation system", []string{"autonomous", "navigation", "system"}},
		{"stop words", "the drone is flying", []string{"drone", "flying"}},
		{"hyphenated terms", "path-planning algorithm", []string{"path", "planning", "algorithm"}},
		{"short terms filtered", "a big AI system", []string{"big", "system"}},
		{"upper case folds", "Hydraulic PUMP", []string{"hydraulic", "pump"}},
		{"repeats kept", "pump pump", []string{"pump", "pump"}},
		{"non-ASCII separates", "café/über", []string{"caf", "ber"}},
		{"32 bytes is a word", strings.Repeat("a", 32), []string{strings.Repeat("a", 32)}},
		{"33 bytes is not", strings.Repeat("a", 33) + " pump", []string{"pump"}},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractWords(tt.input); !slices.Equal(got, tt.want) {
				t.Fatalf("extractWords(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestLongLiteralIsNotAWord: ten members whose string values hold 120 KB alphanumeric
// literals give no keyword from them, and the record stays within MaxRecordBytes. A run
// longer than MaxWordBytes is not a word and is never truncated into one.
func TestLongLiteralIsNotAWord(t *testing.T) {
	wordCounts := map[string]int{}
	typeCounts := map[string]int{}
	var reps []string
	for i := range 10 {
		id := fmt.Sprintf("acme.dep1.src.docs.blob.b%d", i)
		literal := strings.Repeat(string(rune('a'+i)), 120<<10)
		state := &graph.EntityState{ID: id, Triples: []message.Triple{
			{Subject: id, Predicate: "docs.blob.body", Object: literal},
			{Subject: id, Predicate: "docs.blob.title", Object: "release notes " + literal},
		}}
		words := entityWords(defaultText, state)
		if !slices.Equal(words, []string{"notes", "release"}) {
			t.Fatalf("entityWords(%s) = %q, want [notes release]", id, words)
		}
		for _, w := range words {
			wordCounts[w]++
		}
		typeCounts["blob"]++
		reps = append(reps, id)
	}

	// The graph holds ten more members, without these words, so both score alike.
	graphCounts := graphWords{members: 20, holders: wordCounts}
	record := newCommunity(0, "acme.dep1.src.docs", 10, 1, typeCounts, wordCounts, graphCounts, reps)
	if !slices.Equal(record.Keywords, []string{"notes", "release"}) {
		t.Fatalf("keywords = %q, want [notes release]", record.Keywords)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > MaxRecordBytes {
		t.Fatalf("record encodes to %d bytes, above MaxRecordBytes %d", len(encoded), MaxRecordBytes)
	}
}

// TestRecordIsBounded builds the worst-case record: more than MaxKeywords types whose
// type positions are 246 bytes (the longest a 256-byte ID allows), more than
// MaxKeywords 32-byte words, more than MaxRepEntities 256-byte representatives, a
// 254-byte group ID, the largest member count and the largest entity revision. Its
// encoding stays within MaxRecordBytes and every list stays at its bound.
func TestRecordIsBounded(t *testing.T) {
	groupID := strings.Repeat("o", 248) + ".p.s.d" // a level-0 prefix of a 256-byte ID
	typeCounts := map[string]int{}
	for i := range MaxKeywords + 1 {
		typeCounts[strings.Repeat(string(rune('a'+i)), 246)] = math.MaxInt - i
	}
	wordCounts := map[string]int{}
	graphCounts := graphWords{members: math.MaxInt, holders: map[string]int{}}
	for i := range MaxKeywords + 1 {
		w := strings.Repeat(string(rune('a'+i)), MaxWordBytes)
		wordCounts[w] = 1<<40 - i
		graphCounts.holders[w] = 1 << 41
	}
	var reps []string
	for i := range MaxRepEntities + 1 {
		id := fmt.Sprintf("o.p.s.d.t.%d", i)
		reps = append(reps, id+strings.Repeat("x", 256-len(id))) // a 256-byte entity ID
	}

	record := newCommunity(0, groupID, math.MaxInt, math.MaxUint64, typeCounts, wordCounts, graphCounts, reps)
	if record.EntityRevision != math.MaxUint64 {
		t.Fatalf("record carries entity revision %d, want %d", record.EntityRevision, uint64(math.MaxUint64))
	}
	if len(record.TypeCounts) != MaxKeywords {
		t.Fatalf("record keeps %d types, want %d", len(record.TypeCounts), MaxKeywords)
	}
	if len(record.Keywords) != MaxKeywords {
		t.Fatalf("record keeps %d keywords, want %d", len(record.Keywords), MaxKeywords)
	}
	if !slices.Equal(record.RepEntities, reps[:MaxRepEntities]) {
		t.Fatalf("representatives = %q, want the first %d given", record.RepEntities, MaxRepEntities)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > MaxRecordBytes {
		t.Fatalf("worst-case record encodes to %d bytes, above MaxRecordBytes %d", len(encoded), MaxRecordBytes)
	}
}

// TestKeywordsIgnoreIDAndPredicates: every member shares its ID positions and predicate
// names, and one carries a relationship to another entity; only words in string values
// count, so the keywords are exactly the words of the members' text, in the record built
// from the counts and in every record a Maintainer publishes for them. One more entity, in
// another deployment and without those words, keeps them from being held by the whole graph.
func TestKeywordsIgnoreIDAndPredicates(t *testing.T) {
	m, kv := newTestMaintainer(t)
	wordCounts := map[string]int{}
	for i := range 3 {
		id := fmt.Sprintf("acme.dep1.src.robotics.drone.d%d", i)
		state := &graph.EntityState{ID: id, Triples: []message.Triple{
			{Subject: id, Predicate: "robotics.flight.altitude", Object: 120.5},
			{Subject: id, Predicate: "robotics.flight.description", Object: "hovering steady"},
			{Subject: id, Predicate: "robotics.link.parent", Object: "acme.dep1.src.robotics.base.b1"},
		}}
		for _, w := range entityWords(defaultText, state) {
			wordCounts[w]++
		}
		if err := m.Upsert(id, 1, state); err != nil {
			t.Fatalf("Upsert(%s): %v", id, err)
		}
	}
	other := "acme.dep2.src.docs.note.n0"
	mustUpsert(t, m, other, 1, "unrelated")
	want := []string{"hovering", "steady"}
	graphCounts := graphWords{members: 4, holders: wordCounts}
	record := newCommunity(0, "acme.dep1.src.robotics", 3, 1, map[string]int{"drone": 3}, wordCounts, graphCounts, nil)
	if !slices.Equal(record.Keywords, want) {
		t.Fatalf("keywords = %q, want %q", record.Keywords, want)
	}

	m.BootstrapDone()
	if err := m.Tick(t.Context()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	records := kv.records(t)
	if len(records) != 6 {
		t.Fatalf("published %d records, want 6", len(records))
	}
	for _, key := range groupKeys("acme.dep1.src.robotics.drone.d0") {
		r := records[key]
		if r == nil || !slices.Equal(r.Keywords, want) {
			t.Fatalf("published %s keywords = %q, want %q", key, r.Keywords, want)
		}
	}
}

// TestEntityContributionIsCapped: an entity contributes its MaxEntityTerms most frequent
// words, ties broken lexically.
func TestEntityContributionIsCapped(t *testing.T) {
	// w00 appears 40 times, w01 39 times, ..., w39 once, and v31 ties w31 at nine. The
	// 32 slots go to w00..w30 and then v31, the lexically smaller of the tie at nine.
	var text []string
	var want []string
	for i := range 40 {
		w := fmt.Sprintf("w%02d", i)
		for range 40 - i {
			text = append(text, w)
		}
		if i < 31 {
			want = append(want, w)
		}
	}
	for range 9 {
		text = append(text, "v31")
	}
	want = append(want, "v31")
	id := "acme.dep1.src.docs.note.n1"
	state := &graph.EntityState{ID: id, Triples: []message.Triple{
		{Subject: id, Predicate: "docs.note.body", Object: strings.Join(text, " ")},
	}}
	if got := entityWords(defaultText, state); !slices.Equal(got, want) {
		t.Fatalf("entityWords = %q, want %q", got, want)
	}

	id = "acme.dep1.src.docs.note.n2"
	tied := &graph.EntityState{ID: id, Triples: []message.Triple{
		{Subject: id, Predicate: "docs.note.body", Object: "zeta alpha mid"},
	}}
	if got, want := entityWords(defaultText, tied), []string{"alpha", "mid", "zeta"}; !slices.Equal(got, want) {
		t.Fatalf("tied entityWords = %q, want %q", got, want)
	}
}

// TestSummaryTemplate checks the exact text the summary template gives, with lexical
// tie-breaks.
func TestSummaryTemplate(t *testing.T) {
	// Of ten graph members, robotics scores 3/5*ln(10/3) and the other two tie at 2/5*ln(10/2).
	words := map[string]int{"robotics": 3, "autonomous": 2, "navigation": 2}
	record := newCommunity(0, "c360.platform.robotics.system", 5, 1,
		map[string]int{"drone": 3, "sensor": 2}, words, graphWords{members: 10, holders: words}, nil)
	want := "Community of 5 entities including 3 drones, 2 sensors. Key themes: robotics, autonomous, navigation."
	if record.StatisticalSummary != want {
		t.Fatalf("summary = %q, want %q", record.StatisticalSummary, want)
	}

	tiedWords := map[string]int{"f": 2, "e": 2, "d": 2, "c": 2, "b": 2, "a": 2}
	tied := newCommunity(1, "c360.platform.robotics", 4, 1,
		map[string]int{"wheel": 1, "arm": 1, "base": 1, "cam": 1}, tiedWords, graphWords{members: 8, holders: tiedWords}, nil)
	want = "Community of 4 entities including 1 arm, 1 base, 1 cam. Key themes: a, b, c, d, e."
	if tied.StatisticalSummary != want {
		t.Fatalf("tied summary = %q, want %q", tied.StatisticalSummary, want)
	}
	if want := map[string]int{"wheel": 1, "arm": 1, "base": 1, "cam": 1}; !maps.Equal(tied.TypeCounts, want) {
		t.Fatalf("type counts = %v, want %v", tied.TypeCounts, want)
	}

	bare := newCommunity(2, "c360.platform", 1, 1, nil, nil, graphWords{}, nil)
	if want := "Community of 1 entities."; bare.StatisticalSummary != want {
		t.Fatalf("bare summary = %q, want %q", bare.StatisticalSummary, want)
	}
}

// TestKeywordTextMatchesLexical: an entity's keywords are counted from the values graph-lexical
// reads as its text under the same suffixes, and from nothing else. The entity holds values
// under configured and unconfigured suffixes, a number and a relationship; each text value is
// one word held once, so the words the maintainer publishes are exactly those values, lexically
// ordered. A second member, smaller by ID, holds a string only under a suffix no case selects: it
// adds no keyword and is no representative. The expected values come from entitytext built as
// graph-lexical builds it.
func TestKeywordTextMatchesLexical(t *testing.T) {
	id := "acme.dep1.src.code.func.f1"
	state := &graph.EntityState{ID: id, Triples: []message.Triple{
		{Subject: id, Predicate: "code.func.name", Object: "parse"},
		{Subject: id, Predicate: "code.doc.comment", Object: "decoder"},
		{Subject: id, Predicate: "code.func.signature", Object: "reader"},
		{Subject: id, Predicate: "code.func.language", Object: "golang"},
		{Subject: id, Predicate: "code.func.body", Object: "buffer"},
		{Subject: id, Predicate: "code.func.lines", Object: 12.0},
		{Subject: id, Predicate: "code.link.caller", Object: "acme.dep1.src.code.func.main"},
	}}
	for name, suffixes := range map[string][]string{
		"default":   nil,
		"semsource": append(entitytext.DefaultSuffixes(), ".signature", ".comment"),
		"custom":    {".language"},
	} {
		t.Run(name, func(t *testing.T) {
			want := entitytext.New(suffixes, 16000).Values(state)
			slices.Sort(want)
			if len(want) == 0 {
				t.Fatal("the rule selects no value; the case proves nothing")
			}

			m, kv := newTextMaintainer(t, suffixes)
			if err := m.Upsert(id, 1, state); err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			const untexted = "acme.dep1.src.code.func.a0"
			if err := m.Upsert(untexted, 1, &graph.EntityState{ID: untexted, Triples: []message.Triple{
				{Subject: untexted, Predicate: "code.func.status", Object: "deprecated"},
			}}); err != nil {
				t.Fatalf("Upsert(%s): %v", untexted, err)
			}
			m.BootstrapDone()
			mustTick(t, m)
			for key, r := range kv.records(t) {
				if !slices.Equal(r.Keywords, want) {
					t.Fatalf("%s keywords = %q, want the text values %q", key, r.Keywords, want)
				}
				if !slices.Equal(r.RepEntities, []string{id}) {
					t.Fatalf("%s representatives = %q, want [%s]", key, r.RepEntities, id)
				}
			}
		})
	}
}
