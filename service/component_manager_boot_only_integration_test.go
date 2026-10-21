//go:build integration

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/types"
)

// The boot set is composed once (pin component_manager_boot_only_integration_test.go; design
// D3, ADR-096 ¶4): component and model-registry writes a running config.Manager applies after
// NewComponentManager change neither the composed components nor what their factories received.

// TestComponentManagerPostBootComponentAndModelWritesDoNotChangeRuntimeComposition: a component
// manager built over a started config.Manager keeps the component instance and the configuration
// it was built from after a changed component, a new component and a model registry are written
// to the configuration bucket and the config.Manager has applied the model registry.
//
// spec: service-lifecycle / The component manager composes the boot set once
func TestComponentManagerPostBootComponentAndModelWritesDoNotChangeRuntimeComposition(t *testing.T) {
	client := fixtureClient(t)
	deps := createTestServiceDependencies()

	bootConfig := types.ComponentConfig{
		Name: "boot-test", Type: types.ComponentTypeProcessor, Enabled: true,
		Config: json.RawMessage(`{"revision":"boot"}`),
	}
	configManager, err := config.NewConfigManager(&config.Config{
		Version:    "1.0.0",
		Platform:   config.PlatformConfig{Org: "test", ID: "boot-only", Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: config.ComponentConfigs{"worker": bootConfig},
	}, client, deps.Logger)
	require.NoError(t, err)
	require.NoError(t, configManager.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, configManager.Stop(ctx))
	})

	registry := component.NewRegistry()
	created := &mockDiscoverableComponent{}
	var receivedModelRegistry model.RegistryReader
	require.NoError(t, registry.Register(component.Registration{
		Name: "boot-test", Type: types.ComponentTypeProcessor,
		Factory: func(_ json.RawMessage, componentDeps component.Dependencies) (component.Discoverable, error) {
			receivedModelRegistry = componentDeps.ModelRegistry
			return created, nil
		},
		Ports: func(json.RawMessage, string) (component.PortConfig, error) { return component.PortConfig{}, nil },
	}))

	deps.NATSClient = client
	deps.Manager = configManager
	deps.ComponentRegistry = registry
	deps.Platform = types.PlatformMeta{Org: "test", Platform: "boot-only"}
	built, err := NewComponentManager(json.RawMessage(`{}`), deps)
	require.NoError(t, err)
	manager, ok := built.(*ComponentManager)
	require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)

	assertRuntime := func(when string) {
		t.Helper()
		require.NoError(t, manager.withComponents(func(components map[string]*component.ManagedComponent) error {
			require.Len(t, components, 1, "%s: runtime membership = %v, want [worker]", when, components)
			managed, ok := components["worker"]
			require.True(t, ok, "%s: worker is not composed: %v", when, components)
			require.Same(t, created, managed.Component, "%s: the worker instance changed", when)
			require.JSONEq(t, string(bootConfig.Config), string(managed.Config.Config),
				"%s: the worker's configuration is not its boot configuration", when)
			return nil
		}))
		// The boot configuration holds no model registry, so the factory received none. Compared
		// with == because require.Nil also passes a nil *model.Registry inside the interface, which
		// a component would read as a registry (pin component_manager.go:173-174).
		require.True(t, receivedModelRegistry == nil,
			"%s: the factory received model registry %#v", when, receivedModelRegistry)
	}
	assertRuntime("after construction")

	require.NoError(t, configManager.PutComponentToKV(t.Context(), "worker", types.ComponentConfig{
		Name: "boot-test", Type: types.ComponentTypeProcessor, Enabled: true,
		Config: json.RawMessage(`{"revision":"next-boot"}`),
	}))
	require.NoError(t, configManager.PutComponentToKV(t.Context(), "late", bootConfig))
	assertRuntime("after the component writes")

	// Asked for before the write, so the write's update is not missed; the channel first carries
	// the current registry (none), and coalesces, so the wait reads until a registry arrives.
	modelUpdates := configManager.WatchModelRegistry()
	bucketName, err := config.BucketName("test", "boot-only")
	require.NoError(t, err)
	bucket, err := client.GetKeyValueBucket(t.Context(), bucketName)
	require.NoError(t, err)
	nextModel := &model.Registry{Endpoints: map[string]*model.EndpointConfig{
		"next": {URL: "http://next.invalid", Model: "next"},
	}}
	data, err := json.Marshal(nextModel)
	require.NoError(t, err)
	_, err = bucket.Put(t.Context(), "model_registry", data)
	require.NoError(t, err)
	bootOnlyWaitForModelRegistry(t, modelUpdates, "next")
	assertRuntime("after the model registry write")
}

// bootOnlyWaitForModelRegistry reads updates until one holds endpoint, failing the test once
// stopBound passes, the channel closes or the test's context ends.
func bootOnlyWaitForModelRegistry(t *testing.T, updates <-chan *model.Registry, endpoint string) {
	t.Helper()
	timer := time.NewTimer(stopBound)
	defer timer.Stop()
	for {
		select {
		case registry, ok := <-updates:
			require.True(t, ok, "the model registry channel closed before endpoint %q arrived", endpoint)
			if registry != nil && registry.Endpoints[endpoint] != nil {
				return
			}
		case <-timer.C:
			t.Fatalf("the config manager did not apply the model_registry write (endpoint %q)", endpoint)
		case <-t.Context().Done():
			t.Fatalf("test context ended waiting for model registry endpoint %q", endpoint)
		}
	}
}
