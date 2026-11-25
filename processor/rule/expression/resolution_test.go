package expression

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

func evaluateWith(t *testing.T, entity *gtypes.EntityState, state StateFields, msg MessageFields, cond ConditionExpression) (bool, error) {
	t.Helper()
	return NewExpressionEvaluator().EvaluateWithStateAndMessage(entity, state, msg,
		LogicalExpression{Conditions: []ConditionExpression{cond}, Logic: LogicAnd})
}

// On an entity, a list operator treats a missing predicate as an empty list,
// even when required, and does not read the message.
func TestListOperatorMissingPredicateIsEmptyList(t *testing.T) {
	entity := &gtypes.EntityState{Triples: []message.Triple{{Predicate: "test.agent.role", Object: "coordinator"}}}
	msg := nestedMessage("test.gather.done", []any{"a", "b"})

	cases := []struct {
		name string
		cond ConditionExpression
		want bool
	}{
		{"required length_eq 0", ConditionExpression{Field: "test.gather.done", Operator: OpLengthEq, Value: 0, Required: true}, true},
		{"length_gt 0", ConditionExpression{Field: "test.gather.done", Operator: OpLengthGt, Value: 0, Required: true}, false},
		{"length_lte 0", ConditionExpression{Field: "test.gather.done", Operator: OpLengthLte, Value: 0, Required: true}, true},
		{"array_contains", ConditionExpression{Field: "test.gather.done", Operator: OpArrayContains, Value: "a", Required: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evaluateWith(t, entity, nil, msg, tc.cond)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// With no entity, a bare field name is read from the message as a dotted
// path, list operators included.
func TestListOperatorWithoutEntityReadsMessage(t *testing.T) {
	msg := nestedMessage("test.gather.done", []any{"a", "b"})
	got, err := evaluateWith(t, nil, nil, msg,
		ConditionExpression{Field: "test.gather.done", Operator: OpLengthEq, Value: 2, Required: true})
	require.NoError(t, err)
	assert.True(t, got)
}

// A transition with no current or no previous value is false without an
// error, even when required; with no entity it is an error only when required.
func TestRequiredTransitionWithoutHistory(t *testing.T) {
	entity := &gtypes.EntityState{Triples: []message.Triple{{Predicate: "workflow.plan.status", Object: "drafting"}}}
	transition := func(field string) ConditionExpression {
		return ConditionExpression{Field: field, Operator: OpTransition, Value: "drafting", Required: true}
	}

	got, err := evaluateWith(t, entity, StateFields{}, nil, transition("workflow.plan.status"))
	require.NoError(t, err, "no previous value")
	assert.False(t, got)

	got, err = evaluateWith(t, entity, StateFields{"$prev.workflow.plan.other": "created"}, nil, transition("workflow.plan.other"))
	require.NoError(t, err, "no current value")
	assert.False(t, got)

	got, err = evaluateWith(t, entity, StateFields{"$prev.workflow.plan.status": "created"}, nil, transition("workflow.plan.status"))
	require.NoError(t, err)
	assert.True(t, got, "created to drafting, no from: any change counts")

	_, err = evaluateWith(t, nil, StateFields{"$prev.workflow.plan.status": "created"}, nil, transition("workflow.plan.status"))
	var evalErr *EvaluationError
	assert.ErrorAs(t, err, &evalErr, "no entity, required")
}

// $state.*, $prev.* and $entity.lifecycle.* are read from the state fields
// only, never from a triple or the message of the same name.
func TestStateFieldsReadOnlyFromState(t *testing.T) {
	for _, field := range []string{"$state.iteration", "$prev.workflow.plan.status", "$entity.lifecycle.phase"} {
		t.Run(field, func(t *testing.T) {
			entity := &gtypes.EntityState{Triples: []message.Triple{{Predicate: field, Object: "x"}}}
			msg := nestedMessage(field, "x")
			cond := ConditionExpression{Field: field, Operator: OpEqual, Value: "x"}

			got, err := evaluateWith(t, entity, StateFields{}, msg, cond)
			require.NoError(t, err)
			assert.False(t, got, "absent from state: false")

			cond.Required = true
			_, err = evaluateWith(t, entity, StateFields{}, msg, cond)
			var evalErr *EvaluationError
			assert.ErrorAs(t, err, &evalErr, "absent from state and required: error")

			got, err = evaluateWith(t, entity, StateFields{field: "x"}, msg, cond)
			require.NoError(t, err)
			assert.True(t, got)
		})
	}
}

func TestUnknownLogicAndOperatorAreRefused(t *testing.T) {
	entity := &gtypes.EntityState{Triples: []message.Triple{{Predicate: "a.b.c", Object: 1}}}
	cond := ConditionExpression{Field: "a.b.c", Operator: OpEqual, Value: 1}

	_, err := NewExpressionEvaluator().Evaluate(entity, LogicalExpression{Conditions: []ConditionExpression{cond}, Logic: "xor"})
	var evalErr *EvaluationError
	assert.ErrorAs(t, err, &evalErr, "logic xor")

	cond.Operator = "made_up"
	_, err = evaluateWith(t, entity, nil, nil, cond)
	assert.ErrorAs(t, err, &evalErr, "unknown operator")
}

// nestedMessage builds a message holding value at the dotted path.
func nestedMessage(path string, value any) MessageFields {
	parts := strings.Split(path, ".")
	current := value
	for i := len(parts) - 1; i > 0; i-- {
		current = map[string]any{parts[i]: current}
	}
	return MessageFields{parts[0]: current}
}
