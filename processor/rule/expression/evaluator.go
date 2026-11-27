package expression

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	gtypes "github.com/c360studio/semengine/graph"
)

// NewExpressionEvaluator creates a new expression evaluator with all supported operators
func NewExpressionEvaluator() *Evaluator {
	evaluator := &Evaluator{
		operators: make(map[string]operatorFunc),
	}

	// Register numeric operators
	evaluator.operators[OpEqual] = operatorEqual
	evaluator.operators[OpNotEqual] = operatorNotEqual
	evaluator.operators[OpLessThan] = operatorLessThan
	evaluator.operators[OpLessThanEqual] = operatorLessThanEqual
	evaluator.operators[OpGreaterThan] = operatorGreaterThan
	evaluator.operators[OpGreaterThanEqual] = operatorGreaterThanEqual

	// Register array/range operators
	evaluator.operators[OpIn] = operatorIn
	evaluator.operators[OpNotIn] = operatorNotIn
	evaluator.operators[OpBetween] = operatorBetween

	// OpTransition and OpRegexMatch are dispatched outside this map: a
	// transition reads $prev.* state, and a regex reads the pattern Compile
	// kept on the condition.

	// Register string operators
	evaluator.operators[OpContains] = operatorContains
	evaluator.operators[OpStartsWith] = operatorStartsWith
	evaluator.operators[OpEndsWith] = operatorEndsWith

	// Register list operators. A join counts the completion triples its
	// children stamp on the parent with length_eq.
	evaluator.operators[OpLengthEq] = operatorLengthEq
	evaluator.operators[OpLengthGt] = operatorLengthGt
	evaluator.operators[OpLengthGte] = operatorLengthGte
	evaluator.operators[OpLengthLt] = operatorLengthLt
	evaluator.operators[OpLengthLte] = operatorLengthLte
	evaluator.operators[OpArrayContains] = operatorArrayContains

	return evaluator
}

// RegisteredOperators returns the names of every operator the evaluator
// runs, including the two it dispatches outside its operator map (transition
// and regex). A rule loader checks operator names against it, so a name the
// loader accepts is one the evaluator can run.
func RegisteredOperators() map[string]struct{} {
	ev := NewExpressionEvaluator()
	out := make(map[string]struct{}, len(ev.operators)+1)
	for op := range ev.operators {
		out[op] = struct{}{}
	}
	out[OpTransition] = struct{}{}
	out[OpRegexMatch] = struct{}{}
	return out
}

// isArrayOperator reports whether the named operator reads every triple of
// a predicate as a list (fieldValues) rather than the first one
// (fieldValue). List operators count or search all the triples carrying a
// predicate; the others see only the first.
func isArrayOperator(op string) bool {
	switch op {
	case OpLengthEq, OpLengthGt, OpLengthGte, OpLengthLt, OpLengthLte, OpArrayContains:
		return true
	}
	return false
}

// Evaluate evaluates a logical expression against an entity state.
// Equivalent to EvaluateWithStateAndMessage with nil stateFields and nil
// messageFields — entity triples are the only resolution source.
func (e *Evaluator) Evaluate(entityState *gtypes.EntityState, expr LogicalExpression) (bool, error) {
	return e.EvaluateWithStateAndMessage(entityState, nil, nil, expr)
}

