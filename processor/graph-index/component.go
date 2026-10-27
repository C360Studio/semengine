// Package graphindex provides the graph-index component for maintaining graph relationship indexes.
package graphindex

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// key names graph-index once: its registration, its logger, its metric keys and their
// subsystem, and its GRAPH_STATUS key (design D3).
const key = readiness.KeyGraphIndex

var _ component.Discoverable = (*Component)(nil)

// Component implements the graph-index processor.
type Component struct {
	config  Config
	inputs  []component.Port
	outputs []component.Port

	natsClient *natsclient.Client
	logger     *slog.Logger

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle,
	// ADR-108, design D4).
	lifecycle component.Lifecycle

	metrics *indexMetrics
}

// DeclarePorts is the component.PortDeclarer for graph-index: the ports
// CreateGraphIndex will report for rawConfig, computed without dependencies.
func DeclarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, outputs), nil
}

// resolveConfig parses rawConfig, applies defaults, validates, and resolves the effective
// ports. It is the one derivation DeclarePorts and CreateGraphIndex share.
func resolveConfig(rawConfig json.RawMessage) (Config, []component.Port, []component.Port, error) {
	var config Config
	if len(rawConfig) > 0 {
		// Strict decoding: a key Config does not define is refused, naming the key, where
		// encoding/json would drop it (design D9). A port definition decodes through its
		// own UnmarshalJSON, which this setting does not reach.
		decoder := json.NewDecoder(bytes.NewReader(rawConfig))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "CreateGraphIndex", "factory", "config unmarshal")
		}
		// json.Unmarshal, which the pin used, refused data after the object; keep that.
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Config{}, nil, nil, errs.WrapInvalid(errors.New("data after the configuration object"),
				"CreateGraphIndex", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, nil, nil, errs.Wrap(err, "CreateGraphIndex", "factory", "config validation")
	}
	inputs := make([]component.Port, 0, len(config.Ports.Inputs))
	for _, definition := range config.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "CreateGraphIndex", "factory", "resolve input port")
		}
		inputs = append(inputs, port)
	}
	outputs := make([]component.Port, 0, len(config.Ports.Outputs))
	for _, definition := range config.Ports.Outputs {
		port, err := definition.Resolve(component.DirectionOutput)
		if err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "CreateGraphIndex", "factory", "resolve output port")
		}
		outputs = append(outputs, port)
	}
	return config, inputs, outputs, nil
}

// CreateGraphIndex is the factory function for creating graph-index components. It
// registers the component's collectors on deps.MetricsRegistry and opens no connection.
func CreateGraphIndex(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphIndex", "factory", "NATSClient required")
	}
	config, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	metrics, err := newIndexMetrics(deps.MetricsRegistry)
	if err != nil {
		return nil, errs.Wrap(err, "CreateGraphIndex", "factory", "register metrics")
	}
	return &Component{
		config:     config,
		inputs:     inputs,
		outputs:    outputs,
		natsClient: deps.NATSClient,
		logger:     deps.GetLoggerWithComponent(key),
		metrics:    metrics,
	}, nil
}

// Register registers the graph-index factory with the component registry.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph relationship index maintenance processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphIndex,
		Ports:       DeclarePorts,
	})
}

// InputPorts returns the input ports, fixed at construction.
func (c *Component) InputPorts() []component.Port {
	return slices.Clone(c.inputs)
}

// OutputPorts returns the output ports, fixed at construction.
func (c *Component) OutputPorts() []component.Port {
	return slices.Clone(c.outputs)
}

// Health reports the shell's verdict (component.Lifecycle.Health). The follower's stuck
// states join it with the lifecycle (task 4.3).
func (c *Component) Health() component.HealthStatus {
	return c.lifecycle.Health()
}

// DataFlow reports the shell's input counts (component.Lifecycle.DataFlow).
func (c *Component) DataFlow() component.FlowMetrics {
	return c.lifecycle.DataFlow()
}
