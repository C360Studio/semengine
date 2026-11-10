package graphclustering

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/component"
)

// TestCreateGraphClusteringRequiresNATSClient: a missing NATS client is a construction error.
func TestCreateGraphClusteringRequiresNATSClient(t *testing.T) {
	built, err := CreateGraphClustering(json.RawMessage(`{}`), component.Dependencies{})
	if err == nil {
		t.Fatalf("CreateGraphClustering without a NATS client built %T", built)
	}
}

// TestRegisterAddsGraphClustering: Register adds the graph-clustering factory, whose schema lists
// the four kept keys and no removed one, which asks boot for no model registry (no provider at
// tier 0), and whose port declarer reports the ports the factory builds.
func TestRegisterAddsGraphClustering(t *testing.T) {
	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	registration, ok := registry.ListFactories()["graph-clustering"]
	if !ok {
		t.Fatal("Register did not add graph-clustering")
	}
	kept := []string{"ports", "detection_interval", "startup_attempts", "startup_interval_ms"}
	for _, key := range kept {
		if _, ok := registration.Schema.Properties[key]; !ok {
			t.Errorf("schema has no %q property", key)
		}
	}
	if len(registration.Schema.Properties) != len(kept) {
		t.Errorf("schema properties = %d, want the %d kept keys", len(registration.Schema.Properties), len(kept))
	}
	if len(registration.Dependencies) != 0 {
		t.Errorf("dependencies = %q, want none at tier 0", registration.Dependencies)
	}

	declared, err := registration.Ports(json.RawMessage(`{}`), "graph-clustering")
	if err != nil {
		t.Fatalf("declare ports: %v", err)
	}
	c := mustCreate(t, `{}`)
	if want := component.PortConfigFrom(c.InputPorts(), c.OutputPorts()); !reflect.DeepEqual(declared, want) {
		t.Fatalf("declared ports %+v differ from the built component's %+v", declared, want)
	}
}
