package expression

import "strings"

// Stateful field prefixes.
//
// These name the fields read from StateFields rather than entity triples.
// evaluateConditionWithStateAndMessage branches on them to decide where a
// field comes from, and EnsureStatelessResolvable refuses on them so a caller
// that cannot supply state is told rather than answered. A second copy of the
// list would drift, and a prefix in one and not the other gives a confident
// false for a question nobody answered.
const (
	statePrefix     = "$state."
	prevPrefix      = "$prev."
	lifecyclePrefix = "$entity.lifecycle."
)

// hasStatefulPrefix reports whether a condition field is read from StateFields
// rather than from the entity's own triples.
func hasStatefulPrefix(field string) bool {
	return strings.HasPrefix(field, statePrefix) ||
		strings.HasPrefix(field, prevPrefix) ||
		strings.HasPrefix(field, lifecyclePrefix)
}

// EnsureStatelessResolvable reports whether a condition can be answered without a
// stateful evaluation, returning nil when it can and an error naming the offending
// field when it cannot.
//
// It exists because the evaluator's own behavior for an unresolved stateful field
// is correct for the stateful caller and wrong for a stateless one. When
// `Required` is unset, evaluateConditionWithStateAndMessage returns `false, nil`
// for a missing `$state.*` / `$prev.*` / `$entity.lifecycle.*` field. That is
// right when the caller SUPPLIED the state map and its absence is therefore
// meaningful. It is wrong when the caller never had the opportunity to supply
// one: it hands back a confident "no match" for a question that was never asked.
//
// The failure direction is what makes this a guard rather than a nicety. A
// consumer asking "does this rule pack still owe this entity a hop" reads a false
// negative as "this entity is stranded" and intervenes. Across a pack whose
// first-hop rule carries such a condition, that misclassifies every entity in the
// world. An error a caller can refuse on is recoverable; a fabricated verdict is
// not.
//
// lifecycleResolved reports whether the caller resolved the entity's lifecycle
// state into StateFields. Then `$entity.lifecycle.*` fields are answerable;
// otherwise they are refused, as they cannot be answered without a lifecycle
// lookup.
//
// This is a pre-scan, deliberately: it runs BEFORE evaluation and leaves the
// evaluator untouched, so the stateful path cannot regress in service of the
// stateless one.
func EnsureStatelessResolvable(condition ConditionExpression, lifecycleResolved bool) error {
	// The transition operator compares against $prev.* tracked across
	// evaluations. There is no stateless form of "did this change" — a single
	// EntityState is one observation, and one observation has no previous.
	if condition.Operator == OpTransition {
		return &EvaluationError{
			Field:    condition.Field,
			Operator: OpTransition,
			Message:  "transition requires stateful evaluation: a single entity state carries no previous value to compare against",
		}
	}

	if !hasStatefulPrefix(condition.Field) {
		return nil
	}

	// Lifecycle fields are the one stateful class a caller CAN make
	// answerable, by resolving them from a lifecycle lookup first.
	if strings.HasPrefix(condition.Field, lifecyclePrefix) {
		if lifecycleResolved {
			return nil
		}
		return &EvaluationError{
			Field:   condition.Field,
			Message: "lifecycle field cannot be answered without a lifecycle lookup",
		}
	}

	return &EvaluationError{
		Field:   condition.Field,
		Message: "field requires stateful evaluation: it is resolved from rule match state, which does not exist outside a running rule engine",
	}
}
