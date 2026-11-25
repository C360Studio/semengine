package rule

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/processor/rule/expression"
)

const decodeSource = "rules/test.json"

// fullRule sets every Definition key, with an action body holding a space
// and `&`, so a marshal changes its bytes but not its values.
const fullRule = `{
  "id": "full_rule",
  "type": "expression",
  "name": "Full rule",
  "description": "every key set",
  "enabled": false,
  "conditions": [
    {"field": "test.status.level", "operator": "gte", "value": 2.5, "required": true},
    {"field": "test.status.mode", "operator": "transition", "value": "on", "from": ["off", "idle"]}
  ],
  "logic": "or",
  "cooldown": "5s",
  "entity": {"pattern": "c360.test.rule.decode.drone.*"},
  "metadata": {"owner": "ops", "rank": 3},
  "on_enter": [
    {"type": "publish", "subject": "alerts.raised", "properties": {"note": "a & b"},
     "id": "raise", "max_iterations": 0,
     "when": [{"field": "$state.iteration", "operator": "lt", "value": 3}]}
  ],
  "on_exit": [{"type": "remove_triple", "predicate": "test.alert.active", "object": "true"}],
  "while_true": [{"type": "update_kv", "bucket": "B", "key": "k", "payload": {"n": 1.50}, "max_iterations": 7}],
  "on_recovery": [{"type": "publish", "subject": "alerts.recovered"}],
  "rerun_on_recovery": true,
  "max_iterations": 4,
  "fire_every_n_events": 2,
  "schedule": "@hourly",
  "actions": [{"type": "lifecycle_complete", "workflow": "w"}]
}`

