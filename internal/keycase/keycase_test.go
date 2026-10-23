package keycase_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/internal/keycase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type caseInfo struct {
	Title string `json:"title"`
}

type caseEntry struct {
	Enabled bool `json:"enabled"`
}

type caseTarget struct {
	Port    int                  `json:"port"`
	Info    caseInfo             `json:"server_info"`
	Pointer *caseInfo            `json:"pointer"`
	Entries map[string]caseEntry `json:"entries"`
	Plain   string               // no tag: the field name is the key
}

// TestCheckRefusesEachShapeItWalks has one case per shape the walk visits (design
// § Generated checks: each key is judged alone against the names at its own level,
// so examples suffice). Each refusal sits beside an exact match at the same level,
// which must pass.
func TestCheckRefusesEachShapeItWalks(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		wantErr string
	}{
		{"exact keys at every level", `{"port":1,"server_info":{"title":"t"},"pointer":{"title":"p"},` +
			`"entries":{"Any_Name":{"enabled":true}},"Plain":"x"}`, ""},
		{"top-level field", `{"Port":9091}`, "config key Port must be written port"},
		{"untagged field", `{"plain":"x"}`, "config key plain must be written Plain"},
		{"nested struct", `{"server_info":{"Title":"t"}}`, "config key server_info.Title must be written server_info.title"},
		{"pointer to struct", `{"pointer":{"TITLE":"t"}}`, "config key pointer.TITLE must be written pointer.title"},
		{"map value fields", `{"entries":{"a":{"Enabled":true}}}`, "config key entries.a.Enabled must be written entries.a.enabled"},
		{"map key left alone", `{"entries":{"PORT":{"enabled":true}}}`, ""},
		{"unknown key left to the decoder", `{"porty":1}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc any
			require.NoError(t, json.Unmarshal([]byte(tc.doc), &doc))
			err := keycase.Check(doc, reflect.TypeFor[caseTarget](), "")
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.wantErr, err.Error())
		})
	}
}

// TestCheckPrefixesThePathItIsGiven: a caller checking a sub-document names it.
func TestCheckPrefixesThePathItIsGiven(t *testing.T) {
	doc := map[string]any{"Port": 1.0}
	err := keycase.Check(doc, reflect.TypeFor[*caseTarget](), "services.metrics")
	require.Error(t, err)
	assert.Equal(t, "config key services.metrics.Port must be written services.metrics.port", err.Error())
}

// TestCheckReportsTheSameFirstKeyOnEveryRun: keys are visited in sorted order, so
// a document with several variants names the same one every time, though Go's map
// order differs from run to run.
func TestCheckReportsTheSameFirstKeyOnEveryRun(t *testing.T) {
	doc := map[string]any{"Port": 1.0, "Server_Info": map[string]any{}, "Pointer": map[string]any{}, "Entries": map[string]any{}}
	for range 50 {
		err := keycase.Check(doc, reflect.TypeFor[caseTarget](), "")
		require.Error(t, err)
		assert.Equal(t, "config key Entries must be written entries", err.Error())
	}
}
