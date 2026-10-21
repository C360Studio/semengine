//go:build integration

package service

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
)

// The component manager's tests against a live NATS server, from the pin's
// component_manager_integration_test.go. The pin's package was service_test; these are in package
// service so they reach the package's shared server through fixtureClient (#195). The pin's
// Initialize, IsInitialized and IsStarted are gone (design D1): construction builds the
// composition, and what the tests read is what the constructor, Start and Stop report. The
// helpers that build a manager over a started config.Manager are the boot findings tests'
// (component_manager_boot_findings_integration_test.go).

// TestNewComponentManagerCreatesEnabledComponents replaces the pin's
// TestComponentManagerInitializeCreatesComponents: NewComponentManager builds every enabled
// component of the configuration, and only those. The pin composed the real udp and robotics
// factories from a process-wide registry and accepted whatever it found; here each instance's
// factory is the test's own and counts its calls.
//
// spec: service-lifecycle / The component manager composes the boot set once
func TestNewComponentManagerCreatesEnabledComponents(t *testing.T) {
	client := fixtureClient(t)
	registry := component.NewRegistry()
	inputCalls := registerBootFindingsFactory(t, registry, "first-factory", component.PortConfig{})
	processorCalls := registerBootFindingsFactory(t, registry, "second-factory", component.PortConfig{})
	disabledCalls := registerBootFindingsFactory(t, registry, "disabled-factory", component.PortConfig{})
	disabled := bootFindingsProcessor("disabled-factory")
	disabled.Enabled = false

	manager := bootFindingsComponentManager(t, client, registry, "creates-components", config.ComponentConfigs{
		"test-input":     bootFindingsProcessor("first-factory"),
		"test-processor": bootFindingsProcessor("second-factory"),
		"test-disabled":  disabled,
	}, nil)

	require.Equal(t, []string{"test-input", "test-processor"}, slices.Sorted(maps.Keys(manager.GetComponentStatus())),
		"the composed components are the enabled ones")
	require.Equal(t, int32(1), inputCalls.Load(), "first-factory calls")
	require.Equal(t, int32(1), processorCalls.Load(), "second-factory calls")
	require.Equal(t, int32(0), disabledCalls.Load(), "a disabled component was built")
}

// TestComponentManagerWithRealNATS: a manager with no components starts and stops on a live
// server, and its reads answer while it runs.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestComponentManagerWithRealNATS(t *testing.T) {
	client := fixtureClient(t)
	manager := bootFindingsComponentManager(t, client, component.NewRegistry(), "real-nats", nil, nil)

	require.NoError(t, manager.Start(t.Context()), "Start")
	// assert, not require, between Start and Stop: a failed read must not leave the manager
	// running past the test.
	assert.Equal(t, StatusRunning, manager.Status(), "status after Start")
	assert.NotNil(t, manager.GetComponentHealth(), "component health")
	assert.NotNil(t, manager.GetComponentStatus(), "component status")
	// The composition projection is served even with no components.
	paths, err := manager.GetFlowPaths()
	assert.NoError(t, err, "GetFlowPaths")
	assert.NotNil(t, paths, "flow paths")

	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
	defer cancel()
	require.NoError(t, manager.Stop(ctx), "Stop")
	require.Equal(t, StatusStopped, manager.Status(), "status after Stop")
}

// TestComponentManagerCompositionProjection: a running manager with no component admitted serves
// an empty, non-nil set of flow paths from the composition kept at construction.
//
// spec: operator-surface / The composition projections serve the result kept at construction
func TestComponentManagerCompositionProjection(t *testing.T) {
	client := fixtureClient(t)
	manager := bootFindingsComponentManager(t, client, component.NewRegistry(), "projection", nil, nil)
	startTestComponentManager(t, manager)

	t.Run("GetFlowPaths", func(t *testing.T) {
		require.Empty(t, manager.GetComponentStatus(), "a fresh registry composed components")
		paths, err := manager.GetFlowPaths()
		require.NoError(t, err)
		require.NotNil(t, paths, "flow paths should not be nil")
		require.Empty(t, paths, "no paths without components")
	})
}

// TestServiceManagerMandatoryService: a composition whose services configuration names no
// component-manager, over a service registry RegisterAll filled, gets one from StartAll, built
// by NewComponentManager.
//
// spec: service-lifecycle / Every composition has a component manager
func TestServiceManagerMandatoryService(t *testing.T) {
	client := fixtureClient(t)
	deps := createTestServiceDependencies()
	deps.NATSClient = client
	deps.Manager = bootFindingsConfigManager(t, client, "mandatory-service", nil, nil)
	deps.ComponentRegistry = component.NewRegistry()
	require.NotContains(t, deps.Manager.GetConfig().Get().Services, componentManagerName,
		"the configuration names a component-manager")

	// createTestServiceManager binds port 0; ConfigureFromServices would default the main
	// port to 8080, a fixed port. No service is configured, so the registry's metrics
	// constructor is not called, as the pin's explicit metrics opt-out arranged.
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, RegisterAll(manager.registry))
	startTestManager(t, manager)

	built, exists := manager.GetService(componentManagerName)
	require.True(t, exists, "ComponentManager should be created as a mandatory service")
	require.IsType(t, (*ComponentManager)(nil), built, "the mandatory service is a ComponentManager")
}
