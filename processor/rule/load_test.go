package rule

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/pkg/errs"
)

const loadSource = "inline_rules[3]"

func decodeTestRule(t *testing.T, rule string) Definition {
	t.Helper()
	def, err := DecodeDefinition([]byte(rule), loadSource)
	require.NoError(t, err)
	return def
}

func builtinHandlers(t *testing.T) Handlers {
	t.Helper()
	h, err := NewHandlers(nil)
	require.NoError(t, err)
	return h
}

func TestLoadRefuses(t *testing.T) {
	declareTestPredicates()
	h := builtinHandlers(t)

	// Each case is a whole rule that fails one check. position is where the
	// refusal must point, cause a phrase that says what is wrong.
	cases := []struct {
		name     string
		rule     string
		position string
		cause    string
		// disabledOnly marks a case run only with the rule disabled: an
		// enabled cron rule is refused before its actions are checked.
		disabledOnly bool
	}{
		{
			name: "an unknown action type",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "on_enter": [{"type": "publish", "subject": "a.b"}, {"type": "page_oncall"}]}`,
			position: "on_enter[1]",
			cause:    `"page_oncall"`,
		},
		{
			name: "an action with no type",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "on_exit": [{"Type": "publish"}]}`,
			position: "on_exit[0]",
			cause:    "not in the handler set",
		},
		{
			name: "a bad regex in a condition",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2},
			                 {"field": "$message.name", "operator": "regex", "value": "["}]}`,
			position: "conditions[1]",
			cause:    "invalid regex",
		},
		{
			name: "a templated regex in a when condition",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "on_enter": [{"type": "publish", "subject": "a.b"},
			    {"type": "publish", "subject": "a.c", "when": [
			      {"field": "$message.name", "operator": "regex", "value": "^$message.prefix"}]}]}`,
			position: "on_enter[1].when[0]",
			cause:    "substitution token",
		},
		{
			name: "a rule-opaque condition field",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.private.note", "operator": "eq", "value": "x"}]}`,
			position: "conditions[0]",
			cause:    "rule-opaque",
		},
		{
			name: "an undeclared condition field",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.undeclared", "operator": "eq", "value": "x"}]}`,
			position: "conditions[0]",
			cause:    "not declared",
		},
		{
			name: "a condition field that is not a predicate",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "status", "operator": "eq", "value": "x"}]}`,
			position: "conditions[0]",
			cause:    "$message.",
		},
		{
			name:     "a dotted ID",
			rule:     `{"id": "r1.v2", "type": "expression", "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
			position: "id",
			cause:    "KV literal token",
		},
		{
			name: "a negative fire_every_n_events",
			rule: `{"id": "r1", "type": "expression", "fire_every_n_events": -1,
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
			position: "fire_every_n_events",
			cause:    "-1",
		},
		{
			name: "a negative action max_iterations",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "while_true": [{"type": "publish", "subject": "a.b", "max_iterations": -1}]}`,
			position: "while_true[0]",
			cause:    "max_iterations",
		},
		{
			name: "rerun_on_recovery with nothing to re-run",
			rule: `{"id": "r1", "type": "expression", "rerun_on_recovery": true,
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "while_true": [{"type": "publish", "subject": "a.b"}]}`,
			position: "rerun_on_recovery",
			cause:    "neither on_enter nor on_recovery",
		},
		{
			name: "a misspelt setting",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "on_exit": [{"type": "add_triple", "predicat": "test.alert.active", "object": "true"}]}`,
			position: "on_exit[0]",
			cause:    `"predicat"`,
		},
		{
			name: "a reserved publish subject",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "on_recovery": [{"type": "publish", "subject": "user.response.x"}]}`,
			position: "on_recovery[0]",
			cause:    "user.response.>",
		},
		{
			name: "a dynamic predicate",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "on_enter": [{"type": "add_triple", "predicate": "$message.p", "object": "true"}]}`,
			position: "on_enter[0]",
			cause:    "declared literal",
		},
		{
			name:     "an expression rule with no conditions",
			rule:     `{"id": "r1", "type": "expression", "on_enter": [{"type": "publish", "subject": "a.b"}]}`,
			position: "conditions",
			cause:    "at least one condition",
		},
		{
			name: "a condition with an unknown operator",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "about", "value": 2}]}`,
			position: "conditions[0]",
			cause:    `"about"`,
		},
		{
			name: "a condition with no operator",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "value": 2}]}`,
			position: "conditions[0]",
			cause:    "no operator",
		},
		{
			name: "a condition with no field",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"operator": "eq", "value": 2}]}`,
			position: "conditions[0]",
			cause:    "field",
		},
		{
			name: "an unknown logic",
			rule: `{"id": "r1", "type": "expression", "logic": "xor",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
			position: "logic",
			cause:    `"xor"`,
		},
		{
			name: "a cooldown that is not a duration",
			rule: `{"id": "r1", "type": "expression", "cooldown": "soon",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
			position: "cooldown",
			cause:    `"soon"`,
		},
		{
			name: "an entity pattern that is not six positions",
			rule: `{"id": "r1", "type": "expression", "entity": {"pattern": "c360.test.*"},
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
			position: "entity.pattern",
			cause:    "c360.test.*",
		},
		{
			name: "an entity pattern ending in a terminal wildcard",
			rule: `{"id": "r1", "type": "expression", "entity": {"pattern": "*.robotics.>"},
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
			position: "entity.pattern",
			cause:    "*.robotics.>",
		},
		{
			name: "an action predicate with an underscore",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "on_enter": [{"type": "add_triple", "subject": "$entity.id",
			    "predicate": "workflow.state.next_phase", "object": "done"}]}`, // predicate-audit:invalid {"kind":"stored-predicate","value":"workflow.state.next_phase","reason":"segment_character"}
			position: "on_enter[0]",
			cause:    "segment_character",
		},
		{
			name: "an unknown rule type",
			rule: `{"id": "r1", "type": "state_machine",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
			position: "type",
			cause:    `"state_machine"`,
		},
		{
			name:         "a cron rule action of an unknown type",
			rule:         `{"id": "r1", "type": "cron", "enabled": false, "schedule": "@daily", "actions": [{"type": "page_oncall"}]}`,
			position:     "actions[0]",
			cause:        `"page_oncall"`,
			disabledOnly: true,
		},
	}
	for _, tc := range cases {
		states := []bool{true, false}
		if tc.disabledOnly {
			states = []bool{false}
		}
		for _, enabled := range states {
			name := tc.name
			if !enabled {
				name += ", disabled"
			}
			t.Run(name, func(t *testing.T) {
				def := decodeTestRule(t, tc.rule)
				def.Enabled = enabled
				rule, err := Load(def, loadSource, h)
				require.Error(t, err)
				assert.Nil(t, rule)
				assertLoadRefusal(t, err, def.ID, tc.position, tc.cause)
			})
		}
	}

	t.Run("an enabled cron rule", func(t *testing.T) {
		def := decodeTestRule(t, `{"id": "r1", "type": "cron", "schedule": "@daily",
		  "actions": [{"type": "publish", "subject": "reports.daily"}]}`)
		_, err := Load(def, loadSource, h)
		require.Error(t, err)
		assertLoadRefusal(t, err, "r1", "type", "cron rules are not yet supported")
	})

	t.Run("a handler set NewHandlers did not build", func(t *testing.T) {
		def := decodeTestRule(t, `{"id": "r1", "type": "expression",
		  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`)
		_, err := Load(def, loadSource, Handlers{})
		require.Error(t, err)
		assert.True(t, errs.IsInvalid(err), "classified invalid: %v", err)
		for _, want := range []string{`"r1"`, loadSource, "NewHandlers"} {
			assert.Contains(t, err.Error(), want)
		}
	})

	t.Run("a disabled cron rule loads", func(t *testing.T) {
		def := decodeTestRule(t, `{"id": "nightly", "type": "cron", "enabled": false, "schedule": "@daily",
		  "actions": [{"type": "publish", "subject": "reports.nightly"}]}`)
		rule, err := Load(def, loadSource, h)
		require.NoError(t, err)
		assert.NotNil(t, rule)
	})

	t.Run("a product handler's refusal names the position", func(t *testing.T) {
		product, err := NewHandlers(map[string]ActionHandler{"page_oncall": refusingHandler{}})
		require.NoError(t, err)
		def := decodeTestRule(t, `{"id": "r1", "type": "expression",
		  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
		  "on_enter": [{"type": "page_oncall"}]}`)
		_, err = Load(def, loadSource, product)
		require.Error(t, err)
		assertLoadRefusal(t, err, "r1", "on_enter[0]", "no rota named")
	})
}

