package graphindex

import (
	"strings"

	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// metricSubsystem is the Prometheus subsystem of graph-index's collectors, derived from
// its key ("graph-index" -> "graph_index") so the name is spelled once (design D3, D10).
var metricSubsystem = strings.ReplaceAll(key, "-", "_")

// indexMetrics holds graph-index's own collectors. The readiness gauges are the
// follower's (graph/follower, design D10).
type indexMetrics struct {
	eventsProcessed     prometheus.Counter
	indexUpdates        *prometheus.CounterVec
	kvOperations        *prometheus.CounterVec
	watchEvents         *prometheus.CounterVec
	writeFailures       prometheus.Counter     // a required index write ultimately failed
	reindexEvents       *prometheus.CounterVec // re-index events by result (changed|unchanged)
	reconcileOperations *prometheus.CounterVec // owner reconciliation I/O by index, operation, and outcome
	aliasesSkipped      prometheus.Counter     // aliases left out of ALIAS_INDEX: not a valid NATS KV key
	queryRowsSkipped    *prometheus.CounterVec // stored index rows a query left out: not the index's layout, by index
}

func newCounter(name, help string) prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "semengine", Subsystem: metricSubsystem, Name: name, Help: help,
	})
}

func newCounterVec(name, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine", Subsystem: metricSubsystem, Name: name, Help: help,
	}, labels)
}

// newIndexMetrics builds graph-index's collectors and registers them on registry, the
// component.Dependencies.MetricsRegistry it was built with, through metric.RegisterOrGet,
// keeping the collector each call returns: a second graph-index on the same registry
// writes the collectors the first one registered. With a nil registry the collectors are
// built and registered nowhere; nothing is registered on Prometheus' process-global
// registry (design D10, #68).
func newIndexMetrics(registry *metric.MetricsRegistry) (*indexMetrics, error) {
	m := &indexMetrics{
		eventsProcessed: newCounter("events_processed_total", "Total events processed by graph index"),
		indexUpdates:    newCounterVec("updates_total", "Total index update operations by index type", "index_type"),
		kvOperations:    newCounterVec("kv_operations_total", "Total KV bucket operations", "operation", "kv_bucket"),
		watchEvents:     newCounterVec("watch_events_total", "Total watch events received", "event_type"),
		writeFailures: newCounter("write_failures_total",
			"Entities whose required index writes ultimately failed after retry (readiness withheld until re-index)"),
		reindexEvents: newCounterVec("reindex_events_total",
			"Re-index events by whether the index-input projection changed (the L2 change-detection data gate)", "result"),
		reconcileOperations: newCounterVec("reconcile_operations_total",
			"Owner reconciliation KV operations by index type, operation, and outcome", "index_type", "operation", "result"),
		aliasesSkipped: newCounter("aliases_skipped_total",
			"Aliases not written to the alias index because they are not valid NATS KV keys (the entity's other rows are written)"),
		queryRowsSkipped: newCounterVec("query_rows_skipped_total",
			"Stored index rows a query left out of its answer because they are not the index's layout, by index type", "index_type"),
	}
	if registry == nil {
		return m, nil
	}
	var err error
	if m.eventsProcessed, err = metric.RegisterOrGet(registry, key, "events_processed_total", m.eventsProcessed); err != nil {
		return nil, err
	}
	if m.indexUpdates, err = metric.RegisterOrGet(registry, key, "updates_total", m.indexUpdates); err != nil {
		return nil, err
	}
	if m.kvOperations, err = metric.RegisterOrGet(registry, key, "kv_operations_total", m.kvOperations); err != nil {
		return nil, err
	}
	if m.watchEvents, err = metric.RegisterOrGet(registry, key, "watch_events_total", m.watchEvents); err != nil {
		return nil, err
	}
	if m.writeFailures, err = metric.RegisterOrGet(registry, key, "write_failures_total", m.writeFailures); err != nil {
		return nil, err
	}
	if m.reindexEvents, err = metric.RegisterOrGet(registry, key, "reindex_events_total", m.reindexEvents); err != nil {
		return nil, err
	}
	if m.reconcileOperations, err = metric.RegisterOrGet(registry, key, "reconcile_operations_total", m.reconcileOperations); err != nil {
		return nil, err
	}
	if m.aliasesSkipped, err = metric.RegisterOrGet(registry, key, "aliases_skipped_total", m.aliasesSkipped); err != nil {
		return nil, err
	}
	if m.queryRowsSkipped, err = metric.RegisterOrGet(registry, key, "query_rows_skipped_total", m.queryRowsSkipped); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *indexMetrics) recordReconcileOperation(indexType, operation string, err error) {
	result := "success"
	if err != nil {
		result = "failure"
	}
	m.reconcileOperations.WithLabelValues(indexType, operation, result).Inc()
}

// recordReindex records a re-index event, labeled by whether the index-input projection
// was unchanged from the last indexed one.
func (m *indexMetrics) recordReindex(unchanged bool) {
	result := "changed"
	if unchanged {
		result = "unchanged"
	}
	m.reindexEvents.WithLabelValues(result).Inc()
}
