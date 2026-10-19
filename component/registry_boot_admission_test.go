package component

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
)

func TestRegistryCapturesPortsOnceAndReturnsDefensiveDeclarationSnapshots(t *testing.T) {
	registry := NewRegistry()
	created := &declarationTestComponent{outputs: []Port{
		declarationTestPort("events.created"),
		{
			Name: "listener", Direction: DirectionOutput, Required: true,
			Config: NetworkPort{Protocol: "tcp", Host: "127.0.0.1", Port: 8080},
		},
	}}
	requireNoError(t, registry.Register(Registration{
		Name: "declaration-test", Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) { return created, nil },
		Ports:   created.declarePorts,
	}))

	got, err := registry.CreateComponent(
		"worker", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)
	if got != created {
		t.Fatal("CreateComponent returned a different component")
	}
	if created.inputCalls != 1 || created.outputCalls != 1 {
		t.Fatalf("port capture calls = input:%d output:%d, want exactly one each", created.inputCalls, created.outputCalls)
	}

	all := registry.Snapshots()
	if len(all) != 1 {
		t.Fatalf("Snapshots length = %d, want 1", len(all))
	}
	assertWorkerDeclarationUnchanged(t, all[0])
	mutateDeclarationSnapshot(all[0])

	rereadAll := registry.Snapshots()
	if len(rereadAll) != 1 {
		t.Fatalf("second Snapshots length = %d, want 1", len(rereadAll))
	}
	assertWorkerDeclarationUnchanged(t, rereadAll[0])
	if created.inputCalls != 1 || created.outputCalls != 1 {
		t.Fatalf("port capture calls after defensive reads = input:%d output:%d, want exactly one each",
			created.inputCalls, created.outputCalls)
	}
}

func mutateDeclarationSnapshot(snapshot Declaration) {
	jetstream := snapshot.OutputPorts[0].Config.(JetStreamPort)
	jetstream.Subjects[0] = "events.mutated"
	snapshot.OutputFacts[0].natsSubjects[0] = "facts.mutated"
	snapshot.OutputFacts[0].stream.subjects[0] = "stream-facts.mutated"
	snapshot.ExclusiveResources[0] = "tcp:mutated:1"
}

func assertWorkerDeclarationUnchanged(t *testing.T, snapshot Declaration) {
	t.Helper()
	if snapshot.InstanceName != "worker" || snapshot.FactoryIdentity != "declaration-test" {
		t.Fatalf("snapshot identity = %q/%q, want worker/declaration-test",
			snapshot.InstanceName, snapshot.FactoryIdentity)
	}
	outputs := snapshot.OutputPorts
	if len(outputs) != 2 {
		t.Fatalf("output ports = %#v, want two retained declarations", outputs)
	}
	jetstream := outputs[0].Config.(JetStreamPort)
	if !reflect.DeepEqual(jetstream.Subjects, []string{"events.created"}) {
		t.Fatalf("port subjects = %v, want events.created", jetstream.Subjects)
	}
	facts := snapshot.OutputFacts
	if got := facts[0].NATSSubjects(); !reflect.DeepEqual(got, []string{"events.created"}) {
		t.Fatalf("fact subjects = %v, want events.created", got)
	}
	stream, ok := facts[0].Stream()
	if !ok || !reflect.DeepEqual(stream.Subjects(), []string{"events.created"}) {
		t.Fatalf("stream fact subjects = %v, present=%v, want events.created", stream.Subjects(), ok)
	}
	if !reflect.DeepEqual(snapshot.ExclusiveResources, []string{"tcp:127.0.0.1:8080"}) {
		t.Fatalf("exclusive resources = %v, want tcp:127.0.0.1:8080", snapshot.ExclusiveResources)
	}
}

func TestRegistryHasNoLiveHandleOrReplacementSurface(t *testing.T) {
	registryType := reflect.TypeOf((*Registry)(nil))
	for _, retiredMethod := range []string{
		"Component", "GetComponent", "ListComponents", "ReplaceComponent",
		"RemoveComponent", "UnregisterInstance", "ReserveReplacement",
	} {
		if _, ok := registryType.MethodByName(retiredMethod); ok {
			t.Errorf("Registry retains retired live-handle method %q", retiredMethod)
		}
	}
}

func TestRegistryManagedPreparationFailurePublishesNothing(t *testing.T) {
	registry := NewRegistry()
	requireNoError(t, registry.Register(Registration{
		Name: "declaration-test", Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) {
			return &declarationTestComponent{}, nil
		},
		Ports: noPorts,
	}))
	want := errors.New("initialize failed")
	_, err := registry.CreateComponent(
		"worker", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(),
		func(Discoverable) error { return want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("CreateComponent error = %v, want %v", err, want)
	}
	if _, ok := admittedDeclaration(registry, "worker"); ok {
		t.Fatal("failed preparation published a declaration")
	}
}

