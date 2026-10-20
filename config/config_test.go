package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/types"
)

// Helper function to extract enabled flag from service config
func getServiceEnabled(serviceConfig types.ServiceConfig) bool {
	return serviceConfig.Enabled
}

// Test basic config structure
func TestConfig_Structure(t *testing.T) {
	cfg := &Config{
		Platform: PlatformConfig{
			Org:          "c360",
			ID:           "test-platform",
			Type:         "vessel",
			Region:       "gulf_mexico",
			Capabilities: []string{"radar", "ctd"},
		},
		NATS: NATSConfig{
			URLs: []string{"nats://a.invalid:14222"},
		},
	}

	assert.Equal(t, "test-platform", cfg.Platform.ID)
	assert.Equal(t, "vessel", cfg.Platform.Type)
	assert.Contains(t, cfg.Platform.Capabilities, "radar")
}

// Test loading config from JSON file
func TestLoader_LoadJSON(t *testing.T) {
	// Create test config file
	testConfig := `{
		"platform": {
			"org": "c360",
			"id": "rv_walton_smith",
			"type": "vessel",
			"region": "gulf_mexico",
			"capabilities": ["radar", "ctd", "deployment"]
		},
		"nats": {
			"urls": ["nats://a.invalid:14222", "nats://b.invalid:14222"]
		},
		"services": {
			"message-logger": {"enabled": true, "config": {}},
			"discovery": {"enabled": true}
		}
	}`

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	err := os.WriteFile(configFile, []byte(testConfig), 0644)
	require.NoError(t, err)

	// Load config
	loader := NewLoader()
	cfg, err := loader.LoadFile(configFile)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify loaded values
	assert.Equal(t, "rv_walton_smith", cfg.Platform.ID)
	assert.Equal(t, "vessel", cfg.Platform.Type)
	assert.Equal(t, "gulf_mexico", cfg.Platform.Region)
	assert.Len(t, cfg.Platform.Capabilities, 3)
	assert.Equal(t, []string{"nats://a.invalid:14222", "nats://b.invalid:14222"}, cfg.NATS.URLs)
	// Parse service config from types.ServiceConfigs
	msgLoggerEnabled := getServiceEnabled(cfg.Services["message-logger"])
	discoveryEnabled := getServiceEnabled(cfg.Services["discovery"])
	assert.True(t, msgLoggerEnabled)
	assert.True(t, discoveryEnabled)
}

// Test default values
func TestLoader_Defaults(t *testing.T) {
	// Minimal config with missing fields
	testConfig := `{
		"platform": {
			"org": "c360",
			"id": "test-platform",
			"type": "shore"
		}
	}`

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	err := os.WriteFile(configFile, []byte(testConfig), 0644)
	require.NoError(t, err)

	loader := NewLoader()
	cfg, err := loader.LoadFile(configFile)
	require.NoError(t, err)

	// Check defaults were applied. The default URL is read from the loader's
	// defaults rather than spelled here: a test file may not carry the dev
	// broker's fixed address (TestNoFixedAddressesInTests).
	defaultURLs := NewLoader().getDefaults().NATS.URLs
	require.NotEmpty(t, defaultURLs, "the loader must default at least one NATS URL")
	assert.Equal(t, "gulf_mexico", cfg.Platform.Region) // default region
	assert.Equal(t, defaultURLs, cfg.NATS.URLs)         // default URL
	// Parse service config from types.ServiceConfigs
	_, msgLoggerExists := cfg.Services["message-logger"]
	discoveryEnabled := getServiceEnabled(cfg.Services["discovery"])
	assert.False(t, msgLoggerExists)           // default-off by omission
	assert.False(t, discoveryEnabled)          // dormant by default
	assert.True(t, cfg.NATS.JetStream.Enabled) // default enabled
	// ObjectStore moved to components
	// assert.True(t, cfg.ObjectStore.Enabled) // default enabled
	// assert.Equal(t, "messages", cfg.ObjectStore.BucketName) // default bucket
}

