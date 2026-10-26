package readiness

import (
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// TestGauges_FullyQualifiedNamesPreserveTheWireContract asserts on the names Prometheus
// actually EXPOSES, not just the registry keys. A correct registry key with a wrong
// namespace or subsystem would still rename the series a dashboard queries.
func TestGauges_FullyQualifiedNamesPreserveTheWireContract(t *testing.T) {
	g, reg := registeredGauges(t, "graph_index")
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

	// graph-ingest's set, built as metrics.go builds it, exposes exactly its five
	// semengine families: none missing, none renamed, nothing extra.
	t.Run("graph_ingest exposes exactly its five families", func(t *testing.T) {
		registry := metric.NewMetricsRegistry()
		ingest := NewGauges("graph_ingest")
		if err := ingest.Register(registry); err != nil {
			t.Fatalf("Register: %v", err)
		}
		ingest.Set(graph.IndexStatusResponse{State: graph.IndexStateReady})

		families, err := registry.PrometheusRegistry().Gather()
		if err != nil {
			t.Fatalf("gather: %v", err)
		}
		// The registry also carries the Go runtime and process collectors; every
		// family under the semengine namespace must be one of the five.
		var got []string
		for _, f := range families {
			if strings.HasPrefix(f.GetName(), "semengine_") {
				got = append(got, f.GetName())
			}
		}
		slices.Sort(got)
		want := []string{
			"semengine_graph_ingest_bootstrap_complete",
			"semengine_graph_ingest_lag",
			"semengine_graph_ingest_readiness",
			"semengine_graph_ingest_readiness_state",
			"semengine_graph_ingest_status_publish_failures_total",
		}
		if !slices.Equal(got, want) {
			t.Errorf("exposed families = %q, want exactly %q", got, want)
		}
	})
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
	g, reg := registeredGauges(t, "t")

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
	if len(states) != len(graph.IndexStates()) {
		t.Errorf("one-hot emitted %d states, want all %d: %v",
			len(states), len(graph.IndexStates()), states)
	}
	if states[string(graph.IndexStateReady)] != 1 || states[string(graph.IndexStateDegraded)] != 0 {
		t.Errorf("one-hot wrong: %v", states)
	}
}

// TestGauges_BacklogProducerNeverPublishesARevision is the fail-closed guard on the
// spec's explicit prohibition: a backlog producer must not synthesize a revision, and
// publishing a zero-valued gauge WOULD be synthesizing one.
func TestGauges_BacklogProducerNeverPublishesARevision(t *testing.T) {
	g, reg := registeredGauges(t, "graph_ingest")

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
func registeredGauges(t *testing.T, subsystem string) (*Gauges, *prometheus.Registry) {
	t.Helper()
	registry := metric.NewMetricsRegistry()
	g := NewGauges(subsystem)
	if err := g.Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return g, registry.PrometheusRegistry()
}

// TestGaugesWriteTheRegisteredCollector: a second gauge set registered under the same
// subsystem on one registry writes the collectors the first one registered. Had it kept
// its own candidates, its writes would go to collectors nothing gathers (design D4).
func TestGaugesWriteTheRegisteredCollector(t *testing.T) {
	registry := metric.NewMetricsRegistry()
	first := NewGauges("t")
	second := NewGauges("t")
	for _, g := range []*Gauges{first, second} {
		if err := g.Register(registry); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	second.Set(graph.IndexStatusResponse{State: graph.IndexStateDegraded, Lag: 7})
	second.RecordPublishFailure()

	reg := registry.PrometheusRegistry()
	if got := gaugeValue(t, reg, "semengine_t_lag"); got != 7 {
		t.Errorf("semengine_t_lag = %v, want 7 from the second set's write", got)
	}
	if states := labeledValues(t, reg, "semengine_t_readiness_state", "state"); states[string(graph.IndexStateDegraded)] != 1 {
		t.Errorf("readiness_state = %v, want degraded=1 from the second set's write", states)
	}
	if got := counterValue(t, reg, "semengine_t_status_publish_failures_total"); got != 1 {
		t.Errorf("status_publish_failures_total = %v, want 1 from the second set's write", got)
	}
}

// TestGaugesNilRegistryRegistersNothing: with no registry the set registers nowhere,
// not on Prometheus' global registry as the pin did, and still takes writes.
func TestGaugesNilRegistryRegistersNothing(t *testing.T) {
	g := NewGauges("nil_registry")
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
	if err := NewGauges("t").Register(registry); err == nil {
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
