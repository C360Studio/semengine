// Package entitytext says which of an entity's values are its text, for every component that
// reads text from the graph: graph-embedding embeds it, and a word index splits it into a name and
// a body. It also reads an entity's offloaded body from its store, under a deadline. It imports no
// NATS client: a store is reached only through the storage interfaces.
package entitytext

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
)

// defaultSuffixes are the predicate suffixes whose values are text when none are configured.
var defaultSuffixes = []string{".title", ".content", ".description", ".summary", ".text", ".name", ".body",
	".abstract", ".subject"}

// nameSuffixes mark a selected value as part of an entity's name.
var nameSuffixes = []string{".name", ".title"}

// DefaultSuffixes returns the predicate suffixes a Rule uses when it is given none.
func DefaultSuffixes() []string {
	return append([]string(nil), defaultSuffixes...)
}

// Rule says which of an entity's values are its text. A value is text when it is a non-empty
// string, is not a relationship, and its predicate, lower-cased, ends in one of the rule's
// suffixes. The zero Rule selects nothing; build one with New.
type Rule struct {
	suffixes []string
	labels   map[string]int
	maxLen   int
}

// New returns the rule for suffixes, or DefaultSuffixes when suffixes is empty. maxLen is the
// most characters Document's Body holds; 0 or less means no cap. The vocabulary's label
// predicates are read once, here, as the name index reads them, so a product registers its own
// before it builds the components that read text.
func New(suffixes []string, maxLen int) Rule {
	if len(suffixes) == 0 {
		suffixes = defaultSuffixes
	}
	return Rule{
		suffixes: append([]string(nil), suffixes...),
		labels:   vocabulary.DiscoverLabelPredicates(),
		maxLen:   maxLen,
	}
}

// Values returns the entity's text values in triple order, one for each text triple.
func (r Rule) Values(state *graph.EntityState) []string {
	var values []string
	for _, triple := range state.Triples {
		value, ok := literal(triple)
		if ok && hasSuffix(strings.ToLower(triple.Predicate), r.suffixes) {
			values = append(values, value)
		}
	}
	return values
}

// Document is an entity's text split in two. Name holds the values that name the entity; Body
// holds the rest, then the offloaded body. Cut reports that Body was cut at the rule's cap.
type Document struct {
	Name string
	Body string
	Cut  bool
}

// Document splits the entity's text into a name and a body. Name is every string value of a
// vocabulary label predicate, whatever its suffix, and every text value whose predicate ends in
// .name or .title, each value once and in triple order. Body is every other text value in triple
// order, then body, the entity's offloaded body ("" for none). The cap cuts Body's tail as Truncate
// does and never cuts Name.
func (r Rule) Document(state *graph.EntityState, body string) Document {
	var names, rest []string
	for _, triple := range state.Triples {
		value, ok := literal(triple)
		if !ok {
			continue
		}
		predicate := strings.ToLower(triple.Predicate)
		_, label := r.labels[triple.Predicate]
		selected := hasSuffix(predicate, r.suffixes)
		switch {
		case label, selected && hasSuffix(predicate, nameSuffixes):
			names = append(names, value)
		case selected:
			rest = append(rest, value)
		}
	}
	if body != "" {
		rest = append(rest, body)
	}
	full := strings.Join(rest, " ")
	capped := Truncate(full, r.maxLen)
	return Document{Name: strings.Join(names, " "), Body: capped, Cut: len(capped) < len(full)}
}

// literal returns a triple's value when it is a non-empty string that is not a relationship.
func literal(triple message.Triple) (string, bool) {
	if triple.IsRelationship() {
		return "", false
	}
	value, ok := triple.Object.(string)
	return value, ok && value != ""
}

func hasSuffix(predicate string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(predicate, suffix) {
			return true
		}
	}
	return false
}

// Truncate cuts text to at most maxLen characters (runes), preferring the last Unicode
// whitespace boundary in the back half of the cap. It never splits a multibyte rune, so the
// result is valid UTF-8. A maxLen of 0 or less returns text unchanged.
func Truncate(text string, maxLen int) string {
	if maxLen <= 0 {
		return text
	}

	// The byte offset just past the maxLen-th rune; ranging a string yields each rune's start.
	runeCount := 0
	cut := len(text)
	for i := range text {
		if runeCount == maxLen {
			cut = i
			break
		}
		runeCount++
	}
	if cut == len(text) {
		return text
	}
	truncated := text[:cut]

	// Prefer the last whitespace boundary when it is past the halfway point, in runes.
	if sp := strings.LastIndexFunc(truncated, unicode.IsSpace); sp > 0 {
		if utf8.RuneCountInString(truncated[:sp]) > maxLen/2 {
			return truncated[:sp]
		}
	}
	return truncated
}