// Test environment variable overrides. The NATS credential overrides the pin
// applied are refused until boot wires them (TestLoad_RefusesNATSCredentialEnv).
func TestLoader_EnvOverrides(t *testing.T) {
	t.Setenv("SEMENGINE_PLATFORM_ID", "env-platform")

	// Base config
	testConfig := `{
		"platform": {
			"org": "c360",
			"id": "json-platform",
			"type": "vessel"
		}
	}`

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	err := os.WriteFile(configFile, []byte(testConfig), 0644)
	require.NoError(t, err)

	loader := NewLoader()
	cfg, err := loader.LoadFile(configFile)
	require.NoError(t, err)

	// Env vars should override JSON
	assert.Equal(t, "env-platform", cfg.Platform.ID)

	// JSON value should remain when no env override
	assert.Equal(t, "vessel", cfg.Platform.Type)
}

// Test validation
func TestLoader_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		wantError string
	}{
		{
			name: "missing org",
			config: `{
				"platform": {
					"id": "platform1",
					"type": "vessel"
				},
				"object_store": {
					"bucket_name": "test"
				}
			}`,
			wantError: "platform.org is required",
		},
		{
			name: "missing platform ID",
			config: `{
				"platform": {
					"org": "c360",
					"type": "vessel"
				},
				"object_store": {
					"bucket_name": "test"
				}
			}`,
			wantError: "platform.id is required",
		},
		{
			name: "invalid component config - empty component name",
			config: `{
				"platform": {
					"org": "c360",
					"id": "test",
					"type": "vessel"
				},
				"components": {
					"test-component": {
						"type": "input",
						"name": "",
						"enabled": true
					}
				},
				"object_store": {
					"bucket_name": "test"
				}
			}`,
			wantError: "component factory name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "config.json")
			err := os.WriteFile(configFile, []byte(tt.config), 0644)
			require.NoError(t, err)

			// Loading does not validate; the caller does, as boot does.
			cfg, err := NewLoader().LoadFile(configFile)
			require.NoError(t, err)
			err = cfg.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

// Test components configuration
// TODO: Update this test for new ComponentConfigs structure
// The old structure with Components.Enabled is obsolete
/*
func TestConfig_Components(t *testing.T) {
	// OLD FORMAT - NO LONGER SUPPORTED
	// components.enabled array has been replaced with
	// components as a map[string]types.ComponentConfig
}
*/

// Test loading the example config
func TestLoader_ExampleConfig(t *testing.T) {
	// Load the example config from the current directory
	loader := NewLoader()
	cfg, err := loader.LoadFile("example_config.json")
	require.NoError(t, err)

	// The platform and services blocks
	assert.Equal(t, "c360", cfg.Platform.Org)
	assert.Equal(t, "mavlink-demo", cfg.Platform.ID)
	assert.Equal(t, "ground-station", cfg.Platform.Type)
	// Parse service config from types.ServiceConfigs
	msgLoggerEnabled := getServiceEnabled(cfg.Services["message-logger"])
	discoveryEnabled := getServiceEnabled(cfg.Services["discovery"])
	assert.True(t, msgLoggerEnabled)
	assert.True(t, discoveryEnabled)

	// One component: graph-ingest, the one factory SemEngine registers. That
	// it validates against graph-ingest's Register is proven in
	// composition's TestValidateExampleConfigHasNoErrorFindings.
	require.Len(t, cfg.Components, 1, "should have 1 component configured")
	ingest, exists := cfg.Components["graph-ingest"]
	require.True(t, exists, "should have graph-ingest component")
	assert.Equal(t, types.ComponentType("processor"), ingest.Type)
	assert.Equal(t, "graph-ingest", ingest.Name)
	assert.True(t, ingest.Enabled)

	require.NoError(t, cfg.Validate())
}

// writeAndLoadAuthorityPair loads a minimal configuration declaring the given
// authority pair through the production loader, then validates it. This is
// the DECLARATION boundary — the one place that knows the pair it is looking at
// has not yet had the ADR-104 entropy suffix minted onto it.
func writeAndLoadAuthorityPair(t *testing.T, org, id string) error {
	t.Helper()
	configFile := filepath.Join(t.TempDir(), "config.json")
	body := `{"platform": {"org": "` + org + `", "id": "` + id + `", "type": "vessel"}}`
	require.NoError(t, os.WriteFile(configFile, []byte(body), 0o600))
	cfg, err := NewLoader().LoadFile(configFile)
	if err != nil {
		return err
	}
	return cfg.Validate()
}

// TestConfigRejectsOversizedAuthorityPair pins the configuration-load bound on
// a DECLARED authority pair (ADR-102 amends ADR-076 d2, O-14): len(org)+len(id)
// may not exceed the budget derived from the longest fixed-suffix framework
// family — 170 bytes while the rule trigger family binds — LESS the seven bytes
// of the entropy suffix the framework mints onto platform.id (ADR-104), so 163
// bytes is the declarable pair. The error names the family, the budget, and the
// measured pair.
//
// The reserve lives HERE and nowhere else. An effective pair — minted, adopted,
// or running — is bounded at 170 by validateAuthorityPair, because the suffix
// is already on it; see TestEffectivePairIsBoundedWithoutTheDeclarationReserve.
func TestConfigRejectsOversizedAuthorityPair(t *testing.T) {
	org := strings.Repeat("o", 82)
	require.NoError(t, writeAndLoadAuthorityPair(t, org, strings.Repeat("p", 81)),
		"163 bytes is the declarable budget, not over it")

	err := writeAndLoadAuthorityPair(t, org, strings.Repeat("p", 82))
	require.Error(t, err, "164 bytes must not load")
	assert.Contains(t, err.Error(), "170")
	assert.Contains(t, err.Error(), "rule-trigger")
	assert.Contains(t, err.Error(), "164")
}

// TestConfigRejectsPairThatOnlyFitsUnsuffixed pins ADR-104 decision 5 and the
// third collision the revision round found: a pair that fits the 170-byte
// family budget while unsuffixed, but not once the framework mints seven bytes
// onto platform.id, must be refused at LOAD. Refusing it later is unrepairable
// — the suffixed identifier has by then been durably Created, and ADR-102 d7
// forbids rewriting a minted authority.
func TestConfigRejectsPairThatOnlyFitsUnsuffixed(t *testing.T) {
	budget := semtypes.MaxAuthorityPairBytes()
	org := strings.Repeat("o", 60)
	// Exactly the family budget unsuffixed; seven bytes over it once minted.
	id := strings.Repeat("p", budget-len(org))
	require.Equal(t, budget, len(org)+len(id))

	err := writeAndLoadAuthorityPair(t, org, id)
	require.Error(t, err, "a pair that only fits unsuffixed must not load")
	assert.Contains(t, err.Error(), "suffix",
		"the refusal must name the minted suffix as the reason the pair does not fit")
}

// TestEffectivePairIsBoundedWithoutTheDeclarationReserve pins the other half of
// the split the declaration reserve requires: an EFFECTIVE pair already carries
// whatever suffix it will ever carry, so bounding it at the declarable budget
// would reserve the same seven bytes twice and refuse every declaration in
// (156, 163] after it had already passed load.
//
// The pair here is what a 163-byte declaration becomes once minted: 170 bytes,
// exactly the family-table budget, and legal.
func TestEffectivePairIsBoundedWithoutTheDeclarationReserve(t *testing.T) {
	org := strings.Repeat("o", 60)
	declared := strings.Repeat("p", maxDeclarableAuthorityPairBytes()-len(org))
	effective := declared + "-a1b2c3"
	require.Equal(t, semtypes.MaxAuthorityPairBytes(), len(org)+len(effective))

	require.NoError(t, validateAuthorityPair(org, effective),
		"a minted pair at exactly the family-table budget is legal")
	require.Error(t, validateDeclaredAuthorityPair(org, effective),
		"the same pair DECLARED leaves no room for the suffix and must not load")
	require.NoError(t, validateDeclaredAuthorityPair(org, declared),
		"the declaration it was minted from must load")

	require.Error(t, validateAuthorityPair(org, effective+"x"),
		"one byte past the family-table budget is refused on the effective path too")
}

// TestConfigRejectsRemovedInstanceID pins O-2: `platform.instance_id` left
// identity; its presence fails load with guidance naming `platform.id`.
func TestConfigRejectsRemovedInstanceID(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")
	body := `{"platform": {"org": "c360", "id": "backend", "instance_id": "backend-001", "type": "vessel"}}`
	require.NoError(t, os.WriteFile(configFile, []byte(body), 0o600))
	_, err := NewLoader().LoadFile(configFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "instance_id")
	assert.Contains(t, err.Error(), "platform.id")
}
