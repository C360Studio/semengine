package rule

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/processor/rule/expression"
	"github.com/c360studio/semengine/vocabulary"
)

// The rule types a definition may name.
const (
	ruleTypeExpression = "expression"
	ruleTypeCron       = "cron"
)

// Rule is a definition that Load has checked: every condition and every
// action `when` passed the field check and compiled, and every action passed
// its handler's check.
type Rule struct {
	// def is the definition with its conditions compiled. Its action lists
	// are empty: the checked actions are in the lists below.
	def Definition

	onEnter    []loadedAction
	onExit     []loadedAction
	whileTrue  []loadedAction
	onRecovery []loadedAction
	actions    []loadedAction

	// unreadableReported holds the payload types this rule has reported it
	// cannot read (reportUnreadable). Messages are matched concurrently.
	unreadableReported sync.Map
}

// loadedAction is a checked action, its `when` compiled, with the key its
// firing count is kept under.
type loadedAction struct {
	Action
	key string
}

// Load checks def and returns the rule ready to run, or refuses it. source
// names where def came from (a rules file's path, `inline_rules[<index>]`,
// `rules.<id>@<revision>`). handlers must be a set NewHandlers built; each
// action is checked by its type's handler in it.
//
// A disabled rule is checked the same way, so a broken rule cannot wait,
// disabled, until someone enables it. Load reads def and never changes it:
// the rule it returns holds its own compiled copies, so one definition can be
// loaded any number of times.
//
// A refusal is classified invalid and names the rule ID, the source, the
// position (such as `on_enter[1].when[0]`) and the cause.
func Load(def Definition, source string, handlers Handlers) (*Rule, error) {
	refuse := func(position string, err error) (*Rule, error) {
		return nil, errs.Classified(errs.ErrorInvalid,
			fmt.Errorf("rule %q (source %s): %s: %w", def.ID, source, position, err))
	}
	if handlers.byType == nil {
		return refuse("handlers", errors.New("the handler set was not built by NewHandlers"))
	}
	// The rule ID is one token of the configuration bucket's `rules.<id>`
	// keys, so it must be a single KV literal token.
	if err := natsclient.ValidateKVLiteralToken(def.ID); err != nil {
		return refuse("id", fmt.Errorf(
			"a rule ID must be one KV literal token (ASCII letters, digits, '-', '/', '_', '='; no '.'): %w", err))
	}

	switch def.Type {
	case ruleTypeExpression:
		if position, err := checkExpressionRule(def); err != nil {
			return refuse(position, err)
		}
	case ruleTypeCron:
		// shortcut: enabled cron rules are refused, until the rule processor has a timer
		if def.Enabled {
			return refuse("type", errors.New(
				"cron rules are not yet supported: the rule processor has no timer to fire them; disable the rule"))
		}
	default:
		return refuse("type", fmt.Errorf("rule type %q is not %q or %q", def.Type, ruleTypeExpression, ruleTypeCron))
	}

	if def.FireEveryNEvents < 0 {
		return refuse("fire_every_n_events", fmt.Errorf("must be 0 or more, got %d", def.FireEveryNEvents))
	}
	if def.RerunOnRecovery && len(def.OnEnter) == 0 && len(def.OnRecovery) == 0 {
		return refuse("rerun_on_recovery", errors.New(
			"is true but neither on_enter nor on_recovery has an action to re-run"))
	}
	if def.Entity.Pattern != "" {
		if err := semtypes.ValidateEntityIDPattern(def.Entity.Pattern); err != nil {
			return refuse("entity.pattern", fmt.Errorf(
				"%q is not a six-position entity ID pattern (each position a literal segment or *): %w",
				def.Entity.Pattern, err))
		}
	}

	conditions := append([]expression.ConditionExpression(nil), def.Conditions...)
	for i := range conditions {
		position := fmt.Sprintf("conditions[%d]", i)
		if err := checkConditionField(conditions[i].Field); err != nil {
			return refuse(position, err)
		}
		if err := expression.Compile(conditions[i : i+1]); err != nil {
			return refuse(position, err)
		}
	}

	rule := &Rule{def: def}
	rule.def.Conditions = conditions
	rule.def.OnEnter, rule.def.OnExit, rule.def.WhileTrue, rule.def.OnRecovery, rule.def.Actions = nil, nil, nil, nil, nil
	for _, list := range []struct {
		label string
		in    []Action
		out   *[]loadedAction
	}{
		{"on_enter", def.OnEnter, &rule.onEnter},
		{"on_exit", def.OnExit, &rule.onExit},
		{"while_true", def.WhileTrue, &rule.whileTrue},
		{"on_recovery", def.OnRecovery, &rule.onRecovery},
		{"actions", def.Actions, &rule.actions},
	} {
		for i, a := range list.in {
			position := fmt.Sprintf("%s[%d]", list.label, i)
			loaded, at, err := loadAction(def.ID, a, handlers)
			if err != nil {
				return refuse(position+at, err)
			}
			*list.out = append(*list.out, loaded)
		}
	}
	return rule, nil
}

