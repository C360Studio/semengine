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
	"reflect"
	"sort"
	"strings"
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
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/types"
	"github.com/c360studio/semengine/vocabulary"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"
)

// Ensure Component implements required interfaces
var (
	_ component.Discoverable        = (*Component)(nil)
	_ component.LifecycleComponent  = (*Component)(nil)
	_ component.DebugStatusProvider = (*Component)(nil)
)

func newEntitiesUpdatedMetric() prometheus.Counter {
	entitiesUpdated := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "datamanager",
		Name:      "entities_updated_total",
		Help:      "Total entities updated",
	})
	return entitiesUpdated
}

// newIndexingProfileDefaultMetric builds the counter that fires
// whenever graph-ingest falls back to the indexing-profile floor at entity
// creation — i.e. neither a Graphable IndexingProfiler nor a mutation-envelope
// indexing_profile field declared a profile (ADR-054 §5). Labeled by
// message_type so operators can see WHICH producers omit a declaration: a new
// "content" type nobody declared would otherwise silently default to "control"
// and never be embedded. message_type is the low-cardinality registry key; the
// full entity subject is intentionally NOT a label (cardinality bomb).
func newIndexingProfileDefaultMetric() *prometheus.CounterVec {
	indexingProfileDefaultVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "indexing_profile_default_total",
		Help:      "Entities born with no producer-declared indexing profile whose registered message type declares no floor and defaulted to control — edit the type's Registration.IndexingProfile",
	}, []string{"message_type"})
	return indexingProfileDefaultVec
}

// newMutationRejectionsMetric builds the counter for rejected
// graph-mutation requests, labelled by subject + reason (the MutationResponse
// ErrorCode — a bounded closed set). It operationalizes ADR-055 §3's "loud
// fail-fast" observability. Append, reconcile, and delete against an absent
// entity report entity_not_found. Other bounded classes include validation,
// revision conflict, and strict-create conflict.
func newMutationRejectionsMetric() *prometheus.CounterVec {
	mutationRejectionsVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "mutation_rejections_total",
		Help:      "Graph-mutation requests rejected (success=false), by subject and reason (MutationResponse ErrorCode; 'unclassified' when absent)",
	}, []string{"subject", "reason"})
	return mutationRejectionsVec
}

func newMutationOutcomesMetric() *prometheus.CounterVec {
	mutationOutcomesVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "graph_mutation_outcomes_total",
		Help:      "Canonical graph mutation outcomes by bounded operation and outcome",
	}, []string{"operation", "outcome"})
	return mutationOutcomesVec
}

func newPredicateContractRejectionsMetric() *prometheus.CounterVec {
	predicateContractRejectionsVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "predicate_contract_rejections_total",
		Help:      "Canonical predicate contract rejections, counted once per unique bounded reason in a rejected candidate",
	}, []string{"lane", "reason"})
	return predicateContractRejectionsVec
}

func newEntityStateContractRejectionsMetric() *prometheus.CounterVec {
	entityStateContractRejectionsVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "entity_state_contract_rejections_total",
		Help:      "Canonical entity-state identity contract rejections by bounded ingest lane, field, and reason",
	}, []string{"lane", "field", "reason"})
	return entityStateContractRejectionsVec
}

// newProcessingDurationMetric builds the histogram of per-message
// apply time — the merge + CAS write inside handleMessage (gh#480). Paired with
// newIngestLagMetric it separates processing time from queue wait, which the
// component previously could not distinguish (callers inferred end-to-end latency
// downstream). Buckets span sub-ms (a warm CAS is ~1.5ms) to seconds (a stalled KV).
func newProcessingDurationMetric() prometheus.Histogram {
	processingDurationHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "processing_duration_seconds",
		Help:      "Per-message apply time in graph-ingest (merge + CAS write). The processing half of the queue-wait-vs-processing split (gh#480).",
		Buckets:   []float64{0.0005, 0.001, 0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	})
	return processingDurationHistogram
}

// newIngestLagMetric builds the histogram of message age at the
// moment graph-ingest begins processing it (now − the JetStream message
// timestamp) — i.e. how long a message waited in the stream/delivery buffer
// before ingest reached it (gh#480 queue wait). A rising lag with flat
// processing_duration is the backlog signature (consumer_pending_messages
// climbing) the issue describes.
func newIngestLagMetric() prometheus.Histogram {
	ingestLagHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "ingest_lag_seconds",
		Help:      "Age of a message when graph-ingest starts processing it (queue/backlog wait). The queue-wait half of the split (gh#480).",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
	})
	return ingestLagHistogram
}

// newRedeliveriesDroppedMetric builds the counter of messages
// dropped by the keyed-ingest redelivery guard (ADR-072 B1/B2/B3): a delayed
// redelivery whose stream sequence is not newer than the last already applied
// to that entity from the same stream. A small steady number under overload is
// healthy; a large one means MaxAckPending/AckWait are undersized for the lane
// throughput. The keyed pool counts no drops of its own: graph-ingest submits
// with dispatch.KeyedPool.SubmitBlocking, which waits for room in the lane, and
// a refused submit naks the message for redelivery.
func newRedeliveriesDroppedMetric() prometheus.Counter {
	redeliveriesDroppedCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "redeliveries_dropped_total",
		Help:      "Stale redeliveries dropped by the applied-sequence guard (ADR-072).",
	})
	return redeliveriesDroppedCounter
}

// newCasRetriesMetric builds the counter of CAS-conflict retries
// during entity merge (ADR-072). Incremented each time mergeEntityOnLane's CAS
// callback re-runs (attempt > 1) — the previous attempt's revision-checked Put
// lost the CAS and retried. This is a **cross-entity contention-observability**
// signal, NOT a proof of keying correctness: an entity's own key is
// never written concurrently under keying, but legitimate cross-entity
// hierarchy container and inverse writes do touch shared keys and retry. Expect ~0 on workloads
// without hierarchy / dense relationships / entity-birth churn; a spike is a
// workload signal, not necessarily a bug.
func newCasRetriesMetric() prometheus.Counter {
	casRetriesCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "cas_retries_total",
		Help:      "Entity-merge CAS-conflict retries — cross-entity contention observability, NOT a keying-correctness proof (ADR-072).",
	})
	return casRetriesCounter
}

// newStaleSetsMetric builds the counter of (predicate, source) sets a stream message carried that
// were not applied because they were older than the stored set of the same key (design D15,
// "Timestamp orders a replace"; #98). The stream lane has no reply, so this count and the debug
// line beside it are how a producer learns that part of its message was not applied.
func newStaleSetsMetric() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "stale_sets_total",
		Help:      "Stream-lane statement sets not applied because they were older than the stored set of the same predicate and source.",
	})
}

// dedupLane names the append entry point that submitted a suppressed
// duplicate. The label set is a CLOSED enum declared here, never a
// caller-supplied string: the value is chosen at each in-repo call site, so
// label cardinality is bounded by this list no matter what a producer sends.
type dedupLane string

const (
	// dedupLaneAddBatch is the canonical append mutation lane.
	dedupLaneAddBatch dedupLane = "append"
	// dedupLaneHierarchy is hierarchy inference's in-process adder
	// (tripleAdderAdapter), the lane that produced gh#713: createEntity calls
	// GetHierarchyTriples unconditionally, and its container-inverse and
	// sibling-inverse edges commit through here on every re-registration.
	dedupLaneHierarchy dedupLane = "hierarchy"
)

// newDuplicateTriplesSuppressedMetric builds the counter of
// append triples suppressed because the target entity already carried an
// identical six-field tuple (subject, predicate, object, datatype, source,
// context). It exists so a silently-skipped write is distinguishable from
// absent traffic: without it, "the lane wrote nothing" and "the lane never ran"
// look the same to an operator, and a sustained suppression rate cannot be
// attributed to the component producing it.
//
// Deliberately NOT a per-occurrence log — restart replay makes duplicate
// submission unbounded, which is the whole point of suppressing it.
func newDuplicateTriplesSuppressedMetric() *prometheus.CounterVec {
	duplicateTriplesSuppressedVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "duplicate_triples_suppressed_total",
		Help:      "Append triples not written because the entity already carried an identical six-field tuple. Label: lane (append|hierarchy), a closed enum.",
	}, []string{"lane"})
	return duplicateTriplesSuppressedVec
}

// Config holds configuration for graph-ingest component
type Config struct {
	Ports              *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration,category:basic"`
	EnableHierarchy    bool                  `json:"enable_hierarchy" schema:"type:bool,description:Enable hierarchy inference,default:false,category:advanced"`
	EnableTypeSiblings *bool                 `json:"enable_type_siblings" schema:"type:bool,description:Enable sibling edges between same-type entities (default true when hierarchy enabled),category:advanced"`
	// IngestLanes is the number of keyed-concurrent ingest lanes (ADR-072,
	// gh#480). Messages are partitioned by entity ID (same entity → one lane →
	// serial in arrival order, preserving the arrival-order merge; different
	// entities → parallel), so ingest is no longer bound to a single serial
	// Get+CAS round-trip chain. Default 8 (concurrent); `1` selects the prior
	// fully-serial behavior.
	IngestLanes int `json:"ingest_lanes" schema:"type:int,description:Keyed-concurrent ingest lane count (same entity ID → one lane → ordered; 1 = serial),default:8,category:advanced"`
}

// Validate implements component.Validatable interface
func (c *Config) Validate() error {
	if c.Ports == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "ports configuration required")
	}
	if len(c.Ports.Inputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one input port required")
	}
	if len(c.Ports.Outputs) == 0 {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", "at least one output port required")
	}
	if _, err := canonicalMutationProvider(c.Ports); err != nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Config", "Validate", err.Error())
	}
	// IngestLanes < 1 clamps to serial (ADR-072). Clamp rather than reject so a
	// mis-set negative degrades to safe-serial instead of failing boot. A 0 does
	// not reach the clamp: the factory runs ApplyDefaults first, which reads 0 as
	// unset and makes it the default of 8 (owner ruling, #91 comment 6059144952).
	if c.IngestLanes < 1 {
		c.IngestLanes = 1
	}
	return nil
}

// ApplyDefaults sets default values for configuration
func (c *Config) ApplyDefaults() {
	// EnableHierarchy defaults to false
	if c.Ports == nil {
		c.Ports = &component.PortConfig{}
	}
	// Concurrent-by-default (ADR-072): 0 (unset) → 8 lanes. An explicit 1 opts
	// back into serial and is preserved.
	if c.IngestLanes == 0 {
		c.IngestLanes = defaultIngestLanes
	}
}

// DefaultConfig returns a valid default configuration
func DefaultConfig() Config {
	return Config{
		Ports: &component.PortConfig{
			Inputs: []component.PortDefinition{
				{
					Name: "entity_stream", Config: component.JetStreamPort{StreamName: "ENTITY", Subjects: []string{"entity.>"}, DeliverPolicy: "all"}, // Idempotent: catch up on historical entities

				},
				{
					Name: "mutations", Config: component.NATSRequestPort{Subject: graphmutation.SubjectFamily, Interface: &component.InterfaceContract{Type: graphmutation.InterfaceType, Version: graphmutation.InterfaceVersion}}, Required: true,
				},
			},
			Outputs: []component.PortDefinition{
				{
					Name: "entity_states", Config: component.KVWritePort{Bucket: graph.BucketEntityStates},
				},
			},
		},
		EnableHierarchy: false,
		IngestLanes:     defaultIngestLanes,
	}
}

// Keyed-concurrent ingest sizing (ADR-072, gh#480).
const (
	// defaultIngestLanes is the default keyed-concurrent lane count. 8 overlaps
	// the KV Get+CAS round-trips on the ~11/12-idle box the issue profiled;
	// tune against the semboids repro via the inflight/queue-wait metrics.
	defaultIngestLanes = 8

	// ingestLaneQueueDepth bounds each lane's in-memory submit queue. Total
	// buffered work is capped at lanes × this; SubmitBlocking applies
	// backpressure past it. Sized generously vs. AckWait so the pipeline drains
	// before redelivery (defense-in-depth atop the applied-sequence guard).
	ingestLaneQueueDepth = 256

	// ingestGuardMemMaxPerLane bounds the in-memory redelivery-guard cache per
	// lane. This is a CACHE-SIZE knob only, not correctness: an eviction just
	// costs a durable-tier read on the next access (ADR-072 B3). Sized to hold
	// the working set of recently-active entities per lane under load.
	ingestGuardMemMaxPerLane = 65536
)

