package readiness

import (
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// Gauges is the readiness envelope's Prometheus projection, owned ONCE for every
// producer.
//
// Set is the single projection site, so a field added to IndexStatusResponse cannot be
// wired in one producer and forgotten in another. At the SemStreams pin, two producers
// each built their own set, and both left out bootstrap_complete, the field
// EvaluateReadinessGate reads.
type Gauges struct {
	subsystem string

	readiness         prometheus.Gauge
	lag               prometheus.Gauge
	bootstrapComplete prometheus.Gauge
	state             *prometheus.GaugeVec
	publishFailures   prometheus.Counter

	// indexedRevision and targetRevision are set only by NewRevisionGauges. A
	// backlog producer has no ENTITY_STATES revision, and a gauge reading 0 would
	// assert one, so for it they stay nil and are neither registered nor set.
	indexedRevision prometheus.Gauge
	targetRevision  prometheus.Gauge
}

// metricNamespace is the shared Prometheus namespace, held as a constant so every
// collector NewGauges builds uses the same one.
const metricNamespace = "semengine"

// NewGauges builds the readiness gauge set for one producer, named by its Prometheus
// subsystem ("graph_ingest", "graph_index"). Register keys each collector under the
// same subsystem.
//
// The emitted metric NAMES are an external contract (ADR-066): dashboards and operator
// alerts read them. They are the names the SemStreams pin emits, and a test compares
// them against the live registry.
func NewGauges(subsystem string) *Gauges {
	return &Gauges{
		subsystem: subsystem,
		readiness: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Subsystem: subsystem,
			Name:      "readiness",
			Help:      "1 when the readiness envelope is Ready (producer caught up), else 0 (ADR-066)",
		}),
		lag: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Subsystem: subsystem,
			Name:      "lag",
			Help:      "Outstanding work in the producer's OWN unit — ENTITY_STATES revisions for a revision-lag producer, messages for a backlog producer; 0 = caught up",
		}),
		bootstrapComplete: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Subsystem: subsystem,
			Name:      "bootstrap_complete",
			Help:      "1 when the producer has finished its INITIAL build in this process lifetime, else 0. Distinguishes an index mid-format-cutover from one merely behind (gh#474, ADR-084)",
		}),
		state: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Subsystem: subsystem,
			Name:      "readiness_state",
			Help:      "Readiness state one-hot (building|ready|degraded|reset_required): current state=1, others=0, so catching-up is distinguishable from broken",
		}, []string{"state"}),
		publishFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Subsystem: subsystem,
			Name:      "status_publish_failures_total",
			Help:      "Readiness heartbeat writes to the GRAPH_STATUS KV key that failed (ADR-083): consumers go status_unknown and fail closed while this rises",
		}),
	}
}

// NewRevisionGauges builds the gauge set for a revision-lag producer (graph-index's
// follower): NewGauges' set plus indexed_revision and target_revision, the
// envelope's floor and target in ENTITY_STATES revisions (ADR-066). A backlog
// producer uses NewGauges, which exports neither.
func NewRevisionGauges(subsystem string) *Gauges {
	g := NewGauges(subsystem)
	g.indexedRevision = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Subsystem: subsystem,
		Name:      "indexed_revision",
		Help:      "Low-water-of-pending watermark: every delivered ENTITY_STATES revision at or below this has been applied (ADR-066)",
	})
	g.targetRevision = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Subsystem: subsystem,
		Name:      "target_revision",
		Help:      "ENTITY_STATES stream LastSeq the producer must catch up to (ADR-066)",
	})
	return g
}

// Register registers every collector on registry through metric.RegisterOrGet and
// keeps the collectors it returns: when another Gauges already registered a metric
// under the same subsystem and name, this one writes that collector, so its writes are
// the ones gathered. A nil registry registers nothing: the gauges still take writes,
// and nothing gathers them. Nothing is registered on Prometheus' global registry.
func (g *Gauges) Register(registry *metric.MetricsRegistry) error {
	if registry == nil {
		return nil
	}
	if err := g.registerShared(registry); err != nil {
		return err
	}
	if g.indexedRevision == nil {
		return nil
	}
	var err error
	if g.indexedRevision, err = metric.RegisterOrGet(registry, g.subsystem, "indexed_revision", g.indexedRevision); err != nil {
		return err
	}
	if g.targetRevision, err = metric.RegisterOrGet(registry, g.subsystem, "target_revision", g.targetRevision); err != nil {
		return err
	}
	return nil
}

// registerShared registers the collectors every producer's set has.
func (g *Gauges) registerShared(registry *metric.MetricsRegistry) error {
	var err error
	if g.readiness, err = metric.RegisterOrGet(registry, g.subsystem, "readiness", g.readiness); err != nil {
		return err
	}
	if g.lag, err = metric.RegisterOrGet(registry, g.subsystem, "lag", g.lag); err != nil {
		return err
	}
	if g.bootstrapComplete, err = metric.RegisterOrGet(registry, g.subsystem, "bootstrap_complete", g.bootstrapComplete); err != nil {
		return err
	}
	if g.state, err = metric.RegisterOrGet(registry, g.subsystem, "readiness_state", g.state); err != nil {
		return err
	}
	if g.publishFailures, err = metric.RegisterOrGet(registry, g.subsystem, "status_publish_failures_total", g.publishFailures); err != nil {
		return err
	}
	return nil
}

// Set projects one readiness envelope onto the gauges.
//
// THE SINGLE PROJECTION SITE. Every producer funnels through here, so adding a field
// to IndexStatusResponse means adding it once.
//
// The state one-hot iterates graph.IndexStates() rather than the states this producer
// happens to emit, so a new readiness state renders as an explicit 0 instead of a
// silently absent series.
func (g *Gauges) Set(resp graph.IndexStatusResponse) {
	g.readiness.Set(boolGauge(resp.Ready))
	g.lag.Set(float64(resp.Lag))
	g.bootstrapComplete.Set(boolGauge(resp.BootstrapComplete))
	if g.indexedRevision != nil {
		g.indexedRevision.Set(float64(resp.IndexedRevision))
		g.targetRevision.Set(float64(resp.TargetRevision))
	}

	for _, s := range graph.IndexStates() {
		g.state.WithLabelValues(string(s)).Set(boolGauge(s == resp.State))
	}
}

// RecordPublishFailure counts one failed GRAPH_STATUS heartbeat write (ADR-083).
func (g *Gauges) RecordPublishFailure() {
	g.publishFailures.Inc()
}

func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