// checkExpressionRule checks what only an expression rule needs: at least
// one condition, each with a field and a known operator and reading only what
// the rule's path has, a logic of `and` or `or`, and a cooldown that is a
// duration. It returns the position of a
// refusal with its cause.
func checkExpressionRule(def Definition) (string, error) {
	if len(def.Conditions) == 0 {
		return "conditions", errors.New("an expression rule needs at least one condition")
	}
	operators := expression.RegisteredOperators()
	for i, cond := range def.Conditions {
		position := fmt.Sprintf("conditions[%d]", i)
		switch _, known := operators[cond.Operator]; {
		case cond.Field == "":
			return position, errors.New("the condition has no field")
		case cond.Operator == "":
			return position, errors.New("the condition has no operator")
		case !known:
			return position, fmt.Errorf("unknown operator %q", cond.Operator)
		}
		if err := checkConditionPath(def.Entity.Pattern != "", cond); err != nil {
			return position, err
		}
	}
	if def.Logic != "" && def.Logic != "and" && def.Logic != "or" {
		return "logic", fmt.Errorf("logic %q is not \"and\" or \"or\"", def.Logic)
	}
	if def.Cooldown != "" {
		if _, err := time.ParseDuration(def.Cooldown); err != nil {
			return "cooldown", fmt.Errorf("cooldown %q is not a duration: %w", def.Cooldown, err)
		}
	}
	return "", nil
}

// checkConditionPath refuses a condition that reads what the rule's path
// never has: a rule with no entity pattern runs on messages and has no
// entity, one with a pattern runs on entities and has no message. Such a
// field or value would never be filled, so the condition would quietly read
// nothing.
func checkConditionPath(onEntities bool, cond expression.ConditionExpression) error {
	absent, path, missing := "$entity.", "messages", "entity"
	if onEntities {
		absent, path, missing = "$message.", "entities", "message"
	}
	if strings.HasPrefix(cond.Field, absent) {
		return fmt.Errorf("the rule runs on %s, which have no %s, so the field %q can never be filled", path, missing, cond.Field)
	}
	if value, ok := cond.Value.(string); ok && strings.Contains(value, absent) {
		return fmt.Errorf("the rule runs on %s, which have no %s, so the value %q can never be filled", path, missing, value)
	}
	return nil
}

// checkConditionField refuses a condition, of the rule or of an action's
// `when`, on a predicate that is not declared, or that is rule-opaque: such a
// predicate holds an agent's private content and no rule may match on it
// (ADR-036). A field starting with `$` reads the message or the rule's state,
// not a predicate.
func checkConditionField(field string) error {
	if field == "" || strings.HasPrefix(field, "$") {
		return nil
	}
	if err := vocabulary.RequireDeclaredPredicate(field); err != nil {
		return fmt.Errorf("field: %w; a message or state field needs an explicit $message. or $state. prefix", err)
	}
	if vocabulary.IsRuleOpaque(field) {
		return fmt.Errorf("field %q is a rule-opaque predicate, which no rule condition or action guard may match", field)
	}
	return nil
}

// loadAction checks one action and returns it with its `when` compiled and
// its firing-cap key. On a refusal it returns the position inside the action
// (such as `.when[0]`), empty for the action itself, and the cause.
func loadAction(ruleID string, a Action, handlers Handlers) (loadedAction, string, error) {
	notDecoded := func(cause string) (loadedAction, string, error) {
		return loadedAction{}, "", fmt.Errorf("%s: build the action by decoding its JSON", cause)
	}
	if len(a.Body) == 0 {
		return notDecoded("the action has no body")
	}
	// The body is the action. The decode reads the four core keys from it,
	// and its result replaces a, so the rule holds its own copies.
	decoded, err := decodeAction(a.Body)
	if err != nil {
		return notDecoded(err.Error())
	}
	same, err := sameCore(a, decoded)
	if err != nil {
		return loadedAction{}, "", err
	}
	if !same {
		return notDecoded("the action's type, when, id or max_iterations differs from its body")
	}

	if decoded.MaxIterations != nil && *decoded.MaxIterations < 0 {
		return loadedAction{}, "", fmt.Errorf("max_iterations must be 0 or more (0 means no cap), got %d",
			*decoded.MaxIterations)
	}
	handler, ok := handlers.byType[decoded.Type]
	if !ok {
		return loadedAction{}, "", fmt.Errorf("action type %q is not in the handler set", decoded.Type)
	}
	if err := handler.Check(decoded); err != nil {
		return loadedAction{}, "", err
	}
	for i := range decoded.When {
		if err := checkConditionField(decoded.When[i].Field); err != nil {
			return loadedAction{}, fmt.Sprintf(".when[%d]", i), err
		}
		if err := expression.Compile(decoded.When[i : i+1]); err != nil {
			return loadedAction{}, fmt.Sprintf(".when[%d]", i), err
		}
	}
	key, err := capKey(ruleID, decoded)
	if err != nil {
		return loadedAction{}, "", err
	}
	return loadedAction{Action: decoded, key: key}, "", nil
}

// sameCore reports whether a's four core fields say what decoded's do,
// compared in marshalled form so that a regex compiled on a condition, which
// marshalling leaves out, does not count.
func sameCore(a, decoded Action) (bool, error) {
	marshal := func(x Action) ([]byte, error) {
		return json.Marshal(actionCore{Type: x.Type, When: x.When, ID: x.ID, MaxIterations: x.MaxIterations})
	}
	got, err := marshal(a)
	if err != nil {
		return false, fmt.Errorf("action fields: %w", err)
	}
	want, err := marshal(decoded)
	if err != nil {
		return false, fmt.Errorf("action body: %w", err)
	}
	return string(got) == string(want), nil
}
