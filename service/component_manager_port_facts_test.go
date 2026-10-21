package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

// The composition projections, from the pin's component_manager_port_facts_test.go. The pin built
// a struct-literal manager and ran analyzeBootComposition by hand; here NewComponentManager admits
// the components and analyses the composition once, as at boot (design D3), and the tests read the
// result it keeps and the routes that project it, which compute nothing (design D5).

type portFactsDiscoverable struct {
	baseDiscoverable
	inputs  []component.Port
	outputs []component.Port
}

func (c portFactsDiscoverable) InputPorts() []component.Port  { return c.inputs }
func (c portFactsDiscoverable) OutputPorts() []component.Port { return c.outputs }

// projectionComponent is one component a projection test composes under a chosen component type:
// the instance name, which is also its factory's name, the type its factory registers and its
// configuration names, and the instance the factory returns. testComponent's helpers fix the type
// at processor, which cannot reach the origin rule that reads a node's declared type.
type projectionComponent struct {
	name          string
	componentType types.ComponentType
	instance      component.Discoverable
}

// registerProjectionFactory registers on registry a factory named c.name, of type
// c.componentType, whose constructor returns c.instance and whose port declaration is its ports.
func registerProjectionFactory(t *testing.T, registry *component.Registry, c projectionComponent) {
	t.Helper()
	require.NoError(t, registry.Register(component.Registration{
		Name: c.name, Type: c.componentType,
		Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
			return c.instance, nil
		},
		Ports: func(json.RawMessage, string) (component.PortConfig, error) {
			return component.PortConfigFrom(c.instance.InputPorts(), c.instance.OutputPorts()), nil
		},
	}))
}

// newProjectionComponentManager registers a factory for each of components on registry, enables
// each in the configuration under its type, and builds the manager through NewComponentManager.
// A factory the test registered on registry beforehand is admitted only if components names it.
func newProjectionComponentManager(
	t *testing.T, registry *component.Registry, components ...projectionComponent,
) *ComponentManager {
	t.Helper()
	configs := make(config.ComponentConfigs, len(components))
	for _, c := range components {
		registerProjectionFactory(t, registry, c)
		configs[c.name] = types.ComponentConfig{
			Name: c.name, Type: c.componentType, Enabled: true, Config: json.RawMessage(`{}`),
		}
	}
	deps := testComponentDependencies(t, registry)
	manager, err := config.NewConfigManager(&config.Config{
		Version:    "1.0.0",
		Platform:   config.PlatformConfig{Org: "acme", ID: "component-manager-test", Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: configs,
	}, deps.NATSClient, deps.Logger)
	require.NoError(t, err)
	deps.Manager = manager
	built, err := NewComponentManager(nil, deps)
	require.NoError(t, err)
	cm, ok := built.(*ComponentManager)
	require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)
	return cm
}

// A configuration with no model registry leaves the manager's boot model registry a nil
// interface, not an interface holding a nil *model.Registry, so components see none.
func TestComponentManagerAbsentBootModelRegistryRemainsNil(t *testing.T) {
	manager := newTestComponentManager(t)
	if manager.bootModelRegistry != nil {
		t.Fatalf("bootModelRegistry = %T, want nil", manager.bootModelRegistry)
	}
}

// TestComponentManagerPortReportingUsesCanonicalFactsWithoutDroppingKinds: every declared port
// appears, with the kind its port facts report, on the result construction keeps.
func TestComponentManagerPortReportingUsesCanonicalFactsWithoutDroppingKinds(t *testing.T) {
	instance := &portFactsDiscoverable{
		inputs: []component.Port{
			{Name: "events", Direction: component.DirectionInput, Config: component.JetStreamPort{StreamName: "EVENTS", Subjects: []string{"events.>", "audit.>"}}},
			{Name: "entities", Direction: component.DirectionInput, Config: component.KVReadPort{Bucket: "ENTITY_STATES"}},
		},
		outputs: []component.Port{
			{Name: "request", Direction: component.DirectionOutput, Config: component.NATSRequestPort{Subject: "svc.echo"}},
		},
	}
	manager := newTestComponentManager(t, testComponent{name: "facts", instance: instance})

	result := manager.bootCompositionResult()
	if result == nil || len(result.Graph.Nodes) != 1 {
		t.Fatalf("projection nodes = %+v, want exactly the one admitted instance", result)
	}
	node := result.Graph.Nodes[0]
	if node.Instance != "facts" {
		t.Fatalf("projected instance = %q, want the admitted name", node.Instance)
	}
	if len(node.Inputs) != 2 || len(node.Outputs) != 1 {
		t.Fatalf("projected ports = inputs:%d outputs:%d", len(node.Inputs), len(node.Outputs))
	}
	if got := node.Inputs[0]; got.Kind != "jetstream" || got.Name != "events" {
		t.Fatalf("JetStream input view = %+v", got)
	}
	if got := node.Inputs[1]; got.Kind != "kv-read" || got.Name != "entities" {
		t.Fatalf("KV read input view = %+v", got)
	}
	if got := node.Outputs[0]; got.Kind != "nats-request" || got.ConnectionID != "svc.echo" {
		t.Fatalf("request output view = %+v", got)
	}
}

