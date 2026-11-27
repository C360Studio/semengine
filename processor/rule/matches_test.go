package rule

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/processor/rule/expression"
	"github.com/c360studio/semengine/vocabulary"
)

// Requirement: rule-matching/"A loaded rule answers whether it matches an
// entity", "A stateless match checks in a fixed order", "Firing and asking
// share one match", "Outside the pattern, only Matches checks the pattern",
// "A stateless match refuses what it cannot answer", "A missing required
// field is no match" and "A condition is never compared with an unfilled
// template".

// matchRuleID is the ID of every rule the match tests load.
const matchRuleID = "matches-test-rule"

// matchPattern is the entity pattern of the match tests' rules; sensorID
// builds IDs inside it.
const matchPattern = "c360.ops.robotics.gcs.sensor.*"

// declareMatchPredicates declares the predicates the match tests' conditions
// read, so Load accepts them. Registering one again amends nothing.
func declareMatchPredicates() {
	for _, p := range []string{
		"sensor.measurement.fahrenheit",
		"sensor.measurement.psi",
		"sensor.classification.type",
		"geo.location.zone",
		"gather.child.completed",
		"decision.subtopics.list",
		"workflow.plan.status",
	} {
		vocabulary.Register(p)
	}
}

func sensorID(t *testing.T, instance string) string {
	t.Helper()
	return semantictest.EntityID(t, "c360", "ops", "robotics", "gcs", "sensor", instance)
}

// loadRule decodes and loads one rule with the built-in handlers.
func loadRule(t *testing.T, rule string) *Rule {
	t.Helper()
	declareMatchPredicates()
	loaded, err := Load(decodeTestRule(t, rule), loadSource, builtinHandlers(t))
	require.NoError(t, err)
	return loaded
}

// loadExpression loads an enabled expression rule watching matchPattern with
// the given logic and conditions, the conditions written as JSON.
func loadExpression(t *testing.T, logic, conditions string) *Rule {
	t.Helper()
	return loadRule(t, fmt.Sprintf(`{"id": %q, "type": "expression", "logic": %q,
	  "entity": {"pattern": %q}, "conditions": %s}`, matchRuleID, logic, matchPattern, conditions))
}

// fire runs the firing entity match with no lifecycle lookup.
func fire(t *testing.T, r *Rule, state *gtypes.EntityState) (bool, error) {
	t.Helper()
	return r.matchEntity(t.Context(), state, nil)
}

// matchCase is one rule, one entity and the verdict a person wrote for it.
// The verdicts are written by hand, never computed by the code under test.
type matchCase struct {
	name       string
	logic      string
	conditions string
	triples    []message.Triple
	want       bool
}

func matchCorpus() []matchCase {
	return []matchCase{
		{
			name:  "and: both conditions hold",
			logic: "and",
			conditions: `[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0, "required": true},
			  {"field": "geo.location.zone", "operator": "contains", "value": "cold-storage", "required": true}]`,
			triples: []message.Triple{
				{Predicate: "sensor.measurement.fahrenheit", Object: 41.2},
				{Predicate: "geo.location.zone", Object: "zone.cold-storage-1"},
			},
			want: true,
		},
		{
			name:  "and: one condition fails",
			logic: "and",
			conditions: `[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0, "required": true},
			  {"field": "geo.location.zone", "operator": "contains", "value": "cold-storage", "required": true}]`,
			triples: []message.Triple{
				{Predicate: "sensor.measurement.fahrenheit", Object: 38.5},
				{Predicate: "geo.location.zone", Object: "zone.cold-storage-1"},
			},
			want: false,
		},
		{
			name:  "or: second condition carries it",
			logic: "or",
			conditions: `[{"field": "sensor.measurement.psi", "operator": "lt", "value": 100.0},
			  {"field": "sensor.classification.type", "operator": "eq", "value": "pressure"}]`,
			triples: []message.Triple{
				{Predicate: "sensor.measurement.psi", Object: 140.0},
				{Predicate: "sensor.classification.type", Object: "pressure"},
			},
			want: true,
		},
		{
			name:       "optional field absent does not match",
			logic:      "and",
			conditions: `[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0}]`,
			triples:    []message.Triple{{Predicate: "sensor.classification.type", Object: "temperature"}},
			want:       false,
		},
		{
			name:       "required field absent does not match",
			logic:      "and",
			conditions: `[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0, "required": true}]`,
			triples:    []message.Triple{{Predicate: "sensor.classification.type", Object: "temperature"}},
			want:       false,
		},
		{
			// A list-shaped triple's .length fills the value with 2, and the
			// list operator counts the two counter triples.
			name:  "a templated condition value is filled before evaluation",
			logic: "and",
			conditions: `[{"field": "gather.child.completed", "operator": "length_eq",
			  "value": "$entity.triple.decision.subtopics.list.length"}]`,
			triples: []message.Triple{
				{Predicate: "decision.subtopics.list", Object: []string{"a", "b"}},
				{Predicate: "gather.child.completed", Object: "a"},
				{Predicate: "gather.child.completed", Object: "b"},
			},
			want: true,
		},
	}
}

