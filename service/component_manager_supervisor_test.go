package service

import (
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/metric"
)

// The supervisor's tests new in this port (design D4, D7): the health publisher counts the publish
// it could not make, and the override-expiry reporter writes to the gauge metric.RegisterOrGet
// returned, not to its own candidate. The reporter's carried tests are in
// stream_override_expiry_test.go, whose composeComponentManagerWithOverride these use.

// healthPublishErrors is the error count's labels for a health publish the component manager
// could not make.
var healthPublishErrors = map[string]string{"service": "component-manager", "type": "health_publish"}

// TestComponentHealthPublishFailureIsCounted: over a NATS client that is not connected, the health
// publisher's publish for each admitted component fails once when Start returns and again every 5
// seconds; each failure increments semengine_errors_total{service="component-manager",
// type="health_publish"} and logs at Debug naming the subject, the loop goes on, and nothing is
// published after Stop (design D4). The bubble's fake clock makes each count exact.
//
// spec: service-lifecycle / Component health is published and its drops counted
func TestComponentHealthPublishFailureIsCounted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs := &syncBuffer{}
		logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		metrics := metric.NewMetricsRegistry()
		cm := composeComponentManagerWithOverride(t, nil, logger, metrics,
			testComponent{name: "publish-a", instance: &mockDiscoverableComponent{}},
			testComponent{name: "publish-b", instance: &mockDiscoverableComponent{}})

		before, _ := gaugeValue(t, metrics, "semengine_errors_total", healthPublishErrors)
		require.Zero(t, before, "construction published health")

		require.NoError(t, cm.Start(t.Context()))
		synctest.Wait()
		require.Equal(t, 2.0, requireGauge(t, metrics, "semengine_errors_total", healthPublishErrors),
			"the publish of each of the two components' health when Start returned was not counted")
		require.Contains(t, logs.String(), "health.component.publish-a", "the failed publish's log names its subject")
		require.Contains(t, logs.String(), "health.component.publish-b", "the failed publish's log names its subject")

		<-time.After(5 * time.Second)
		synctest.Wait()
		require.Equal(t, 4.0, requireGauge(t, metrics, "semengine_errors_total", healthPublishErrors),
			"the publisher did not go on after a failed publish: no second round 5 s after Start")

		require.NoError(t, cm.Stop(stopContext(t)))
		<-time.After(5 * time.Second)
		synctest.Wait()
		require.Equal(t, 4.0, requireGauge(t, metrics, "semengine_errors_total", healthPublishErrors),
			"the publisher published after Stop returned")
	})
}

// TestOverrideGaugeUsesTheRegisteredCollector: two component managers built on one metrics
// registry share one override gauge, so the second manager's evaluation at construction shows on
// the gathered family (design D4). A manager that wrote to its own candidate collector, not to the
// one metric.RegisterOrGet returned, would leave the second manager's series out of the family.
//
// spec: service-lifecycle / A lapsed stream migration override is reported while running
func TestOverrideGaugeUsesTheRegisteredCollector(t *testing.T) {
	metrics := metric.NewMetricsRegistry()

	composeComponentManagerWithOverride(t, config.StreamMigrationOverrides{
		"FIRST": {Owner: "team-first", Expires: "2020-01-01", Reason: "sizing study"},
	}, discardLogger, metrics)
	require.Equal(t, 1.0,
		requireGauge(t, metrics, overrideExpiryMetric, map[string]string{"stream": "FIRST", "owner": "team-first"}),
		"the first manager's evaluation is not on the registered gauge")

	composeComponentManagerWithOverride(t, config.StreamMigrationOverrides{
		"SECOND": {Owner: "team-second", Expires: "2020-01-01", Reason: "sizing study"},
	}, discardLogger, metrics)
	value, ok := gaugeValue(t, metrics, overrideExpiryMetric, map[string]string{"stream": "SECOND", "owner": "team-second"})
	require.True(t, ok, "the second manager's evaluation is not on the gathered gauge family: "+
		"it wrote to its own candidate, not to the collector RegisterOrGet returned")
	require.Equal(t, 1.0, value)
}
