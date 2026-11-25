package rule

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeTestAction(t *testing.T, body string) Action {
	t.Helper()
	var a Action
	require.NoError(t, json.Unmarshal([]byte(body), &a))
	return a
}

func testCapKey(t *testing.T, ruleID, body string) string {
	t.Helper()
	key, err := capKey(ruleID, decodeTestAction(t, body))
	require.NoError(t, err)
	return key
}

func TestCapKey(t *testing.T) {
	const body = `{
	  "type": "publish",
	  "subject": "alerts.a&b",
	  "properties": {"weight": 1.50, "html": "<b>"},
	  "max_iterations": 3
	}`

	t.Run("hash over rule ID, type and canonical body", func(t *testing.T) {
		// The canonical form written by hand: keys sorted, numbers as
		// written, no spacing, no HTML escaping.
		canonical := `{"max_iterations":3,"properties":{"html":"<b>","weight":1.50},"subject":"alerts.a&b","type":"publish"}`
		h := sha256.New()
		for _, part := range []string{"rule_a", "publish", canonical} {
			fmt.Fprintf(h, "%d:%s", len(part), part)
		}
		want := hex.EncodeToString(h.Sum(nil))[:16]
		assert.Equal(t, want, testCapKey(t, "rule_a", body))
	})

	t.Run("storing and reformatting keep the key", func(t *testing.T) {
		key := testCapKey(t, "rule_a", body)
		stored, err := json.Marshal(decodeTestAction(t, body))
		require.NoError(t, err)
		require.Contains(t, string(stored), `\u0026`, "the stored body is escaped")
		assert.Equal(t, key, testCapKey(t, "rule_a", string(stored)))
		reordered := `{"max_iterations":3,"subject":"alerts.a\u0026b","properties":{"html":"\u003cb\u003e","weight":1.50},"type":"publish"}`
		assert.Equal(t, key, testCapKey(t, "rule_a", reordered))
	})

	t.Run("an edit changes the key", func(t *testing.T) {
		key := testCapKey(t, "rule_a", body)
		for name, edited := range map[string]string{
			"max_iterations":    `{"type":"publish","subject":"alerts.a&b","properties":{"weight":1.50,"html":"<b>"},"max_iterations":5}`,
			"number as written": `{"type":"publish","subject":"alerts.a&b","properties":{"weight":1.5,"html":"<b>"},"max_iterations":3}`,
			"a when guard":      `{"type":"publish","subject":"alerts.a&b","properties":{"weight":1.50,"html":"<b>"},"max_iterations":3,"when":[{"field":"x","operator":"eq","value":1}]}`,
			"subject":           `{"type":"publish","subject":"alerts.c","properties":{"weight":1.50,"html":"<b>"},"max_iterations":3}`,
			"type":              `{"type":"update_kv","subject":"alerts.a&b","properties":{"weight":1.50,"html":"<b>"},"max_iterations":3}`,
		} {
			assert.NotEqual(t, key, testCapKey(t, "rule_a", edited), name)
		}
		assert.NotEqual(t, key, testCapKey(t, "rule_b", body), "another rule")
	})

	t.Run("an id is the key", func(t *testing.T) {
		assert.Equal(t, "my-key", testCapKey(t, "rule_a",
			`{"type":"publish","subject":"x","id":"my-key","max_iterations":5}`))
		assert.Equal(t, "my-key", testCapKey(t, "rule_b",
			`{"type":"update_kv","id":"my-key"}`))
	})
}