func TestRegistrySnapshotReportsCompleteBootSetAndSealRejectsLaterAdmission(t *testing.T) {
	registry := NewRegistry()
	factoryCalls := 0
	requireNoError(t, registry.Register(Registration{
		Name: "declaration-test", Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) {
			factoryCalls++
			return &declarationTestComponent{}, nil
		},
		Ports: noPorts,
	}))
	_, err := registry.CreateComponent(
		"worker-a", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil,
	)
	requireNoError(t, err)
	_, err = registry.CreateComponent(
		"worker-b", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil,
	)
	requireNoError(t, err)
	admitted := registry.Snapshots()
	if len(admitted) != 2 || admitted[0].InstanceName != "worker-a" || admitted[1].InstanceName != "worker-b" {
		t.Fatalf("boot declaration snapshot = %#v, want complete worker-a/worker-b set", admitted)
	}

	registry.SealComposition()
	_, err = registry.CreateComponent(
		"late", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil,
	)
	if err == nil {
		t.Fatal("post-seal component admission succeeded")
	}
	if factoryCalls != 2 {
		t.Fatalf("factory calls after rejected post-seal admission = %d, want 2", factoryCalls)
	}
	if snapshots := registry.Snapshots(); len(snapshots) != 2 {
		t.Fatalf("post-seal snapshots = %#v, want unchanged two-declaration set", snapshots)
	}
}

// registerPortsFactory registers a factory whose component reports exactly
// these ports, with a declarer that declares the same ones.
func registerPortsFactory(t *testing.T, registry *Registry, factory string, inputs, outputs []Port) {
	t.Helper()
	built := &declarationTestComponent{inputs: inputs, outputs: outputs}
	requireNoError(t, registry.Register(Registration{
		Name: factory, Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) { return built, nil },
		Ports:   built.declarePorts,
	}))
}

func networkTestPort(name string, direction Direction, port int) Port {
	return Port{
		Name: name, Direction: direction, Required: true,
		Config: NetworkPort{Protocol: "tcp", Host: "127.0.0.1", Port: port},
	}
}

func admittedNames(registry *Registry) []string {
	names := []string{}
	for _, declaration := range registry.Snapshots() {
		names = append(names, declaration.InstanceName)
	}
	return names
}

func TestRegistryRefusesASecondComponentOnATakenAddress(t *testing.T) {
	registry := NewRegistry()
	registerPortsFactory(t, registry, "listener", nil, []Port{networkTestPort("listen", DirectionOutput, 8080)})
	registerPortsFactory(t, registry, "acceptor", []Port{networkTestPort("accept", DirectionInput, 8080)}, nil)
	_, err := registry.CreateComponent("first", declarationTestConfig("listener", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)

	_, err = registry.CreateComponent("second", declarationTestConfig("acceptor", `{}`), declarationTestDeps(), nil)

	if err == nil || !errs.IsInvalid(err) ||
		!strings.Contains(err.Error(), "resource conflict: tcp:127.0.0.1:8080 already used by component 'first'") {
		t.Fatalf("second component on a taken address: error = %v, "+
			"want an invalid-class refusal naming tcp:127.0.0.1:8080 and its holder first", err)
	}
	if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("admitted after the refusal = %v, want [first]", got)
	}
}

// TestRegistryAdmitsComponentsOnDistinctOrSharedResources is the sensitivity
// pair of the refusal above: what is not one exclusive address is admitted.
func TestRegistryAdmitsComponentsOnDistinctOrSharedResources(t *testing.T) {
	tests := []struct {
		name          string
		first, second Port
	}{
		{
			name:   "two different addresses",
			first:  networkTestPort("listen", DirectionOutput, 8080),
			second: networkTestPort("listen", DirectionOutput, 8081),
		},
		{
			name:   "two file ports on one path",
			first:  Port{Name: "file", Direction: DirectionOutput, Config: FilePort{Path: "/shared/events.jsonl"}},
			second: Port{Name: "file", Direction: DirectionOutput, Config: FilePort{Path: "/shared/events.jsonl"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			registerPortsFactory(t, registry, "first-factory", nil, []Port{tt.first})
			registerPortsFactory(t, registry, "second-factory", nil, []Port{tt.second})
			_, err := registry.CreateComponent(
				"first", declarationTestConfig("first-factory", `{}`), declarationTestDeps(), nil)
			requireNoError(t, err)
			_, err = registry.CreateComponent(
				"second", declarationTestConfig("second-factory", `{}`), declarationTestDeps(), nil)
			requireNoError(t, err)
			if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"first", "second"}) {
				t.Fatalf("admitted = %v, want [first second]", got)
			}
		})
	}
}
