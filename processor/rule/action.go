package rule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/processor/rule/expression"
)

// The built-in action types. A product adds its own through NewHandlers.
const (
	// ActionTypePublish publishes a message to a NATS subject.
	ActionTypePublish = "publish"
	// ActionTypeAddTriple creates a relationship triple in the graph.
	ActionTypeAddTriple = "add_triple"
	// ActionTypeRemoveTriple removes a relationship triple from the graph.
	ActionTypeRemoveTriple = "remove_triple"
	// ActionTypeUpdateTriple updates metadata on an existing triple.
	ActionTypeUpdateTriple = "update_triple"
	// ActionTypeReconcilePredicates replaces, or clears, the selected
	// predicate through the rule pack's bound projection target, which
	// reconciles the whole named group and leaves predicates outside it
	// alone. An empty object clears the group.
	ActionTypeReconcilePredicates = "reconcile_predicates"
	// ActionTypeUpdateKV writes JSON to a named KV bucket, optionally merging
	// it into the stored document.
	ActionTypeUpdateKV = "update_kv"
	// ActionTypeLifecycleTransition moves a lifecycle-managed entity to a new
	// phase, with optional set operations in the same update.
	ActionTypeLifecycleTransition = "lifecycle_transition"
	// ActionTypeLifecycleComplete moves a lifecycle-managed entity to the
	// first terminal phase reachable from its current phase.
	ActionTypeLifecycleComplete = "lifecycle_complete"
	// ActionTypeLifecycleFail moves a lifecycle-managed entity to its declared
	// failed phase, carrying the reason.
	ActionTypeLifecycleFail = "lifecycle_fail"
)

// builtinActionTypes lists every built-in type, for NewHandlers.
var builtinActionTypes = []string{
	ActionTypePublish,
	ActionTypeAddTriple,
	ActionTypeRemoveTriple,
	ActionTypeUpdateTriple,
	ActionTypeReconcilePredicates,
	ActionTypeUpdateKV,
	ActionTypeLifecycleTransition,
	ActionTypeLifecycleComplete,
	ActionTypeLifecycleFail,
}

// Action is one action of a rule. Body is the action: the JSON object as
// written, kept byte for byte by the decode and written back by MarshalJSON.
// Type, When, ID and MaxIterations are read-only copies of the four keys the
// engine reads, filled by the decode from Body; every other key in Body
// belongs to the handler of the action's type. Build an Action by decoding
// its JSON, never as a Go literal: changing a copied field changes nothing
// the action does, and loading refuses an action whose fields differ from
// its Body.
type Action struct {
	// Type is the `type` key: the name of the handler that checks and runs
	// the action.
	Type string

	// When is the `when` key: an optional guard. All its conditions must
	// match for the action to run; an action without one always runs. A
	// field resolves as in the rule's own conditions: `$state.<field>` and
	// `$prev.<field>` read the rule's match state, `$message.<path>` the
	// inbound message, and a bare name the entity's triples first, then the
	// message. As in the rule's conditions, Load refuses a bare name that is
	// not a declared predicate, or is a rule-opaque one.
	When []expression.ConditionExpression

	// ID is the `id` key: an optional firing-cap key. Without it the key is a
	// hash of the rule ID and the whole Body, so any edit to the action
	// starts a fresh count. Set it to keep a count across edits, or to share
	// one count between actions.
	ID string

	// MaxIterations is the `max_iterations` key: the firing cap, the most
	// times the rule processor that runs rules may fire the action for one
	// rule and entity match. Absent (nil) means defaultActionMaxIterations,
	// 0 means no cap. Load refuses a negative value.
	MaxIterations *int

	// Body is the action's JSON object as written.
	Body json.RawMessage
}

// actionCore holds the four keys the engine reads from an action's body.
type actionCore struct {
	Type          string                           `json:"type"`
	When          []expression.ConditionExpression `json:"when"`
	ID            string                           `json:"id"`
	MaxIterations *int                             `json:"max_iterations"`
}

