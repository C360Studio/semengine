package rule

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"pgregory.net/rapid"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-matching/"Firing and asking share one match".
//
// The law: for an enabled rule with an entity pattern, an entity inside it
// and conditions that read no transition, $state.*, $prev.* or
// $entity.lifecycle.* in field or value, the firing match run with no
// lifecycle lookup and Matches give the same answer: equal verdicts with no
// error, or an error from both. The hand-written corpus in matches_test.go
// says which answer is right; this check only guards the two from drifting
// apart.
//
// The generator draws operators whose value cannot be used (length_eq
// "abc"), condition values with a token that does not resolve, and required
// fields the entity lacks, so the error and the absent-required answers are
// reached as well as true and false. Each of the four outcomes is a named
// witness run through the same check before the generated run.

// agreement is the answer both matches gave.
type agreement int

const (
	agreeTrue agreement = iota
	agreeFalse
	agreeError
)

// checkFiringAndAskingAgree runs the firing match with no lookup and Matches
// on one entity and fails unless they agree. It returns the shared answer.
func checkFiringAndAskingAgree(ctx context.Context, t failer, r *Rule, state *gtypes.EntityState) agreement {
	t.Helper()
	fired, fireErr := r.matchEntity(ctx, state, nil)
	asked, askErr := r.Matches(ctx, state)
	switch {
	case fireErr != nil && askErr != nil:
		return agreeError
	case fireErr != nil || askErr != nil:
		t.Fatalf("one match errs and the other does not: firing (%t, %v), Matches (%t, %v); rule %s, entity %+v",
			fired, fireErr, asked, askErr, mustJSON(r.def.Conditions), state.Triples)
	case fired != asked:
		t.Fatalf("the verdicts differ: firing %t, Matches %t; rule %s, entity %+v",
			fired, asked, mustJSON(r.def.Conditions), state.Triples)
	}
	if fired {
		return agreeTrue
	}
	return agreeFalse
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v (not JSON: %v)", v, err)
	}
	return string(b)
}

// propCondition is one generated condition, written as the JSON a rule pack
// carries.
type propCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Required bool   `json:"required,omitempty"`
}

// loadProp loads an enabled rule watching matchPattern. The predicates are
// declared by the caller before any draw.
func loadProp(t failer, handlers Handlers, logic string, conditions []propCondition) *Rule {
	t.Helper()
	rule := fmt.Sprintf(`{"id": %q, "type": "expression", "logic": %q, "entity": {"pattern": %q}, "conditions": %s}`,
		matchRuleID, logic, matchPattern, mustJSON(conditions))
	def, err := DecodeDefinition([]byte(rule), loadSource)
	if err != nil {
		t.Fatalf("decode %s: %v", rule, err)
	}
	r, err := Load(def, loadSource, handlers)
	if err != nil {
		t.Fatalf("load %s: %v", rule, err)
	}
	return r
}

// propPredicates are the fields generated conditions read; declareMatchPredicates
// declares each.
var propPredicates = []string{
	"sensor.measurement.fahrenheit",
	"sensor.classification.type",
	"geo.location.zone",
	"gather.child.completed",
	"decision.subtopics.list",
}

// propOperators are every registered operator but transition, which Matches
// refuses, and regex, whose values are drawn apart because Load compiles them.
var propOperators = []string{
	"eq", "ne", "lt", "lte", "gt", "gte", "between", "contains", "starts_with", "ends_with",
	"in", "not_in", "length_eq", "length_gt", "length_gte", "length_lt", "length_lte", "array_contains",
}

// propValues are literal condition values, "abc" among them, which a length
// operator cannot use.
var propValues = []any{
	40.0, 9.9, 10, -1, "abc", "temperature", "cold-storage", "9.9", "", []any{"a", "temperature"}, []any{0, 50},
}

// propTemplates are condition values filled from the entity. The last does
// not resolve on any generated entity, which never holds that predicate.
var propTemplates = []string{
	"$entity.id",
	"$entity.instance",
	"$entity.triple.sensor.classification.type",
	"$entity.triple.sensor.classification.type.value",
	"$entity.triple.decision.subtopics.list.length",
	"$entity.triple.gather.child.completed.triples",
	"$entity.triple.x.y.missing",
}

