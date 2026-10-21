package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadPath is one of the two production loaders, fed the same document. Every
// loading rule below holds for both: LoadFile and LoadFromBytes share one
// parse-merge-override path, and a rule that only one of them enforces is the
// defect these tables exist to catch.
type loadPath struct {
	name string
	load func(t *testing.T, document string) (*Config, error)
}

var loadPaths = []loadPath{
	{name: "LoadFile", load: func(t *testing.T, document string) (*Config, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "config.json")
		require.NoError(t, os.WriteFile(path, []byte(document), 0o600))
		return NewLoader().LoadFile(path)
	}},
	{name: "LoadFromBytes", load: func(t *testing.T, document string) (*Config, error) {
		t.Helper()
		return NewLoader().LoadFromBytes([]byte(document))
	}},
}

// validDocument is a document every loader accepts and Validate passes.
const validDocument = `{"platform":{"org":"acme","id":"p1","type":"vessel"}}`

// TestLoad_RefusesUnappliedNATSKeys pins D2 (Q1, #9 comment 6275744386): a
// NATS setting nothing in SemEngine applies yet is refused at load, naming the
// key, so an operator who sets one is never silently ignored (#74). A key is
// refused when present, whatever its value, and in any letter case.
func TestLoad_RefusesUnappliedNATSKeys(t *testing.T) {
	cases := []struct {
		key      string
		document string
	}{
		{"nats.username", `{"platform":{"org":"acme","id":"p1"},"nats":{"username":"u"}}`},
		{"nats.password", `{"platform":{"org":"acme","id":"p1"},"nats":{"password":"p"}}`},
		{"nats.token", `{"platform":{"org":"acme","id":"p1"},"nats":{"token":"x"}}`},
		{"nats.token", `{"platform":{"org":"acme","id":"p1"},"nats":{"token":""}}`},
		{"nats.tls", `{"platform":{"org":"acme","id":"p1"},"nats":{"tls":{"enabled":true}}}`},
		{"nats.max_reconnects", `{"platform":{"org":"acme","id":"p1"},"nats":{"max_reconnects":5}}`},
		{"nats.reconnect_wait", `{"platform":{"org":"acme","id":"p1"},"nats":{"reconnect_wait":"5s"}}`},
		{"nats.jetstream.domain", `{"platform":{"org":"acme","id":"p1"},"nats":{"jetstream":{"domain":"hub"}}}`},
		{"nats.jetstream.retention_policy",
			`{"platform":{"org":"acme","id":"p1"},"nats":{"jetstream":{"retention_policy":"limits"}}}`},
		{"nats.jetstream.replication_factor",
			`{"platform":{"org":"acme","id":"p1"},"nats":{"jetstream":{"replication_factor":3}}}`},
		// encoding/json matches field names case-insensitively, so a case
		// variant is the same key and is refused the same way.
		{"nats.tls", `{"platform":{"org":"acme","id":"p1"},"nats":{"TLS":{"enabled":true}}}`},
		{"nats.token", `{"platform":{"org":"acme","id":"p1"},"nats":{"Token":"x"}}`},
		{"nats.token", `{"platform":{"org":"acme","id":"p1"},"NATS":{"token":"x"}}`},
		{"nats.jetstream.domain", `{"platform":{"org":"acme","id":"p1"},"nats":{"JetStream":{"Domain":"hub"}}}`},
	}
	for _, path := range loadPaths {
		for _, tc := range cases {
			t.Run(path.name+"/"+tc.key, func(t *testing.T) {
				cfg, err := path.load(t, tc.document)
				require.Error(t, err, "%s must be refused until boot applies it", tc.key)
				assert.Nil(t, cfg, "a refused document returns no Config")
				assert.Contains(t, err.Error(), tc.key, "the refusal names the key")
				assert.Contains(t, err.Error(), "not applied", "the refusal says the key is not applied")
			})
		}
	}
}

