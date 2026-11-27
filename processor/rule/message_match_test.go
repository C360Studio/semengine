package rule

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/message"
)

// Requirement: rule-matching/"A message rule reads the payload's declared
// fields", "An unreadable payload is reported once per rule and type", "A
// missing required field is no match" (message side) and "A condition is
// never compared with an unfilled template" (message side).

// messageRuleID is the ID of every rule the message match tests load.
const messageRuleID = "message-match-test-rule"

// unreadablePayload stands in for a product payload that is registered and
// decoded but declares no rule-readable fields. It is not a framework type,
// so it cannot start declaring fields under the test.
type unreadablePayload struct {
	Role string `json:"role"`
}

func (p *unreadablePayload) Schema() message.Type {
	return message.Type{Domain: "test", Category: "unreadable", Version: "v1"}
}

func (p *unreadablePayload) Validate() error { return nil }

func (p *unreadablePayload) MarshalJSON() ([]byte, error) {
	type alias unreadablePayload
	return json.Marshal((*alias)(p))
}

func (p *unreadablePayload) UnmarshalJSON(data []byte) error {
	type alias unreadablePayload
	return json.Unmarshal(data, (*alias)(p))
}

// otherUnreadablePayload is a second payload type declaring no fields.
type otherUnreadablePayload struct{ unreadablePayload }

func (p *otherUnreadablePayload) Schema() message.Type {
	return message.Type{Domain: "test", Category: "unreadable-other", Version: "v1"}
}

// declaredSubsetPayload declares only `role` and leaves its `outcome` struct
// field out, so a condition on `outcome` shows the match never falls back to
// the payload's structure.
type declaredSubsetPayload struct {
	Role    string `json:"role"`
	Outcome string `json:"outcome"`
}

func (p *declaredSubsetPayload) Schema() message.Type {
	return message.Type{Domain: "test", Category: "subset", Version: "v1"}
}

func (p *declaredSubsetPayload) Validate() error { return nil }

func (p *declaredSubsetPayload) MarshalJSON() ([]byte, error) {
	type alias declaredSubsetPayload
	return json.Marshal((*alias)(p))
}

func (p *declaredSubsetPayload) UnmarshalJSON(data []byte) error {
	type alias declaredSubsetPayload
	return json.Unmarshal(data, (*alias)(p))
}

func (p *declaredSubsetPayload) RuleFields() map[string]any {
	return map[string]any{"role": p.Role}
}

// messageOf wraps a payload in the message the rule processor would match.
func messageOf(p message.Payload) message.Message {
	return message.NewBaseMessage(p.Schema(), p, "test")
}

func genericMessage(data map[string]any) message.Message {
	return messageOf(message.NewGenericJSON(data))
}

// loadMessageRule loads an enabled expression rule with no entity pattern,
// one the rule processor runs on messages, the conditions written as JSON.
func loadMessageRule(t *testing.T, id, conditions string) *Rule {
	t.Helper()
	return loadRule(t, fmt.Sprintf(`{"id": %q, "type": "expression", "conditions": %s}`, id, conditions))
}

// architectSucceeded is a message rule needing both role and outcome.
const architectSucceeded = `[{"field": "$message.role", "operator": "eq", "value": "architect"},
  {"field": "$message.outcome", "operator": "eq", "value": "success"}]`

// matchMessageOK runs the message match and requires no error.
func matchMessageOK(t *testing.T, r *Rule, msg message.Message) bool {
	t.Helper()
	got, err := r.matchMessage(t.Context(), msg)
	require.NoError(t, err)
	return got
}

