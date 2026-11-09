package fusion

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
)

// metricService is the logical service name fusion's collectors are registered
// under in a metric.MetricsRegistry.
const metricService = "fusion"

// Option configures an Engine at construction. Nothing changes an engine after
// NewEngine returns it.
type Option func(*Engine)

// WithSignals attaches the framework ranking signals — ontology specificity +
// predicate salience (ADR-062 increment 5, gh#396) — folded into ranking on top
// of resolve-order + lexical. nil (the default) keeps ranking at resolve-order +
// lexical.
func WithSignals(s RankSignals) Option {
	return func(e *Engine) { e.signals = s }
}

// WithMetrics registers the engine's two counters on registry:
// semengine_fusion_body_hydration_failures_total{reason} and
// semengine_fusion_skipped_reads_total{facet}. Engines given the same registry
// share one series. Without it the engine has no counters; its log lines and
// the response's labels still declare what was left out.
func WithMetrics(registry *metric.MetricsRegistry) Option {
	return func(e *Engine) { e.registry = registry }
}

// registerCounters registers both counters on the engine's registry, through
// metric.RegisterOrGet, and keeps the collectors it returns.
func (e *Engine) registerCounters() error {
	if e.registry == nil {
		return nil
	}
	bodyFailures := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "fusion",
		Name:      "body_hydration_failures_total",
		Help:      "Fusion nodes whose requested verbatim body could not be loaded, by reason (not_found, error).",
	}, []string{"reason"})
	skippedReads := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine",
		Subsystem: "fusion",
		Name:      "skipped_reads_total",
		Help:      "Fusion walk reads that failed and were skipped, by facet (relations, paths, impact, graph).",
	}, []string{"facet"})
	var err error
	if e.bodyFailures, err = metric.RegisterOrGet(e.registry, metricService, "body_hydration_failures_total", bodyFailures); err != nil {
		return err
	}
	if e.skippedReads, err = metric.RegisterOrGet(e.registry, metricService, "skipped_reads_total", skippedReads); err != nil {
		return err
	}
	return nil
}

// skipFacets is the order a call's Warn lines are written in.
var skipFacets = []string{string(WantRelations), string(WantPaths), string(WantImpact), string(WantGraph)}

// skipLog records one Fuse call's skipped walk reads per facet: how many, and the
// first error. It belongs to one call, so concurrent calls never share it.
type skipLog struct {
	counts map[string]int
	first  map[string]error
}

func newSkipLog() *skipLog {
	return &skipLog{counts: map[string]int{}, first: map[string]error{}}
}

// add records one skipped read for facet.
func (s *skipLog) add(facet string, err error) {
	if s.counts[facet] == 0 {
		s.first[facet] = err
	}
	s.counts[facet]++
}

// report declares the call's skipped reads: one Warn line per facet that skipped
// any, naming the count and the first error, and the skipped-read counter, so an
// index outage costs at most four lines per call, not one per edge.
func (e *Engine) report(s *skipLog) {
	for _, facet := range skipFacets {
		n := s.counts[facet]
		if n == 0 {
			continue
		}
		e.logger.Warn("fusion skipped graph reads; the answer is labelled incomplete",
			slog.String("facet", facet), slog.Int("skipped", n), slog.Any("first_error", s.first[facet]))
		if e.skippedReads != nil {
			e.skippedReads.WithLabelValues(facet).Add(float64(n))
		}
	}
}