func TestMatches(t *testing.T) {
	id := sensorID(t, "001")

	t.Run("hand-written verdicts, firing and asking alike", func(t *testing.T) {
		for _, c := range matchCorpus() {
			t.Run(c.name, func(t *testing.T) {
				r := loadExpression(t, c.logic, c.conditions)

				got, err := r.Matches(t.Context(), entityWith(id, c.triples...))
				require.NoError(t, err)
				assert.Equal(t, c.want, got, "Matches")

				fired, err := fire(t, r, entityWith(id, c.triples...))
				require.NoError(t, err)
				assert.Equal(t, c.want, fired, "firing match")
			})
		}
	})

	// Step 1: nil inputs are refused.
	t.Run("a nil rule, context or state is refused", func(t *testing.T) {
		r := loadExpression(t, "and", `[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0}]`)
		state := entityWith(id, message.Triple{Predicate: "sensor.measurement.fahrenheit", Object: 41.2})
		var nilRule *Rule
		var nilCtx context.Context

		for name, call := range map[string]func() (bool, error){
			"rule":    func() (bool, error) { return nilRule.Matches(t.Context(), state) },
			"context": func() (bool, error) { return r.Matches(nilCtx, state) },
			"state":   func() (bool, error) { return r.Matches(t.Context(), nil) },
		} {
			got, err := call()
			assert.Error(t, err, "a nil %s", name)
			assert.False(t, got, "a nil %s", name)
		}
	})

	// Step 2: a malformed entity ID is an error, before the rule's own
	// state is read.
	t.Run("a malformed entity ID is an error, for a disabled rule too", func(t *testing.T) {
		for _, enabled := range []bool{true, false} {
			r := loadRule(t, fmt.Sprintf(`{"id": %q, "type": "expression", "enabled": %t,
			  "entity": {"pattern": %q},
			  "conditions": [{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0}]}`,
				matchRuleID, enabled, matchPattern))
			got, err := r.Matches(t.Context(), entityWith("test.entity.id",
				message.Triple{Predicate: "sensor.measurement.fahrenheit", Object: 41.2}))
			assert.Error(t, err, "enabled %t", enabled)
			assert.False(t, got)
		}
	})

	// Step 3: rules production never evaluates for this entity owe nothing.
	t.Run("rules production would not run for the entity give false", func(t *testing.T) {
		holds := message.Triple{Predicate: "sensor.measurement.fahrenheit", Object: 41.2}
		cases := map[string]string{
			"a disabled cron rule, with no pattern and no conditions": `{"id": "` + matchRuleID + `",
			  "type": "cron", "enabled": false, "schedule": "@daily",
			  "actions": [{"type": "publish", "subject": "reports.daily"}]}`,
			"a disabled expression rule": `{"id": "` + matchRuleID + `", "type": "expression", "enabled": false,
			  "entity": {"pattern": "` + matchPattern + `"},
			  "conditions": [{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0}]}`,
			"an enabled rule with no entity pattern": `{"id": "` + matchRuleID + `", "type": "expression",
			  "conditions": [{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0}]}`,
			// These come before the stateless refusals: a rule that owes
			// nothing is not asked a question it cannot answer.
			"a disabled rule with a condition needing state": `{"id": "` + matchRuleID + `", "type": "expression",
			  "enabled": false, "entity": {"pattern": "` + matchPattern + `"},
			  "conditions": [{"field": "$state.iteration", "operator": "gte", "value": 1}]}`,
			"a rule with no entity pattern and a condition needing state": `{"id": "` + matchRuleID + `",
			  "type": "expression", "conditions": [{"field": "$state.iteration", "operator": "gte", "value": 1}]}`,
		}
		for name, rule := range cases {
			t.Run(name, func(t *testing.T) {
				got, err := loadRule(t, rule).Matches(t.Context(), entityWith(id, holds))
				require.NoError(t, err)
				assert.False(t, got)
			})
		}

		t.Run("an entity outside the pattern", func(t *testing.T) {
			r := loadExpression(t, "and", `[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0}]`)
			outside := semantictest.EntityID(t, "c360", "ops", "robotics", "gcs", "drone", "001")

			got, err := r.Matches(t.Context(), entityWith(outside, holds))
			require.NoError(t, err)
			assert.False(t, got, "Matches checks the pattern")

			// The rule processor picks entity rules by pattern before it
			// calls the firing match, so the firing match does not.
			fired, err := fire(t, r, entityWith(outside, holds))
			require.NoError(t, err)
			assert.True(t, fired, "the firing match answers on the conditions")
		})

		t.Run("an entity outside the pattern, the condition needing state", func(t *testing.T) {
			r := loadExpression(t, "and", `[{"field": "$state.iteration", "operator": "gte", "value": 1}]`)
			outside := semantictest.EntityID(t, "c360", "ops", "robotics", "gcs", "drone", "001")
			got, err := r.Matches(t.Context(), entityWith(outside))
			require.NoError(t, err)
			assert.False(t, got)
		})
	})

	// Step 4: what a single entity state cannot answer is refused.
	t.Run("a condition needing state is refused", func(t *testing.T) {
		cases := map[string]string{
			"$state.":            `{"field": "$state.iteration", "operator": "gte", "value": 1}`,
			"$prev.":             `{"field": "$prev.workflow.plan.status", "operator": "eq", "value": "pending"}`,
			"transition":         `{"field": "workflow.plan.status", "operator": "transition", "value": "done", "from": "pending"}`,
			"$entity.lifecycle.": `{"field": "$entity.lifecycle.phase", "operator": "eq", "value": "flying"}`,
		}
		for name, condition := range cases {
			t.Run(name, func(t *testing.T) {
				r := loadExpression(t, "and", "["+condition+"]")
				got, err := r.Matches(t.Context(), entityWith(id))
				require.Error(t, err)
				assert.False(t, got)
				var evalErr *expression.EvaluationError
				assert.ErrorAs(t, err, &evalErr, "the error names the field")
				assert.Contains(t, err.Error(), matchRuleID)
			})
		}

		t.Run("the lifecycle refusal says a lookup is needed", func(t *testing.T) {
			r := loadExpression(t, "and", `[{"field": "$entity.lifecycle.phase", "operator": "eq", "value": "flying"}]`)
			_, err := r.Matches(t.Context(), entityWith(id))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "$entity.lifecycle.phase")
			assert.Contains(t, err.Error(), "cannot be answered without a lifecycle lookup")
		})
	})

	t.Run("an unfilled value template is an error, not a verdict", func(t *testing.T) {
		r := loadExpression(t, "and", `[{"field": "sensor.classification.type", "operator": "eq", "value": "$state.iteration"}]`)
		got, err := r.Matches(t.Context(), entityWith(id,
			message.Triple{Predicate: "sensor.classification.type", Object: "temperature"}))
		require.Error(t, err)
		assert.False(t, got)
		assert.Contains(t, err.Error(), "$state.iteration")
	})

	// A cooldown limits how often a running rule fires; the conditions
	// still hold, so the rule is answered.
	t.Run("a rule with a cooldown is answered, not refused", func(t *testing.T) {
		r := loadRule(t, fmt.Sprintf(`{"id": %q, "type": "expression", "cooldown": "1h",
		  "entity": {"pattern": %q},
		  "conditions": [{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0, "required": true}]}`,
			matchRuleID, matchPattern))
		got, err := r.Matches(t.Context(), entityWith(id,
			message.Triple{Predicate: "sensor.measurement.fahrenheit", Object: 41.2}))
		require.NoError(t, err)
		assert.True(t, got)
	})
}