// TestLoad_AcceptsAppliedNATSKeys is the control for the refusal above: the
// four NATS keys something reads (boot reads urls; VerifyJetStreamLimits reads
// the jetstream three) load, and their values arrive.
func TestLoad_AcceptsAppliedNATSKeys(t *testing.T) {
	const document = `{
		"platform": {"org": "acme", "id": "p1"},
		"nats": {
			"urls": ["nats://a.invalid:14222"],
			"jetstream": {"enabled": false, "max_memory": 1048576, "max_file_store": 2097152}
		}
	}`
	for _, path := range loadPaths {
		t.Run(path.name, func(t *testing.T) {
			cfg, err := path.load(t, document)
			require.NoError(t, err)
			assert.Equal(t, []string{"nats://a.invalid:14222"}, cfg.NATS.URLs)
			assert.False(t, cfg.NATS.JetStream.Enabled, "the document's false overrides the default true")
			assert.Equal(t, int64(1048576), cfg.NATS.JetStream.MaxMemory)
			assert.Equal(t, int64(2097152), cfg.NATS.JetStream.MaxFileStore)
		})
	}
}

// TestLoad_RefusesNATSCredentialEnv pins D2's environment half: the pin applied
// _NATS_USERNAME, _PASSWORD and _TOKEN; SemEngine applies none of them yet, so a
// set credential fails the load, naming the variable, instead of being dropped.
func TestLoad_RefusesNATSCredentialEnv(t *testing.T) {
	variables := []string{"SEMENGINE_NATS_USERNAME", "SEMENGINE_NATS_PASSWORD", "SEMENGINE_NATS_TOKEN"}
	for _, path := range loadPaths {
		for _, variable := range variables {
			t.Run(path.name+"/"+variable, func(t *testing.T) {
				t.Setenv(variable, "x")
				cfg, err := path.load(t, validDocument)
				require.Error(t, err, "%s set must fail the load", variable)
				assert.Nil(t, cfg)
				assert.Contains(t, err.Error(), variable, "the refusal names the variable")
			})
			t.Run(path.name+"/"+variable+" empty", func(t *testing.T) {
				t.Setenv(variable, "")
				_, err := path.load(t, validDocument)
				require.NoError(t, err, "an empty %s is not set and must not fail the load", variable)
			})
		}
	}
}

// TestLoad_EnvOverrideNames pins the SEMENGINE_ override names (#69 ruling 1,
// D4): each of the four overrides replaces the document's value.
func TestLoad_EnvOverrideNames(t *testing.T) {
	const document = `{
		"platform": {"org": "acme", "id": "file-id", "type": "file-type", "region": "file-region"},
		"nats": {"urls": ["nats://file.invalid:14222"]}
	}`
	for _, path := range loadPaths {
		t.Run(path.name, func(t *testing.T) {
			t.Setenv("SEMENGINE_PLATFORM_ID", "env-id")
			t.Setenv("SEMENGINE_PLATFORM_TYPE", "env-type")
			t.Setenv("SEMENGINE_PLATFORM_REGION", "env-region")
			t.Setenv("SEMENGINE_NATS_URLS", "nats://a.invalid:14222,nats://b.invalid:14222")

			cfg, err := path.load(t, document)
			require.NoError(t, err)
			assert.Equal(t, "env-id", cfg.Platform.ID)
			assert.Equal(t, "env-type", cfg.Platform.Type)
			assert.Equal(t, "env-region", cfg.Platform.Region)
			assert.Equal(t, []string{"nats://a.invalid:14222", "nats://b.invalid:14222"}, cfg.NATS.URLs)
			assert.Equal(t, "acme", cfg.Platform.Org, "a key with no override keeps the document's value")
		})
	}
}