func TestDecodeDefinition(t *testing.T) {
	t.Run("round trip keeps values, not bytes", func(t *testing.T) {
		first, err := DecodeDefinition([]byte(fullRule), decodeSource)
		require.NoError(t, err)

		// The decode read the file's values; the expected values are
		// written out from fullRule above.
		zero, seven := 0, 7
		assert.Equal(t, "full_rule", first.ID)
		assert.Equal(t, "expression", first.Type)
		assert.Equal(t, "Full rule", first.Name)
		assert.Equal(t, "every key set", first.Description)
		assert.False(t, first.Enabled)
		assert.Equal(t, "or", first.Logic)
		assert.Equal(t, "5s", first.Cooldown)
		assert.Equal(t, "c360.test.rule.decode.drone.*", first.Entity.Pattern)
		assert.Equal(t, map[string]interface{}{"owner": "ops", "rank": float64(3)}, first.Metadata)
		assert.True(t, first.RerunOnRecovery)
		assert.Equal(t, 4, first.MaxIterations)
		assert.Equal(t, 2, first.FireEveryNEvents)
		assert.Equal(t, "@hourly", first.Schedule)
		require.Len(t, first.Conditions, 2)
		assert.Equal(t, expression.ConditionExpression{
			Field: "test.status.level", Operator: "gte", Value: 2.5, Required: true,
		}, first.Conditions[0])
		assert.Equal(t, []interface{}{"off", "idle"}, first.Conditions[1].From)
		require.Len(t, first.OnEnter, 1)
		assert.Equal(t, "publish", first.OnEnter[0].Type)
		assert.Equal(t, "raise", first.OnEnter[0].ID)
		assert.Equal(t, &zero, first.OnEnter[0].MaxIterations)
		assert.Equal(t, []expression.ConditionExpression{
			{Field: "$state.iteration", Operator: "lt", Value: float64(3)},
		}, first.OnEnter[0].When)
		require.Len(t, first.WhileTrue, 1)
		assert.Equal(t, &seven, first.WhileTrue[0].MaxIterations)
		assert.Equal(t, "lifecycle_complete", first.Actions[0].Type)
		require.Len(t, first.OnExit, 1)
		require.Len(t, first.OnRecovery, 1)

		stored, err := json.Marshal(first)
		require.NoError(t, err)
		second, err := DecodeDefinition(stored, "rules.full_rule@2")
		require.NoError(t, err)

		assert.Equal(t, withoutBodies(first), withoutBodies(second), "every field but Body is equal")
		firstActions, secondActions := allActions(first), allActions(second)
		require.Len(t, secondActions, len(firstActions))
		for i := range firstActions {
			assert.True(t, sameJSONValue(t, firstActions[i].Body, secondActions[i].Body),
				"action %d: Body is equal in value", i)
			firstKey, err := capKey(first.ID, firstActions[i])
			require.NoError(t, err)
			secondKey, err := capKey(second.ID, secondActions[i])
			require.NoError(t, err)
			assert.Equal(t, firstKey, secondKey, "action %d: firing-cap key is unchanged", i)
		}
		// The stored bytes are compacted and escaped: not the file's bytes.
		assert.NotEqual(t, string(firstActions[0].Body), string(secondActions[0].Body))
		assert.Contains(t, string(secondActions[0].Body), `a \u0026 b`)
		assert.Contains(t, string(firstActions[0].Body), `"note": "a & b"`)
	})

	t.Run("absent keys", func(t *testing.T) {
		def, err := DecodeDefinition([]byte(`{
		  "id": "absent",
		  "type": "expression",
		  "conditions": [{"field": "test.status.level", "operator": "gte", "value": 2}],
		  "on_enter": [{"type": "publish", "subject": "s", "when": [{"field": "x", "operator": "eq", "value": 1}]}],
		  "on_exit": [],
		  "metadata": {}
		}`), decodeSource)
		require.NoError(t, err)
		assert.True(t, def.Enabled, "absent enabled means enabled")
		assert.Equal(t, "and", def.Logic, "absent logic means and")
		assert.False(t, def.Conditions[0].Required, "absent required means not required")
		assert.False(t, def.OnEnter[0].When[0].Required, "absent required in when means not required")
		assert.Nil(t, def.OnEnter[0].MaxIterations, "absent max_iterations means the default cap")
		assert.Empty(t, def.Name)
		assert.Nil(t, def.OnExit, "an empty action list decodes as an absent one")
		assert.Nil(t, def.Metadata, "empty metadata decodes as absent")

		// A missing field under a condition that is not required is a
		// non-match, not an evaluation error.
		entity := &gtypes.EntityState{ID: semantictest.EntityID(t, "c360", "test", "rule", "decode", "drone", "001")}
		matched, err := expression.NewExpressionEvaluator().Evaluate(entity,
			expression.LogicalExpression{Conditions: def.Conditions, Logic: def.Logic})
		require.NoError(t, err)
		assert.False(t, matched)
	})

	refusals := []struct {
		name     string
		rule     string
		contains []string
	}{
		{
			name:     "related_patterns",
			rule:     `{"id": "r1", "type": "expression", "related_patterns": ["a"]}`,
			contains: []string{`"related_patterns"`},
		},
		{
			name:     "entity watch_buckets",
			rule:     `{"id": "r1", "type": "expression", "entity": {"pattern": "p", "watch_buckets": ["ENTITY_STATES"]}}`,
			contains: []string{`"watch_buckets"`},
		},
		{
			name:     "misspelt rule key",
			rule:     `{"id": "r1", "type": "expression", "on_entr": []}`,
			contains: []string{`"on_entr"`},
		},
		{
			name:     "undeclared condition key",
			rule:     `{"id": "r1", "conditions": [{"field": "a.b.c", "operator": "eq", "value": 1, "requried": true}]}`,
			contains: []string{`"requried"`},
		},
		{
			name:     "misspelt key in a when condition",
			rule:     `{"id": "r1", "on_enter": [{"type": "publish", "when": [{"field": "x", "operator": "eq", "valeu": 1}]}]}`,
			contains: []string{`"valeu"`, "when[0]"},
		},
		{
			name:     "misspelt key in a later when condition",
			rule:     `{"id": "r1", "on_exit": [{"type": "publish", "when": [{"field": "x", "operator": "eq"}, {"fild": "y"}]}]}`,
			contains: []string{`"fild"`, "when[1]"},
		},
		{
			name:     "action that is not an object",
			rule:     `{"id": "r1", "on_enter": ["publish"]}`,
			contains: []string{"action"},
		},
		{
			name:     "null action",
			rule:     `{"id": "r1", "on_enter": [null]}`,
			contains: []string{"null"},
		},
		{
			name:     "core action key of the wrong type",
			rule:     `{"id": "r1", "on_enter": [{"type": "publish", "max_iterations": "3"}]}`,
			contains: []string{`"max_iterations"`},
		},
		{
			name:     "a rule key spelt with another case",
			rule:     `{"id": "r1", "ID": "r1", "type": "expression"}`,
			contains: []string{`"ID"`},
		},
		{
			name:     "a rule key spelt only with another case",
			rule:     `{"id": "r1", "type": "expression", "Enabled": false}`,
			contains: []string{`"Enabled"`},
		},
		{
			name:     "an entity key spelt with another case",
			rule:     `{"id": "r1", "entity": {"Pattern": "c360.test.rule.decode.drone.*"}}`,
			contains: []string{`"Pattern"`, "entity"},
		},
		{
			name: "a condition key spelt with another case",
			rule: `{"id": "r1", "conditions": [{"field": "a.b.c", "operator": "eq", "value": 1},
			  {"Field": "a.b.c", "operator": "eq", "value": 1}]}`,
			contains: []string{`"Field"`, "conditions[1]"},
		},
		{
			name: "a when condition key spelt with another case",
			rule: `{"id": "r1", "on_enter": [{"type": "publish", "when": [{"field": "x", "operator": "eq"},
			  {"field": "x", "Operator": "eq"}]}]}`,
			contains: []string{`"Operator"`, "when[1]"},
		},
		{
			name:     "data after the object",
			rule:     `{"id": "r1"} {"id": "r2"}`,
			contains: []string{"data after"},
		},
	}
	for _, tc := range refusals {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			_, err := DecodeDefinition([]byte(tc.rule), decodeSource)
			require.Error(t, err)
			assert.True(t, errs.IsInvalid(err), "classified invalid: %v", err)
			for _, want := range append([]string{`"r1"`, decodeSource}, tc.contains...) {
				assert.Contains(t, err.Error(), want)
			}
		})
	}

	t.Run("core action keys match exactly", func(t *testing.T) {
		// "Type" is not the core key "type", so it is left to the body and
		// the action has no type, which loading refuses.
		def, err := DecodeDefinition([]byte(`{"id": "r1", "on_enter": [{"Type": "publish"}]}`), decodeSource)
		require.NoError(t, err)
		assert.Empty(t, def.OnEnter[0].Type)
	})
}

