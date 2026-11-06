package graphquery

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/c360studio/semengine/metric"
)

const truncatedFamily = "semengine_graph_query_search_truncated_total"

// truncatedCount is the literal value the registry gathers for the verb's series, 0 when it has none.
func truncatedCount(t *testing.T, gatherer prometheus.Gatherer, verb string) float64 {
	t.Helper()
	families, err := gatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != truncatedFamily {
			continue
		}
		for _, series := range family.GetMetric() {
			if labelOf(series, "verb") == verb {
				return series.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func labelOf(series *dto.Metric, name string) string {
	for _, label := range series.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}

// TestSearchTruncatedSharesItsCollector: a second graph-query on one registry counts into the
// collector the first registered, so the registry gathers both counts (metric.RegisterOrGet returns the
// canonical collector; a caller's own candidate is not gathered).
func TestSearchTruncatedSharesItsCollector(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	first, err := newQueryMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newQueryMetrics(registry)
	if err != nil {
		t.Fatal(err)
	}
	first.searchTruncated.WithLabelValues("localSearch").Inc()
	second.searchTruncated.WithLabelValues("localSearch").Inc()
	gatherer := registry.PrometheusRegistry()
	if got := truncatedCount(t, gatherer, "localSearch"); got != 2 {
		t.Errorf("localSearch = %v, want 2", got)
	}
}
