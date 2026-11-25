package expression

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/c360studio/semengine/pkg/errs"
)

// maxPatternLength bounds, in characters, the regex pattern a rule author can
// hand the engine.
const maxPatternLength = 500

// SubstitutionPrefixes returns the tokens the rule engine's substituter fills in
// anywhere in a condition value: "$now", "$entity.", "$related.", "$state.",
// "$schedule.", "$caller." and "$message.". Compile refuses a regex pattern
// holding any of them, since a pattern rewritten after it was checked is not
// the pattern that was checked. Each call returns a fresh slice.
func SubstitutionPrefixes() []string {
	return []string{"$now", "$entity.", "$related.", "$state.", "$schedule.", "$caller.", "$message."}
}

// Compile prepares every regex condition in conds for evaluation: it compiles
// each pattern once and keeps it on that element of the slice. Conditions with
// other operators are left alone, as is a regex condition already compiled from
// its current value.
//
// It refuses a value that is not a string, a pattern holding one of
// SubstitutionPrefixes, a pattern over 500 characters, and a pattern regexp
// rejects. The error is classified invalid and names the element's index and
// field; on a refusal no element of conds is changed.
//
// Evaluation never compiles: it refuses a regex condition that Compile did not
// prepare from the condition's current value. A JSON copy of a condition does
// not carry the compiled pattern.
func Compile(conds []ConditionExpression) error {
	compiled := make([]*regexp.Regexp, len(conds))
	for i, cond := range conds {
		if cond.Operator != OpRegexMatch || cond.compiledFromValue() {
			continue
		}
		re, err := compilePattern(cond.Value)
		if err != nil {
			return errs.Classified(errs.ErrorInvalid, fmt.Errorf(
				"condition %d (field %q, operator %q): %w", i, cond.Field, cond.Operator, err))
		}
		compiled[i] = re
	}
	for i, re := range compiled {
		if re != nil {
			conds[i].pattern = re
		}
	}
	return nil
}

func compilePattern(value any) (*regexp.Regexp, error) {
	pattern, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("regex pattern must be a string, got %T", value)
	}
	for _, token := range SubstitutionPrefixes() {
		if strings.Contains(pattern, token) {
			return nil, fmt.Errorf(
				"regex pattern holds the substitution token %q, which would be replaced after the pattern is checked; "+
					"to match its text, write the dollar sign as [$]", token)
		}
	}
	if n := utf8.RuneCountInString(pattern); n > maxPatternLength {
		return nil, fmt.Errorf("regex pattern is %d characters, over the limit of %d", n, maxPatternLength)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}
	return re, nil
}

// compiledFromValue reports whether the kept pattern was compiled from the
// condition's current value.
func (c ConditionExpression) compiledFromValue() bool {
	pattern, ok := c.Value.(string)
	return ok && c.pattern != nil && c.pattern.String() == pattern
}

// checkCompiled refuses a regex condition whose pattern Compile did not
// prepare from the condition's current value; other operators pass.
func checkCompiled(condition ConditionExpression) error {
	if condition.Operator != OpRegexMatch || condition.compiledFromValue() {
		return nil
	}
	return &EvaluationError{
		Field:    condition.Field,
		Operator: condition.Operator,
		Message:  "regex pattern is not compiled from the condition's current value: prepare the conditions with Compile before evaluating",
	}
}

// matchRegex runs a compiled regex condition against the field's string
// form. The evaluator has already refused an unprepared one (checkCompiled).
func matchRegex(condition ConditionExpression, fieldValue any) (bool, error) {
	fieldStr, ok := fieldValue.(string)
	if !ok {
		fieldStr = fmt.Sprintf("%v", fieldValue)
	}
	return condition.pattern.MatchString(fieldStr), nil
}