// schema defines the configuration schema for graph-ingest component
var schema = component.GenerateConfigSchema(reflect.TypeOf(Config{}))

// entityManagerAdapter adapts Component to implement inference.EntityManager interface
type entityManagerAdapter struct {
	component *Component
}

func (a *entityManagerAdapter) ExistsEntity(ctx context.Context, id string) (bool, error) {
	_, err := a.component.entityBucket.Get(ctx, id)
	if err != nil {
		if natsclient.IsKVNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a *entityManagerAdapter) CreateEntity(ctx context.Context, entity *graph.EntityState) (*graph.EntityState, error) {
	err := a.component.CreateEntity(ctx, entity)
	if err != nil {
		return nil, err
	}
	return entity, nil
}

func (a *entityManagerAdapter) ListWithPrefix(ctx context.Context, prefix string) ([]string, error) {
	// Use server-side prefix filtering (prefix + "." to ensure we match the exact level)
	return a.component.entityBucket.KeysByPrefix(ctx, prefix+".")
}

// tripleAdderAdapter adapts Component to implement inference.TripleAdder interface
type tripleAdderAdapter struct {
	component *Component
}

// AddTriple routes hierarchy inference's container-inverse and sibling-inverse
// edges through the shared append implementation, labelled so their suppressed duplicates
// are attributable to hierarchy rather than to an operator-issued mutation
// (gh#713: createEntity re-derives these on every re-registration).
func (a *tripleAdderAdapter) AddTriple(ctx context.Context, triple message.Triple) error {
	_, _, err := a.component.addTripleLane(ctx, triple, dedupLaneHierarchy)
	return err
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
	// payloadRegistry is the binary's single type authority (ADR-103): the
	// create seams refuse a stamp it does not hold and the indexing-profile
	// floor is read from the registered type. It is a registry, not a context.
	payloadRegistry *payloadregistry.Registry
	natsClient      *natsclient.Client
	logger          *slog.Logger
	// Test-only acquisition seams. Production falls through to the exact Client
	// methods; no exported or adopter-facing lifecycle surface is introduced.
	waitForStreamInput func(context.Context, string) error
	consumeStream      func(
		context.Context,
		natsclient.PortConsumerContext,
		natsclient.StreamConsumerConfig,
		func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error)

	// Domain resources
	entityBucket *natsclient.KVStore            // authoritative KV operations, snapshot watch, and CAS support
	entityCache  cache.Cache[graph.EntityState] // Read-through cache for query handlers

	// Inference components
	hierarchyInference *inference.HierarchyInference

	// Lifecycle state
	mu          sync.RWMutex
	running     bool
	initialized bool
	startTime   time.Time
	// lifecycle is the owner-lifecycle state (design D13); Start and Stop go through it.
	lifecycle lifecycleguard.Guard
	// handlesMu guards the handles Start records for release: the cancel functions,
	// statusDone and consumers.
	handlesMu               sync.Mutex
	statusDone              chan struct{}
	cancel                  context.CancelFunc
	entityWatchLost         atomic.Bool
	entityBootstrapStarted  atomic.Bool
	entityBootstrapComplete atomic.Bool

	// Readiness producer state (ADR-083 envelope on the graph-ingest GRAPH_STATUS
	// key). See readiness.go for the projection and the bootstrap latch rules.
	statusPublisher  *readiness.Publisher
	readinessGauges  *readiness.Gauges
	boundConsumers   []boundConsumer
	boundConsumersMu sync.RWMutex
	// bootBacklog is the outstanding count at the first successful read after
	// binding — the initial catch-up this producer latched against, published as
	// BootstrapScope. bootBacklogKnown makes that capture once-only.
	bootBacklog      atomic.Uint64
	bootBacklogKnown atomic.Bool
	// bootBacklogPartial latches when ANY bind-time read failed, so the accumulated
	// partial is discarded in favour of the first whole observation. One global
	// "known" flag was not enough: a success on one consumer marked the aggregate
	// captured and permanently omitted a failed consumer's backlog.
	bootBacklogPartial atomic.Bool
	// bootBacklogDrained latches when outstanding first reaches zero. It is a latch
	// rather than a live read because Ready already carries "caught up right now";
	// a bootstrap bit that flickered under write load would defer every consumer
	// during ordinary operation.
	bootBacklogDrained atomic.Bool
	// lastAppliedAt is the JetStream timestamp of the most recently APPLIED message
	// (stamped on the ack path). It ages the reported staleness.
	lastAppliedAt atomic.Value // stores time.Time
	// statusInterval and statusNowFn are TEST SEAMS only, so an integration test can
	// observe successive heartbeats and assert staleness without racing the clock.
	statusInterval time.Duration
	statusNowFn    func() time.Time

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
	// concurrent write invalidated the entry. One uint64 per key; the map is
	// bounded by DISTINCT ENTITY IDS EVER WRITTEN (not live ENTITY_STATES
	// cardinality) — it is not pruned, so it grows slowly under create/delete
	// churn. Acceptable for expected populations; a size gauge + bounded pruning
	// are a tracked follow-up (pruning must exclude in-flight reads to preserve
	// the ABA guarantee the counter provides).
	cacheGenMu sync.Mutex
	cacheGen   map[string]uint64

	// repopulateHook is a test-only seam. When non-nil it is invoked by the
	// cache-miss fetch goroutine AFTER the KV Get + generation capture and BEFORE
	// the repopulating Set's generation re-check, so a test can deterministically
	// inject a concurrent invalidation into the read window. Production is nil.
	repopulateHook func(entityID string)

	// org and platform are the DEPLOYMENT's own authority — positions 1-2 of
	// every identity this deployment may mint (ADR-102 d5). Read once from
	// deps.Platform at construction, which refuses an empty pair, so they are
	// non-empty for the component's whole life and never re-derived from a
	// candidate, a payload, or a subject.
	// platform is the deployment's authority, deps.Platform as the component was
	// built with it: the authority gate and hierarchy inference both read it.
	platform types.PlatformMeta

	// Metrics (atomic)
	messagesProcessed int64
	bytesProcessed    int64
	errors            int64
	lastActivity      atomic.Value // stores time.Time

	// Prometheus metrics. The historical exported series name remains stable
	// because dashboards and operator alerts consume it as an external contract.
	entitiesUpdated               prometheus.Counter
	indexingProfileDefault        *prometheus.CounterVec
	mutationRejections            *prometheus.CounterVec
	mutationOutcomes              *prometheus.CounterVec
	batchMissing                  *prometheus.CounterVec
	predicateContractRejections   *prometheus.CounterVec
	entityStateContractRejections *prometheus.CounterVec
	processingDuration            prometheus.Histogram   // gh#480 per-message apply time (processing half)
	ingestLag                     prometheus.Histogram   // gh#480 message age at processing start (queue-wait half)
	redeliveriesDropped           prometheus.Counter     // ADR-072 stale redeliveries dropped by the applied-sequence guard
	casRetries                    prometheus.Counter     // ADR-072 entity-merge CAS-conflict retries (contention observability)
	staleSets                     prometheus.Counter     // stream-lane sets not applied as older (design D15, #98)
	duplicateTriplesSuppressed    *prometheus.CounterVec // append duplicates not stored, by lane (closed enum)
	poisonedEntities              prometheus.Gauge       // per-entity poison inventory size (single gauge, no per-entity labels)
	metricsRegistry               *metric.MetricsRegistry

	// Keyed-concurrent entity ingest (ADR-072, gh#480). The pool partitions
	// ingest by entity ID so same-entity updates stay ordered while different
	// entities ingest in parallel. Built in Start BEFORE subscriptions (M3);
	// drained in Stop after every native consumer reports Closed. The pool and
	// submission contexts are lexical children of Start; only their cancel
	// functions are retained so no production struct stores context authority.
	ingestPool         *dispatch.KeyedPool[ingestWork]
	ingestPoolCancel   context.CancelFunc
	ingestSubmitCancel context.CancelFunc

	// Redelivery guard (ADR-072 B1/B2/B3), two-tier. ingestGuardMem is the
	// in-memory fast path, one map per lane (lane-local → lock-free, since a
	// lane is drained by a single goroutine). ingestGuardBucket is the durable
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
		// inference.RejectUnknownKeys, ADR-054). A port definition decodes through its
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
	// and, under ADR-103, every create fail closed — so it is a boot error.
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
	// A registry that lacks it would refuse every container birth per arrival
	// (WARN and skip — hierarchy edges silently absent), so the composition
	// root learns at construction instead.
	if config.EnableHierarchy {
		containerType := inference.HierarchyContainerMessageType().Key()
		if _, ok := deps.PayloadRegistry.GetRegistration(containerType); !ok {
			return nil, errs.WrapInvalid(
				fmt.Errorf("enable_hierarchy requires %s in the payload registry (register it with payloadbuiltins.Register)", containerType),
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

	// Initialize last activity
	comp.lastActivity.Store(time.Now())

	if err := comp.registerMetrics(deps.MetricsRegistry); err != nil {
		return nil, errs.Wrap(err, "CreateGraphIngest", "factory", "register metrics")
	}
	return comp, nil
}

// metricService is the service half of every metric key graph-ingest registers.
const metricService = "graph-ingest"

// registerMetrics builds the component's collectors and registers them on registry, the
// component.Dependencies.MetricsRegistry it was built with, through metric.RegisterOrGet,
// keeping the collector each call returns: a second graph-ingest on the same registry
// writes the collectors the first one registered, so every write is gathered. With a nil
// registry the collectors are built and registered nowhere; nothing is ever registered on
// Prometheus' process-global registry (design D4). Registration happens here, at
// construction, so the series exist before the first message and the first status tick.
//
// Two instances on one registry share each collector, so the gauges set from one
// instance's own state (the poison inventory, the readiness gauges) are overwritten by the
// other's (#75's class, recorded on the graph-ingest row; not fixed here).
func (c *Component) registerMetrics(registry *metric.MetricsRegistry) error {
	c.entitiesUpdated = newEntitiesUpdatedMetric()
	c.indexingProfileDefault = newIndexingProfileDefaultMetric()
	c.mutationRejections = newMutationRejectionsMetric()
	c.mutationOutcomes = newMutationOutcomesMetric()
	c.batchMissing = newBatchMissingMetric()
	c.predicateContractRejections = newPredicateContractRejectionsMetric()
	c.entityStateContractRejections = newEntityStateContractRejectionsMetric()
	c.processingDuration = newProcessingDurationMetric()
	c.ingestLag = newIngestLagMetric()
	c.redeliveriesDropped = newRedeliveriesDroppedMetric()
	c.casRetries = newCasRetriesMetric()
	c.staleSets = newStaleSetsMetric()
	c.duplicateTriplesSuppressed = newDuplicateTriplesSuppressedMetric()
	c.poisonedEntities = newPoisonedEntitiesMetric()
	// Readiness gauges: the scrapeable half of the ADR-066 envelope. NO revision
	// gauges: graph-ingest is a BACKLOG producer, and the spec states such a
	// producer SHALL NOT expose indexed_revision / target_revision and SHALL NOT
	// synthesize a value — a fabricated revision is worse than an absent one.
	c.readinessGauges = readiness.NewGauges(
		readiness.ProducerNames{Service: metricService, Subsystem: "graph_ingest"},
	)
	if err := c.readinessGauges.Register(registry); err != nil {
		return err
	}
	if registry == nil {
		return nil
	}
	var err error
	if c.entitiesUpdated, err = metric.RegisterOrGet(registry, metricService, "entities_updated_total", c.entitiesUpdated); err != nil {
		return err
	}
	if c.indexingProfileDefault, err = metric.RegisterOrGet(registry, metricService, "indexing_profile_default_total", c.indexingProfileDefault); err != nil {
		return err
	}
	if c.mutationRejections, err = metric.RegisterOrGet(registry, metricService, "mutation_rejections_total", c.mutationRejections); err != nil {
		return err
	}
	if c.mutationOutcomes, err = metric.RegisterOrGet(registry, metricService, "graph_mutation_outcomes_total", c.mutationOutcomes); err != nil {
		return err
	}
	if c.batchMissing, err = metric.RegisterOrGet(registry, metricService, "batch_query_missing_total", c.batchMissing); err != nil {
		return err
	}
	if c.predicateContractRejections, err = metric.RegisterOrGet(registry, metricService, "predicate_contract_rejections_total", c.predicateContractRejections); err != nil {
		return err
	}
	if c.entityStateContractRejections, err = metric.RegisterOrGet(registry, metricService, "entity_state_contract_rejections_total", c.entityStateContractRejections); err != nil {
		return err
	}
	if c.processingDuration, err = metric.RegisterOrGet(registry, metricService, "processing_duration_seconds", c.processingDuration); err != nil {
		return err
	}
	if c.ingestLag, err = metric.RegisterOrGet(registry, metricService, "ingest_lag_seconds", c.ingestLag); err != nil {
		return err
	}
	if c.redeliveriesDropped, err = metric.RegisterOrGet(registry, metricService, "redeliveries_dropped_total", c.redeliveriesDropped); err != nil {
		return err
	}
	if c.casRetries, err = metric.RegisterOrGet(registry, metricService, "cas_retries_total", c.casRetries); err != nil {
		return err
	}
	if c.staleSets, err = metric.RegisterOrGet(registry, metricService, "stale_sets_total", c.staleSets); err != nil {
		return err
	}
	if c.duplicateTriplesSuppressed, err = metric.RegisterOrGet(registry, metricService, "duplicate_triples_suppressed_total", c.duplicateTriplesSuppressed); err != nil {
		return err
	}
	if c.poisonedEntities, err = metric.RegisterOrGet(registry, metricService, "poisoned_entities", c.poisonedEntities); err != nil {
		return err
	}
	return nil
}

// Register registers the graph-ingest factory with the component registry
func Register(registry *component.Registry) error {
	return registry.RegisterFactory("graph-ingest", &component.Registration{
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
// Discoverable Interface (6 methods)
// ============================================================================

// Meta returns component metadata
func (c *Component) Meta() component.Metadata {
	return component.Metadata{
		Name:        "graph-ingest",
		Type:        "processor",
		Description: "Entity and triple ingestion processor for graph system",
		Version:     "1.0.0",
	}
}

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

// ConfigSchema returns the configuration schema
func (c *Component) ConfigSchema() component.ConfigSchema {
	return schema
}

// Health returns current health status
func (c *Component) Health() component.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	uptime := time.Duration(0)
	if c.running && !c.startTime.IsZero() {
		uptime = time.Since(c.startTime)
	}

	errorCount := int(atomic.LoadInt64(&c.errors))
	lastErr := ""
	status := "stopped"

	if c.running {
		status = "running"
		// A non-empty poison inventory degrades Health per-entity: status is
		// "degraded", NOT "reset_required" (that stays a per-read error code —
		// design D6), and the message carries count + a bounded ID/reason
		// sample. Full enumeration rides DebugStatus.
		if c.entityPoisonSize.Load() > 0 {
			status = graph.IndexStateDegraded
			lastErr = c.entityPoisonHealthMessage()
		} else if c.entityWatchLost.Load() {
			status = graph.IndexStateDegraded
			lastErr = graph.ErrorCodeIndexNotReady + ": ENTITY_STATES snapshot sweep unavailable"
		} else if errorCount > 0 {
			lastErr = "errors occurred during processing"
		}
	}

	return component.HealthStatus{
		Healthy: c.running && errorCount == 0 && c.entityPoisonSize.Load() == 0 &&
			!c.entityWatchLost.Load() && (!c.entityBootstrapStarted.Load() || c.entityBootstrapComplete.Load()),
		LastCheck:  time.Now(),
		ErrorCount: errorCount,
		LastError:  lastErr,
		Uptime:     uptime,
		Status:     status,
	}
}

// DataFlow returns current data flow metrics
func (c *Component) DataFlow() component.FlowMetrics {
	messages := atomic.LoadInt64(&c.messagesProcessed)
	bytes := atomic.LoadInt64(&c.bytesProcessed)
	errorCount := atomic.LoadInt64(&c.errors)

	c.mu.RLock()
	uptime := time.Duration(0)
	if c.running && !c.startTime.IsZero() {
		uptime = time.Since(c.startTime)
	}
	c.mu.RUnlock()

	// Calculate rates
	var messagesPerSec, bytesPerSec, errorRate float64
	if uptime > 0 {
		seconds := uptime.Seconds()
		messagesPerSec = float64(messages) / seconds
		bytesPerSec = float64(bytes) / seconds
		if messages > 0 {
			errorRate = float64(errorCount) / float64(messages)
		}
	}

	lastAct := time.Now()
	if stored := c.lastActivity.Load(); stored != nil {
		if t, ok := stored.(time.Time); ok {
			lastAct = t
		}
	}

	return component.FlowMetrics{
		MessagesPerSecond: messagesPerSec,
		BytesPerSecond:    bytesPerSec,
		ErrorRate:         errorRate,
		LastActivity:      lastAct,
	}
}

// ============================================================================
// LifecycleComponent Interface (3 methods)
// ============================================================================

// Initialize validates configuration and sets up ports (no I/O)
func (c *Component) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil // Idempotent
	}

	// Validate configuration
	if err := c.config.Validate(); err != nil {
		return errs.Wrap(err, "Component", "Initialize", "config validation")
	}

	c.initialized = true
	c.logger.Info("component initialized", slog.String("component", "graph-ingest"))

	return nil
}

// Start begins processing (must be initialized first). The lifecycle state is the
// guard's (internal/lifecycleguard, design D13): a nil or ended context and a second
// Start are refused before anything is acquired; a Start that fails is rolled back
// through release, and a rollback that fails leaves release pending for the next Stop.
// An uninitialized component, or one with no NATS client, is refused before the guard
// is consulted, so a later Start after Initialize is still admitted.
func (c *Component) Start(ctx context.Context) error {
	c.mu.RLock()
	initialized := c.initialized
	c.mu.RUnlock()
	if !initialized {
		return errs.WrapFatal(fmt.Errorf("component not initialized"), "Component", "Start", "initialization check")
	}
	if c.natsClient == nil {
		return errs.WrapFatal(errs.ErrNoConnection, "Component", "Start", "NATS client is required")
	}
	return c.lifecycle.Start(ctx, c.start, c.release)
}

// start acquires everything Start owns, under the guard. Each handle it takes is
// recorded before the next acquisition, so release can return whatever a failed start
// left behind.
func (c *Component) start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	poolCtx, poolCancel := context.WithCancel(runCtx)
	submitCtx, submitCancel := context.WithCancel(runCtx)
	c.handlesMu.Lock()
	c.cancel = cancel
	c.ingestPoolCancel = poolCancel
	c.ingestSubmitCancel = submitCancel
	c.handlesMu.Unlock()

	if c.natsClient.Status() != natsclient.StatusConnected {
		if err := c.natsClient.Connect(runCtx); err != nil {
			if runCtx.Err() != nil {
				return errs.Wrap(runCtx.Err(), "Component", "Start", "context cancelled during NATS connection")
			}
			return errs.Wrap(err, "Component", "Start", "NATS connection failed")
		}
		if err := c.natsClient.WaitForConnection(runCtx); err != nil {
			if runCtx.Err() != nil {
				return errs.Wrap(runCtx.Err(), "Component", "Start", "context cancelled waiting for NATS")
			}
			return errs.Wrap(err, "Component", "Start", "wait for NATS connection")
		}
	}

	if err := c.initStorage(runCtx); err != nil {
		return err
	}
	c.startEntityStateGuard(runCtx, c.entityBucket)
	c.initHierarchyInference()
	if err := c.createStatusBucket(runCtx); err != nil {
		return errs.Wrap(err, "Component", "Start", "readiness status bucket")
	}
	if err := c.buildIngestPool(poolCtx); err != nil {
		return errs.Wrap(err, "Component", "Start", "keyed ingest pool")
	}
	if err := c.setupSubscriptions(runCtx, submitCtx); err != nil {
		return errs.Wrap(err, "Component", "Start", "subscription setup")
	}
	if err := c.setupQueryHandlers(runCtx); err != nil {
		return errs.Wrap(err, "Component", "Start", "query handler setup")
	}
	if err := c.setupMutationHandlers(runCtx); err != nil {
		return errs.Wrap(err, "Component", "Start", "mutation handler setup")
	}

	c.startStatusMetricsLoop(runCtx)
	c.mu.Lock()
	c.running = true
	c.startTime = time.Now()
	startTime := c.startTime
	c.mu.Unlock()
	c.logger.Info("component started",
		slog.String("component", "graph-ingest"),
		slog.Time("start_time", startTime))
	return nil
}

func (c *Component) startStatusMetricsLoop(ctx context.Context) {
	statusDone := make(chan struct{})
	c.handlesMu.Lock()
	c.statusDone = statusDone
	c.handlesMu.Unlock()
	go func() {
		defer close(statusDone)
		c.statusMetricsLoop(ctx)
	}()
}

// Stop gracefully shuts down the component through the guard: Stop before Start
// releases nothing; Stop during Start waits for it, or returns ctx's error first; a
// Stop whose release fails keeps it pending, and the next Stop runs it again; after a
// release that succeeded, Stop returns nil and runs nothing.
func (c *Component) Stop(ctx context.Context) error {
	return c.lifecycle.Stop(ctx, func(ctx context.Context) error {
		if err := c.release(ctx); err != nil {
			return err
		}
		c.logger.Info("component stopped gracefully", slog.String("component", "graph-ingest"))
		return nil
	})
}

// release is the cleanup the guard runs, for a Stop or for a failed Start's rollback.
// The handles stay set when cleanup fails, so the next Stop releases them again; they
// are cleared only by a cleanup that succeeded (design D22). A consumer whose drain was
// already issued is not drained again (drainIssued).
func (c *Component) release(ctx context.Context) error {
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	if err := c.cleanup(ctx); err != nil {
		return err
	}
	c.handlesMu.Lock()
	c.clearLifecycleHandles()
	c.handlesMu.Unlock()
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
	c.entityCache = nil
	c.statusPublisher = nil
	c.clearBoundConsumers()
}

// initStorage initializes KV buckets and query caches. Every bucket this
// component owns is acquired through the catalog seam
// (natsclient.EnsureFrameworkBucket): create-or-open, reconcile to the
// declared policy (a foreign TTL is stripped, an adopted divergent History is
// converged), verify, or fail this Start closed — which the composition
// root's component-start barrier turns into a failed boot.
func (c *Component) initStorage(ctx context.Context) error {
	// Entity states KV bucket - we are the WRITER (catalog owner).
	bucket, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketEntityStates)
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "KV bucket creation")
	}
	c.entityBucket = c.natsClient.NewKVStore(bucket)

	// Entity query cache (HybridCache: LRU capacity + TTL freshness)
	entityCache, err := cache.NewFromConfig[graph.EntityState](ctx, cache.Config{
		Enabled:         true,
		Strategy:        cache.StrategyHybrid,
		MaxSize:         5000,
		TTL:             30 * time.Second,
		CleanupInterval: 10 * time.Second,
	}, cache.WithMetrics[graph.EntityState](c.metricsRegistry, "entity_query_cache"))
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "entity cache creation")
	}
	c.entityCache = entityCache

	// ADR-072 redelivery-guard durable tier: graph-ingest-owned bucket mapping
	// `(entityID/streamName) → last-applied stream sequence`. Operational state
	// graph-ingest owns (bucket-ownership rubric), a bare uint64 value — no
	// cross-repo/EntityState schema change. No TTL: correct for MaxDeliver=0
	// (unlimited redelivery), and the key set is bounded by entity cardinality
	// (same order as ENTITY_STATES).
	guardBucket, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketGraphIngestAppliedSeq)
	if err != nil {
		return errs.Wrap(err, "Component", "Start", "redelivery guard bucket creation")
	}
	c.ingestGuardBucket = c.natsClient.NewKVStore(guardBucket)

	return nil
}

