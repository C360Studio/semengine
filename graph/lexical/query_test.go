package lexical

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// thirtyThree distinct words that tokenizing leaves as they are, one more
// than a query may hold.
var thirtyThree = strings.Fields("alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike " +
	"november oscar papa quebec romeo sierra tango uniform victor whiskey xray yankee zulu " +
	"amber basil cedar dune ember fjord glade")

func TestParseQuery(t *testing.T) {
	thirtyTwo := strings.Join(thirtyThree[:32], " ")
	for _, tc := range []struct {
		text string
		want Query
	}{
		{`retry -test "docker compose down -v"`, Query{
			words:    []string{"retry"},
			phrases:  [][]string{{"docker", "compose", "down", "v"}},
			excluded: [][]string{{"test"}},
		}},
		{`go test -race`, Query{words: []string{"go", "test"}, excluded: [][]string{{"race"}}}},
		{`go test "-race"`, Query{words: []string{"go", "test"}, phrases: [][]string{{"race"}}}},
		{`deploy --force`, Query{words: []string{"deploy", "force"}}},
		{`retry "to be" -the`, Query{words: []string{"retry"}}},
		{`retry "docker compose`, Query{words: []string{"retry"}, phrases: [][]string{{"docker", "compose"}}}},
		{`retry Retry retries`, Query{words: []string{"retry"}}},
		{`up -"docker compose"`, Query{words: []string{"up"}, excluded: [][]string{{"docker", "compose"}}}},
		{`-graph.query.status run`, Query{words: []string{"run"}, excluded: [][]string{{"graph", "query", "status"}}}},
		{"retry\t-test\n-7", Query{words: []string{"retry"}, excluded: [][]string{{"test"}, {"7"}}}},
		{`retry - x -*y`, Query{words: []string{"retry", "x", "y"}}},
		{`retry"docker compose"up`, Query{words: []string{"retry", "up"}, phrases: [][]string{{"docker", "compose"}}}},
		{`-état x`, Query{words: []string{"x"}, excluded: [][]string{{"état"}}}},
		{thirtyTwo, Query{words: thirtyThree[:32]}},
		{thirtyTwo + " Alpha alphas", Query{words: thirtyThree[:32]}},
	} {
		got, err := ParseQuery(tc.text)
		if err != nil {
			t.Errorf("ParseQuery(%q): %v", tc.text, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseQuery(%q) = %+v, want %+v", tc.text, got, tc.want)
		}
	}

	for _, tc := range []struct {
		text string
		want error
	}{
		{`the of and`, ErrEmptyQuery},
		{`"to be"`, ErrEmptyQuery},
		{`-test`, ErrEmptyQuery},
		{`""`, ErrEmptyQuery},
		{``, ErrEmptyQuery},
		{" \t ", ErrEmptyQuery},
		{`-test -"docker compose" "the"`, ErrEmptyQuery},
		{strings.Join(thirtyThree, " "), ErrQueryTooLong},
		{thirtyTwo + " -glade", ErrQueryTooLong},
		{strings.Join(thirtyThree[:31], " ") + ` "fjord glade"`, ErrQueryTooLong},
	} {
		if q, err := ParseQuery(tc.text); !errors.Is(err, tc.want) {
			t.Errorf("ParseQuery(%q) = %+v, %v; want %v", tc.text, q, err, tc.want)
		}
	}
}

func TestParseQueryReservesForms(t *testing.T) {
	for _, tc := range []struct{ text, term string }{
		{`name:retry`, `name:retry`}, {`retr*`, `retr*`}, {`void*`, `void*`}, {`pump*`, `pump*`},
		{`body:retry`, `body:retry`}, {`-name:retry`, `name:retry`}, {`name:"retry"`, `name:`},
		{`retry -retr*`, `retr*`}, {`x*`, `x*`}, {`é*`, `é*`}, {`retr*"docker compose"`, `retr*`},
		{`retr*x*`, `retr*x*`},
	} {
		q, err := ParseQuery(tc.text)
		if !errors.Is(err, ErrReservedSyntax) {
			t.Errorf("ParseQuery(%q) = %+v, %v; want ErrReservedSyntax", tc.text, q, err)
			continue
		}
		// The refusal names the term and tells the caller how to search it as text.
		if msg := err.Error(); !strings.Contains(msg, `"`+tc.term+`"`) || !strings.Contains(msg, "quote") {
			t.Errorf("ParseQuery(%q) refusal %q does not name %q and say to quote it", tc.text, msg, tc.term)
		}
	}

	for _, tc := range []struct {
		text string
		want Query
	}{
		{`pkg.Func() name: retry`, Query{words: []string{"pkg", "func", "name", "retry"}}},
		{`SELECT * FROM users`, Query{words: []string{"select", "user"}}},
		{`java.util.*`, Query{words: []string{"java", "util"}}},
		{`**bold**`, Query{words: []string{"bold"}}},
		{`*bold* text`, Query{words: []string{"bold", "text"}}},
		{`*retr*`, Query{words: []string{"retr"}}},
		{`git reset HEAD~`, Query{words: []string{"git", "reset", "head"}}},
		{`retry~`, Query{words: []string{"retry"}}},
		{`7~ HEAD~2`, Query{words: []string{"7", "head", "2"}}},
		{`cd ~`, Query{words: []string{"cd"}}},
		{`"name:retry void*"`, Query{phrases: [][]string{{"name", "retry", "void"}}}},
		{`body: -* ~retry`, Query{words: []string{"body", "retry"}}},
	} {
		got, err := ParseQuery(tc.text)
		if err != nil {
			t.Errorf("ParseQuery(%q): %v", tc.text, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseQuery(%q) = %+v, want %+v", tc.text, got, tc.want)
		}
	}
}

func TestParseQueryExcluded(t *testing.T) {
	for _, tc := range []struct {
		text string
		want [][]string
	}{
		{`go test -race ./pkg -run TestX`, [][]string{{"race"}, {"run"}}},
		{`up -"docker compose" -test -the`, [][]string{{"docker", "compose"}, {"test"}}},
		{`go test "-race"`, nil},
	} {
		q, err := ParseQuery(tc.text)
		if err != nil {
			t.Errorf("ParseQuery(%q): %v", tc.text, err)
			continue
		}
		if got := q.Excluded(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseQuery(%q).Excluded() = %q, want %q", tc.text, got, tc.want)
		}
	}

	// The caller gets its own copy: changing it changes nothing the query searches.
	q := mustParse(t, `retry -test`)
	q.Excluded()[0][0] = "changed"
	if got := q.Excluded(); !reflect.DeepEqual(got, [][]string{{"test"}}) {
		t.Errorf("after the caller changed its copy, Excluded() = %q, want [[test]]", got)
	}
}

// renderQuery writes q back as query text: plain words, then each phrase in
// quotes, then each excluded term as a quoted phrase after "-".
func renderQuery(q Query) string {
	parts := slices.Clone(q.words)
	for _, p := range q.phrases {
		parts = append(parts, `"`+strings.Join(p, " ")+`"`)
	}
	for _, x := range q.excluded {
		parts = append(parts, `-"`+strings.Join(x, " ")+`"`)
	}
	return strings.Join(parts, " ")
}

func FuzzParseQuery(f *testing.F) {
	for _, s := range []string{
		`retry -test "docker compose down -v"`, `go test -race`, `go test "-race"`, `deploy --force`,
		`retry "to be" -the`, `the of and`, `"to be"`, `-test`, `""`, ``, `"unclosed`, `-"unclosed`,
		`name:retry`, `retr*`, `retry~`, `void*`, `name:"retry"`, `-name:retry`,
		`pkg.Func() name: retry`, `SELECT * FROM users`, `java.util.*`, `**bold**`, `cd ~`, `"name:retry void*"`,
		`*bold* text`, `git reset HEAD~`, `*retr*`, `retr*x*`,
		strings.Join(thirtyThree, " "), `-`, `"`, `-"`, `--`, `withRetry HTTPServer`, "\xff-\xfe\"", "-\xff",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		q, err := ParseQuery(s)
		if err != nil {
			if !errors.Is(err, ErrEmptyQuery) && !errors.Is(err, ErrReservedSyntax) && !errors.Is(err, ErrQueryTooLong) {
				t.Fatalf("ParseQuery(%q): untyped error %v", s, err)
			}
		} else {
			checkParsed(t, s, q)
		}

		// Quoting makes any text literal: with its quotes removed and put in
		// quotes, the text is one phrase of its words, never reserved syntax.
		literal := strings.ReplaceAll(s, `"`, " ")
		words := Tokens(literal)
		q, err = ParseQuery(`"` + literal + `"`)
		switch {
		case len(words) == 0:
			if !errors.Is(err, ErrEmptyQuery) {
				t.Fatalf("quoted %q: %+v, %v; want ErrEmptyQuery", literal, q, err)
			}
		case len(distinct(words)) > 32:
			if !errors.Is(err, ErrQueryTooLong) {
				t.Fatalf("quoted %q: %+v, %v; want ErrQueryTooLong", literal, q, err)
			}
		case err != nil || !reflect.DeepEqual(q, Query{phrases: [][]string{words}}):
			t.Fatalf("quoted %q = %+v, %v; want the one phrase %q", literal, q, err, words)
		}
	})
}

// checkParsed holds a parsed query to the rules every query meets, and
// requires that writing it back as text parses to the same query.
func checkParsed(t *testing.T, s string, q Query) {
	t.Helper()
	if len(q.words) == 0 && len(q.phrases) == 0 {
		t.Fatalf("ParseQuery(%q) = %+v: nothing to search for, yet no error", s, q)
	}
	if !slices.Equal(distinct(q.words), q.words) {
		t.Fatalf("ParseQuery(%q) = %+v: repeated plain word", s, q)
	}
	all := slices.Clone(q.words)
	for _, run := range slices.Concat(q.phrases, q.excluded) {
		if len(run) == 0 {
			t.Fatalf("ParseQuery(%q) = %+v: a phrase or excluded term with no word", s, q)
		}
		all = append(all, run...)
	}
	if n := len(distinct(all)); n > 32 {
		t.Fatalf("ParseQuery(%q) = %+v: %d distinct words", s, q, n)
	}
	for _, w := range all {
		if got := Tokens(w); !slices.Equal(got, []string{w}) {
			t.Fatalf("ParseQuery(%q) = %+v: %q is not a word Tokens gives (%q)", s, q, w, got)
		}
	}
	text := renderQuery(q)
	again, err := ParseQuery(text)
	if err != nil || !reflect.DeepEqual(again, q) {
		t.Fatalf("ParseQuery(%q) = %+v, written back as %q parses to %+v, %v", s, q, text, again, err)
	}
}

func TestCombinedQuery(t *testing.T) {
	ix := NewIndex()
	const a, b, c = "c360.lex.test.doc.item.a", "c360.lex.test.doc.item.b", "c360.lex.test.doc.item.c"
	mustPut(t, ix, a, Document{Body: "retry docker compose"})
	mustPut(t, ix, b, Document{Body: "compose retry docker"})
	mustPut(t, ix, c, Document{Body: "docker compose"})

	res := mustSearch(t, ix, `retry "docker compose"`, Options{Limit: 10})
	if got, want := hitIDs(res), []string{a, c}; !slices.Equal(got, want) || res.Matched != 2 {
		t.Fatalf("hits = %+v, want %v with Matched 2", res, want)
	}
	// The phrase filters and its words rank: each hit scores as it does for
	// the same words searched plainly.
	plain := mustSearch(t, ix, "retry docker compose", Options{Limit: 10})
	scores := map[string]float64{}
	for _, h := range plain.Hits {
		scores[h.EntityID] = h.Score
	}
	for _, h := range res.Hits {
		if h.Score != scores[h.EntityID] {
			t.Errorf("%s scores %v with the phrase, %v with plain words", h.EntityID, h.Score, scores[h.EntityID])
		}
	}
}

func TestUnmatchedPhraseFindsNothing(t *testing.T) {
	ix := NewIndex()
	mustPut(t, ix, idOne, Document{Body: "retry backoff"})
	mustPut(t, ix, idTwo, Document{Name: "retry", Body: "phrase such no"})
	res, err := ix.Search(mustParse(t, `retry "no such phrase"`), Options{Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Hits) != 0 || res.Matched != 0 {
		t.Errorf("Search = %+v, want no hit and Matched 0", res)
	}
}

func TestPhraseNeedsAdjacentWords(t *testing.T) {
	ix := NewIndex()
	const together, apart, stopword = "c360.lex.test.doc.item.together", "c360.lex.test.doc.item.apart", "c360.lex.test.doc.item.stop"
	mustPut(t, ix, together, Document{Body: "then run docker compose down -v to reset"})
	mustPut(t, ix, apart, Document{Body: "docker run; compose up; shut down; ls -v"})
	mustPut(t, ix, stopword, Document{Body: "ran out the memory"})
	if got := hitIDs(mustSearch(t, ix, `"docker compose down -v"`, Options{Limit: 10})); !slices.Equal(got, []string{together}) {
		t.Errorf(`"docker compose down -v" = %v, want [%s]`, got, together)
	}
	// Stopwords are dropped before the words are compared, so a phrase
	// matches across a different stopword.
	if got := hitIDs(mustSearch(t, ix, `"out of memory"`, Options{Limit: 10})); !slices.Equal(got, []string{stopword}) {
		t.Errorf(`"out of memory" = %v, want [%s]`, got, stopword)
	}
}

func TestPhraseStaysInOneField(t *testing.T) {
	ix := NewIndex()
	const across, body, name = "c360.lex.test.doc.item.across", "c360.lex.test.doc.item.body", "c360.lex.test.doc.item.name"
	mustPut(t, ix, across, Document{Name: "pump docker", Body: "compose stack"})
	mustPut(t, ix, body, Document{Body: "docker compose"})
	mustPut(t, ix, name, Document{Name: "docker compose", Body: "stack"})
	got := hitIDs(mustSearch(t, ix, `"docker compose"`, Options{Limit: 10}))
	slices.Sort(got)
	if want := []string{body, name}; !slices.Equal(got, want) {
		t.Errorf(`"docker compose" = %v, want %v`, got, want)
	}
}

func TestExcludedWordRemovesEntities(t *testing.T) {
	ix := NewIndex()
	const (
		kept   = "c360.lex.test.doc.item.kept"
		file   = "c360.lex.test.doc.item.file"
		named  = "c360.lex.test.doc.item.named"
		split  = "c360.lex.test.doc.item.split"
		joined = "c360.lex.test.doc.item.joined"
	)
	mustPut(t, ix, kept, Document{Body: "retry backoff"})
	mustPut(t, ix, file, Document{Body: "retry loop in retry_test.go"})
	mustPut(t, ix, named, Document{Name: "test helper", Body: "retry"})
	mustPut(t, ix, split, Document{Name: "backoff", Body: "loop retry"})
	mustPut(t, ix, joined, Document{Body: "retry with backoff loop"})
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{`retry -test`, []string{joined, kept, split}},
		// An excluded term's words must stand together within one field.
		{`retry -"backoff loop"`, []string{file, kept, named, split}},
		{`retry -backoff.loop -test`, []string{kept, split}},
	} {
		got := hitIDs(mustSearch(t, ix, tc.query, Options{Limit: 10}))
		slices.Sort(got)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.query, got, tc.want)
		}
	}
}
