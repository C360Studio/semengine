package service

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
)

// The heartbeat's and the metrics forwarder's shared test helpers (design D6, U-a). Every
// service is built the one production way, through its registered constructor, so a test sees
// only what the constructor composes.

// newTestHeartbeat builds a heartbeat service the way a composition does: a service manager
// holding a component manager composing components (newTestComponentManager) under
// component-manager, and the heartbeat constructed by that manager through CreateService from
// config, a JSON HeartbeatConfig, with logger as Dependencies.Logger. The component manager is
// not started; the heartbeat's Start looks it up and reads its health (design D2).
func newTestHeartbeat(
	t *testing.T, config string, logger *slog.Logger, components ...testComponent,
) *HeartbeatService {
	t.Helper()
	registry := NewServiceRegistry()
	require.NoError(t, registry.Register("heartbeat", NewHeartbeatService))
	manager := NewServiceManager(registry)
	require.NoError(t, manager.RegisterInstance(componentManagerName, newTestComponentManager(t, components...)))
	built, err := manager.CreateService("heartbeat", json.RawMessage(config), &Dependencies{Logger: logger})
	require.NoError(t, err)
	hb, ok := built.(*HeartbeatService)
	require.True(t, ok, "NewHeartbeatService returned %T, want *HeartbeatService", built)
	return hb
}

// newTestMetricsForwarder builds a metrics forwarder through NewMetricsForwarderService from
// config, a JSON MetricsForwarderConfig, over client and logger, with a fresh metrics registry,
// which it returns so a test can register the series the forwarder gathers.
func newTestMetricsForwarder(
	t *testing.T, client *natsclient.Client, config string, logger *slog.Logger,
) (*MetricsForwarder, *metric.MetricsRegistry) {
	t.Helper()
	registry := metric.NewMetricsRegistry()
	built, err := NewMetricsForwarderService(json.RawMessage(config), &Dependencies{
		NATSClient:      client,
		MetricsRegistry: registry,
		Logger:          logger,
	})
	require.NoError(t, err)
	mf, ok := built.(*MetricsForwarder)
	require.True(t, ok, "NewMetricsForwarderService returned %T, want *MetricsForwarder", built)
	return mf, registry
}
