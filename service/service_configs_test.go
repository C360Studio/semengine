package service

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/c360studio/semengine/types"
)

// TestResolveServiceConfigsIsPureAndCanonical is moved unchanged from the pin's
// service_composition_test.go:233-273 (design D6).
func TestResolveServiceConfigsIsPureAndCanonical(t *testing.T) {
	original := types.ServiceConfigs{
		"optional": {
			Enabled: true,
			Config:  json.RawMessage(`{ "b": 2, "a": { "y": 2, "x": 1 } }`),
		},
		"component-manager": {
			Enabled: false,
			Config:  json.RawMessage(`{}`),
		},
	}
	wantOriginal := cloneServiceConfigs(t, original)

	resolved, err := ResolveServiceConfigs(original)
	if err != nil {
		t.Fatalf("ResolveServiceConfigs: %v", err)
	}
	if !reflect.DeepEqual(original, wantOriginal) {
		t.Fatalf("resolver mutated input: got %#v want %#v", original, wantOriginal)
	}
	if got := string(resolved["optional"].Config); got != `{"a":{"x":1,"y":2},"b":2}` {
		t.Fatalf("canonical config = %s", got)
	}
	if resolved["component-manager"].Enabled {
		t.Fatal("explicit component-manager false was not preserved")
	}
	if got, ok := resolved["service-manager"]; !ok || !got.Enabled {
		t.Fatalf("service-manager was not materialized enabled: %#v", got)
	}
	if got, ok := resolved["metrics"]; !ok || !got.Enabled {
		t.Fatalf("optional metrics default was not retained: %#v", got)
	}
	if _, ok := resolved["message-logger"]; ok {
		t.Fatal("message-logger must not be injected")
	}

	resolved["optional"] = types.ServiceConfig{Enabled: false}
	if !original["optional"].Enabled {
		t.Fatal("resolved map aliases input map")
	}
}