// EvaluateWithStateAndMessage evaluates a logical expression against the
// full field-resolution surface: entity triples, rule match-state
// (`$state.*`/`$prev.*`), and the inbound message payload (`$message.*`).
//
// Field resolution precedence per condition:
//
//  1. `$state.*` / `$prev.*` → stateFields map
//  2. `$message.<dotted.path>` → messageFields via deep-walk
//  3. Bare field name → entity triples first (when entity is non-nil);
//     falls through to messageFields if not present on the entity.
//     When entity is nil (message-path rules), bare names resolve from
//     messageFields directly.
//
// This is the single condition evaluator for both rule-level conditions
// (where the rule fires on message-path or entity-state events) and
// action-level `when` guards. See ADR-041 for the unification rationale.
//
// `$message.command` is the recommended form when both rule-level and
// when-clause use the same field, because it makes the resolution source
// explicit. Bare `command` works equivalently on message-path rules but
// implicitly prefers entity triples on entity-path rules.
func (e *Evaluator) EvaluateWithStateAndMessage(entityState *gtypes.EntityState, stateFields StateFields, messageFields MessageFields, expr LogicalExpression) (bool, error) {
	if len(expr.Conditions) == 0 {
		return true, nil // Empty condition list passes
	}

	results := make([]bool, len(expr.Conditions))

	// Evaluate each condition
	for i, condition := range expr.Conditions {
		result, err := e.evaluateConditionWithStateAndMessage(entityState, stateFields, messageFields, condition)
		if err != nil {
			return false, err
		}
		results[i] = result
	}

	// Apply logic operator
	switch expr.Logic {
	case LogicAnd, "": // No logic given means all conditions must hold.
		for _, result := range results {
			if !result {
				return false, nil
			}
		}
		return true, nil

	case LogicOr:
		for _, result := range results {
			if result {
				return true, nil
			}
		}
		return false, nil

	default:
		return false, &EvaluationError{
			Message: fmt.Sprintf("unsupported logic operator: %s", expr.Logic),
		}
	}
}

// evaluateConditionWithStateAndMessage resolves a single condition against
// the layered sources (stateFields, messageFields, entity triples) per
// the precedence rule documented on EvaluateWithStateAndMessage.
func (e *Evaluator) evaluateConditionWithStateAndMessage(entityState *gtypes.EntityState, stateFields StateFields, messageFields MessageFields, condition ConditionExpression) (bool, error) {
	// A regex condition not compiled from its current value is refused
	// before its field is looked up, so an absent field cannot turn the
	// refusal into a quiet false.
	if err := checkCompiled(condition); err != nil {
		return false, err
	}

	// 1. `$state.*`, `$prev.*` and `$entity.lifecycle.*` come from
	// stateFields, which the caller fills: rule match state, previous values
	// and lifecycle values it resolved itself. The prefixes are in
	// stateless.go, shared with EnsureStatelessResolvable.
	if hasStatefulPrefix(condition.Field) {
		fieldValue, exists := stateFields[condition.Field]
		if !exists {
			if condition.Required {
				return false, &EvaluationError{
					Field:   condition.Field,
					Message: "required state field not found",
					Err:     ErrRequiredFieldMissing,
				}
			}
			return false, nil
		}
		return e.applyOperator(condition, fieldValue)
	}

	// 2. `$message.<path>` — explicit message payload access.
	if strings.HasPrefix(condition.Field, "$message.") {
		path := strings.TrimPrefix(condition.Field, "$message.")
		fieldValue, exists := ExtractMessageValue(messageFields, path)
		if !exists {
			if condition.Required {
				return false, &EvaluationError{
					Field:   condition.Field,
					Message: "required message field not found",
					Err:     ErrRequiredFieldMissing,
				}
			}
			return false, nil
		}
		return e.applyOperator(condition, fieldValue)
	}

	// 3. Transition operator — entity-state-only (uses $prev.* from
	//    stateFields against the current entity triple value).
	if condition.Operator == OpTransition {
		return e.evaluateTransition(entityState, stateFields, condition)
	}

	// 4. Bare field name resolution.
	//    Entity-path (entity non-nil): triples first, fall through to
	//    messageFields if not present.
	//    Message-path (entity nil): messageFields only.
	//    List operators read every triple carrying the predicate, so a
	//    join that counts N completion triples sees all N.
	if entityState != nil {
		if isArrayOperator(condition.Operator) {
			if values, exists := fieldValues(entityState, condition.Field); exists {
				return e.applyOperator(condition, values)
			}
			// No triple carries the predicate: a list operator sees an
			// empty list, not a missing field, even when required, and
			// does not read the message. length_eq 0 then matches (no
			// child has completed yet).
			return e.applyOperator(condition, []interface{}{})
		}
		if value, exists := fieldValue(entityState, condition.Field); exists {
			return e.applyOperator(condition, value)
		}
		// Fall through to messageFields below.
	}

	if messageFields != nil {
		if fieldValue, ok := ExtractMessageValue(messageFields, condition.Field); ok {
			return e.applyOperator(condition, fieldValue)
		}
	}

	if condition.Required {
		if entityState == nil && messageFields == nil {
			return false, &EvaluationError{
				Field:   condition.Field,
				Message: "no entity state or message fields available",
				Err:     ErrRequiredFieldMissing,
			}
		}
		return false, &EvaluationError{
			Field:   condition.Field,
			Message: "required field not found",
			Err:     ErrRequiredFieldMissing,
		}
	}
	return false, nil
}

