package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// List operators (length_*, array_contains) are checked as truth tables and
// through Evaluate, the path a rule's conditions take.

// --- operator truth tables ---

func TestOperatorLengthEq(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		field interface{}
		cmp   interface{}
		want  bool
	}{
		{"zero/zero", []interface{}{}, 0, true},
		{"zero/zero nil field", nil, 0, true},
		{"three/three", []interface{}{"a", "b", "c"}, 3, true},
		{"three/four", []interface{}{"a", "b", "c"}, 4, false},
		{"native []string three/three", []string{"a", "b", "c"}, 3, true},
		{"single string field length-of-one", "single", 1, true},
		{"compare value as float (JSON unmarshal shape)", []interface{}{"a", "b"}, float64(2), true},
		{"compare value as string-encoded integer", []interface{}{"a"}, "1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := operatorLengthEq(tc.field, tc.cmp)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOperatorLengthGt(t *testing.T) {
	t.Parallel()
	got, err := operatorLengthGt([]interface{}{"a", "b", "c"}, 2)
	require.NoError(t, err)
	assert.True(t, got)
	got, err = operatorLengthGt([]interface{}{"a"}, 1)
	require.NoError(t, err)
	assert.False(t, got, "length 1 is not > 1")
}

func TestOperatorLengthLt(t *testing.T) {
	t.Parallel()
	got, err := operatorLengthLt([]interface{}{"a"}, 2)
	require.NoError(t, err)
	assert.True(t, got)
	got, err = operatorLengthLt([]interface{}{"a", "b"}, 2)
	require.NoError(t, err)
	assert.False(t, got, "length 2 is not < 2")
}

func TestOperatorArrayContains(t *testing.T) {
	t.Parallel()
	got, err := operatorArrayContains([]interface{}{"alpha", "beta", "gamma"}, "beta")
	require.NoError(t, err)
	assert.True(t, got)

	got, err = operatorArrayContains([]interface{}{"alpha", "gamma"}, "beta")
	require.NoError(t, err)
	assert.False(t, got)

	// Scalar field degenerates to a one-element list — useful when a
	// predicate fired exactly once.
	got, err = operatorArrayContains("single", "single")
	require.NoError(t, err)
	assert.True(t, got, "scalar field should degenerate to a one-element list")
}

// TestOperatorLengthEq_NonArrayShapeErrors checks the loudness contract:
// passing a non-array, non-scalar-string shape returns an error rather
// than silently returning false. Authoring bugs surface.
func TestOperatorLengthEq_NonArrayShapeErrors(t *testing.T) {
	t.Parallel()
	_, err := operatorLengthEq(42, 1)
	assert.Error(t, err, "integer fieldValue is not array-shaped — must error")
}

// --- end-to-end via Evaluate ---

// TestEvaluator_LengthEqEndToEnd runs a length_eq condition through Evaluate
// and checks that it counts every triple of the predicate.
func TestEvaluator_LengthEqEndToEnd(t *testing.T) {
	t.Parallel()
	ev := NewExpressionEvaluator()

	// A parent entity with 3 child-completion triples. The join rule fires
	// when their count matches the expected number of children.
	entity := &gtypes.EntityState{
		Triples: []message.Triple{
			{Predicate: "test.gather.completed-subtopic", Object: "hydraulics"},
			{Predicate: "test.gather.completed-subtopic", Object: "pneumatics"},
			{Predicate: "test.gather.completed-subtopic", Object: "electrics"},
		},
	}

	got, err := ev.Evaluate(entity, LogicalExpression{
		Conditions: []ConditionExpression{
			{Field: "test.gather.completed-subtopic", Operator: OpLengthEq, Value: 3},
		},
		Logic: "and",
	})
	require.NoError(t, err)
	assert.True(t, got, "length_eq must match when N triples carry the predicate and value==N")
}

// TestEvaluator_LengthEqMissingPredicateIsZero verifies the
// zero-length semantic on a predicate the entity doesn't carry at
// all. A rule that fires the join action when count==0 (e.g. "no
// children spawned yet") is legitimate; the operator returns true
// rather than erroring on the missing-predicate path.
func TestEvaluator_LengthEqMissingPredicateIsZero(t *testing.T) {
	t.Parallel()
	ev := NewExpressionEvaluator()

	entity := &gtypes.EntityState{
		Triples: []message.Triple{
			{Predicate: "test.agent.role", Object: "coordinator"},
		},
	}

	got, err := ev.Evaluate(entity, LogicalExpression{
		Conditions: []ConditionExpression{
			{Field: "test.gather.completed-subtopic", Operator: OpLengthEq, Value: 0},
		},
		Logic: "and",
	})
	require.NoError(t, err)
	assert.True(t, got, "missing predicate must resolve to zero-length array for length_eq")
}

// TestEvaluator_ArrayContainsEndToEnd runs array_contains through Evaluate:
// "did the gather phase complete for this subtopic before I act on it?"
func TestEvaluator_ArrayContainsEndToEnd(t *testing.T) {
	t.Parallel()
	ev := NewExpressionEvaluator()

	entity := &gtypes.EntityState{
		Triples: []message.Triple{
			{Predicate: "test.gather.completed-subtopic", Object: "hydraulics"},
			{Predicate: "test.gather.completed-subtopic", Object: "pneumatics"},
		},
	}

	got, err := ev.Evaluate(entity, LogicalExpression{
		Conditions: []ConditionExpression{
			{Field: "test.gather.completed-subtopic", Operator: OpArrayContains, Value: "hydraulics"},
		},
		Logic: "and",
	})
	require.NoError(t, err)
	assert.True(t, got)

	// Negative case — predicate doesn't carry that value.
	got, err = ev.Evaluate(entity, LogicalExpression{
		Conditions: []ConditionExpression{
			{Field: "test.gather.completed-subtopic", Operator: OpArrayContains, Value: "electrics"},
		},
		Logic: "and",
	})
	require.NoError(t, err)
	assert.False(t, got)
}

// RegisteredOperators names exactly the declared operators, so a rule loader
// checking names against it accepts only what the evaluator runs.
func TestRegisteredOperators_NoSplitBrainBetweenDeclarationsAndWiring(t *testing.T) {
	t.Parallel()
	// Every Op* constant declared in types.go, written out by hand.
	declared := []string{
		OpEqual, OpNotEqual,
		OpLessThan, OpLessThanEqual, OpGreaterThan, OpGreaterThanEqual,
		OpContains, OpStartsWith, OpEndsWith, OpRegexMatch,
		OpIn, OpNotIn, OpBetween,
		OpLengthEq, OpLengthGt, OpLengthGte, OpLengthLt, OpLengthLte, OpArrayContains,
		OpTransition,
	}
	registered := RegisteredOperators()
	for _, op := range declared {
		_, ok := registered[op]
		assert.True(t, ok,
			"operator %q is declared in types.go but the evaluator does not run it", op)
	}
	assert.Len(t, registered, len(declared), "RegisteredOperators names an operator not declared here")
}

// TestIsArrayOperator pins which operators read every triple of a predicate.
func TestIsArrayOperator(t *testing.T) {
	t.Parallel()
	for _, op := range []string{OpLengthEq, OpLengthGt, OpLengthGte, OpLengthLt, OpLengthLte, OpArrayContains} {
		assert.True(t, isArrayOperator(op), "%s should be array operator", op)
	}
	for _, op := range []string{OpEqual, OpContains, OpIn, OpRegexMatch, OpTransition, "made_up_operator"} {
		assert.False(t, isArrayOperator(op), "%s should NOT be array operator", op)
	}
}

// length_gte and length_lte complete the length family. The boundary case
// (length == N) is the point: a "count >= budget B" route needs len == B to
// fire, which neither length_gt B nor length_eq B expresses alone.

func TestOperatorLengthGte(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		field interface{}
		cmp   interface{}
		want  bool
	}{
		{"boundary: length 2 >= 2", []interface{}{"a", "b"}, 2, true},
		{"above: length 3 >= 2", []interface{}{"a", "b", "c"}, 2, true},
		{"below: length 1 >= 2", []interface{}{"a"}, 2, false},
		{"zero field >= 0", []interface{}{}, 0, true},
		{"nil field >= 1", nil, 1, false},
		{"compare value as float (JSON unmarshal shape)", []interface{}{"a", "b"}, float64(2), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := operatorLengthGte(tc.field, tc.cmp)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOperatorLengthLte(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		field interface{}
		cmp   interface{}
		want  bool
	}{
		{"boundary: length 2 <= 2", []interface{}{"a", "b"}, 2, true},
		{"below: length 1 <= 2", []interface{}{"a"}, 2, true},
		{"above: length 3 <= 2", []interface{}{"a", "b", "c"}, 2, false},
		{"nil field <= 0", nil, 0, true},
		{"compare value as string-encoded integer", []interface{}{"a"}, "1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := operatorLengthLte(tc.field, tc.cmp)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestEvaluator_LengthGteBudgetBoundaryEndToEnd is an attempt budget: retry
// routes on count < B (length_lt), escalate on count >= B (length_gte). At
// exactly B attempts the escalate condition, and only it, matches.
func TestEvaluator_LengthGteBudgetBoundaryEndToEnd(t *testing.T) {
	t.Parallel()
	ev := NewExpressionEvaluator()

	const budget = 3
	entity := &gtypes.EntityState{
		Triples: []message.Triple{
			{Predicate: "test.task.attempt", Object: "attempt-1"},
			{Predicate: "test.task.attempt", Object: "attempt-2"},
			{Predicate: "test.task.attempt", Object: "attempt-3"},
		},
	}

	escalate, err := ev.Evaluate(entity, LogicalExpression{
		Conditions: []ConditionExpression{
			{Field: "test.task.attempt", Operator: OpLengthGte, Value: budget},
		},
		Logic: "and",
	})
	require.NoError(t, err)
	assert.True(t, escalate, "count == B must fire the >= B escalate route")

	retry, err := ev.Evaluate(entity, LogicalExpression{
		Conditions: []ConditionExpression{
			{Field: "test.task.attempt", Operator: OpLengthLt, Value: budget},
		},
		Logic: "and",
	})
	require.NoError(t, err)
	assert.False(t, retry, "count == B must NOT fire the < B retry route")
}