// TestLoad_RefusesMalformedDocument pins D3 (Q2, #9 comment 6275744386): a
// document whose value does not fit its field fails the load with the decode
// error. The pin returned the defaults instead, so a URL list written as a
// string loaded as org="" and the default URL, with no error.
func TestLoad_RefusesMalformedDocument(t *testing.T) {
	cases := []struct {
		name     string
		document string
		want     string // text of the decode failure the error must carry
	}{
		{
			name:     "URL list written as a string",
			document: `{"platform":{"org":"acme","id":"p1"},"nats":{"urls":"nats://a.invalid:14222"}}`,
			want:     "urls",
		},
		{
			name:     "service entry with an unknown field",
			document: `{"platform":{"org":"acme","id":"p1"},"services":{"metrics":{"name":"m","enabled":true}}}`,
			want:     `unknown field "name"`,
		},
	}
	for _, path := range loadPaths {
		for _, tc := range cases {
			t.Run(path.name+"/"+tc.name, func(t *testing.T) {
				cfg, err := path.load(t, tc.document)
				require.Error(t, err, "a malformed document must not load as the defaults")
				assert.Nil(t, cfg, "a refused document returns no Config")
				assert.Contains(t, err.Error(), tc.want, "the error carries the decode failure")
			})
		}
	}
}

// TestLoadFromBytes_RoundTrip pins LoadFromBytes as framework API (semboids
// loads its configuration through it): the values a document writes come back
// on the Config, and a loaded Config marshalled and loaded again is unchanged.
func TestLoadFromBytes_RoundTrip(t *testing.T) {
	const document = `{
		"version": "1.2.3",
		"platform": {"org": "acme", "id": "p1", "type": "vessel", "region": "north", "capabilities": ["radar"]},
		"nats": {"urls": ["nats://a.invalid:14222"], "jetstream": {"enabled": true, "max_memory": 1024}},
		"services": {"metrics": {"enabled": true, "config": {"port": 9090}}},
		"components": {"udp-input": {"type": "input", "name": "udp", "enabled": true, "config": {"port": 14550}}}
	}`
	loaded, err := NewLoader().LoadFromBytes([]byte(document))
	require.NoError(t, err)
	require.NoError(t, loaded.Validate())

	assert.Equal(t, "1.2.3", loaded.Version)
	assert.Equal(t, "acme", loaded.Platform.Org)
	assert.Equal(t, "p1", loaded.Platform.ID)
	assert.Equal(t, "vessel", loaded.Platform.Type)
	assert.Equal(t, "north", loaded.Platform.Region)
	assert.Equal(t, []string{"radar"}, loaded.Platform.Capabilities)
	assert.Equal(t, []string{"nats://a.invalid:14222"}, loaded.NATS.URLs)
	assert.True(t, loaded.NATS.JetStream.Enabled)
	assert.Equal(t, int64(1024), loaded.NATS.JetStream.MaxMemory)
	require.Contains(t, loaded.Services, "metrics")
	assert.True(t, loaded.Services["metrics"].Enabled)
	assert.JSONEq(t, `{"port": 9090}`, string(loaded.Services["metrics"].Config))
	require.Contains(t, loaded.Components, "udp-input")
	assert.Equal(t, "udp", loaded.Components["udp-input"].Name)
	assert.JSONEq(t, `{"port": 14550}`, string(loaded.Components["udp-input"].Config))

	data, err := json.Marshal(loaded)
	require.NoError(t, err)
	reloaded, err := NewLoader().LoadFromBytes(data)
	require.NoError(t, err, "a loaded Config, marshalled, must load again")
	assert.Equal(t, loaded, reloaded)
}

// TestLoad_RefusesRemovedInstanceIDInAnyCase pins the ADR-102 refusal against
// letter case: encoding/json matches field names case-insensitively, so a case
// variant of platform or instance_id is the same withdrawn field, and it must
// fail the load rather than be dropped.
func TestLoad_RefusesRemovedInstanceIDInAnyCase(t *testing.T) {
	documents := map[string]string{
		"field in capitals":  `{"platform":{"org":"acme","id":"p1","INSTANCE_ID":"x"}}`,
		"parent capitalised": `{"Platform":{"org":"acme","id":"p1","instance_id":"x"}}`,
		"second platform key": `{"platform":{"org":"acme","id":"p1"},` +
			`"PLATFORM":{"Instance_Id":"x"}}`,
	}
	for _, path := range loadPaths {
		for name, document := range documents {
			t.Run(path.name+"/"+name, func(t *testing.T) {
				cfg, err := path.load(t, document)
				require.Error(t, err, "a case variant of platform.instance_id must not load")
				assert.Nil(t, cfg)
				assert.Contains(t, err.Error(), "instance_id")
				assert.Contains(t, err.Error(), "platform.id")
			})
		}
	}
}

