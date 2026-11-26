package expression

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// FuzzCompile feeds Compile arbitrary pattern text and checks it against the
// rule as the spec states it, written here without Compile's code: a pattern
// is refused exactly when it holds a substitution token, is over 500
// characters, or regexp.Compile refuses it. A refusal is classified invalid
// and names the condition; an accepted pattern matches as regexp.MatchString
// does.
func FuzzCompile(f *testing.F) {
	// Accepted classes: literals, anchors, other dollar signs, names no
	// substituter fills, the bracketed token, 500 characters, the semteams list
	// pattern.
	for _, seed := range []string{
		"", "drone", `^drone-[0-9]+$`, `[$]now`, `[$a-z]+`, `^\$USD`, `\Q$x\E`, `^[a-z]+$`, `(a+)+$`,
		`$caller.id`, `^x-$related.a`,
		`(?i)\$price`, `^a{1000}$`, strings.Repeat("a", 500), strings.Repeat("é", 500),
		`^\[(?:\s*"(?:[^"\\]|\\.)*"\s*)(?:,\s*"(?:[^"\\]|\\.)*"\s*){0,3}\]$`,
	} {
		f.Add(seed, "drone-42")
	}
	// Refused classes: each token, a token mid-pattern or escaped, too long,
	// invalid syntax, a repeat over 1000, invalid UTF-8.
	for _, seed := range []string{
		"$now", `$entity.id`, `^x-$state.iteration`, `$schedule.next`, `$message.kind`, `^\$now`, strings.Repeat("a", 501), `[unclosed`, `a{1001}`, "(", "\xff",
	} {
		f.Add(seed, "$now")
	}

	f.Fuzz(func(t *testing.T, pattern, text string) {
		conds := []ConditionExpression{{Field: "test.fuzz.v", Operator: OpRegexMatch, Value: pattern}}
		err := Compile(conds)

		_, stdErr := regexp.Compile(pattern)
		wantRefused := stdErr != nil || utf8.RuneCountInString(pattern) > 500
		for _, token := range []string{"$now", "$entity.", "$state.", "$schedule.", "$message."} {
			wantRefused = wantRefused || strings.Contains(pattern, token)
		}

		if wantRefused {
			if err == nil {
				t.Fatalf("Compile(%q) accepted a pattern the rule refuses", pattern)
			}
			if !errs.IsInvalid(err) || !strings.Contains(err.Error(), "condition 0") {
				t.Fatalf("Compile(%q) refusal is not classified invalid naming the condition: %v", pattern, err)
			}
			return
		}
		if err != nil {
			t.Fatalf("Compile(%q) refused a pattern the rule accepts: %v", pattern, err)
		}

		want := regexp.MustCompile(pattern).MatchString(text)
		entity := &gtypes.EntityState{Triples: []message.Triple{{Predicate: "test.fuzz.v", Object: text}}}
		got, evalErr := NewExpressionEvaluator().Evaluate(entity, LogicalExpression{Conditions: conds, Logic: LogicAnd})
		if evalErr != nil {
			t.Fatalf("%q on %q: %v", pattern, text, evalErr)
		}
		if got != want {
			t.Fatalf("%q on %q = %v, regexp.MatchString = %v", pattern, text, got, want)
		}
	})
}
