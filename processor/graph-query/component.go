package graphquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync/atomic"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// key names graph-query once: its registration, its logger and its rows in the verb table.
const key = graph.ResponderGraphQuery

// Component implements the graph-query processor.
type Component struct {
	config Config
	inputs []component.Port

	natsClient *natsclient.Client
	logger     *slog.Logger
	metrics    *queryMetrics

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle

	// subscriptions are the query request subscriptions, one per verb the table in graph declares
	// for graph-query. Start records them and cleanup clears them once it has drained them all.
	subscriptions []*natsclient.Subscription

	// labelPredicates is the label set, most salient first, from which the searches label their
	// matches. start sets it, before any handler is subscribed; handlers only read it.
	labelPredicates []string

	// communities is the view of COMMUNITY_INDEX localSearch and the text search read, nil while there is none. Only the
	// community supervisor stores it; handlers load it.
	communities atomic.Pointer[communityView]

	// runCancel ends the run context the community supervisor runs under, and supervised is closed
	// once the supervisor has returned, after it has set supervisorErr to what it returned. start
	// sets them and cleanup clears them; the shell runs the two one at a time.
	runCancel     context.CancelFunc
	supervised    chan struct{}
	supervisorErr error
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
	// embedding_enabled is on when left out: decoding leaves this true unless the key says false.
	config := Config{EmbeddingEnabled: true}
	if len(rawConfig) > 0 {
		// Strict decoding: a key Config does not define is refused, naming the key, where
		// encoding/json would drop it. A port definition decodes through its own UnmarshalJSON,
		// which is strict too.
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
	metrics, err := newQueryMetrics(deps.MetricsRegistry)
	if err != nil {
		return nil, errs.Wrap(err, "CreateGraphQuery", "factory", "register metrics")
	}
	return &Component{
		config:     config,
		inputs:     inputs,
		natsClient: deps.NATSClient,
		logger:     deps.GetLoggerWithComponent(key),
		metrics:    metrics,
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

// Health reports the shell's verdict (component.Lifecycle.Health). A missing or lost community view
// is not in it: localSearch answers without records and the text search refuses per request instead.
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

// Start begins the community view's supervisor and subscribes the verbs the table declares for
// graph-query. The lifecycle state is the shell's (component.Lifecycle): Start before Initialize, a
// nil or ended context and a second Start are refused, the second with errs.ErrAlreadyStarted, and a
// Start that fails is rolled back through cleanup. If that rollback fails too, Start reports both
// errors and a later Stop tries again. A component with no NATS client is refused first.
//
// Start neither waits for nor fails on a missing COMMUNITY_INDEX: until a view of it has caught up,
// localSearch answers with record_state not_ready, the text search refuses index_not_ready and the other
// verbs answer. ctx bounds the subscribing only and is not kept; the supervisor runs under the one run
// context, which Stop ends (ADR-113).
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.cleanup)
}

// start creates the run context, starts the community supervisor under it, and subscribes the verbs
// last, as graph-index does, so a failed subscription's rollback ends the supervisor too.
func (c *Component) start(ctx context.Context) error {
	// Read once, as graph-index reads it (processor/graph-index/index.go:88): the composition has
	// registered its predicates before Start.
	c.labelPredicates = labelOrder(vocabulary.DiscoverLabelPredicates())
	// The one run context: Start's values without its cancellation, ended only by cleanup.
	runCtx, runCancel := context.WithCancel(context.WithoutCancel(ctx))
	supervised := make(chan struct{})
	c.runCancel, c.supervised = runCancel, supervised
	go func() {
		defer close(supervised)
		c.supervisorErr = c.superviseCommunities(runCtx)
	}()
	return c.subscribeQueries(ctx)
}

// Stop stops the component through the shell, which runs cleanup: Stop before Start does nothing,
// and any non-nil Stop on a started component initiates every teardown step, bounds drain/join
// waits by ctx, and keeps unjoined handles for the next Stop if ctx ends first.
func (c *Component) Stop(ctx context.Context) error {
	return c.lifecycle.Stop(ctx)
}

// cleanup releases what start acquired, for a Stop or a failed Start's rollback, tolerating a
// partial start. It initiates every subscription drain and ends the run context without returning
// early on an earlier ctx error, then waits up to ctx for the community view supervisor. A ctx
// that ends before all joins complete returns its error with the handles kept, and a repeated
// Drain rejoins the first. A supervisor whose view could not stop its watch is reported once, by
// the Stop that joined all handles.
func (c *Component) cleanup(ctx context.Context) error {
	var cleanupErr error
	for _, sub := range c.subscriptions {
		cleanupErr = errors.Join(cleanupErr, sub.Drain(ctx))
	}
	if c.runCancel != nil {
		c.runCancel()
	}
	if c.supervised != nil {
		select {
		case <-c.supervised:
		case <-ctx.Done():
			cleanupErr = errors.Join(cleanupErr, ctx.Err())
		}
	}
	if err := errors.Join(cleanupErr, ctx.Err()); err != nil {
		return err
	}
	// Read after supervised closed. A view that could not stop its watch is reported by this Stop
	// only: the supervisor has returned and there is nothing to retry, so the handles are cleared
	// and the next Stop returns nil: returning it again would fail every later Stop.
	err := c.supervisorErr
	c.subscriptions, c.runCancel, c.supervised, c.supervisorErr = nil, nil, nil, nil
	if err != nil {
		return fmt.Errorf("graph-query: stop the community view: %w", err)
	}
	return nil
}
