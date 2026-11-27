package expression

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-conditions/"An absent required field is told apart".
func TestErrRequiredFieldMissing(t *testing.T) {
	evaluator := NewExpressionEvaluator()
	id := semantictest.EntityID(t, "test", "rule", "expression", "required", "drone", "001")
	entity := createTestEntity(id, []message.Triple{{Subject: id, Predicate: "robotics.battery.level", Object: 5}})
	required := func(field, operator string, value any) LogicalExpression {
		return LogicalExpression{Conditions: []ConditionExpression{
			{Field: field, Operator: operator, Value: value, Required: true},
		}, Logic: LogicAnd}
	}

	// Each absent required field, one per place the evaluator looks a field up.
	missing := []struct {
		name     string
		entity   *gtypes.EntityState
		state    StateFields
		messages MessageFields
		expr     LogicalExpression
	}{
		{"a message field", nil, nil, MessageFields{"y": 1}, required("$message.x", OpEqual, "a")},
		{"a state field", entity, StateFields{}, nil, required("$state.iteration", OpEqual, 1)},
		{"an entity predicate", entity, nil, nil, required("robotics.battery.voltage", OpGreaterThan, 1)},
		{"a bare field with no entity or message", nil, nil, nil, required("robotics.battery.level", OpEqual, 5)},
	}
	for _, tc := range missing {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evaluator.EvaluateWithStateAndMessage(tc.entity, tc.state, tc.messages, tc.expr)
			var evalErr *EvaluationError
			require.ErrorAs(t, err, &evalErr)
			assert.True(t, errors.Is(err, ErrRequiredFieldMissing), "wraps the sentinel: %v", err)
			assert.False(t, got)
		})
	}

	// Errors that are not an absent field.
	other := []struct {
		name   string
		entity *gtypes.EntityState
		expr   LogicalExpression
	}{
		// gt and the other ordering operators fall back to comparing text,
		// so a length operator with a value that is not a number is the
		// operator failure.
		{"an operator failure", entity, required("robotics.battery.level", OpLengthEq, "abc")},
		{"a transition with no entity", nil, required("robotics.battery.level", OpTransition, 5)},
	}
	for _, tc := range other {
		t.Run(tc.name, func(t *testing.T) {
			_, err := evaluator.EvaluateWithStateAndMessage(tc.entity, nil, nil, tc.expr)
			var evalErr *EvaluationError
			require.ErrorAs(t, err, &evalErr)
			assert.False(t, errors.Is(err, ErrRequiredFieldMissing), "does not wrap the sentinel: %v", err)
		})
	}
}
