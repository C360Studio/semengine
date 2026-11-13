package graphlexical

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// key names graph-lexical once: its registration, its logger and its GRAPH_STATUS key.
const key = readiness.KeyGraphLexical

var _ component.Discoverable = (*Component)(nil)

// Component implements the graph-lexical processor.
type Component struct {
	config Config
	inputs []component.Port

	natsClient      *natsclient.Client
	logger          *slog.Logger
	metricsRegistry *metric.MetricsRegistry
	metrics         *lexicalMetrics

	// rule is the text rule, built once at construction, when the vocabulary's label predicates
	// are read.
	rule entitytext.Rule
	// stores finds the store an offloaded body names; nil when the dependencies carry no store
	// registry, so every body has no store.
	stores entitytext.Stores

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle

	// start sets the follower, the applier and the body path before the follower's Run begins.
	follower atomic.Pointer[follower.Follower] // read by Health at any time
	apply    *applier
	bodies   *bodyPath

	// The handles start records for cleanup, which clears them once it has released them all.
	// runCancel ends the one run context (ADR-113) the follower's Run and the body path run
	// under; stopped closes once both have returned.
	runCancel context.CancelFunc
	stopped   chan struct{}
}

// DeclarePorts is the component.PortDeclarer for graph-lexical: the ports CreateGraphLexical will
// report for rawConfig, computed without dependencies.
func DeclarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, nil), nil
}

// resolveConfig decodes rawConfig, applies defaults, validates, and resolves the input ports. It
// is the one derivation DeclarePorts and CreateGraphLexical share. Validate refused every declared
// output, so there are none to resolve.
func resolveConfig(rawConfig json.RawMessage) (Config, []component.Port, error) {
	config, err := decodeConfig(rawConfig)
	if err != nil {
		return Config{}, nil, err
	}
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, nil, errs.Wrap(err, "CreateGraphLexical", "factory", "config validation")
	}
	inputs := make([]component.Port, 0, len(config.Ports.Inputs))
	for _, definition := range config.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return Config{}, nil, errs.WrapInvalid(err, "CreateGraphLexical", "factory", "resolve input port")
		}
		inputs = append(inputs, port)
	}
	return config, inputs, nil
}

// CreateGraphLexical is the factory function for graph-lexical components. It opens no
// connection. It reads the vocabulary's label predicates, so a product registers its own first.
// Offloaded bodies are read from the stores in deps.StoreRegistry, looked up at each read.
func CreateGraphLexical(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphLexical", "factory", "NATSClient required")
	}
	config, inputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	metrics, err := newLexicalMetrics(deps.MetricsRegistry)
	if err != nil {
		return nil, errs.Wrap(err, "CreateGraphLexical", "factory", "register metrics")
	}
	// A nil *storeregistry.Registry in the interface would be a non-nil Stores that panics.
	var stores entitytext.Stores
	if deps.StoreRegistry != nil {
		stores = deps.StoreRegistry
	}
	return &Component{
		config:          config,
		inputs:          inputs,
		natsClient:      deps.NATSClient,
		logger:          deps.GetLoggerWithComponent(key),
		metricsRegistry: deps.MetricsRegistry,
		metrics:         metrics,
		rule:            entitytext.New(config.TextSuffixes, config.MaxTextLen),
		stores:          stores,
	}, nil
}

// Register registers the graph-lexical factory with the component registry. It asks boot for no
// model registry: the word index needs no provider at any tier.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "In-memory word index over entity text, rebuilt from ENTITY_STATES on every start",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphLexical,
		Ports:       DeclarePorts,
	})
}

// InputPorts returns the input ports, fixed at construction.
func (c *Component) InputPorts() []component.Port { return slices.Clone(c.inputs) }

// OutputPorts returns no ports: the index lives in the component's memory.
func (c *Component) OutputPorts() []component.Port { return nil }

// Health reports the shell's verdict (component.Lifecycle.Health) over the follower's stuck
// states: a latched reset or a lost feed.
func (c *Component) Health() component.HealthStatus {
	var stuck []string
	if f := c.follower.Load(); f != nil {
		stuck = f.Stuck()
	}
	return c.lifecycle.Health(stuck...)
}

// DataFlow reports input messages: one per delivered ENTITY_STATES entry, update or delete, with
// the delivered value's length.
func (c *Component) DataFlow() component.FlowMetrics { return c.lifecycle.DataFlow() }

