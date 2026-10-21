package service

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

// The composition root's walk over the admitted components (pin components_implementing_test.go).
// The pin built a struct-literal manager and called its exported Initialize; here
// NewComponentManager composes the components from the configuration (design D7), and the test
// reads what ComponentsImplementing returns through the service manager (design D6).

type seamProbe interface{ seamName() string }

type seamComponent struct {
	baseDiscoverable
	instance string
}

func (c *seamComponent) seamName() string          { return c.instance }
func (*seamComponent) Initialize() error           { return nil }
func (*seamComponent) Start(context.Context) error { return nil }
func (*seamComponent) Stop(context.Context) error  { return nil }

type plainComponent struct{ baseDiscoverable }

func (*plainComponent) Initialize() error           { return nil }
func (*plainComponent) Start(context.Context) error { return nil }
func (*plainComponent) Stop(context.Context) error  { return nil }

// implementingTestDependencies returns the Dependencies testComponentDependencies builds, with a
// config.Manager holding components in place of its one-empty-configuration-per-name set, so a
// test can enable, disable and configure instances of one factory.
func implementingTestDependencies(
	t *testing.T, registry *component.Registry, components config.ComponentConfigs,
) *Dependencies {
	t.Helper()
	deps := testComponentDependencies(t, registry)
	manager, err := config.NewConfigManager(&config.Config{
		Version:    "1.0.0",
		Platform:   config.PlatformConfig{Org: "acme", ID: "component-manager-test", Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: components,
	}, deps.NATSClient, deps.Logger)
	require.NoError(t, err)
	deps.Manager = manager
	return deps
}

// The composition root finds its rule hot-reload targets through this walk;
// it must return exactly the enabled components the ComponentManager built
// that implement the seam, and nothing when there is no component manager.
func TestComponentsImplementingReturnsTheBuiltComponentsThatImplementTheSeam(t *testing.T) {
	t.Parallel()
	registry := component.NewRegistry()
	noPorts := func(json.RawMessage, string) (component.PortConfig, error) { return component.PortConfig{}, nil }
	require.NoError(t, registry.Register(component.Registration{
		Name: "seam", Type: types.ComponentTypeProcessor, Ports: noPorts,
		Factory: func(raw json.RawMessage, _ component.Dependencies) (component.Discoverable, error) {
			var cfg struct {
				Instance string `json:"instance"`
			}
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return nil, err
			}
			return &seamComponent{instance: cfg.Instance}, nil
		},
	}))
	require.NoError(t, registry.Register(component.Registration{
		Name: "plain", Type: types.ComponentTypeProcessor, Ports: noPorts,
		Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
			return &plainComponent{}, nil
		},
	}))
	deps := implementingTestDependencies(t, registry, config.ComponentConfigs{
		"a": {Type: types.ComponentTypeProcessor, Name: "seam", Enabled: true, Config: json.RawMessage(`{"instance":"a"}`)},
		"b": {Type: types.ComponentTypeProcessor, Name: "seam", Enabled: true, Config: json.RawMessage(`{"instance":"b"}`)},
		"off": {Type: types.ComponentTypeProcessor, Name: "seam", Enabled: false,
			Config: json.RawMessage(`{"instance":"off"}`)},
		"p": {Type: types.ComponentTypeProcessor, Name: "plain", Enabled: true, Config: json.RawMessage(`{}`)},
	})
	cm, err := NewComponentManager(nil, deps)
	require.NoError(t, err)

	manager := NewServiceManager(NewServiceRegistry())
	require.Empty(t, ComponentsImplementing[seamProbe](manager), "no component manager registered yet")
	require.NoError(t, manager.RegisterInstance(componentManagerName, cm))

	found := ComponentsImplementing[seamProbe](manager)
	names := make([]string, 0, len(found))
	for _, probe := range found {
		names = append(names, probe.seamName())
	}
	sort.Strings(names)
	require.Equal(t, []string{"a", "b"}, names)
	require.Len(t, ComponentsImplementing[component.Discoverable](manager), 3)
	require.Nil(t, ComponentsImplementing[seamProbe](nil))
}
