package graphclustering

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

// defaultDetectionInterval is the time between publications when detection_interval is
// unset: the pin's.
const defaultDetectionInterval = "30s"

// removedKeys are the pin's configuration keys graph-clustering no longer defines, each with
// what replaces it (design D3). A configuration carrying one is refused at construction with
// that guidance, where strict decoding alone would only say the key is unknown.
var removedKeys = map[string]string{
	"batch_size":          "it was never read; changed groups are published every detection_interval",
	"enable_llm":          "LLM community summaries are tier 2 and not ported (#485); records carry a statistical summary",
	"enhancement_workers": "LLM community summaries are tier 2 and not ported (#485)",
	"min_community_size": "communities are entity-ID prefix groups, not label propagation (ADR-099); " +
		"every group is kept whatever its size",
	"max_iterations": "communities are entity-ID prefix groups, not label propagation (ADR-099); nothing iterates",
	"entity_id_edges": "communities are entity-ID prefix groups (ADR-099); no edges are synthesized, " +
		"siblings share their prefix group",
	"semantic_edges":      "communities are entity-ID prefix groups (ADR-099); no semantic edges are synthesized",
	"allow_ungated_reads": "graph-clustering reads no other index, so there is no readiness gate to bypass (ADR-099)",
	"enable_anomaly_detection": "anomaly detection is not ported and will not be (owner ruling, " +
		"PR #582 comment 6768832111)",
	"anomaly_config": "anomaly detection is not ported and will not be (owner ruling, PR #582 comment 6768832111)",
}

// Config holds the graph-clustering configuration. Decoding is strict: a key it does not
// define is refused, and a removed key is refused with what replaces it (design D3).
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	// DetectionInterval is the time between publications of the changed groups, a Go duration
	// such as 30s or 5m; empty means 30s.
	DetectionInterval string `json:"detection_interval,omitempty" schema:"type:string,description:Interval between publications of changed groups (e.g. 30s or 5m),category:basic"`

	// Dependency startup configuration
	StartupAttempts int `json:"startup_attempts,omitempty" schema:"type:int,description:Max attempts to wait for dependencies at startup,category:advanced"`
	StartupInterval int `json:"startup_interval_ms,omitempty" schema:"type:int,description:Interval between startup attempts in milliseconds,category:advanced"`
}

// Validate implements component.Validatable.
func (c *Config) Validate() error {
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	// The component writes COMMUNITY_INDEX itself, not through a declared output, so a
	// declared output would advertise a write it never performs.
	if len(c.Ports.Outputs) > 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("graph-clustering declares no output ports; it writes %s itself; remove ports.outputs",
				graph.BucketCommunityIndex))
	}
	if err := validateInputs(c.Ports.Inputs); err != nil {
		return err
	}
	if _, err := c.interval(); err != nil {
		return err
	}
	if c.StartupAttempts < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "startup_attempts cannot be negative")
	}
	if c.StartupInterval < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "startup_interval_ms cannot be negative")
	}
	return nil
}

// validateInputs accepts exactly one input, the follower's watch of every ENTITY_STATES key with
// no history. Any other input would advertise a feed nothing reads, so it is refused naming the
// port (#424, as graph-index).
func validateInputs(definitions []component.PortDefinition) error {
	if len(definitions) != 1 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("exactly one input port is required, a kv-watch of every %s key; got %d",
				graph.BucketEntityStates, len(definitions)))
	}
	input, err := definitions[0].Resolve(component.DirectionInput)
	if err != nil {
		return errs.WrapInvalid(err, "Config", "Validate", "resolve input port")
	}
	if port, ok := input.Config.(component.KVWatchPort); ok &&
		port.Bucket == graph.BucketEntityStates && len(port.Keys) == 0 && !port.History {
		return nil
	}
	return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
		fmt.Sprintf("input port %q must be a kv-watch of every %s key with no history, the one input graph-clustering reads",
			input.Name, graph.BucketEntityStates))
}

// interval is the parsed detection_interval. Unlike the pin, which fell back to 30s on a
// value it could not parse, a value that does not parse or is not above zero is refused.
func (c *Config) interval() (time.Duration, error) {
	d, err := time.ParseDuration(c.DetectionInterval)
	if err != nil {
		return 0, errs.WrapInvalid(err, "Config", "Validate",
			fmt.Sprintf("detection_interval %q is not a duration such as 30s or 5m", c.DetectionInterval))
	}
	if d <= 0 {
		return 0, errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("detection_interval %q must be above zero", c.DetectionInterval))
	}
	return d, nil
}

// ApplyDefaults resolves each zero value to the value its reader uses. Validate only checks.
func (c *Config) ApplyDefaults() {
	if c.DetectionInterval == "" {
		c.DetectionInterval = defaultDetectionInterval
	}
	if c.StartupAttempts == 0 {
		c.StartupAttempts = 30 // ~15 seconds with 500ms interval
	}
	if c.StartupInterval == 0 {
		c.StartupInterval = 500 // 500ms
	}
	// A configuration with no ports takes DefaultConfig's; a ports block that is present is
	// used as written, never merged with the defaults (ruling K, #91 comment 6062681355).
	if c.Ports == nil {
		c.Ports = DefaultConfig().Ports
	}
}

// DefaultConfig returns a valid default configuration.
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}},
			},
		},
		DetectionInterval: defaultDetectionInterval,
	}
}

// decodeConfig decodes rawConfig strictly: a removed key is refused naming it and what replaces
// it, then any other key Config does not define is refused naming it, and data after the object
// is refused, as json.Unmarshal did at the pin. Empty rawConfig is DefaultConfig.
func decodeConfig(rawConfig json.RawMessage) (Config, error) {
	if len(rawConfig) == 0 {
		return DefaultConfig(), nil
	}
	// A value that is not one object is left to the strict decoder below, which names its fault.
	var fields map[string]json.RawMessage
	if json.Unmarshal(rawConfig, &fields) == nil {
		var refused []string
		for _, key := range slices.Sorted(maps.Keys(fields)) {
			if guidance, ok := removedKeys[key]; ok {
				refused = append(refused, fmt.Sprintf("%q was removed: %s", key, guidance))
			}
		}
		if len(refused) > 0 {
			return Config{}, errs.WrapInvalid(errors.New(strings.Join(refused, "; ")),
				"CreateGraphClustering", "factory", "config unmarshal")
		}
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(rawConfig))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, errs.WrapInvalid(err, "CreateGraphClustering", "factory", "config unmarshal")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Config{}, errs.WrapInvalid(errors.New("data after the configuration object"),
			"CreateGraphClustering", "factory", "config unmarshal")
	}
	return config, nil
}

// schema defines the configuration schema for the graph-clustering component.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))
