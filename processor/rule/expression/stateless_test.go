package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureStatelessResolvable(t *testing.T) {
	cases := []struct {
		name              string
		condition         ConditionExpression
		lifecycleResolved bool
		refused           bool
	}{
		{"entity triple", ConditionExpression{Field: "robotics.battery.level", Operator: OpLessThan, Value: 20}, false, false},
		{"message field", ConditionExpression{Field: "$message.command", Operator: OpEqual, Value: "go"}, false, false},
		{"state field", ConditionExpression{Field: "$state.iteration", Operator: OpEqual, Value: 1}, false, true},
		{"state field with lifecycle", ConditionExpression{Field: "$state.iteration", Operator: OpEqual, Value: 1}, true, true},
		{"previous value", ConditionExpression{Field: "$prev.workflow.plan.status", Operator: OpEqual, Value: "x"}, false, true},
		{"transition", ConditionExpression{Field: "workflow.plan.status", Operator: OpTransition, Value: "done"}, true, true},
		{"lifecycle without manager", ConditionExpression{Field: "$entity.lifecycle.phase", Operator: OpEqual, Value: "run"}, false, true},
		{"lifecycle with manager", ConditionExpression{Field: "$entity.lifecycle.phase", Operator: OpEqual, Value: "run"}, true, false},
		{"entity prefix that is not lifecycle", ConditionExpression{Field: "$entity.id", Operator: OpEqual, Value: "x"}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := EnsureStatelessResolvable(tc.condition, tc.lifecycleResolved)
			if !tc.refused {
				assert.NoError(t, err)
				return
			}
			var evalErr *EvaluationError
			require.ErrorAs(t, err, &evalErr)
			assert.Equal(t, tc.condition.Field, evalErr.Field)
		})
	}
}
