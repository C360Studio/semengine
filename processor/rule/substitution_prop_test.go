package rule

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-substitution/"A template is read once" and "The values of a
// predicate".
//
// The laws, each checked against an oracle the test owns:
//   - a template built from literal text and resolvable tokens comes out as
//     the literal text with each token's value in its place, whatever the
//     values hold (token text included), and nothing is reported;
//   - a template with no dollar sign comes out as it went in;
//   - .length is the number of values .triples lists, and both match the
//     count the test works out from the triples it stored.
//
// Literal text never extends a token: each literal run starts with a piece no
// token name may continue. Every token kind and every object shape is a named
// witness run through the same checks before the generated run, and every
// check runs on every case: none sits behind a guard.

// literalUnits are the pieces literal text is drawn from. The first
// literalStarts of them, which no token name may continue, start a run.
var literalUnits = []string{" ", ", ", "(", "$ ", "$1", "word", "x.y", "/", "$entity"}

const literalStarts = 5

// valueFragments are what generated triple and message values are made of,
// token text included, so a value that is read again shows.
var valueFragments = []string{"$message.secret", "$entity.id", "$now", "$state.iteration", "a", ".", "$", " ", "-"}

// tokenFixture is the context the generated templates read, with each token's
// expected value written out by the test.
type tokenFixture struct {
	ec     *ExecutionContext
	values map[string]string
	tokens []string
}

func newTokenFixture(t failer, id, notePredicate, note, secret string) tokenFixture {
	t.Helper()
	triples, err := json.Marshal([]string{note})
	if err != nil {
		t.Fatalf("encode the expected triples: %v", err)
	}
	f := tokenFixture{
		ec: &ExecutionContext{
			EntityID:    id,
			Entity:      entityWith(id, message.Triple{Predicate: notePredicate, Object: note}),
			State:       &MatchState{Iteration: 7, MaxIterations: 9},
			MessageData: map[string]any{"secret": secret},
		},
		values: map[string]string{
			"$entity.id":                               id,
			"$entity.instance":                         "001",
			"$entity.domain":                           "mav1",
			"$entity.triple.test.fixture.note":         note,
			"$entity.triple.test.fixture.note.length":  "1",
			"$entity.triple.test.fixture.note.triples": string(triples),
			"$entity.triple.test.fixture.gone.value":   "",
			"$message.secret":                          secret,
			"$state.iteration":                         "7",
			"$state.max_iterations":                    "9",
		},
	}
	for token := range f.values {
		f.tokens = append(f.tokens, token)
	}
	slices.Sort(f.tokens) // a fixed order, so a seed draws the same tokens
	return f
}

// checkReadOnce substitutes the template built from literals and tokens, one
// literal before each token and one after the last, and compares the result
// with the literals as written and each token's value in its place.
func checkReadOnce(ctx context.Context, t failer, log *warningLog, f tokenFixture, literals, tokens []string) {
	t.Helper()
	var template, want strings.Builder
	for i, lit := range literals {
		template.WriteString(lit)
		want.WriteString(lit)
		if i < len(tokens) {
			template.WriteString(tokens[i])
			want.WriteString(f.values[tokens[i]])
		}
	}
	got := f.ec.SubstituteVariables(ctx, template.String())
	if got != want.String() {
		t.Fatalf("SubstituteVariables(%q) = %q, want %q", template.String(), got, want.String())
	}
	if unresolved := log.unresolved(t); unresolved != nil {
		t.Fatalf("SubstituteVariables(%q) reported %q, want nothing", template.String(), unresolved)
	}
}

// checkNoDollar substitutes a template holding no dollar sign.
func checkNoDollar(ctx context.Context, t failer, log *warningLog, ec *ExecutionContext, template string) {
	t.Helper()
	if got := ec.SubstituteVariables(ctx, template); got != template {
		t.Fatalf("SubstituteVariables(%q) = %q, want it unchanged", template, got)
	}
	if unresolved := log.unresolved(t); unresolved != nil {
		t.Fatalf("SubstituteVariables(%q) reported %q", template, unresolved)
	}
}

// storedObject is one triple's object and the number of values the test
// expects it to contribute.
type storedObject struct {
	object any
	values int
}

func listObject(shape string, items []string) storedObject {
	if items == nil {
		items = []string{} // an empty list, never JSON null
	}
	switch shape {
	case "strings":
		return storedObject{object: items, values: len(items)}
	case "any":
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = item
		}
		return storedObject{object: out, values: len(items)}
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		panic(err)
	}
	return storedObject{object: string(encoded), values: len(items)}
}

