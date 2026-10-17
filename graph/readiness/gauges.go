package readiness

import (
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// Gauges is the readiness envelope's Prometheus projection, owned ONCE for every
// producer.
//
// It exists because the duplication it replaces did not merely permit a bug, it
// produced one twice: graph-index and graph-embedding each hand-rolled this set, and
// both independently omitted bootstrap_complete — the field the KV envelope treats as
// load-bearing and EvaluateReadinessGate consumes. A second producer shape (backlog)
// would have made four copies with the same drift mode.
//
// Set is the single projection site, so a field added to IndexStatusResponse cannot be
// wired in three producers and forgotten in a fourth.
type Gauges struct {
	subsystem string

	readiness         prometheus.Gauge
	lag               prometheus.Gauge
	bootstrapComplete prometheus.Gauge
	state             *prometheus.GaugeVec
	publishFailures   prometheus.Counter
}

// metricNamespace is the shared Prometheus namespace, held as a constant so every
// collector NewGauges builds uses the same one.
const metricNamespace = "semengine"

// NewGauges builds the readiness gauge set for one producer, named by its Prometheus
// subsystem ("graph_ingest", "graph_index"). Register keys each collector under the
// same subsystem.
//
// The emitted metric NAMES are an external contract — dashboards and operator alerts
// consume them, and the graph-index-readiness spec explicitly protects graph-index's
// published output. They are therefore reproduced here exactly as the two hand-rolled
// sets emitted them; a test compares them against the live registry.
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

// Register registers every collector on registry through metric.RegisterOrGet and
// keeps the collectors it returns: when another Gauges already registered a metric
// under the same subsystem and name, this one writes that collector, so its writes are
// the ones gathered. A nil registry registers nothing: the gauges still take writes,
// and nothing gathers them. Nothing is registered on Prometheus' global registry.
func (g *Gauges) Register(registry *metric.MetricsRegistry) error {
	if registry == nil {
		return nil
	}
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
// to IndexStatusResponse means adding it once rather than remembering four places —
// which is exactly the omission that left bootstrap_complete unexposed on two
// independently-written producers.
//
// The state one-hot iterates graph.IndexStates() rather than the states this producer
// happens to emit, so a new readiness state renders as an explicit 0 instead of a
// silently absent series.
func (g *Gauges) Set(resp graph.IndexStatusResponse) {
	g.readiness.Set(boolGauge(resp.Ready))
	g.lag.Set(float64(resp.Lag))
	g.bootstrapComplete.Set(boolGauge(resp.BootstrapComplete))

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
