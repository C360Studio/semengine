package graphclustering

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
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// key names graph-clustering once: its registration, its logger and its GRAPH_STATUS key.
const key = readiness.KeyGraphClustering

// The Failed reasons graph-clustering names itself (design D2): reasonRefusedEntity for entities
// the last tick that reported them left out of the records, reasonPublicationFailed while the last
// tick's publication failed.
const (
	reasonRefusedEntity     = "refused_entity"
	reasonPublicationFailed = "publication_failed"
)

var _ component.Discoverable = (*Component)(nil)

// Component implements the graph-clustering processor.
type Component struct {
	config Config
	inputs []component.Port

	natsClient      *natsclient.Client
	logger          *slog.Logger
	metricsRegistry *metric.MetricsRegistry
	metrics         *clusteringMetrics

	// interval is the resolved detection_interval: the time between ticks.
	interval time.Duration

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle

	// start sets the follower and the maintainer before the follower's Run begins.
	follower   atomic.Pointer[follower.Follower] // read by Health at any time
	maintainer *clustering.Maintainer

	// outcomeMu guards the last outcome: refused, the refused-entity count of the last tick that
	// reported one, and publicationFailed, whether the last tick's publication failed.
	outcomeMu         sync.Mutex
	refused           int
	publicationFailed bool

	// The handles start records for cleanup, which clears them once it has released them all.
	// runCancel ends the one run context (ADR-113) the follower's Run and the ticker run under;
	// feeds closes once that Run has returned, ticks once the ticker has.
	runCancel context.CancelFunc
	feeds     chan struct{}
	ticks     chan struct{}
}

// DeclarePorts is the component.PortDeclarer for graph-clustering: the ports
// CreateGraphClustering will report for rawConfig, computed without dependencies.
func DeclarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, nil), nil
}

// resolveConfig decodes rawConfig, applies defaults, validates, and resolves the input ports. It
// is the one derivation DeclarePorts and CreateGraphClustering share. Validate refused every
// declared output, so there are none to resolve.
func resolveConfig(rawConfig json.RawMessage) (Config, []component.Port, error) {
	config, err := decodeConfig(rawConfig)
	if err != nil {
		return Config{}, nil, err
	}
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return Config{}, nil, errs.Wrap(err, "CreateGraphClustering", "factory", "config validation")
	}
	inputs := make([]component.Port, 0, len(config.Ports.Inputs))
	for _, definition := range config.Ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		if err != nil {
			return Config{}, nil, errs.WrapInvalid(err, "CreateGraphClustering", "factory", "resolve input port")
		}
		inputs = append(inputs, port)
	}
	return config, inputs, nil
}

// CreateGraphClustering is the factory function for graph-clustering components. It opens no
// connection.
func CreateGraphClustering(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	if deps.NATSClient == nil {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "CreateGraphClustering", "factory", "NATSClient required")
	}
	config, inputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	interval, err := config.interval() // Validate has refused a value that does not resolve
	if err != nil {
		return nil, err
	}
	metrics, err := newClusteringMetrics(deps.MetricsRegistry)
	if err != nil {
		return nil, errs.Wrap(err, "CreateGraphClustering", "factory", "register metrics")
	}
	return &Component{
		config:          config,
		inputs:          inputs,
		natsClient:      deps.NATSClient,
		logger:          deps.GetLoggerWithComponent(key),
		metricsRegistry: deps.MetricsRegistry,
		metrics:         metrics,
		interval:        interval,
	}, nil
}

// Register registers the graph-clustering factory with the component registry. It asks boot for
// no model registry: at tier 0 the records need no provider.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "processor",
		Protocol:    "nats",
		Domain:      "graph",
		Description: "Graph community records by entity-ID prefix group",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     CreateGraphClustering,
		Ports:       DeclarePorts,
	})
}

// InputPorts returns the input ports, fixed at construction.
func (c *Component) InputPorts() []component.Port { return slices.Clone(c.inputs) }

// OutputPorts returns no ports: graph-clustering writes COMMUNITY_INDEX itself.
func (c *Component) OutputPorts() []component.Port { return nil }

// Health reports the shell's verdict (component.Lifecycle.Health) over the follower's stuck
// states: a latched reset, the failures the last tick reported (refused entities, a failed
// publication), a lost feed.
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

// Start begins following ENTITY_STATES into the maintainer. The lifecycle state is the shell's
// (component.Lifecycle): Start before Initialize, a nil or ended context and a second Start are
// refused, the second with errs.ErrAlreadyStarted, and a Start that fails is rolled back through
// cleanup. A component with no NATS client is refused first.
//
// ctx bounds Start only: the wait for ENTITY_STATES and opening COMMUNITY_INDEX run under it. The
// follower's watch and the ticker, which publishes every detection_interval, run under a run
// context start creates and cleanup ends, so ending ctx after Start returned stops nothing
// (ADR-113).
func (c *Component) Start(ctx context.Context) error {
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.cleanup)
}

