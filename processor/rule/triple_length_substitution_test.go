package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-substitution/"The values of a predicate", for .length: the
// number of values .triples lists, so a join rule can compare against the size
// of the list it fanned out over instead of a fixed N.

// Every object shape a list can take, an absent predicate and an absent entity.
func TestTripleLengthSubstitution_TruthTable(t *testing.T) {
	cases := []struct {
		name     string
		template string
		entity   *gtypes.EntityState
		want     string
	}{
		{
			name:     "native []string of 3 → 3",
			template: "$entity.triple.test.fixture.subtopics.length",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []string{"a", "b", "c"}},
				},
			},
			want: "3",
		},
		{
			name:     "[]any of 5 → 5 (JSON-unmarshal shape)",
			template: "$entity.triple.test.fixture.subtopics.length",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []any{"a", "b", "c", "d", "e"}},
				},
			},
			want: "5",
		},
		{
			name:     "JSON-encoded string list → integer count (decide tool wire shape)",
			template: "$entity.triple.coordinator.decision.subtopics.length",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "coordinator", "decision", "subtopics"), Object: `["hydraulics","pneumatics","electrics","sensors"]`},
				},
			},
			want: "4",
		},
		{
			name:     "empty list → 0",
			template: "$entity.triple.test.fixture.subtopics.length",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []any{}},
				},
			},
			want: "0",
		},
		{
			name:     "missing predicate → 0",
			template: "$entity.triple.test.fixture.subtopics.length",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "other", "predicate"), Object: "x"},
				},
			},
			want: "0",
		},
		{
			name:     "entity nil → 0",
			template: "$entity.triple.test.fixture.subtopics.length",
			entity:   nil,
			want:     "0",
		},
		{
			name:     "deep predicate path (dots) → count",
			template: "$entity.triple.coordinator.decision.subtopics.length",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "coordinator", "decision", "subtopics"), Object: []string{"a", "b"}},
				},
			},
			want: "2",
		},
		{
			name:     "interpolated into a larger string",
			template: "expected=$entity.triple.test.fixture.subtopics.length children",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []string{"a", "b", "c"}},
				},
			},
			want: "expected=3 children",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ec := &ExecutionContext{
				EntityID: semantictest.EntityID(t, "test", "rule", "fixture", "substitution", "entity", "001"),
				Entity:   tc.entity,
			}
			got := ec.SubstituteVariables(t.Context(), tc.template)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A scalar triple is one value, so .length counts it as one.
func TestTripleLengthSubstitution_ScalarCountsAsOne(t *testing.T) {
	ec := &ExecutionContext{
		EntityID: semantictest.EntityID(t, "test", "rule", "fixture", "substitution", "entity", "001"),
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "test", "agent", "role"), Object: "researcher"},
			},
		},
	}
	got := ec.SubstituteVariables(t.Context(), "$entity.triple.test.agent.role.length")
	assert.Equal(t, "1", got)
}

// A bare triple token and a .length token in one template each resolve.
func TestTripleLengthSubstitution_NotInterferingWithOrdinarySubstitution(t *testing.T) {
	ec := &ExecutionContext{
		EntityID: semantictest.EntityID(t, "test", "rule", "fixture", "substitution", "entity", "001"),
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "test", "fixture", "subtopics"), Object: []string{"a", "b", "c"}},
				{Predicate: semantictest.Predicate(t, "test", "agent", "role"), Object: "coordinator"},
			},
		},
	}
	got := ec.SubstituteVariables(t.Context(), "role=$entity.triple.test.agent.role count=$entity.triple.test.fixture.subtopics.length")
	assert.Equal(t, "role=coordinator count=3", got)
}

func TestTripleLengthSubstitution_CanonicalHyphenatedPredicate(t *testing.T) {
	t.Parallel()

	ec := &ExecutionContext{Entity: &gtypes.EntityState{Triples: []message.Triple{
		{Predicate: semantictest.Predicate(t, "coordinator", "decision", "next-actions"), Object: []string{"review", "merge"}},
	}}}

	got := ec.SubstituteVariables(t.Context(), "$entity.triple.coordinator.decision.next-actions.length")
	assert.Equal(t, "2", got)
}
