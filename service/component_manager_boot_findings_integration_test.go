//go:build integration

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/types"
)

// The boot findings tests, from the pin's component_manager_boot_findings_integration_test.go.
// Each builds a component manager through NewComponentManager over a config.Manager started on
// the package's shared NATS server (fixtureClient, #195), so the boot analysis runs at the real
// boundary: the configuration the config.Manager settled on, its explicit streams included.
// Construction analyses the composition and keeps the result (design D3); the HTTP projections
// serve that result and compute nothing (design D5).

// bootFindingsComponent is a fake whose constructed ports are exactly the resolution of its
// declarer, so the registry's declared-versus-constructed check holds and the boot-time findings
// come from the composition alone.
type bootFindingsComponent struct {
	baseDiscoverable
	inputs  []component.Port
	outputs []component.Port
}

func (d *bootFindingsComponent) InputPorts() []component.Port  { return d.inputs }
func (d *bootFindingsComponent) OutputPorts() []component.Port { return d.outputs }

// registerBootFindingsFactory registers on registry a processor factory named name that declares
// ports and builds a bootFindingsComponent with those ports resolved. It returns the count of the
// factory's calls, which a test reads to see whether admission reached the factory.
func registerBootFindingsFactory(
	t *testing.T, registry *component.Registry, name string, ports component.PortConfig,
) *atomic.Int32 {
	t.Helper()
	inputs := make([]component.Port, 0, len(ports.Inputs))
	for _, definition := range ports.Inputs {
		port, err := definition.Resolve(component.DirectionInput)
		require.NoError(t, err)
		inputs = append(inputs, port)
	}
	outputs := make([]component.Port, 0, len(ports.Outputs))
	for _, definition := range ports.Outputs {
		port, err := definition.Resolve(component.DirectionOutput)
		require.NoError(t, err)
		outputs = append(outputs, port)
	}
	calls := new(atomic.Int32)
	require.NoError(t, registry.Register(component.Registration{
		Name: name, Type: types.ComponentTypeProcessor,
		Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
			calls.Add(1)
			return &bootFindingsComponent{inputs: inputs, outputs: outputs}, nil
		},
		Ports: func(json.RawMessage, string) (component.PortConfig, error) { return ports, nil },
	}))
	return calls
}

// bootFindingsConfigManager returns a config.Manager started on client, holding components and
// streams under the platform org "acme" and the given platform ID. Each configuration a test
// boots gets its own ID: two managers with one ID share one configuration bucket, and the second
// would take the first one's components from it. The manager is stopped when the test ends,
// before fixtureClient closes client.
func bootFindingsConfigManager(
	t *testing.T, client *natsclient.Client, platformID string,
	components config.ComponentConfigs, streams config.StreamConfigs,
) *config.Manager {
	t.Helper()
	manager, err := config.NewConfigManager(&config.Config{
		Version:    "1.0.0",
		Platform:   config.PlatformConfig{Org: "acme", ID: platformID, Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: components,
		Streams:    streams,
	}, client, createTestServiceDependencies().Logger)
	require.NoError(t, err)
	require.NoError(t, manager.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, manager.Stop(ctx))
	})
	return manager
}

// newBootFindingsManager builds a component manager through NewComponentManager over registry and
// a started config.Manager holding components and streams, and returns what the constructor
// returned.
func newBootFindingsManager(
	t *testing.T, client *natsclient.Client, registry *component.Registry, platformID string,
	components config.ComponentConfigs, streams config.StreamConfigs,
) (Service, error) {
	t.Helper()
	deps := createTestServiceDependencies()
	deps.NATSClient = client
	deps.Manager = bootFindingsConfigManager(t, client, platformID, components, streams)
	deps.ComponentRegistry = registry
	deps.Platform = types.PlatformMeta{Org: "acme", Platform: platformID}
	return NewComponentManager(json.RawMessage(`{}`), deps)
}

// bootFindingsComponentManager is newBootFindingsManager for a composition that must boot.
func bootFindingsComponentManager(
	t *testing.T, client *natsclient.Client, registry *component.Registry, platformID string,
	components config.ComponentConfigs, streams config.StreamConfigs,
) *ComponentManager {
	t.Helper()
	built, err := newBootFindingsManager(t, client, registry, platformID, components, streams)
	require.NoError(t, err, "boot")
	manager, ok := built.(*ComponentManager)
	require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)
	return manager
}

// bootFindingsProcessor is the enabled processor instance built by factory, configured `{}`.
func bootFindingsProcessor(factory string) types.ComponentConfig {
	return types.ComponentConfig{
		Name: factory, Type: types.ComponentTypeProcessor, Enabled: true, Config: json.RawMessage(`{}`),
	}
}