// TestPendingServiceChangesClassifications is moved unchanged from the pin's
// service_composition_test.go:351-392 (design D6).
func TestPendingServiceChangesClassifications(t *testing.T) {
	boot := types.ServiceConfigs{
		"add-disabled":      {Enabled: false, Config: json.RawMessage(`{"a":1}`)},
		"enable":            {Enabled: false, Config: json.RawMessage(`{"a":1}`)},
		"disable":           {Enabled: true, Config: json.RawMessage(`{"a":1}`)},
		"remove":            {Enabled: true, Config: json.RawMessage(`{"a":1}`)},
		"reconfigure":       {Enabled: true, Config: json.RawMessage(`{"a":1}`)},
		"disabled-churn":    {Enabled: false, Config: json.RawMessage(`{"a":1}`)},
		"activation-wins":   {Enabled: false, Config: json.RawMessage(`{"a":1}`)},
		"canonical-no-diff": {Enabled: true, Config: json.RawMessage(`{"a":1,"b":2}`)},
	}
	desired := types.ServiceConfigs{
		"add":               {Enabled: true, Config: json.RawMessage(`{"a":1}`)},
		"enable":            {Enabled: true, Config: json.RawMessage(`{"a":1}`)},
		"disable":           {Enabled: false, Config: json.RawMessage(`{"a":2}`)},
		"reconfigure":       {Enabled: true, Config: json.RawMessage(`{"a":2}`)},
		"disabled-churn":    {Enabled: false, Config: json.RawMessage(`{"a":2}`)},
		"activation-wins":   {Enabled: true, Config: json.RawMessage(`{"a":2}`)},
		"canonical-no-diff": {Enabled: true, Config: json.RawMessage(`{ "b": 2, "a": 1 }`)},
	}

	resolvedBoot, err := resolveServiceConfigs(boot, false)
	if err != nil {
		t.Fatal(err)
	}
	resolvedDesired, err := resolveServiceConfigs(desired, false)
	if err != nil {
		t.Fatal(err)
	}
	got := pendingServiceChanges(resolvedBoot, resolvedDesired)
	want := []PendingServiceChange{
		{Name: "activation-wins", Change: "enable"},
		{Name: "add", Change: "add"},
		{Name: "disable", Change: "disable"},
		{Name: "enable", Change: "enable"},
		{Name: "reconfigure", Change: "reconfigure"},
		{Name: "remove", Change: "remove"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pending changes = %#v, want %#v", got, want)
	}
}

// cloneServiceConfigs is moved with the two tests above from the pin's
// service_composition_test.go:604-612.
func cloneServiceConfigs(t *testing.T, configs types.ServiceConfigs) types.ServiceConfigs {
	t.Helper()
	clone := make(types.ServiceConfigs, len(configs))
	for name, serviceConfig := range configs {
		serviceConfig.Config = bytes.Clone(serviceConfig.Config)
		clone[name] = serviceConfig
	}
	return clone
}

// TestDecodeStrictServiceJSON: the strict decoder fills known fields, refuses an unknown key and a
// trailing value, and leaves the target untouched for empty input (design D6).
func TestDecodeStrictServiceJSON(t *testing.T) {
	type target struct {
		Port int    `json:"port"`
		Path string `json:"path"`
	}
	for _, tt := range []struct {
		name    string
		raw     string
		want    target
		wantErr string
	}{
		{name: "known keys", raw: `{"port":9,"path":"/m"}`, want: target{Port: 9, Path: "/m"}},
		{name: "unknown key", raw: `{"port":9,"bogus":1}`, wantErr: `unknown field "bogus"`},
		{name: "trailing value", raw: `{"port":9} {"port":10}`, wantErr: "multiple values"},
		{name: "trailing garbage", raw: `{"port":9} x`, wantErr: "invalid character"},
		{name: "empty", raw: ``, want: target{Port: 1, Path: "kept"}},
		{name: "whitespace", raw: " \n\t", want: target{Port: 1, Path: "kept"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := target{Port: 1, Path: "kept"}
			err := decodeStrictServiceJSON(json.RawMessage(tt.raw), &got)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestCanonicalServiceJSONIsIdempotent checks canonicalServiceJSON's normalization law on generated
// nested JSON objects (design, Generated checks): canonicalizing twice equals canonicalizing once,
// and the canonical text decodes, with UseNumber, to a value deep-equal to the input's decoded
// value, an oracle that does not call the code under test. Each generated input is written with
// its own key order and whitespace, so the input is rarely canonical already. Every case reaches
// both assertions: the generator writes only valid JSON, and an error from the code fails the case.
func TestCanonicalServiceJSONIsIdempotent(t *testing.T) {
	t.Run("examples", func(t *testing.T) {
		for raw, want := range map[string]string{
			``:                                 `{}`,
			" \n":                              `{}`,
			`{ "b": 1.50, "a": [true, null] }`: `{"a":[true,null],"b":1.50}`,
		} {
			once, err := canonicalServiceJSON(json.RawMessage(raw))
			require.NoError(t, err)
			require.Equal(t, want, string(once), "input %q", raw)
			twice, err := canonicalServiceJSON(once)
			require.NoError(t, err)
			require.Equal(t, string(once), string(twice))
		}
	})

	rapid.Check(t, func(t *rapid.T) {
		raw := writeJSONObject(t, 0)

		once, err := canonicalServiceJSON(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("canonicalServiceJSON(%s): %v", raw, err)
		}
		twice, err := canonicalServiceJSON(once)
		if err != nil {
			t.Fatalf("canonicalServiceJSON(%s): %v", once, err)
		}
		if !bytes.Equal(once, twice) {
			t.Fatalf("not idempotent: once %s, twice %s (input %s)", once, twice, raw)
		}
		if in, out := decodeWithNumbers(t, raw), decodeWithNumbers(t, string(once)); !reflect.DeepEqual(in, out) {
			t.Fatalf("canonical text %s decodes to %#v, input %s to %#v", once, out, raw, in)
		}
	})
}

// decodeWithNumbers decodes text as the oracle does: encoding/json with UseNumber.
func decodeWithNumbers(t *rapid.T, text string) any {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("oracle decode %s: %v", text, err)
	}
	return value
}

// maxJSONDepth bounds nesting so a generated document stays small.
const maxJSONDepth = 3

// space is the whitespace a generated document puts between tokens.
func space(t *rapid.T) string {
	return rapid.SampledFrom([]string{"", " ", "\n", "\t ", "  "}).Draw(t, "space")
}

// writeJSONObject writes an object whose keys come in generated order, duplicates allowed.
func writeJSONObject(t *rapid.T, depth int) string {
	n := rapid.IntRange(0, 4).Draw(t, "members")
	var b strings.Builder
	b.WriteString("{" + space(t))
	for i := range n {
		if i > 0 {
			b.WriteString("," + space(t))
		}
		key, _ := json.Marshal(rapid.String().Draw(t, "key"))
		b.WriteString(string(key) + space(t) + ":" + space(t) + writeJSONValue(t, depth+1) + space(t))
	}
	b.WriteString("}")
	return b.String()
}

// writeJSONValue writes a string, number, boolean, null, array or object.
func writeJSONValue(t *rapid.T, depth int) string {
	kinds := []string{"string", "number", "bool", "null"}
	if depth < maxJSONDepth {
		kinds = append(kinds, "array", "object")
	}
	switch rapid.SampledFrom(kinds).Draw(t, "kind") {
	case "string":
		text, _ := json.Marshal(rapid.String().Draw(t, "string"))
		return string(text)
	case "number":
		return rapid.SampledFrom([]string{"0", "-0", "1", "-17", "1.50", "0.001", "1e3", "-2.5E-4", "12345678901234567890"}).
			Draw(t, "number")
	case "bool":
		return rapid.SampledFrom([]string{"true", "false"}).Draw(t, "bool")
	case "null":
		return "null"
	case "array":
		n := rapid.IntRange(0, 3).Draw(t, "elements")
		parts := make([]string, n)
		for i := range parts {
			parts[i] = space(t) + writeJSONValue(t, depth+1) + space(t)
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		return writeJSONObject(t, depth)
	}
}