// TestLoadAccepts loads rules that sit next to a refusal: each differs from
// a TestLoadRefuses case only in the value that case refuses.
func TestLoadAccepts(t *testing.T) {
	declareTestPredicates()
	h := builtinHandlers(t)

	cases := []struct {
		name string
		rule string
	}{
		{
			name: "an ID with a hyphen",
			rule: `{"id": "a-b", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
		},
		{
			name: "a six-position entity pattern with wildcards",
			rule: `{"id": "r1", "type": "expression", "entity": {"pattern": "*.*.robotics.*.drone.*"},
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}]}`,
		},
		{
			name: "a condition on a predicate beside a rule-opaque one",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "` + semantictest.Predicate(t, "test", "private", "status") + `",
			    "operator": "eq", "value": "completed"}]}`,
		},
		{
			name: "an action predicate with a hyphen",
			rule: `{"id": "r1", "type": "expression",
			  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
			  "on_enter": [{"type": "add_triple", "subject": "$entity.id",
			    "predicate": "` + semantictest.Predicate(t, "workflow", "state", "next-phase") + `", "object": "done"}]}`,
		},
	}
	for _, tc := range cases {
		for _, enabled := range []bool{true, false} {
			name := tc.name
			if !enabled {
				name += ", disabled"
			}
			t.Run(name, func(t *testing.T) {
				def := decodeTestRule(t, tc.rule)
				def.Enabled = enabled
				rule, err := Load(def, loadSource, h)
				require.NoError(t, err)
				assert.NotNil(t, rule)
			})
		}
	}
}

