//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// TestIntegration_ReadinessGaugesAreEmitted closes the #763 requirement for this
// producer on the WIRE: the spec's minimum gauge set must actually be scrapeable, not
// merely constructed. A field-level check would pass even if registration were skipped.
//
// The gauges are the component's own, registered on the registry it is built with
// (design D4); the pin gathered from prometheus.DefaultGatherer.
func gatheredValue(t *testing.T, g prometheus.Gatherer, name string) float64 {
	t.Helper()
	fams, err := g.Gather()
	require.NoError(t, err)
	for _, f := range fams {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			if g := m.GetGauge(); g != nil {
				return g.GetValue()
			}
		}
	}
	return -1
}

func TestIntegration_ReadinessGaugesAreEmitted(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	client := newFixtureClient(t, entityStream)
	registry := metric.NewMetricsRegistry()

	cfg := DefaultConfig()
	cj, err := json.Marshal(cfg)
	require.NoError(t, err)
	deps := testDependencies(t, client)
	deps.MetricsRegistry = registry
	comp, err := CreateGraphIngest(cj, deps)
	require.NoError(t, err)
	c := comp.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.finish(ctx, t)
	c.statusInterval = 100 * time.Millisecond
	require.NoError(t, c.Initialize())
	registerMergeTestPayload(t, c)
	require.NoError(t, c.Start(owner.startContext(ctx)))

	// Assert a VALUE this component drove, not mere presence. Presence alone is
	// satisfied by any earlier test's collectors, because DefaultRegisterer swallows
	// AlreadyRegistered — so the original form could pass before Start even ran.
	// bootstrap_complete going to 1 can only come from a real Set on a caught-up
	// producer.
	require.Eventually(t, func() bool {
		return gatheredValue(t, registry.PrometheusRegistry(), "semengine_graph_ingest_bootstrap_complete") == 1
	}, 20*time.Second, 100*time.Millisecond,
		"bootstrap_complete must reach 1 — presence alone proves only that some "+
			"instance registered, not that this one projected")

	fams, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)
	emitted := map[string]bool{}
	for _, f := range fams {
		if strings.HasPrefix(f.GetName(), "semengine_graph_ingest_") {
			emitted[f.GetName()] = true
		}
	}
	for _, want := range []string{
		"semengine_graph_ingest_readiness",
		"semengine_graph_ingest_lag",
		"semengine_graph_ingest_bootstrap_complete",
		"semengine_graph_ingest_readiness_state",
	} {
		require.True(t, emitted[want], "missing required series %q; emitted=%v", want, emitted)
	}
	// A backlog producer must NOT synthesize revisions.
	for name := range emitted {
		require.NotContains(t, name, "revision",
			"backlog producer emitted %q — a fabricated revision is worse than an absent one", name)
	}
}
