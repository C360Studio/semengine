package rule

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-substitution/"The values of a predicate", for .triples: a
// JSON array of every value, so a join rule can hand the IDs of sibling runs
// to the run that collects them.

// Several scalar triples, one list-valued triple in each object shape, and the
// edge cases. The output is a JSON array, never a comma-joined list, since a
// value may hold a comma.
func TestTripleTriplesSubstitution_TruthTable(t *testing.T) {
	cases := []struct {
		name     string
		template string
		entity   *gtypes.EntityState
		want     string
	}{
		{
			name:     "N scalar triples → JSON array of N strings",
			template: "$entity.triple.gather.child.completed.triples",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "gather", "child", "completed"), Object: "loop_a"},
					{Predicate: semantictest.Predicate(t, "gather", "child", "completed"), Object: "loop_b"},
					{Predicate: semantictest.Predicate(t, "gather", "child", "completed"), Object: "loop_c"},
				},
			},
			want: `["loop_a","loop_b","loop_c"]`,
		},
		{
			name:     "one triple with a []string object → JSON array of elements",
			template: "$entity.triple.test.fixture.subtopics.triples",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []string{"hydraulics", "pneumatics", "electrics"}},
				},
			},
			want: `["hydraulics","pneumatics","electrics"]`,
		},
		{
			name:     "one triple with a []any object, as JSON decodes a list",
			template: "$entity.triple.test.fixture.subtopics.triples",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []any{"a", "b"}},
				},
			},
			want: `["a","b"]`,
		},
		{
			name:     "one triple holding a JSON list as a string",
			template: "$entity.triple.coordinator.decision.subtopics.triples",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "coordinator", "decision", "subtopics"), Object: `["alpha","beta","gamma"]`},
				},
			},
			want: `["alpha","beta","gamma"]`,
		},
		{
			name:     "Predicate not present → empty JSON array",
			template: "$entity.triple.test.never.set.triples",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "other", "predicate"), Object: "value"},
				},
			},
			want: "[]",
		},
		{
			name:     "one scalar triple → JSON array of one element",
			template: "$entity.triple.gather.child.completed.triples",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "gather", "child", "completed"), Object: "only_child"},
				},
			},
			want: `["only_child"]`,
		},
		{
			name:     "an empty list → empty JSON array",
			template: "$entity.triple.test.fixture.subtopics.triples",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []string{}},
				},
			},
			want: "[]",
		},
		{
			name:     "scalars that are not strings → written with %v",
			template: "$entity.triple.test.fixture.counts.triples",
			entity: &gtypes.EntityState{
				// Numeric scalar Objects get stringified via %v —
				// same path that []any non-string elements take in
				// coerceTripleObjectToStrings.
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "fixture", "counts"), Object: 42},
					{Predicate: semantictest.Predicate(t, "test", "fixture", "counts"), Object: 17},
				},
			},
			want: `["42","17"]`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ec := &ExecutionContext{Entity: tc.entity}
			got := ec.SubstituteVariables(t.Context(), tc.template)
			assert.Equal(t, tc.want, got)
		})
	}
}

// With no entity, as on the message path, .triples is [] and nothing is
// reported.
func TestTripleTriplesSubstitution_NilEntityResolvesToEmpty(t *testing.T) {
	ec := &ExecutionContext{} // Entity nil
	got := ec.SubstituteVariables(t.Context(), "$entity.triple.test.fixture.anything.triples")
	assert.Equal(t, "[]", got)
}

// A bare triple token and a .triples token on the same predicate each resolve.
func TestTripleTriplesSubstitution_RunsBeforeGenericSubstitution(t *testing.T) {
	ec := &ExecutionContext{
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []string{"a", "b", "c"}},
			},
		},
	}
	got := ec.SubstituteVariables(t.Context(), "plain=$entity.triple.test.fixture.subtopics triples=$entity.triple.test.fixture.subtopics.triples")
	// The bare token writes the list with %v; .triples writes JSON.
	assert.Equal(t, `plain=[a b c] triples=["a","b","c"]`, got)
}

// Several .triples tokens in one prompt each resolve. Substitution is text
// replacement: inside a JSON template the array arrives as a string the
// receiver parses.
func TestTripleTriplesSubstitution_MultipleTokensInOneTemplate(t *testing.T) {
	ec := &ExecutionContext{
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "test", "fixture", "first"), Object: "a"},
				{Predicate: semantictest.Predicate(t, "test", "fixture", "first"), Object: "b"},
				{Predicate: semantictest.Predicate(t, "test", "fixture", "second"), Object: "x"},
			},
		},
	}
	prompt := "Sibling IDs: $entity.triple.test.fixture.first.triples; Subtopics: $entity.triple.test.fixture.second.triples"
	got := ec.SubstituteVariables(t.Context(), prompt)
	assert.Equal(t, `Sibling IDs: ["a","b"]; Subtopics: ["x"]`, got)
}

func TestTripleTriplesSubstitution_CanonicalHyphenatedPredicate(t *testing.T) {
	t.Parallel()

	ec := &ExecutionContext{Entity: &gtypes.EntityState{Triples: []message.Triple{
		{Predicate: semantictest.Predicate(t, "gather", "child", "completed"), Object: "child-a"},
		{Predicate: semantictest.Predicate(t, "gather", "child", "completed"), Object: "child-b"},
	}}}

	got := ec.SubstituteVariables(t.Context(), "$entity.triple.gather.child.completed.triples")
	assert.Equal(t, `["child-a","child-b"]`, got)
}

// A list a tool encodes as JSON and stores as a string object reads back as
// the same list.
func TestCoerceTripleObjectToStrings_JSONRoundTrip(t *testing.T) {
	original := []string{"hydraulics", "pneumatics", "electrics"}
	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	got, ok := coerceTripleObjectToStrings(string(encoded))
	require.True(t, ok)
	assert.Equal(t, original, got)
}