// TestLoad_PreservesNumbers pins that the merge keeps each JSON number as
// written. Component and service payloads are carried as raw JSON for their
// owners to decode, so an integer above 2^53 must arrive exactly, not rounded
// through a float64; and an int64 field takes its full range.
func TestLoad_PreservesNumbers(t *testing.T) {
	const document = `{
		"platform": {"org": "acme", "id": "p1"},
		"nats": {"jetstream": {"max_memory": 9223372036854775807}},
		"services": {"metrics": {"enabled": true, "config": {"offset": 9007199254740993}}},
		"components": {"x": {"type": "input", "name": "udp", "enabled": true, "config": {"offset": 9007199254740993}}}
	}`
	for _, path := range loadPaths {
		t.Run(path.name, func(t *testing.T) {
			cfg, err := path.load(t, document)
			require.NoError(t, err)
			assert.Equal(t, `{"offset":9007199254740993}`, string(cfg.Components["x"].Config))
			assert.Equal(t, `{"offset":9007199254740993}`, string(cfg.Services["metrics"].Config))
			assert.Equal(t, int64(9223372036854775807), cfg.NATS.JetStream.MaxMemory)
		})
	}
}

// TestLoad_RefusesNumberThatDoesNotFitItsField pins D3 for numbers: a value
// json.Unmarshal into the field refuses must not load, even where a float64
// would round it to one that fits.
func TestLoad_RefusesNumberThatDoesNotFitItsField(t *testing.T) {
	documents := map[string]string{
		"nonintegral just above 1": `{"platform":{"org":"acme","id":"p1"},` +
			`"nats":{"jetstream":{"max_memory":1.0000000000000001}}}`,
		"nonintegral": `{"platform":{"org":"acme","id":"p1"},"nats":{"jetstream":{"max_memory":1.5}}}`,
	}
	for _, path := range loadPaths {
		for name, document := range documents {
			t.Run(path.name+"/"+name, func(t *testing.T) {
				cfg, err := path.load(t, document)
				require.Error(t, err, "a nonintegral value in an integer field must not load")
				assert.Nil(t, cfg)
				assert.Contains(t, err.Error(), "max_memory")
			})
		}
	}
}

// TestLoad_RefusesTrailingData pins that a document is exactly one JSON value.
func TestLoad_RefusesTrailingData(t *testing.T) {
	documents := map[string]string{
		"second document": validDocument + ` {}`,
		"trailing text":   validDocument + ` x`,
	}
	for _, path := range loadPaths {
		for name, document := range documents {
			t.Run(path.name+"/"+name, func(t *testing.T) {
				cfg, err := path.load(t, document)
				require.Error(t, err, "data after the document must not load")
				assert.Nil(t, cfg)
			})
		}
	}
}

// TestLoad_RefusesNullDocument pins that a document is a JSON object: `null`
// decodes into a nil map without error, and must not load as the defaults.
func TestLoad_RefusesNullDocument(t *testing.T) {
	for _, path := range loadPaths {
		t.Run(path.name, func(t *testing.T) {
			cfg, err := path.load(t, `null`)
			require.Error(t, err, "a null document must not load as the defaults")
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), "not a JSON object")
		})
	}
}