// withoutBodies returns def with every action's Body cleared.
func withoutBodies(def Definition) Definition {
	strip := func(actions []Action) []Action {
		if actions == nil {
			return nil
		}
		out := make([]Action, len(actions))
		for i, a := range actions {
			a.Body = nil
			out[i] = a
		}
		return out
	}
	def.OnEnter = strip(def.OnEnter)
	def.OnExit = strip(def.OnExit)
	def.WhileTrue = strip(def.WhileTrue)
	def.OnRecovery = strip(def.OnRecovery)
	def.Actions = strip(def.Actions)
	return def
}

func allActions(def Definition) []Action {
	var out []Action
	for _, list := range [][]Action{def.OnEnter, def.OnExit, def.WhileTrue, def.OnRecovery, def.Actions} {
		out = append(out, list...)
	}
	return out
}

// sameJSONValue reports whether a and b hold the same JSON value, numbers
// compared as written.
func sameJSONValue(t *testing.T, a, b []byte) bool {
	t.Helper()
	decode := func(data []byte) any {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var v any
		require.NoError(t, decoder.Decode(&v))
		return v
	}
	return reflect.DeepEqual(decode(a), decode(b))
}

func FuzzDecodeDefinition(f *testing.F) {
	for _, seed := range []string{
		fullRule,
		`{"id": "r"}`,
		`{"id": "r", "on_exit": [], "metadata": {}}`,
		`{"id": "r", "on_enter": [{"type": "publish", "note": "<&>"}]}`,
		`{"id": "r", "related_patterns": []}`,
		`{"id": "r", "on_enter": [{"type": "x", "when": [{"fild": 1}]}]}`,
		`{"id": "r", "on_enter": [null]}`,
		`{"id": "r"} 1`,
		`[]`,
		`not json`,
		`{"id": "r", "type": "expression", "conditions": [{"field": "$message.name", "operator": "regex", "value": "^a"}],
		  "on_enter": [{"type": "publish", "subject": "a.b",
		    "when": [{"field": "$message.name", "operator": "regex", "value": "b$"}]}]}`,
		`{"id": "r", "type": "cron", "enabled": false, "actions": [{"type": "lifecycle_fail", "workflow": "w"}]}`,
	} {
		f.Add([]byte(seed))
	}
	handlers, err := NewHandlers(nil)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		first, err := DecodeDefinition(data, decodeSource)
		if err != nil {
			if !errs.IsInvalid(err) || !strings.Contains(err.Error(), decodeSource) {
				t.Fatalf("refusal is not classified invalid naming the source: %v", err)
			}
			return
		}
		stored, err := json.Marshal(first)
		if err != nil {
			t.Fatalf("a decoded definition does not marshal: %v", err)
		}
		second, err := DecodeDefinition(stored, decodeSource)
		if err != nil {
			t.Fatalf("a stored definition does not decode: %v\n%s", err, stored)
		}
		if !reflect.DeepEqual(withoutBodies(first), withoutBodies(second)) {
			t.Fatalf("round trip changed a field:\n%#v\n%#v", first, second)
		}
		firstActions, secondActions := allActions(first), allActions(second)
		for i := range firstActions {
			firstKey, err := capKey(first.ID, firstActions[i])
			if err != nil {
				t.Fatalf("action %d: %v", i, err)
			}
			secondKey, err := capKey(second.ID, secondActions[i])
			if err != nil {
				t.Fatalf("action %d: %v", i, err)
			}
			if firstKey != secondKey {
				t.Fatalf("action %d: firing-cap key changed in a round trip", i)
			}
		}

		// Load either refuses, classified invalid and naming the source, or
		// accepts; either way it leaves the definition as decoded. A second
		// decode of the same bytes is the independent copy compared with.
		want, err := DecodeDefinition(data, decodeSource)
		if err != nil {
			t.Fatalf("a second decode of the same bytes failed: %v", err)
		}
		if _, err := Load(first, decodeSource, handlers); err != nil {
			if !errs.IsInvalid(err) || !strings.Contains(err.Error(), decodeSource) {
				t.Fatalf("Load refusal is not classified invalid naming the source: %v", err)
			}
		}
		if !reflect.DeepEqual(want, first) {
			t.Fatalf("Load changed the definition:\n%#v\n%#v", want, first)
		}
	})
}
