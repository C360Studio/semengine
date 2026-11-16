package graphembedding

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/component"
)

// TestCreateGraphEmbeddingRequiresNATSClient: a missing NATS client is a construction error of
// the factory itself, which the registry's own check would otherwise hide.
func TestCreateGraphEmbeddingRequiresNATSClient(t *testing.T) {
	built, err := newFactory(testEmbedderType, (&embedderSource{}).newEmbedder)(json.RawMessage(`{}`), component.Dependencies{})
	if err == nil {
		t.Fatalf("graph-embedding without a NATS client built %T", built)
	}
}

// TestRegisterAddsGraphEmbedding: Register and RegisterWithEmbedder add the graph-embedding
// factory, whose schema lists the seven keys Config defines and not batch_size or
// embedder_type, which asks boot for no model registry (the product brings its embedder), and
// whose port declarer reports the ports the factory builds.
func TestRegisterAddsGraphEmbedding(t *testing.T) {
	for name, register := range map[string]func(*component.Registry) error{
		"Register": Register,
		"RegisterWithEmbedder": func(r *component.Registry) error {
			return RegisterWithEmbedder(r, testEmbedderType, (&embedderSource{}).newEmbedder)
		},
	} {
		t.Run(name, func(t *testing.T) { checkRegistration(t, register) })
	}
}

func checkRegistration(t *testing.T, register func(*component.Registry) error) {
	t.Helper()
	registry := component.NewRegistry()
	if err := register(registry); err != nil {
		t.Fatalf("register: %v", err)
	}
	registration, ok := registry.ListFactories()["graph-embedding"]
	if !ok {
		t.Fatal("register did not add graph-embedding")
	}
	kept := []string{"ports", "workers", "max_text_len", "text_suffixes", "startup_attempts",
		"startup_interval_ms", "coalesce_ms"}
	for _, key := range kept {
		if _, ok := registration.Schema.Properties[key]; !ok {
			t.Errorf("schema has no %q property", key)
		}
	}
	if len(registration.Schema.Properties) != len(kept) {
		t.Errorf("schema properties = %d, want the %d kept keys", len(registration.Schema.Properties), len(kept))
	}
	for _, removed := range []string{"batch_size", "embedder_type"} {
		if _, ok := registration.Schema.Properties[removed]; ok {
			t.Errorf("schema still has %q", removed)
		}
	}
	if len(registration.Dependencies) != 0 {
		t.Errorf("dependencies = %q, want none", registration.Dependencies)
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