// corePublisherPorts and streamConsumerPorts are the pin's mismatch: a JetStream input on
// boot.data whose only publisher is a core-NATS output on the same subject.
func corePublisherPorts() component.PortConfig {
	return component.PortConfig{Outputs: []component.PortDefinition{
		{Name: "out", Required: true, Config: component.NATSPort{Subject: "boot.data"}},
	}}
}

func streamConsumerPorts() component.PortConfig {
	return component.PortConfig{Inputs: []component.PortDefinition{
		{Name: "in", Required: true, Config: component.JetStreamPort{StreamName: "BOOT", Subjects: []string{"boot.data"}}},
	}}
}

// TestComponentManagerRefusesBootOnErrorFinding: a JetStream input fed only by a core-NATS output
// is a stream_requirement error; NewComponentManager refuses, naming the finding and the
// component, returns no manager, and leaves the registry unsealed (design D3: refuse, then seal).
//
// spec: service-lifecycle / The component manager composes the boot set once
func TestComponentManagerRefusesBootOnErrorFinding(t *testing.T) {
	client := fixtureClient(t)
	registry := component.NewRegistry()
	registerBootFindingsFactory(t, registry, "core-publisher", corePublisherPorts())
	registerBootFindingsFactory(t, registry, "stream-consumer", streamConsumerPorts())

	built, err := newBootFindingsManager(t, client, registry, "boot-refused", config.ComponentConfigs{
		"pub": bootFindingsProcessor("core-publisher"),
		"sub": bootFindingsProcessor("stream-consumer"),
	}, nil)
	require.Error(t, err, "NewComponentManager booted a composition with a stream_requirement error")
	require.Nil(t, built, "a refused boot returned a manager")
	for _, want := range []string{composition.TypeStreamRequirement, "sub"} {
		require.Contains(t, err.Error(), want, "the boot error does not name %q", want)
	}

	// Not sealed: the pin created a late instance on the registry directly; here a second
	// composition over the same registry reaches the late factory, which a sealed registry
	// refuses before any factory runs (component.Registry.CreateComponent). Whether the second
	// boot is refused is not asserted: the first boot's admissions may stay in the registry.
	lateCalls := registerBootFindingsFactory(t, registry, "late", component.PortConfig{})
	_, _ = newBootFindingsManager(t, client, registry, "boot-refused-late", config.ComponentConfigs{
		"late-instance": bootFindingsProcessor("late"),
	}, nil)
	require.Equal(t, int32(1), lateCalls.Load(), "the registry was sealed after a refused boot")
}

// TestComponentManagerExposesBootFindings: a booted composition with one disconnected_node
// warning serves the result construction kept, verbatim, at <components>/validate.
//
// spec: operator-surface / The composition projections serve the result kept at construction
func TestComponentManagerExposesBootFindings(t *testing.T) {
	client := fixtureClient(t)
	registry := component.NewRegistry()
	registerBootFindingsFactory(t, registry, "lonely", component.PortConfig{Inputs: []component.PortDefinition{
		{Name: "in", Required: false, Config: component.NATSPort{Subject: "nobody.publishes"}},
	}})
	manager := bootFindingsComponentManager(t, client, registry, "boot-findings", config.ComponentConfigs{
		"alone": bootFindingsProcessor("lonely"),
	}, nil)

	retained := manager.bootCompositionResult()
	require.NotNil(t, retained, "ComponentManager retained no boot findings")
	require.Equal(t, composition.StatusWarnings, retained.Status)
	require.Contains(t, bootFindingTypes(retained.Warnings), composition.TypeDisconnectedNode)

	mux := http.NewServeMux()
	manager.RegisterHTTPHandlers("/components/", mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/components/validate", nil))
	require.Equal(t, http.StatusOK, recorder.Code, "GET /components/validate: %s", recorder.Body.String())
	var served composition.Result
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &served), recorder.Body.String())
	if !reflect.DeepEqual(bootFindingsNormalize(t, served), bootFindingsNormalize(t, *retained)) {
		t.Fatalf("served result differs from the retained boot result:\nserved=%s\nretained=%s",
			recorder.Body.String(), bootFindingsJSON(t, retained))
	}
}

