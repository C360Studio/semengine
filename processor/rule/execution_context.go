package rule

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/processor/rule/expression"
)

// ExecutionContext is what a rule's actions run with: the entity or message
// the rule was evaluated for and the rule's state at that evaluation. Its
// substitution methods fill the tokens of an action's templates from it.
//
// Entity, State and MessageData are read-only to an action handler: the rule
// processor builds them for one evaluation, and a handler that changes them
// changes what every later action of that evaluation reads. An
// ExecutionContext is not safe for concurrent use.
type ExecutionContext struct {
	// EntityID is the ID of the entity the rule was evaluated for. To name the
	// entity in a template, use $entity.id: $entity.instance is only the last of
	// its six parts, which other entities may share.
	EntityID string

	// Entity is the entity's state with its triples; nil on the message path.
	Entity *gtypes.EntityState

	// State is the rule's state for this entity at this evaluation; nil before
	// the rule has state.
	State *MatchState

	// MessageData holds the fields of the message the rule was evaluated for,
	// for $message.<path>; nil when no message triggered the evaluation.
	MessageData map[string]any
}

// RuleID returns the ID of the rule whose state the context holds, or "" when
// it holds none.
func (ec *ExecutionContext) RuleID() string {
	if ec == nil || ec.State == nil {
		return ""
	}
	return ec.State.RuleID
}

// SubstituteVariables returns template with each token it holds replaced by
// the token's value. It reads the template once, left to right; text it puts in
// is never read again, so a value holding token text comes out as it is, and
// text outside tokens comes out unchanged.
//
// A token starts with $now or with one of expression.SubstitutionPrefixes:
//
//   - $now: the current time, RFC 3339 in UTC.
//   - $entity.id, and $entity.org, $entity.platform, $entity.system,
//     $entity.domain, $entity.type and $entity.instance, the parts of a valid
//     six-part entity ID.
//   - $state.iteration and $state.max_iterations, from State.
//   - $entity.triple.<predicate>: the object of the predicate's first triple.
//     $entity.triple.<predicate>.triples is a JSON array of every value of the
//     predicate (a scalar triple is one value, a list-valued triple each of its
//     elements), .length the number of those values, and .value the first
//     triple's object when <predicate> is a valid three-part predicate. These
//     three give [], 0 and "" when the predicate is absent.
//   - $message.<path>: the field at a dotted path in MessageData.
//
// A fixed name ($now, $entity.id and the like) is a token only when the
// character after it is not a letter, digit or underscore, so $entity.idx is
// not $entity.id. A triple predicate is the longest run of dot-joined
// segments of [a-z0-9_-], and a message path of [A-Za-z0-9_], a final dot
// excluded.
//
// A token that does not resolve stays as written, and every such token of one
// call is named in one warning with the rule ID. A dollar sign starting
// anything else is ordinary text and is not reported.
func (ec *ExecutionContext) SubstituteVariables(ctx context.Context, template string) string {
	if !strings.Contains(template, "$") {
		return template
	}
	result, unresolved := ec.substitute(template)
	if len(unresolved) > 0 {
		slog.Default().WarnContext(ctx, "rule template tokens did not resolve and stay as written",
			slog.Any("unresolved", unresolved),
			slog.String("template", template),
			slog.String("rule_id", ec.RuleID()),
			slog.String("entity_id", ec.EntityID))
	}
	return result
}

// substitute is the one scan: it copies template, replacing each token that
// resolves, and returns the tokens that did not.
func (ec *ExecutionContext) substitute(template string) (string, []string) {
	prefixes := expression.SubstitutionPrefixes()
	var out strings.Builder
	var unresolved []string
	rest := template
	for {
		i := strings.IndexByte(rest, '$')
		if i < 0 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:i])
		rest = rest[i:]
		if !hasAnyPrefix(rest, prefixes) {
			out.WriteByte('$')
			rest = rest[1:]
			continue
		}
		if value, n, ok := ec.resolve(rest); ok {
			out.WriteString(value)
			rest = rest[n:]
			continue
		}
		n := 1 + dottedRun(rest[1:], isUnresolvedByte)
		unresolved = append(unresolved, rest[:n])
		out.WriteString(rest[:n])
		rest = rest[n:]
	}
	return out.String(), unresolved
}

// resolve reads the token at the start of s, which starts with a substitution
// prefix. It returns the token's value and length, or false when the token
// does not resolve.
func (ec *ExecutionContext) resolve(s string) (string, int, bool) {
	switch {
	case fixedName(s, "$now"):
		return time.Now().UTC().Format(time.RFC3339), len("$now"), true
	case strings.HasPrefix(s, "$entity."):
		return ec.resolveEntity(s)
	case strings.HasPrefix(s, "$state."):
		return ec.resolveState(s)
	case strings.HasPrefix(s, "$message."):
		return resolveMessage(s, ec.MessageData)
	}
	return "", 0, false
}

// resolveState reads $state.iteration and $state.max_iterations.
func (ec *ExecutionContext) resolveState(s string) (string, int, bool) {
	if ec.State == nil {
		return "", 0, false
	}
	switch {
	case fixedName(s, "$state.iteration"):
		return strconv.Itoa(ec.State.Iteration), len("$state.iteration"), true
	case fixedName(s, "$state.max_iterations"):
		return strconv.Itoa(ec.State.MaxIterations), len("$state.max_iterations"), true
	}
	return "", 0, false
}

// fixedName reports whether s starts with name and the character after it is
// not a letter, digit or underscore.
func fixedName(s, name string) bool {
	return strings.HasPrefix(s, name) && (len(s) == len(name) || !isWordByte(s[len(name)]))
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// dottedRun returns the length of the longest run at the start of s of
// non-empty segments of bytes in, joined by single dots, a final dot excluded.
func dottedRun(s string, in func(byte) bool) int {
	end, i := 0, 0
	for {
		start := i
		for i < len(s) && in(s[i]) {
			i++
		}
		if i == start {
			return end
		}
		end = i
		if i >= len(s) || s[i] != '.' {
			return end
		}
		i++
	}
}

func isWordByte(b byte) bool {
	return b == '_' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

func isUnresolvedByte(b byte) bool { return isWordByte(b) || b == '-' }

// substituteConditionValues returns a copy of conds in which each string value
// holding a dollar sign is substituted, so a condition can compare against a
// value such as $entity.triple.<predicate>.length. Other values are kept.
func substituteConditionValues(ctx context.Context, conds []expression.ConditionExpression, ec *ExecutionContext) []expression.ConditionExpression {
	if ec == nil || len(conds) == 0 {
		return conds
	}
	out := make([]expression.ConditionExpression, len(conds))
	for i, c := range conds {
		out[i] = c
		if s, ok := c.Value.(string); ok && strings.Contains(s, "$") {
			out[i].Value = ec.SubstituteVariables(ctx, s)
		}
	}
	return out
}
