package graphindex

import (
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// newOnRegistry builds graph-index with the default configuration on registry, with a
// NATS client that is never connected.
func newOnRegistry(t *testing.T, registry *metric.MetricsRegistry) *Component {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphIndex(nil, component.Dependencies{NATSClient: client, MetricsRegistry: registry})
	if err != nil {
		t.Fatalf("CreateGraphIndex: %v", err)
	}
	return built.(*Component)
}

// graphIndexFamilies gathers g and returns graph-index's own families by name.
func graphIndexFamilies(t *testing.T, g prometheus.Gatherer) map[string]*dto.MetricFamily {
	t.Helper()
	families, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	found := map[string]*dto.MetricFamily{}
	for _, family := range families {
		if strings.HasPrefix(family.GetName(), "semengine_graph_index_") {
			found[family.GetName()] = family
		}
	}
	return found
}

// reindexCount is reindex_events_total{result} in families, and whether the series exists.
func reindexCount(families map[string]*dto.MetricFamily, result string) (float64, bool) {
	for _, m := range families["semengine_graph_index_reindex_events_total"].GetMetric() {
		for _, label := range m.GetLabel() {
			if label.GetName() == "result" && label.GetValue() == result {
				return m.GetCounter().GetValue(), true
			}
		}
	}
	return 0, false
}

// TestGraphIndexMetricsRegisterOnItsRegistry: two components built on two registries each
// put their series on their own registry under the pin's names (namespace semengine,
// subsystem graph_index), a write through one is gathered only from its registry, and
// Prometheus' global registry gathers none (design D10; metric-registry, "Two
// registries, two components").
func TestGraphIndexMetricsRegisterOnItsRegistry(t *testing.T) {
	registryA, registryB := metric.NewMetricsRegistry(), metric.NewMetricsRegistry()
	componentA := newOnRegistry(t, registryA)
	newOnRegistry(t, registryB)

	componentA.metrics.recordReindex(true)
	componentA.metrics.recordReindex(true)
	componentA.metrics.recordReindex(false)

	// Plain counters exist from construction, with no write.
	alwaysPresent := []string{
		"semengine_graph_index_events_processed_total",
		"semengine_graph_index_write_failures_total",
	}
	familiesA := graphIndexFamilies(t, registryA.PrometheusRegistry())
	familiesB := graphIndexFamilies(t, registryB.PrometheusRegistry())
	for _, name := range alwaysPresent {
		if _, ok := familiesA[name]; !ok {
			t.Errorf("registry A does not gather %s", name)
		}
		if _, ok := familiesB[name]; !ok {
			t.Errorf("registry B does not gather %s", name)
		}
	}
	if got, ok := reindexCount(familiesA, "unchanged"); !ok || got != 2 {
		t.Errorf("registry A reindex_events_total{result=unchanged} = %v (present %v), want 2", got, ok)
	}
	if got, ok := reindexCount(familiesA, "changed"); !ok || got != 1 {
		t.Errorf("registry A reindex_events_total{result=changed} = %v (present %v), want 1", got, ok)
	}
	if got, ok := reindexCount(familiesB, "unchanged"); ok {
		t.Errorf("registry B gathers component A's write: reindex_events_total{result=unchanged} = %v", got)
	}
	for name := range graphIndexFamilies(t, prometheus.DefaultGatherer) {
		t.Errorf("Prometheus' global registry gathers graph-index's %s", name)
	}
}

// TestGraphIndexSecondInstanceSharesTheCollectors: a second graph-index on the same
// registry builds, and writes the collectors the first registered, so its writes are
// gathered (metric.RegisterOrGet returns the held collector).
func TestGraphIndexSecondInstanceSharesTheCollectors(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	newOnRegistry(t, registry)
	second := newOnRegistry(t, registry)

	second.metrics.recordReindex(false)

	if got, ok := reindexCount(graphIndexFamilies(t, registry.PrometheusRegistry()), "changed"); !ok || got != 1 {
		t.Fatalf("the second instance's write is not gathered: reindex_events_total{result=changed} = %v (present %v)", got, ok)
	}
}

// TestGraphIndexNilRegistryRegistersNothing: a component built with no metrics registry
// takes writes and registers its collectors nowhere (metric-registry, "A component built
// without a registry").
func TestGraphIndexNilRegistryRegistersNothing(t *testing.T) {
	c := newOnRegistry(t, nil)

	c.metrics.recordReindex(true)
	c.metrics.recordReconcileOperation("outgoing", "put", nil)

	for name := range graphIndexFamilies(t, prometheus.DefaultGatherer) {
		t.Errorf("Prometheus' global registry gathers graph-index's %s", name)
	}
}