// Requirement: rule-matching/"A missing required field is no match" and
// rule-conditions/"An absent required field is told apart", entity side.
func TestRequiredFieldMissing(t *testing.T) {
	id := sensorID(t, "002")
	other := message.Triple{Predicate: "sensor.classification.type", Object: "temperature"}

	t.Run("an entity lacking a required field", func(t *testing.T) {
		r := loadExpression(t, "and",
			`[{"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0, "required": true}]`)

		fired, err := fire(t, r, entityWith(id, other))
		require.NoError(t, err, "firing match")
		assert.False(t, fired)

		got, err := r.Matches(t.Context(), entityWith(id, other))
		require.NoError(t, err, "Matches")
		assert.False(t, got)
	})

	// The evaluator stops at the first error, so an absent required field
	// vetoes the rule even when another condition holds.
	t.Run("under or, an absent required field vetoes a condition that holds", func(t *testing.T) {
		r := loadExpression(t, "or", `[{"field": "sensor.classification.type", "operator": "eq", "value": "temperature"},
		  {"field": "sensor.measurement.fahrenheit", "operator": "gte", "value": 40.0, "required": true}]`)

		fired, err := fire(t, r, entityWith(id, other))
		require.NoError(t, err, "firing match")
		assert.False(t, fired)

		got, err := r.Matches(t.Context(), entityWith(id, other))
		require.NoError(t, err, "Matches")
		assert.False(t, got)
	})

	// gt and the other ordering operators compare text when a value is not
	// a number, so a length operator whose value is not a number is the
	// operator failure.
	t.Run("an operator failure is an error and not the sentinel", func(t *testing.T) {
		r := loadExpression(t, "and",
			`[{"field": "sensor.classification.type", "operator": "length_eq", "value": "abc", "required": true}]`)

		for name, call := range map[string]func() (bool, error){
			"firing match": func() (bool, error) { return fire(t, r, entityWith(id, other)) },
			"Matches":      func() (bool, error) { return r.Matches(t.Context(), entityWith(id, other)) },
		} {
			got, err := call()
			require.Error(t, err, name)
			assert.False(t, got, name)
			assert.False(t, errors.Is(err, expression.ErrRequiredFieldMissing), "%s: %v", name, err)
			assert.Contains(t, err.Error(), matchRuleID, name)
			assert.Contains(t, err.Error(), "sensor.classification.type", name)
		}
	})
}

