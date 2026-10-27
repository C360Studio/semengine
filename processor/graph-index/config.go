package graphindex

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/pkg/errs"
)

// maxWorkers is the most index lanes a configuration may ask for.
const maxWorkers = 16

// Config holds configuration for graph-index component. Decoding is strict: a key it
// does not define, batch_size among them, is refused (design D9).
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	// Workers is the index lane count, 1 to 16; 0 means the default, 1 (semsource sends 0
	// to mean "use the default").
	Workers int `json:"workers" schema:"type:int,description:Index lane count (1-16; 0 = default 1),category:advanced"`

	// Dependency startup configuration
	StartupAttempts int `json:"startup_attempts,omitempty" schema:"type:int,description:Max attempts to wait for dependencies at startup,category:advanced"`
	StartupInterval int `json:"startup_interval_ms,omitempty" schema:"type:int,description:Interval between startup attempts in milliseconds,category:advanced"`
	CoalesceMs      int `json:"coalesce_ms,omitempty" schema:"type:int,description:Debounce window for entity updates in ms. 0=immediate processing,category:advanced"`
}

// Validate implements component.Validatable interface
func (c *Config) Validate() error {
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	if len(c.Ports.Inputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one input port required")
	}
	if len(c.Ports.Outputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one output port required")
	}

	// graph-index reads one fixed source: the follower watches every key of ENTITY_STATES,
	// latest values only. An input declaring another kind, bucket, key filter or history
	// would advertise a feed nothing reads, so it is refused (#424).
	for _, definition := range c.Ports.Inputs {
		input, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "resolve input port")
		}
		watch, ok := input.Config.(component.KVWatchPort)
		if !ok || watch.Bucket != graph.BucketEntityStates || len(watch.Keys) > 0 || watch.History {
			return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
				fmt.Sprintf("input port %q must be a kv-watch of every %s key with no history, the one source graph-index reads",
					input.Name, graph.BucketEntityStates))
		}
	}

	// The four graph-index-owned buckets are all required and the only outputs permitted.
	// An output outside the set is refused whether it is off the catalog (a typo that
	// would otherwise become a stray bucket) or a catalog bucket another component owns
	// (which would route graph-index through the owner seam for a bucket it does not own).
	requiredBuckets := map[string]bool{
		graph.BucketOutgoingIndex:  false,
		graph.BucketIncomingIndex:  false,
		graph.BucketAliasIndex:     false,
		graph.BucketPredicateIndex: false,
	}
	for _, definition := range c.Ports.Outputs {
		output, err := definition.Resolve(component.DirectionOutput)
		if err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "resolve output port")
		}
		facts, err := output.Facts()
		if err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "project output port facts")
		}
		if facts.Kind() != component.PortKindKVWrite {
			return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
				fmt.Sprintf("output port %q kind %q is not a KV writer", output.Name, facts.Kind()))
		}
		bucket := strings.TrimPrefix(facts.ResourceID(), "kv:")
		if _, required := requiredBuckets[bucket]; !required {
			if owner := kvcatalog.OwnerOf(bucket); owner != "" {
				return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
					fmt.Sprintf("output port %q subject %q is a framework bucket owned by %s, not graph-index",
						output.Name, bucket, owner))
			}
			return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
				fmt.Sprintf("output port %q subject %q does not resolve to a graph-index-owned framework KV catalog bucket",
					output.Name, bucket))
		}
		requiredBuckets[bucket] = true
	}
	for bucket, found := range requiredBuckets {
		if !found {
			return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
				fmt.Sprintf("required output bucket missing: %s", bucket))
		}
	}

	if c.Workers < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "workers cannot be negative")
	}
	if c.Workers > maxWorkers {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("workers cannot exceed %d", maxWorkers))
	}
	return nil
}

// ApplyDefaults sets default values for configuration. Validate only checks.
func (c *Config) ApplyDefaults() {
	if c.Workers == 0 {
		c.Workers = 1
	}
	if c.StartupAttempts == 0 {
		c.StartupAttempts = 30 // ~15 seconds with 500ms interval
	}
	if c.StartupInterval == 0 {
		c.StartupInterval = 500 // 500ms
	}
	// A configuration with no ports takes DefaultConfig's; a ports block that is
	// present is used as written, never merged with the defaults (ruling K, #91 comment
	// 6062681355; PR #414 comment 6431777824).
	if c.Ports == nil {
		c.Ports = DefaultConfig().Ports
	}
}

// DefaultConfig returns a valid default configuration
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}},
			},
			Outputs: []component.PortDefinition{
				{Name: "outgoing_index", Config: component.KVWritePort{Bucket: graph.BucketOutgoingIndex}},
				{Name: "incoming_index", Config: component.KVWritePort{Bucket: graph.BucketIncomingIndex}},
				{Name: "alias_index", Config: component.KVWritePort{Bucket: graph.BucketAliasIndex}},
				{Name: "predicate_index", Config: component.KVWritePort{Bucket: graph.BucketPredicateIndex}},
			},
		},
		Workers: 1,
	}
}

// schema defines the configuration schema for graph-index component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))
