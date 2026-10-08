package graphingest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/types"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// newUnstartedGraphIngest builds a graph-ingest component through CreateGraphIngest on
// registry, with a NATS client that is never connected: construction is all these tests
// exercise, and construction opens no connection.
func newUnstartedGraphIngest(t *testing.T, registry *metric.MetricsRegistry, rawConfig json.RawMessage) (*Component, error) {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphIngest(rawConfig, component.Dependencies{
		NATSClient:      client,
		PayloadRegistry: payloadregistry.New(),
		MetricsRegistry: registry,
		Platform:        types.PlatformMeta{Org: "acme", Platform: "ops"},
	})
	if err != nil {
		return nil, err
	}
	return built.(*Component), nil
}

// isGraphIngestFamily reports whether a gathered family is one of graph-ingest's own
// collectors (README, "Metrics"): the graph_ingest subsystem, and the datamanager counter.
func isGraphIngestFamily(name string) bool {
	return strings.HasPrefix(name, "semengine_graph_ingest_") ||
		strings.HasPrefix(name, "semengine_datamanager_")
}

// gatherFamilies gathers g and returns graph-ingest's families by name.
func gatherFamilies(t *testing.T, g prometheus.Gatherer) map[string]*dto.MetricFamily {
	t.Helper()
	families, err := g.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	found := map[string]*dto.MetricFamily{}
	for _, family := range families {
		if isGraphIngestFamily(family.GetName()) {
			found[family.GetName()] = family
		}
	}
	return found
}

// suppressedOnLane is the value of duplicate_triples_suppressed_total{lane} in families,
// and whether the series exists.
func suppressedOnLane(families map[string]*dto.MetricFamily, lane dedupLane) (float64, bool) {
	family, ok := families["semengine_graph_ingest_duplicate_triples_suppressed_total"]
	if !ok {
		return 0, false
	}
	for _, m := range family.GetMetric() {
		for _, label := range m.GetLabel() {
			if label.GetName() == "lane" && label.GetValue() == string(lane) {
				return m.GetCounter().GetValue(), true
			}
		}
	}
	return 0, false
}

// TestGraphIngestMetricsRegisterOnItsRegistry: two components built on two registries
// each put their series on their own registry, a write through one component is gathered
// only from its registry, and Prometheus' global registry gathers none of them (design D4;
// metric-registry, "Two registries, two components").
func TestGraphIngestMetricsRegisterOnItsRegistry(t *testing.T) {
	registryA, registryB := metric.NewMetricsRegistry(), metric.NewMetricsRegistry()
	componentA, err := newUnstartedGraphIngest(t, registryA, nil)
	if err != nil {
		t.Fatalf("CreateGraphIngest on registry A: %v", err)
	}
	if _, err := newUnstartedGraphIngest(t, registryB, nil); err != nil {
		t.Fatalf("CreateGraphIngest on registry B: %v", err)
	}

	componentA.recordSuppressedDuplicates(dedupLaneAddBatch, 3)

	// Series that exist from construction, with no write: plain counters, histograms
	// and the gauge, and the batch counter whose reasons are pre-initialized.
	alwaysPresent := []string{
		"semengine_datamanager_entities_updated_total",
		"semengine_graph_ingest_processing_duration_seconds",
		"semengine_graph_ingest_ingest_lag_seconds",
		"semengine_graph_ingest_redeliveries_dropped_total",
		"semengine_graph_ingest_cas_retries_total",
		"semengine_graph_ingest_stale_sets_total",
		"semengine_graph_ingest_poisoned_entities",
		"semengine_graph_ingest_batch_query_missing_total",
	}
	familiesA := gatherFamilies(t, registryA.PrometheusRegistry())
	familiesB := gatherFamilies(t, registryB.PrometheusRegistry())
	for _, name := range alwaysPresent {
		if _, ok := familiesA[name]; !ok {
			t.Errorf("registry A does not gather %s", name)
		}
		if _, ok := familiesB[name]; !ok {
			t.Errorf("registry B does not gather %s", name)
		}
	}
	if got, ok := suppressedOnLane(familiesA, dedupLaneAddBatch); !ok || got != 3 {
		t.Errorf("registry A duplicate_triples_suppressed_total{lane=append} = %v (present %v), want 3", got, ok)
	}
	if got, ok := suppressedOnLane(familiesB, dedupLaneAddBatch); ok {
		t.Errorf("registry B gathers component A's write: duplicate_triples_suppressed_total{lane=append} = %v", got)
	}
	for name := range gatherFamilies(t, prometheus.DefaultGatherer) {
		t.Errorf("Prometheus' global registry gathers graph-ingest's %s", name)
	}
}

// TestGraphIngestNilRegistryRegistersNothing: a component built with no metrics registry
// takes writes and registers its collectors nowhere, so Prometheus' global registry
// gathers none of its series (design D4; metric-registry, "A component built without a
// registry").
func TestGraphIngestNilRegistryRegistersNothing(t *testing.T) {
	c, err := newUnstartedGraphIngest(t, nil, nil)
	if err != nil {
		t.Fatalf("CreateGraphIngest with a nil registry: %v", err)
	}

	c.recordSuppressedDuplicates(dedupLaneAddBatch, 1)

	for name := range gatherFamilies(t, prometheus.DefaultGatherer) {
		t.Errorf("Prometheus' global registry gathers graph-ingest's %s", name)
	}
}