// startEntityStateGuard synchronously validates the resident ENTITY_STATES
// snapshot into the per-entity poison inventory, then STOPS the watcher —
// graph-ingest holds no steady-state self-watch on the bucket it writes
// (poison-response-scoping D1; steady-state detection rides the read points
// that already exist). It intentionally does not fail startup on transport
// errors: this component is the canonical writer and must remain available,
// while every query fails closed until the sweep completes.
//
// Snapshot completeness leans on ENTITY_STATES History=1: with a per-subject
// limit of 1 the pre-marker replay carries exactly the latest revision per
// key, so gap-resets cannot inflate the received set and the nil
// end-of-snapshot marker means the full resident state was seen. Raising the
// bucket's history depth invalidates this marker math.
func (c *Component) startEntityStateGuard(ctx context.Context, bucket *natsclient.KVStore) {
	c.entityBootstrapStarted.Store(true)
	watcher, err := bucket.Watch(ctx, ">")
	if err != nil {
		if ctx.Err() == nil {
			c.markEntityWatchLost()
			c.logger.Error("failed to start ENTITY_STATES snapshot sweep watcher", slog.Any("error", err))
		}
		return
	}

	// Drain bookkeeping is LAST-REVISION-WINS per key: a poisoned revision
	// superseded by a valid (or tombstoned) pre-marker revision of the same
	// key ends with no inventory entry.
	sweep := make(map[string]entityPoisonRecord)
	updates := watcher.Updates()
	for {
		select {
		case <-ctx.Done():
			c.stopEntityStateGuardWatcher(watcher, updates)
			return
		case entry, ok := <-updates:
			if !ok {
				// PRE-marker channel closure is a genuine transport failure:
				// ingest writers still boot, entity queries stay not-ready
				// (transient), and no poison is recorded from the failure.
				_ = watcher.Stop()
				if ctx.Err() == nil {
					c.markEntityWatchLost()
				}
				return
			}
			if entry == nil {
				// End-of-snapshot marker: the full resident snapshot was seen.
				// Stop deliberately (never classified as watch loss), record
				// the surviving sweep entries, then open the query surface.
				c.stopEntityStateGuardWatcher(watcher, updates)
				c.recordBootSweepPoison(ctx, sweep)
				c.entityBootstrapComplete.Store(true)
				return
			}
			c.sweepEntityStateGuardEntry(sweep, entry)
		}
	}
}

