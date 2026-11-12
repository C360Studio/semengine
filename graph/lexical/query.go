package lexical

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/c360studio/semengine/graph"
)

var (
	// ErrEmptyQuery is returned by ParseQuery for a query left with no plain
	// word and no phrase after tokenizing.
	ErrEmptyQuery = errors.New("lexical: query has nothing to search for")
	// ErrReservedSyntax is returned by ParseQuery for syntax kept for a later
	// query form, so that adding the form changes no answer given today.
	ErrReservedSyntax = errors.New("lexical: query uses syntax reserved for a later query form")
	// ErrQueryTooLong is returned by ParseQuery for a query holding more than
	// graph.MaxSearchTerms distinct words.
	ErrQueryTooLong = errors.New("lexical: query holds too many distinct words")
)

// Query is what Search looks for. ParseQuery builds one; the zero Query
// matches nothing.
type Query struct {
	// words are the distinct plain words, in the order the query gave them.
	words []string
	// phrases must each stand, word after word, within one field.
	phrases [][]string
	// excluded remove every entity holding one of them, word after word,
	// within one field.
	excluded [][]string
}

// reservedFields are the field prefixes kept for a later field-scoped form.
var reservedFields = []string{"name:", "body:"}

// ParseQuery reads a query typed by a person or passed on by a product.
//
// Text in double quotes is a phrase, and is literal: only tokenizing applies
// inside quotes. A quote left open ends at the end of the text. A term
// outside quotes starting with a single "-" followed by a letter, a digit or
// a quote is excluded: "go test -race" excludes "race", while "--force" is
// the word "force". Everything else is plain words. Every form is tokenized
// as documents are, and a phrase or excluded term left with no word is
// ignored.
//
// A term outside quotes starting with "name:" or "body:" followed by more
// text, or ending in a letter or digit followed by "*" or "~", is refused
// with an error matching ErrReservedSyntax. A query with no plain word and no
// phrase is refused with ErrEmptyQuery, and one holding more than
// graph.MaxSearchTerms distinct words, plain, phrase and excluded counted
// together, with ErrQueryTooLong.
func ParseQuery(text string) (Query, error) {
	var q Query
	// seen holds the distinct words of every form, so a long text is refused
	// as soon as it passes the bound rather than after it is all read.
	seen := make(map[string]bool)
	rest := text
	for {
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		if rest == "" {
			break
		}
		excluded := false
		if after, ok := strings.CutPrefix(rest, "-"); ok {
			if r, _ := utf8.DecodeRuneInString(after); r == '"' || !notWordRune(r) {
				excluded, rest = true, after
			}
		}

		var words []string
		phrase := false
		if after, ok := strings.CutPrefix(rest, `"`); ok {
			quoted, tail, _ := strings.Cut(after, `"`)
			words, rest, phrase = Tokens(quoted), tail, true
		} else {
			end := strings.IndexFunc(rest, func(r rune) bool { return unicode.IsSpace(r) || r == '"' })
			if end < 0 {
				end = len(rest)
			}
			term := rest[:end]
			rest = rest[end:]
			if isReserved(term, strings.HasPrefix(rest, `"`)) {
				return Query{}, fmt.Errorf("%w: %q", ErrReservedSyntax, term)
			}
			words = Tokens(term)
		}

		for _, w := range words {
			seen[w] = true
		}
		if len(seen) > graph.MaxSearchTerms {
			return Query{}, fmt.Errorf("%w: more than %d", ErrQueryTooLong, graph.MaxSearchTerms)
		}

		switch {
		case len(words) == 0:
			// A phrase or term of stopwords and punctuation only adds nothing.
		case excluded:
			q.excluded = append(q.excluded, words)
		case phrase:
			q.phrases = append(q.phrases, words)
		default:
			for _, w := range words {
				if !slices.Contains(q.words, w) {
					q.words = append(q.words, w)
				}
			}
		}
	}

	if len(q.words) == 0 && len(q.phrases) == 0 {
		return Query{}, fmt.Errorf("%w: %q", ErrEmptyQuery, text)
	}
	return q, nil
}

// isReserved reports a term, read outside quotes, that a later query form
// would read differently: a field prefix with something after it (a quote
// counts), or a letter or digit followed by a final "*" or "~".
func isReserved(term string, quoteFollows bool) bool {
	for _, field := range reservedFields {
		if strings.HasPrefix(term, field) && (len(term) > len(field) || quoteFollows) {
			return true
		}
	}
	last, size := utf8.DecodeLastRuneInString(term)
	if last != '*' && last != '~' {
		return false
	}
	prev, _ := utf8.DecodeLastRuneInString(term[:len(term)-size])
	return !notWordRune(prev)
}

// scored is the words a search scores: the distinct plain and phrase words.
func (q Query) scored() []string {
	out := slices.Clone(q.words)
	for _, p := range q.phrases {
		for _, w := range p {
			if !slices.Contains(out, w) {
				out = append(out, w)
			}
		}
	}
	return out
}
