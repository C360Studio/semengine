package graphingest

import (
	"fmt"
	"reflect"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/pkg/errs"
)

// Config holds configuration for graph-ingest component
type Config struct {
	Ports           *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	EnableHierarchy bool                  `json:"enable_hierarchy" schema:"type:bool,description:Enable hierarchy inference,default:false,category:advanced"`
	// IngestLanes is the number of keyed-concurrent ingest lanes (SemStreams ADR-072,
	// SemStreams gh#480). Messages are partitioned by entity ID (same entity → one lane →
	// serial in arrival order, preserving the arrival-order merge; different
	// entities → parallel), so ingest is no longer bound to a single serial
	// Get+CAS round-trip chain. Default 8 (concurrent); `1` selects the prior
	// fully-serial behavior.
	IngestLanes int `json:"ingest_lanes" schema:"type:int,description:Keyed-concurrent ingest lane count (same entity ID → one lane → ordered; 1 = serial),default:8,category:advanced"`
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
	// graph-ingest names each JetStream input's durable consumer from its subject
	// (consume.go), so a declared consumer_name would be dropped; it is refused (#442).
	for _, definition := range c.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "resolve input port")
		}
		if js, ok := port.Config.(component.JetStreamPort); ok && js.ConsumerName != "" {
			return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", fmt.Sprintf(
				"check input port %q (consumer_name is not supported: graph-ingest names the consumer from the subject)", definition.Name))
		}
	}
	// graph-ingest writes entity state to ENTITY_STATES only (initStorage opens it by
	// its fixed name) and publishes nowhere else, so any other output would be
	// declared and reported while nothing is written there (checkpoint 3 H2).
	for _, definition := range c.Ports.Outputs {
		port, err := definition.Resolve(component.DirectionOutput)
		if err != nil {
			return errs.WrapInvalid(err, "Config", "Validate", "resolve output port")
		}
		if kv, ok := port.Config.(component.KVWritePort); !ok || kv.Bucket != graph.BucketEntityStates {
			return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", fmt.Sprintf(
				"check output port %q (graph-ingest writes only a kv-write output on %s)", definition.Name, graph.BucketEntityStates))
		}
	}
	if err := requireMutationProviderPort(c.Ports); err != nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", err.Error())
	}
	return nil
}

// ApplyDefaults sets default values for configuration. Validate only checks
// (ruling K, #91 comment 6062681355).
func (c *Config) ApplyDefaults() {
	// A configuration with no ports takes DefaultConfig's; a ports block that is
	// present is used as written, never merged with the defaults (ruling K).
	if c.Ports == nil {
		c.Ports = DefaultConfig().Ports
	}
	// Concurrent-by-default (SemStreams ADR-072): 0 (unset) → 8 lanes. An explicit 1 opts
	// back into serial and is preserved. A negative count clamps to serial rather
	// than failing boot (owner ruling, #91 comment 6059144952).
	switch {
	case c.IngestLanes == 0:
		c.IngestLanes = defaultIngestLanes
	case c.IngestLanes < 1:
		c.IngestLanes = 1
	}
}

// DefaultConfig returns a valid default configuration
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name: "entity_stream", Config: component.JetStreamPort{StreamName: "ENTITY", Subjects: []string{"entity.>"}, DeliverPolicy: "all"}, // Idempotent: catch up on historical entities

				},
				{
					Name: "mutations", Config: component.NATSRequestPort{Subject: graphmutation.SubjectFamily, Interface: &component.InterfaceContract{Type: graphmutation.InterfaceType, Version: graphmutation.InterfaceVersion}}, Required: true,
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name: "entity_states", Config: component.KVWritePort{Bucket: graph.BucketEntityStates},
				},
			},
		},
		EnableHierarchy: false,
		IngestLanes:     defaultIngestLanes,
	}
}

// Keyed-concurrent ingest sizing (SemStreams ADR-072, SemStreams gh#480).
const (
	// defaultIngestLanes is the default keyed-concurrent lane count. 8 overlaps
	// the KV Get+CAS round-trips on the ~11/12-idle box the issue profiled;
	// tune against the semboids repro via the inflight/queue-wait metrics.
	defaultIngestLanes = 8

	// ingestLaneQueueDepth bounds each lane's in-memory submit queue. Total
	// buffered work is capped at lanes × this; SubmitBlocking applies
	// backpressure past it. Sized generously vs. AckWait so the pipeline drains
	// before redelivery (defense-in-depth atop the applied-sequence guard).
	ingestLaneQueueDepth = 256

	// ingestGuardMemMaxPerLane bounds the in-memory redelivery-guard cache per
	// lane. This is a CACHE-SIZE knob only, not correctness: an eviction just
	// costs a durable-tier read on the next access (SemStreams ADR-072 B3). Sized to hold
	// the working set of recently-active entities per lane under load.
	ingestGuardMemMaxPerLane = 65536
)

// schema defines the configuration schema for graph-ingest component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))