// propRegexes are regex values Load compiles.
var propRegexes = []string{"^temp", "cold", `^[0-9.]+$`}

// propObjects are triple objects: numbers, numeric text, text, and lists in
// both their Go and their JSON-text forms.
var propObjects = []any{41.2, 9.9, 3, "9.9", "temperature", "zone.cold-storage-1", "a", []string{"a", "b"}, `["a","b"]`}

func TestFiringAndAskingAgree(t *testing.T) {
	declareMatchPredicates()
	handlers := builtinHandlers(t)
	ctx := t.Context()
	id := sensorID(t, "001")
	kind := message.Triple{Predicate: "sensor.classification.type", Object: "temperature"}

	t.Run("witnesses", func(t *testing.T) {
		for _, w := range []struct {
			name       string
			conditions []propCondition
			triples    []message.Triple
			want       agreement
		}{
			{
				name:       "conditions that hold",
				conditions: []propCondition{{Field: "sensor.classification.type", Operator: "eq", Value: "temperature"}},
				triples:    []message.Triple{kind},
				want:       agreeTrue,
			},
			{
				name:       "conditions that do not hold",
				conditions: []propCondition{{Field: "sensor.classification.type", Operator: "eq", Value: "humidity"}},
				triples:    []message.Triple{kind},
				want:       agreeFalse,
			},
			{
				name: "an absent required field",
				conditions: []propCondition{{Field: "sensor.measurement.fahrenheit", Operator: "gte", Value: 40.0,
					Required: true}},
				triples: []message.Triple{kind},
				want:    agreeFalse,
			},
			{
				name:       "an operator that cannot use its value",
				conditions: []propCondition{{Field: "sensor.classification.type", Operator: "length_eq", Value: "abc"}},
				triples:    []message.Triple{kind},
				want:       agreeError,
			},
			{
				name: "a value token that does not resolve",
				conditions: []propCondition{{Field: "sensor.classification.type", Operator: "eq",
					Value: "$entity.triple.x.y.missing"}},
				triples: []message.Triple{kind},
				want:    agreeError,
			},
		} {
			r := loadProp(t, handlers, "and", w.conditions)
			if got := checkFiringAndAskingAgree(ctx, t, r, entityWith(id, w.triples...)); got != w.want {
				t.Fatalf("%s: both matches answered %d, want %d", w.name, got, w.want)
			}
		}
	})

	t.Run("generated", func(t *testing.T) {
		condition := rapid.Custom(func(rt *rapid.T) propCondition {
			c := propCondition{
				Field:    rapid.SampledFrom(propPredicates).Draw(rt, "field"),
				Required: rapid.Bool().Draw(rt, "required"),
			}
			switch rapid.SampledFrom([]string{"literal", "template", "regex"}).Draw(rt, "value kind") {
			case "literal":
				c.Operator = rapid.SampledFrom(propOperators).Draw(rt, "operator")
				c.Value = rapid.SampledFrom(propValues).Draw(rt, "value")
			case "template":
				c.Operator = rapid.SampledFrom(propOperators).Draw(rt, "operator")
				c.Value = rapid.SampledFrom(propTemplates).Draw(rt, "template")
			default:
				c.Operator = "regex"
				c.Value = rapid.SampledFrom(propRegexes).Draw(rt, "pattern")
			}
			return c
		})
		triple := rapid.Custom(func(rt *rapid.T) message.Triple {
			return message.Triple{
				Predicate: rapid.SampledFrom(propPredicates).Draw(rt, "predicate"),
				Object:    rapid.SampledFrom(propObjects).Draw(rt, "object"),
			}
		})

		rapid.Check(t, func(rt *rapid.T) {
			logic := rapid.SampledFrom([]string{"and", "or"}).Draw(rt, "logic")
			r := loadProp(rt, handlers, logic, rapid.SliceOfN(condition, 1, 3).Draw(rt, "conditions"))
			instance := rapid.StringMatching(`[a-z0-9]{1,6}`).Draw(rt, "instance")
			state := entityWith(matchPattern[:len(matchPattern)-1]+instance,
				rapid.SliceOfN(triple, 0, 5).Draw(rt, "triples")...)
			checkFiringAndAskingAgree(ctx, rt, r, state)
		})
	})
}