// TestLoadRefusesReservedSubject puts a publish action with a subject in
// user.response.> in each action list, and expects each refused at its
// position.
func TestLoadRefusesReservedSubject(t *testing.T) {
	declareTestPredicates()
	h := builtinHandlers(t)

	const expressionRule = `{"id": "reserved-subject-rule", "type": "expression",
	  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}], %q: [%s]}`
	// A cron rule's list is "actions"; the rule is disabled, as an enabled
	// cron rule is refused before its actions are checked.
	const cronRule = `{"id": "reserved-subject-rule", "type": "cron", "enabled": false, "schedule": "@daily",
	  %q: [%s]}`
	lists := map[string]string{
		"on_enter":    expressionRule,
		"on_exit":     expressionRule,
		"while_true":  expressionRule,
		"on_recovery": expressionRule,
		"actions":     cronRule,
	}
	for list, rule := range lists {
		for _, subject := range []string{"user.response.cli.channel-1", "user.response.$entity.instance"} {
			t.Run(list+"/"+subject, func(t *testing.T) {
				action := `{"type": "publish", "subject": "` + subject + `"}`
				def := decodeTestRule(t, fmt.Sprintf(rule, list, action))
				_, err := Load(def, loadSource, h)
				require.Error(t, err)
				assertLoadRefusal(t, err, "reserved-subject-rule", list+"[0]", "user.response.>")
				assert.Contains(t, err.Error(), ActionTypePublish)
			})
		}
	}
}

// refusingHandler is a product handler that refuses every action.
type refusingHandler struct{}

func (refusingHandler) Check(Action) error { return errors.New("no rota named") }

