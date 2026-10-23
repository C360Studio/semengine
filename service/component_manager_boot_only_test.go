package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
)

// Construction's refusals and the empty component-manager configuration (pin
// component_manager_boot_only_test.go; design D1, D3). ComponentManagerConfig defines no key, so
// a strict decode refuses every key, naming it; the pin's TestNewComponentManagerRejectsRetiredWatchConfig
// is one of its cases.

// TestComponentManagerConfigRefusesEveryKey: an empty ComponentManagerConfig decoded strictly
// refuses any key, the retired enabled_components and watch_config among them, and names it.
func TestComponentManagerConfigRefusesEveryKey(t *testing.T) {
	for key, raw := range map[string]string{
		"enabled_components": `{"enabled_components":["graph-ingest"]}`,
		"watch_config":       `{"watch_config":true}`,
		"anything":           `{"anything":1}`,
	} {
		t.Run(key, func(t *testing.T) {
			deps := testComponentDependencies(t, component.NewRegistry())
			built, err := NewComponentManager(json.RawMessage(raw), deps)
			require.Error(t, err)
			require.Nil(t, built)
			require.ErrorContains(t, err, "parse component-manager config")
			require.ErrorContains(t, err, `unknown field "`+key+`"`)
		})
	}
}

// TestNewComponentManagerAcceptsTheEmptyConfiguration: no configuration and an empty object both
// build a manager.
func TestNewComponentManagerAcceptsTheEmptyConfiguration(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"absent":       nil,
		"empty object": json.RawMessage(`{}`),
	} {
		t.Run(name, func(t *testing.T) {
			built, err := NewComponentManager(raw, testComponentDependencies(t, component.NewRegistry()))
			require.NoError(t, err)
			_, ok := built.(*ComponentManager)
			require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)
		})
	}
}

// TestNewComponentManagerRefusesANilConfigManager: with no dependencies, or dependencies without a
// config.Manager, there is no configuration to compose and construction refuses (pin :155-157).
func TestNewComponentManagerRefusesANilConfigManager(t *testing.T) {
	built, err := NewComponentManager(nil, nil)
	require.ErrorContains(t, err, "component-manager requires config manager")
	require.Nil(t, built)

	deps := testComponentDependencies(t, component.NewRegistry())
	deps.Manager = nil
	built, err = NewComponentManager(nil, deps)
	require.ErrorContains(t, err, "component-manager requires config manager")
	require.Nil(t, built)
}

// TestNewComponentManagerRefusesANilComponentRegistry: sweep C2. The pin fell back to a private
// empty registry, where no configured component found its factory and boot went on without them;
// here construction refuses.
func TestNewComponentManagerRefusesANilComponentRegistry(t *testing.T) {
	deps := testComponentDependencies(t, component.NewRegistry())
	deps.ComponentRegistry = nil
	built, err := NewComponentManager(nil, deps)
	require.ErrorContains(t, err, "component-manager requires a component registry")
	require.Nil(t, built)
}

// TestComponentManagerHasNoPostBootConfigOrMutationSeam: the boot set is composed once (ADR-096
// ¶4), so the manager keeps no post-boot configuration or mutation seam, and the surface design D1
// drops for want of a reader is gone with it.
func TestComponentManagerHasNoPostBootConfigOrMutationSeam(t *testing.T) {
	managerType := reflect.TypeOf(ComponentManager{})
	for _, retiredField := range []string{
		"configManager", "configUpdates", "modelRegistryUpdates", "supervisorRequests",
	} {
		if _, ok := managerType.FieldByName(retiredField); ok {
			t.Errorf("ComponentManager retains retired post-boot field %q", retiredField)
		}
	}

	pointerType := reflect.TypeOf((*ComponentManager)(nil))
	for _, retiredMethod := range []string{
		"CreateComponent", "RemoveComponent", "CreateComponentsFromConfig", "GetManagedComponents",
		// Design D1: dropped, no reader; Initialize became the constructor's unexported initialize.
		"Initialize", "IsInitialized", "IsStarted", "GetHealthyComponents", "GetUnhealthyComponents",
	} {
		if _, ok := pointerType.MethodByName(retiredMethod); ok {
			t.Errorf("ComponentManager retains retired method %q", retiredMethod)
		}
	}
}
