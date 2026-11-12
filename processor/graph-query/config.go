package graphquery

import (
	"fmt"
	"math"
	"reflect"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

// defaultQueryTimeoutMs is above graph-ingest's 10 s budget on batch and prefix
// (processor/graph-ingest/query.go), so the slowest forward can finish, and inside natsclient's 30 s
// budget for a request handler (design D7).
const defaultQueryTimeoutMs = 15000

// maxQueryTimeoutMs is the largest millisecond count a time.Duration holds; one more wraps negative.
const maxQueryTimeoutMs = math.MaxInt64 / int64(time.Millisecond)

// Config holds the graph-query configuration. Decoding is strict: a key it does not define is
// refused, naming the key.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:The one required graph_queries input,category:basic"`
	// QueryTimeoutMs is the deadline on the reply to each downstream request, in milliseconds; 0 means
	// the default, 15000. It does not end a wait to write to a broker that has stopped reading
	// (ADR-112, "Time").
	QueryTimeoutMs int `json:"query_timeout_ms,omitempty" schema:"type:int,description:Deadline on the reply to each downstream request in milliseconds; a blocked write can outlast it (ADR-112),default:15000,category:basic"`
	// EmbeddingEnabled says the composition runs graph-embedding. When false, semantic and similar
	// refuse every request not_enabled without sending it. A configuration that leaves the key out,
	// or sets it null, reads true: resolveConfig decodes into a Config that already holds true. A
	// Config built in Go starts false.
	EmbeddingEnabled bool `json:"embedding_enabled" schema:"type:bool,description:Whether graph-embedding runs; when false semantic and similar are refused not_enabled,default:true,category:basic"`
}

// queriesPort is the one input graph-query serves.
func queriesPort() component.PortDefinition {
	return component.PortDefinition{
		Name:     "graph_queries",
		Required: true,
		Config: component.NATSRequestPort{
			Subject:   "graph.query.*",
			Interface: &component.InterfaceContract{Type: "graph.query", Version: "v1"},
		},
	}
}

// Validate implements component.Validatable. It only checks; a Config that has not had its
// defaults applied has no ports and is refused.
//
// graph-query answers the verbs graph.QueryVerbs declares for it, on one subject family, and writes
// nothing of its own. A port that says anything else would advertise what nothing serves, so the
// one input must match queriesPort in name, required and everything under config. Its description
// and external statement change nothing it serves, and are the operator's.
func (c *Config) Validate() error {
	if c.QueryTimeoutMs < 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "query_timeout_ms cannot be negative")
	}
	if int64(c.QueryTimeoutMs) > maxQueryTimeoutMs {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("query_timeout_ms %d does not fit a duration, at most %d", c.QueryTimeoutMs, maxQueryTimeoutMs))
	}
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	if len(c.Ports.Outputs) != 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("graph-query has no output ports, got %d", len(c.Ports.Outputs)))
	}
	want := queriesPort()
	if len(c.Ports.Inputs) != 1 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("exactly one %s input port is required, got %d", want.Name, len(c.Ports.Inputs)))
	}
	input, err := c.Ports.Inputs[0].Resolve(component.DirectionInput)
	if err != nil {
		return errs.WrapInvalid(err, "Config", "Validate", "resolve input port")
	}
	wantPort, err := want.Resolve(component.DirectionInput)
	if err != nil {
		return errs.WrapInvalid(err, "Config", "Validate", "resolve the required port")
	}
	if input.Name != wantPort.Name || !input.Required || !reflect.DeepEqual(input.Config, wantPort.Config) {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("input port %q must be the required %s nats-request port on graph.query.*, graph.query v1, with no retries and no timeout but the default 1s",
				input.Name, want.Name))
	}
	return nil
}

// ApplyDefaults sets default values. A configuration with no ports takes DefaultConfig's; a ports
// block that is present is used as written, never merged with the defaults (ruling K, #91 comment
// 6062681355).
func (c *Config) ApplyDefaults() {
	if c.QueryTimeoutMs == 0 {
		c.QueryTimeoutMs = defaultQueryTimeoutMs
	}
	if c.Ports == nil {
		c.Ports = DefaultConfig().Ports
	}
}

// DefaultConfig returns a valid default configuration.
func DefaultConfig() Config {
	return Config{
		Ports:            &component.PortConfig{Inputs: []component.PortDefinition{queriesPort()}},
		QueryTimeoutMs:   defaultQueryTimeoutMs,
		EmbeddingEnabled: true,
	}
}

// schema defines the configuration schema for the graph-query component.
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))
