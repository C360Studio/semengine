package lexical

import (
	"slices"
	"strings"
	"testing"
	"unicode"

	"pgregory.net/rapid"
)

func TestTokens(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "code and prose",
			text: "withRetry HTTPServer graph.query.status docker compose down -v pump-7 Services",
			want: []string{"retry", "http", "server", "graph", "query", "status", "docker", "compose", "down", "v", "pump", "7", "service"},
		},
		{
			name: "stopword beside meaningful short words",
			text: "withRetry not found",
			want: []string{"retry", "not", "found"},
		},
		{
			name: "stopwords and their plurals",
			text: "wills does haves ands ares thes fors withs",
			want: nil,
		},
		{
			name: "long identifier and a commit SHA",
			text: "TestComponentManagerStopAfterProviderFailureDoesNotWaitForUnlaunchedSupervisor 8b99efe9c66a4faa4fa509f9f62cc6bad8392128",
			want: []string{"test", "component", "manager", "stop", "after", "provider", "failure", "not", "wait", "unlaunched", "supervisor"},
		},
		{
			name: "a fold of 32 bytes stays, a word of 33 goes",
			text: strings.Repeat("q", 32) + "s " + strings.Repeat("q", 33),
			want: []string{strings.Repeat("q", 32)},
		},
		{
			name: "capitals outside ASCII",
			text: "İstanbul ΣΑΣ ǅemal",
			want: []string{"istanbul", "σασ", "ǆemal"},
		},
		{
			name: "acronym plurals",
			text: "IDs IDS URLs URLS CPUs UUIDs ids",
			want: []string{"id", "id", "url", "url", "cpu", "uuid", "id"},
		},
		{
			name: "one capital before the s is not an acronym plural",
			text: "kBs",
			want: []string{"k", "bs"},
		},
		{
			name: "a capital run before a word is unchanged",
			text: "HTTPServer",
			want: []string{"http", "server"},
		},
		{
			name: "an acronym plural inside a run is unchanged",
			text: "EntityIDsByType",
			want: []string{"entity", "i", "ds", "type"},
		},
		{
			name: "no words",
			text: " .-_/ ",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Tokens(tc.text); !slices.Equal(got, tc.want) {
				t.Errorf("Tokens(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestPluralRules(t *testing.T) {
	together := [][2]string{
		{"service", "services"}, {"query", "queries"}, {"class", "classes"}, {"match", "matches"},
		{"box", "boxes"}, {"hash", "hashes"}, {"cache", "caches"}, {"index", "indexes"},
		{"process", "processes"}, {"address", "addresses"}, {"id", "ids"}, {"pr", "prs"},
	}
	for _, p := range together {
		a, b := Tokens(p[0]), Tokens(p[1])
		if len(a) != 1 || !slices.Equal(a, b) {
			t.Errorf("%q and %q give %q and %q, want one shared word", p[0], p[1], a, b)
		}
	}
	apart := [][2]string{{"bus", "buses"}, {"status", "statuses"}, {"cookie", "cookies"}}
	for _, p := range apart {
		a, b := Tokens(p[0]), Tokens(p[1])
		if len(a) != 1 || len(b) != 1 || a[0] == b[0] {
			t.Errorf("%q and %q give %q and %q, want two different words", p[0], p[1], a, b)
		}
	}

	// Each rule at its length bound and just past it, and each excluded
	// ending, worked by hand from the five rules.
	folds := map[string]string{
		"boxes": "box", "xxes": "xxe", // rule 1 needs over 4 bytes
		"boxe": "box", "axe": "axe", // rule 2 needs over 3 bytes
		"flies": "fly", "pies": "pie", "beies": "beie", "baies": "baie", // rule 3
		"files": "file", "uses": "use", "trees": "tree", "goes": "goe", "caes": "cae", // rule 4
		"bugs": "bug", "its": "it", "os": "os", "bus": "bus", "bonus": "bonus", "chess": "chess", // rule 5 needs over 2 bytes
	}
	for in, want := range folds {
		if got := Tokens(in); !slices.Equal(got, []string{want}) {
			t.Errorf("Tokens(%q) = %q, want [%q]", in, got, want)
		}
	}
}

func TestPropFoldIsIdempotent(t *testing.T) {
	check := func(t interface{ Fatalf(string, ...any) }, w string) {
		once := fold(w)
		if twice := fold(once); twice != once {
			t.Fatalf("fold(%q) = %q, but fold(%q) = %q", w, once, once, twice)
		}
	}
	// Each rule's ending at and past its length bound, and the endings
	// that a fold can leave behind for a later rule to see.
	for _, w := range []string{
		"sses", "ssess", "xes", "xxes", "boxes", "boxeses", "chesses", "caches", "cache",
		"axe", "boxe", "ies", "fies", "flies", "eies", "beies", "aies", "baies", "queries",
		"aes", "caes", "goes", "trees", "uses", "statuses", "bus", "buses", "ss", "s", "es", "e", "",
	} {
		check(t, w)
	}

	// Short stems over the letters the rules test, followed by an ending
	// that brings each rule, and each of its exclusions, into play.
	stems := rapid.StringOfN(rapid.RuneFrom([]rune("acehiosuxy")), 0, 6, -1)
	endings := rapid.SampledFrom([]string{
		"", "s", "es", "e", "ss", "sses", "sse", "xes", "xe", "ches", "che", "shes", "she",
		"ies", "eies", "aies", "aes", "ees", "oes", "us", "uses",
	})
	rapid.Check(t, func(t *rapid.T) {
		check(t, stems.Draw(t, "stem")+endings.Draw(t, "ending"))
	})
}

func FuzzTokens(f *testing.F) {
	for _, s := range []string{
		"withRetry HTTPServer graph.query.status docker compose down -v pump-7 Services",
		"wills does haves ands ares thes fors withs",
		"İstanbul", "ǅemal", "Xϒ", "Straße", "ﬁle", "ΣΑΣ", "x́y", "a.b-c", "caches boxes queries", "IDs ids URLs",
		strings.Repeat("q", 33), "\xff\xfe", "",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		words := Tokens(s)
		for _, w := range words {
			if w == "" || len(w) > maxWordBytes || strings.ToLower(w) != w ||
				strings.IndexFunc(w, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) >= 0 {
				t.Fatalf("Tokens(%q) gave the word %q", s, w)
			}
		}
		if again := Tokens(strings.Join(words, " ")); !slices.Equal(again, words) {
			t.Fatalf("Tokens(%q) = %q, but tokenizing those words gives %q", s, words, again)
		}
	})
}
