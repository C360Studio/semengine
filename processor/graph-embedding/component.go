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
	"github.com/c360studio/semengine/graph/entitytext"
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
// run context was live: nothing embeds a pending record until the process restarts.
const workerWatchLost = "embedding worker watch lost; restart the process"

// vectorCacheWatchLost is the stuck state Health names once the vector cache's Run has returned
// while the run context was live: the verbs refuse until the process restarts.
const vectorCacheWatchLost = "vector cache watch lost; restart the process"

// Component implements the graph-embedding processor.
type Component struct {
	config  Config
	inputs  []component.Port
	outputs []component.Port

	natsClient      *natsclient.Client
	logger          *slog.Logger
	metricsRegistry *metric.MetricsRegistry
	metrics         *embeddingMetrics

	// text selects the values hop 1 embeds, from text_suffixes.
	text entitytext.Rule

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
	// delivered is the highest revision the follower has handed deliver; a read that finds no
	// value completes the key only up to it (readFailed). start zeroes it for each watch.
	delivered atomic.Uint64

	// offloadExcludedWarn logs the first offloaded body left out for want of its store.
	offloadExcludedWarn sync.Once

	// workerLost latches when the Worker's Run returns while the run context is live; nothing
	// rebinds the watch, as for the follower's lost feed.
	workerLost atomic.Bool
	// cacheLost latches when the vector cache's Run returns while the run context is live; the
	// verbs refuse from then on.
	cacheLost atomic.Bool

	// embedderType is the name the product registered its embedder under: the type part of the
	// dedup key and search's embedder_type. newEmbedder builds the embedder; start calls it.
	embedderType string
	newEmbedder  func() (embedding.Embedder, error)
	// embedder is the one embedder start built: the Worker embeds documents with it and the search
	// verb embeds queries with it (GenerateQuery). cleanup closes it last and clears it once
	// closed. cache is the in-memory view of the generated vectors in EMBEDDING_INDEX the verbs
	// rank; start sets it.
	embedder embedding.Embedder
	cache    *embedding.VectorCache
	// subscriptions are the query request subscriptions, one per verb the table in graph declares
	// for graph-embedding; cleanup drains them first.
	subscriptions []*natsclient.Subscription

	// failed is the failed map (failed.go): seeded at Start, changed by the Worker's
	// terminal outcomes and hop 1, read by the follower through Failed and by the repair lane.
	failed failedMap

	// The handles start records for cleanup, which clears them once it has released them
	// all. runCancel ends the one run context (ADR-113) the follower's Run, the repair lane,
	// the Worker's Run and the coalescer run under; feeds closes once the follower's Run has
	// returned, repairs once the repair lane has, worked once the Worker's Run has, cached once
	// the vector cache's Run has.
	runCancel context.CancelFunc
	feeds     chan struct{}
	repairs   chan struct{}
	worked    chan struct{}
	cached    chan struct{}
}

// DeclarePorts is the component.PortDeclarer for graph-embedding: the ports
// the factory will report for rawConfig, computed without dependencies.
func DeclarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, outputs), nil
}

// resolveConfig parses rawConfig, applies defaults, validates, and resolves the effective
// ports. It is the one derivation DeclarePorts and the factory share.
func resolveConfig(rawConfig json.RawMessage) (Config, []component.Port, []component.Port, error) {
	var config Config
	if len(rawConfig) > 0 {
		// Strict decoding: a key Config does not define is refused, naming the key, where
		// encoding/json would drop it. A port definition decodes through its own UnmarshalJSON.
		decoder := json.NewDecoder(bytes.NewReader(rawConfig))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "graphembedding", "factory", "config unmarshal")
		}
		// Data after the object is refused, as json.Unmarshal refuses it.
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Config{}, nil, nil, errs.WrapInvalid(errors.New("data after the configuration object"),
				"graphembedding", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, nil, nil, errs.Wrap(err, "graphembedding", "factory", "config validation")
	}
	inputs := make([]component.Port, 0, len(config.Ports.Inputs))
	for _, definition := range config.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "graphembedding", "factory", "resolve input port")
		}
		inputs = append(inputs, port)
	}
	// Validate refused every declared output, so there are none to resolve.
	return config, inputs, nil, nil
}

// refuseWithoutEmbedder is the factory Register adds. It always refuses, class invalid:
// graph-embedding has no embedder of its own, so a product with one (tier 1, such as semembed)
// registers it with RegisterWithEmbedder, and a tier 0 composition leaves graph-embedding out.
func refuseWithoutEmbedder(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
	return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "graphembedding", "factory",
		"embedder check: graph-embedding has no embedder of its own (from tier 1 a product registers one with RegisterWithEmbedder)")
}

// newFactory returns the factory RegisterWithEmbedder adds: it builds graph-embedding with the
// product's embedder constructor, which only Start calls. It opens no connection.
func newFactory(embedderType string, newEmbedder func() (embedding.Embedder, error)) component.Factory {
	return func(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
		return create(rawConfig, deps, embedderType, newEmbedder)
	}
}