func assertLoadRefusal(t *testing.T, err error, ruleID, position, cause string) {
	t.Helper()
	assert.True(t, errs.IsInvalid(err), "classified invalid: %v", err)
	for _, want := range []string{`"` + ruleID + `"`, loadSource, position, cause} {
		assert.Contains(t, err.Error(), want)
	}
}

// TestSemboidsPackLoads loads semboids' zone-steering rules, which use only
// built-in action types, with the handler set built from no product handlers.
func TestSemboidsPackLoads(t *testing.T) {
	h := builtinHandlers(t)
	paths, err := filepath.Glob(filepath.Join("testdata", "semboids-zone-steering", "*.json"))
	require.NoError(t, err)
	require.Len(t, paths, 3)

	var loaded []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var elements []json.RawMessage
		require.NoError(t, json.Unmarshal(data, &elements))
		for _, element := range elements {
			def, err := DecodeDefinition(element, path)
			require.NoError(t, err)
			_, err = Load(def, path, h)
			require.NoError(t, err, "rule %s in %s", def.ID, path)
			loaded = append(loaded, def.ID)
		}
	}
	// The seven rule IDs, read from the files by hand.
	assert.ElementsMatch(t, []string{
		"food-attract", "food-clear", "predator-flee", "predator-clear", "predator-cull", "wind-bias", "wind-clear",
	}, loaded)
}

// TestLoadChecksWhenFields puts an action with a two-condition `when` in each
// action list, enabled and disabled. The first condition is permitted; the
// second is the one under test, so a refusal must name `when[1]`. A `when`
// field gets the same check as a rule condition: a rule-opaque or undeclared
// predicate is refused, a declared structural predicate and a `$message.` or
// `$state.` field are not.
func TestLoadChecksWhenFields(t *testing.T) {
	declareTestPredicates()
	h := builtinHandlers(t)

	const expressionRule = `{"id": "when-rule", "type": "expression",
	  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}], %q: [%s]}`
	// A cron rule's list is "actions"; an enabled cron rule is refused before
	// its actions are checked, so the cron rule runs disabled only.
	const cronRule = `{"id": "when-rule", "type": "cron", "enabled": false, "schedule": "@daily", %q: [%s]}`
	lists := []struct {
		name   string
		rule   string
		states []bool
	}{
		{"on_enter", expressionRule, []bool{true, false}},
		{"on_exit", expressionRule, []bool{true, false}},
		{"while_true", expressionRule, []bool{true, false}},
		{"on_recovery", expressionRule, []bool{true, false}},
		{"actions", cronRule, []bool{false}},
	}
	action := func(field string) string {
		return `{"type": "publish", "subject": "alerts.x", "when": [
		  {"field": "$state.iteration", "operator": "lt", "value": 3},
		  {"field": "` + field + `", "operator": "eq", "value": "secret"}]}`
	}
	refused := []struct{ field, cause string }{
		{ruleOpaquePredicate, "rule-opaque"},
		{undeclaredPredicate, "not declared"},
		{"status", "$message."},
	}
	accepted := []string{structuralPredicate, "$message.name", "$state.iteration"}

	for _, list := range lists {
		for _, enabled := range list.states {
			suffix := ""
			if !enabled {
				suffix = ", disabled"
			}
			load := func(t *testing.T, field string) (*Rule, error) {
				def := decodeTestRule(t, fmt.Sprintf(list.rule, list.name, action(field)))
				def.Enabled = enabled
				return Load(def, loadSource, h)
			}
			for _, tc := range refused {
				t.Run(list.name+"/refuses "+tc.field+suffix, func(t *testing.T) {
					rule, err := load(t, tc.field)
					require.Error(t, err)
					assert.Nil(t, rule)
					assertLoadRefusal(t, err, "when-rule", list.name+"[0].when[1]", tc.cause)
				})
			}
			for _, field := range accepted {
				t.Run(list.name+"/accepts "+field+suffix, func(t *testing.T) {
					rule, err := load(t, field)
					require.NoError(t, err)
					assert.NotNil(t, rule)
				})
			}
		}
	}
}
