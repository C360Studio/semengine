// Package graphingest provides the graph-ingest component for entity and triple ingestion.
package graphingest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/internal/cache"
	"github.com/c360studio/semengine/internal/dispatch"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

// Ensure Component implements required interfaces
var (
	_ component.Discoverable        = (*Component)(nil)
	_ component.LifecycleComponent  = (*Component)(nil)
	_ component.DebugStatusProvider = (*Component)(nil)
)

// hierarchyStore is graph-ingest as hierarchy inference's inference.EntityStore:
// the create is the in-process create, which refuses a key the bucket holds with
// natsclient.ErrKVKeyExists.
type hierarchyStore struct {
	component *Component
}

func (a *hierarchyStore) CreateEntity(ctx context.Context, entity *graph.EntityState) error {
	return a.component.CreateEntity(ctx, entity)
}

// Component implements the graph-ingest processor
type Component struct {
	// Component metadata
	name    string
	config  Config
	inputs  []component.Port
	outputs []component.Port

	// Dependencies
	decoder *message.Decoder
	// payloadRegistry is the binary's single type authority (SemStreams ADR-103): the
	// create seams refuse a stamp it does not hold and the indexing-profile
	// floor is read from the registered type. It is a registry, not a context.
	payloadRegistry *payloadregistry.Registry
	natsClient      *natsclient.Client
	logger          *slog.Logger
	// Test-only acquisition seams. Production falls through to the exact Client
	// methods; no exported or adopter-facing lifecycle surface is introduced.
	waitForStreamInput func(context.Context, string) (time.Time, error)
	consumeStream      func(
		context.Context,
		natsclient.PortConsumerContext,
		natsclient.StreamConsumerConfig,
		func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error)
	// watchEntityStates stands in for the entity bucket's Watch in the boot sweep.
	watchEntityStates func(context.Context, string) (jetstream.KeyWatcher, error)
	// subscribeRequests stands in for the Client's SubscribeForRequests at the query and
	// mutation registrations (subscribeForRequests).
	subscribeRequests func(
		context.Context,
		string,
		func(context.Context, []byte) ([]byte, error),
	) (*natsclient.Subscription, error)

	// Domain resources
	entityBucket *natsclient.KVStore            // authoritative KV operations, snapshot watch, and CAS support
	entityCache  cache.Cache[graph.EntityState] // Read-through cache for query handlers

	// Inference components
	hierarchyInference *inference.HierarchyInference

	// mu guards the ports and the entityCache handle: each cache use reads the handle
	// once under mu and uses that copy, and clearLifecycleHandles clears it under mu, so
	// a use running during or after Stop holds a handle that stays usable (#306).
	mu sync.RWMutex
	// shell is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108);
	// Initialize, Start, Stop, Health and DataFlow go through it.
	lifecycle component.Lifecycle
	// startCompleted is set as start's last step and never cleared, since graph-ingest is
	// one-shot (D13). Request handlers refuse until it is set (subscribeForRequests).
	// The shell's running state cannot stand in: it ends when release begins, before
	// Stop's drain, which still serves.
	startCompleted atomic.Bool
	// The handles start records for release: release reads and clears them. Both run
	// under the lifecycle guard, one at a time, so no lock is needed (ruling I, #91
	// comment 6062681355). No other goroutine reads them: the status loop closes its own
	// statusDone, and each consumer callback holds the pool it was built with.
	statusDone chan struct{}
	cancel     context.CancelFunc

	// Readiness producer state (ADR-083 envelope on the graph-ingest GRAPH_STATUS
	// key). See readiness.go for the projection and the bootstrap scope rules.
	statusPublisher  *readiness.Publisher
	readinessGauges  *readiness.Gauges
	boundConsumers   []boundConsumer
	boundConsumersMu sync.RWMutex
	// bootBacklog is the outstanding count read at bind, summed over consumers —
	// the initial catch-up, published as BootstrapScope. bootBacklogKnown makes the
	// replacement of a partial capture once-only.
	bootBacklog      atomic.Uint64
	bootBacklogKnown atomic.Bool
	// bootBacklogPartial latches when ANY bind-time read failed, so the accumulated
	// partial is discarded in favour of the first whole observation. One global
	// "known" flag was not enough: a success on one consumer marked the aggregate
	// captured and permanently omitted a failed consumer's backlog.
	bootBacklogPartial atomic.Bool
	// backlogObservationFault is the latest status tick's failed backlog observation,
	// naming the consumers it could not read; nil when that tick read every one. Each
	// tick that runs to its end replaces it (computeBacklogStatus), and Health reads it.
	backlogObservationFault atomic.Pointer[string]
	// lastAppliedAt is the JetStream timestamp of the most recently APPLIED message
	// (stamped on the applied path, before the Ack). It ages the reported staleness.
	lastAppliedAt atomic.Value // stores time.Time

	// Per-entity poison inventory (poison-response-scoping D2/D3). Observability
	// only — no read or write path consults it for a decision; refusal derives
	// solely from decoding stored bytes at each seam. entityPoisonSize mirrors
	// len(entityPoison) so hot-path clear checks cost one atomic load when the
	// inventory is empty (design D2, required). See poison_inventory.go.
	entityPoisonMu   sync.Mutex
	entityPoison     map[string]entityPoisonRecord
	entityPoisonSize atomic.Int64

	// Entity-query-cache read-after-write coherence guard. cacheGen is a per-key
	// invalidation generation counter: invalidateEntityCacheEntry bumps the key's
	// generation and drops the cache entry ATOMICALLY under cacheGenMu, and a
	// cache-miss repopulating Set (repopulateEntityCacheEntry) applies only if the
	// key's generation is unchanged since the reader captured it (before its KV
	// Get). This closes the stale-repopulation race: a slow reader that loaded a
	// pre-write revision from KV cannot resurrect it into the cache after a
	// concurrent write invalidated the entry. One uint64 per key; the map holds
	// every distinct entity ID a write was ever attempted on, successful or not
	// (writeFailed invalidates after any errored write, so a request naming an
	// ID that never existed adds one too). It is not pruned, so it grows with
	// create/delete churn and with requests for made-up IDs. Removing or bounding
	// it is #300 (the entity cache question); pruning must exclude in-flight
	// reads to keep the guarantee the counter provides.
	cacheGenMu sync.Mutex
	cacheGen   map[string]uint64

	// platform is the deployment's own authority, the org and platform at
	// positions 1-2 of every identity this deployment may mint (ADR-102 d5). It is
	// deps.Platform, read once at construction, which refuses an empty pair, and
	// never re-derived from a candidate, a payload or a subject. The authority gate
	// and hierarchy inference both read it.
	platform types.PlatformMeta

	// Prometheus metrics. The historical exported series name remains stable
	// because dashboards and operator alerts consume it as an external contract.
	entitiesUpdated               prometheus.Counter
	indexingProfileDefault        *prometheus.CounterVec
	mutationRejections            *prometheus.CounterVec
	mutationOutcomes              *prometheus.CounterVec
	batchMissing                  *prometheus.CounterVec
	predicateContractRejections   *prometheus.CounterVec
	entityStateContractRejections *prometheus.CounterVec
	processingDuration            prometheus.Histogram   // SemStreams gh#480 per-message apply time (processing half)
	ingestLag                     prometheus.Histogram   // SemStreams gh#480 message age at processing start (queue-wait half)
	redeliveriesDropped           prometheus.Counter     // SemStreams ADR-072 stale redeliveries dropped by the applied-sequence guard
	guardRecordRefusals           prometheus.Counter     // inputs refused for an undecodable applied-sequence record (design D21)
	unguardedInputs               prometheus.Counter     // inputs applied as first seen with no applied-sequence bucket (design D8)
	workInterrupted               *prometheus.CounterVec // inputs whose application was interrupted and left unsettled, by cause (design D8)
	casRetries                    prometheus.Counter     // SemStreams ADR-072 entity-merge CAS-conflict retries (contention observability)
	staleSets                     prometheus.Counter     // stream-lane sets not applied as older (design D15, #98)
	duplicateTriplesSuppressed    *prometheus.CounterVec // append duplicates not stored, by lane (closed enum)
	poisonedEntities              prometheus.Gauge       // per-entity poison inventory size (single gauge, no per-entity labels)
	metricsRegistry               *metric.MetricsRegistry

	// Keyed-concurrent entity ingest (SemStreams ADR-072, SemStreams gh#480). The pool partitions
	// ingest by entity ID so same-entity updates stay ordered while different
	// entities ingest in parallel. Built in Start BEFORE subscriptions (M3);
	// drained in Stop after every native consumer reports Closed. The pool and
	// submission contexts are children of the run context Start creates; only their cancel
	// functions are retained so no production struct stores context authority.
	ingestPool         *dispatch.KeyedPool[ingestWork]
	ingestPoolCancel   context.CancelFunc
	ingestSubmitCancel context.CancelFunc

	// Redelivery guard (SemStreams ADR-072 B1/B2/B3), two-tier. ingestGuardMem is the
	// in-memory fast path, one map per lane (lane-local → lock-free, since a
	// lane's inputs are applied one at a time). ingestGuardBucket is the durable
	// backstop `(entityID/streamName) → last-applied stream seq`; it survives
	// restart and cache eviction so correctness does not depend on the in-memory
	// retention policy or MaxDeliver/AckWait sizing.
	ingestGuardMem    []*laneGuard
	ingestGuardBucket *natsclient.KVStore

	// Exact callback resources retained for terminal cleanup. boundConsumers is
	// observation-only readiness state and is deliberately not a lifecycle handle.
	consumers     []graphIngestConsumerBinding
	subscriptions []graphIngestCoreSubscription
}

