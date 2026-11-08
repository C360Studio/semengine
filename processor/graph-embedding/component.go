package graphembedding

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
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/internal/cache"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// key names graph-embedding once: its registration, its logger and its GRAPH_STATUS key.
const key = readiness.KeyGraphEmbedding

var _ component.Discoverable = (*Component)(nil)

// workerWatchLost is the stuck state Health names once the Worker's Run has returned while the
// run context was live (design D2): nothing embeds a pending record until the process restarts.
const workerWatchLost = "embedding worker watch lost; restart the process"

// Component implements the graph-embedding processor.
type Component struct {
	config  Config
	inputs  []component.Port
	outputs []component.Port

	natsClient      *natsclient.Client
	logger          *slog.Logger
	metricsRegistry *metric.MetricsRegistry
	metrics         *embeddingMetrics

	// storeRegistry resolves an offloaded body's StorageRef to its store (ADR-063); it is the
	// injected registry when a store-read input is declared and nil otherwise.
	storeRegistry *storeregistry.Registry

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle

	// Hop 1 (hop1.go). start sets the follower, the storage, the ENTITY_STATES reader and,
	// when coalesce_ms is above zero, the coalescer. hop1Mu serialises every hop-1 mutation.
	follower  atomic.Pointer[follower.Follower] // read by Health at any time
	storage   *embedding.Storage
	entities  kvcatalog.CatalogReader
	coalescer *cache.CoalescingSet
	hop1Mu    sync.Mutex

	// offloadExcludedWarn logs the first offloaded body left out for want of its store.
	offloadExcludedWarn sync.Once

	// workerLost latches when the Worker's Run returns while the run context is live; nothing
	// rebinds the watch, as for the follower's lost feed (design D2).
	workerLost atomic.Bool

	// The handles start records for cleanup, which clears them once it has released them
	// all. runCancel ends the one run context (ADR-113) the follower's Run, the Worker's Run
	// and the coalescer run under; feeds closes once the follower's Run has returned, worked
	// once the Worker's has.
	runCancel context.CancelFunc
	feeds     chan struct{}
	worked    chan struct{}
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
	metrics, err := newEmbeddingMetrics(deps.MetricsRegistry)
	if err != nil {
		return nil, errs.Wrap(err, "CreateGraphEmbedding", "factory", "register metrics")
	}
	return &Component{
		config:          config,
		inputs:          inputs,
		outputs:         outputs,
		natsClient:      deps.NATSClient,
		logger:          deps.GetLoggerWithComponent(key),
		metricsRegistry: deps.MetricsRegistry,
		metrics:         metrics,
		storeRegistry:   storeRegistry,
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

// Health reports the shell's verdict (component.Lifecycle.Health) over the follower's stuck
// states (a latched reset, unresolved writes, a lost feed) and a lost Worker watch.
func (c *Component) Health() component.HealthStatus {
	var stuck []string
	if f := c.follower.Load(); f != nil {
		stuck = f.Stuck()
	}
	if c.workerLost.Load() {
		stuck = append(stuck, workerWatchLost)
	}
	return c.lifecycle.Health(stuck...)
}

// DataFlow reports input messages: one per delivered ENTITY_STATES entry, update or delete,
// with the delivered value's length, counted before coalescing.
func (c *Component) DataFlow() component.FlowMetrics { return c.lifecycle.DataFlow() }

// Initialize marks the component initialized (no I/O); the factory validated the
// configuration.
func (c *Component) Initialize() error { return c.lifecycle.Initialize() }

// Start begins following ENTITY_STATES and writing pending records. The lifecycle state is
// the shell's (component.Lifecycle): Start before Initialize, a nil or ended context and a
// second Start are refused, the second with errs.ErrAlreadyStarted, and a Start that fails is
// rolled back through cleanup. A component with no NATS client is refused first.
//
// ctx bounds Start only: the wait for ENTITY_STATES and opening the buckets run under it.
// The follower's watch, the Worker (hop 2) and the coalescer run under a run context start
// creates and cleanup ends, so ending ctx after Start returned stops nothing (ADR-113).
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.cleanup)
}