// UnmarshalJSON keeps data as the action's Body and fills the four core
// fields from it. A key repeated in any object of the body is refused, naming
// the key and its object's path. A key in a `when` condition that a condition
// does not declare, or declares with another case, is refused, naming
// `when[<i>]`; any other key is left to the handler of the action's type.
func (a *Action) UnmarshalJSON(data []byte) error {
	decoded, err := decodeAction(data)
	if err != nil {
		return err
	}
	*a = decoded
	return nil
}

// MarshalJSON writes the action's Body. It refuses an action with no Body,
// which was not built by decoding.
func (a Action) MarshalJSON() ([]byte, error) {
	if len(a.Body) == 0 {
		return nil, errors.New("action has no body: build an action by decoding its JSON")
	}
	return a.Body, nil
}

// decodeAction is the one reading of an action's body: it returns the action
// whose core fields are what body says, with a copy of body as its Body.
// Core keys, and the keys of each `when` condition, are matched exactly as
// written.
func decodeAction(body []byte) (Action, error) {
	if err := checkNoDuplicateKeys(body, "action"); err != nil {
		return Action{}, err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(body, &keys); err != nil {
		return Action{}, fmt.Errorf("action: %w", err)
	}
	if keys == nil {
		return Action{}, errors.New("action: must be a JSON object, not null")
	}
	var core actionCore
	for _, field := range []struct {
		key string
		dst any
	}{
		{"type", &core.Type},
		{"id", &core.ID},
		{"max_iterations", &core.MaxIterations},
	} {
		raw, ok := keys[field.key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(raw, field.dst); err != nil {
			return Action{}, fmt.Errorf("action key %q: %w", field.key, err)
		}
	}
	if raw, ok := keys["when"]; ok {
		var conds []json.RawMessage
		if err := json.Unmarshal(raw, &conds); err != nil {
			return Action{}, fmt.Errorf("action key %q: %w", "when", err)
		}
		for i, condRaw := range conds {
			var cond expression.ConditionExpression
			decoder := json.NewDecoder(bytes.NewReader(condRaw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&cond); err != nil {
				return Action{}, fmt.Errorf("when[%d]: %w", i, err)
			}
			if err := checkObjectKeys(condRaw, conditionKeys); err != nil {
				return Action{}, fmt.Errorf("when[%d]: %w", i, err)
			}
			core.When = append(core.When, cond)
		}
	}
	return Action{
		Type:          core.Type,
		When:          core.When,
		ID:            core.ID,
		MaxIterations: core.MaxIterations,
		Body:          append(json.RawMessage(nil), body...),
	}, nil
}

// ActionHandler checks the actions of one type when a rule loads. Check reads
// only the action, never a dependency, and does no I/O; it refuses a body the
// type cannot run.
type ActionHandler interface {
	Check(a Action) error
}

// Handlers is the set of action types a rule may use, with the handler for
// each. NewHandlers is the only way to build one; a zero Handlers is not a
// set.
type Handlers struct {
	byType map[string]ActionHandler
}

// NewHandlers builds the handler set: the built-in types plus the product's.
// It refuses a product type named exactly as a built-in one, an empty name
// and a nil handler, classified invalid and naming the type. Later changes
// to product do not change the set.
func NewHandlers(product map[string]ActionHandler) (Handlers, error) {
	byType := make(map[string]ActionHandler, len(builtinActionTypes)+len(product))
	for _, name := range builtinActionTypes {
		byType[name] = builtinHandler{actionType: name}
	}
	names := make([]string, 0, len(product))
	for name := range product {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		handler := product[name]
		switch {
		case name == "":
			return Handlers{}, errs.Classified(errs.ErrorInvalid,
				errors.New("action handler: a type name must not be empty"))
		case handler == nil:
			return Handlers{}, errs.Classified(errs.ErrorInvalid,
				fmt.Errorf("action handler %q: the handler is nil", name))
		}
		if _, builtin := byType[name]; builtin {
			return Handlers{}, errs.Classified(errs.ErrorInvalid,
				fmt.Errorf("action handler %q: the name is a built-in action type", name))
		}
		byType[name] = handler
	}
	return Handlers{byType: byType}, nil
}
