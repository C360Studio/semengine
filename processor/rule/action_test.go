package rule

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/pkg/errs"
)

// productHandler is a product's action handler: it accepts every action.
type productHandler struct{}

func (productHandler) Check(Action) error { return nil }

func handlerTypes(h Handlers) []string {
	names := make([]string, 0, len(h.byType))
	for name := range h.byType {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestNewHandlers(t *testing.T) {
	// The nine built-in type names, written out from the rule-definitions
	// spec rather than read from the package.
	builtins := []string{
		"add_triple", "lifecycle_complete", "lifecycle_fail", "lifecycle_transition",
		"publish", "reconcile_predicates", "remove_triple", "update_kv", "update_triple",
	}

	t.Run("no product handlers", func(t *testing.T) {
		for _, product := range []map[string]ActionHandler{nil, {}} {
			h, err := NewHandlers(product)
			require.NoError(t, err)
			assert.Equal(t, builtins, handlerTypes(h))
		}
	})

	t.Run("product handlers join the built-ins", func(t *testing.T) {
		product := map[string]ActionHandler{"publish_agent": productHandler{}, "Publish": productHandler{}}
		h, err := NewHandlers(product)
		require.NoError(t, err)
		want := append([]string{"Publish", "publish_agent"}, builtins...)
		sort.Strings(want)
		assert.Equal(t, want, handlerTypes(h), "a name is compared as written, so Publish is not publish")

		// A later change to the caller's map does not reach the set.
		product["late"] = productHandler{}
		delete(product, "publish_agent")
		assert.Equal(t, want, handlerTypes(h))
	})

	refusals := []struct {
		name    string
		product map[string]ActionHandler
		names   string
	}{
		{"a built-in name", map[string]ActionHandler{"publish": productHandler{}}, `"publish"`},
		{"another built-in name", map[string]ActionHandler{"ok": productHandler{}, "lifecycle_fail": productHandler{}}, `"lifecycle_fail"`},
		{"an empty name", map[string]ActionHandler{"": productHandler{}}, "empty"},
		{"a nil handler", map[string]ActionHandler{"publish_agent": nil}, `"publish_agent"`},
	}
	for _, tc := range refusals {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			h, err := NewHandlers(tc.product)
			require.Error(t, err)
			assert.True(t, errs.IsInvalid(err), "classified invalid: %v", err)
			assert.Contains(t, err.Error(), tc.names)
			assert.Nil(t, h.byType, "a refusal builds no set")
		})
	}
}

func TestActionBody(t *testing.T) {
	t.Run("marshalling writes Body", func(t *testing.T) {
		var a Action
		require.NoError(t, json.Unmarshal(
			[]byte(`{"type": "publish", "subject": "a.b", "extra": {"k": "<&>"}, "max_iterations": 2}`), &a))

		got, err := json.Marshal(a)
		require.NoError(t, err)
		// Key order and the key no core field holds are as written; only
		// spacing and escaping differ, which json.Marshal applies to any
		// raw JSON it writes.
		assert.Equal(t, `{"type":"publish","subject":"a.b","extra":{"k":"\u003c\u0026\u003e"},"max_iterations":2}`,
			string(got))

		// A changed copy is not written: the body is the action.
		a.Type = "remove_triple"
		got, err = json.Marshal(a)
		require.NoError(t, err)
		assert.Contains(t, string(got), `"type":"publish"`)
	})

	t.Run("marshalling refuses an action with no body", func(t *testing.T) {
		_, err := json.Marshal(Action{Type: "publish"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "build an action by decoding")
	})
}

// TestBodyKeepsBytes decodes semteams' rule-family fixtures. Each action's
// Body must be the file's bytes for that action, and the settings no core
// field holds must decode to the fixtures' independently written values.
func TestBodyKeepsBytes(t *testing.T) {
	dir := filepath.Join("testdata", "rule-family-consumer")
	expectedRaw, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	require.NoError(t, err)
	var expected map[string]map[string]any
	require.NoError(t, json.Unmarshal(expectedRaw, &expected))
	require.Len(t, expected, 3)

	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			file, err := os.ReadFile(path)
			require.NoError(t, err)

			// The fixtures are indented two spaces per level, so the one
			// on_enter action runs from the `{` after `"on_enter": [` to the
			// four-space `}` before the closing `]`.
			start := bytes.Index(file, []byte("\"on_enter\": [\n    {"))
			end := bytes.LastIndex(file, []byte("\n    }\n  ]"))
			require.True(t, start >= 0 && end > start, "fixture layout")
			wantBytes := file[start+len("\"on_enter\": [\n    ") : end+len("\n    }")]

			def, err := DecodeDefinition(file, path)
			require.NoError(t, err)
			require.Len(t, def.OnEnter, 1)
			assert.Equal(t, string(wantBytes), string(def.OnEnter[0].Body), "Body is the file's bytes")

			var body map[string]any
			require.NoError(t, json.Unmarshal(def.OnEnter[0].Body, &body))
			for _, key := range []string{"subject", "role", "model", "tool_choice", "response_format"} {
				// expected.json writes null for a setting the action does not carry.
				assert.True(t, reflect.DeepEqual(want[key], body[key]), "%s: got %v, want %v", key, body[key], want[key])
			}
		})
	}
}
