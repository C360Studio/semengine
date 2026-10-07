package readiness

import (
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// TestGauges_MetricNamesArePinned is the external-contract gate.
//
// Emitted metric names are consumed by dashboards and operator alerts, and the
// graph-index-readiness spec explicitly protects graph-index's published output. This
// pins the EXACT set the two hand-rolled implementations emitted before the shared set
// replaced them, so "we preserved the names" is a gate rather than a claim.
//
// The revision-lag list is transcribed from the pre-existing registration blocks
// (processor/graph-index/metrics.go and processor/graph-embedding/metrics.go), which
// registered an identical set for both producers.
func TestGauges_MetricNamesArePinned(t *testing.T) {
	t.Run("revision-lag producer", func(t *testing.T) {
		// Exactly what graph-index and graph-embedding registered before, PLUS
		// bootstrap_complete — the field both independently omitted and the one
		// unmet spec requirement this closes.
		want := []string{
			"readiness",
			"lag",
			"bootstrap_complete",
			"indexed_revision",
			"target_revision",
			"readiness_state",
			"status_publish_failures_total",
		}
		got := NewGauges(
			ProducerNames{Service: "graph-index", Subsystem: "graph_index"},
			WithRevisionGauges(),
		).MetricNames()
		assertNames(t, got, want)
	})

	t.Run("backlog producer omits revision gauges", func(t *testing.T) {
		// A backlog producer SHALL NOT expose revision gauges and SHALL NOT
		// synthesize a value — a fabricated revision is worse than an absent one.
		want := []string{
			"readiness",
			"lag",
			"bootstrap_complete",
			"readiness_state",
			"status_publish_failures_total",
		}
		got := NewGauges(
			ProducerNames{Service: "graph-ingest", Subsystem: "graph_ingest"},
		).MetricNames()
		assertNames(t, got, want)
	})
}

func assertNames(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("emitted %d names, want %d:\n got: %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

// TestGauges_FullyQualifiedNamesPreserveTheWireContract asserts on the names Prometheus
// actually EXPOSES, not just the registry keys. A correct registry key with a wrong
// namespace or subsystem would still rename the series a dashboard queries.
func TestGauges_FullyQualifiedNamesPreserveTheWireContract(t *testing.T) {
	g, reg := registeredGauges(t, ProducerNames{Service: "graph-index", Subsystem: "graph_index"},
		WithRevisionGauges())
	g.Set(graph.IndexStatusResponse{State: graph.IndexStateReady})

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	emitted := make(map[string]bool, len(families))
	for _, f := range families {
		emitted[f.GetName()] = true
	}

	// These are the fully-qualified series the pre-existing producers emitted.
	for _, want := range []string{
		"semengine_graph_index_readiness",
		"semengine_graph_index_lag",
		"semengine_graph_index_indexed_revision",
		"semengine_graph_index_target_revision",
		"semengine_graph_index_readiness_state",
		"semengine_graph_index_status_publish_failures_total",
		// New, and the point of the exercise.
		"semengine_graph_index_bootstrap_complete",
	} {
		if !emitted[want] {
			t.Errorf("missing series %q — a dashboard querying it would go dark; got %v",
				want, keys(emitted))
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestGauges_SetProjectsTheEnvelope pins the projection, including the two things that
// are easy to get subtly wrong.
func TestGauges_SetProjectsTheEnvelope(t *testing.T) {
	g, reg := registeredGauges(t, ProducerNames{Service: "t", Subsystem: "t"}, WithRevisionGauges())

	g.Set(graph.IndexStatusResponse{
		Ready:             true,
		State:             graph.IndexStateReady,
		BootstrapComplete: true,
		Lag:               7,
		IndexedRevision:   100,
		TargetRevision:    107,
	})

	if got := gaugeValue(t, reg, "semengine_t_bootstrap_complete"); got != 1 {
		t.Errorf("bootstrap_complete = %v, want 1", got)
	}
	if got := gaugeValue(t, reg, "semengine_t_lag"); got != 7 {
		t.Errorf("lag = %v, want 7", got)
	}

	// The one-hot must iterate the CLOSED state domain, so a state this producer is
	// not currently in renders as an explicit 0 rather than an absent series —
	// absent reads as "no data", which is not the same as "not in this state".
	states := labeledValues(t, reg, "semengine_t_readiness_state", "state")
	if len(states) != len(graph.AllIndexStates) {
		t.Errorf("one-hot emitted %d states, want all %d: %v",
			len(states), len(graph.AllIndexStates), states)
	}
	if states[graph.IndexStateReady] != 1 || states[graph.IndexStateDegraded] != 0 {
		t.Errorf("one-hot wrong: %v", states)
	}
}

// TestGauges_BacklogProducerNeverPublishesARevision is the fail-closed guard on the
// spec's explicit prohibition: a backlog producer must not synthesize a revision, and
// publishing a zero-valued gauge WOULD be synthesizing one.
func TestGauges_BacklogProducerNeverPublishesARevision(t *testing.T) {
	g, reg := registeredGauges(t, ProducerNames{Service: "graph-ingest", Subsystem: "graph_ingest"})

	// An envelope that (wrongly) carried revisions must still not produce the series.
	g.Set(graph.IndexStatusResponse{
		State:           graph.IndexStateReady,
		IndexedRevision: 42,
		TargetRevision:  99,
	})

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if strings.Contains(f.GetName(), "revision") {
			t.Errorf("backlog producer published %q — a fabricated revision is worse "+
				"than an absent one", f.GetName())
		}
	}
}

func gaugeValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		if len(f.GetMetric()) == 0 {
			t.Fatalf("%s has no metrics", name)
		}
		return f.GetMetric()[0].GetGauge().GetValue()
	}
	t.Fatalf("series %q not found", name)
	return 0
}

func labeledValues(t *testing.T, reg *prometheus.Registry, name, label string) map[string]float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]float64{}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == label {
					out[lp.GetValue()] = m.GetGauge().GetValue()
				}
			}
		}
	}
	return out
}