// Requirement: rule-matching/"A condition is never compared with an unfilled
// template".
func TestUnresolvedValueTemplate(t *testing.T) {
	log := captureWarnings(t)
	id := sensorID(t, "003")
	const token = "$entity.triple.x.y.missing"

	t.Run("a missing value fails both matches and logs nothing", func(t *testing.T) {
		r := loadExpression(t, "and", `[{"field": "sensor.classification.type", "operator": "eq", "value": "`+token+`"}]`)
		// The triple holds the token's own text, which a comparison of the
		// unfilled template would find equal.
		holdsText := message.Triple{Predicate: "sensor.classification.type", Object: token}

		for name, call := range map[string]func() (bool, error){
			"firing match": func() (bool, error) { return fire(t, r, entityWith(id, holdsText)) },
			"Matches":      func() (bool, error) { return r.Matches(t.Context(), entityWith(id, holdsText)) },
		} {
			got, err := call()
			require.Error(t, err, name)
			assert.False(t, got, name)
			for _, want := range []string{matchRuleID, "sensor.classification.type", token} {
				assert.Contains(t, err.Error(), want, name)
			}
		}
		assert.Empty(t, log.take(t), "the match logs nothing")
	})

	t.Run("semteams' research rules load and match", func(t *testing.T) {
		for _, p := range []string{
			"agent.loop.role",
			"coordinator.decision.next-action",
			"coordinator.decision.subtopics",
			"research.gather.completed-subtopic",
		} {
			vocabulary.Register(p)
		}
		handlers, err := NewHandlers(map[string]ActionHandler{"publish_agent": productHandler{}})
		require.NoError(t, err)

		plan := semantictest.EntityID(t, "c360", "ops", "agentic-loop", "agent", "execution", "plan1")
		// The planner decided to gather two subtopics, as a JSON list in a
		// string, and both gatherers have stamped their completion.
		entity := entityWith(plan,
			message.Triple{Predicate: "agent.loop.role", Object: "researcher-research-plan"},
			message.Triple{Predicate: "coordinator.decision.next-action", Object: "gather"},
			message.Triple{Predicate: "coordinator.decision.subtopics", Object: `["actors","boundaries"]`},
			message.Triple{Predicate: "research.gather.completed-subtopic", Object: "gather-loop-1"},
			message.Triple{Predicate: "research.gather.completed-subtopic", Object: "gather-loop-2"},
		)
		for _, file := range []string{"02-plan-to-gather.json", "03b-synthesize-when-all-gathers-complete.json"} {
			t.Run(file, func(t *testing.T) {
				path := filepath.Join("testdata", "semteams-research", file)
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				def, err := DecodeDefinition(data, path)
				require.NoError(t, err)
				r, err := Load(def, path, handlers)
				require.NoError(t, err)

				fired, err := r.matchEntity(t.Context(), entity, nil)
				require.NoError(t, err)
				assert.True(t, fired, "firing match")

				got, err := r.Matches(t.Context(), entity)
				require.NoError(t, err)
				assert.True(t, got, "Matches")
			})
		}
		assert.Empty(t, log.take(t), "the matches log nothing")
	})
}
