// Package graphindex provides the graph-index component for maintaining graph relationship indexes.
package graphindex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/metric"
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

	natsClient      *natsclient.Client
	logger          *slog.Logger
	metricsRegistry *metric.MetricsRegistry

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle,
	// ADR-108, design D4).
	lifecycle component.Lifecycle

	metrics *indexMetrics

	// The index write path (index.go). The buckets and the ENTITY_STATES reader are set by
	// openIndexBuckets; follower, pool and coalescer by startIndexPool (nil coalescer when
	// coalesce_ms is 0).
	outgoingBucket  *natsclient.KVStore
	incomingBucket  *natsclient.KVStore
	aliasBucket     *natsclient.KVStore
	predicateBucket *natsclient.KVStore
	nameBucket      *natsclient.KVStore
	entities        kvcatalog.CatalogReader
	follower        atomic.Pointer[follower.Follower] // read by Health at any time
	pool            *dispatch.KeyedPool[indexWork]
	coalescer       *revisionCoalescer

	// The handles start records for cleanup, which clears them once it has released them all.
	// poolCancel ends the one run context (ADR-113), which the pool's lanes write under;
	// runCancel ends its child, which the follower's Run, the coalescer and the repair loop run
	// under. feeds closes once those three have returned.
	runCancel  context.CancelFunc
	poolCancel context.CancelFunc
	feeds      chan struct{}

	// Label and alias predicates, read from the vocabulary when the pool starts.
	aliasPredicates map[string]int
	namePredicates  map[string]int

	// failedEntities holds the entities whose required index writes did not all succeed
	// after retry; failedCount is its size, the follower's Failed. While it is above zero
	// the index withholds readiness. An entity leaves the set when it later indexes or
	// deletes cleanly.
	failedEntities sync.Map
	failedCount    atomic.Int64

	// lastProjections maps an entity ID to the projection last indexed for it, for the
	// reindex_events_total metric (#108's instrument): one string per entity.
	lastProjections sync.Map
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
		config:          config,
		inputs:          inputs,
		outputs:         outputs,
		natsClient:      deps.NATSClient,
		logger:          deps.GetLoggerWithComponent(key),
		metricsRegistry: deps.MetricsRegistry,
		metrics:         metrics,
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

// Health reports the shell's verdict (component.Lifecycle.Health) over the follower's stuck
// states: a latched reset, unresolved index writes, a lost feed. ErrorCount is reported and
// never lowers Healthy (ADR-109 Decision 2), so a write failure lowers it only until its repair.
func (c *Component) Health() component.HealthStatus {
	var stuck []string
	if f := c.follower.Load(); f != nil {
		stuck = f.Stuck()
	}
	return c.lifecycle.Health(stuck...)
}

// DataFlow reports input messages: one per delivered ENTITY_STATES entry, update or delete,
// with the delivered value's length, counted before coalescing (design D4). ErrorRate is
// the errors counted (Health's ErrorCount) per input.
func (c *Component) DataFlow() component.FlowMetrics {
	return c.lifecycle.DataFlow()
}

// Initialize marks the component initialized (no I/O); the factory validated the
// configuration.
func (c *Component) Initialize() error {
	return c.lifecycle.Initialize()
}

// Start begins following ENTITY_STATES and indexing it. The lifecycle state is the shell's
// (component.Lifecycle): Start before Initialize, a nil or ended context and a second Start
// are refused, the second with errs.ErrAlreadyStarted, and a Start that fails is rolled back
// through cleanup. A component with no NATS client is refused first.
//
// ctx bounds Start only: the wait for ENTITY_STATES and opening the buckets run under it,
// and a ctx that ends first fails Start, which then holds nothing. The follower's watch, the
// keyed pool, the coalescer and the repair loop run under a run context start creates and
// cleanup ends, so ending ctx after Start returned stops nothing (ADR-113).
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.cleanup)
}

// start acquires what Start owns, in the order the write path needs (notes/4.2.md): the
// follower, the index buckets, the pool, then the goroutines Stop joins. Nothing it acquires
// before the run context needs releasing.
func (c *Component) start(ctx context.Context) error {
	f, err := follower.Open(ctx, follower.Config{
		Client:          c.natsClient,
		Key:             key,
		MetricsRegistry: c.metricsRegistry,
		Logger:          c.logger,
		StartupAttempts: c.config.StartupAttempts,
		StartupInterval: time.Duration(c.config.StartupInterval) * time.Millisecond,
		Deliver:         c.deliver,
		Failed:          c.failedTotal,
	})
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "open the ENTITY_STATES follower")
	}
	if err := c.openIndexBuckets(ctx); err != nil {
		return err
	}
	// A Start context that ended during setup fails Start here, before anything is held
	// (ADR-113).
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "Start", "context ended during startup")
	}

	// The one run context: Start's values without its cancellation, ended only by cleanup.
	// The lanes write under it; the feeds run under a child that cleanup ends first, so the
	// pool drains accepted work while its own context is still live.
	poolCtx, poolCancel := context.WithCancel(context.WithoutCancel(ctx))
	runCtx, runCancel := context.WithCancel(poolCtx)
	c.poolCancel, c.runCancel = poolCancel, runCancel
	if err := c.startIndexPool(poolCtx, f); err != nil {
		return err
	}

	feeds := make(chan struct{})
	c.feeds = feeds
	var wg sync.WaitGroup
	wg.Go(func() {
		if err := f.Run(runCtx); err != nil {
			c.logger.Warn("ENTITY_STATES watch did not stop cleanly", slog.Any("error", err))
		}
	})
	if coalescer := c.coalescer; coalescer != nil {
		wg.Go(func() { coalescer.run(runCtx) })
	}
	wg.Go(func() { c.repairLoop(runCtx) })
	go func() {
		wg.Wait()
		close(feeds)
	}()
	return nil
}

// Stop stops the component through the shell, which runs cleanup: Stop before Start does
// nothing, and a Stop whose context ends first returns that context's error and keeps what
// is left for the next Stop.
func (c *Component) Stop(ctx context.Context) error {
	return c.lifecycle.Stop(ctx)
}

// cleanup releases what start acquired, for a Stop or a failed Start's rollback, tolerating
// a partial start. The order is graph-ingest's (design D4): end the feeds' context and join
// them, so nothing new is submitted; drain the pool with Shutdown while the run context is
// still live, so work it accepted is written; then end the run context. A ctx that ends
// first returns its error with the handles kept, and the next Stop resumes; the handles are
// cleared only once everything is released.
func (c *Component) cleanup(ctx context.Context) error {
	if c.runCancel != nil {
		c.runCancel()
	}
	if c.feeds != nil {
		select {
		case <-c.feeds:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if c.pool != nil {
		if err := c.pool.Shutdown(ctx); err != nil {
			return err
		}
	}
	if c.poolCancel != nil {
		c.poolCancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.runCancel, c.poolCancel, c.feeds, c.pool, c.coalescer = nil, nil, nil, nil, nil
	return nil
}