// registeredGauges builds a gauge set and registers it the production way, on a
// metric.MetricsRegistry, returning the Prometheus registry that gathers it.
func registeredGauges(t *testing.T, names ProducerNames, opts ...GaugeOption) (*Gauges, *prometheus.Registry) {
	t.Helper()
	registry := metric.NewMetricsRegistry()
	g := NewGauges(names, opts...)
	if err := g.Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return g, registry.PrometheusRegistry()
}

// TestGaugesWriteTheRegisteredCollector: a second gauge set registered under the same
// service on one registry writes the collectors the first one registered. Had it kept
// its own candidates, its writes would go to collectors nothing gathers (design D4).
func TestGaugesWriteTheRegisteredCollector(t *testing.T) {
	names := ProducerNames{Service: "t", Subsystem: "t"}
	registry := metric.NewMetricsRegistry()
	first := NewGauges(names, WithRevisionGauges())
	second := NewGauges(names, WithRevisionGauges())
	for _, g := range []*Gauges{first, second} {
		if err := g.Register(registry); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	second.Set(graph.IndexStatusResponse{
		State: graph.IndexStateDegraded, Lag: 7, IndexedRevision: 100, TargetRevision: 107,
	})
	second.RecordPublishFailure()

	reg := registry.PrometheusRegistry()
	for name, want := range map[string]float64{
		"semengine_t_lag":              7,
		"semengine_t_indexed_revision": 100,
		"semengine_t_target_revision":  107,
	} {
		if got := gaugeValue(t, reg, name); got != want {
			t.Errorf("%s = %v, want %v from the second set's write", name, got, want)
		}
	}
	if states := labeledValues(t, reg, "semengine_t_readiness_state", "state"); states[graph.IndexStateDegraded] != 1 {
		t.Errorf("readiness_state = %v, want degraded=1 from the second set's write", states)
	}
	if got := counterValue(t, reg, "semengine_t_status_publish_failures_total"); got != 1 {
		t.Errorf("status_publish_failures_total = %v, want 1 from the second set's write", got)
	}
}

// TestGaugesNilRegistryRegistersNothing: with no registry the set registers nowhere,
// not on Prometheus' global registry as the pin did, and still takes writes.
func TestGaugesNilRegistryRegistersNothing(t *testing.T) {
	g := NewGauges(ProducerNames{Service: "nil-registry", Subsystem: "nil_registry"}, WithRevisionGauges())
	if err := g.Register(nil); err != nil {
		t.Fatalf("Register(nil) = %v, want nil", err)
	}
	g.Set(graph.IndexStatusResponse{State: graph.IndexStateReady, Lag: 3})
	g.RecordPublishFailure()

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if strings.HasPrefix(f.GetName(), "semengine_nil_registry_") {
			t.Errorf("global registry gathers %q", f.GetName())
		}
	}
}

// TestGaugesRegisterReturnsARefusal: a registration the registry refuses is returned,
// not dropped as the pin's Register dropped it.
func TestGaugesRegisterReturnsARefusal(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "occupied_lag", Help: "a counter under the gauge's key"})
	if _, err := metric.RegisterOrGet(registry, "t", "lag", counter); err != nil {
		t.Fatalf("seed registration: %v", err)
	}
	if err := NewGauges(ProducerNames{Service: "t", Subsystem: "t"}).Register(registry); err == nil {
		t.Fatal("Register = nil, want the refusal of a gauge under a key a counter holds")
	}
}

func counterValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() == name && len(f.GetMetric()) > 0 {
			return f.GetMetric()[0].GetCounter().GetValue()
		}
	}
	t.Fatalf("series %q not found", name)
	return 0
}