// TestGraphProjectionMatchesAdmittedComposition: <components>/flowgraph names every admitted
// instance with its factory and resolved ports and every derived edge, as JSON and as Mermaid.
// The expected nodes are the test's own declarations; the pin read them back from the registry's
// Snapshots, which only the component manager calls (AGENTS.md section 1).
//
// spec: operator-surface / The composition projections serve the result kept at construction
func TestGraphProjectionMatchesAdmittedComposition(t *testing.T) {
	client := fixtureClient(t)
	registry := component.NewRegistry()
	registerBootFindingsFactory(t, registry, "js-publisher", component.PortConfig{Outputs: []component.PortDefinition{
		{Name: "out", Required: true, Config: component.JetStreamPort{StreamName: "PROJ", Subjects: []string{"proj.data"}}},
	}})
	registerBootFindingsFactory(t, registry, "js-consumer", component.PortConfig{Inputs: []component.PortDefinition{
		{Name: "in", Required: true, Config: component.JetStreamPort{StreamName: "PROJ", Subjects: []string{"proj.data"}}},
	}})
	manager := bootFindingsComponentManager(t, client, registry, "boot-projection", config.ComponentConfigs{
		"pub": bootFindingsProcessor("js-publisher"),
		"sub": bootFindingsProcessor("js-consumer"),
	}, nil)

	mux := http.NewServeMux()
	manager.RegisterHTTPHandlers("/components/", mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/components/flowgraph", nil))
	require.Equal(t, http.StatusOK, recorder.Code, "GET /components/flowgraph: %s", recorder.Body.String())
	var graph composition.Graph
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &graph), recorder.Body.String())

	type nodeShape struct {
		factory         string
		inputs, outputs int
	}
	want := map[string]nodeShape{
		"pub": {factory: "js-publisher", inputs: 0, outputs: 1},
		"sub": {factory: "js-consumer", inputs: 1, outputs: 0},
	}
	got := make(map[string]nodeShape, len(graph.Nodes))
	for _, node := range graph.Nodes {
		got[node.Instance] = nodeShape{factory: node.Factory, inputs: len(node.Inputs), outputs: len(node.Outputs)}
	}
	require.Equal(t, want, got, "flowgraph nodes")
	require.Len(t, graph.Edges, 1, "edges = %+v, want one pub to sub edge", graph.Edges)
	require.Equal(t, "pub", graph.Edges[0].From)
	require.Equal(t, "sub", graph.Edges[0].To)

	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/components/flowgraph?format=mermaid", nil))
	require.Equal(t, http.StatusOK, recorder.Code,
		"GET /components/flowgraph?format=mermaid: %s", recorder.Body.String())
	rendered := recorder.Body.String()
	require.Equal(t, composition.Mermaid(graph), rendered, "Mermaid projection differs from Mermaid(graph)")
	for _, instance := range []string{"pub", "sub"} {
		require.Contains(t, rendered, instance, "Mermaid lacks %s", instance)
	}
	require.Equal(t, 1, strings.Count(rendered, "-->"), "Mermaid edges:\n%s", rendered)
}

// TestComponentManagerBootFindingsHonourExplicitStreams: at the real boundary too, a JetStream
// input fed by a core-NATS output is not a stream_requirement finding when the configuration's
// explicit streams cover its subjects: the boot analysis receives the configuration's streams,
// not only its components (design D3).
//
// spec: service-lifecycle / The component manager composes the boot set once
func TestComponentManagerBootFindingsHonourExplicitStreams(t *testing.T) {
	client := fixtureClient(t)
	registry := component.NewRegistry()
	registerBootFindingsFactory(t, registry, "core-publisher", corePublisherPorts())
	registerBootFindingsFactory(t, registry, "stream-consumer", streamConsumerPorts())
	manager := bootFindingsComponentManager(t, client, registry, "boot-streams", config.ComponentConfigs{
		"pub": bootFindingsProcessor("core-publisher"),
		"sub": bootFindingsProcessor("stream-consumer"),
	}, config.StreamConfigs{"BOOT": {
		Subjects: []string{"boot.>"},
		// Bounds declared because config.Manager.Start validates the effective configuration
		// when it applies the established platform identity (ADR-104); the finding under test
		// is stream_requirement, not bounds.
		MaxAge: "24h", MaxBytes: 1 << 20, Discard: "old",
	}})

	retained := manager.bootCompositionResult()
	require.NotNil(t, retained, "ComponentManager retained no boot findings")
	for _, finding := range retained.Errors {
		require.NotEqual(t, composition.TypeStreamRequirement, finding.Type,
			"boot analysis reported stream_requirement although streams.BOOT covers boot.data: %+v", finding)
	}
	require.Len(t, retained.Graph.Edges, 1, "edges = %+v, want the pub to sub edge", retained.Graph.Edges)
}

// bootFindingTypes returns each finding's type, in order.
func bootFindingTypes(findings []composition.Finding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.Type)
	}
	return out
}

// bootFindingsNormalize returns result as its JSON object decodes, so a served answer and the
// retained value compare field by field whatever nil-versus-empty choices the encoder made.
func bootFindingsNormalize(t *testing.T, result composition.Result) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(bootFindingsJSON(t, result)), &out))
	return out
}

// bootFindingsJSON returns value encoded as JSON.
func bootFindingsJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
