package graphquery

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/component"
)

// TestCreateGraphQueryRequiresNATSClient: a missing NATS client is a construction error.
func TestCreateGraphQueryRequiresNATSClient(t *testing.T) {
	built, err := CreateGraphQuery(json.RawMessage(`{}`), component.Dependencies{})
	if err == nil {
		t.Fatalf("CreateGraphQuery without a NATS client built %T", built)
	}
}

// TestRegisterAddsGraphQuery: Register adds the graph-query factory, whose schema lists exactly the
// two keys and whose port declarer reports the ports the factory builds. Importing the package
// registers nothing: the registry is empty until Register runs.
func TestRegisterAddsGraphQuery(t *testing.T) {
	registry := component.NewRegistry()
	if len(registry.ListFactories()) != 0 {
		t.Fatalf("a new registry lists %v", registry.ListFactories())
	}
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	registration, ok := registry.ListFactories()["graph-query"]
	if !ok {
		t.Fatal("Register did not add graph-query")
	}
	if len(registration.Schema.Properties) != 2 {
		t.Errorf("schema properties = %v, want ports and query_timeout_ms", registration.Schema.Properties)
	}
	for _, key := range []string{"ports", "query_timeout_ms"} {
		if _, ok := registration.Schema.Properties[key]; !ok {
			t.Errorf("schema has no %q property", key)
		}
	}

	declared, err := registration.Ports(json.RawMessage(`{}`), "graph-query")
	if err != nil {
		t.Fatalf("declare ports: %v", err)
	}
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery({}): %v", err)
	}
	if want := component.PortConfigFrom(c.InputPorts(), c.OutputPorts()); !reflect.DeepEqual(declared, want) {
		t.Fatalf("declared ports %+v differ from the built component's %+v", declared, want)
	}
}

// TestInputPortsAreACopy: a caller cannot change the component's ports through the returned slice.
func TestInputPortsAreACopy(t *testing.T) {
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery({}): %v", err)
	}
	c.InputPorts()[0].Name = "changed"
	if name := c.InputPorts()[0].Name; name != "graph_queries" {
		t.Fatalf("changing the returned inputs changed the component's port to %q", name)
	}
}