// stopEntityStateGuardWatcher performs the deliberate snapshot-watcher stop
// (design D1 mandatory shape): Stop(), then KEEP READING the updates channel
// until nats.go closes it, discarding entries. The nats.go update callback
// blocks on a full channel while holding the watcher mutex, so an unread
// channel would wedge the connection's async-callback dispatcher (via
// SetClosedHandler). The post-Stop closure here is deliberate and MUST NOT be
// classified as watch loss.
func (c *Component) stopEntityStateGuardWatcher(watcher jetstream.KeyWatcher, updates <-chan jetstream.KeyValueEntry) {
	_ = watcher.Stop()
	discarded := 0
	for range updates {
		discarded++ // deliberate drain-to-close; entries are discarded unvalidated
	}
	if discarded > 0 {
		c.logger.Debug("snapshot sweep watcher drained to close",
			slog.Int("discarded_entries", discarded))
	}
}

// sweepEntityStateGuardEntry folds one pre-marker snapshot delivery into the
// sweep map, last-revision-wins per key: a valid or tombstoned delivery erases
// an earlier poisoned one for the same key.
func (c *Component) sweepEntityStateGuardEntry(sweep map[string]entityPoisonRecord, entry jetstream.KeyValueEntry) {
	if kvcatalog.IsKVTombstone(entry.Operation()) {
		delete(sweep, entry.Key()) // key deleted: nothing resident to inventory
		return
	}
	var state graph.EntityState
	err := graph.UnmarshalEntityState(entry.Value(), &state)
	if err == nil {
		delete(sweep, entry.Key()) // valid revision supersedes an earlier poisoned one
		return
	}
	var contractErr *graph.StateContractError
	if !errors.As(err, &contractErr) {
		contractErr = &graph.StateContractError{Reason: graph.GraphStateReasonUnreadableEntity, Err: err}
	}
	contractErr.EntityID = entry.Key()
	sweep[entry.Key()] = entityPoisonRecord{contractErr: contractErr, revision: entry.Revision()}
}

// recordBootSweepPoison moves the surviving sweep entries into the poison
// inventory. The first bootSweepErrorLogCap new entries log individually as
// structured ERRORs; the remainder is summarized by one count-only WARN so a
// mass-poison boot cannot flood the log.
func (c *Component) recordBootSweepPoison(ctx context.Context, sweep map[string]entityPoisonRecord) {
	if len(sweep) == 0 {
		return
	}
	keys := make([]string, 0, len(sweep))
	for key := range sweep {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	logged := 0
	for _, key := range keys {
		rec := sweep[key]
		if c.recordEntityPoison(rec.contractErr, rec.revision) {
			if logged < bootSweepErrorLogCap {
				c.logEntityPoisonRecorded(rec.contractErr, rec.revision)
				logged++
			}
			// Close the cross-process record/repair window: a repair that
			// completed after this key's snapshot delivery found no entry to
			// clear; verifying against current bytes drops the stale record.
			c.verifyEntityPoisonRecord(ctx, key)
		}
	}
	if len(keys) > logged {
		c.logger.Warn("boot snapshot sweep: additional poisoned entities inventoried past the log cap",
			slog.Int("total", len(keys)),
			slog.Int("logged_individually", logged))
	}
}

// classifyStoredStateRMWError re-attributes a read-modify-write failure to
// resident stored-state poison (gh#562). The owner's own RMW reads check only
// the trusted decode and the key (decodeStoredForWrite, design D23), so a
// noncanonical STORED statement does not fail on the read — it surfaces either
// as a trusted-decode failure (unreadable JSON) or as a MarshalEntityState
// write-gate rejection that would otherwise blame the merged CANDIDATE the
// caller submitted. On this slow path (the write is already failing)
// re-validate the stored bytes: when the poison predates the merge, stamp the
// RMW target's entity ID, record it in the per-entity poison inventory at
// revision, the revision the RMW read current at, and return the
// graph-state-reset-required classification instead of a candidate-invalid
// error. A canonical stored state returns cycleErr unchanged — the candidate
// really was at fault.
//
// entityID is the RMW target (the CAS key) — the closures are the only place
// the identity is reliably in scope, so stamping happens here rather than at
// any outer helper (design D4).
func (c *Component) classifyStoredStateRMWError(ctx context.Context, entityID string, current []byte, revision uint64, cycleErr error) error {
	var stored graph.EntityState
	err := graph.UnmarshalEntityState(current, &stored)
	if err == nil {
		return cycleErr
	}
	var contractErr *graph.StateContractError
	if errors.As(err, &contractErr) {
		contractErr.EntityID = entityID
		c.inventoryEntityPoison(ctx, contractErr, revision)
	}
	return err
}

func (c *Component) markEntityWatchLost() {
	c.entityWatchLost.Store(true)
}

// initHierarchyInference initializes hierarchy inference if enabled.
func (c *Component) initHierarchyInference() {
	if !c.config.EnableHierarchy {
		return
	}

	// Enable sibling edges by default, can be disabled via config
	enableTypeSiblings := true
	if c.config.EnableTypeSiblings != nil {
		enableTypeSiblings = *c.config.EnableTypeSiblings
	}

	hierarchyConfig := inference.HierarchyConfig{
		Enabled:            true,
		CreateTypeEdges:    true,
		CreateSystemEdges:  true,
		CreateDomainEdges:  true,
		CreateTypeSiblings: enableTypeSiblings,
	}

	// The deployment's own authority, the same deps.Platform the gate reads:
	// containers and sibling edges are minted from the ingested entity's prefix,
	// so an imported entity would mint them under a peer's authority. Inference
	// skips those entities entirely (ADR-102).
	c.hierarchyInference = inference.NewHierarchyInference(
		&entityManagerAdapter{component: c},
		&tripleAdderAdapter{component: c},
		hierarchyConfig,
		c.platform,
		c.logger,
	)
}

// ============================================================================
// Subscription Management
// ============================================================================

// setupSubscriptions sets up JetStream consumers for input ports
func (c *Component) setupSubscriptions(ctx, submitCtx context.Context) error {
	for _, port := range c.inputs {
		facts, err := port.Facts()
		if err != nil {
			return err
		}
		if facts.Kind() != component.PortKindJetStream {
			continue
		}

		if err := c.setupJetStreamConsumer(ctx, submitCtx, port); err != nil {
			return errs.Wrap(err, "Component", "setupSubscriptions",
				fmt.Sprintf("JetStream consumer for %s", port.Name))
		}
	}
	return nil
}

// setupJetStreamConsumer creates a JetStream consumer for an input port
func (c *Component) setupJetStreamConsumer(ctx, submitCtx context.Context, port component.Port) error {
	facts, err := port.Facts()
	if err != nil {
		return err
	}
	stream, ok := facts.Stream()
	if !ok || len(stream.Subjects()) != 1 {
		return fmt.Errorf("port %s must declare one JetStream subject", port.Name)
	}
	subject := stream.Subjects()[0]
	streamName := stream.Name()
	// The operator's import declaration is read from the PORT, once, here — a
	// message can never claim it (ADR-102 d5).
	importLane := stream.Import()

	// Wait for stream to be available
	waitForStream := c.waitForStream
	if c.waitForStreamInput != nil {
		waitForStream = c.waitForStreamInput
	}
	if err := waitForStream(ctx, streamName); err != nil {
		return fmt.Errorf("stream %s not available: %w", streamName, err)
	}

	// Generate unique consumer name
	sanitizedSubject := strings.ReplaceAll(subject, ".", "-")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, "*", "all")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, ">", "wildcard")
	consumerName := fmt.Sprintf("graph-ingest-%s", sanitizedSubject)

	c.logger.Debug("Setting up JetStream consumer",
		slog.String("stream", streamName),
		slog.String("consumer", consumerName),
		slog.String("filter_subject", subject))

	// Get consumer config from port definition (allows user configuration)
	// graph-ingest is idempotent (ENTITY_STATES merge/CAS overwrites), so it
	// defaults to "all": it MUST catch up on entities published before its
	// consumer bound. A JSON config omitting deliver_policy would otherwise fall
	// to the framework "new" default and silently drop the first entities (the
	// startup first-message race). An explicit deliver_policy still wins.
	consumerCfg, consumerErr := component.GetConsumerConfig(port)
	if consumerErr != nil {
		return fmt.Errorf("resolve JetStream consumer config for port %q: %w", port.Name, consumerErr)
	}
	if stream.DeliverPolicy() == "" {
		consumerCfg.DeliverPolicy = "all"
	}

	cfg := natsclient.StreamConsumerConfig{
		StreamName:    streamName,
		ConsumerName:  consumerName,
		FilterSubject: subject,
		DeliverPolicy: consumerCfg.DeliverPolicy,
		AckPolicy:     consumerCfg.AckPolicy,
		MaxDeliver:    consumerCfg.MaxDeliver,
		MaxAckPending: consumerCfg.MaxAckPending, // gh#480: 0 leaves inherited/default/capped policy to NATS
		AutoCreate:    false,
	}

	consumeStream := c.natsClient.ConsumeStreamWithConfig
	if c.consumeStream != nil {
		consumeStream = c.consumeStream
	}
	handle, err := consumeStream(ctx, natsclient.PortConsumerContext{Component: c.Meta().Name, Port: port.Name}, cfg, func(_ context.Context, msg jetstream.Msg) {
		// ADR-072: the consume closure decodes ONCE and submits to the keyed pool;
		// the redelivery guard, apply, and ack move into processIngest (run on a
		// pool lane, not this consumer goroutine). Metadata carries the stream name
		// + sequence the guard keys on; a message without it can't be keyed/guarded.
		meta, metaErr := msg.Metadata()
		if metaErr != nil {
			c.logger.Warn("graph-ingest: message missing JetStream metadata; dropping",
				slog.String("subject", subject), slog.Any("error", metaErr))
			atomic.AddInt64(&c.errors, 1)
			if ackErr := msg.Ack(); ackErr != nil {
				c.logger.Error("Failed to ack metadata-less message", slog.Any("error", ackErr))
			}
			return
		}
		// gh#480 queue-wait (message age when we reach it). Host clock skew can make
		// this marginally negative on a co-located deploy — harmless (skews _sum only).
		if !meta.Timestamp.IsZero() {
			c.ingestLag.Observe(time.Since(meta.Timestamp).Seconds())
		}
		// Decode + extract ONCE (KeyOf reads the already-parsed entity.ID; the lane
		// reuses the parsed entity — no double parse). A decode/extract failure is a
		// poison message: count + ack-drop (today's behavior).
		entity, derr := c.decodeEntity(subject, msg.Data())
		if derr != nil {
			// A refused statement (design D15) is this message's own fault and can never be
			// admitted: it is terminated, counted and logged as processIngest does a
			// structurally invalid candidate, not acked as an undecodable message is.
			var refusal *statementRefusal
			if errors.As(derr, &refusal) {
				c.recordStructuralRejection("graphable", derr)
				if termErr := msg.Term(); termErr != nil {
					c.logger.Error("Failed to terminate structurally invalid ingest", slog.Any("error", termErr))
				}
				return
			}
			c.logger.Warn("graph-ingest: decode/extract failed; dropping",
				slog.String("subject", subject), slog.Any("error", derr))
			atomic.AddInt64(&c.errors, 1)
			if ackErr := msg.Ack(); ackErr != nil {
				c.logger.Error("Failed to ack poison message", slog.Any("error", ackErr))
			}
			return
		}
		// Submit on the dedicated submit ctx (NOT msgCtx — a block past AckWait would
		// otherwise surface as an ignored error → message neither enqueued nor acked).
		// A submit failure MUST Nak so the server redelivers (ADR-072 B1), never
		// silent-drop.
		work := ingestWork{
			entity:      entity,
			msg:         msg,
			entityID:    entity.ID,
			stream:      meta.Stream,
			seq:         meta.Sequence.Stream,
			deliveredAt: meta.Timestamp,
			subject:     subject,
			importLane:  importLane,
		}
		if serr := c.ingestPool.SubmitBlocking(submitCtx, work); serr != nil {
			if nakErr := msg.Nak(); nakErr != nil {
				c.logger.Error("Failed to Nak after submit failure", slog.Any("error", nakErr))
			}
		}
	})
	if err != nil {
		return fmt.Errorf("consumer setup failed for stream %s: %w", streamName, err)
	}
	c.handlesMu.Lock()
	c.consumers = append(c.consumers, graphIngestConsumerBinding{handle: handle})
	c.handlesMu.Unlock()

	// Record the consumer for the readiness tick ONLY after the bind committed, so
	// the backlog sum never asks about a consumer that failed to bind (which would
	// degrade the envelope on a consumer that does not exist).
	c.registerBoundConsumer(streamName, consumerName)

	// Capture this port's contribution to BootstrapScope NOW, while the backlog is
	// still whole. See recordBindBacklog: reading it at the first status tick lets a
	// fast-draining backlog report "nothing to do".
	c.recordBindBacklog(ctx, streamName, consumerName)

	c.logger.Debug("graph-ingest subscribed (JetStream)",
		slog.String("subject", subject),
		slog.String("stream", streamName))
	return nil
}

