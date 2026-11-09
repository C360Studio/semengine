package graphembedding

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/component"
)

// TestCreateGraphEmbeddingRequiresNATSClient: a missing NATS client is a construction
// error (pin component_test.go:722).
func TestCreateGraphEmbeddingRequiresNATSClient(t *testing.T) {
	built, err := CreateGraphEmbedding(json.RawMessage(`{}`), component.Dependencies{})
	if err == nil {
		t.Fatalf("CreateGraphEmbedding without a NATS client built %T", built)
	}
}

// TestRegisterAddsGraphEmbedding: Register adds the graph-embedding factory, whose schema
// lists the eight kept keys and not batch_size, which asks boot for no model registry (no
// provider at tier 0), and whose port declarer reports the ports the factory builds (pin
// component_test.go:770).
func TestRegisterAddsGraphEmbedding(t *testing.T) {
	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	registration, ok := registry.ListFactories()["graph-embedding"]
	if !ok {
		t.Fatal("Register did not add graph-embedding")
	}
	kept := []string{"ports", "embedder_type", "workers", "max_text_len", "text_suffixes", "startup_attempts",
		"startup_interval_ms", "coalesce_ms"}
	for _, key := range kept {
		if _, ok := registration.Schema.Properties[key]; !ok {
			t.Errorf("schema has no %q property", key)
		}
	}
	if len(registration.Schema.Properties) != len(kept) {
		t.Errorf("schema properties = %d, want the %d kept keys", len(registration.Schema.Properties), len(kept))
	}
	if _, ok := registration.Schema.Properties["batch_size"]; ok {
		t.Error(`schema still has "batch_size"`)
	}
	if len(registration.Dependencies) != 0 {
		t.Errorf("dependencies = %q, want none at tier 0", registration.Dependencies)
	}

	declared, err := registration.Ports(json.RawMessage(`{}`), "graph-embedding")
	if err != nil {
		t.Fatalf("declare ports: %v", err)
	}
	c := mustCreate(t, `{}`)
	if want := component.PortConfigFrom(c.InputPorts(), c.OutputPorts()); !reflect.DeepEqual(declared, want) {
		t.Fatalf("declared ports %+v differ from the built component's %+v", declared, want)
	}
}
