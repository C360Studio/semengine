//go:build integration

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/types"
)

// TestSystemHealthCarriesTheConfigManagersHealth: /health from a manager with a started config
// manager carries a "config" sub-status that is the config manager's own Health, so an operator
// reading /health sees when configuration has stopped reaching the process.
//
// spec: operator-surface / Health and service listings are served as at the pin
func TestSystemHealthCarriesTheConfigManagersHealth(t *testing.T) {
	client := fixtureClient(t)
	deps := createTestServiceDependencies()
	configManager, err := config.NewConfigManager(&config.Config{
		Version:    "1.0.0",
		Platform:   config.PlatformConfig{Org: "test", ID: "system-health", Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: make(config.ComponentConfigs),
	}, client, deps.Logger)
	require.NoError(t, err)
	require.NoError(t, configManager.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, configManager.Stop(ctx))
	})

	deps.NATSClient = client
	deps.Manager = configManager
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", newStandInComponentManager()))
	startTestManager(t, manager)

	status, body, err := httpGet(t.Context(), observedManagerHTTPURL(t, manager)+"/health")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, body)

	var system health.Status
	require.NoError(t, json.Unmarshal([]byte(body), &system))
	var got *health.Status
	for i := range system.SubStatuses {
		if system.SubStatuses[i].Component == "config" {
			got = &system.SubStatuses[i]
		}
	}
	require.NotNil(t, got, "no config sub-status in %s", body)
	want := configManager.Health()
	require.True(t, got.Healthy, "config sub-status %+v", *got)
	require.Equal(t, want.Status, got.Status)
	require.Equal(t, want.Message, got.Message)
}
