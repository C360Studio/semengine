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
	"github.com/c360studio/semengine/message"
)

// TestExtractTerms is the pin's TestExtractTerms (summarizer_test.go:218), with the
// cases the 2026-10-31 word definition adds.
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
		words := entityWords(state)
		if !slices.Equal(words, []string{"notes", "release"}) {
			t.Fatalf("entityWords(%s) = %q, want [notes release]", id, words)
		}
		for _, w := range words {
			wordCounts[w]++
		}
		typeCounts["blob"]++
		reps = append(reps, id)
	}

	record := newCommunity(0, "acme.dep1.src.docs", 10, typeCounts, wordCounts, reps)
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

// TestRecordIsBounded builds the worst-case record design D4 names: more than MaxKeywords
// types whose type positions are 246 bytes (the longest a 256-byte ID allows), more than
// MaxKeywords 32-byte words, more than MaxRepEntities 256-byte representatives, a
// 254-byte group ID and the largest member count. Its encoding stays within
// MaxRecordBytes and every list stays at its bound.
func TestRecordIsBounded(t *testing.T) {
	groupID := strings.Repeat("o", 248) + ".p.s.d" // a level-0 prefix of a 256-byte ID
	typeCounts := map[string]int{}
	for i := range MaxKeywords + 1 {
		typeCounts[strings.Repeat(string(rune('a'+i)), 246)] = math.MaxInt - i
	}
	wordCounts := map[string]int{}
	for i := range MaxKeywords + 1 {
		wordCounts[strings.Repeat(string(rune('a'+i)), MaxWordBytes)] = math.MaxInt - i
	}
	var reps []string
	for i := range MaxRepEntities + 1 {
		id := fmt.Sprintf("o.p.s.d.t.%d", i)
		reps = append(reps, id+strings.Repeat("x", 256-len(id))) // a 256-byte entity ID
	}

	record := newCommunity(0, groupID, math.MaxInt, typeCounts, wordCounts, reps)
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
// count, so the keywords are exactly the words of the members' text.
func TestKeywordsIgnoreIDAndPredicates(t *testing.T) {
	wordCounts := map[string]int{}
	for i := range 3 {
		id := fmt.Sprintf("acme.dep1.src.robotics.drone.d%d", i)
		state := &graph.EntityState{ID: id, Triples: []message.Triple{
			{Subject: id, Predicate: "robotics.flight.altitude", Object: 120.5},
			{Subject: id, Predicate: "robotics.flight.status", Object: "hovering steady"},
			{Subject: id, Predicate: "robotics.link.parent", Object: "acme.dep1.src.robotics.base.b1"},
		}}
		for _, w := range entityWords(state) {
			wordCounts[w]++
		}
	}
	record := newCommunity(0, "acme.dep1.src.robotics", 3, map[string]int{"drone": 3}, wordCounts, nil)
	if want := []string{"hovering", "steady"}; !slices.Equal(record.Keywords, want) {
		t.Fatalf("keywords = %q, want %q", record.Keywords, want)
	}
}

// TestEntityContributionIsCapped: an entity contributes its MaxEntityTerms most frequent
// words, ties broken lexically (design D2, Words).
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
	if got := entityWords(state); !slices.Equal(got, want) {
		t.Fatalf("entityWords = %q, want %q", got, want)
	}

	id = "acme.dep1.src.docs.note.n2"
	tied := &graph.EntityState{ID: id, Triples: []message.Triple{
		{Subject: id, Predicate: "docs.note.body", Object: "zeta alpha mid"},
	}}
	if got, want := entityWords(tied), []string{"alpha", "mid", "zeta"}; !slices.Equal(got, want) {
		t.Fatalf("tied entityWords = %q, want %q", got, want)
	}
}

// TestSummaryTemplate is the pin's TestStatisticalSummarizer_SummaryGeneration
// (summarizer_test.go:142) with the exact text the pin's template gives
// (summarizer.go:388-461) and lexical tie-breaks.
func TestSummaryTemplate(t *testing.T) {
	record := newCommunity(0, "c360.platform.robotics.system", 5,
		map[string]int{"drone": 3, "sensor": 2},
		map[string]int{"robotics": 3, "autonomous": 2, "navigation": 1}, nil)
	want := "Community of 5 entities including 3 drones, 2 sensors. Key themes: robotics, autonomous, navigation."
	if record.StatisticalSummary != want {
		t.Fatalf("summary = %q, want %q", record.StatisticalSummary, want)
	}

	tied := newCommunity(1, "c360.platform.robotics", 4,
		map[string]int{"wheel": 1, "arm": 1, "base": 1, "cam": 1},
		map[string]int{"f": 1, "e": 1, "d": 1, "c": 1, "b": 1, "a": 1}, nil)
	want = "Community of 4 entities including 1 arm, 1 base, 1 cam. Key themes: a, b, c, d, e."
	if tied.StatisticalSummary != want {
		t.Fatalf("tied summary = %q, want %q", tied.StatisticalSummary, want)
	}
	if want := map[string]int{"wheel": 1, "arm": 1, "base": 1, "cam": 1}; !maps.Equal(tied.TypeCounts, want) {
		t.Fatalf("type counts = %v, want %v", tied.TypeCounts, want)
	}

	bare := newCommunity(2, "c360.platform", 1, nil, nil, nil)
	if want := "Community of 1 entities."; bare.StatisticalSummary != want {
		t.Fatalf("bare summary = %q, want %q", bare.StatisticalSummary, want)
	}
}
