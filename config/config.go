package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/pkg/platform"
	"github.com/c360studio/semengine/pkg/security"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/types"
)

// PlatformConfig is an alias for platform.Config.
//
// PlatformConfig was promoted to its own leaf package so message/ (and
// any future consumer of platform identity) can reference it without
// importing all of config. Keeping the alias preserves config.PlatformConfig
// usage at every call site that already had it. New code should prefer
// platform.Config directly. Removing the alias would be a
// mechanical rename across ~10 files; defer until there's a reason.
type PlatformConfig = platform.Config

// ComponentConfigs holds component instance configurations.
// The map key is the instance name (e.g., "udp-sensor-main").
// Components are only created if both:
// 1. Their factory has been registered via init()
// 2. They have an entry in this config map with enabled=true
type ComponentConfigs map[string]types.ComponentConfig

// Config represents the complete application configuration
// Simplified to 6 fields: Version (semver), Platform (identity), Security (TLS), NATS (connection), Services, Components
type Config struct {
	Version       string               `json:"version"` // Semantic version (e.g., "1.0.0") for KV sync control
	Platform      PlatformConfig       `json:"platform"`
	Security      security.Config      `json:"security,omitempty"` // Platform-wide security configuration
	NATS          NATSConfig           `json:"nats"`
	Services      types.ServiceConfigs `json:"services"`                 // Map of service configs
	Components    ComponentConfigs     `json:"components"`               // Map of component instance configs
	Streams       StreamConfigs        `json:"streams,omitempty"`        // Explicit JetStream stream definitions
	ModelRegistry *model.Registry      `json:"model_registry,omitempty"` // Unified model endpoint registry

	// StreamMigrationOverrides admits named ordinary streams that predate the
	// bounds contract, each for a declared period. Readiness reports every
	// active override and FAILS once one expires; an override with no expiry is
	// rejected at validation, so a bridge cannot become permanent.
	StreamMigrationOverrides StreamMigrationOverrides `json:"stream_migration_overrides,omitempty"`

	// ArchivalStreams declares ordinary streams whose contract is permanence.
	// Structurally separate from StreamMigrationOverrides: an archive that could
	// only be expressed as a renewed override would train operators to renew
	// without reading, which is what makes genuinely time-limited overrides
	// invisible.
	ArchivalStreams ArchivalStreams `json:"archival_streams,omitempty"`
}

// SafeConfig provides thread-safe access to configuration
type SafeConfig struct {
	mu     sync.RWMutex
	config *Config
}

// NewSafeConfig creates a new thread-safe config wrapper
func NewSafeConfig(cfg *Config) *SafeConfig {
	if cfg == nil {
		cfg = &Config{}
	}
	return &SafeConfig{
		config: cfg,
	}
}

// Get returns a deep copy of the current configuration
func (sc *SafeConfig) Get() *Config {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.config.Clone()
}

// Update atomically updates the configuration after validation
func (sc *SafeConfig) Update(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Validate before updating
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.config = cfg
	return nil
}

// Mutate atomically applies a read-modify-write to the configuration. It holds
// the write lock across the WHOLE clone → mutate → validate → swap sequence, so
// two concurrent mutations cannot lose one another's change (gh#515). Without
// this, a caller doing Get() (a clone) → mutate → Update() races another doing
// the same: each starts from the same base and the second swap clobbers the
// first. -race does not flag it — each Get/Update is individually locked; the
// atomicity violation is at the compound level.
//
// fn receives a private deep copy it may mutate freely; returning a non-nil error
// aborts the mutation with no change. The result is validated before the swap.
//
// Re-entrancy contract: fn MUST NOT call any SafeConfig method (Get/Update/Mutate)
// — it operates only on the draft it is handed, or it self-deadlocks on sc.mu. Do
// post-mutation side effects (KV writes, subscriber notification) AFTER Mutate
// returns; capture any value they need into a variable inside fn.
//
// Unlike Update, validation runs INSIDE the write lock (it must, to keep the whole
// RMW atomic). Validate does filesystem I/O (os.Stat) when TLS is configured, so a
// mutation briefly blocks concurrent Get() readers — acceptable because config
// mutations are rare and off the hot path.
func (sc *SafeConfig) Mutate(fn func(*Config) error) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	draft := sc.config.Clone()
	if err := fn(draft); err != nil {
		return err
	}
	if err := draft.Validate(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	sc.config = draft
	return nil
}