// waitForStream waits for a JetStream stream to be available
func (c *Component) waitForStream(ctx context.Context, streamName string) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return fmt.Errorf("failed to get JetStream context: %w", err)
	}

	maxRetries := 30
	retryInterval := 100 * time.Millisecond
	maxInterval := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		_, err := js.Stream(ctx, streamName)
		if err == nil {
			c.logger.Debug("Stream available", slog.String("stream", streamName))
			return nil
		}

		// Exponential backoff
		c.logger.Debug("Waiting for stream",
			slog.String("stream", streamName),
			slog.Int("attempt", i+1),
			slog.Duration("interval", retryInterval))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
			retryInterval = time.Duration(float64(retryInterval) * 1.5)
			if retryInterval > maxInterval {
				retryInterval = maxInterval
			}
		}
	}

	return fmt.Errorf("stream %s not available after %d retries", streamName, maxRetries)
}

// handleMessage processes an incoming message and creates/updates entity state
func (c *Component) handleMessage(ctx context.Context, subject string, data []byte) {
	c.logger.Debug("Received message",
		slog.String("subject", subject),
		slog.Int("size", len(data)))

	entity, err := c.decodeEntity(subject, data)
	if err != nil {
		c.logger.Warn("Failed to decode/extract message",
			slog.String("subject", subject),
			slog.Any("error", err))
		atomic.AddInt64(&c.errors, 1)
		return
	}

	_ = c.ingestEntity(ctx, entity, false)
}

// decodeEntity decodes a message's bytes into an EntityState: unmarshal the
// BaseMessage envelope, then extract the Graphable payload's entity. Shared by
// the consume closure (decode-once, then submit to the keyed pool) and
// handleMessage (the synchronous test/compat path).
func (c *Component) decodeEntity(subject string, data []byte) (*graph.EntityState, error) {
	baseMsg, err := c.decoder.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode base message (subject %s): %w", subject, err)
	}
	entity, err := c.extractEntityFromMessage(baseMsg)
	if err != nil {
		return nil, fmt.Errorf("extract entity (subject %s): %w", subject, err)
	}
	return entity, nil
}

// ingestEntity merges one Graphable projection into its authority entity.
// importLane is the arrival port's declared lane (ADR-102 d5); the synchronous
// compat entry point and every direct caller pass false.
func (c *Component) ingestEntity(ctx context.Context, entity *graph.EntityState, importLane bool) error {
	if err := c.prepareFactProjection(entity, importLane); err != nil {
		return err
	}

	// Structural-identity predicate gate on the Graphable ingest path
	// (unconditionally fail-closed; the primary vector for product/source-authored
	// predicates). On
	// this lane it is a defense-in-depth backstop: prepareFactProjection above has
	// already rejected any non-canonical predicate via the authoritative
	// ValidateEntityStateContract, so this call cannot fire unless that seam moves.
	if err := c.validateTriplePredicates(entity.Triples); err != nil {
		return err
	}

	// Store entity in KV bucket — MERGE semantics (gh#177). Earlier code
	// used CreateEntity (Put = full-replace) here, which clobbered any
	// pre-existing triples on the entity. The Graphable lane merges while the
	// four explicit mutation operations apply their own strict semantics; the
	// JetStream consumer path previously replaced all state and silently
	// erased lifecycle-managed entity state on every subsequent
	// Graphable arrival.
	if err := c.mergeEntityOnLane(ctx, entity, importLane); err != nil {
		c.logger.Error("Failed to merge entity",
			slog.String("entity_id", entity.ID),
			slog.Any("error", err))
		return err
	}

	c.logger.Debug("Entity ingested",
		slog.String("entity_id", entity.ID),
		slog.Int("triples", len(entity.Triples)))
	return nil
}

// prepareFactProjection applies the Graphable lane's single projection
// convenience and validates the complete candidate. processIngest calls this
// before its redelivery guard so malformed identity cannot become a guard key
// or trigger guard I/O; ingestEntity calls it as the reusable direct-entry
// safety net. The operation is idempotent.
func (c *Component) prepareFactProjection(entity *graph.EntityState, importLane bool) error {
	if entity == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "ingestEntity", "entity cannot be nil")
	}
	if err := validateEntityID(entity.ID); err != nil {
		return errs.WrapInvalid(&graph.EntityStateContractError{
			Field:       graph.EntityStateContractFieldID,
			TripleIndex: -1,
			Err:         err,
		}, "Component", "ingestEntity", "validate envelope entity ID")
	}
	// Authority gate (ADR-102 d5), before the projection fill and before any KV
	// I/O. importLane is the arrival port's own declaration, carried from the
	// consume closure; a direct caller supplies false.
	if err := c.authorizeSubject(entity.ID, importLane); err != nil {
		return err
	}

	// Graphable projection convenience: an omitted subject means the envelope
	// entity. Fill only this fact-arrival lane, before the authoritative seam;
	// mutation, direct persistence, and replay receive no such fill.
	entity.Triples = fillFactProjectionSubjects(entity.ID, entity.Triples)
	for index, triple := range entity.Triples {
		if triple.Subject != entity.ID {
			return errs.WrapInvalid(
				fmt.Errorf("triple[%d] subject %q does not match Graphable entity %q", index, triple.Subject, entity.ID),
				"Component", "ingestEntity", "validate projected entity subject",
			)
		}
	}

	// Preflight the complete Graphable projection before persistence.
	if err := graph.ValidateEntityStateContract(entity); err != nil {
		return errs.WrapInvalid(err, "Component", "ingestEntity", "validate projected entity state")
	}
	return nil
}

// fillFactProjectionSubjects returns triples with each omitted Subject filled
// from the exact Graphable envelope ID. It copies only when a fill is needed
// and never repairs or rewrites non-empty subject bytes.
func fillFactProjectionSubjects(entityID string, triples []message.Triple) []message.Triple {
	var filled []message.Triple
	for index := range triples {
		if triples[index].Subject != "" {
			continue
		}
		if filled == nil {
			filled = append([]message.Triple(nil), triples...)
		}
		filled[index].Subject = entityID
	}
	if filled != nil {
		return filled
	}
	return triples
}

// extractEntityFromMessage extracts an EntityState from a BaseMessage
func (c *Component) extractEntityFromMessage(msg *message.BaseMessage) (*graph.EntityState, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	payload := msg.Payload()
	if payload == nil {
		return nil, fmt.Errorf("message has no payload")
	}

	// Check if payload implements Graphable
	graphable, ok := payload.(graph.Graphable)
	if !ok {
		return nil, fmt.Errorf("payload does not implement Graphable interface")
	}

	// Get entity ID and triples from Graphable
	entityID := graphable.EntityID()
	if entityID == "" {
		return nil, fmt.Errorf("graphable payload returned empty entity ID")
	}

	// Statement metadata (design D15, #98): a payload with no statement refuses the message,
	// a statement without a Source or Timestamp takes the envelope's, and a statement under a
	// reserved source refuses the message. This reads only the payload's statements, before the
	// indexing-profile statement graph-ingest stamps below under its own reserved source.
	own := graphable.Triples()
	if len(own) == 0 {
		return nil, refuseNoStatement()
	}
	triples, err := stampFromEnvelope(own, msg.Meta())
	if err != nil {
		return nil, err
	}

	// Build EntityState
	entity := &graph.EntityState{
		ID:          entityID,
		Triples:     triples,
		MessageType: msg.Type(),
	}

	// ADR-055 §5 StorageRef extraction (consumer half): a Graphable payload that
	// also implements message.Storable carries an ObjectStore reference to its
	// offloaded content. Lift it onto the EntityState so mergeEntityOnLane persists it
	// (the create branch marshals it verbatim; the existing-entity branch merges
	// it onto the stored copy). Without this, a ContentStorable producer's offloaded
	// content (tool results, model responses, document bodies) loses its storage
	// pointer at the ingest seam and the embedding worker can never find the
	// text — the "offloaded content stops embedding" gap.
	if storable, ok := payload.(message.Storable); ok {
		if ref := storable.StorageRef(); ref != nil {
			entity.StorageRef = ref
		}
	}

	// ADR-054 channel (a): a Graphable payload MAY also declare its indexing
	// profile via the optional IndexingProfiler interface. Stamp it explicitly
	// here so it's present on entity.Triples by the time mergeEntityOnLane reaches
	// its create seam; absence (or an invalid value) falls through to the
	// fallback floor there. It carries the time read from the stamped statements
	// before it is added (design D15), the time mergeEntityOnLane reads again.
	if profiler, ok := payload.(message.IndexingProfiler); ok {
		stampExplicitIndexingProfile(entity, profiler.IndexingProfile(), triggeringTime(entity))
	}

	return entity, nil
}

