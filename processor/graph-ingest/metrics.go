package graphingest

import (
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
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
// indexing_profile field declared a profile (SemStreams ADR-054 §5). Labeled by
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

// newMutationRejectionsMetric builds the counter for rejected graph writes:
// mutation requests, and authority refusals on the fact lanes (their arrival
// port's subject) and on direct writes ("direct"). The reason is a bounded set
// (#287): authority_foreign or authority_claimed for an authority refusal
// (authorityMetricReason), otherwise the error's classified code, or internal
// when it carries none. It operationalizes SemStreams ADR-055 §3's "loud fail-fast"
// observability. Append, reconcile, and delete against an absent entity report
// entity_not_found. Other bounded classes include validation, revision
// conflict, and strict-create conflict.
func newMutationRejectionsMetric() *prometheus.CounterVec {
	mutationRejectionsVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "mutation_rejections_total",
		Help:      "Graph writes rejected, by subject or arrival lane and reason: an authority reason (authority_foreign, authority_claimed), else the error's classified code, 'internal' when unclassified.",
	}, []string{"subject", "reason"})
	return mutationRejectionsVec
}

func newMutationOutcomesMetric() *prometheus.CounterVec {
	mutationOutcomesVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "graph_mutation_outcomes_total",
		Help:      "Canonical graph mutation outcomes by operation and outcome: one count per reply, and one per subject for an append",
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
// apply time — the merge + CAS write applyIngest times (SemStreams gh#480). Paired with
// newIngestLagMetric it separates processing time from queue wait, which the
// component previously could not distinguish (callers inferred end-to-end latency
// downstream). Buckets span sub-ms (a warm CAS is ~1.5ms) to seconds (a stalled KV).
func newProcessingDurationMetric() prometheus.Histogram {
	processingDurationHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "processing_duration_seconds",
		Help:      "Per-message apply time in graph-ingest (merge + CAS write). The processing half of the queue-wait-vs-processing split (SemStreams gh#480).",
		Buckets:   []float64{0.0005, 0.001, 0.002, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	})
	return processingDurationHistogram
}

// newIngestLagMetric builds the histogram of message age at the
// moment graph-ingest begins processing it (now − the JetStream message
// timestamp) — i.e. how long a message waited in the stream/delivery buffer
// before ingest reached it (SemStreams gh#480 queue wait). A rising lag with flat
// processing_duration is the backlog signature (consumer_pending_messages
// climbing) the issue describes.
func newIngestLagMetric() prometheus.Histogram {
	ingestLagHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "ingest_lag_seconds",
		Help:      "Age of a message when graph-ingest starts processing it (queue/backlog wait). The queue-wait half of the split (SemStreams gh#480).",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
	})
	return ingestLagHistogram
}

// newRedeliveriesDroppedMetric builds the counter of messages
// dropped by the keyed-ingest redelivery guard (SemStreams ADR-072 B1/B2/B3): a delayed
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
		Help:      "Stale redeliveries dropped by the applied-sequence guard (SemStreams ADR-072).",
	})
	return redeliveriesDroppedCounter
}

// newGuardRecordRefusalsMetric builds the counter of stream inputs refused because the stored
// applied-sequence record for their entity and stream cannot be decoded (design D21). Each refused
// delivery counts once; the key is logged once. A rising count means a key in
// GRAPH_INGEST_APPLIED_SEQ needs deleting, after which the redelivery applies.
func newGuardRecordRefusalsMetric() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "guard_record_refusals_total",
		Help:      "Stream inputs not applied and not acknowledged because their stored applied-sequence record cannot be decoded; deleting the key repairs it.",
	})
}

// newUnguardedInputsMetric builds the counter of stream inputs applied as first seen because
// graph-ingest has no applied-sequence bucket (design D8), which only a component built without
// Start can lack. Each such input counts once and is logged.
func newUnguardedInputsMetric() prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "unguarded_inputs_total",
		Help:      "Stream inputs applied as first seen because graph-ingest has no applied-sequence bucket to check them against.",
	})
}