type graphIngestConsumerBinding struct {
	handle      jetstream.ConsumeContext
	drainIssued bool
}

type graphIngestCoreSubscription interface {
	Drain(context.Context) error
}

// DeclarePorts is the component.PortDeclarer for graph-ingest: the ports
// CreateGraphIngest will report for rawConfig, computed without dependencies.
func DeclarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, outputs), nil
}

// resolveConfig parses rawConfig, applies defaults, validates, and resolves
// the effective ports. It is the one derivation DeclarePorts and
// CreateGraphIngest share.
func resolveConfig(rawConfig json.RawMessage) (Config, []component.Port, []component.Port, error) {
	var config Config
	if len(rawConfig) > 0 {
		// Strict decoding: a key Config does not define is refused, naming the key, where
		// encoding/json would drop it and leave the component running on a default the
		// operator meant to change (design D6, Config (b); the shape of the pin's
		// inference.RejectUnknownKeys, SemStreams ADR-054). A port definition decodes through its
		// own UnmarshalJSON, which this setting does not reach.
		decoder := json.NewDecoder(bytes.NewReader(rawConfig))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return Config{}, nil, nil, errs.Wrap(err, "CreateGraphIngest", "factory", "config unmarshal")
		}
		// json.Unmarshal, which this replaces, refused data after the object; keep that.
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Config{}, nil, nil, errs.Wrap(errors.New("data after the configuration object"),
				"CreateGraphIngest", "factory", "config unmarshal")
		}
	} else {
		config = DefaultConfig()
	}

	// Apply defaults and validate
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, nil, nil, errs.Wrap(err, "CreateGraphIngest", "factory", "config validation")
	}
	inputs := make([]component.Port, 0, len(config.Ports.Inputs))
	for _, definition := range config.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "CreateGraphIngest", "factory", "resolve input port")
		}
		inputs = append(inputs, port)
	}
	outputs := make([]component.Port, 0, len(config.Ports.Outputs))
	for _, definition := range config.Ports.Outputs {
		port, err := definition.Resolve(component.DirectionOutput)
		if err != nil {
			return Config{}, nil, nil, errs.WrapInvalid(err, "CreateGraphIngest", "factory", "resolve output port")
		}
		outputs = append(outputs, port)
	}
	return config, inputs, outputs, nil
}