// TestComponentManagerProjectionCarriesOnlyAdmittedInstances: the projection is derived from the
// declarations the registry admitted at construction, so a factory the configuration never names
// has no node, and an empty composition projects no node and no path.
func TestComponentManagerProjectionCarriesOnlyAdmittedInstances(t *testing.T) {
	t.Run("RegisteredButUnconfiguredFactory", func(t *testing.T) {
		registry := component.NewRegistry()
		registerProjectionFactory(t, registry, projectionComponent{
			name: "unconfigured", componentType: types.ComponentTypeProcessor, instance: &portFactsDiscoverable{},
		})
		manager := newProjectionComponentManager(t, registry, projectionComponent{
			name: "admitted", componentType: types.ComponentTypeProcessor, instance: &portFactsDiscoverable{},
		})
		result := manager.bootCompositionResult()
		require.NotNil(t, result, "boot composition result missing")
		instances := make([]string, 0, len(result.Graph.Nodes))
		for _, node := range result.Graph.Nodes {
			instances = append(instances, node.Instance)
		}
		require.Equal(t, []string{"admitted"}, instances,
			"projection nodes = %+v, want only the configured instance", result.Graph.Nodes)
	})
	t.Run("EmptyComposition", func(t *testing.T) {
		manager := newTestComponentManager(t)
		result := manager.bootCompositionResult()
		require.NotNil(t, result, "boot composition result missing")
		require.Empty(t, result.Graph.Nodes, "projection nodes for an empty composition")
		paths, err := manager.GetFlowPaths()
		require.NoError(t, err, "GetFlowPaths over an empty composition")
		require.Empty(t, paths, "paths for an empty composition")
	})
}

// TestComponentManagerFlowReportingUsesRetainedPortsAfterComponentMutation: after construction
// the live instance changes its port, and the result and the flowgraph and validate routes still
// answer with the declaration admitted at construction (ADR-100 P5): they project the kept
// result and compute nothing (design D5).
func TestComponentManagerFlowReportingUsesRetainedPortsAfterComponentMutation(t *testing.T) {
	instance := &portFactsDiscoverable{
		inputs: []component.Port{{
			Name: "events", Direction: component.DirectionInput,
			Config: component.NATSPort{Subject: "events.original"},
		}},
	}
	manager := newTestComponentManager(t, testComponent{name: "source", instance: instance})
	instance.inputs[0].Config = component.NATSPort{}

	result := manager.bootCompositionResult()
	if result == nil || len(result.Graph.Nodes) != 1 {
		t.Fatalf("projection = %+v, want the one retained declaration", result)
	}
	node := result.Graph.Nodes[0]
	if node.Instance != "source" || len(node.Inputs) != 1 || node.Inputs[0].ConnectionID != "events.original" {
		t.Fatalf("projected node = %#v, want retained events.original declaration", node)
	}

	retainedConnection := func(t *testing.T, graph composition.Graph) {
		t.Helper()
		require.Len(t, graph.Nodes, 1, "served graph = %+v", graph)
		require.Len(t, graph.Nodes[0].Inputs, 1, "served node = %+v", graph.Nodes[0])
		require.Equal(t, "events.original", graph.Nodes[0].Inputs[0].ConnectionID,
			"served node = %+v, want the declaration admitted at construction", graph.Nodes[0])
	}
	handlers := map[string]struct {
		handler http.HandlerFunc
		check   func(t *testing.T, body []byte)
	}{
		"graph": {manager.handleFlowGraph, func(t *testing.T, body []byte) {
			var graph composition.Graph
			require.NoError(t, json.Unmarshal(body, &graph), "flowgraph body %s", body)
			retainedConnection(t, graph)
		}},
		"validation": {manager.handleFlowValidation, func(t *testing.T, body []byte) {
			var served composition.Result
			require.NoError(t, json.Unmarshal(body, &served), "validate body %s", body)
			retainedConnection(t, served.Graph)
		}},
		"paths": {manager.handleFlowPaths, func(*testing.T, []byte) {}},
	}
	for name, route := range handlers {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/flow/"+name, nil)
			route.handler(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code, "HTTP status, body %s", recorder.Body.Bytes())
			route.check(t, recorder.Body.Bytes())
		})
	}
}
