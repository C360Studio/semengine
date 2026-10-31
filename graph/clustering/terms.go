package clustering

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/c360studio/semengine/graph"
)

// minWordBytes is the shortest word, as at the pin (summarizer.go:483).
const minWordBytes = 3

// stopWords are dropped from a value's words; the pin's list (summarizer.go:474-479).
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "as": true, "is": true, "was": true,
	"are": true, "be": true, "has": true, "have": true, "had": true, "do": true,
	"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
}

// extractWords returns the words of text in order, repeats kept: each run of ASCII
// letters and digits, lower-cased, of minWordBytes to MaxWordBytes bytes, that is not a
// stop word. A longer run is not a word and is never cut into one, so a long literal such
// as an encoded blob adds nothing.
func extractWords(text string) []string {
	var words []string
	for i := 0; i < len(text); {
		if !isWordByte(text[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(text) && isWordByte(text[j]) {
			j++
		}
		if n := j - i; n >= minWordBytes && n <= MaxWordBytes {
			if w := strings.ToLower(text[i:j]); !stopWords[w] {
				words = append(words, w)
			}
		}
		i = j
	}
	return words
}

func isWordByte(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9'
}

// entityWords returns what an entity contributes to its groups' keywords: its
// MaxEntityTerms most frequent words, most frequent first, ties broken lexically. Words
// come only from string values; the ID's positions, predicate names and the object of a
// relationship (another entity's ID) add none, unlike the pin (summarizer.go:136-163).
func entityWords(state *graph.EntityState) []string {
	counts := map[string]int{}
	for _, triple := range state.Triples {
		if triple.IsRelationship() {
			continue
		}
		if value, ok := triple.Object.(string); ok {
			for _, w := range extractWords(value) {
				counts[w]++
			}
		}
	}
	return topCounted(counts, MaxEntityTerms)
}

// newCommunity builds a group's record from its counts: the MaxKeywords most common
// types and words, the summary, and the first MaxRepEntities of reps, which the caller
// passes in ascending order.
func newCommunity(level int, id string, memberCount int, typeCounts, wordCounts map[string]int, reps []string) *Community {
	types := topCounted(typeCounts, MaxKeywords)
	kept := make(map[string]int, len(types))
	for _, typ := range types {
		kept[typ] = typeCounts[typ]
	}
	keywords := topCounted(wordCounts, MaxKeywords)
	return &Community{
		ID:                 id,
		Level:              level,
		MemberCount:        memberCount,
		TypeCounts:         kept,
		Keywords:           keywords,
		StatisticalSummary: summarize(memberCount, types, typeCounts, keywords),
		RepEntities:        slices.Clone(reps[:min(len(reps), MaxRepEntities)]),
	}
}

// summarize is the pin's summary template (summarizer.go:388-461): the member count, up
// to three types with their counts, and up to five keywords.
func summarize(memberCount int, types []string, typeCounts map[string]int, keywords []string) string {
	var summary strings.Builder
	fmt.Fprintf(&summary, "Community of %d entities", memberCount)
	if len(types) > 0 {
		descriptions := make([]string, 0, 3)
		for _, typ := range types[:min(len(types), 3)] {
			description := fmt.Sprintf("%d %s", typeCounts[typ], typ)
			if typeCounts[typ] > 1 {
				description += "s"
			}
			descriptions = append(descriptions, description)
		}
		summary.WriteString(" including " + strings.Join(descriptions, ", "))
	}
	if len(keywords) > 0 {
		summary.WriteString(". Key themes: " + strings.Join(keywords[:min(len(keywords), 5)], ", "))
	}
	summary.WriteString(".")
	return summary.String()
}

// topCounted returns the at most n keys with the highest counts, highest first, ties
// broken lexically. It keeps only the n best while it reads the counts, so a group with v
// distinct words costs O(v log n) comparisons, not a sort of all v.
func topCounted(counts map[string]int, n int) []string {
	before := func(a, b string) int {
		if c := cmp.Compare(counts[b], counts[a]); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	}
	if n <= 0 {
		return nil
	}
	best := make([]string, 0, min(n, len(counts)))
	for k := range counts {
		if len(best) == n && before(k, best[n-1]) > 0 {
			continue
		}
		i, _ := slices.BinarySearchFunc(best, k, before)
		best = slices.Insert(best, i, k)
		if len(best) > n {
			best = best[:n]
		}
	}
	return best
}