// applyOperator runs the registered operator function for the condition
// against fieldValue and returns the boolean result. Used by every field
// resolution branch in evaluateConditionWithStateAndMessage so error
// shapes stay uniform.
func (e *Evaluator) applyOperator(condition ConditionExpression, fieldValue any) (bool, error) {
	if condition.Operator == OpRegexMatch {
		return matchRegex(condition, fieldValue)
	}
	opFunc, ok := e.operators[condition.Operator]
	if !ok {
		return false, &EvaluationError{
			Field:    condition.Field,
			Operator: condition.Operator,
			Message:  "unsupported operator",
		}
	}
	result, err := opFunc(fieldValue, condition.Value)
	if err != nil {
		return false, &EvaluationError{
			Field:    condition.Field,
			Operator: condition.Operator,
			Message:  "operator execution failed",
			Err:      err,
		}
	}
	return result, nil
}

// evaluateTransition checks whether a field is transitioning from an allowed previous
// value to a specified target value. It requires:
//   - The current field value (from entity triples) equals condition.Value (the "to" state)
//   - The previous field value (from stateFields["$prev.<field>"]) is in condition.From
//
// If no previous value is tracked (first evaluation), returns false — a transition
// requires history. If condition.From is nil, any change from a different previous value
// to condition.Value is treated as a valid transition.
func (e *Evaluator) evaluateTransition(entityState *gtypes.EntityState, stateFields StateFields, condition ConditionExpression) (bool, error) {
	// Entity state is required for transition checks
	if entityState == nil {
		if condition.Required {
			return false, &EvaluationError{
				Field:    condition.Field,
				Operator: OpTransition,
				Message:  "entity state is nil, transition cannot be evaluated",
			}
		}
		return false, nil
	}

	// Get current field value from entity triples
	currentValue, exists := fieldValue(entityState, condition.Field)
	if !exists {
		return false, nil
	}

	// Check that current value matches the target ("to") value
	if compareValues(currentValue, condition.Value) != 0 {
		return false, nil
	}

	// Look up previous value from state fields
	prevKey := "$prev." + condition.Field
	prevValue, hasPrev := stateFields[prevKey]
	if !hasPrev {
		// First evaluation — no previous value tracked, can't detect a transition
		return false, nil
	}

	// If From is specified, check that previous value is in the allowed set
	if condition.From != nil {
		fromSlice := normalizeToSlice(condition.From)
		for _, allowed := range fromSlice {
			if compareValues(prevValue, allowed) == 0 {
				return true, nil
			}
		}
		// Previous value not in allowed From set
		return false, nil
	}

	// From is nil — any change to the target value counts as a valid transition
	// (but the previous value must differ from current to be a real transition)
	return compareValues(prevValue, currentValue) != 0, nil
}

// normalizeToSlice converts a value that may be a single item or a slice into []interface{}.
func normalizeToSlice(v interface{}) []interface{} {
	if arr, ok := v.([]interface{}); ok {
		return arr
	}
	return []interface{}{v}
}

// fieldValue returns the object of the entity's first triple carrying the
// predicate field.
func fieldValue(entityState *gtypes.EntityState, field string) (interface{}, bool) {
	for _, triple := range entityState.Triples {
		if triple.Predicate == field {
			return triple.Object, true
		}
	}
	return nil, false
}

// fieldValues returns the objects of every triple carrying the predicate
// field, in triple order, or false when none does. List operators read it:
// they count or search all the triples of a predicate, not the first.
func fieldValues(entityState *gtypes.EntityState, field string) ([]interface{}, bool) {
	var values []interface{}
	for _, triple := range entityState.Triples {
		if triple.Predicate == field {
			values = append(values, triple.Object)
		}
	}
	return values, len(values) > 0
}

// Operator implementations

func operatorEqual(fieldValue, compareValue interface{}) (bool, error) {
	return compareValues(fieldValue, compareValue) == 0, nil
}

func operatorNotEqual(fieldValue, compareValue interface{}) (bool, error) {
	return compareValues(fieldValue, compareValue) != 0, nil
}