// hasIndexingProfileTriple reports whether the entity already carries a
// create-time indexing profile. That profile is immutable (ADR-054), so a
// later Graphable arrival must not override it through the stream lane's replace.
func hasIndexingProfileTriple(entity *graph.EntityState) bool {
	for _, t := range entity.Triples {
		if t.Predicate == vocabulary.EntityIndexingProfile {
			return true
		}
	}
	return false
}

// triplesWithoutPredicate returns triples with every triple carrying predicate
// removed. Non-mutating; used to drop an incoming immutable predicate before the
// stream lane's replace.
func triplesWithoutPredicate(triples []message.Triple, predicate string) []message.Triple {
	out := make([]message.Triple, 0, len(triples))
	for _, t := range triples {
		if t.Predicate != predicate {
			out = append(out, t)
		}
	}
	return out
}

// removeIndexingProfileTriples drops every entity.indexing.profile triple from
// the entity (the single-valued predicate is replace-on-write, never appended).
func removeIndexingProfileTriples(entity *graph.EntityState) {
	filtered := entity.Triples[:0]
	for _, t := range entity.Triples {
		if t.Predicate != vocabulary.EntityIndexingProfile {
			filtered = append(filtered, t)
		}
	}
	entity.Triples = filtered
}

// triggeringTime is the time every statement graph-ingest derives for a write carries (the
// indexing profile, hierarchy statements, a hierarchy container's statements): the latest
// Timestamp among entity's statements, never the clock (design D15; owner ruling, #91 comment
// 6066791396). Each lane reads it from the write's own statements before graph-ingest adds one
// of its own, and reads all of them, reserved sources included: a hierarchy container's
// statements are all graph-ingest-hierarchy, and pkg/lifecycle writes under its own. A
// statement graph-ingest has added already carries this time, so reading after it gives the
// same answer. Every lane refuses a write that carries no statement before it reads this, so
// the time it returns is never zero.
func triggeringTime(entity *graph.EntityState) time.Time {
	var latest time.Time
	for _, triple := range entity.Triples {
		if triple.Timestamp.After(latest) {
			latest = triple.Timestamp
		}
	}
	return latest
}

// appendIndexingProfileTriple appends one entity.indexing.profile triple carrying at,
// the write's triggeringTime. Callers are responsible for having removed any prior value
// first.
func appendIndexingProfileTriple(entity *graph.EntityState, profile string, at time.Time) {
	entity.Triples = append(entity.Triples, message.Triple{
		Subject:    entity.ID,
		Predicate:  vocabulary.EntityIndexingProfile,
		Object:     profile,
		Source:     "graph-ingest-indexing-profile",
		Timestamp:  at,
		Confidence: 1.0,
	})
}

// stampExplicitIndexingProfile sets entity.indexing.profile to an explicitly
// declared profile (replace-on-write, single-valued). Empty or unrecognized
// values are ignored — the entity then falls through to the fallback floor at
// its create seam rather than failing (lenient Phase 1 semantics). Used by the
// Graphable IndexingProfiler channel (extractEntityFromMessage) and the
// mutation-envelope channel (handleEntityCreateWithTriples). The statement carries at, the
// write's triggeringTime.
func stampExplicitIndexingProfile(entity *graph.EntityState, profile string, at time.Time) {
	if entity == nil || !vocabulary.IsValidIndexingProfile(profile) {
		return
	}
	removeIndexingProfileTriples(entity)
	appendIndexingProfileTriple(entity, profile, at)
}

// reconcileIndexingProfile is the entity-CREATION-seam stamp (ADR-054 §5). It
// runs at every place an entity is born — createEntity, mergeEntityOnLane's
// first-write branch, and the stub→real upgrade in mergeEntityOnLane's merge branch —
// and never on a plain update of an already-profiled entity, so a profile is
// immutable once set. It enforces the single-valued invariant and applies the
// fallback floor when nothing was declared:
//
//   - ≥1 profile triple present (an explicit declaration was stamped upstream,
//     or an incoming producer is supplying one to an existing unprofiled entity): keep the FIRST and
//     drop any duplicates. No floor, no metric.
//   - 0 profile triples present: apply the floor registered with the type
//     (Registration.IndexingProfile, ADR-103) and append it; increment
//     indexing_profile_default_total{message_type} ONLY when the registered type
//     declares no floor (control is the fail-safe default). The create seams
//     refuse an unregistered type before this runs.
//
// The floor statement carries at, the write's triggeringTime.
func (c *Component) reconcileIndexingProfile(entity *graph.EntityState, at time.Time) {
	if entity == nil {
		return
	}
	kept := false
	filtered := entity.Triples[:0]
	for _, t := range entity.Triples {
		if t.Predicate == vocabulary.EntityIndexingProfile {
			if kept {
				continue // single-valued: drop duplicates, keep the first
			}
			kept = true
		}
		filtered = append(filtered, t)
	}
	entity.Triples = filtered
	if kept {
		return
	}
	// No explicit declaration → apply the floor registered with the type
	// (ADR-054 channel c, ADR-103). The default-fallback metric fires ONLY when
	// the registered type declares no floor and silently took the control
	// default. A registered floor (e.g. agentic.request → trace) is a deliberate
	// classification, not an operator gap; the label now points at a
	// Registration literal.
	profile, floored := c.registeredIndexingProfile(entity.MessageType)
	appendIndexingProfileTriple(entity, profile, at)
	if !floored && c.indexingProfileDefault != nil {
		c.indexingProfileDefault.WithLabelValues(indexingProfileMetricLabel(entity.MessageType)).Inc()
	}
}

// registeredIndexingProfile returns the floor registered with mt and whether
// the type declared one. A registered type with an empty floor and a type the
// registry does not hold both fall to control (fail-safe toward keeping the
// substrate reachable) with floored=false, which fires the default metric; a
// component holding no registry answers the same way rather than guessing.
func (c *Component) registeredIndexingProfile(mt message.Type) (profile string, floored bool) {
	if c.payloadRegistry == nil {
		return vocabulary.IndexingProfileControl, false
	}
	floor, registered := c.payloadRegistry.IndexingProfileFor(mt.Key())
	if !registered || floor == "" {
		return vocabulary.IndexingProfileControl, false
	}
	return floor, true
}

// indexingProfileMetricLabel renders a message.Type as a stable, low-cardinality
// metric label, or "unknown" for any incomplete Type. The IsValid guard (all
// three of Domain/Category/Version present) prevents a partial Type from
// producing a non-semantic label like ".widget.v1" or "test.widget." — a
// malformed producer surfaces as "unknown" rather than a junk label key.
func indexingProfileMetricLabel(mt message.Type) string {
	if !mt.IsValid() {
		return "unknown"
	}
	return mt.Key()
}

// ============================================================================
// Entity Operations
// ============================================================================

// validateEntityID validates that an entity ID follows the expected format
func validateEntityID(id string) error {
	return semtypes.ValidateEntityID(id)
}

// CreateEntity atomically creates a new entity. Existing keys are never
// overwritten; callers observe natsclient.ErrKVKeyExists and decide whether
// that conflict is acceptable for their operation.
func (c *Component) CreateEntity(ctx context.Context, entity *graph.EntityState) error {
	_, _, err := c.createEntityWithReceipt(ctx, entity)
	return err
}

// mergeEntityOnLane ingests a streaming-consumer EntityState (typically built
// by extractEntityFromMessage from a Graphable arriving on the
// JetStream input) WITHOUT clobbering pre-existing triples on the
// entity. First write behaves like CreateEntity (the entity didn't
// exist; its fields land verbatim); subsequent writes replace each
// (predicate, source) set the arrival carries, unless it is older than the
// stored set, keeping every other source's statements (graph.ReplaceBySource,
// design D15; the create-time indexing profile is preserved), and take the
// arrival's MessageType and StorageRef when no set was older (UpdatedAt always).
//
// Closes gh#177: the prior code called CreateEntity (Put = full-
// replace) from handleMessage, which erased triples written through mutation
// request/reply. The JetStream consumer
// path was the lone outlier. Lifecycle-managed entities surfaced this
// most loudly: Manager.Create stamped the phase triple, then the
// first Graphable arrival via a downstream processor wiped it.
//
// Writes through replaceEntity (entity_writes.go), a CAS read-modify-write,
// so concurrent arrivals on the same Subject converge without racing.
//
// importLane carries the arrival lane so the authority gate can admit a
// foreign subject on a declared import port and refuse it everywhere else
// (ADR-102 d5); only ingestEntity, for such a port, passes true.
func (c *Component) mergeEntityOnLane(ctx context.Context, entity *graph.EntityState, importLane bool) error {
	if entity == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "mergeEntityOnLane", "entity cannot be nil")
	}
	// Entity-ID-only preflight (cheap subset): the ID is the CAS key and the
	// hierarchy-probe key, so reject a malformed ID before any KV I/O.
	//
	// gh#562 write-cost: the full ValidateEntityStateContract pass that used to
	// run here was redundant with the MarshalEntityState write gate inside the
	// CAS closure — BOTH closure branches marshal a superset of the incoming
	// candidate (create: candidate + hierarchy/profile stamps; merge: every
	// incoming statement but a set older than the stored one, which is not
	// committed), so an invalid candidate never
	// commits under its own key, and classifyStoredStateRMWError keeps the
	// caller-vs-resident attribution honest. The Graphable ingest lane already
	// runs one caller-blaming full pass before side effects, so the pass here was a
	// third full validation per mutation on the
	// per-key-serialized hot path (ADR-072). Two narrow ergonomic changes for
	// un-preflighted direct callers: (1) an incoming entity.indexing.profile
	// triple dropped pre-merge on an already-profiled entity, and a set older
	// than the stored one, are not validated — they are left out, not
	// committed, so the store contract is unaffected; (2) with EnableHierarchy set, an invalid candidate with a
	// valid ID reaches the pre-closure hierarchy step below, whose
	// GetHierarchyTriples COMMITS container entities + inverse contains-edges +
	// sibling edges before the write gate rejects the candidate itself — that
	// pre-committed content is contract-valid and identical to what a later
	// legitimate birth of the same ID would create. References to absent entities
	// are valid graph facts and remain observable as unresolved references.
	if err := validateEntityID(entity.ID); err != nil {
		return err
	}
	// The write chokepoint's own authority gate (ADR-102 d5): the fact lane has
	// already run it in prepareFactProjection AND returned, so reaching a
	// rejection here means a DIRECT caller, which is why the recording is the
	// direct lane's and cannot double-count the fact lane's.
	if err := c.authorizeSubject(entity.ID, importLane); err != nil {
		return c.recordDirectAuthorityRejection(err)
	}
	if err := requireOwnStatements(entity.Triples); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "Component", "mergeEntityOnLane", "context cancelled")
	}
	// Read over the arrival's statements, stamped from the envelope, and any explicit profile
	// extractEntityFromMessage stamped with this same time: every statement graph-ingest derives
	// for this write carries it.
	at := triggeringTime(entity)

	// Hierarchy inference is deterministic per entityID — calling it
	// on every merge would APPEND the same hierarchy triples on each
	// arrival (50 mission-command Graphables → 50 copies of
	// type.container / system.container / etc. on the same entity).
	// Gate to first-write only, applied inside the CAS callback so the
	// fetch happens once per genuine create. See go-reviewer concern 5
	// on the gh#177 fix.
	var hierarchyTriples []message.Triple
	if c.config.EnableHierarchy && c.hierarchyInference != nil {
		// Probe-then-fetch: cheap pre-check avoids the inference cost
		// on guaranteed-second-write paths. Even if the entity is
		// concurrently created between probe and CAS, the callback's
		// len(current) == 0 branch is the gate that actually applies
		// the hierarchy triples — the probe is an optimization, not
		// correctness.
		if _, err := c.entityBucket.Get(ctx, entity.ID); err != nil && natsclient.IsKVNotFoundError(err) {
			triples, herr := c.hierarchyInference.GetHierarchyTriples(ctx, entity.ID, at)
			if herr != nil {
				c.logger.Warn("Failed to get hierarchy triples",
					slog.String("entity_id", entity.ID),
					slog.Any("error", herr))
			} else {
				hierarchyTriples = triples
			}
		}
	}

	revision, bytesWritten, err := c.replaceEntity(ctx, entity, hierarchyTriples, at)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return errs.Wrap(err, "Component", "mergeEntityOnLane", "CAS update")
	}

	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(bytesWritten))
	c.lastActivity.Store(time.Now())
	if revision == 0 {
		// Every set was older and nothing was written (design D23); replaceEntity counted and
		// logged each set. The message is processed, but no entity was updated.
		return nil
	}
	// replaceEntity cleared the key's poison record and invalidated its cache entry.
	c.entitiesUpdated.Inc()

	c.logger.Debug("entity merged",
		slog.String("entity_id", entity.ID),
		slog.Int("triples_in", len(entity.Triples)))

	return nil
}

