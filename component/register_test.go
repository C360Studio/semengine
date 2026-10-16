package component

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

func registerTestFactory(json.RawMessage, Dependencies) (Discoverable, error) {
	return &declarationTestComponent{}, nil
}

func requireInvalidNaming(t *testing.T, err error, name string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Register(%q) = nil, want an invalid-class error", name)
	}
	if !errs.IsInvalid(err) {
		t.Fatalf("Register(%q) = %v, want an invalid-class error", name, err)
	}
	if !strings.Contains(err.Error(), name) {
		t.Fatalf("Register(%q) error %q does not name the factory", name, err)
	}
}

func TestRegisterRefusesATypeOutsideTheFive(t *testing.T) {
	for _, componentType := range []types.ComponentType{"procesor", ""} {
		registry := NewRegistry()
		err := registry.Register(Registration{
			Name: "misspelled", Type: componentType, Factory: registerTestFactory, Ports: noPorts,
		})
		requireInvalidNaming(t, err, "misspelled")
		if _, registered := registry.ListFactories()["misspelled"]; registered {
			t.Fatalf("type %q: a refused registration was stored", componentType)
		}
	}
}

func TestRegisterRefusesAnIncompleteRegistration(t *testing.T) {
	tests := []struct {
		name         string
		registration Registration
	}{
		{"name the create path refuses", Registration{
			Name: "bad name!", Type: "processor", Factory: registerTestFactory, Ports: noPorts}},
		{"nil factory", Registration{Name: "no-factory", Type: "processor", Ports: noPorts}},
		{"nil port declarer", Registration{Name: "no-ports", Type: "processor", Factory: registerTestFactory}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			requireInvalidNaming(t, registry.Register(tt.registration), tt.registration.Name)
			if got := len(registry.ListFactories()); got != 0 {
				t.Fatalf("refused registration left %d factories, want 0", got)
			}
		})
	}
}

func TestRegisterRefusesADuplicateName(t *testing.T) {
	registry := NewRegistry()
	requireNoError(t, registry.Register(Registration{
		Name: "worker", Type: "processor", Description: "first", Factory: registerTestFactory, Ports: noPorts,
	}))
	err := registry.Register(Registration{
		Name: "worker", Type: "output", Description: "second", Factory: registerTestFactory, Ports: noPorts,
	})
	requireInvalidNaming(t, err, "worker")
	got := registry.ListFactories()["worker"]
	if got == nil || got.Description != "first" || got.Type != "processor" {
		t.Fatalf("after a refused duplicate, worker = %+v, want the first registration", got)
	}
}

func TestRegisterKeepsItsOwnCopy(t *testing.T) {
	registry := NewRegistry()
	maxPort := 65535
	registration := Registration{
		Name: "worker", Type: "processor", Description: "as registered", Version: "1.0.0",
		Schema: ConfigSchema{
			Properties: map[string]PropertySchema{
				"port": {Type: "int", Description: "listen port", Maximum: &maxPort, Enum: []string{"80"}},
			},
			Required: []string{"port"},
		},
		Dependencies: []string{DepModelRegistry},
		Factory:      registerTestFactory,
		Ports:        noPorts,
	}
	requireNoError(t, registry.Register(registration))

	registration.Description = "changed"
	registration.Schema.Properties["port"] = PropertySchema{Type: "string", Description: "changed"}
	registration.Schema.Properties["extra"] = PropertySchema{Type: "bool"}
	registration.Schema.Required[0] = "extra"
	registration.Dependencies[0] = "changed"
	maxPort = 1

	got := registry.ListFactories()["worker"]
	if got == nil {
		t.Fatal("worker is not registered")
	}
	if got.Description != "as registered" || got.Version != "1.0.0" {
		t.Fatalf("description/version = %q/%q, want as registered/1.0.0", got.Description, got.Version)
	}
	wantMax := 65535
	wantSchema := ConfigSchema{
		Properties: map[string]PropertySchema{
			"port": {Type: "int", Description: "listen port", Maximum: &wantMax, Enum: []string{"80"}},
		},
		Required: []string{"port"},
	}
	if !reflect.DeepEqual(got.Schema, wantSchema) {
		t.Fatalf("schema = %+v, want %+v", got.Schema, wantSchema)
	}
	if !reflect.DeepEqual(got.Dependencies, []string{"model-registry"}) {
		t.Fatalf("dependencies = %v, want [model-registry]", got.Dependencies)
	}

	// The stored copy keeps the factory: the registered type can still be created.
	if _, err := registry.CreateComponent(
		"instance", declarationTestConfig("worker", `{}`), declarationTestDeps(), nil); err != nil {
		t.Fatalf("CreateComponent after the caller changed its value: %v", err)
	}
}

func TestSnapshotsReturnsIndependentDeclarations(t *testing.T) {
	registry := NewRegistry()
	created := &declarationTestComponent{outputs: []Port{declarationTestPort("events.created")}}
	requireNoError(t, registry.Register(Registration{
		Name: "declaration-test", Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) { return created, nil },
		Ports:   created.declarePorts,
	}))
	_, err := registry.CreateComponent(
		"worker", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)

	first := registry.Snapshots()
	if len(first) != 1 || first[0].InstanceName != "worker" {
		t.Fatalf("Snapshots = %+v, want one worker declaration", first)
	}
	first[0].OutputPorts[0].Config.(JetStreamPort).Subjects[0] = "events.mutated"
	first[0].OutputPorts[0].Name = "mutated"
	first[0].OutputFacts[0].natsSubjects[0] = "facts.mutated"

	second := registry.Snapshots()
	if len(second) != 1 {
		t.Fatalf("second Snapshots length = %d, want 1", len(second))
	}
	port := second[0].OutputPorts[0]
	if subjects := port.Config.(JetStreamPort).Subjects; !reflect.DeepEqual(subjects, []string{"events.created"}) {
		t.Fatalf("port subjects after a caller's change = %v, want [events.created]", subjects)
	}
	if port.Name != "events" {
		t.Fatalf("port name after a caller's change = %q, want events", port.Name)
	}
	if facts := second[0].OutputFacts[0].NATSSubjects(); !reflect.DeepEqual(facts, []string{"events.created"}) {
		t.Fatalf("fact subjects after a caller's change = %v, want [events.created]", facts)
	}
}