func operatorLessThan(fieldValue, compareValue interface{}) (bool, error) {
	cmp, err := compareValuesWithError(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return cmp < 0, nil
}

func operatorLessThanEqual(fieldValue, compareValue interface{}) (bool, error) {
	cmp, err := compareValuesWithError(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return cmp <= 0, nil
}

func operatorGreaterThan(fieldValue, compareValue interface{}) (bool, error) {
	cmp, err := compareValuesWithError(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return cmp > 0, nil
}

func operatorGreaterThanEqual(fieldValue, compareValue interface{}) (bool, error) {
	cmp, err := compareValuesWithError(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return cmp >= 0, nil
}

func operatorContains(fieldValue, compareValue interface{}) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		fieldStr = fmt.Sprintf("%v", fieldValue)
	}

	compareStr, ok := compareValue.(string)
	if !ok {
		compareStr = fmt.Sprintf("%v", compareValue)
	}

	return strings.Contains(fieldStr, compareStr), nil
}

func operatorStartsWith(fieldValue, compareValue interface{}) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		fieldStr = fmt.Sprintf("%v", fieldValue)
	}

	compareStr, ok := compareValue.(string)
	if !ok {
		compareStr = fmt.Sprintf("%v", compareValue)
	}

	return strings.HasPrefix(fieldStr, compareStr), nil
}

func operatorEndsWith(fieldValue, compareValue interface{}) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		fieldStr = fmt.Sprintf("%v", fieldValue)
	}

	compareStr, ok := compareValue.(string)
	if !ok {
		compareStr = fmt.Sprintf("%v", compareValue)
	}

	return strings.HasSuffix(fieldStr, compareStr), nil
}

func operatorIn(fieldValue, compareValue interface{}) (bool, error) {
	arr, ok := compareValue.([]interface{})
	if !ok {
		return false, fmt.Errorf("in operator requires array value, got %T", compareValue)
	}
	for _, item := range arr {
		if compareValues(fieldValue, item) == 0 {
			return true, nil
		}
	}
	return false, nil
}

func operatorNotIn(fieldValue, compareValue interface{}) (bool, error) {
	result, err := operatorIn(fieldValue, compareValue)
	if err != nil {
		return false, err
	}
	return !result, nil
}

func operatorBetween(fieldValue, compareValue interface{}) (bool, error) {
	arr, ok := compareValue.([]interface{})
	if !ok || len(arr) != 2 {
		return false, fmt.Errorf("between operator requires array of [min, max], got %T", compareValue)
	}
	gteResult, err := operatorGreaterThanEqual(fieldValue, arr[0])
	if err != nil {
		return false, err
	}
	if !gteResult {
		return false, nil
	}
	return operatorLessThanEqual(fieldValue, arr[1])
}

// coerceToInt extracts an int from a comparison value. Accepts the
// JSON-unmarshal float64 default, plus the common int widths, plus
// strings (rule configs sometimes carry numeric literals as strings
// when loaded from substitution paths that pre-stringify). Returns
// an error for anything else so authoring mistakes surface loudly.
func coerceToInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case int32:
		return int(n), nil
	case float64:
		return int(n), nil
	case float32:
		return int(n), nil
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("cannot parse %q as integer: %w", n, err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("expected integer-coercible value, got %T", v)
	}
}

// arrayLen returns the length of a field value resolved via
// fieldValues. The resolver returns []interface{}, but
// stateFields / messageFields paths may pass other shapes — handle
// the common ones loudly.
func arrayLen(fieldValue interface{}) (int, error) {
	switch v := fieldValue.(type) {
	case nil:
		return 0, nil
	case []interface{}:
		return len(v), nil
	case []string:
		return len(v), nil
	case string:
		// A single scalar triple Object that arrived through the
		// non-array resolution path. Length-of-one semantics so a
		// rule that expects exactly-one-completion-stamp still
		// matches without authors having to special-case.
		return 1, nil
	default:
		return 0, fmt.Errorf("length operator requires array-shaped field, got %T", fieldValue)
	}
}

func operatorLengthEq(fieldValue, compareValue interface{}) (bool, error) {
	got, err := arrayLen(fieldValue)
	if err != nil {
		return false, err
	}
	want, err := coerceToInt(compareValue)
	if err != nil {
		return false, fmt.Errorf("length_eq compare value: %w", err)
	}
	return got == want, nil
}

