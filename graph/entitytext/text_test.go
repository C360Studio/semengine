package entitytext_test

import (
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
)

const subject = "acme.ops.robotics.gcs.document.001"

func entity(triples ...message.Triple) *graph.EntityState {
	return &graph.EntityState{ID: subject, Triples: triples}
}

func triple(predicate string, object any) message.Triple {
	return message.Triple{Subject: subject, Predicate: predicate, Object: object}
}

// registerLabel registers predicate as a vocabulary label predicate for the rest of the test.
func registerLabel(t *testing.T, predicate string) {
	t.Helper()
	t.Cleanup(vocabulary.SnapshotRegistry())
	vocabulary.Register(predicate, vocabulary.WithAlias(vocabulary.AliasTypeLabel, 0))
}

// TestNameAndBodyRoles: Document puts a label predicate's value in Name whatever its suffix,
// with every .name and .title value, and every other selected value in Body; Values leaves the
// label predicate's value out, because its suffix is not configured. A relationship is in
// neither.
//
// Requirement: entity-text/Document splits an entity's text into Name and Body
func TestNameAndBodyRoles(t *testing.T) {
	registerLabel(t, "fleet.identity.callsign")
	rule := entitytext.New(append(entitytext.DefaultSuffixes(), ".comment"), 0)
	state := entity(
		triple("dc.terms.title", "Battery report"),
		triple("fleet.identity.name", "Falcon"),
		triple("code.doc.comment", "explains the battery curve"),
		triple("fleet.identity.callsign", "Red Leader"),
		triple("fleet.member.of", "acme.ops.robotics.gcs.fleet.alpha"),
	)

	doc := rule.Document(state, "")
	if want := "Battery report Falcon Red Leader"; doc.Name != want {
		t.Errorf("Name = %q, want %q", doc.Name, want)
	}
	if want := "explains the battery curve"; doc.Body != want {
		t.Errorf("Body = %q, want %q", doc.Body, want)
	}
	if doc.Cut {
		t.Error("Cut = true with no cap")
	}
	if got, want := rule.Values(state), []string{"Battery report", "Falcon", "explains the battery curve"}; !slices.Equal(got, want) {
		t.Errorf("Values = %q, want %q", got, want)
	}

	t.Run("a name value whose suffix is not configured is not text", func(t *testing.T) {
		rule := entitytext.New([]string{".comment"}, 0)
		doc := rule.Document(state, "")
		if doc.Name != "Battery report Red Leader" || doc.Body != "explains the battery curve" {
			t.Errorf("Document = %+v, want the two label predicates in Name and the comment in Body", doc)
		}
	})
	t.Run("the offloaded body follows the inline body", func(t *testing.T) {
		doc := rule.Document(state, "the stored body")
		if want := "explains the battery curve the stored body"; doc.Body != want {
			t.Errorf("Body = %q, want %q", doc.Body, want)
		}
		doc = rule.Document(entity(triple("dc.terms.title", "Only a name")), "the stored body")
		if doc.Name != "Only a name" || doc.Body != "the stored body" {
			t.Errorf("Document = %+v, want the name and the stored body alone", doc)
		}
	})
	t.Run("empty suffixes mean the defaults", func(t *testing.T) {
		got := entitytext.New(nil, 0).Values(state)
		if want := []string{"Battery report", "Falcon"}; !slices.Equal(got, want) {
			t.Errorf("Values = %q, want %q", got, want)
		}
	})
}

