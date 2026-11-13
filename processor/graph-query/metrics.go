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
}

// newQueryMetrics builds graph-query's collectors and registers them on registry, the
// component.Dependencies.MetricsRegistry it was built with, through metric.RegisterOrGet, keeping
// the collector the call returns: a second graph-query on the same registry counts into the
// first's. With a nil registry the collectors are built and registered nowhere; nothing is
// registered on Prometheus' process-global registry.
func newQueryMetrics(registry *metric.MetricsRegistry) (*queryMetrics, error) {
	m := &queryMetrics{searchTruncated: prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine", Subsystem: strings.ReplaceAll(key, "-", "_"), Name: "search_truncated_total",
		Help: "Search answers cut short, by verb",
	}, []string{"verb"})}
	if registry == nil {
		return m, nil
	}
	var err error
	if m.searchTruncated, err = metric.RegisterOrGet(registry, key, "search_truncated_total", m.searchTruncated); err != nil {
		return nil, err
	}
	return m, nil
}