func create(rawConfig json.RawMessage, deps component.Dependencies, embedderType string,
	newEmbedder func() (embedding.Embedder, error)) (component.Discoverable, error) {
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "graphembedding", "factory", "NATSClient required")
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
			return nil, errs.WrapInvalid(err, "graphembedding", "factory", "project input port")
		}
		if _, ok := facts.StoreReadBucket(); ok {
			storeRegistry = deps.StoreRegistry
			break
		}
	}
	metrics, err := newEmbeddingMetrics(deps.MetricsRegistry)
	if err != nil {
		return nil, errs.Wrap(err, "graphembedding", "factory", "register metrics")
	}
	return &Component{
		config:          config,
		inputs:          inputs,
		outputs:         outputs,
		natsClient:      deps.NATSClient,
		logger:          deps.GetLoggerWithComponent(key),
		metricsRegistry: deps.MetricsRegistry,
		metrics:         metrics,
		text:            entitytext.New(config.TextSuffixes, config.MaxTextLen),
		storeRegistry:   storeRegistry,
		embedderType:    embedderType,
		newEmbedder:     newEmbedder,
	}, nil
}

// Register registers a graph-embedding factory that refuses construction, naming
// RegisterWithEmbedder and tier 1: graph-embedding has no embedder of its own. It is the
// registration every component package exports, so a composition that lists graph-embedding
// without an embedder fails at construction rather than running without one.
func Register(registry *component.Registry) error {
	return register(registry, refuseWithoutEmbedder)
}

// RegisterWithEmbedder registers graph-embedding with a product's embedder. embedderType names
// the embedder: it is the type part of the dedup key, so two embedders never share a stored
// vector, and search replies it as embedder_type. newEmbedder is called once per Start, never at
// construction, and the embedder it returns is closed by Stop. An empty embedderType or a nil
// newEmbedder is refused, class invalid, and nothing is registered. Registering asks boot for no
// model registry: the product owns the embedder and whatever it connects to.
func RegisterWithEmbedder(registry *component.Registry, embedderType string,
	newEmbedder func() (embedding.Embedder, error)) error {
	if embedderType == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "graphembedding", "RegisterWithEmbedder",
			"embedder type is required")
	}
	if newEmbedder == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "graphembedding", "RegisterWithEmbedder",
			"embedder constructor is required")
	}
	return register(registry, newFactory(embedderType, newEmbedder))
}

func register(registry *component.Registry, factory component.Factory) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph entity embedding generation processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     factory,
		Ports:       DeclarePorts,
	})
}

// InputPorts returns the input ports, fixed at construction.
func (c *Component) InputPorts() []component.Port { return slices.Clone(c.inputs) }

// OutputPorts returns the output ports, fixed at construction.
func (c *Component) OutputPorts() []component.Port { return slices.Clone(c.outputs) }

// Health reports the shell's verdict (component.Lifecycle.Health) over the follower's stuck
// states (damaged entities, unresolved writes, a lost feed), a lost Worker watch and a lost vector
// cache watch.
func (c *Component) Health() component.HealthStatus {
	var stuck []string
	if f := c.follower.Load(); f != nil {
		stuck = f.Stuck()
	}
	if c.workerLost.Load() {
		stuck = append(stuck, workerWatchLost)
	}
	if c.cacheLost.Load() {
		stuck = append(stuck, vectorCacheWatchLost)
	}
	return c.lifecycle.Health(stuck...)
}

// identity is the running embedder's identity as the Worker folds it into the dedup key: the
// registered type, the embedder's live Model and Dimensions, and the text cap. It is read on
// each call, because a remote embedder resolves its width from its first response.
func (c *Component) identity() embedding.EmbedderIdentity {
	return embedding.IdentityOf(c.embedderType, c.embedder, c.config.MaxTextLen)
}

// DataFlow reports input messages: one per delivered ENTITY_STATES entry, update or delete,
// with the delivered value's length, counted before coalescing.
func (c *Component) DataFlow() component.FlowMetrics { return c.lifecycle.DataFlow() }

// Initialize marks the component initialized (no I/O); the factory validated the
// configuration.
func (c *Component) Initialize() error { return c.lifecycle.Initialize() }

// Start builds the embedder, then begins following ENTITY_STATES and writing pending records.
// The lifecycle state is the shell's (component.Lifecycle): Start before Initialize, a nil or ended context and a
// second Start are refused, the second with errs.ErrAlreadyStarted, and a Start that fails is
// rolled back through cleanup. A component with no NATS client is refused first.
//
// ctx bounds Start only: the wait for ENTITY_STATES, opening the buckets and seeding the failed
// map run under it. The follower's watch, the repair lane, the Worker (hop 2) and the coalescer
// run under a run context start creates and cleanup ends, so ending ctx after Start returned
// stops nothing (ADR-113).
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.cleanup)
}

