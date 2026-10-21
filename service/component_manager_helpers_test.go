package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

// The component manager's shared test helpers, from the pin's
// component_manager_test_helpers_test.go and component_manager_lifecycle_test.go:1-20. The pin
// built managers as struct literals and set their lifecycle fields by hand; here every manager is
// built through NewComponentManager over a test component.Registry and a config.Manager holding
// the test components (design D7), so a test sees only what the constructor composes and asserts
// outcomes, never the manager's lifecycle state (design D2).

// baseDiscoverable is a component with no ports that reports itself healthy. Test components
// embed it for the component.Discoverable methods they do not care about.
type baseDiscoverable struct{}

func (baseDiscoverable) InputPorts() []component.Port  { return nil }
func (baseDiscoverable) OutputPorts() []component.Port { return nil }
func (baseDiscoverable) Health() component.HealthStatus {
	return component.HealthStatus{Healthy: true}
}
func (baseDiscoverable) DataFlow() component.FlowMetrics { return component.FlowMetrics{} }

// mockDiscoverableComponent is a component with the ports a test gives it.
type mockDiscoverableComponent struct {
	inputPorts  []component.Port
	outputPorts []component.Port
}

func (m *mockDiscoverableComponent) InputPorts() []component.Port  { return m.inputPorts }
func (m *mockDiscoverableComponent) OutputPorts() []component.Port { return m.outputPorts }
func (m *mockDiscoverableComponent) Health() component.HealthStatus {
	return component.HealthStatus{Healthy: true}
}
func (m *mockDiscoverableComponent) DataFlow() component.FlowMetrics { return component.FlowMetrics{} }

// admitTestRegistryComponent registers on registry a processor factory named name whose
// constructor returns instance and whose port declaration is instance's ports. The pin's helper
// also called registry.CreateComponent itself; here the component manager creates the component,
// from the configuration testComponentDependencies builds, as it does at boot.
func admitTestRegistryComponent(
	t *testing.T, registry *component.Registry, name string, instance component.Discoverable,
) {
	t.Helper()
	require.NoError(t, registry.Register(component.Registration{
		Name: name, Type: types.ComponentTypeProcessor,
		Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
			return instance, nil
		},
		Ports: func(json.RawMessage, string) (component.PortConfig, error) {
			return component.PortConfigFrom(instance.InputPorts(), instance.OutputPorts()), nil
		},
	}))
}

// testComponent is one component a test composes: the instance name, which is also the name of
// the factory admitTestRegistryComponent registers for it, and the instance that factory returns.
type testComponent struct {
	name     string
	instance component.Discoverable
}

// testComponentDependencies returns the Dependencies the service manager would hand the component
// manager: registry, and a config.Manager whose configuration enables one component per name,
// built by the factory of that name with an empty configuration. The NATS client is not connected
// (newUnconnectedClient): the config.Manager needs one, and nothing here reaches a broker.
func testComponentDependencies(t *testing.T, registry *component.Registry, names ...string) *Dependencies {
	t.Helper()
	components := make(config.ComponentConfigs, len(names))
	for _, name := range names {
		components[name] = types.ComponentConfig{
			Name: name, Type: types.ComponentTypeProcessor, Enabled: true, Config: json.RawMessage(`{}`),
		}
	}
	deps := createTestServiceDependencies()
	deps.NATSClient = newUnconnectedClient(t)
	manager, err := config.NewConfigManager(&config.Config{
		Version:    "1.0.0",
		Platform:   config.PlatformConfig{Org: "acme", ID: "component-manager-test", Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: components,
	}, deps.NATSClient, deps.Logger)
	require.NoError(t, err)
	deps.Manager = manager
	deps.ComponentRegistry = registry
	return deps
}

// newTestComponentManager builds a component manager through NewComponentManager, with no
// component-manager configuration, composing components in a fresh registry.
func newTestComponentManager(t *testing.T, components ...testComponent) *ComponentManager {
	t.Helper()
	registry := component.NewRegistry()
	names := make([]string, 0, len(components))
	for _, c := range components {
		admitTestRegistryComponent(t, registry, c.name, c.instance)
		names = append(names, c.name)
	}
	built, err := NewComponentManager(nil, testComponentDependencies(t, registry, names...))
	require.NoError(t, err)
	cm, ok := built.(*ComponentManager)
	require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)
	return cm
}

// startTestComponentManager starts cm and stops it when the test ends. It replaces the pin's
// startPostBootComponentManager, which marked a struct-literal manager initialized by hand; a
// manager NewComponentManager returns is composed already.
func startTestComponentManager(t *testing.T, cm *ComponentManager) {
	t.Helper()
	require.NoError(t, cm.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, cm.Stop(ctx))
	})
}

// waitForSignal waits for signal to close or receive, failing the test, naming description, once
// stopBound passes or the test's context ends.
func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	timer := time.NewTimer(stopBound)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", description)
	case <-t.Context().Done():
		t.Fatalf("test context ended waiting for %s", description)
	}
}