// CreateGraphIngest is the factory function for creating graph-ingest components
func CreateGraphIngest(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	// Validate dependencies
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphIngest", "factory", "NATSClient required")
	}
	natsClient := deps.NATSClient
	// A nil registry would make the decoder fail at the first fact-lane message
	// and, under SemStreams ADR-103, every create fail closed — so it is a boot error.
	if deps.PayloadRegistry == nil {
		return nil, errs.WrapInvalid(errors.New("payload registry is required"), "CreateGraphIngest", "factory", "PayloadRegistry required")
	}
	// The authority gate compares every candidate subject against this pair
	// (ADR-102 d5). An absent pair has no honest reading: admitting everything
	// would silently retire the gate, and rejecting everything would take the
	// graph down at the first fact. Both are worse than refusing to construct,
	// and config load already requires platform.org and platform.id
	// (config/config.go Validate), so a real deployment cannot reach this.
	if deps.Platform.Org == "" || deps.Platform.Platform == "" {
		return nil, errs.WrapInvalid(
			errors.New("deps.Platform must carry the deployment authority (platform.org and platform.id)"),
			"CreateGraphIngest", "factory", "Platform required")
	}

	config, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	// O-16 (a): hierarchy containers are born with a registered framework type.
	// A registry that lacks it would refuse every container birth, and with it
	// every birth that infers hierarchy (design D21), so the composition root
	// learns at construction instead.
	if config.EnableHierarchy {
		containerType := inference.HierarchyContainerMessageType()
		if _, ok := deps.PayloadRegistry.GetRegistration(containerType); !ok {
			return nil, errs.WrapInvalid(
				fmt.Errorf("enable_hierarchy requires %s in the payload registry (register it with inference.RegisterPayloads)", containerType),
				"CreateGraphIngest", "factory", "hierarchy container type")
		}
	}

	// Create logger with component context
	logger := deps.GetLoggerWithComponent("graph-ingest")

	// Create component
	comp := &Component{
		name:            "graph-ingest",
		config:          config,
		inputs:          inputs,
		outputs:         outputs,
		decoder:         message.NewDecoder(deps.PayloadRegistry),
		payloadRegistry: deps.PayloadRegistry,
		natsClient:      natsClient,
		logger:          logger,
		metricsRegistry: deps.MetricsRegistry,
		cacheGen:        make(map[string]uint64),
		platform:        deps.Platform,
	}

	if err := comp.registerMetrics(deps.MetricsRegistry); err != nil {
		return nil, errs.Wrap(err, "CreateGraphIngest", "factory", "register metrics")
	}
	return comp, nil
}