// Clone creates a deep copy of the configuration
func (c *Config) Clone() *Config {
	if c == nil {
		return &Config{}
	}

	// Use JSON marshaling/unmarshaling for deep copy
	data, err := json.Marshal(c)
	if err != nil {
		// Fallback to shallow copy if marshaling fails
		copied := *c
		return &copied
	}

	var clone Config
	if err := json.Unmarshal(data, &clone); err != nil {
		// Fallback to shallow copy if unmarshaling fails
		copied := *c
		return &copied
	}

	return &clone
}

// NATSConfig defines NATS connection settings. It carries only the settings
// something reads: boot dials URLs, and VerifyJetStreamLimits reads JetStream.
// Credentials, TLS and reconnect settings are refused at load until boot
// applies them (unappliedNATSKeys).
type NATSConfig struct {
	URLs      []string        `json:"urls,omitempty"`
	JetStream JetStreamConfig `json:"jetstream,omitempty"`
}

// JetStreamConfig for JetStream settings. MaxMemory and MaxFileStore are
// verification hints; JetStream account limits are exclusively server-side configuration via
// nats.conf — the nats.go SDK exposes AccountInfo as read-only and
// offers no client-side mutation surface, so the framework cannot push
// operator intent. What it CAN do is compare configured-vs-server at
// boot and Warn on gap (see StreamsManager.VerifyJetStreamLimits) so an
// operator who set max_file_store: 10GB in the framework config but
// didn't update nats.conf isn't left wondering why stream-create
// eventually fails with "insufficient storage resources".
type JetStreamConfig struct {
	Enabled bool `json:"enabled"`
	// MaxMemory is the operator's expected server-side
	// max_memory_store limit. VerifyJetStreamLimits Warns at boot if
	// this exceeds the server's actual AccountInfo.Limits.MaxMemory.
	// 0 disables the check (the default for operators who manage
	// JetStream sizing entirely via nats.conf without surfacing intent
	// in the framework config).
	MaxMemory int64 `json:"max_memory,omitempty"`
	// MaxFileStore is the operator's expected server-side
	// max_file_store limit. Same Warn-on-gap semantics as MaxMemory.
	MaxFileStore int64 `json:"max_file_store,omitempty"`
}

// Validate checks if the config is valid
func (c *Config) Validate() error {
	// Validate and normalize org
	if c.Platform.Org == "" {
		return errors.New("platform.org is required")
	}

	// Normalize org to lowercase
	c.Platform.Org = strings.ToLower(c.Platform.Org)

	// Validate org is NATS-subject compatible
	if !isValidNATSSubjectPart(c.Platform.Org) {
		return fmt.Errorf(
			"platform.org '%s' is not valid for NATS subjects (must be alphanumeric with dots, dashes, underscore s)",
			c.Platform.Org,
		)
	}

	if c.Platform.ID == "" {
		return errors.New("platform.id is required")
	}
	if err := validateAuthorityPair(c.Platform.Org, c.Platform.ID); err != nil {
		return err
	}

	// Validate Security Configuration
	if err := c.validateSecurity(); err != nil {
		return fmt.Errorf("security configuration: %w", err)
	}

	// Validate Components
	for instanceName, config := range c.Components {
		if instanceName == "" {
			return errors.New("component instance name cannot be empty")
		}
		if err := config.Validate(); err != nil {
			return fmt.Errorf("component %s: %w", instanceName, err)
		}
	}

	// Validate stream declarations: the backing-stream prohibition, and the
	// bounds contract every ordinary stream owes. This runs here, and not only
	// in the provisioner, because `semstreams --validate` prints
	// "✓ Configuration is valid" and exits BEFORE streams are ensured
	// (internal/boot/run.go) — so without it a config that hard-fails boot
	// would be greenlit by the very tool an operator uses to avoid that.
	// Resolution is pure and I/O-free precisely so it can run here, which is
	// also what lets the port-derived lane be checked at validation rather than
	// only at boot.
	if _, err := ValidateStreamDeclarations(c); err != nil {
		return fmt.Errorf("streams: %w", err)
	}

	// Validate model registry if present
	if c.ModelRegistry != nil {
		if err := c.ModelRegistry.Validate(); err != nil {
			return fmt.Errorf("model_registry: %w", err)
		}
	}

	return nil
}