// TestLoad_RefusesKeyInAnotherLetterCase pins the #325 ruling (comment
// 6294481629): a key that differs from its declared name only in letter case is
// refused, naming the path as written and as declared. encoding/json would match
// it to the field anyway, and the merge with the defaults would then drop or
// apply it depending on what the defaults hold.
func TestLoad_RefusesKeyInAnotherLetterCase(t *testing.T) {
	const pair = `"platform":{"org":"acme","id":"p1"}`
	cases := []struct {
		written, declared, document string
	}{
		{"NATS", "nats", `{` + pair + `,"NATS":{"urls":["nats://a.invalid:14222"]}}`},
		{"nats.URLs", "nats.urls", `{` + pair + `,"nats":{"URLs":["nats://a.invalid:14222"]}}`},
		{"nats.JetStream", "nats.jetstream", `{` + pair + `,"nats":{"JetStream":{"enabled":false}}}`},
		{"Components", "components",
			`{` + pair + `,"Components":{"x":{"type":"input","name":"udp","enabled":true,"config":{}}}}`},
		{"Streams", "streams", `{` + pair + `,"Streams":{"S":{"subjects":["s.>"]}}}`},
		{"services.heartbeat.Enabled", "services.heartbeat.enabled",
			`{` + pair + `,"services":{"heartbeat":{"Enabled":false}}}`},
		{"components.x.Config", "components.x.config",
			`{` + pair + `,"components":{"x":{"type":"input","name":"udp","enabled":true,"Config":{}}}}`},
		{"NATS", "nats", `{` + pair + `,"nats":{"urls":["nats://a.invalid:1"]},"NATS":{"urls":["nats://b.invalid:2"]}}`},
		{"NATS", "nats", `{` + pair + `,"NATS":null}`},
	}
	for _, path := range loadPaths {
		for _, tc := range cases {
			t.Run(path.name+"/"+tc.written, func(t *testing.T) {
				cfg, err := path.load(t, tc.document)
				require.Error(t, err, "%s must be refused, not dropped or applied", tc.written)
				assert.Nil(t, cfg, "a refused document returns no Config")
				assert.Contains(t, err.Error(), "config key "+tc.written+" must be written "+tc.declared)
			})
		}
	}
}

// TestLoad_AcceptsNamesOutsideTheLetterCaseRule is the control for the refusal
// above: a key matching no declared name stays accepted (part 1's semteams
// "tier"), names a deployment chooses are not declared keys, and a component's
// config payload belongs to its owner and arrives unchanged.
func TestLoad_AcceptsNamesOutsideTheLetterCaseRule(t *testing.T) {
	const pair = `"platform":{"org":"acme","id":"p1"}`
	documents := map[string]string{
		"unknown top-level key": `{` + pair + `,"tier":"edge"}`,
		"unknown nats key":      `{` + pair + `,"nats":{"extra":1}}`,
		"chosen names": `{` + pair + `,
			"components":{"Config":{"type":"input","name":"udp","enabled":true,"config":{"Port":8080}}},
			"services":{"Enabled":{"enabled":true,"config":{}}},
			"streams":{"Subjects":{"subjects":["s.>"]}}}`,
		"owners' payloads and other chosen names": `{` + pair + `,
			"services":{"custom":{"enabled":true,"config":{"Interval":"5s"}}},
			"model_registry":{
				"capabilities":{"Timeout":{"preferred":["Model"]}},
				"endpoints":{"Model":{"provider":"openai","model":"m","max_tokens":1,"options":{"Temperature":1}}},
				"defaults":{"model":"Model"}},
			"stream_migration_overrides":{"Owner":{"owner":"ops","expires":"2099-01-01T00:00:00Z"}},
			"archival_streams":{"Reason":{"owner":"ops","reason":"audit"}}}`,
	}
	for _, path := range loadPaths {
		for name, document := range documents {
			t.Run(path.name+"/"+name, func(t *testing.T) {
				cfg, err := path.load(t, document)
				require.NoError(t, err)
				require.NotNil(t, cfg)
				if name == "owners' payloads and other chosen names" {
					assert.Equal(t, `{"Interval":"5s"}`, string(cfg.Services["custom"].Config),
						"the service payload is carried byte for byte")
					require.NotNil(t, cfg.ModelRegistry)
					require.Contains(t, cfg.ModelRegistry.Endpoints, "Model")
					assert.Equal(t, map[string]any{"Temperature": float64(1)},
						cfg.ModelRegistry.Endpoints["Model"].Options)
					assert.Contains(t, cfg.ModelRegistry.Capabilities, "Timeout")
					assert.Contains(t, cfg.StreamMigrationOverrides, "Owner")
					assert.Contains(t, cfg.ArchivalStreams, "Reason")
					return
				}
				if name != "chosen names" {
					return
				}
				require.Contains(t, cfg.Components, "Config")
				assert.Equal(t, `{"Port":8080}`, string(cfg.Components["Config"].Config),
					"the payload is carried byte for byte")
				assert.Contains(t, cfg.Services, "Enabled")
				assert.Contains(t, cfg.Streams, "Subjects")
			})
		}
	}
}