// Initialize marks the component initialized (no I/O); the factory validated the configuration.
func (c *Component) Initialize() error { return c.lifecycle.Initialize() }

// Start begins following ENTITY_STATES into a new, empty index, so every start rebuilds it. The
// lifecycle state is the shell's (component.Lifecycle): Start before Initialize, a nil or ended
// context and a second Start are refused, the second with errs.ErrAlreadyStarted, and a Start that
// fails is rolled back through cleanup. A component with no NATS client is refused first.
//
// ctx bounds Start only: the wait for ENTITY_STATES runs under it. The follower's watch runs
// under a run context start creates and cleanup ends, so ending ctx after Start returned stops
// nothing (ADR-113).
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.cleanup)
}

// start opens the follower and builds the applier, then starts what Stop joins. Nothing it
// acquires before the run context needs releasing.
func (c *Component) start(ctx context.Context) error {
	f, err := follower.Open(ctx, follower.Config{
		Client:          c.natsClient,
		Key:             key,
		MetricsRegistry: c.metricsRegistry,
		Logger:          c.logger,
		StartupAttempts: c.config.StartupAttempts,
		StartupInterval: time.Duration(c.config.StartupInterval) * time.Millisecond,
		Deliver:         c.deliver,
		Failed:          c.failed,
		// No body state withholds the verb: an unread body reads degraded and the rest answer.
		Withhold: func() int64 { return 0 },
	})
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "open the ENTITY_STATES follower")
	}
	// A Start context that ended during setup fails Start here, before anything is held
	// (ADR-113).
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "Start", "context ended during startup")
	}
	c.apply = newApplier(c.rule, c.config.MaxTextLen, f.Complete, c.metrics, c.logger)
	c.bodies = newBodyPath(c.apply, c.stores, c.config.MaxTextLen, c.logger)
	c.follower.Store(f)

	// The one run context: Start's values without its cancellation, ended only by cleanup.
	runCtx, runCancel := context.WithCancel(context.WithoutCancel(ctx))
	c.runCancel = runCancel
	stopped := make(chan struct{})
	c.stopped = stopped
	go func() {
		defer close(stopped)
		var bodies sync.WaitGroup
		bodies.Go(func() { c.bodies.run(runCtx) })
		defer bodies.Wait()
		if err := f.Run(runCtx); err != nil {
			c.logger.Warn("ENTITY_STATES watch did not stop cleanly", slog.Any("error", err))
		}
	}()
	return nil
}

// failed is the follower's Failed, the applier's unread bodies. The follower is published to
// Health, and runs, only after start has built the applier.
func (c *Component) failed() (int64, map[string]uint64) {
	return c.apply.failed()
}

// deliver is the follower's Deliver, on its Run goroutine. It does no I/O: the applier puts the
// entry's inline text and completes its revision, or names the offloaded body, which goes to the
// body path to read. A value that is not a canonical entity state latches the follower's reset,
// and its revision stays pending.
func (c *Component) deliver(_ context.Context, e follower.Entry) {
	c.lifecycle.CountInput(e.Bytes)
	read, ok, err := c.apply.deliver(e)
	if err != nil {
		reason := graph.GraphStateReasonUnreadableEntity
		var stateErr *graph.StateContractError
		if errors.As(err, &stateErr) {
			reason = stateErr.Reason
		}
		c.logger.Error("entity state refused; graph reset required",
			slog.String("entity", e.Key), slog.Any("error", err))
		c.lifecycle.CountError()
		c.follower.Load().Reset(reason)
		return
	}
	if ok {
		c.bodies.enqueue(read)
	}
}

// Stop stops the component through the shell, which runs cleanup: Stop before Start does nothing,
// and a Stop whose context ends first returns that context's error and keeps what is left for the
// next Stop.
func (c *Component) Stop(ctx context.Context) error { return c.lifecycle.Stop(ctx) }

// cleanup releases what start acquired, for a Stop or a failed Start's rollback, tolerating a
// partial start: it ends the run context and joins the follower's Run and the body path, so
// nothing more is delivered or read. A read in flight ends by its own deadline at the latest. A ctx that ends first returns its error with the handles kept, and the next Stop
// resumes.
func (c *Component) cleanup(ctx context.Context) error {
	if c.runCancel != nil {
		c.runCancel()
	}
	if c.stopped != nil {
		select {
		case <-c.stopped:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.runCancel, c.stopped = nil, nil
	return nil
}
