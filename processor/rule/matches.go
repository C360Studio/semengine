package rule

import (
	"context"
	"errors"
	"fmt"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/processor/rule/expression"
)

// Matches reports whether the rule's conditions hold for an entity's state,
// with no rule processor running: a recovery pass asks it whether a rule
// pack still owes an entity work.
//
// Ask Matches rather than rebuilding the match from expression.Evaluator.
// Matches runs the match the rule processor fires on: it fills condition
// values such as $entity.triple.<predicate>.length from the entity, and
// treats an absent required field as no match. A bare evaluator does
// neither, and answers true for an empty condition list.
//
// Matches answers whether the rule is owed, not whether it would fire at
// this moment. A cooldown limits how often a running rule fires; a rule
// inside its cooldown still owes the entity its actions, so Matches neither
// applies a cooldown nor refuses a rule that has one.
//
// There are three answers:
//
//   - true, nil: the conditions hold.
//   - false, nil: nothing is owed. The rule is disabled, has no
//     entity.pattern (it runs on messages or a schedule), does not watch
//     this entity, or its conditions do not hold, an absent required field
//     included. Safe to act on.
//   - false and an error: the answer could not be found. A nil rule,
//     context or state, or a malformed entity ID; a condition a single
//     entity state cannot answer (`transition`, `$state.*`, `$prev.*`, and
//     `$entity.lifecycle.*`, which needs a lifecycle lookup Matches does not
//     have); a condition value whose tokens did not resolve; an operator
//     that failed. This says nothing about what is owed: leave the entity
//     alone.
//
// Matches writes nothing, so calling it is not observable in a running rule
// processor, and it is safe for concurrent use.
func (r *Rule) Matches(ctx context.Context, state *gtypes.EntityState) (bool, error) {
	switch {
	case r == nil:
		return false, errs.Classified(errs.ErrorInvalid, errors.New("rule.Matches: nil rule"))
	case ctx == nil:
		return false, errs.Classified(errs.ErrorInvalid, fmt.Errorf("rule.Matches: rule %q: nil context", r.def.ID))
	case state == nil:
		return false, errs.Classified(errs.ErrorInvalid, fmt.Errorf("rule.Matches: rule %q: nil entity state", r.def.ID))
	}
	if err := semtypes.ValidateEntityID(state.ID); err != nil {
		return false, errs.Classified(errs.ErrorInvalid,
			fmt.Errorf("rule.Matches: rule %q: entity ID %q: %w", r.def.ID, state.ID, err))
	}

	// The rule processor never evaluates these rules for this entity, so
	// nothing is owed.
	if !r.def.Enabled || r.def.Entity.Pattern == "" || len(r.def.Conditions) == 0 {
		return false, nil
	}
	inside, err := semtypes.MatchEntityIDPattern(r.def.Entity.Pattern, state.ID)
	if err != nil {
		return false, fmt.Errorf("rule.Matches: rule %q: entity pattern %q: %w", r.def.ID, r.def.Entity.Pattern, err)
	}
	if !inside {
		return false, nil
	}

	for _, condition := range r.def.Conditions {
		if err := expression.EnsureStatelessResolvable(condition, false); err != nil {
			return false, fmt.Errorf("rule.Matches: rule %q: %w", r.def.ID, err)
		}
	}
	return r.matchEntity(ctx, state, nil)
}

// matchEntity is the entity match: the one the rule processor fires on, and
// the one Matches asks. It answers on the conditions alone, since the rule
// processor picks a rule by its entity pattern before calling it. A disabled
// rule, or one with no conditions, does not match.
//
// lookup fills $entity.lifecycle.* fields and condition values for the
// entity; with a nil lookup they stay absent and unfilled. Condition values
// are filled under ctx, and one whose tokens do not resolve is an error. An
// absent required field is no match, with no error; any other evaluation
// error is returned with the rule ID.
func (r *Rule) matchEntity(ctx context.Context, state *gtypes.EntityState, lookup LifecycleManager) (bool, error) {
	if !r.def.Enabled || len(r.def.Conditions) == 0 {
		return false, nil
	}

	fields := expression.StateFields{}
	// shortcut: a failed lifecycle lookup leaves the lifecycle fields
	// absent and is not reported, until the firing path that owns the
	// lookup decides how a failure is reported
	_ = populateLifecycleStateFields(ctx, lookup, state.ID, fields)

	ec := &ExecutionContext{EntityID: state.ID, Entity: state, Lifecycle: lookup}
	conditions, err := substituteConditionValues(ctx, r.def.Conditions, ec)
	if err != nil {
		return false, fmt.Errorf("rule %q: %w", r.def.ID, err)
	}

	matched, err := expression.NewExpressionEvaluator().EvaluateWithStateAndMessage(state, fields, nil,
		expression.LogicalExpression{Conditions: conditions, Logic: r.def.Logic})
	switch {
	case err == nil:
		return matched, nil
	case errors.Is(err, expression.ErrRequiredFieldMissing):
		// Rules mark a field required to mean "match only when it is
		// present", so its absence is the ordinary no match.
		return false, nil
	}
	return false, fmt.Errorf("rule %q: %w", r.def.ID, err)
}