// placeholder fills every map level of the coverage document. It folds to no
// declared key, so the walker treats it as a name the deployment chose.
const placeholder = "PH"

// TestLoad_RefusesEveryDeclaredKeyInCapitals is the coverage oracle for the
// letter-case rule: every key path json.Marshal writes for a Config with every
// field populated is refused when its last key is upper-cased. The paths come
// from encoding/json's own naming, never from the walker's, so a naming bug in
// the walker cannot hide itself; the filler is reflection, so a new field is
// covered with no edit.
func TestLoad_RefusesEveryDeclaredKeyInCapitals(t *testing.T) {
	var cfg Config
	fillForCoverage(reflect.ValueOf(&cfg).Elem())
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))

	var paths [][]string
	collectKeyPaths(doc, nil, &paths)
	written := make([]string, 0, len(paths))
	for _, p := range paths {
		written = append(written, strings.Join(p, "."))
	}
	// The filler reached a pointer's struct, a map of pointers and the deepest
	// security struct; without these the oracle would be partial.
	for _, deep := range []string{
		"model_registry.defaults.capability",
		"model_registry.capabilities.PH.timeout",
		"security.tls.client.mtls.key_file",
		"services.PH.config",
	} {
		require.Contains(t, written, deep)
	}
	t.Logf("%d declared key paths checked", len(paths))

	for _, path := range loadPaths {
		for _, p := range paths {
			last := len(p) - 1
			upper := slices.Clone(p)
			upper[last] = strings.ToUpper(p[last])
			value := valueAt(doc, p)
			document := map[string]any{upper[last]: value}
			for i := last - 1; i >= 0; i-- {
				document = map[string]any{p[i]: document}
			}
			body, err := json.Marshal(document)
			require.NoError(t, err)
			wantErr := "config key " + strings.Join(upper, ".") + " must be written " + strings.Join(p, ".")
			t.Run(path.name+"/"+strings.Join(p, "."), func(t *testing.T) {
				cfg, err := path.load(t, string(body))
				require.Error(t, err, "%s must be refused", strings.Join(upper, "."))
				assert.Nil(t, cfg)
				assert.Contains(t, err.Error(), wantErr)
			})
		}
	}
}

// fillForCoverage gives every field reachable from v a non-zero value, so
// json.Marshal omits nothing: a pointer gets a new value, a map one entry under
// the placeholder, and each leaf a fixed value.
func fillForCoverage(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillForCoverage(v.Elem())
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Field(i).CanSet() {
				fillForCoverage(v.Field(i))
			}
		}
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		elem := reflect.New(v.Type().Elem()).Elem()
		if elem.Kind() == reflect.Interface {
			elem.Set(reflect.ValueOf(1))
		} else {
			fillForCoverage(elem)
		}
		m.SetMapIndex(reflect.ValueOf(placeholder).Convert(v.Type().Key()), elem)
		v.Set(m)
	case reflect.Slice:
		if v.Type() == reflect.TypeFor[json.RawMessage]() {
			v.Set(reflect.ValueOf(json.RawMessage("{}")))
			return
		}
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillForCoverage(s.Index(0))
		v.Set(s)
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	default:
		panic("fillForCoverage: no fill for " + v.Type().String())
	}
}

// collectKeyPaths appends the path to every key in doc whose last key is not
// the placeholder, descending into every object.
func collectKeyPaths(doc map[string]any, prefix []string, paths *[][]string) {
	for key, value := range doc {
		path := append(slices.Clone(prefix), key)
		if key != placeholder {
			*paths = append(*paths, path)
		}
		if obj, ok := value.(map[string]any); ok {
			collectKeyPaths(obj, path, paths)
		}
	}
}

func valueAt(doc map[string]any, path []string) any {
	var value any = doc
	for _, key := range path {
		value = value.(map[string]any)[key]
	}
	return value
}