// Register registers the graph-ingest factory with the component registry
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        "graph-ingest",
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Entity and triple ingestion processor",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphIngest,
		Ports:       DeclarePorts,
	})
}

// ============================================================================
// Discoverable Interface
// ============================================================================

// InputPorts returns input port definitions.
// Reads directly from config so ports are available before Initialize().
func (c *Component) InputPorts() []component.Port {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return append([]component.Port(nil), c.inputs...)
}

// OutputPorts returns output port definitions.
// Reads directly from config so ports are available before Initialize().
func (c *Component) OutputPorts() []component.Port {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return append([]component.Port(nil), c.outputs...)
}

// Health reports the shell's verdict (component.Lifecycle.Health) over graph-ingest's
// stuck states, not a count of past errors (ADR-109 Decision 2): a poisoned entity, or
// a backlog the latest status tick could not read. ErrorCount is reported and never
// lowers Healthy. A non-empty poison inventory degrades Health per-entity: status is
// "degraded", NOT "reset_required" (that stays a per-read error code, design D6), and
// the message carries count + a bounded ID/reason sample. Full enumeration rides
// DebugStatus.
func (c *Component) Health() component.HealthStatus {
	var poison, fault string
	if c.entityPoisonSize.Load() > 0 {
		poison = c.entityPoisonHealthMessage()
	}
	if f := c.backlogObservationFault.Load(); f != nil {
		fault = *f
	}
	return c.lifecycle.Health(poison, fault)
}