// isValidNATSSubjectPart checks if a string is valid for use in NATS subjects.
// Valid characters are alphanumeric, dots, dashes, and underscore s.
func isValidNATSSubjectPart(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) &&
			r != '-' && r != '_' && r != '.' {
			return false
		}
	}
	return true
}

// validateSecurity validates the security configuration
func (c *Config) validateSecurity() error {
	// Validate Server TLS
	if c.Security.TLS.Server.Enabled {
		if c.Security.TLS.Server.CertFile == "" {
			return errors.New("tls.server.cert_file is required when TLS is enabled")
		}
		if c.Security.TLS.Server.KeyFile == "" {
			return errors.New("tls.server.key_file is required when TLS is enabled")
		}

		// Check if cert file exists
		if _, err := os.Stat(c.Security.TLS.Server.CertFile); err != nil {
			return fmt.Errorf("tls.server.cert_file: %w", err)
		}

		// Check if key file exists
		if _, err := os.Stat(c.Security.TLS.Server.KeyFile); err != nil {
			return fmt.Errorf("tls.server.key_file: %w", err)
		}

		// Validate MinVersion if specified
		if c.Security.TLS.Server.MinVersion != "" {
			if err := validateTLSVersion(c.Security.TLS.Server.MinVersion); err != nil {
				return fmt.Errorf("tls.server.min_version: %w", err)
			}
		}
	}

	// Validate Client TLS
	// Check all CA files exist
	for i, caFile := range c.Security.TLS.Client.CAFiles {
		if _, err := os.Stat(caFile); err != nil {
			return fmt.Errorf("tls.client.ca_files[%d]: %w", i, err)
		}
	}

	// Warn if InsecureSkipVerify is enabled
	if c.Security.TLS.Client.InsecureSkipVerify {
		_, _ = fmt.Fprintf(
			os.Stderr,
			"WARNING: TLS certificate verification is disabled (insecure_skip_verify=true). This should only be used in development/testing!\n",
		)
	}

	// Validate MinVersion if specified
	if c.Security.TLS.Client.MinVersion != "" {
		if err := validateTLSVersion(c.Security.TLS.Client.MinVersion); err != nil {
			return fmt.Errorf("tls.client.min_version: %w", err)
		}
	}

	return nil
}

// validateTLSVersion checks if a TLS version string is valid
func validateTLSVersion(version string) error {
	switch version {
	case "1.2", "1.3":
		return nil
	default:
		return fmt.Errorf("invalid TLS version %q (must be \"1.2\" or \"1.3\")", version)
	}
}

// Loader loads one configuration document over the defaults.
type Loader struct {
	envPrefix string
}

// NewLoader creates a new configuration loader.
func NewLoader() *Loader {
	return &Loader{envPrefix: "SEMENGINE"}
}

// LoadFile loads configuration from a single file. It applies the defaults,
// the document, the environment overrides and the authority-pair reserve; it
// does not call Validate, which the caller does.
func (l *Loader) LoadFile(path string) (*Config, error) {
	rawConfig, err := l.loadRawJSON(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", path, err)
	}
	cfg, err := l.mergeFromMap(l.getDefaults(), rawConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", path, err)
	}
	return l.applyEnvAndReserve(cfg)
}

