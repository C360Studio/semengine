package expression

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Values reach the comparison operators as numbers, numeric text and
// json.Number. Expected results are written out by hand.
func TestComparisonCoercion(t *testing.T) {
	cases := []struct {
		name  string
		op    func(any, any) (bool, error)
		field any
		value any
		want  bool
	}{
		// Numeric text compares as a number: as text "9.9" sorts after "10.0".
		{"numeric strings", operatorLessThan, "9.9", "10.0", true},
		{"numeric string and number", operatorGreaterThan, "10", 9.5, true},
		{"version-like strings compare as numbers", operatorLessThan, "1.10", "1.9", true},
		{"empty string is text", operatorLessThan, "", "a", true},
		// NaN and the infinities are text, compared byte by byte.
		{"NaN equals NaN as text", operatorEqual, "NaN", "NaN", true},
		{"NaN is not less than 5", operatorLessThan, "NaN", 5, false},
		{"NaN sorts after 5 as text", operatorGreaterThan, "NaN", 5, true},
		{"Inf is text", operatorGreaterThan, "Inf", "9", true},
		// json.Number compares as a number.
		{"json.Number against float", operatorGreaterThan, json.Number("10"), 9.9, true},
		{"json.Number against json.Number", operatorLessThan, json.Number("9.9"), json.Number("10.0"), true},
		{"json.Number equal to int", operatorEqual, json.Number("3"), 3, true},
		{"json.Number NaN is text", operatorEqual, json.Number("NaN"), "NaN", true},
		{"invalid json.Number is text", operatorLessThan, json.Number("abc"), "abd", true},
		// Integer widths.
		{"int8 and uint64", operatorLessThan, int8(-1), uint64(1), true},
		{"float32 and int16", operatorEqual, float32(2), int16(2), true},
		{"uint8 uint16 uint32", operatorLessThanEqual, uint8(7), uint32(7), true},
		{"int32 and uint16", operatorGreaterThanEqual, int32(5), uint16(6), false},
		{"int64 and uint", operatorNotEqual, int64(4), uint(4), false},
		{"bool against text", operatorEqual, true, "true", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.op(tc.field, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A length operator's value is an integer given as any integer width, a
// float (truncated) or integer text; anything else is refused.
func TestLengthCompareValue(t *testing.T) {
	two := []any{"a", "b"}
	for _, value := range []any{2, int64(2), int32(2), 2.0, float32(2.9), "2"} {
		got, err := operatorLengthEq(two, value)
		require.NoError(t, err, "%T %v", value, value)
		assert.True(t, got, "%T %v", value, value)
	}
	for _, value := range []any{"two", true, nil, []any{2}} {
		_, err := operatorLengthEq(two, value)
		assert.Error(t, err, "%T %v", value, value)
	}

	got, err := operatorLengthEq([]string{"a", "b"}, 2)
	require.NoError(t, err)
	assert.True(t, got, "a []string has its length")
	got, err = operatorLengthEq("one", 1)
	require.NoError(t, err)
	assert.True(t, got, "a single text value is a list of one")
	got, err = operatorLengthEq(nil, 0)
	require.NoError(t, err)
	assert.True(t, got, "nil is an empty list")
}

func TestStringOperatorsOnNonStrings(t *testing.T) {
	cases := []struct {
		name  string
		op    func(any, any) (bool, error)
		value any
		want  bool
	}{
		{"contains", operatorContains, 3, true},
		{"starts_with", operatorStartsWith, 1, true},
		{"ends_with", operatorEndsWith, 5, true},
		{"ends_with miss", operatorEndsWith, 1, false},
	}
	for _, tc := range cases {
		got, err := tc.op(12345, tc.value)
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.want, got, "%s reads 12345 and %v as text", tc.name, tc.value)
	}
}

func TestEvaluationErrorText(t *testing.T) {
	_, err := NewExpressionEvaluator().Evaluate(nil, LogicalExpression{
		Conditions: []ConditionExpression{{Field: "$message.kind", Operator: OpIn, Value: "not a list"}},
		Logic:      LogicAnd,
	})
	assert.NoError(t, err, "a missing optional message field is false before the operator runs")

	_, err = NewExpressionEvaluator().EvaluateWithStateAndMessage(nil, nil, MessageFields{"kind": "a"},
		LogicalExpression{
			Conditions: []ConditionExpression{{Field: "$message.kind", Operator: OpIn, Value: "not a list"}},
			Logic:      LogicAnd,
		})
	var evalErr *EvaluationError
	require.ErrorAs(t, err, &evalErr)
	require.Error(t, evalErr.Err, "the operator's own error is kept")
	assert.Same(t, evalErr.Err, errors.Unwrap(evalErr))
	assert.Equal(t,
		"evaluation error for field '$message.kind' with operator 'in': operator execution failed: "+
			"in operator requires array value, got string",
		err.Error())

	plain := &EvaluationError{Field: "f", Operator: "eq", Message: "m"}
	assert.Equal(t, "evaluation error for field 'f' with operator 'eq': m", plain.Error())
	assert.NoError(t, errors.Unwrap(plain))
}
