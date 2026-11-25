package expression

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// Requirement: rule-conditions/"Conditions combine by and or or", "List operators see every triple of a predicate"
// and "A regex pattern is compiled before evaluation".
//
// The laws, each checked against an oracle the test owns:
//   - and is true exactly when every condition is, or when at least one is, an
//     empty list passes, and empty logic means and;
//   - not_in is the negation of in, and in is list membership;
//   - between [lo, hi] equals gte lo and lte hi, and both equal lo <= v <= hi;
//   - a compiled regex answers what regexp.MatchString answers on the
//     field's text, and Compile refuses exactly the patterns regexp refuses.
//
// The boundaries (an empty list, lo == v == hi, lo > hi, an empty pattern) are
// named witnesses run through the same checks before the generated run, so no
// boundary depends on a random draw reaching it. Every check runs on every
// case: none sits behind a guard.

// numericCondition is one comparison on its own field, with its expected
// truth computed here from the integers.
type numericCondition struct {
	op    string
	value int
	field int
}

func (c numericCondition) truth() bool {
	switch c.op {
	case OpEqual:
		return c.field == c.value
	case OpNotEqual:
		return c.field != c.value
	case OpLessThan:
		return c.field < c.value
	case OpLessThanEqual:
		return c.field <= c.value
	case OpGreaterThan:
		return c.field > c.value
	case OpGreaterThanEqual:
		return c.field >= c.value
	}
	panic("unknown operator " + c.op)
}

type failer interface {
	Helper()
	Fatalf(format string, args ...any)
}

func checkLogic(t failer, logic string, conds []numericCondition) {
	t.Helper()
	entity := &gtypes.EntityState{}
	expr := LogicalExpression{Logic: logic}
	every, some := true, false
	for i, c := range conds {
		field := fmt.Sprintf("test.prop.f%d", i)
		entity.Triples = append(entity.Triples, message.Triple{Predicate: field, Object: float64(c.field)})
		expr.Conditions = append(expr.Conditions, ConditionExpression{Field: field, Operator: c.op, Value: float64(c.value)})
		every = every && c.truth()
		some = some || c.truth()
	}
	want := every
	if logic == LogicOr {
		want = some
	}
	if len(conds) == 0 {
		want = true
	}

	got, err := NewExpressionEvaluator().Evaluate(entity, expr)
	if err != nil {
		t.Fatalf("logic %q over %+v: %v", logic, conds, err)
	}
	if got != want {
		t.Fatalf("logic %q over %+v = %v, want %v", logic, conds, got, want)
	}
}

func evalSingle(t failer, field any, op string, value any) bool {
	t.Helper()
	entity := &gtypes.EntityState{Triples: []message.Triple{{Predicate: "test.prop.v", Object: field}}}
	got, err := NewExpressionEvaluator().Evaluate(entity, LogicalExpression{
		Conditions: []ConditionExpression{{Field: "test.prop.v", Operator: op, Value: value}},
		Logic:      LogicAnd,
	})
	if err != nil {
		t.Fatalf("%v %s %v: %v", field, op, value, err)
	}
	return got
}

func checkMembership(t failer, v int, list []int) {
	t.Helper()
	values := make([]any, len(list))
	for i, n := range list {
		values[i] = float64(n)
	}
	in := evalSingle(t, float64(v), OpIn, values)
	notIn := evalSingle(t, float64(v), OpNotIn, values)
	if want := slices.Contains(list, v); in != want {
		t.Fatalf("%d in %v = %v, want %v", v, list, in, want)
	}
	if notIn == in {
		t.Fatalf("%d not_in %v = %v, the same as in", v, list, notIn)
	}
}

func checkBetween(t failer, v, lo, hi int) {
	t.Helper()
	between := evalSingle(t, float64(v), OpBetween, []any{float64(lo), float64(hi)})
	gte := evalSingle(t, float64(v), OpGreaterThanEqual, float64(lo))
	lte := evalSingle(t, float64(v), OpLessThanEqual, float64(hi))
	want := lo <= v && v <= hi
	if between != want || (gte && lte) != want {
		t.Fatalf("%d between [%d, %d] = %v, gte && lte = %v, want %v", v, lo, hi, between, gte && lte, want)
	}
}

func checkRegex(t failer, pattern, text string) {
	t.Helper()
	conds := []ConditionExpression{{Field: "test.prop.v", Operator: OpRegexMatch, Value: pattern}}
	err := Compile(conds)
	want, stdErr := regexp.MatchString(pattern, text)
	if (err != nil) != (stdErr != nil) {
		t.Fatalf("Compile(%q) = %v, regexp says %v", pattern, err, stdErr)
	}
	if err != nil {
		return // both refuse; nothing to evaluate
	}
	entity := &gtypes.EntityState{Triples: []message.Triple{{Predicate: "test.prop.v", Object: text}}}
	got, evalErr := NewExpressionEvaluator().Evaluate(entity, LogicalExpression{Conditions: conds, Logic: LogicAnd})
	if evalErr != nil {
		t.Fatalf("%q on %q: %v", pattern, text, evalErr)
	}
	if got != want {
		t.Fatalf("%q on %q = %v, regexp.MatchString = %v", pattern, text, got, want)
	}
}

// regexPieces build patterns from a small alphabet, so generated patterns are
// often valid and often match, and sometimes are not valid at all.
var regexPieces = []string{"a", "b", "$", "^", ".", "*", "+", "?", "[ab]", "[$]", "(", ")", "|", `\$`, "{2}"}

func TestPropConditions(t *testing.T) {
	// Named witnesses: the boundaries every run must check.
	for _, logic := range []string{LogicAnd, LogicOr, ""} {
		checkLogic(t, logic, nil)
		checkLogic(t, logic, []numericCondition{{OpLessThan, 5, 1}, {OpGreaterThan, 5, 1}})
		checkLogic(t, logic, []numericCondition{{OpEqual, 3, 3}, {OpLessThanEqual, 3, 3}})
	}
	checkMembership(t, 1, nil)
	checkMembership(t, 1, []int{1})
	checkBetween(t, 4, 4, 4)
	checkBetween(t, 4, 5, 3)
	checkBetween(t, 3, 4, 5)
	checkRegex(t, "", "")
	checkRegex(t, "a$", "a")
	checkRegex(t, "(", "a")

	ops := []string{OpEqual, OpNotEqual, OpLessThan, OpLessThanEqual, OpGreaterThan, OpGreaterThanEqual}
	small := rapid.IntRange(-3, 3)
	rapid.Check(t, func(rt *rapid.T) {
		conds := rapid.SliceOfN(rapid.Custom(func(rt *rapid.T) numericCondition {
			return numericCondition{
				op:    rapid.SampledFrom(ops).Draw(rt, "op"),
				value: small.Draw(rt, "value"),
				field: small.Draw(rt, "field"),
			}
		}), 0, 5).Draw(rt, "conditions")
		checkLogic(rt, rapid.SampledFrom([]string{LogicAnd, LogicOr, ""}).Draw(rt, "logic"), conds)

		checkMembership(rt, small.Draw(rt, "member"), rapid.SliceOfN(small, 0, 4).Draw(rt, "list"))
		checkBetween(rt, small.Draw(rt, "v"), small.Draw(rt, "lo"), small.Draw(rt, "hi"))

		pieces := rapid.SliceOfN(rapid.SampledFrom(regexPieces), 0, 6).Draw(rt, "pattern")
		text := rapid.StringOfN(rapid.SampledFrom([]rune("ab$")), 0, 6, -1).Draw(rt, "text")
		checkRegex(rt, strings.Join(pieces, ""), text)
	})
}