func TestMessageMatch(t *testing.T) {
	log := captureWarnings(t)

	t.Run("a typed payload's declared field is read", func(t *testing.T) {
		r := loadMessageRule(t, messageRuleID, `[{"field": "$message.role", "operator": "eq", "value": "architect"}]`)
		assert.True(t, matchMessageOK(t, r, messageOf(&declaredSubsetPayload{Role: "architect"})))
		assert.False(t, matchMessageOK(t, r, messageOf(&declaredSubsetPayload{Role: "editor"})))
	})

	t.Run("a generic JSON payload is read through its data map", func(t *testing.T) {
		r := loadMessageRule(t, messageRuleID, architectSucceeded)
		assert.True(t, matchMessageOK(t, r, genericMessage(map[string]any{"role": "architect", "outcome": "success"})))
		assert.False(t, matchMessageOK(t, r, genericMessage(map[string]any{"role": "editor", "outcome": "success"})))
	})

	t.Run("an undeclared field is not reachable", func(t *testing.T) {
		r := loadMessageRule(t, messageRuleID, architectSucceeded)
		msg := messageOf(&declaredSubsetPayload{Role: "architect", Outcome: "success"})
		assert.False(t, matchMessageOK(t, r, msg), "the outcome struct field is not declared")

		fields, readable := ruleFields(msg)
		require.True(t, readable)
		assert.NotContains(t, fields, "outcome")
	})

	t.Run("a condition value is filled from the message", func(t *testing.T) {
		r := loadMessageRule(t, messageRuleID,
			`[{"field": "$message.role", "operator": "eq", "value": "$message.expected"}]`)
		assert.True(t, matchMessageOK(t, r, genericMessage(map[string]any{"role": "editor", "expected": "editor"})))
		assert.False(t, matchMessageOK(t, r, genericMessage(map[string]any{"role": "editor", "expected": "architect"})))
	})

	// An empty payload is read like any other: a literal condition gives the
	// answer a payload without the field gives.
	t.Run("an empty payload answers as one without the field", func(t *testing.T) {
		for _, condition := range []string{
			`{"field": "$message.x", "operator": "ne", "value": "a"}`,
			`{"field": "$message.x", "operator": "eq", "value": "a"}`,
			`{"field": "$message.x", "operator": "not_in", "value": ["a"]}`,
		} {
			r := loadMessageRule(t, messageRuleID, "["+condition+"]")
			assert.False(t, matchMessageOK(t, r, genericMessage(map[string]any{})), "empty: %s", condition)
			assert.False(t, matchMessageOK(t, r, genericMessage(map[string]any{"y": 1})), "without x: %s", condition)
		}
	})

	t.Run("a value template on an empty payload is an error", func(t *testing.T) {
		r := loadMessageRule(t, messageRuleID,
			`[{"field": "$message.role", "operator": "eq", "value": "$message.expected"}]`)
		got, err := r.matchMessage(t.Context(), genericMessage(map[string]any{}))
		require.Error(t, err)
		assert.False(t, got)
		for _, want := range []string{messageRuleID, "$message.role", "$message.expected"} {
			assert.Contains(t, err.Error(), want)
		}
	})

	t.Run("a legitimate false logs nothing", func(t *testing.T) {
		r := loadMessageRule(t, messageRuleID, `[{"field": "$message.role", "operator": "eq", "value": "architect"}]`)
		assert.False(t, matchMessageOK(t, r, messageOf(&declaredSubsetPayload{Role: "editor"})))
		assert.False(t, matchMessageOK(t, r, genericMessage(map[string]any{})))
	})
	assert.Empty(t, log.take(t), "no readable payload is reported and no match logs")
}

// unreadableReports returns the unreadable reports logged since the last
// take, by rule ID and then payload type, counting each.
func unreadableReports(t *testing.T, log *warningLog) map[string]map[string]int {
	t.Helper()
	out := map[string]map[string]int{}
	for _, w := range log.take(t) {
		require.Equal(t, "WARN", w.Level, "%+v", w)
		require.NotEmpty(t, w.Remedy, "the report names its remedy: %+v", w)
		if out[w.RuleID] == nil {
			out[w.RuleID] = map[string]int{}
		}
		out[w.RuleID][w.PayloadType]++
	}
	return out
}

func TestUnreadableReportedOnce(t *testing.T) {
	log := captureWarnings(t)
	const neighbourID = "unaffected-neighbour"
	def := decodeTestRule(t, fmt.Sprintf(`{"id": %q, "type": "expression", "conditions": %s}`,
		messageRuleID, architectSucceeded))
	r, err := Load(def, loadSource, builtinHandlers(t))
	require.NoError(t, err)
	neighbour := loadMessageRule(t, neighbourID, architectSucceeded)

	unreadable := messageOf(&unreadablePayload{Role: "architect"})
	other := messageOf(&otherUnreadablePayload{})

	const goroutines, each = 16, 50
	type result struct {
		matched, want bool
		err           error
	}
	results := make([]result, goroutines*each)
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			for i := range each {
				target, msg, want := r, unreadable, false
				switch i % 4 {
				case 1:
					target = neighbour
				case 2:
					msg = other
				case 3:
					// A readable payload that matches, read alongside.
					msg, want = genericMessage(map[string]any{"role": "architect", "outcome": "success"}), true
				}
				matched, err := target.matchMessage(t.Context(), msg)
				results[g*each+i] = result{matched, want, err}
			}
		})
	}
	// Wait orders the reads of results and of the log after every match.
	wg.Wait()

	for i, res := range results {
		require.NoError(t, res.err, "match %d", i)
		assert.Equal(t, res.want, res.matched, "match %d: an unreadable payload is no match", i)
	}
	assert.Equal(t, map[string]map[string]int{
		messageRuleID: {"test.unreadable.v1": 1, "test.unreadable-other.v1": 1},
		neighbourID:   {"test.unreadable.v1": 1},
	}, unreadableReports(t, log), "one report per loaded rule and payload type")

	t.Run("a rule loaded again reports again", func(t *testing.T) {
		reloaded, err := Load(def, loadSource, builtinHandlers(t))
		require.NoError(t, err)
		for range each {
			matched, err := reloaded.matchMessage(t.Context(), unreadable)
			require.NoError(t, err)
			require.False(t, matched)
		}
		assert.Equal(t, map[string]map[string]int{messageRuleID: {"test.unreadable.v1": 1}},
			unreadableReports(t, log))
	})
}