// DataFlow reports input messages: each delivery on a stream input, redeliveries
// included, and each mutation request. Queries, in-process creates and the containers
// hierarchy inference creates are not input messages. ErrorRate is the processing
// errors counted (Health's ErrorCount) per input message.
func (c *Component) DataFlow() component.FlowMetrics {
	return c.lifecycle.DataFlow()
}

// ============================================================================
// LifecycleComponent Interface (3 methods)
// ============================================================================

// Initialize marks the component initialized (no I/O). The factory validated the
// configuration.
func (c *Component) Initialize() error {
	return c.lifecycle.Initialize()
}

// Start begins processing (must be initialized first). The lifecycle state is the
// shell's (component.Lifecycle, over internal/lifecycleguard): a nil or ended context and
// a second Start are refused before anything is acquired; a Start that fails is rolled
// back through release, and a rollback that fails leaves release pending for the next
// Stop. A component with no NATS client, or an uninitialized one, is refused before the
// guard is consulted, so a later Start after Initialize is still admitted.
//
// ctx bounds Start only. Everything Start does before it returns runs under ctx:
// connecting to NATS (or waiting while the client reconnects), bucket setup, the
// ENTITY_STATES sweep and consumer setup. A ctx that ends first fails Start.
// Subscriptions, the ingest pool, the entity cache's expiry sweep and the status loop run
// under a run context start creates and release ends, so ending ctx after Start returned
// stops nothing (ADR-113). Request handlers run under a context natsclient owns; release
// drains their subscriptions, which waits for running handlers.
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.release)
}

// start acquires everything Start owns, under the guard. Each handle it takes is
// recorded before the next acquisition, so release can return whatever a failed start
// left behind.
func (c *Component) start(ctx context.Context) error {
	// The run context: Start's values without its cancellation, ended only by release.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	poolCtx, poolCancel := context.WithCancel(runCtx)
	submitCtx, submitCancel := context.WithCancel(runCtx)
	c.cancel = cancel
	c.ingestPoolCancel = poolCancel
	c.ingestSubmitCancel = submitCancel

	if c.natsClient.Status() != natsclient.StatusConnected {
		// Connect refuses a client whose connection is installed, as it is while nats.go
		// reconnects it; Start then waits for that connection (#291).
		if err := c.natsClient.Connect(ctx); err != nil && !errors.Is(err, errs.ErrAlreadyStarted) {
			if ctx.Err() != nil {
				return errs.Wrap(ctx.Err(), "Component", "Start", "context cancelled during NATS connection")
			}
			return errs.Wrap(err, "Component", "Start", "NATS connection failed")
		}
		if err := c.natsClient.WaitForConnection(ctx); err != nil {
			if ctx.Err() != nil {
				return errs.Wrap(ctx.Err(), "Component", "Start", "context cancelled waiting for NATS")
			}
			return errs.Wrap(err, "Component", "Start", "wait for NATS connection")
		}
	}

	if err := c.initStorage(ctx, runCtx); err != nil {
		return err
	}
	if err := c.startEntityStateGuard(ctx, c.entityBucket); err != nil {
		return errs.Wrap(err, "Component", "Start", "ENTITY_STATES snapshot sweep")
	}
	if err := c.initHierarchyInference(); err != nil {
		return errs.Wrap(err, "Component", "Start", "hierarchy inference")
	}
	if err := c.createStatusBucket(ctx); err != nil {
		return errs.Wrap(err, "Component", "Start", "readiness status bucket")
	}
	if err := c.buildIngestPool(poolCtx); err != nil {
		return errs.Wrap(err, "Component", "Start", "keyed ingest pool")
	}
	if err := c.setupSubscriptions(ctx, submitCtx); err != nil {
		return errs.Wrap(err, "Component", "Start", "subscription setup")
	}
	if err := c.setupQueryHandlers(ctx); err != nil {
		return errs.Wrap(errors.Join(err, ctx.Err()), "Component", "Start", "query handler setup")
	}
	if err := c.setupMutationHandlers(ctx); err != nil {
		return errs.Wrap(errors.Join(err, ctx.Err()), "Component", "Start", "mutation handler setup")
	}
	// A Start context that ends after the last registration succeeded, while setup is still
	// running, fails Start here (ADR-113: if ctx ends before Start returns, Start returns its
	// error and holds nothing). The guard's rollback then runs release.
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "Start", "context ended during startup")
	}

	c.startStatusMetricsLoop(runCtx)
	c.logger.Info("component started",
		slog.String("component", "graph-ingest"),
		slog.Time("start_time", time.Now()))
	// The last step: nothing between here and the guard's commit can fail.
	c.startCompleted.Store(true)
	return nil
}