// checkLengthCountsTriples stores objects on one predicate and checks that
// .length gives the count the test expects and the length of .triples decoded.
func checkLengthCountsTriples(ctx context.Context, t failer, id, predicate string, objects []storedObject) {
	t.Helper()
	entity := entityWith(id)
	want := 0
	for _, o := range objects {
		entity.Triples = append(entity.Triples, message.Triple{Subject: id, Predicate: predicate, Object: o.object})
		want += o.values
	}
	ec := &ExecutionContext{EntityID: id, Entity: entity}

	length := ec.SubstituteVariables(ctx, "$entity.triple."+predicate+".length")
	var listed []string
	if err := json.Unmarshal([]byte(ec.SubstituteVariables(ctx, "$entity.triple."+predicate+".triples")), &listed); err != nil {
		t.Fatalf(".triples of %v is not a JSON list of strings: %v", objects, err)
	}
	if length != strconv.Itoa(want) || length != strconv.Itoa(len(listed)) {
		t.Fatalf(".length of %v = %q, want %d, and .triples lists %d", objects, length, want, len(listed))
	}
}

func TestSubstitutionProperties(t *testing.T) {
	log := captureWarnings(t)
	ctx := t.Context()
	id := droneID(t)
	notePredicate := semantictest.Predicate(t, "test", "fixture", "note")
	listPredicate := semantictest.Predicate(t, "gather", "child", "completed")

	// Named witnesses: every token once, with values holding token text, and
	// each literal start before a token.
	witness := newTokenFixture(t, id, notePredicate, "see $message.secret and $entity.id", "ask $now.")
	checkReadOnce(ctx, t, log, witness, []string{"", " "}, witness.tokens[:1])
	for i, start := range literalUnits[:literalStarts] {
		literals := make([]string, len(witness.tokens)+1)
		for j := range literals {
			literals[j] = start
		}
		literals[0] = literalUnits[(i+5)%len(literalUnits)]
		checkReadOnce(ctx, t, log, witness, literals, witness.tokens)
	}
	checkNoDollar(ctx, t, log, witness.ec, "")
	checkNoDollar(ctx, t, log, witness.ec, "entity.id now. message.secret")
	checkLengthCountsTriples(ctx, t, id, listPredicate, nil)
	checkLengthCountsTriples(ctx, t, id, listPredicate, []storedObject{
		{object: "a", values: 1}, {object: 42, values: 1}, listObject("strings", []string{"x", "y"}),
		listObject("any", []string{"z"}), listObject("json", []string{"p", "q", "r"}), listObject("json", nil),
	})

	fragment := rapid.SampledFrom(valueFragments)
	value := rapid.Custom(func(rt *rapid.T) string {
		return strings.Join(rapid.SliceOfN(fragment, 0, 4).Draw(rt, "fragments"), "")
	})
	literal := rapid.Custom(func(rt *rapid.T) string {
		start := rapid.SampledFrom(literalUnits[:literalStarts]).Draw(rt, "start")
		rest := rapid.SliceOfN(rapid.SampledFrom(literalUnits), 0, 3).Draw(rt, "rest")
		return start + strings.Join(rest, "")
	})
	object := rapid.Custom(func(rt *rapid.T) storedObject {
		items := rapid.SliceOfN(rapid.StringMatching(`[a-z0-9 ,"]{0,4}`), 0, 4).Draw(rt, "items")
		switch rapid.SampledFrom([]string{"scalar", "number", "strings", "any", "json"}).Draw(rt, "shape") {
		case "scalar":
			return storedObject{object: rapid.StringMatching(`[a-z0-9 ,"]{0,6}`).Draw(rt, "scalar"), values: 1}
		case "number":
			return storedObject{object: rapid.IntRange(-5, 5).Draw(rt, "number"), values: 1}
		case "strings":
			return listObject("strings", items)
		case "any":
			return listObject("any", items)
		}
		return listObject("json", items)
	})

	rapid.Check(t, func(rt *rapid.T) {
		f := newTokenFixture(rt, id, notePredicate, value.Draw(rt, "note"), value.Draw(rt, "secret"))
		tokens := rapid.SliceOfN(rapid.SampledFrom(f.tokens), 0, 6).Draw(rt, "tokens")
		literals := make([]string, len(tokens)+1)
		for i := range literals {
			literals[i] = literal.Draw(rt, "literal")
		}
		literals[0] = rapid.SampledFrom([]string{"", literals[0]}).Draw(rt, "first")
		checkReadOnce(ctx, rt, log, f, literals, tokens)

		checkNoDollar(ctx, rt, log, f.ec, rapid.StringMatching(`[^$]{0,20}`).Draw(rt, "plain"))

		checkLengthCountsTriples(ctx, rt, id, listPredicate, rapid.SliceOfN(object, 0, 5).Draw(rt, "objects"))
	})
}
