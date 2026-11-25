package expression

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/semantictest"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

const regexField = "robotics.system.status"

// regexEntity returns an entity holding one triple, regexField = object.
func regexEntity(t *testing.T, object any) *gtypes.EntityState {
	t.Helper()
	id := semantictest.EntityID(t, "test", "rule", "expression", "regex", "drone", "001")
	return createTestEntity(id, []message.Triple{
		{Subject: id, Predicate: regexField, Object: object, Source: "test", Timestamp: time.Now()},
	})
}

func regexCondition(pattern any) ConditionExpression {
	return ConditionExpression{Field: regexField, Operator: OpRegexMatch, Value: pattern, Required: true}
}

func evaluateOne(t *testing.T, entity *gtypes.EntityState, cond ConditionExpression) (bool, error) {
	t.Helper()
	return NewExpressionEvaluator().Evaluate(entity, LogicalExpression{
		Conditions: []ConditionExpression{cond},
		Logic:      LogicAnd,
	})
}

// The standard library is the oracle: a compiled condition answers what
// regexp.MatchString answers for the same pattern and the field's text.
func TestCompiledRegexMatchesAsStandardLibrary(t *testing.T) {
	cases := []struct {
		pattern string
		object  any
		text    string // the field's string form, written out by hand
	}{
		{`^a.*z$`, "abcz", "abcz"},
		{`^a.*z$`, "abczx", "abczx"},
		{`^ARMED_.*`, "ARMED_READY", "ARMED_READY"},
		{`^ARMED_.*`, "DISARMED", "DISARMED"},
		{`^[0-9]+$`, 12345, "12345"},
		{`^12\.5$`, 12.5, "12.5"},
		{`^true$`, true, "true"},
		{`(?i)\$price`, "$PRICE", "$PRICE"},
	}
	for _, tc := range cases {
		conds := []ConditionExpression{regexCondition(tc.pattern)}
		require.NoError(t, Compile(conds))

		want, err := regexp.MatchString(tc.pattern, tc.text)
		require.NoError(t, err)

		got, err := evaluateOne(t, regexEntity(t, tc.object), conds[0])
		require.NoError(t, err, "pattern %q on %v", tc.pattern, tc.object)
		assert.Equal(t, want, got, "pattern %q on %v", tc.pattern, tc.object)
	}
}

// Compile writes by index, so every element of the caller's slice is ready,
// not a loop copy.
func TestCompileSliceThenEvaluateEachElement(t *testing.T) {
	conds := []ConditionExpression{
		{Field: regexField, Operator: OpEqual, Value: "ARMED_READY", Required: true},
		regexCondition(`^ARMED_`),
		regexCondition(`_READY$`),
		regexCondition(`^DISARMED`),
	}
	require.NoError(t, Compile(conds))

	entity := regexEntity(t, "ARMED_READY")
	want := []bool{true, true, true, false}
	for i, cond := range conds {
		got, err := evaluateOne(t, entity, cond)
		require.NoError(t, err, "condition %d", i)
		assert.Equal(t, want[i], got, "condition %d", i)
	}

	got, err := NewExpressionEvaluator().Evaluate(entity, LogicalExpression{Conditions: conds[:3], Logic: LogicAnd})
	require.NoError(t, err)
	assert.True(t, got)
}

func TestEvaluateRefusesUncompiledRegex(t *testing.T) {
	cond := regexCondition(`^ARMED_`)

	got, err := evaluateOne(t, regexEntity(t, "ARMED_READY"), cond)

	assert.False(t, got)
	var evalErr *EvaluationError
	require.ErrorAs(t, err, &evalErr)
	assert.Equal(t, regexField, evalErr.Field)
	assert.Equal(t, OpRegexMatch, evalErr.Operator)
}

func TestEvaluateRefusesRegexEditedAfterCompile(t *testing.T) {
	for name, edit := range map[string]any{
		"another pattern":   `^DISARMED`,
		"a non-string":      42,
		"a matching prefix": `^ARMED`,
	} {
		t.Run(name, func(t *testing.T) {
			conds := []ConditionExpression{regexCondition(`^ARMED_`)}
			require.NoError(t, Compile(conds))
			conds[0].Value = edit

			got, err := evaluateOne(t, regexEntity(t, "ARMED_READY"), conds[0])

			assert.False(t, got)
			var evalErr *EvaluationError
			require.ErrorAs(t, err, &evalErr)
			assert.Equal(t, regexField, evalErr.Field)
		})
	}
}

// The seven tokens the substituter fills in anywhere in a value.
var wantSubstitutionPrefixes = []string{
	"$now", "$entity.", "$related.", "$state.", "$schedule.", "$caller.", "$message.",
}

func TestSubstitutionPrefixes(t *testing.T) {
	assert.ElementsMatch(t, wantSubstitutionPrefixes, SubstitutionPrefixes())

	first := SubstitutionPrefixes()
	first[0] = "changed"
	assert.ElementsMatch(t, wantSubstitutionPrefixes, SubstitutionPrefixes(),
		"each call returns a fresh slice")
}