// start opens the follower and COMMUNITY_INDEX, builds the maintainer, then starts what Stop
// joins. Nothing it acquires before the run context needs releasing.
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
		// A refused entity is left out of its groups' records; the others still publish, so no
		// failure withholds anything (design D2).
		Withhold: func() int64 { return 0 },
	})
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "open the ENTITY_STATES follower")
	}
	bucket, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketCommunityIndex)
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "KV bucket: "+graph.BucketCommunityIndex)
	}
	storage, err := clustering.NewNATSCommunityStorage(c.natsClient.NewKVStore(bucket))
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "community storage")
	}
	maintainer, err := clustering.NewMaintainer(storage)
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "community maintainer")
	}
	// A Start context that ended during setup fails Start here, before anything is held
	// (ADR-113).
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "Start", "context ended during startup")
	}
	c.maintainer = maintainer
	c.follower.Store(f)

	// The one run context: Start's values without its cancellation, ended only by cleanup.
	runCtx, runCancel := context.WithCancel(context.WithoutCancel(ctx))
	c.runCancel = runCancel
	feeds := make(chan struct{})
	c.feeds = feeds
	ticks := make(chan struct{})
	c.ticks = ticks
	go func() {
		defer close(feeds)
		if err := f.Run(runCtx); err != nil {
			c.logger.Warn("ENTITY_STATES watch did not stop cleanly", slog.Any("error", err))
		}
	}()
	go func() {
		defer close(ticks)
		c.runTicks(runCtx, c.tick)
	}()
	return nil
}

// runTicks calls tick every detection_interval, the first one interval after it starts, until ctx
// ends. A tick that outlasts the interval delays the next; ticks never overlap.
func (c *Component) runTicks(ctx context.Context, tick func(context.Context)) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick(ctx)
		}
	}
}

// deliver is the follower's Deliver, on its Run goroutine: it applies the delivered value at its
// revision to the maintainer, then completes the revision at once, so the graph-clustering key's
// indexed revision counts what the maintainer holds, not what is published (design D1). A value
// that does not decode is applied as a refused entity, which contributes nothing to its groups;
// the next tick counts it under refused_entity. The maintainer ignores a delivery at or below the
// revision it last applied for the key.
func (c *Component) deliver(_ context.Context, e follower.Entry) {
	c.lifecycle.CountInput(e.Bytes)
	var err error
	if e.Deleted {
		err = c.maintainer.Delete(e.Key, e.Revision)
	} else {
		var state graph.EntityState
		if err = graph.UnmarshalEntityState(e.Key, e.Value, &state); err != nil {
			// A nil state is the maintainer's refusal; its own error says less than the decoder's.
			_ = c.maintainer.Upsert(e.Key, e.Revision, nil)
		} else {
			err = c.maintainer.Upsert(e.Key, e.Revision, &state)
		}
	}
	if err != nil {
		// The entity contributes nothing until a later write for it applies; the tick that
		// follows names it in an *IncompleteError, which Failed reports.
		c.logger.Warn("entity refused by the community maintainer; it is left out of the records",
			slog.String("key", e.Key), slog.Uint64("revision", e.Revision), slog.Any("error", err))
	}
	c.follower.Load().Complete(e.Key, e.Revision)
}

// failed is the follower's Failed, from the last tick's outcome: the refused entities the last
// tick that reported them left out of the records, under refused_entity, and 1 under
// publication_failed while the last tick's publication failed; none before the first tick.
func (c *Component) failed() (int64, map[string]uint64) {
	c.outcomeMu.Lock()
	refused, publicationFailed := c.refused, c.publicationFailed
	c.outcomeMu.Unlock()
	reasons := map[string]uint64{}
	if refused > 0 {
		reasons[reasonRefusedEntity] = uint64(refused)
	}
	if publicationFailed {
		reasons[reasonPublicationFailed] = 1
	}
	if len(reasons) == 0 {
		return 0, nil
	}
	n := int64(refused)
	if publicationFailed {
		n++
	}
	return n, reasons
}

// tick runs one publication: none while the follower's QueryReady refuses (the replay not yet
// applied, a lost feed, a latched reset), so no record is built from a partial replay or a broken
// feed; otherwise the maintainer's bootstrap is ended, its changed groups are published, and the
// outcome is kept for Failed (design D2). A tick the run context ends is not an outcome: Stop is
// under way, and the work stays pending in the maintainer.
func (c *Component) tick(ctx context.Context) {
	if err := c.follower.Load().QueryReady(ctx); err != nil {
		return
	}
	c.maintainer.BootstrapDone()
	err := c.maintainer.Tick(ctx)
	var incomplete *clustering.IncompleteError
	switch {
	case err == nil:
		c.setOutcome(0, false)
	case errors.As(err, &incomplete):
		c.setOutcome(len(incomplete.Refused), false)
	case ctx.Err() != nil:
		// Stop ended the run context mid-tick: not a publication failure.
	default:
		// A failed write returns no refusal count, so the last known one is kept. The failure is
		// counted and kept before it is logged.
		c.metrics.publicationFailures.Inc()
		c.outcomeMu.Lock()
		c.publicationFailed = true
		c.outcomeMu.Unlock()
		c.logger.Warn("community publication failed; the changed groups are retried on the next tick",
			slog.Any("error", err))
	}
}

func (c *Component) setOutcome(refused int, publicationFailed bool) {
	c.outcomeMu.Lock()
	c.refused, c.publicationFailed = refused, publicationFailed
	c.outcomeMu.Unlock()
}

// Stop stops the component through the shell, which runs cleanup: Stop before Start does nothing,
// and a Stop whose context ends first returns that context's error and keeps what is left for the
// next Stop.
func (c *Component) Stop(ctx context.Context) error { return c.lifecycle.Stop(ctx) }

// cleanup releases what start acquired, for a Stop or a failed Start's rollback, tolerating a
// partial start: it ends the run context and joins the follower's Run and the ticker, so nothing
// more is delivered or published. A ctx that ends first returns its error with the handles kept,
// and the next Stop resumes.
func (c *Component) cleanup(ctx context.Context) error {
	if c.runCancel != nil {
		c.runCancel()
	}
	for _, done := range []chan struct{}{c.feeds, c.ticks} {
		if done == nil {
			continue
		}
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.runCancel, c.feeds, c.ticks = nil, nil, nil
	return nil
}
