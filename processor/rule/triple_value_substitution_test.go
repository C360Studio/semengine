package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-substitution/"The value of a 3-part predicate". .value is the
// suffix form only when the text before it is a valid three-part predicate
// (vocabulary.ParsePredicate); otherwise the whole path is the predicate.

// A present predicate gives its first object written with %v, an absent one "".
func TestTripleValueSubstitution_TruthTable(t *testing.T) {
	cases := []struct {
		name     string
		template string
		entity   *gtypes.EntityState
		want     string
	}{
		{
			name:     "present predicate resolves to scalar object value",
			template: "$entity.triple.openspec.change.revision.value",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "openspec", "change", "revision"), Object: "r42"},
				},
			},
			want: "r42",
		},
		{
			name:     "absent predicate resolves to empty string",
			template: "$entity.triple.openspec.change.revision.value",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "other", "unrelated", "field"), Object: "x"},
				},
			},
			want: "",
		},
		{
			name:     "non-string Object renders via canonical scalar formatting (fmt %v)",
			template: "$entity.triple.metrics.sample.count.value",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "metrics", "sample", "count"), Object: 42},
				},
			},
			want: "42",
		},
		{
			name:     "boolean Object renders via canonical scalar formatting",
			template: "$entity.triple.metrics.sample.flagged.value",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "metrics", "sample", "flagged"), Object: true},
				},
			},
			want: "true",
		},
		{
			name:     "multi-valued predicate resolves to first triple in entity.Triples order",
			template: "$entity.triple.gather.child.completed.value",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "gather", "child", "completed"), Object: "loop_a"},
					{Predicate: semantictest.Predicate(t, "gather", "child", "completed"), Object: "loop_b"},
				},
			},
			want: "loop_a",
		},
		{
			name:     "agent.lineage.<key>.value resolves (hyphenated property segment)",
			template: "$entity.triple.agent.lineage.research-reviewer.value",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: "agent.lineage.research-reviewer", Object: "loop-abc-123"},
				},
			},
			want: "loop-abc-123",
		},
		{
			name:     "literal predicate ending in .value is not truncated (2-part prefix doesn't parse)",
			template: "$entity.triple.metrics.sample.value",
			entity: &gtypes.EntityState{
				Triples: []message.Triple{
					{Predicate: semantictest.Predicate(t, "metrics", "sample", "value"), Object: "7"},
				},
			},
			want: "7",
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

// With no entity, as on the message path, .value is "".
func TestTripleValueSubstitution_NilEntityResolvesToEmpty(t *testing.T) {
	ec := &ExecutionContext{} // Entity nil
	got := ec.SubstituteVariables(t.Context(), "$entity.triple.openspec.change.revision.value")
	assert.Equal(t, "", got)
}

// An absent predicate's .value is "" and is not reported: .value is how a
// rule reads a predicate that may not be there.
func TestTripleValueSubstitution_AbsenceDoesNotWarn(t *testing.T) {
	log := captureWarnings(t)

	ec := &ExecutionContext{
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "openspec", "validated", "flag"), Object: "r42"},
			},
		},
	}

	got := ec.SubstituteVariables(t.Context(), "$entity.triple.openspec.change.revision.value")
	assert.Equal(t, "", got)
	assert.Nil(t, log.unresolved(t), "an absent predicate's .value is not reported")
}

// When the text before .value is not a three-part predicate, the whole path is
// the predicate; absent, it stays and is reported like any bare triple token.
func TestTripleValueSubstitution_LiteralFallbackAbsenceStillWarns(t *testing.T) {
	log := captureWarnings(t)

	// "a.b" is only 2 segments — does not parse as a canonical 3-part
	// predicate, so the whole sequence "a.b.value" is the literal
	// predicate under test. It is not present on the entity, so this
	// must behave exactly like any other unresolved literal predicate.
	ec := &ExecutionContext{
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "other", "unrelated", "field"), Object: "x"},
			},
		},
	}

	got := ec.SubstituteVariables(t.Context(), "$entity.triple.a.b.value")
	assert.Equal(t, "$entity.triple.a.b.value", got, "non-3-part prefix must not be treated as a .value suffix")
	assert.Equal(t, []string{"$entity.triple.a.b.value"}, log.unresolved(t))
}

// The suffix is the final .value: the predicate checked is everything before it.
func TestTripleValueSubstitution_SuffixIsTheFinalDotValue(t *testing.T) {
	// "a.value.b" is a legal 3-part predicate (domain="a", category=
	// "value", property="b"); the trailing ".value" is the actual
	// suffix, so the candidate under arity test is "a.value.b", not "a".
	ec := &ExecutionContext{
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: "a.value.b", Object: "resolved"},
			},
		},
	}
	got := ec.SubstituteVariables(t.Context(), "$entity.triple.a.value.b.value")
	assert.Equal(t, "resolved", got)
}

// A bare triple token and a .value token on the same predicate each resolve.
func TestTripleValueSubstitution_RunsBeforeGenericSubstitution(t *testing.T) {
	ec := &ExecutionContext{
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "openspec", "change", "revision"), Object: "r42"},
			},
		},
	}
	got := ec.SubstituteVariables(t.Context(), "plain=$entity.triple.openspec.change.revision value=$entity.triple.openspec.change.revision.value")
	assert.Equal(t, "plain=r42 value=r42", got)
}

// Several .value tokens in one template, present and absent, each resolve.
func TestTripleValueSubstitution_MultipleTokensInOneTemplate(t *testing.T) {
	ec := &ExecutionContext{
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "openspec", "change", "revision"), Object: "r42"},
			},
		},
	}
	tmpl := "rev=$entity.triple.openspec.change.revision.value missing=$entity.triple.openspec.change.author.value"
	got := ec.SubstituteVariables(t.Context(), tmpl)
	assert.Equal(t, "rev=r42 missing=", got)
}

// A bare triple token whose predicate is absent stays and is reported.
func TestTripleValueSubstitution_BareFormUnchanged(t *testing.T) {
	log := captureWarnings(t)

	ec := &ExecutionContext{
		Entity: &gtypes.EntityState{
			Triples: []message.Triple{
				{Predicate: semantictest.Predicate(t, "other", "unrelated", "field"), Object: "x"},
			},
		},
	}

	got := ec.SubstituteVariables(t.Context(), "$entity.triple.openspec.change.revision")
	assert.Equal(t, "$entity.triple.openspec.change.revision", got)
	assert.Equal(t, []string{"$entity.triple.openspec.change.revision"}, log.unresolved(t))
}
