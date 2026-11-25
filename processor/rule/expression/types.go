// Package expression evaluates rule conditions and action when guards: a list
// of field, operator and value conditions combined by and or or, read against
// an entity's triples, the rule's state fields and the inbound message.
package expression

import (
	"fmt"
	"regexp"
)

// ConditionExpression represents a single field/operator/value condition
type ConditionExpression struct {
	Field    string      `json:"field"`    // Predicate field (e.g., "robotics.battery.level")
	Operator string      `json:"operator"` // Comparison operator (e.g., "lte", "eq", "contains")
	Value    interface{} `json:"value"`    // Comparison value (20.0, "active", true)
	// Required: when true, a missing field causes evaluation to error
	// rather than silently match false. Scalar operators (eq, lt,
	// contains, ...) honour this strictly: missing predicate +
	// Required=true → EvaluationError.
	//
	// List operators (length_*, array_contains; see isArrayOperator)
	// differ on an entity: a missing predicate is an empty list and the
	// operator runs against it, so length_eq 0 matches, length_gt N does
	// not, and array_contains is false. A join rule can then fire before
	// any child completes. To require the predicate, add a scalar
	// condition on the same field.
	Required bool        `json:"required"`
	From     interface{} `json:"from,omitempty"` // For transition operator: allowed previous value(s)

	// pattern is the regex Compile built from Value; evaluation reads it and
	// never compiles.
	pattern *regexp.Regexp
}

// LogicalExpression combines multiple conditions with logic operators
type LogicalExpression struct {
	Conditions []ConditionExpression `json:"conditions"`
	Logic      string                `json:"logic"` // "and" (also when empty) or "or"
}

// Evaluator processes expressions against entity state
type Evaluator struct {
	operators map[string]operatorFunc
}

// operatorFunc compares a resolved field value with a condition's value.
type operatorFunc func(fieldValue, compareValue interface{}) (bool, error)

// EvaluationError represents an error during expression evaluation
type EvaluationError struct {
	Field    string
	Operator string
	Message  string
	Err      error
}

func (e *EvaluationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("evaluation error for field '%s' with operator '%s': %s: %v",
			e.Field, e.Operator, e.Message, e.Err)
	}
	return fmt.Sprintf("evaluation error for field '%s' with operator '%s': %s",
		e.Field, e.Operator, e.Message)
}

func (e *EvaluationError) Unwrap() error {
	return e.Err
}

// Supported operators by field type
const (
	// Numeric operators
	OpEqual            = "eq"
	OpNotEqual         = "ne"
	OpLessThan         = "lt"
	OpLessThanEqual    = "lte"
	OpGreaterThan      = "gt"
	OpGreaterThanEqual = "gte"
	OpBetween          = "between"

	// String operators
	OpContains   = "contains"
	OpStartsWith = "starts_with"
	OpEndsWith   = "ends_with"
	OpRegexMatch = "regex"

	// Boolean operators (eq/ne only)

	// Array operators
	OpIn            = "in"
	OpNotIn         = "not_in"
	OpLengthEq      = "length_eq"
	OpLengthGt      = "length_gt"
	OpLengthGte     = "length_gte"
	OpLengthLt      = "length_lt"
	OpLengthLte     = "length_lte"
	OpArrayContains = "array_contains"

	// State transition operator
	OpTransition = "transition"
)

// Logic operators
const (
	LogicAnd = "and"
	LogicOr  = "or"
)

// StateFields provides rule match state values for $state.* pseudo-field resolution
// in condition expressions. Keys are the full field names (e.g., "$state.iteration").
// This avoids circular dependencies between the expression and rule packages.
type StateFields map[string]interface{}

// MessageFields carries the inbound NATS message payload for `$message.*`
// pseudo-field resolution in condition expressions. The map shape is
// `map[string]any` (matching `GenericJSONPayload.Data` and what
// `ExecutionContext.MessageData` carries through the rule pipeline).
//
// Deep-path access is supported: `$message.tool_args.command` walks the
// map along the dotted path. Bare field names (e.g. `command`) also
// resolve here when entity state is nil or when the field isn't present
// as an entity triple — see Evaluator.EvaluateWithStateAndMessage for the
// full precedence rule.
//
// Empty or nil maps cause `$message.*` and bare-name message lookups to
// return "not found" — same surfacing behaviour as missing entity triples
// (condition fails when Required=false, errors when Required=true).
type MessageFields map[string]any