// subscribeForRequests registers handler for the requests on subject. Every request
// subscription graph-ingest makes goes through it, and handler does no work until start
// has completed: before that, a request gets a transient error matching
// errs.ErrNotStarted. natsclient can run a handler during a registration that then fails
// and returns no subscription, which a failed Start's rollback cannot drain (checkpoint 3
// B2); the check keeps that handler off what the rollback clears. The requester receives
// the refusal as a transient error with no code (SemStreams ADR-060's code set is closed). The check
// wraps the metering wrappers, so a refused mutation records no outcome or rejection.
func (c *Component) subscribeForRequests(
	ctx context.Context,
	subject string,
	handler func(context.Context, []byte) ([]byte, error),
) (*natsclient.Subscription, error) {
	subscribe := c.natsClient.SubscribeForRequests
	if c.subscribeRequests != nil {
		subscribe = c.subscribeRequests
	}
	return subscribe(ctx, subject, func(ctx context.Context, data []byte) ([]byte, error) {
		if !c.startCompleted.Load() {
			return nil, errs.WrapTransient(errs.ErrNotStarted, "Component", "request", "serve "+subject)
		}
		return handler(ctx, data)
	})
}

func (c *Component) startStatusMetricsLoop(ctx context.Context) {
	statusDone := make(chan struct{})
	c.statusDone = statusDone
	go func() {
		defer close(statusDone)
		c.statusMetricsLoop(ctx)
	}()
}

// Stop gracefully shuts down the component through the shell, which runs release, the
// cleanup Start gave it: Stop before Start releases nothing; Stop during Start waits for
// it, or returns ctx's error first; a Stop whose release fails keeps it pending, and the
// next Stop runs it again; after a release that succeeded, Stop returns nil and runs
// nothing. Stop logs nothing itself: release logs "component stopped gracefully" when
// it succeeds.
func (c *Component) Stop(ctx context.Context) error {
	return c.lifecycle.Stop(ctx)
}

// release is the cleanup the shell runs, for a Stop or for a failed Start's rollback.
// The handles stay set when cleanup fails, so the next Stop releases them again; they
// are cleared only by a cleanup that succeeded (design D22). A consumer whose drain was
// already issued is not drained again (drainIssued). Each release that succeeds logs
// "component stopped gracefully" once, after the handles are cleared, whether it ran
// for a Stop or for a failed Start's rollback; one that fails logs nothing.
func (c *Component) release(ctx context.Context) error {
	if err := c.cleanup(ctx); err != nil {
		return err
	}
	c.clearLifecycleHandles()
	c.logger.Info("component stopped gracefully", slog.String("component", "graph-ingest"))
	return nil
}