func TestCompileRefusals(t *testing.T) {
	cases := map[string]struct {
		value    any
		wantText string
	}{
		"non-string value":   {value: 42, wantText: "string"},
		"nil value":          {value: nil, wantText: "string"},
		"regexp rejects":     {value: `[unclosed`, wantText: "missing closing ]"},
		"repeat over 1000":   {value: `a{1001}`, wantText: "invalid repeat count"},
		"501 characters":     {value: strings.Repeat("a", 501), wantText: "500"},
		"token whole":        {value: `$entity.id`, wantText: "[$]"},
		"token mid-pattern":  {value: `^x-$now`, wantText: "[$]"},
		"token after anchor": {value: `^x-$state.iteration`, wantText: "[$]"},
		"escaped token":      {value: `^\$now`, wantText: "[$]"},
	}
	for _, prefix := range wantSubstitutionPrefixes {
		cases["prefix "+prefix] = struct {
			value    any
			wantText string
		}{value: "^" + prefix + "x", wantText: prefix}
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			conds := []ConditionExpression{
				{Field: "a.b.c", Operator: OpEqual, Value: 1},
				regexCondition(`^ok$`),
				{Field: "x.y.bad", Operator: OpRegexMatch, Value: tc.value},
			}

			err := Compile(conds)

			require.Error(t, err)
			assert.True(t, errs.IsInvalid(err), "refusal is classified invalid: %v", err)
			assert.Contains(t, err.Error(), "condition 2")
			assert.Contains(t, err.Error(), `"x.y.bad"`)
			assert.Contains(t, err.Error(), tc.wantText)

			// A refused slice is left as it was: the good pattern before
			// the bad one is not half-prepared.
			_, evalErr := evaluateOne(t, regexEntity(t, "ok"), conds[1])
			var target *EvaluationError
			assert.True(t, errors.As(evalErr, &target), "condition 1 stays uncompiled, got %v", evalErr)
		})
	}
}

// semteams' one regex condition, as its rule file spells it
// (configs/rules/research/02-plan-to-gather.json): a JSON list of one to four
// strings.
const semteamsListPatternJSON = `"^\\[(?:\\s*\"(?:[^\"\\\\]|\\\\.)*\"\\s*)(?:,\\s*\"(?:[^\"\\\\]|\\\\.)*\"\\s*){0,3}\\]$"`

func TestCompileAccepts(t *testing.T) {
	var semteams string
	require.NoError(t, json.Unmarshal([]byte(semteamsListPatternJSON), &semteams))

	cases := map[string]struct {
		pattern string
		match   string
	}{
		"bracketed dollar":         {`[$]now`, "$now"},
		"dollar in a class":        {`[$a-z]+`, "$"},
		"escaped dollar":           {`^\$USD`, "$USD"},
		"quoted dollar":            {`\Q$x\E`, "$x"},
		"end-anchored":             {`^[a-z]+$`, "abc"},
		"500 characters":           {strings.Repeat("a", 500), strings.Repeat("a", 500)},
		"500 two-byte runes":       {strings.Repeat("é", 500), strings.Repeat("é", 500)},
		"nested repeat":            {`(a+)+$`, "aaa"},
		"semteams list pattern":    {semteams, `["a", "b"]`},
		"repeat at the regexp max": {`^a{1000}$`, strings.Repeat("a", 1000)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			conds := []ConditionExpression{regexCondition(tc.pattern)}
			require.NoError(t, Compile(conds))

			got, err := evaluateOne(t, regexEntity(t, tc.match), conds[0])
			require.NoError(t, err)
			assert.True(t, got, "%q matches %q", tc.pattern, tc.match)
		})
	}
}

func TestCompileIgnoresOtherOperators(t *testing.T) {
	conds := []ConditionExpression{
		{Field: "a.b.c", Operator: OpEqual, Value: 42},
		{Field: "a.b.c", Operator: OpContains, Value: "$now"},
		{Field: "a.b.c", Operator: OpIn, Value: []any{"[unclosed"}},
	}
	assert.NoError(t, Compile(conds))
	assert.NoError(t, Compile(nil))
}

func TestCompileTwiceChangesNothing(t *testing.T) {
	conds := []ConditionExpression{regexCondition(`^ARMED_`)}
	require.NoError(t, Compile(conds))
	first := conds[0]

	require.NoError(t, Compile(conds))

	assert.Same(t, first.pattern, conds[0].pattern)
	assert.Equal(t, first, conds[0])
}

// A Go copy keeps the compiled pattern; a JSON copy does not, and is refused
// at evaluation rather than compiled there.
func TestCopiesOfACompiledCondition(t *testing.T) {
	conds := []ConditionExpression{regexCondition(`^ARMED_`)}
	require.NoError(t, Compile(conds))
	entity := regexEntity(t, "ARMED_READY")

	goCopy := conds[0]
	got, err := evaluateOne(t, entity, goCopy)
	require.NoError(t, err)
	assert.True(t, got)

	raw, err := json.Marshal(conds[0])
	require.NoError(t, err)
	var jsonCopy ConditionExpression
	require.NoError(t, json.Unmarshal(raw, &jsonCopy))
	_, err = evaluateOne(t, entity, jsonCopy)
	var evalErr *EvaluationError
	assert.ErrorAs(t, err, &evalErr)
}