// newWorkInterruptedMetric builds the counter of stream inputs whose application was interrupted and
// left unsettled, so the server delivers them again after the ack wait (design D8, "Long apply"; #91
// comment 6109278240, item 1). Its cause label is a closed enum: panic (the application panicked),
// heartbeat_failed (an in-progress signal could not be sent) or invalid (the delivery's metadata or
// the application's decision was invalid). Each such input is also logged at ERROR, and the lane
// keeps taking input. The interruption adds nothing to the error count Health reports (#91 comment
// 6124130342).
func newWorkInterruptedMetric() *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "work_interrupted_total",
		Help:      "Stream inputs whose application was interrupted and left unsettled, to be delivered again after the ack wait. Label: cause (panic, heartbeat_failed, invalid), a closed enum.",
	}, []string{"cause"})
}

// newCasRetriesMetric builds the counter of CAS-conflict retries
// during entity merge (SemStreams ADR-072). Incremented each time mergeEntityOnLane's CAS
// callback re-runs (attempt > 1) — the previous attempt's revision-checked Put
// lost the CAS and retried. This is a **cross-entity contention-observability**
// signal, NOT a proof of keying correctness: an entity's own key is
// never written concurrently under keying, but legitimate cross-entity
// hierarchy container writes do touch shared keys and retry. Expect ~0 on workloads
// without hierarchy / dense relationships / entity-birth churn; a spike is a
// workload signal, not necessarily a bug.
func newCasRetriesMetric() prometheus.Counter {
	casRetriesCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "graph_ingest",
		Name:      "cas_retries_total",
		Help:      "Entity-merge CAS-conflict retries — cross-entity contention observability, NOT a keying-correctness proof (SemStreams ADR-072).",
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

// dedupLaneAddBatch is the canonical append mutation lane, the one append lane
// (ruling M, #91 comment 6080973822, removed hierarchy's in-process append).
const dedupLaneAddBatch dedupLane = "append"

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
		Help:      "Append triples not written because the entity already carried an identical six-field tuple. Label: lane (append), a closed enum.",
	}, []string{"lane"})
	return duplicateTriplesSuppressedVec
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
// A registry holds one graph-ingest instance's series. A second instance on the same
// registry shares each collector, so the gauges each sets from its own state (the poison
// inventory, the readiness gauges) overwrite each other, and which instance's value a
// scrape shows is not promised. No configuration we know of runs two on one registry.
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
	c.guardRecordRefusals = newGuardRecordRefusalsMetric()
	c.unguardedInputs = newUnguardedInputsMetric()
	c.workInterrupted = newWorkInterruptedMetric()
	c.casRetries = newCasRetriesMetric()
	c.staleSets = newStaleSetsMetric()
	c.duplicateTriplesSuppressed = newDuplicateTriplesSuppressedMetric()
	c.poisonedEntities = newPoisonedEntitiesMetric()
	// Readiness gauges: the scrapeable half of the ADR-066 envelope. NO revision
	// gauges: graph-ingest is a BACKLOG producer, and the spec states such a
	// producer SHALL NOT expose indexed_revision / target_revision and SHALL NOT
	// synthesize a value — a fabricated revision is worse than an absent one.
	c.readinessGauges = readiness.NewGauges("graph_ingest")
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
	if c.guardRecordRefusals, err = metric.RegisterOrGet(registry, metricService, "guard_record_refusals_total", c.guardRecordRefusals); err != nil {
		return err
	}
	if c.unguardedInputs, err = metric.RegisterOrGet(registry, metricService, "unguarded_inputs_total", c.unguardedInputs); err != nil {
		return err
	}
	if c.workInterrupted, err = metric.RegisterOrGet(registry, metricService, "work_interrupted_total", c.workInterrupted); err != nil {
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

// newBatchMissingMetric builds the counter for requested entity IDs a
// batch query could not hydrate, labelled by the closed graph.MissingReason set.
//
// It exists because partial hydration used to be invisible: an ID whose KV read came
// back not-found was dropped from the response, and the symptom surfaced several hops
// downstream as a missing search result (SemStreams gh#597). Counting it at the source is what
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
		Help:      "Requested entity IDs a batch query could not hydrate, by reason — partial hydration made visible (SemStreams gh#597)",
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