// start opens the follower and the buckets, then starts what Stop joins. Nothing it acquires
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
	if err := c.openBuckets(ctx); err != nil {
		return err
	}
	// A Start context that ended during setup fails Start here, before anything is held
	// (ADR-113).
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "Start", "context ended during startup")
	}
	c.follower.Store(f)

	// The one run context: Start's values without its cancellation, ended only by cleanup.
	runCtx, runCancel := context.WithCancel(context.WithoutCancel(ctx))
	c.runCancel = runCancel
	if c.config.CoalesceMs > 0 {
		coalescer, err := cache.NewCoalescingSet(runCtx, time.Duration(c.config.CoalesceMs)*time.Millisecond,
			func(keys []string) { c.reconcileBatch(runCtx, keys) }, cache.WithPanicLogger(c.logger),
			cache.WithCoalescingMetrics(c.metricsRegistry, key))
		if err != nil {
			return errs.Wrap(err, "Component", "Start", "build the coalescer")
		}
		c.coalescer = coalescer
	}
	feeds := make(chan struct{})
	c.feeds = feeds
	go func() {
		defer close(feeds)
		if err := f.Run(runCtx); err != nil {
			c.logger.Warn("ENTITY_STATES watch did not stop cleanly", slog.Any("error", err))
		}
	}()

	// Hop 2: the Worker embeds each pending record and reports its terminal outcome to
	// onTerminal. It is launched last, so no failure in start follows it.
	worker := embedding.NewWorker(c.storage, embedding.NewBM25Embedder(embedding.BM25Config{}), c.logger).
		WithWorkers(c.config.Workers).
		WithMaxSourceTextLen(c.config.MaxTextLen).
		WithEmbedderType(c.config.EmbedderType).
		WithOnTerminal(c.onTerminal)
	// A nil *storeregistry.Registry in the interface would be a non-nil resolver that panics.
	if c.storeRegistry != nil {
		worker = worker.WithStoreResolver(c.storeRegistry)
	}
	worked := make(chan struct{})
	c.worked = worked
	go func() {
		defer close(worked)
		err := worker.Run(runCtx)
		// The run context decides, not Run's error: any return while it is live is a lost
		// watch. The latch precedes the log line, so a reader of the line reads the latch.
		if runCtx.Err() == nil {
			c.workerLost.Store(true)
			c.logger.Error("embedding worker watch lost; pending records are not embedded until the process restarts",
				slog.Any("error", err))
			return
		}
		// A clean stop returns exactly the run context's error.
		if err != nil && err != runCtx.Err() {
			c.logger.Warn("EMBEDDING_INDEX watch did not stop cleanly", slog.Any("error", err))
		}
	}()
	return nil
}

// openBuckets acquires EMBEDDING_INDEX and EMBEDDING_DEDUP as their owner, and the read handle
// on ENTITY_STATES hop 1 reads current state from.
func (c *Component) openBuckets(ctx context.Context) error {
	index, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketEmbeddingIndex)
	if err != nil {
		return errs.Wrap(err, "Component", "openBuckets", "KV bucket: "+graph.BucketEmbeddingIndex)
	}
	dedup, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketEmbeddingDedup)
	if err != nil {
		return errs.Wrap(err, "Component", "openBuckets", "KV bucket: "+graph.BucketEmbeddingDedup)
	}
	entities, err := kvcatalog.OpenCatalogReader(ctx, c.natsClient, graph.BucketEntityStates)
	if err != nil {
		return errs.Wrap(err, "Component", "openBuckets", "KV bucket: "+graph.BucketEntityStates)
	}
	c.storage = embedding.NewStorage(c.natsClient.NewKVStore(index), c.natsClient.NewKVStore(dedup))
	c.entities = entities
	return nil
}

// failedTotal is the follower's Failed.
//
// shortcut: always 0 until task 2.4 of setup-04a-05-first-consumer-graph-embedding adds the
// failed map; until then a failure hop 1 meets is logged and not counted.
func (c *Component) failedTotal() int64 { return 0 }

// Stop stops the component through the shell, which runs cleanup: Stop before Start does
// nothing, and a Stop whose context ends first returns that context's error and keeps what is
// left for the next Stop.
func (c *Component) Stop(ctx context.Context) error { return c.lifecycle.Stop(ctx) }

// cleanup releases what start acquired, for a Stop or a failed Start's rollback, tolerating a
// partial start: it ends the run context, joins the follower's Run, so nothing more is
// delivered, and the Worker's Run, so no terminal outcome follows, then shuts the coalescer
// down, which waits for a batch in progress. A ctx that ends first returns its error with the
// handles kept, and the next Stop resumes.
func (c *Component) cleanup(ctx context.Context) error {
	if c.runCancel != nil {
		c.runCancel()
	}
	for _, done := range []chan struct{}{c.feeds, c.worked} {
		if done == nil {
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if c.coalescer != nil {
		if err := c.coalescer.Shutdown(ctx); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.runCancel, c.feeds, c.worked, c.coalescer = nil, nil, nil, nil
	return nil
}