// TestDocumentCap: the cap applies to Body only, inline values first and the offloaded body after
// them, and cuts at the last whitespace in the back half of the cap, else at the cap; Name is never
// cut, even when it alone is longer than the cap; the cut is reported.
//
// Requirement: entity-text/The text cap cuts the body's tail and says so
func TestDocumentCap(t *testing.T) {
	longName := strings.Repeat("name ", 10) + "end"
	state := entity(triple("doc.meta.title", longName), triple("doc.meta.summary", "alpha beta"))

	t.Run("a body past the cap", func(t *testing.T) {
		// "alpha beta gamma delta epsilon" is 30 characters; at a cap of 20 the last space at
		// or before it ends "gamma".
		doc := entitytext.New(nil, 20).Document(state, "gamma delta epsilon")
		if doc.Name != longName {
			t.Errorf("Name = %q, want it whole: %q", doc.Name, longName)
		}
		if want := "alpha beta gamma"; doc.Body != want {
			t.Errorf("Body = %q, want %q", doc.Body, want)
		}
		if !doc.Cut {
			t.Error("Cut = false for a body past the cap")
		}
	})
	t.Run("a body at the cap", func(t *testing.T) {
		doc := entitytext.New(nil, 30).Document(state, "gamma delta epsilon")
		if want := "alpha beta gamma delta epsilon"; doc.Body != want || doc.Cut {
			t.Errorf("Document = %+v, want Body %q, not cut", doc, want)
		}
	})
	t.Run("an inline body past the cap", func(t *testing.T) {
		doc := entitytext.New(nil, 7).Document(state, "")
		if doc.Name != longName || doc.Body != "alpha" || !doc.Cut {
			t.Errorf("Document = %+v, want the whole name, Body %q, cut", doc, "alpha")
		}
	})
	t.Run("a word longer than half the cap is cut at the cap", func(t *testing.T) {
		doc := entitytext.New(nil, 7).Document(entity(), "ab cdefghij")
		if doc.Body != "ab cdef" || !doc.Cut {
			t.Errorf("Document = %+v, want Body %q, cut", doc, "ab cdef")
		}
	})
	t.Run("a cap counts characters, not bytes", func(t *testing.T) {
		doc := entitytext.New(nil, 4).Document(entity(), "été été")
		if doc.Body != "été" || !doc.Cut {
			t.Errorf("Document = %+v, want Body %q, cut", doc, "été")
		}
	})
}

// TestWithBodyMatchesDocument: a body joined later with WithBody to the Document an entity makes
// with no body gives what Document gives with the body, and both equal the text rule written out
// here: the name's values joined; the other values, then the body, joined by spaces and cut at
// the cap as Truncate cuts, the cut reported. The named witnesses put the inline body text below,
// at and past the cap, cut back to a space with room left for the body, and leave the inline text
// or the body empty; the generated run draws texts with spaces and multibyte characters around a
// small cap.
//
// Requirement: entity-text/Document splits an entity's text into Name and Body
func TestWithBodyMatchesDocument(t *testing.T) {
	type failer interface {
		Helper()
		Fatalf(format string, args ...any)
	}
	check := func(f failer, maxLen int, rest []string, body string) {
		f.Helper()
		state := entity(triple("acme.doc.name", "pumpname"))
		for _, value := range rest {
			state.Triples = append(state.Triples, triple("acme.doc.description", value))
		}
		var parts []string
		for _, value := range append(slices.Clone(rest), body) {
			if value != "" {
				parts = append(parts, value)
			}
		}
		full := strings.Join(parts, " ")
		cut := entitytext.Truncate(full, maxLen)
		want := entitytext.Document{Name: "pumpname", Body: cut, Cut: len(cut) < len(full)}

		rule := entitytext.New(nil, maxLen)
		if got := rule.Document(state, body); got != want {
			f.Fatalf("cap %d, rest %q, body %q: Document = %+v; want %+v", maxLen, rest, body, got, want)
		}
		if got := rule.WithBody(rule.Document(state, ""), body); got != want {
			f.Fatalf("cap %d, rest %q, body %q: WithBody = %+v; want %+v", maxLen, rest, body, got, want)
		}
	}
	witnesses := []struct {
		name string
		rest []string
		body string
	}{
		{"inline below the cap", []string{"abc"}, "defgh ijk"},
		{"inline at the cap", []string{"abcdefghij"}, "klm"},
		{"inline past the cap", []string{"abcde fghijkl"}, "mno"},
		{"inline cut back to a space, with room for the body", []string{"abcdefg hijklmn"}, "x"},
		{"no inline text", nil, "abcde fghijkl"},
		{"no body", []string{"abc"}, ""},
		{"two inline values", []string{"ab", "cd"}, "efghij"},
	}
	for _, w := range witnesses {
		t.Run(w.name, func(t *testing.T) { check(t, 10, w.rest, w.body) })
	}
	if t.Failed() {
		return // a failing witness is the finding; a generated run would only repeat it
	}
	text := rapid.StringOfN(rapid.SampledFrom([]rune{'a', 'b', ' ', 'é', '水'}), 0, 24, -1)
	rapid.Check(t, func(t *rapid.T) {
		check(t, rapid.IntRange(1, 16).Draw(t, "maxLen"), rapid.SliceOfN(text, 0, 3).Draw(t, "rest"),
			text.Draw(t, "body"))
	})
}