// LoadFromBytes loads configuration from JSON bytes.
// This is useful when you need to pre-process the configuration (e.g., environment
// variable expansion) before loading.
//
// The data is merged with the defaults exactly as LoadFile merges a file.
func (l *Loader) LoadFromBytes(data []byte) (*Config, error) {
	rawConfig, err := l.loadRawJSONFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	cfg, err := l.mergeFromMap(l.getDefaults(), rawConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return l.applyEnvAndReserve(cfg)
}

// applyEnvAndReserve is the tail both loaders share: the environment
// overrides, then the declared-pair reserve.
func (l *Loader) applyEnvAndReserve(cfg *Config) (*Config, error) {
	if err := l.rejectUnappliedEnv(); err != nil {
		return nil, err
	}
	l.applyEnvOverrides(cfg)

	// The declared-pair reserve runs on EVERY load: it is the one boundary
	// that knows this pair is a declaration, and a binary that skipped it
	// would mint a pair it can never carry (ADR-104).
	if err := validateDeclaredAuthorityPair(cfg.Platform.Org, cfg.Platform.ID); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadRawJSONFromBytes parses JSON bytes into a configuration map.
// This follows the same validation as loadRawJSON but without file I/O.
func (l *Loader) loadRawJSONFromBytes(data []byte) (map[string]any, error) {
	// Validate size to prevent DoS
	if len(data) > maxConfigSize {
		return nil, fmt.Errorf("config data too large: %d bytes > %d", len(data), maxConfigSize)
	}

	// Validate JSON depth to prevent DoS
	if err := validateJSONDepth(data); err != nil {
		return nil, fmt.Errorf("invalid JSON structure: %w", err)
	}

	// Unmarshal into map
	var rawConfig map[string]any
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return nil, err
	}
	if err := rejectRemovedPlatformFields(rawConfig); err != nil {
		return nil, err
	}
	if err := rejectUnappliedNATSKeys(rawConfig); err != nil {
		return nil, err
	}

	return rawConfig, nil
}

// getDefaults returns default configuration
func (l *Loader) getDefaults() *Config {
	return &Config{
		Platform: PlatformConfig{
			Region: "gulf_mexico",
		},
		NATS: NATSConfig{
			URLs: []string{"nats://localhost:4222"},
			JetStream: JetStreamConfig{
				Enabled: true,
			},
		},
		Services: types.ServiceConfigs{
			"discovery": types.ServiceConfig{
				Enabled: false, // Dormant by default
				Config:  json.RawMessage(`{}`),
			},
			"heartbeat": types.ServiceConfig{
				Enabled: true,
				Config:  json.RawMessage(`{"interval": "30s"}`),
			},
		},
		// Graph and ObjectStore configuration moved to components
	}
}

// loadRawJSON loads configuration from a JSON file as a map
func (l *Loader) loadRawJSON(path string) (map[string]any, error) {
	// Use secure file reading with validation
	data, err := safeReadFile(path)
	if err != nil {
		return nil, err
	}

	// Validate JSON depth to prevent DoS
	if err := validateJSONDepth(data); err != nil {
		return nil, fmt.Errorf("invalid JSON structure: %w", err)
	}

	// Unmarshal into map
	var rawConfig map[string]any
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return nil, err
	}
	if err := rejectRemovedPlatformFields(rawConfig); err != nil {
		return nil, err
	}
	if err := rejectUnappliedNATSKeys(rawConfig); err != nil {
		return nil, err
	}

	return rawConfig, nil
}

