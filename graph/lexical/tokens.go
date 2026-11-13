package lexical

import (
	"strings"
	"unicode"
)

// maxWordBytes is the longest word kept, measured after folding. Splitting
// identifiers first keeps long test and type names as words; what is left
// over the cap is almost all hex (commit SHAs and digests), which no one
// searches for by word.
const maxWordBytes = 32

// stopwords are dropped from every text and query. The list is short on
// purpose: words such as "not", "no", "get", "find", "show" and "all" carry
// meaning in technical text and stay searchable.
var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "as": true, "is": true, "was": true,
	"are": true, "be": true, "has": true, "have": true, "had": true, "do": true,
	"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
}

// Tokens returns the words of text in order, repeats kept.
//
// A word starts as a run of Unicode letters and digits, so "graph.query.status"
// and "pump-7" split at their punctuation and one-letter words such as "v" stay.
// A run that ends in a lower-case "s" right after two or more capitals is an
// acronym in the plural: the "s" goes and the capitals stay one part, so
// "IDs" gives "id" and "URLs" gives "url". Only the end of a run counts,
// because inside one the same letters are as often an acronym followed by a
// word ("EntityIDIsTerminated"), so "EntityIDsByType" splits as before.
// A run splits again where an identifier changes case: at a lower-case letter
// followed by a capital, and before the last capital of a run of capitals
// followed by lower case, so "withRetry" gives "with" and "retry" and
// "HTTPServer" gives "http" and "server". The joined form ("withretry") is not
// a word. Each part is lower-cased and its plural folded to the singular
// ("services" to "service", "caches" to "cach", like "cache", "ids" to "id").
// A word is dropped when it or its fold is a stopword, so "wills" goes with
// "will" and "ins" with "in", or when its fold is over 32 bytes.
//
// Tokenizing the words Tokens returns, joined by spaces, gives them back
// unchanged.
func Tokens(text string) []string {
	var words []string
	for _, run := range strings.FieldsFunc(text, notWordRune) {
		for _, part := range splitIdentifier(run) {
			// Split again so that every word holds only letters and digits
			// even if lower-casing yields another rune. Go's Unicode tables
			// have no such case today; the guard costs one pass over the part.
			for _, w := range strings.FieldsFunc(strings.ToLower(part), notWordRune) {
				f := fold(w)
				if stopwords[w] || stopwords[f] || len(f) > maxWordBytes {
					continue
				}
				words = append(words, f)
			}
		}
	}
	return words
}

func notWordRune(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

// isCapital reports a letter that lower-casing changes. A letter with no
// lower-case form is not a capital, so a lower-cased word never splits again.
func isCapital(r rune) bool { return unicode.ToLower(r) != r }

// splitIdentifier drops the "s" of a run ending in two or more capitals and a
// lower-case "s", then splits the run at each lower-case letter followed by a
// capital, and before the last capital of a run of capitals followed by lower
// case.
func splitIdentifier(run string) []string {
	rs := []rune(run)
	if n := len(rs); n >= 3 && rs[n-1] == 's' && isCapital(rs[n-2]) && isCapital(rs[n-3]) {
		rs = rs[:n-1]
	}
	var parts []string
	start := 0
	for i := 1; i < len(rs); i++ {
		prev, cur := rs[i-1], rs[i]
		lowerNext := i+1 < len(rs) && unicode.IsLower(rs[i+1])
		if (unicode.IsLower(prev) && isCapital(cur)) || (isCapital(prev) && isCapital(cur) && lowerNext) {
			parts = append(parts, string(rs[start:i]))
			start = i
		}
	}
	return append(parts, string(rs[start:]))
}

// fold maps a lower-case plural to its singular by the first rule that
// applies. The first two rules come before the plain plural rules so that
// "caches" and "cache" meet at "cach", and "boxes" at "box" rather than "boxe".
// Words ending in "us" or "ss" (status, class) keep their final "s". The last
// rule needs over 2 bytes so that "ids" meets "id" and "prs" meets "pr"; it
// also folds words that are not plurals: "its" meets "it", and an acronym of
// three letters ending in "s" loses it ("tls" to "tl", "cas" to "ca"). Two
// bytes keep their "s" ("os", "ms"). Folding a fold changes nothing.
func fold(w string) string {
	n := len(w)
	switch {
	case n > 4 && strings.HasSuffix(w, "es") && endsInSibilant(w[:n-2]):
		return w[:n-2]
	case n > 3 && strings.HasSuffix(w, "e") && endsInSibilant(w[:n-1]):
		return w[:n-1]
	case n > 4 && strings.HasSuffix(w, "ies") && !hasAnySuffix(w, "eies", "aies"):
		return w[:n-3] + "y"
	case n > 3 && strings.HasSuffix(w, "es") && !hasAnySuffix(w, "aes", "ees", "oes"):
		return w[:n-1]
	case n > 2 && strings.HasSuffix(w, "s") && !hasAnySuffix(w, "us", "ss"):
		return w[:n-1]
	}
	return w
}

func endsInSibilant(stem string) bool { return hasAnySuffix(stem, "ss", "x", "ch", "sh") }

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}
