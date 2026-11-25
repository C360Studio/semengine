package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/vocabulary"
)

// Predicates the tests in this package declare. Registering one again
// amends nothing, so every test may call declareTestPredicates.
const (
	declaredPredicate   = "test.alert.active"
	conditionPredicate  = "test.status.level"
	ruleOpaquePredicate = "test.private.note"
	// structuralPredicate shares ruleOpaquePredicate's domain and category
	// but is not rule-opaque, so a rule may match on it.
	structuralPredicate = "test.private.status"
	undeclaredPredicate = "test.status.undeclared"
	// hyphenPredicate has a hyphen in its last segment, which the grammar
	// allows; the same name with an underscore is refused.
	hyphenPredicate = "workflow.state.next-phase"
)

func declareTestPredicates() {
	vocabulary.Register(declaredPredicate)
	vocabulary.Register(conditionPredicate)
	vocabulary.Register(ruleOpaquePredicate, vocabulary.WithRuleOpaque(true))
	vocabulary.Register(structuralPredicate)
	vocabulary.Register(hyphenPredicate)
}

func TestBuiltinCheck(t *testing.T) {
	declareTestPredicates()
	h, err := NewHandlers(nil)
	require.NoError(t, err)

	// Each accepted body carries every key the type reads, written out from
	// the rule-definitions spec, plus the four core keys; each refused body
	// adds one key the type does not read.
	cases := []struct {
		actionType string
		accepted   string
		refused    string
		refusedKey string
	}{
		{
			ActionTypePublish,
			`{"type": "publish", "subject": "alerts.raised", "properties": {"level": 3},
			  "id": "p", "max_iterations": 2, "when": [{"field": "$state.iteration", "operator": "lt", "value": 3}]}`,
			`{"type": "publish", "subject": "alerts.raised", "predicate": "test.alert.active"}`,
			"predicate",
		},
		{
			ActionTypeAddTriple,
			`{"type": "add_triple", "subject": "$entity.id", "predicate": "test.alert.active", "object": "true", "ttl": "5m"}`,
			`{"type": "add_triple", "predicat": "test.alert.active", "predicate": "test.alert.active", "object": "true"}`,
			"predicat",
		},
		{
			ActionTypeRemoveTriple,
			`{"type": "remove_triple", "subject": "$entity.id", "predicate": "test.alert.active", "object": "true"}`,
			`{"type": "remove_triple", "predicate": "test.alert.active", "ttl": "5m"}`,
			"ttl",
		},
		{
			ActionTypeUpdateTriple,
			`{"type": "update_triple", "subject": "$entity.id", "predicate": "test.alert.active", "object": "true",
			  "ttl": "5m", "properties": {"source": "rule"}}`,
			`{"type": "update_triple", "predicate": "test.alert.active", "merge": true}`,
			"merge",
		},
		{
			ActionTypeReconcilePredicates,
			`{"type": "reconcile_predicates", "subject": "$entity.id", "predicate": "test.alert.active",
			  "object": "true", "ttl": "5m", "projection_contract": "alerts", "projection_group": "active"}`,
			`{"type": "reconcile_predicates", "projection_contract": "alerts", "properties": {}}`,
			"properties",
		},
		{
			ActionTypeUpdateKV,
			`{"type": "update_kv", "bucket": "ALERTS", "key": "$entity.id", "payload": {"n": 1}, "merge": true}`,
			`{"type": "update_kv", "bucket": "ALERTS", "key": "k", "value": {"n": 1}}`,
			"value",
		},
		{
			ActionTypeLifecycleTransition,
			`{"type": "lifecycle_transition", "workflow": "flock.boid", "phase": "culled", "reason": "r",
			  "set": {"test.alert.active": "true"}}`,
			`{"type": "lifecycle_transition", "workflow": "flock.boid", "phase": "culled", "to": "culled"}`,
			"to",
		},
		{
			ActionTypeLifecycleComplete,
			`{"type": "lifecycle_complete", "workflow": "flock.boid"}`,
			`{"type": "lifecycle_complete", "workflow": "flock.boid", "reason": "done"}`,
			"reason",
		},
		{
			ActionTypeLifecycleFail,
			`{"type": "lifecycle_fail", "workflow": "flock.boid", "reason": "lost"}`,
			`{"type": "lifecycle_fail", "workflow": "flock.boid", "phase": "failed"}`,
			"phase",
		},
	}
	require.Len(t, cases, 9, "one case per built-in type")
	for _, tc := range cases {
		t.Run(tc.actionType, func(t *testing.T) {
			handler := h.byType[tc.actionType]
			require.NotNil(t, handler)
			assert.NoError(t, handler.Check(decodeTestAction(t, tc.accepted)))

			err := handler.Check(decodeTestAction(t, tc.refused))
			require.Error(t, err)
			assert.Contains(t, err.Error(), `"`+tc.refusedKey+`"`)
		})
	}

	loadChecks := []struct {
		name  string
		body  string
		cause string
	}{
		{"a reserved subject", `{"type": "publish", "subject": "user.response.x"}`, "user.response.>"},
		{"a deeper reserved subject", `{"type": "publish", "subject": "user.response.a.b"}`, "user.response.>"},
		{"a dynamic predicate", `{"type": "add_triple", "predicate": "$message.predicate", "object": "x"}`, "declared literal"},
		{"an undeclared predicate", `{"type": "update_triple", "predicate": "` + undeclaredPredicate + `"}`, "not declared"},
		{"a predicate that is not three parts", `{"type": "remove_triple", "predicate": "alert"}`, "alert"},
		{"a key of the wrong JSON type", `{"type": "update_kv", "bucket": "B", "merge": "yes"}`, "merge"},
		{"a key spelt with another case", `{"type": "publish", "Subject": "alerts.raised"}`, `"Subject"`},
	}
	for _, tc := range loadChecks {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			a := decodeTestAction(t, tc.body)
			err := h.byType[a.Type].Check(a)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.cause)
		})
	}

	t.Run("accepts a subject next to the reserved family", func(t *testing.T) {
		for _, subject := range []string{"user.response", "user.responses.x", "users.response.x"} {
			assert.NoError(t, h.byType[ActionTypePublish].Check(
				decodeTestAction(t, `{"type": "publish", "subject": "`+subject+`"}`)), subject)
		}
	})
}