// mergeFromMap merges configuration from a raw map, only overriding fields
// present in the map. A value that does not fit its field is an error, never
// the defaults in its place.
func (l *Loader) mergeFromMap(base *Config, override map[string]any) (*Config, error) {
	baseJSON, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var baseMap map[string]any
	if err := json.Unmarshal(baseJSON, &baseMap); err != nil {
		return nil, err
	}

	mergedJSON, err := json.Marshal(l.deepMergeMaps(baseMap, override))
	if err != nil {
		return nil, err
	}
	var merged Config
	if err := json.Unmarshal(mergedJSON, &merged); err != nil {
		return nil, err
	}
	return &merged, nil
}

// deepMergeMaps recursively merges two maps, with override taking precedence
func (l *Loader) deepMergeMaps(base, override map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy base values
	for k, v := range base {
		result[k] = v
	}

	// Override with values from override map
	for k, v := range override {
		if v == nil {
			continue
		}

		// If both base and override have maps at this key, merge them
		if baseMap, baseOk := base[k].(map[string]any); baseOk {
			if overrideMap, overrideOk := v.(map[string]any); overrideOk {
				result[k] = l.deepMergeMaps(baseMap, overrideMap)
				continue
			}
		}

		// Otherwise, override takes precedence
		result[k] = v
	}

	return result
}

