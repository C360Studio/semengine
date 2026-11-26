package rule

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	gtypes "github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/vocabulary"
)

// Templates often want one part of an entity ID rather than the whole six-part
// string: a tool keyed by the bare instance, say. $entity.org through
// $entity.instance name the six parts, each read from its named field of
// types.ParseEntityID, never from a position in the dotted string, so a token
// keeps its meaning if the order of the parts changes. An entity ID that is not
// a valid six-part ID resolves none of them, so the tokens stay and are
// reported rather than rendering empty.

// entityPart reads one named part of a parsed entity ID.
type entityPart struct {
	name string
	get  func(types.EntityID) string
}

var entityParts = []entityPart{
	{"$entity.org", func(id types.EntityID) string { return id.Org }},
	{"$entity.platform", func(id types.EntityID) string { return id.Platform }},
	{"$entity.system", func(id types.EntityID) string { return id.System }},
	{"$entity.domain", func(id types.EntityID) string { return id.Domain }},
	{"$entity.type", func(id types.EntityID) string { return id.Type }},
	{"$entity.instance", func(id types.EntityID) string { return id.Instance }},
}

const triplePrefix = "$entity.triple."

// resolveEntity reads a token starting with $entity.: the ID, one of its parts,
// or a triple.
func (ec *ExecutionContext) resolveEntity(s string) (string, int, bool) {
	if fixedName(s, "$entity.id") {
		return ec.EntityID, len("$entity.id"), true
	}
	if strings.HasPrefix(s, triplePrefix) {
		return ec.resolveTriple(s)
	}
	for _, part := range entityParts {
		if !fixedName(s, part.name) {
			continue
		}
		parsed, err := types.ParseEntityID(ec.EntityID)
		if err != nil {
			return "", 0, false
		}
		return part.get(parsed), len(part.name), true
	}
	return "", 0, false
}

// resolveTriple reads $entity.triple.<path>. The path is a suffix form
// (.length, .triples, or .value after a valid three-part predicate) or, when
// it is none of those, the whole predicate.
func (ec *ExecutionContext) resolveTriple(s string) (string, int, bool) {
	path := s[len(triplePrefix):]
	path = path[:dottedRun(path, isPredicateByte)]
	if path == "" {
		return "", 0, false
	}
	n := len(triplePrefix) + len(path)
	if predicate, ok := strings.CutSuffix(path, ".length"); ok {
		return strconv.Itoa(len(collectPredicateValues(ec.Entity, predicate))), n, true
	}
	if predicate, ok := strings.CutSuffix(path, ".triples"); ok {
		values := collectPredicateValues(ec.Entity, predicate)
		if values == nil {
			values = []string{} // [] for an absent predicate, never null
		}
		encoded, err := json.Marshal(values)
		if err != nil {
			return "", 0, false
		}
		return string(encoded), n, true
	}
	if predicate, ok := strings.CutSuffix(path, ".value"); ok {
		if _, err := vocabulary.ParsePredicate(predicate); err == nil {
			object, _ := firstObject(ec.Entity, predicate)
			return object, n, true
		}
	}
	object, ok := firstObject(ec.Entity, path)
	if !ok {
		return "", 0, false
	}
	return object, n, true
}

// firstObject returns the object of the predicate's first triple on entity,
// as text.
func firstObject(entity *gtypes.EntityState, predicate string) (string, bool) {
	if entity == nil {
		return "", false
	}
	for _, triple := range entity.Triples {
		if triple.Predicate == predicate {
			return fmt.Sprintf("%v", triple.Object), true
		}
	}
	return "", false
}

// collectPredicateValues returns every value of predicate on entity, in triple
// order: a list-shaped object contributes each element, any other object
// itself. It returns nil when the predicate is absent.
func collectPredicateValues(entity *gtypes.EntityState, predicate string) []string {
	if entity == nil {
		return nil
	}
	var out []string
	for _, triple := range entity.Triples {
		if triple.Predicate != predicate {
			continue
		}
		if items, ok := coerceTripleObjectToStrings(triple.Object); ok {
			out = append(out, items...)
			continue
		}
		out = append(out, fmt.Sprintf("%v", triple.Object))
	}
	return out
}

// coerceTripleObjectToStrings returns the elements of a list-shaped triple
// object as text. A list is a []string, a []any (a list after a JSON decode),
// or a string holding a JSON array, the form a tool uses to keep the object a
// plain string on the wire. Non-string elements are written with %v, so the
// numbers [1, 2] become "1" and "2". It returns false for any other object.
func coerceTripleObjectToStrings(obj any) ([]string, bool) {
	switch v := obj.(type) {
	case []string:
		return v, true
	case []any:
		return stringsOf(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if !strings.HasPrefix(trimmed, "[") {
			return nil, false
		}
		var parsed []any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, false
		}
		return stringsOf(parsed), true
	}
	return nil, false
}

func stringsOf(items []any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprintf("%v", item))
	}
	return out
}

func isPredicateByte(b byte) bool {
	return b == '_' || b == '-' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z'
}
