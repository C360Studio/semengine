package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
)

// gaugeValue and requireGauge are the pin's (storage_observability_test.go:75-112), moved here for
// the startup-metric tests (design D7). They also read a counter, such as semengine_errors_total
// (TestHealthPublishFailureIsCounted): the pin read only gauges.

func gaugeValue(t *testing.T, registry *metric.MetricsRegistry, name string, labels map[string]string) (float64, bool) {
	t.Helper()
	families, err := registry.PrometheusRegistry().Gather()
	require.NoError(t, err)

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, m := range family.GetMetric() {
			matched := len(labels) > 0 || len(m.GetLabel()) == 0
			for key, want := range labels {
				found := false
				for _, label := range m.GetLabel() {
					if label.GetName() == key && label.GetValue() == want {
						found = true
						break
					}
				}
				if !found {
					matched = false
					break
				}
			}
			if matched {
				if counter := m.GetCounter(); counter != nil {
					return counter.GetValue(), true
				}
				return m.GetGauge().GetValue(), true
			}
		}
	}
	return 0, false
}

func requireGauge(t *testing.T, registry *metric.MetricsRegistry, name string, labels map[string]string) float64 {
	t.Helper()
	value, ok := gaugeValue(t, registry, name, labels)
	require.True(t, ok, "metric %s%v must be published", name, labels)
	return value
}