func operatorLengthGt(fieldValue, compareValue interface{}) (bool, error) {
	got, err := arrayLen(fieldValue)
	if err != nil {
		return false, err
	}
	want, err := coerceToInt(compareValue)
	if err != nil {
		return false, fmt.Errorf("length_gt compare value: %w", err)
	}
	return got > want, nil
}

func operatorLengthLt(fieldValue, compareValue interface{}) (bool, error) {
	got, err := arrayLen(fieldValue)
	if err != nil {
		return false, err
	}
	want, err := coerceToInt(compareValue)
	if err != nil {
		return false, fmt.Errorf("length_lt compare value: %w", err)
	}
	return got < want, nil
}

func operatorLengthGte(fieldValue, compareValue interface{}) (bool, error) {
	got, err := arrayLen(fieldValue)
	if err != nil {
		return false, err
	}
	want, err := coerceToInt(compareValue)
	if err != nil {
		return false, fmt.Errorf("length_gte compare value: %w", err)
	}
	return got >= want, nil
}

func operatorLengthLte(fieldValue, compareValue interface{}) (bool, error) {
	got, err := arrayLen(fieldValue)
	if err != nil {
		return false, err
	}
	want, err := coerceToInt(compareValue)
	if err != nil {
		return false, fmt.Errorf("length_lte compare value: %w", err)
	}
	return got <= want, nil
}

// operatorArrayContains reports whether the array fieldValue contains
// an item equal to compareValue. Equality is compareValues == 0 to
// match the cross-type-coercion semantics the rest of the evaluator
// uses for `in` / `eq`. Scalar fieldValue degenerates to a
// single-element array (so a rule that expects one specific value can
// match without special-casing whether the predicate fired once or
// multiple times).
func operatorArrayContains(fieldValue, compareValue interface{}) (bool, error) {
	arr := normalizeToSlice(fieldValue)
	for _, item := range arr {
		if compareValues(item, compareValue) == 0 {
			return true, nil
		}
	}
	return false, nil
}

// Helper functions for value comparison

func compareValues(a, b interface{}) int {
	result, _ := compareValuesWithError(a, b)
	return result
}

func compareValuesWithError(a, b interface{}) (int, error) {
	// Try numeric comparison first
	aNum, aIsNum := toFloat64(a)
	bNum, bIsNum := toFloat64(b)

	if aIsNum && bIsNum {
		if aNum < bNum {
			return -1, nil
		} else if aNum > bNum {
			return 1, nil
		}
		return 0, nil
	}

	// Fallback to string comparison
	aStr := fmt.Sprintf("%v", a)
	bStr := fmt.Sprintf("%v", b)

	if aStr < bStr {
		return -1, nil
	} else if aStr > bStr {
		return 1, nil
	}
	return 0, nil
}

// toFloat64 coerces v to a float64: any numeric Go type, a json.Number, or a
// string strconv.ParseFloat accepts. A json.Number or string spelling NaN or
// an infinity is not a number; a float32 or float64 NaN or infinity is
// returned as it is. It returns false otherwise.
//
// Numeric strings compare as numbers because producers sometimes stamp a
// number as text, and as text "9.9" sorts after "10.0". The cost: two strings
// that both look numeric compare as numbers, so the version "1.10" is less
// than "1.9". A rule that needs text order uses regex or contains, not lt or
// gt.
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case json.Number:
		// A decoder with UseNumber yields json.Number; without this case
		// it would compare as text.
		f, err := val.Float64()
		if err != nil {
			return 0, false
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case string:
		// Empty string is never numeric — ParseFloat would error and
		// we'd return (0, false), but the empty check makes the intent
		// explicit and avoids a tight-loop allocation in
		// compareValuesWithError when both sides are empty strings.
		if val == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, false
		}
		// NaN/Inf rejection: ParseFloat accepts "NaN", "Inf",
		// "infinity" (case-insensitive). NaN is especially dangerous
		// because every comparison (==, <, >) returns false, which
		// compareValuesWithError converts to `return 0, nil` — silently
		// equivalent to "equal". Treat both as "not numeric" so the
		// fallback string-compare path handles them deterministically.
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