// cleanup preserves effect -> durable guard -> settlement by keeping callback,
// submission, pool, KV, and cache authority live until exact native closure.
func (c *Component) cleanup(ctx context.Context) error {
	var cleanupErr error
	for index := range c.consumers {
		binding := &c.consumers[index]
		if !binding.drainIssued {
			binding.handle.Drain()
			binding.drainIssued = true
		}
	}
	for index := range c.consumers {
		select {
		case <-c.consumers[index].handle.Closed():
		case <-ctx.Done():
			cleanupErr = errors.Join(cleanupErr, ctx.Err())
		}
	}
	if c.ingestSubmitCancel != nil {
		c.ingestSubmitCancel()
	}
	if c.ingestPool != nil {
		cleanupErr = errors.Join(cleanupErr, c.ingestPool.Shutdown(ctx))
	}
	if c.ingestPoolCancel != nil {
		c.ingestPoolCancel()
	}
	for _, sub := range c.subscriptions {
		if sub != nil {
			cleanupErr = errors.Join(cleanupErr, sub.Drain(ctx))
		}
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.statusDone != nil {
		select {
		case <-c.statusDone:
		case <-ctx.Done():
			cleanupErr = errors.Join(cleanupErr, ctx.Err())
		}
	}
	if c.entityCache != nil {
		cleanupErr = errors.Join(cleanupErr, c.entityCache.Close())
	}
	return errors.Join(cleanupErr, ctx.Err())
}

func (c *Component) clearLifecycleHandles() {
	c.cancel = nil
	c.ingestPoolCancel = nil
	c.ingestSubmitCancel = nil
	c.ingestPool = nil
	c.statusDone = nil
	c.consumers = nil
	c.subscriptions = nil
	c.mu.Lock()
	c.entityCache = nil
	c.mu.Unlock()
	c.statusPublisher = nil
	c.clearBoundConsumers()
}

// initStorage initializes KV buckets and query caches. Every bucket this
// component owns is acquired through the catalog seam
// (natsclient.EnsureFrameworkBucket): create-or-open, reconcile to the
// declared policy (a foreign TTL is stripped, an adopted divergent History is
// converged), verify, or fail this Start closed — which the composition
// root's component-start barrier turns into a failed boot. The buckets are
// acquired under ctx, the Start context; the entity cache is built under runCtx,
// the run context, because its expiry sweep runs until that context ends.
func (c *Component) initStorage(ctx, runCtx context.Context) error {
	// Entity states KV bucket - we are the WRITER (catalog owner).
	bucket, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketEntityStates)
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "KV bucket creation")
	}
	c.entityBucket = c.natsClient.NewKVStore(bucket)

	// Entity query cache (HybridCache: LRU capacity + TTL freshness)
	entityCache, err := cache.NewFromConfig[graph.EntityState](runCtx, cache.Config{
		Enabled:         true,
		Strategy:        cache.StrategyHybrid,
		MaxSize:         5000,
		TTL:             30 * time.Second,
		CleanupInterval: 10 * time.Second,
	}, cache.WithMetrics[graph.EntityState](c.metricsRegistry, "entity_query_cache"))
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "entity cache creation")
	}
	c.mu.Lock()
	c.entityCache = entityCache
	c.mu.Unlock()

	// SemStreams ADR-072 redelivery-guard durable tier: graph-ingest-owned bucket mapping
	// `(entityID/streamName) → (stream generation, last-applied stream sequence)`
	// (#15). Operational state graph-ingest owns (bucket-ownership rubric), a
	// sixteen-byte value — no cross-repo/EntityState schema change. No TTL: a
	// record must outlive every redelivery or re-publish of its input, whatever
	// the consumer's delivery limit, and the key set is bounded by entity
	// cardinality (same order as ENTITY_STATES); a recreated stream's apply
	// replaces its predecessor's record under the same key.
	guardBucket, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketGraphIngestAppliedSeq)
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "redelivery guard bucket creation")
	}
	c.ingestGuardBucket = c.natsClient.NewKVStore(guardBucket)

	return nil
}

// initHierarchyInference builds hierarchy inference when enable_hierarchy is
// set. With it off the inference stays nil, and a nil inference is what "off"
// means at every birth.
func (c *Component) initHierarchyInference() error {
	if !c.config.EnableHierarchy {
		return nil
	}
	// The deployment's own authority, the same deps.Platform the gate reads:
	// containers are minted from the ingested entity's prefix, so an imported
	// entity would mint them under a peer's authority. Inference skips those
	// entities entirely (ADR-102). The factory has already refused an empty
	// pair, so the constructor's own refusal is not reached from here.
	hi, err := inference.NewHierarchyInference(&hierarchyStore{component: c}, c.platform, c.logger)
	if err != nil {
		return err
	}
	c.hierarchyInference = hi
	return nil
}
