package graphquery

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
)

// queryMetrics holds graph-query's own collectors.
type queryMetrics struct {
	// searchTruncated counts the search answers cut short, by verb: the number that says when a
	// keyword index per community is due.
	searchTruncated *prometheus.CounterVec
	// searchMergeFailed counts the searchGraph answers given as the word list alone because the
	// embedding search failed: how often tier 1 search ran at tier 0.
	searchMergeFailed prometheus.Counter
}

// newQueryMetrics builds graph-query's collectors and registers them on registry, the
// component.Dependencies.MetricsRegistry it was built with, through metric.RegisterOrGet, keeping
// the collector the call returns: a second graph-query on the same registry counts into the
// first's. With a nil registry the collectors are built and registered nowhere; nothing is
// registered on Prometheus' process-global registry.
func newQueryMetrics(registry *metric.MetricsRegistry) (*queryMetrics, error) {
	subsystem := strings.ReplaceAll(key, "-", "_")
	m := &queryMetrics{
		searchTruncated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "semengine", Subsystem: subsystem, Name: "search_truncated_total",
			Help: "Search answers cut short, by verb",
		}, []string{"verb"}),
		searchMergeFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semengine", Subsystem: subsystem, Name: "search_merge_failed_total",
			Help: "searchGraph answers given as the word list alone because the embedding search failed",
		}),
	}
	if registry == nil {
		return m, nil
	}
	var err error
	if m.searchTruncated, err = metric.RegisterOrGet(registry, key, "search_truncated_total", m.searchTruncated); err != nil {
		return nil, err
	}
	if m.searchMergeFailed, err = metric.RegisterOrGet(registry, key, "search_merge_failed_total", m.searchMergeFailed); err != nil {
		return nil, err
	}
	return m, nil
}
