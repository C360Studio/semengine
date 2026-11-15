package clustering

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
)

// minWordBytes is the shortest word.
const minWordBytes = 3

// stopWords are dropped from a value's words.
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
// come only from the values text selects (entitytext.Rule.Values), so keywords read the
// same text as the word index; the ID's positions, predicate names, other values and the
// object of a relationship (another entity's ID) add none.
func entityWords(text entitytext.Rule, state *graph.EntityState) []string {
	counts := map[string]int{}
	for _, value := range text.Values(state) {
		for _, w := range extractWords(value) {
			counts[w]++
		}
	}
	// The words are substrings of the values (strings.ToLower returns lower-case input
	// as is), so each kept word is copied: a short word must not keep a large value alive
	// (TestRetainedWordsDoNotPinInput).
	top := topCounted(counts, MaxEntityTerms)
	words := make([]string, len(top))
	for i, w := range top {
		words[i] = strings.Clone(w)
	}
	return words
}

// graphWords are the graph-wide counts keywords are scored against: how many members
// hold at least one word, and how many hold each word.
type graphWords struct {
	members int
	holders map[string]int
}

// distinctiveWords returns a group's keywords: the MaxKeywords words that score best, ties
// broken lexically. A word's score is the share of the group's memberCount members holding
// it (wordCounts) times ln(g.members / g.holders[word]), so a word most of the graph holds
// scores near zero however often the group uses it, and one held by every member with words
// scores zero and is never a keyword. In a group of two or more a word needs two holders: one
// member's word says nothing about the group.
func distinctiveWords(memberCount int, wordCounts map[string]int, g graphWords) []string {
	minHolders := min(2, memberCount)
	scores := make(map[string]float64, len(wordCounts))
	for w, held := range wordCounts {
		if held < minHolders {
			continue
		}
		if s := float64(held) / float64(memberCount) * math.Log(float64(g.members)/float64(g.holders[w])); s > 0 {
			scores[w] = s
		}
	}
	return topCounted(scores, MaxKeywords)
}

// newCommunity builds a group's record from its counts: the MaxKeywords most common
// types, the group's distinctive words scored against g, the summary, and the first
// MaxRepEntities of reps, which the caller passes in ascending order. revision is the
// group's mark (Community.EntityRevision).
func newCommunity(level int, id string, memberCount int, revision uint64, typeCounts, wordCounts map[string]int, g graphWords, reps []string) *Community {
	types := topCounted(typeCounts, MaxKeywords)
	kept := make(map[string]int, len(types))
	for _, typ := range types {
		kept[typ] = typeCounts[typ]
	}
	keywords := distinctiveWords(memberCount, wordCounts, g)
	return &Community{
		ID:                 id,
		Level:              level,
		MemberCount:        memberCount,
		EntityRevision:     revision,
		TypeCounts:         kept,
		Keywords:           keywords,
		StatisticalSummary: summarize(memberCount, types, typeCounts, keywords),
		RepEntities:        slices.Clone(reps[:min(len(reps), MaxRepEntities)]),
	}
}

// summarize is the summary template: the member count, up to three types with their
// counts, and up to five keywords.
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

// topCounted returns the at most n keys with the highest counts or scores, highest first,
// ties broken lexically. It keeps only the n best while it reads the counts, so a group with
// v distinct words costs O(v log n) comparisons, not a sort of all v.
func topCounted[V int | float64](counts map[string]V, n int) []string {
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
