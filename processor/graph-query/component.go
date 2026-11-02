package graphquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// key names graph-query once: its registration, its logger and its rows in the verb table.
const key = graph.ResponderGraphQuery

// Component implements the graph-query processor.
type Component struct {
	config Config
	inputs []component.Port

	natsClient *natsclient.Client
	logger     *slog.Logger

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle

	// subscriptions are the query request subscriptions, one per verb the table in graph declares
	// for graph-query. Start records them and cleanup clears them once it has drained them all.
	subscriptions []*natsclient.Subscription
}

// DeclarePorts is the component.PortDeclarer for graph-query: the ports CreateGraphQuery will
// report for rawConfig, computed without dependencies.
func DeclarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, nil), nil
}

// resolveConfig parses rawConfig, applies defaults, validates, and resolves the one input port. It
// is the one derivation DeclarePorts and CreateGraphQuery share.
func resolveConfig(rawConfig json.RawMessage) (Config, []component.Port, error) {
	var config Config
	if len(rawConfig) > 0 {
		// Strict decoding: a key Config does not define is refused, naming the key, where
		// encoding/json would drop it, as the pin's did (design D7). A port definition decodes
		// through its own UnmarshalJSON, which is strict too.
		decoder := json.NewDecoder(bytes.NewReader(rawConfig))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return Config{}, nil, errs.WrapInvalid(err, "CreateGraphQuery", "factory", "config unmarshal")
		}
		// DisallowUnknownFields reads one value; json.Unmarshal would refuse what follows it.
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Config{}, nil, errs.WrapInvalid(errors.New("data after the configuration object"),
				"CreateGraphQuery", "factory", "config unmarshal")
		}
	}
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, nil, errs.Wrap(err, "CreateGraphQuery", "factory", "config validation")
	}
	input, err := config.Ports.Inputs[0].Resolve(component.DirectionInput)
	if err != nil {
		return Config{}, nil, errs.WrapInvalid(err, "CreateGraphQuery", "factory", "resolve input port")
	}
	return config, []component.Port{input}, nil
}

// CreateGraphQuery is the factory function for creating graph-query components. It opens no
// connection.
func CreateGraphQuery(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphQuery", "factory", "NATSClient required")
	}
	config, inputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	return &Component{
		config:     config,
		inputs:     inputs,
		natsClient: deps.NATSClient,
		logger:     deps.GetLoggerWithComponent(key),
	}, nil
}

// Register registers the graph-query factory with the component registry.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph query processor: answers the graph.query.* verbs",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphQuery,
		Ports:       DeclarePorts,
	})
}

// InputPorts returns a new slice of the input ports. The ports share their Interface pointer with
// the component; no code writes through it.
func (c *Component) InputPorts() []component.Port {
	return slices.Clone(c.inputs)
}

// OutputPorts returns no ports: graph-query has none.
func (c *Component) OutputPorts() []component.Port {
	return nil
}

// Health reports the shell's verdict (component.Lifecycle.Health). graph-query keeps no state of
// its own that could be stuck.
func (c *Component) Health() component.HealthStatus {
	return c.lifecycle.Health()
}

// DataFlow reports the shell's flow metrics (component.Lifecycle.DataFlow).
func (c *Component) DataFlow() component.FlowMetrics {
	return c.lifecycle.DataFlow()
}

// Initialize marks the component initialized (no I/O); the factory validated the configuration.
func (c *Component) Initialize() error {
	return c.lifecycle.Initialize()
}

// Start subscribes the verbs the table declares for graph-query. The lifecycle state is the shell's
// (component.Lifecycle): Start before Initialize, a nil or ended context and a second Start are
// refused, the second with errs.ErrAlreadyStarted, and a Start that fails is rolled back through
// cleanup. If that rollback fails too, Start reports both errors and a later Stop tries again. A
// component with no NATS client is refused first.
//
// ctx bounds the subscribing only and is not kept. Nothing outlives a handler, so graph-query has
// no run context (ADR-113).
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.subscribeQueries, c.cleanup)
}

// Stop stops the component through the shell, which runs cleanup: Stop before Start does nothing,
// and a Stop whose context ends first returns that context's error and keeps what is left for the
// next Stop.
func (c *Component) Stop(ctx context.Context) error {
	return c.lifecycle.Stop(ctx)
}

// cleanup drains every subscription, for a Stop or a failed Start's rollback, tolerating a partial
// start. Drain returns once every running handler of that subscription has returned. One handler
// that outlasts ctx does not keep the other verbs serving: each drain is begun whatever the one
// before returned. The handles are cleared only once all of them are released, and a repeated
// Drain rejoins the first.
func (c *Component) cleanup(ctx context.Context) error {
	var failures []error
	for _, sub := range c.subscriptions {
		failures = append(failures, sub.Drain(ctx))
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	c.subscriptions = nil
	return nil
}