// parseDurationWithDays parses durations that may include days (e.g., "14d")
func parseDurationWithDays(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// applyEnvOverrides applies environment variable overrides
func (l *Loader) applyEnvOverrides(cfg *Config) {
	// Platform overrides
	if val := os.Getenv(l.envPrefix + "_PLATFORM_ID"); val != "" {
		cfg.Platform.ID = val
	}
	if val := os.Getenv(l.envPrefix + "_PLATFORM_TYPE"); val != "" {
		cfg.Platform.Type = val
	}
	if val := os.Getenv(l.envPrefix + "_PLATFORM_REGION"); val != "" {
		cfg.Platform.Region = val
	}

	// NATS overrides
	if val := os.Getenv(l.envPrefix + "_NATS_URLS"); val != "" {
		cfg.NATS.URLs = strings.Split(val, ",")
	}
}

// unappliedNATSEnv are the NATS credential overrides the pin applied and
// SemEngine does not yet: no field carries them until boot wires the client
// (#74). A set one fails the load rather than being dropped.
var unappliedNATSEnv = []string{"_NATS_USERNAME", "_NATS_PASSWORD", "_NATS_TOKEN"}

// rejectUnappliedEnv fails the load when an unapplied credential variable is
// set and not empty, naming it.
func (l *Loader) rejectUnappliedEnv() error {
	for _, suffix := range unappliedNATSEnv {
		name := l.envPrefix + suffix
		if os.Getenv(name) != "" {
			return fmt.Errorf("environment variable %s is set, but NATS credentials are not applied yet: unset it", name)
		}
	}
	return nil
}

// GetOrg returns the organization from platform config
func (c *Config) GetOrg() string {
	return c.Platform.Org
}

// GetPlatform returns the minting deployment authority: platform.id, the one
// identity field (ADR-102, ruled O-2; platform.instance_id was removed).
func (c *Config) GetPlatform() string {
	return c.Platform.ID
}

// mintedSuffixBytes is what the framework's entropy suffix costs the authority
// pair: one separator plus six lowercase hex bytes (ADR-104). It is reserved at
// configuration load rather than checked after the mint, because by then the
// suffixed identifier has been durably Created and ADR-102 d7 forbids rewriting
// a minted authority — a pair discovered to be too long at that point could
// never boot again.
const mintedSuffixBytes = len("-") + 6

// maxDeclarableAuthorityPairBytes is the budget for a pair as DECLARED — in a
// configuration document, before the framework mints anything onto it. It is
// the family-table budget less the entropy suffix that will be minted onto
// platform.id.
//
// It is deliberately NOT the bound on an effective pair. The reserve is a fact
// about a declaration, not about a pair: an effective pair already carries the
// suffix, so bounding it here too would reserve the same seven bytes twice and
// refuse every declaration in (156, 163] at Start after it had passed load.
func maxDeclarableAuthorityPairBytes() int {
	return semtypes.MaxAuthorityPairBytes() - mintedSuffixBytes
}

// validateDeclaredAuthorityPair bounds platform.org + platform.id as the
// configuration document declares them, reserving the seven bytes of the
// ADR-104 entropy suffix that will be minted onto platform.id at first boot.
//
// It runs at the load boundary — the one place that sees a pair which is
// certainly a declaration — and nowhere else. Reserving at load is what stops a
// pair that fits only unsuffixed from being durably recorded and then refused
// forever, which ADR-102 decision 7 makes unrepairable.
func validateDeclaredAuthorityPair(org, id string) error {
	pair := len(org) + len(id)
	if pair <= maxDeclarableAuthorityPairBytes() {
		return nil
	}
	binding := semtypes.LongestFrameworkIdentityFamily()
	return fmt.Errorf(
		"platform.org + platform.id is %d bytes; a declared authority pair may not exceed %d bytes — the %d-byte budget the %s identity family (%d fixed bytes) leaves under the %d-byte entity-ID bound, less the %d bytes of the entropy suffix the framework mints onto platform.id on first boot (ADR-104)",
		pair, maxDeclarableAuthorityPairBytes(), semtypes.MaxAuthorityPairBytes(),
		binding.Name, binding.FixedBytes(), semtypes.MaxEntityIDBytes, mintedSuffixBytes,
	)
}

// validateAuthorityPair bounds an EFFECTIVE platform.org + platform.id so every
// framework identity family fits under the canonical 256-byte entity-ID bound
// (ADR-102 amends ADR-076 d2; ruled O-14). The budget is derived from the
// framework's own family table, never configured by the operator, and the
// second check is by observation rather than arithmetic: the binding family's
// identity is composed under the pair and must validate, which also refuses an
// org or id that is not one canonical entity-ID segment.
//
// Effective means "the pair this deployment actually mints under": the minted
// identifier, an adopted platform_identity record's, or the running
// configuration's. Those already carry whatever suffix they will ever carry, so
// this bound is the family-table budget with NO reserve —
// validateDeclaredAuthorityPair owns the reserve, at the load boundary. Every
// effective-pair site shares this one rule, so none of them can admit a pair
// another rejects.
func validateAuthorityPair(org, id string) error {
	binding := semtypes.LongestFrameworkIdentityFamily()
	if pair := len(org) + len(id); pair > semtypes.MaxAuthorityPairBytes() {
		return fmt.Errorf(
			"platform.org + platform.id is %d bytes; the authority pair may not exceed %d bytes so that the %s identity family (%d fixed bytes) fits the %d-byte entity-ID bound",
			pair, semtypes.MaxAuthorityPairBytes(), binding.Name, binding.FixedBytes(), semtypes.MaxEntityIDBytes,
		)
	}
	if _, err := binding.EntityID(org, id, strings.Repeat("0", binding.InstanceBytes)); err != nil {
		return fmt.Errorf("platform.org %q / platform.id %q cannot carry the %s identity family: %w", org, id, binding.Name, err)
	}
	return nil
}

// removedPlatformFields maps every field withdrawn from the platform block to
// the guidance that replaces it. encoding/json silently DROPS a key with no
// matching struct field, so without this probe an operator upgrading past
// ADR-102 would keep `instance_id` in their config, see no error, and mint
// under platform.id instead of the value they believed was their authority.
var removedPlatformFields = map[string]string{
	"instance_id": "removed (ADR-102, BREAKING): platform.id is the single deployment authority field and positions 1-2 of every minted entity ID. Delete instance_id and put its value in platform.id",
}

// rejectRemovedPlatformFields fails the load when a withdrawn platform field is
// present, naming its replacement.
func rejectRemovedPlatformFields(raw map[string]any) error {
	platform, ok := raw["platform"].(map[string]any)
	if !ok {
		return nil
	}
	for field, guidance := range removedPlatformFields {
		if _, found := platform[field]; found {
			return fmt.Errorf("config field platform.%s was %s", field, guidance)
		}
	}
	return nil
}

// unappliedNATSKeys and unappliedJetStreamKeys are settings nothing in
// SemEngine applies yet (#74). encoding/json would drop them silently once
// their fields are gone, so a document that sets one, whatever its value, is
// refused, naming the key. The boot change that builds the NATS client adds
// each back with its wiring.
var (
	unappliedNATSKeys      = []string{"username", "password", "token", "tls", "max_reconnects", "reconnect_wait"}
	unappliedJetStreamKeys = []string{"domain", "retention_policy", "replication_factor"}
)

// rejectUnappliedNATSKeys fails the load when the document sets an unapplied
// NATS key. Keys match in any letter case, as encoding/json matches field
// names; the error names the canonical key. A nats block that is not an
// object is left to the decoder.
func rejectUnappliedNATSKeys(raw map[string]any) error {
	nats := objectsFolded([]map[string]any{raw}, "nats")
	for _, key := range unappliedNATSKeys {
		if hasKeyFolded(nats, key) {
			return fmt.Errorf("config key nats.%s is not applied yet: remove it", key)
		}
	}
	jetStream := objectsFolded(nats, "jetstream")
	for _, key := range unappliedJetStreamKeys {
		if hasKeyFolded(jetStream, key) {
			return fmt.Errorf("config key nats.jetstream.%s is not applied yet: remove it", key)
		}
	}
	return nil
}

// objectsFolded returns every object in the given maps stored under a key
// equal to name in any letter case.
func objectsFolded(maps []map[string]any, name string) []map[string]any {
	var found []map[string]any
	for _, m := range maps {
		for k, v := range m {
			if obj, ok := v.(map[string]any); ok && strings.EqualFold(k, name) {
				found = append(found, obj)
			}
		}
	}
	return found
}

// hasKeyFolded reports whether any of the maps has a key equal to name in any
// letter case.
func hasKeyFolded(maps []map[string]any, name string) bool {
	for _, m := range maps {
		for k := range m {
			if strings.EqualFold(k, name) {
				return true
			}
		}
	}
	return false
}

// CompareVersions compares two semver version strings
// Returns:
//
//	-1 if v1 < v2
//	 0 if v1 == v2
//	 1 if v1 > v2
//	error if either version is invalid
func CompareVersions(v1, v2 string) (int, error) {
	// Parse version 1
	major1, minor1, patch1, err := parseSemVer(v1)
	if err != nil {
		return 0, fmt.Errorf("invalid version '%s': %w", v1, err)
	}

	// Parse version 2
	major2, minor2, patch2, err := parseSemVer(v2)
	if err != nil {
		return 0, fmt.Errorf("invalid version '%s': %w", v2, err)
	}

	// Compare major
	if major1 != major2 {
		if major1 > major2 {
			return 1, nil
		}
		return -1, nil
	}

	// Compare minor
	if minor1 != minor2 {
		if minor1 > minor2 {
			return 1, nil
		}
		return -1, nil
	}

	// Compare patch
	if patch1 != patch2 {
		if patch1 > patch2 {
			return 1, nil
		}
		return -1, nil
	}

	// Equal
	return 0, nil
}

// parseSemVer parses a semantic version string (e.g., "1.2.3")
// Returns major, minor, patch, error
func parseSemVer(version string) (int, int, int, error) {
	if version == "" {
		return 0, 0, 0, errors.New("version cannot be empty")
	}

	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// Split into parts
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("version must be in format 'major.minor.patch', got '%s'", version)
	}

	// Parse major
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version '%s': %w", parts[0], err)
	}

	// Parse minor
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version '%s': %w", parts[1], err)
	}

	// Parse patch
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version '%s': %w", parts[2], err)
	}

	return major, minor, patch, nil
}
