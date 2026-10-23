package flowgraph

import (
	"strings"
	"testing"
)

func TestSubjectCoversIsDirectionalCover(t *testing.T) {
	cases := []struct {
		filter, pattern string
		want            bool
	}{
		{"data.>", "data.raw", true},
		{"data.>", "data.*", true},
		{"data.>", "data.a.b", true},
		{"data.>", "data", false},
		{"data.*", "data.raw", true},
		{"data.*", "data.>", false},
		{"data.*", "data.a.b", false},
		{"data.raw", "data.raw", true},
		{"data.raw", "data.*", false},
		{"*.raw", "data.raw", true},
		{"data.raw", "*.raw", false},
		{">", "anything.at.all", true},
		{"data.>", "other.raw", false},
		{"", "data.raw", false},
		{"data.>", "data..raw", false},
		// Two filters whose concrete subject sets intersect (foo.baz.bar
		// matches both) but where neither covers the other. The retired
		// SubjectMatches advertised overlap and answered false for this pair;
		// cover is the question the validator asks, and false is correct here.
		{"foo.*.bar", "foo.baz.*", false},
		{"foo.baz.*", "foo.*.bar", false},
		{"foo.*.bar", "foo.baz.bar", true},
	}
	for _, c := range cases {
		if got := SubjectCovers(c.filter, c.pattern); got != c.want {
			t.Errorf("SubjectCovers(%q, %q) = %v, want %v", c.filter, c.pattern, got, c.want)
		}
	}
	// Cover is stricter than the direct match edge derivation uses: the two
	// agree that `data.*` reaches `data.raw`, and only cover refuses the
	// reverse.
	if !matchNATSPattern("data.*", "data.raw") || !matchNATSPattern("data.raw", "data.*") {
		t.Fatal("direct match is symmetric on this pair")
	}
	if SubjectCovers("data.raw", "data.*") {
		t.Fatal("cover must be stricter than the direct match")
	}
}

func TestValidSubject(t *testing.T) {
	for _, subject := range validSubjectCases {
		if !ValidSubject(subject) {
			t.Errorf("ValidSubject(%q) = false, want true", subject)
		}
	}
	for _, subject := range malformedSubjectCases {
		if ValidSubject(subject) {
			t.Errorf("ValidSubject(%q) = true, want false", subject)
		}
	}
}

// FuzzValidSubject checks ValidSubject against referenceValidSubject, the
// grammar written out in this file (D3), and checks that a subject covers
// itself exactly when it is valid.
func FuzzValidSubject(f *testing.F) {
	for _, subject := range validSubjectCases {
		f.Add(subject)
	}
	for _, subject := range malformedSubjectCases {
		f.Add(subject)
	}
	f.Fuzz(func(t *testing.T, subject string) {
		want := referenceValidSubject(subject)
		if got := ValidSubject(subject); got != want {
			t.Fatalf("ValidSubject(%q) = %v, reference grammar says %v", subject, got, want)
		}
		if got := SubjectCovers(subject, subject); got != want {
			t.Fatalf("SubjectCovers(%q, %q) = %v, want %v (valid subjects cover themselves, malformed ones nothing)",
				subject, subject, got, want)
		}
	})
}

// TestSubjectHelpersRefuseWhitespace is #368's reproduction, widened to form
// feed: a malformed subject is covered by nothing and overlaps nothing.
func TestSubjectHelpersRefuseWhitespace(t *testing.T) {
	for _, ws := range subjectWhitespace {
		subject := "a.foo" + ws + "bar"
		if SubjectCovers("a.>", subject) {
			t.Errorf("SubjectCovers(%q, %q) = true, want false", "a.>", subject)
		}
		if SubjectsOverlap("a.>", subject) {
			t.Errorf("SubjectsOverlap(%q, %q) = true, want false", "a.>", subject)
		}
	}
}

// subjectWhitespace is nats-server's set of bytes a subject token may not
// hold (isValidSubject, server/sublist.go at v2.15.1).
var subjectWhitespace = []string{" ", "\t", "\n", "\r", "\f"}

var validSubjectCases = []string{"a", "a.b", "*", ">", "a.*.c", "a.>"}

var malformedSubjectCases = func() []string {
	cases := []string{"", "a..b", ".a", "a.", "a.>.b", "a*", "b*.c"}
	for _, ws := range subjectWhitespace {
		cases = append(cases, "a.foo"+ws+"bar")
	}
	return cases
}()

// referenceValidSubject is the subject grammar stated in the design (D3),
// written independently of the package's own check: one or more
// `.`-separated tokens; each is `*`, `>` (last only), or a non-empty run with
// no `*`, `>`, space, tab, line feed, carriage return or form feed.
func referenceValidSubject(subject string) bool {
	tokens := strings.Split(subject, ".")
	for index, token := range tokens {
		switch {
		case token == "*":
		case token == ">":
			if index != len(tokens)-1 {
				return false
			}
		case token == "":
			return false
		default:
			for _, r := range token {
				switch r {
				case '*', '>', ' ', '\t', '\n', '\r', '\f':
					return false
				}
			}
		}
	}
	return true
}
