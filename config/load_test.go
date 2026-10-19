package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