func (c *Component) createEntityWithReceipt(
	ctx context.Context,
	entity *graph.EntityState,
) (*graph.EntityState, uint64, error) {
	if entity == nil {
		return nil, 0, errs.WrapInvalid(errs.ErrInvalidData, "Component", "CreateEntity", "entity cannot be nil")
	}
	// ADR-103 d3: the in-process birth passes the same registered-type gate as
	// the RPC lane. The classified error goes back to the caller (the hierarchy
	// container path WARNs and continues); it is not metered here.
	if err := c.requireRegisteredMessageType(entity); err != nil {
		return nil, 0, err
	}

	// Validate entity ID format
	if err := validateEntityID(entity.ID); err != nil {
		return nil, 0, err
	}
	// In-process creation is a LOCAL lane: hierarchy container births are minted
	// by this deployment, so a foreign subject here would be the framework
	// minting under a peer's authority (ADR-102 d5). Metered and logged under
	// the `direct` arrival, so the requirement's "each rejection is metered
	// exactly once and loudly logged" holds on this lane too — an in-process
	// birth has no RPC subject, not no destination.
	if err := c.authorizeSubject(entity.ID, false); err != nil {
		return nil, 0, c.recordDirectAuthorityRejection(err)
	}
	if err := graph.ValidateEntityStateContract(entity); err != nil {
		return nil, 0, errs.WrapInvalid(err, "Component", "CreateEntity", "validate entity state contract")
	}
	if err := requireOwnStatements(entity.Triples); err != nil {
		return nil, 0, err
	}
	// Read before graph-ingest adds hierarchy statements or the profile.
	at := triggeringTime(entity)

	// Check context
	if err := ctx.Err(); err != nil {
		return nil, 0, errs.Wrap(err, "Component", "CreateEntity", "context cancelled")
	}

	// SYNCHRONOUS HIERARCHY INFERENCE:
	// Get hierarchy triples BEFORE writing entity to storage
	// This ensures entity is written once with all triples included (no cascade)
	if c.config.EnableHierarchy && c.hierarchyInference != nil {
		hierarchyTriples, err := c.hierarchyInference.GetHierarchyTriples(ctx, entity.ID, at)
		if err != nil {
			c.logger.Warn("Failed to get hierarchy triples",
				slog.String("entity_id", entity.ID),
				slog.Any("error", err))
			// Don't fail entity creation if hierarchy fails - just log warning
		} else if len(hierarchyTriples) > 0 {
			// Add hierarchy triples to entity before writing
			entity.Triples = append(entity.Triples, hierarchyTriples...)
		}
	}

	// ADR-054: stamp the indexing profile at the creation seam — keeps an
	// explicit declaration (envelope/Graphable, already on entity.Triples) and
	// otherwise applies the fallback floor + default metric.
	c.reconcileIndexingProfile(entity, at)

	// Serialize entity (now includes hierarchy triples if enabled)
	data, err := graph.MarshalEntityState(entity)
	if err != nil {
		atomic.AddInt64(&c.errors, 1)
		return nil, 0, errs.Wrap(err, "Component", "CreateEntity", "entity serialization")
	}

	// Atomic create-or-fail is the only admitted birth primitive. Preserve the
	// conflict sentinel so the calling component can make its own decision.
	committedRev, writeErr := c.createEntity(ctx, entity.ID, data)
	if writeErr != nil && errors.Is(writeErr, natsclient.ErrKVKeyExists) {
		return nil, 0, writeErr
	}
	if writeErr != nil {
		atomic.AddInt64(&c.errors, 1)
		return nil, 0, errs.Wrap(writeErr, "Component", "CreateEntity", "KV store")
	}

	// createEntity cleared the key's poison record and invalidated its cache entry.
	// Update metrics
	atomic.AddInt64(&c.messagesProcessed, 1)
	atomic.AddInt64(&c.bytesProcessed, int64(len(data)))
	c.lastActivity.Store(time.Now())
	c.entitiesUpdated.Inc()

	c.logger.Debug("entity created",
		slog.String("entity_id", entity.ID),
		slog.Int("triples", len(entity.Triples)))

	return entity.Clone(), committedRev, nil
}

func (c *Component) deleteEntityAtRevision(ctx context.Context, entityID string, revision uint64) error {
	if err := validateEntityID(entityID); err != nil {
		return err
	}
	// Deleting a foreign subject is a mutation of an imported mirror, refused
	// on every local lane (ADR-102 d5, ruled O-12(a)). handleCanonicalDelete
	// authorizes and returns before it reaches this body, so a rejection here is
	// a direct in-process delete and is metered as one.
	if err := c.authorizeSubject(entityID, false); err != nil {
		return c.recordDirectAuthorityRejection(err)
	}
	if revision == 0 {
		return errs.WrapInvalid(errs.ErrInvalidData, "Component", "deleteEntityAtRevision", "revision must be nonzero")
	}
	if c.entityBucket == nil {
		return errors.New("ENTITY_STATES authority bucket unavailable")
	}
	if err := c.deleteEntity(ctx, entityID, revision); err != nil {
		atomic.AddInt64(&c.errors, 1)
		return err
	}
	atomic.AddInt64(&c.messagesProcessed, 1)
	c.lastActivity.Store(time.Now())
	return nil
}

// ============================================================================
// Triple Operations
// ============================================================================

// invalidateEntityCacheEntry drops a stale entity-query-cache entry after a
// direct entityBucket write, preserving read-after-write coherence for callers
// that query via graph.ingest.query.* (e.g. the gated-DAG executor reading the
// whole unit set right after committing a claim, or any read-after-mutate via
// the NATS mutation API). This is the single invalidation primitive for every
// write path (canonical create/reconcile/append/delete, Graphable merge, and
// hierarchy inference) so the coherence guard below is uniform.
//
// Read-after-write cache-coherence contract: a repopulating Set is dropped if
// the key was invalidated during the read window, so a slow reader cannot
// resurrect pre-write state past a concurrent invalidate. This method bumps the
// per-key invalidation generation AND drops the entry under cacheGenMu; the
// cache-miss repopulate (repopulateEntityCacheEntry) captures the generation
// before its KV Get and re-checks it under the same lock before Setting, so
// {bump+delete} and {gen-check+set} are mutually atomic. An invalidation that
// lands after a reader's KV Get is therefore guaranteed to be observed by that
// reader (it skips its Set); an invalidation that lands after a reader's Set is
// guaranteed to delete the value it wrote. No-op when the cache is disabled.
//
// Correctness precondition: entityBucket.Get is read-your-writes/monotonic
// relative to a committed write's invalidate — a Get issued after an
// invalidation is reflected in gen0 returns that write's revision or newer.
// This holds for single-server NATS (all integration/e2e testcontainers) and
// for leader reads. A clustered deployment serving stale allow_direct reads
// from a lagging replica could return a revision older than an acknowledged
// write — but such a Get already returns stale data on a cold cache, so the
// guard introduces no regression relative to the no-cache baseline.
func (c *Component) invalidateEntityCacheEntry(id string) {
	if c.entityCache == nil {
		return
	}
	c.cacheGenMu.Lock()
	defer c.cacheGenMu.Unlock()
	c.cacheGen[id]++
	c.entityCache.Delete(id) //nolint:errcheck
}

// loadEntityCacheGen captures the current invalidation generation for a key.
// The cache-miss read path calls this BEFORE its KV Get so a later repopulating
// Set can detect an invalidation that raced the read window. Returns 0 for a
// never-invalidated key (map zero value), which is a valid baseline.
func (c *Component) loadEntityCacheGen(id string) uint64 {
	c.cacheGenMu.Lock()
	g := c.cacheGen[id]
	c.cacheGenMu.Unlock()
	return g
}

// repopulateEntityCacheEntry stores a freshly-read entity into the query cache
// UNLESS the key was invalidated during the read window — i.e. gen0 (captured
// before the KV Get that produced entity) no longer matches the key's current
// generation. See invalidateEntityCacheEntry for the coherence contract and the
// happens-before argument. No-op when the cache is disabled.
func (c *Component) repopulateEntityCacheEntry(id string, entity graph.EntityState, gen0 uint64) {
	if c.entityCache == nil {
		return
	}
	c.cacheGenMu.Lock()
	defer c.cacheGenMu.Unlock()
	if c.cacheGen[id] == gen0 {
		c.entityCache.Set(id, entity) //nolint:errcheck
	}
}

// recordSuppressedDuplicates meters triples the append path declined to store
// because the entity already carried them. Nil-guarded so it is safe on a
// hand-built Component (the test-construction pattern) that reaches it without
// full metric wiring.
func (c *Component) recordSuppressedDuplicates(lane dedupLane, suppressed int) {
	if suppressed <= 0 || c.duplicateTriplesSuppressed == nil {
		return
	}
	c.duplicateTriplesSuppressed.WithLabelValues(string(lane)).Add(float64(suppressed))
}

