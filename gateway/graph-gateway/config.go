package graphgateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

// defaultQueryTimeoutMs is the default query timeout, 60 s.
const defaultQueryTimeoutMs = 60000

// maxQueryTimeoutMs is the largest millisecond count a time.Duration holds; one more wraps negative.
const maxQueryTimeoutMs = math.MaxInt64 / int64(time.Millisecond)

// Config holds the graph-gateway configuration. Decoding is strict: a key it does not define is
// refused, naming the key.
type Config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:The one required graph_queries output,category:basic"`
	// QueryTimeoutMs bounds each request the gateway sends for a root field, in milliseconds; 0 means
	// the default, 60000. A request past it answers HTTP 504.
	QueryTimeoutMs int `json:"query_timeout_ms,omitempty" schema:"type:int,description:Deadline on each graph.query request in milliseconds; past it the answer is HTTP 504,default:60000,category:basic"`
}

// queriesPort is the one output graph-gateway requests on.
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

// Validate implements component.Validatable. It only checks; a Config that has not had its defaults
// applied has no ports and is refused.
//
// The gateway requests only the graph.query.* verbs graph.QueryVerbs declares, so its one output
// must match queriesPort in name, required and everything under config, as graph-query checks its
// input (processor/graph-query/config.go). Its description and external statement change nothing it
// requests, and are the operator's.
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
	if len(c.Ports.Inputs) != 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("graph-gateway has no input ports, got %d", len(c.Ports.Inputs)))
	}
	want := queriesPort()
	if len(c.Ports.Outputs) != 1 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("exactly one %s output port is required, got %d", want.Name, len(c.Ports.Outputs)))
	}
	output, err := c.Ports.Outputs[0].Resolve(component.DirectionOutput)
	if err != nil {
		return errs.WrapInvalid(err, "Config", "Validate", "resolve output port")
	}
	wantPort, err := want.Resolve(component.DirectionOutput)
	if err != nil {
		return errs.WrapInvalid(err, "Config", "Validate", "resolve the required port")
	}
	if output.Name != wantPort.Name || !output.Required || !reflect.DeepEqual(output.Config, wantPort.Config) {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate",
			fmt.Sprintf("output port %q must be the required %s nats-request port on graph.query.*, graph.query v1, with no retries and no timeout of its own",
				output.Name, want.Name))
	}
	return nil
}

// ApplyDefaults sets default values. A configuration with no ports takes DefaultConfig's; a ports
// block that is present is used as written, never merged with the defaults.
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
		Ports:          &component.PortConfig{Outputs: []component.PortDefinition{queriesPort()}},
		QueryTimeoutMs: defaultQueryTimeoutMs,
	}
}

// configSchema is the configuration schema of the registration.
var configSchema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// resolveConfig parses rawConfig strictly, applies defaults, validates, and resolves the one output
// port. It is the one derivation DeclarePorts and CreateGraphGateway share.
func resolveConfig(rawConfig json.RawMessage) (Config, []component.Port, error) {
	var config Config
	if len(rawConfig) > 0 {
		// A port definition decodes through its own UnmarshalJSON, which is strict too.
		decoder := json.NewDecoder(bytes.NewReader(rawConfig))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return Config{}, nil, errs.WrapInvalid(err, "CreateGraphGateway", "factory", "config unmarshal")
		}
		// DisallowUnknownFields reads one value; json.Unmarshal would refuse what follows it.
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Config{}, nil, errs.WrapInvalid(errors.New("data after the configuration object"),
				"CreateGraphGateway", "factory", "config unmarshal")
		}
	}
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, nil, errs.Wrap(err, "CreateGraphGateway", "factory", "config validation")
	}
	output, err := config.Ports.Outputs[0].Resolve(component.DirectionOutput)
	if err != nil {
		return Config{}, nil, errs.WrapInvalid(err, "CreateGraphGateway", "factory", "resolve output port")
	}
	return config, []component.Port{output}, nil
}
