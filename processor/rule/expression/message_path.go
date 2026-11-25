package expression

import "strings"

// ExtractMessageValue walks data along a dotted path. It is the one reading of
// a `$message.` path, for conditions here and for the rule engine's value
// substitution. It returns the terminal value and true on success, and false
// (the field is not present) when:
//
//   - any non-terminal segment is missing from the current map, OR
//   - any non-terminal segment resolves to a non-map value (can't
//     descend further), OR
//   - the terminal segment is missing.
//
// A nil terminal value is a successful resolution: JSON null is not
// "missing".
//
// The path is the bare dotted form (no `$message.` prefix). Callers
// trim the prefix before calling.
func ExtractMessageValue(data MessageFields, path string) (any, bool) {
	if len(data) == 0 || path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var current any = map[string]any(data)
	for i, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, exists := m[part]
		if !exists {
			return nil, false
		}
		if i == len(parts)-1 {
			return value, true
		}
		current = value
	}
	return nil, false
}