// start builds the embedder, opens the follower and the buckets, then starts what Stop joins.
// Of what it acquires before the run context, only the embedder needs releasing; cleanup closes
// it when a later step fails.
func (c *Component) start(ctx context.Context) error {
	embedder, err := c.newEmbedder()
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "build the embedder")
	}
	if embedder == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Component", "Start", "the embedder constructor returned no embedder")
	}
	c.embedder = embedder

	f, err := follower.Open(ctx, follower.Config{
		Client:          c.natsClient,
		Key:             key,
		MetricsRegistry: c.metricsRegistry,
		Logger:          c.logger,
		StartupAttempts: c.config.StartupAttempts,
		StartupInterval: time.Duration(c.config.StartupInterval) * time.Millisecond,
		Deliver:         c.deliver,
		Failed:          c.failed.counts,
		// Only hop 1's failures, which can leave a stale vector in the index, withhold the verbs;
		// a Worker failure leaves no vector and is answered past.
		Withhold: c.failed.withheld,
	})
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "open the ENTITY_STATES follower")
	}
	if err := c.openBuckets(ctx); err != nil {
		return err
	}
	// The failed records EMBEDDING_INDEX holds seed the failed map before the follower publishes,
	// so the first envelope counts them. A seed that cannot be read fails Start: a map left short
	// would let readiness read ready over failed records.
	failedRecords, err := c.storage.ScanFailed(ctx)
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "seed the failed map from "+graph.BucketEmbeddingIndex)
	}
	c.failed.seed(failedRecords)
	// A Start context that ended during setup fails Start here, before anything is held
	// (ADR-113).
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "Start", "context ended during startup")
	}
	c.delivered.Store(0)
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
	repairs := make(chan struct{})
	c.repairs = repairs
	go func() {
		defer close(repairs)
		c.repairLoop(runCtx)
	}()

	// Hop 2: the Worker embeds each pending record and reports its terminal outcome to
	// onTerminal. Its embedder also embeds the search verb's queries, so documents and queries
	// share one vector space.
	worker := embedding.NewWorker(c.storage, c.embedder, c.logger).
		WithWorkers(c.config.Workers).
		WithMaxSourceTextLen(c.config.MaxTextLen).
		WithEmbedderType(c.embedderType).
		WithMetrics(c.metrics).
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

	// The vector cache the verbs answer from. Until it has applied its watch's first snapshot,
	// and after its watch is lost, the verbs refuse index_not_ready.
	c.cache = embedding.NewVectorCache(c.storage, c.identity)
	cached := make(chan struct{})
	c.cached = cached
	go func() {
		defer close(cached)
		if err := c.cache.Run(runCtx); runCtx.Err() == nil {
			// The latch precedes the log line, so a reader of the line reads the latch.
			c.cacheLost.Store(true)
			c.logger.Error("vector cache watch lost; the embedding verbs refuse until the process restarts",
				slog.Any("error", err))
		} else if err != nil && err != runCtx.Err() {
			c.logger.Warn("vector cache watch did not stop cleanly", slog.Any("error", err))
		}
	}()
	// Last, so a request is served only once everything a handler reads is set; a failed
	// subscription is the last step that can fail, and its rollback drains those made.
	return c.subscribeQueries(ctx)
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

// Stop stops the component through the shell, which runs cleanup: Stop before Start does
// nothing, and any non-nil Stop on a started component initiates every teardown step, bounds
// drain/join waits by ctx, and keeps unjoined handles for the next Stop if ctx ends first.
func (c *Component) Stop(ctx context.Context) error { return c.lifecycle.Stop(ctx) }

// cleanup releases what start acquired, for a Stop or a failed Start's rollback, tolerating a
// partial start. It initiates every query subscription drain, ends the run context, waits up to
// ctx for the follower's Run, repair lane, Worker's Run, and vector cache's Run, and initiates
// coalescer shutdown without returning early on an earlier ctx error. A ctx that ends before
// all joins complete returns its error with the handles kept, and the next Stop resumes. The
// embedder is closed last, once nothing that embeds can still run; a close that fails is
// returned with the embedder kept, and the next Stop closes it again.
func (c *Component) cleanup(ctx context.Context) error {
	var cleanupErr error
	for _, sub := range c.subscriptions {
		cleanupErr = errors.Join(cleanupErr, sub.Drain(ctx))
	}
	if c.runCancel != nil {
		c.runCancel()
	}
	var waitErr error
	for _, done := range []chan struct{}{c.feeds, c.repairs, c.worked, c.cached} {
		if done == nil {
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			if waitErr == nil {
				waitErr = ctx.Err()
			}
		}
	}
	if c.coalescer != nil {
		cleanupErr = errors.Join(cleanupErr, c.coalescer.Shutdown(ctx))
	}
	if err := errors.Join(cleanupErr, waitErr, ctx.Err()); err != nil {
		return err
	}
	c.runCancel, c.feeds, c.repairs, c.worked, c.cached, c.coalescer, c.subscriptions = nil, nil, nil, nil, nil, nil, nil
	if c.embedder != nil {
		if err := c.embedder.Close(); err != nil {
			return errs.Wrap(err, "Component", "Stop", "close the embedder")
		}
		c.embedder = nil
	}
	return nil
}
