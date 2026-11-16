package graphclustering

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
)

// metricSubsystem is the Prometheus subsystem of graph-clustering's collectors, derived from its
// key ("graph-clustering" -> "graph_clustering") so the name is spelled once. The readiness gauges
// are the follower's.
var metricSubsystem = strings.ReplaceAll(key, "-", "_")

// clusteringMetrics holds graph-clustering's own collectors.
type clusteringMetrics struct {
	// publicationFailures counts ticks whose publication to COMMUNITY_INDEX failed.
	publicationFailures prometheus.Counter
}

// newClusteringMetrics builds the collectors and registers them on registry, the injected
// component.Dependencies.MetricsRegistry, through metric.RegisterOrGet, keeping the collector each
// call returns. With a nil registry they are built and registered nowhere.
func newClusteringMetrics(registry *metric.MetricsRegistry) (*clusteringMetrics, error) {
	m := &clusteringMetrics{
		publicationFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semengine", Subsystem: metricSubsystem, Name: "publication_failures_total",
			Help: "Ticks whose publication of the changed community groups to COMMUNITY_INDEX failed",
		}),
	}
	if registry == nil {
		return m, nil
	}
	var err error
	if m.publicationFailures, err = metric.RegisterOrGet(registry, key, "publication_failures_total", m.publicationFailures); err != nil {
		return nil, err
	}
	return m, nil
}
