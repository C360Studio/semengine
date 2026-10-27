package graphindex

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
)

// TestCreateGraphIndexRequiresNATSClient: a missing NATS client is a construction error
// (pin component_test.go:816).
func TestCreateGraphIndexRequiresNATSClient(t *testing.T) {
	built, err := CreateGraphIndex(json.RawMessage(`{}`), component.Dependencies{})
	if err == nil {
		t.Fatalf("CreateGraphIndex without a NATS client built %T", built)
	}
}

// TestRegisterAddsGraphIndex: Register adds the graph-index factory, whose schema lists
// the five keys and not batch_size, and whose port declarer reports the ports the factory
// builds (pin component_test.go:579, :866).
func TestRegisterAddsGraphIndex(t *testing.T) {
	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	registration, ok := registry.ListFactories()["graph-index"]
	if !ok {
		t.Fatal("Register did not add graph-index")
	}
	for _, key := range []string{"ports", "workers", "coalesce_ms", "startup_attempts", "startup_interval_ms"} {
		if _, ok := registration.Schema.Properties[key]; !ok {
			t.Errorf("schema has no %q property", key)
		}
	}
	if _, ok := registration.Schema.Properties["batch_size"]; ok {
		t.Error(`schema still has "batch_size"`)
	}

	declared, err := registration.Ports(json.RawMessage(`{}`), "graph-index")
	if err != nil {
		t.Fatalf("declare ports: %v", err)
	}
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphIndex({}): %v", err)
	}
	if want := component.PortConfigFrom(c.InputPorts(), c.OutputPorts()); !reflect.DeepEqual(declared, want) {
		t.Fatalf("declared ports %+v differ from the built component's %+v", declared, want)
	}
}

// TestComponentPortsAreTheConfiguredPorts: the component reports one kv-watch input on
// ENTITY_STATES and the four index outputs, before Initialize (pin component_test.go:530,
// :554), and a caller cannot change them through the returned slice.
func TestComponentPortsAreTheConfiguredPorts(t *testing.T) {
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphIndex({}): %v", err)
	}
	inputs := c.InputPorts()
	if len(inputs) != 1 || inputs[0].Direction != component.DirectionInput {
		t.Fatalf("inputs = %+v, want one input", inputs)
	}
	if watch, ok := inputs[0].Config.(component.KVWatchPort); !ok || watch.Bucket != graph.BucketEntityStates {
		t.Fatalf("input %+v is not a kv-watch on %s", inputs[0], graph.BucketEntityStates)
	}
	buckets := map[string]bool{}
	for _, port := range c.OutputPorts() {
		if port.Direction != component.DirectionOutput {
			t.Errorf("output %s has direction %v", port.Name, port.Direction)
		}
		if write, ok := port.Config.(component.KVWritePort); ok {
			buckets[write.Bucket] = true
		}
	}
	for _, bucket := range []string{graph.BucketOutgoingIndex, graph.BucketIncomingIndex, graph.BucketAliasIndex, graph.BucketPredicateIndex} {
		if !buckets[bucket] {
			t.Errorf("no kv-write output on %s", bucket)
		}
	}

	inputs[0].Name = "changed"
	if c.InputPorts()[0].Name != "entity_watch" {
		t.Fatal("changing the returned inputs changed the component's ports")
	}
}