// addTripleLane applies one hierarchy append, carrying the lane label for the
// suppressed-duplicate metric. deduplicated=true means the write was a no-op:
// this call wrote nothing, and err is nil.
//
// committedRevision is the EXACT revision this call's CAS produced, and is 0
// whenever the call attributes no commit to itself (suppressed, or an error; an
// error does not mean nothing committed: a write cut short by an infrastructure
// error may have committed, design D23). It is deliberately not
// a post-hoc read: another writer can commit between the CAS and a re-read, and
// a caller attributing that writer's revision to its own write would suppress
// the writer's genuine change (Codex C2).
func (c *Component) addTripleLane(ctx context.Context, triple message.Triple, lane dedupLane) (deduplicated bool, committedRevision uint64, err error) {
	// Deliberately KEPT under the gh#562 write-cost dedup (unlike mergeEntityOnLane's
	// removed candidate pass): this is an O(1-triple) caller-blaming preflight
	// that runs BEFORE any KV I/O. Without it a malformed Subject would drive
	// the CAS Get on a nonexistent key and misclassify as entity-not-found
	// instead of invalid (and the pre-I/O contract test pins the ordering).
	// The MarshalEntityState write gate below remains the authoritative
	// full-pass over the committed union.
	if contractErr := graph.ValidateEntityStateContract(&graph.EntityState{
		ID: triple.Subject, Triples: []message.Triple{triple},
	}); contractErr != nil {
		return false, 0, errs.WrapInvalid(contractErr, "Component", "AddTriple", "validate triple contract")
	}
	// Same seam, same lane: an in-process append names its own subject, so a
	// foreign one would be the framework annotating an imported mirror
	// (ADR-102 d5, ruled O-12(a)). This body has no RPC caller — hierarchy
	// inference's inverse edges are its only entry — so the rejection is the
	// direct lane's to meter.
	if authErr := c.authorizeSubject(triple.Subject, false); authErr != nil {
		return false, 0, c.recordDirectAuthorityRejection(authErr)
	}
	if err := requireStatementMetadata([]message.Triple{triple}); err != nil {
		return false, 0, err
	}

	// Check context
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, 0, errs.Wrap(ctxErr, "Component", "AddTriple", "context cancelled")
	}

	// appendEntityTriples: atomic read-modify-write with CAS, returning the exact
	// revision the commit produced (see the committedRevision doc above).
	appended, casErr := c.appendEntityTriples(ctx, triple.Subject, []message.Triple{triple})
	if casErr != nil {
		atomic.AddInt64(&c.errors, 1)
		return false, 0, errs.Wrap(casErr, "Component", "AddTriple", "CAS update")
	}
	if appended.outcome == graph.MutationUnchanged {
		// Duplicate suppression is a SUCCESS that wrote nothing, so it is not
		// metered as a component error: that would make every restart replay look
		// like a fault. The committed revision stays 0: the call attributes no
		// commit to itself.
		c.recordSuppressedDuplicates(lane, 1)
		return true, 0, nil
	}
	// appendEntityTriples cleared the key's poison record and invalidated its
	// cache entry, so the next query reads the appended state.
	return false, appended.revision, nil
}

// addTriplesResult is one internal append batch's outcome. Grouped into a struct
// rather than a fifth and sixth return value so the batch handler can ask the
// two questions it actually needs — "did THIS subject commit, and at which
// revision" — without a positional argument pile.
type addTriplesResult struct {
	// Written counts NEWLY appended tuples only, excluding suppressed duplicates.
	Written int
	// Deduplicated counts tuples not appended because the entity already
	// carried them, or because the request repeated them.
	Deduplicated int
	// FailedSubjects maps each subject that rolled back to its per-subject error.
	FailedSubjects map[string]string
	// NotFoundSubjects identifies definite absent targets separately from
	// subject-local internal failures.
	NotFoundSubjects map[string]struct{}
	// SubjectErrors preserves typed subject-local failures for the canonical
	// partial-result wire without erasing earlier receipts.
	SubjectErrors map[string]error
	// UnchangedSubjects maps each subject whose submitted tuples were all already
	// present to the revision its CAS read: nothing was written, and the canonical
	// reply is unchanged at that revision, never at a later read's (design D23).
	UnchangedSubjects map[string]uint64
	// CommittedRevisions holds the EXACT revision each subject's CAS produced,
	// keyed by subject. A subject that was wholly suppressed or that failed is
	// ABSENT — presence in this map is the authoritative answer to "did this
	// call commit a write for that subject", which is what a caller must gate
	// on before attributing a revision to itself (Codex C1/C2).
	CommittedRevisions map[string]uint64
}

// addTriplesLane is AddTriples' body, carrying the lane label for the
// suppressed-duplicate metric and additionally reporting how many submitted
// tuples were suppressed as already-stored duplicates. writtenCount +
// deduplicated accounts for every triple whose subject committed, which is what
// lets a caller distinguish "already present" from "nothing happened" without
// an authoritative read-back.
func (c *Component) addTriplesLane(ctx context.Context, triples []message.Triple, lane dedupLane) (addTriplesResult, error) {
	if len(triples) == 0 {
		return addTriplesResult{}, nil
	}

	// Preserve the established positional diagnostic for an omitted mutation
	// subject. The authoritative candidate validator below remains acceptance
	// authority for all subject syntax, references, and predicates.
	for index := range triples {
		if triples[index].Subject == "" {
			return addTriplesResult{}, errs.WrapInvalid(errs.ErrInvalidData, "Component", "AddTriples", fmt.Sprintf("triple[%d] subject cannot be empty", index))
		}
	}

	// Validate the whole batch before any CAS read. The first subject supplies
	// the synthetic candidate root; every subject/reference/predicate is still
	// validated independently by the authoritative contract.
	if contractErr := graph.ValidateEntityStateContract(&graph.EntityState{
		ID: triples[0].Subject, Triples: triples,
	}); contractErr != nil {
		return addTriplesResult{}, errs.WrapInvalid(contractErr, "Component", "AddTriples", "validate batch contract")
	}
	// EVERY subject in the batch, not just the synthetic root: a batch is
	// rejected whole, so one foreign subject must not ride in beside local ones
	// (ADR-102 d5). One record for the batch, at the first refused subject:
	// the batch is one rejected operation, not one per subject.
	// handleCanonicalAppend authorizes every subject and returns before this
	// body, so the RPC lane never reaches here with a foreign subject.
	for index := range triples {
		if authErr := c.authorizeSubject(triples[index].Subject, false); authErr != nil {
			return addTriplesResult{}, c.recordDirectAuthorityRejection(authErr)
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return addTriplesResult{}, errs.Wrap(ctxErr, "Component", "AddTriples", "context cancelled")
	}

	// Group triples by Subject so each entity sees a single CAS.
	// Map iteration is non-deterministic; we sort subject keys before
	// committing so retries replay in stable order (helpful for tests
	// and for any operator reading a partial-failure trace).
	bySubject := make(map[string][]message.Triple, len(triples))
	for _, t := range triples {
		bySubject[t.Subject] = append(bySubject[t.Subject], t)
	}
	subjects := make([]string, 0, len(bySubject))
	for s := range bySubject {
		subjects = append(subjects, s)
	}
	sort.Strings(subjects)

	var writtenCount, deduplicated int
	failedSubjects := make(map[string]string)
	notFoundSubjects := make(map[string]struct{})
	subjectErrors := make(map[string]error)
	unchangedSubjects := make(map[string]uint64)
	committedRevisions := make(map[string]uint64)
	// allAbsences tracks whether EVERY per-subject failure was an ADR-055
	// entity-not-found rejection. When true the aggregated error wraps
	// ErrKVKeyNotFound so the handler can set ErrorCodeEntityNotFound on the
	// response without string-sniffing. Mixed-reason batches (allAbsences=false)
	// stay un-sentineled → handler leaves ErrorCode empty (unclassified).
	allAbsences := true
	// poisonErr preserves the FIRST typed resident-poison failure so the
	// aggregated error can wrap it: without this the typed
	// *graph.StateContractError would die as a FailedSubjects string and the
	// whole-batch reply would misclassify as retryable-internal (design D4).
	var poisonErr error
	for i, subject := range subjects {
		// Short-circuit on context cancellation: don't burn the retry
		// budget for every remaining subject when the caller has
		// already given up. Mark the rest as failed-due-to-cancel so
		// the response shape is honest about what didn't commit, but
		// only count one operational error event for the cancellation
		// rather than N (one per remaining subject).
		if ctxErr := ctx.Err(); ctxErr != nil {
			for _, s := range subjects[i:] {
				failedSubjects[s] = ctxErr.Error()
				subjectErrors[s] = ctxErr
			}
			// A cancelled batch is NOT a pure entity-not-found batch.
			allAbsences = false
			atomic.AddInt64(&c.errors, 1)
			break
		}
		group := bySubject[subject]
		appended, casErr := c.appendEntityTriples(ctx, subject, group)
		if casErr != nil {
			atomic.AddInt64(&c.errors, 1)
			failedSubjects[subject] = casErr.Error()
			if errors.Is(casErr, natsclient.ErrKVKeyNotFound) {
				notFoundSubjects[subject] = struct{}{}
			} else {
				subjectErrors[subject] = casErr
			}
			var stateErr *graph.StateContractError
			if poisonErr == nil && errors.As(casErr, &stateErr) {
				poisonErr = casErr
			}
			if !errors.Is(casErr, natsclient.ErrKVKeyNotFound) {
				allAbsences = false
			}
			continue
		}
		// appended.suppressed is the count appendEntityTriples took against the
		// state its last CAS attempt read, so a retry never double-counts.
		c.recordSuppressedDuplicates(lane, appended.suppressed)
		deduplicated += appended.suppressed
		if appended.outcome == graph.MutationUnchanged {
			// A wholly-duplicate subject committed nothing but did not FAIL: it
			// stays out of failedSubjects, c.errors and allAbsences, which would
			// otherwise misclassify a mixed batch as a pure entity-not-found batch
			// (wrong ErrorCodeEntityNotFound on the reply).
			unchangedSubjects[subject] = appended.revision
			continue
		}
		// appendEntityTriples cleared the key's poison record and invalidated its
		// cache entry, so the next query reads the appended state.
		// Only NEWLY appended tuples count as written.
		writtenCount += len(group) - appended.suppressed
		// This subject COMMITTED, at exactly this revision.
		committedRevisions[subject] = appended.revision
	}

	result := addTriplesResult{
		Written:            writtenCount,
		Deduplicated:       deduplicated,
		CommittedRevisions: committedRevisions,
		NotFoundSubjects:   notFoundSubjects,
		SubjectErrors:      subjectErrors,
		UnchangedSubjects:  unchangedSubjects,
	}
	if len(failedSubjects) == 0 {
		return result, nil
	}
	result.FailedSubjects = failedSubjects

	// Sort failed-subject names for stable error messages.
	failed := make([]string, 0, len(failedSubjects))
	for s := range failedSubjects {
		failed = append(failed, s)
	}
	sort.Strings(failed)
	var innerErr error
	switch {
	case allAbsences:
		// All failures were entity-not-found: wrap the sentinel so the handler
		// can set ErrorCodeEntityNotFound without string-sniffing.
		innerErr = fmt.Errorf("CAS update failed for %d/%d subjects: %v: %w",
			len(failedSubjects), len(subjects), failed, natsclient.ErrKVKeyNotFound)
	case poisonErr != nil:
		// Resident poison in the batch: wrap the typed cause so the aggregated
		// error stays errors.As-able as *graph.StateContractError and the
		// whole-batch reply classifies fatal/graph_state_reset_required instead
		// of dying as a FailedSubjects string.
		innerErr = fmt.Errorf("CAS update failed for %d/%d subjects: %v: %w",
			len(failedSubjects), len(subjects), failed, poisonErr)
	default:
		innerErr = fmt.Errorf("CAS update failed for %d/%d subjects: %v",
			len(failedSubjects), len(subjects), failed)
	}
	return result, errs.Wrap(innerErr, "Component", "AddTriples", "batch CAS partial failure")
}

// newBatchMissingMetric builds the counter for requested entity IDs a
// batch query could not hydrate, labelled by the closed graph.MissingReason set.
//
// It exists because partial hydration used to be invisible: an ID whose KV read came
// back not-found was dropped from the response, and the symptom surfaced several hops
// downstream as a missing search result (gh#597). Counting it at the source is what
// makes "how often, and is it not-found or a fault?" answerable from a dashboard rather
// than from a bug report.
//
// Labelled by reason only. Entity IDs are an unbounded label space and belong on the
// log line, which carries them bounded.
func newBatchMissingMetric() *prometheus.CounterVec {
	batchMissingVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "batch_query_missing_total",
		Help:      "Requested entity IDs a batch query could not hydrate, by reason — partial hydration made visible (gh#597)",
	}, []string{"reason"})
	// Pre-initialize the closed label set: an absent series breaks rate() alerting,
	// so a reason that has never fired must still be scrapeable at zero. `unknown`
	// is client-synthesized and never emitted here, so only the two handler-emitted
	// reasons are seeded.
	for _, reason := range []graph.MissingReason{graph.MissingNotFound, graph.MissingError} {
		batchMissingVec.WithLabelValues(string(reason))
	}
	return batchMissingVec
}
