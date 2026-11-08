package graphembedding

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
	"github.com/c360studio/semengine/storage/storeregistry"
)

// key names graph-embedding once: its registration, its logger and its GRAPH_STATUS key.
const key = readiness.KeyGraphEmbedding

var _ component.Discoverable = (*Component)(nil)

// Component implements the graph-embedding processor.
//
// shortcut: construction only, until tasks 2.2 and 2.3 of
// setup-04a-05-first-consumer-graph-embedding add Initialize, Start, Stop, the follower
// plug-in and the Worker.
type Component struct {
	config  Config
	inputs  []component.Port
	outputs []component.Port

	natsClient *natsclient.Client
	logger     *slog.Logger

	// storeRegistry resolves an offloaded body's StorageRef to its store (ADR-063); it is the
	// injected registry when a store-read input is declared and nil otherwise.
	storeRegistry *storeregistry.Registry

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle
}

// DeclarePorts is the component.PortDeclarer for graph-embedding: the ports
// CreateGraphEmbedding will report for rawConfig, computed without dependencies.
func DeclarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, outputs), nil
}

// resolveConfig parses rawConfig, applies defaults, validates, and resolves the effective
// ports. It is the one derivation DeclarePorts and CreateGraphEmbedding share.
func resolveConfig(rawConfig json.RawMessage) (Config, []component.Port, []component.Port, error) {
	var config Config
	if len(rawConfig) > 0 {
		// Strict decoding: a key Config does not define is refused, naming the key, where
		// encoding/json would drop it (design D4). It replaces the pin's targeted cache_ttl
		// probe. A port definition decodes through its own UnmarshalJSON.
		decoder := json.NewDecoder(bytes.NewReader(rawConfig))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "CreateGraphEmbedding", "factory", "config unmarshal")
		}
		// json.Unmarshal, which the pin used, refused data after the object; keep that.
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Config{}, nil, nil, errs.WrapInvalid(errors.New("data after the configuration object"),
				"CreateGraphEmbedding", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, nil, nil, errs.Wrap(err, "CreateGraphEmbedding", "factory", "config validation")
	}
	inputs := make([]component.Port, 0, len(config.Ports.Inputs))
	for _, definition := range config.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "CreateGraphEmbedding", "factory", "resolve input port")
		}
		inputs = append(inputs, port)
	}
	// Validate refused every declared output, so there are none to resolve.
	return config, inputs, nil, nil
}

// CreateGraphEmbedding is the factory function for creating graph-embedding components. It
// opens no connection.
func CreateGraphEmbedding(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphEmbedding", "factory", "NATSClient required")
	}
	config, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	// The store registry is admitted only by a declared store-read input (ADR-063).
	var storeRegistry *storeregistry.Registry
	for _, input := range inputs {
		facts, err := input.Facts()
		if err != nil {
			return nil, errs.WrapInvalid(err, "CreateGraphEmbedding", "factory", "project input port")
		}
		if _, ok := facts.StoreReadBucket(); ok {
			storeRegistry = deps.StoreRegistry
			break
		}
	}
	return &Component{
		config:        config,
		inputs:        inputs,
		outputs:       outputs,
		natsClient:    deps.NATSClient,
		logger:        deps.GetLoggerWithComponent(key),
		storeRegistry: storeRegistry,
	}, nil
}

// Register registers the graph-embedding factory with the component registry. It asks boot
// for no model registry: at tier 0 the embedder is BM25, which needs no provider.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph entity embedding generation processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphEmbedding,
		Ports:       DeclarePorts,
	})
}

// InputPorts returns the input ports, fixed at construction.
func (c *Component) InputPorts() []component.Port { return slices.Clone(c.inputs) }

// OutputPorts returns the output ports, fixed at construction.
func (c *Component) OutputPorts() []component.Port { return slices.Clone(c.outputs) }

// Health reports the shell's verdict (component.Lifecycle.Health).
func (c *Component) Health() component.HealthStatus { return c.lifecycle.Health() }

// DataFlow reports the shell's flow metrics.
func (c *Component) DataFlow() component.FlowMetrics { return c.lifecycle.DataFlow() }
