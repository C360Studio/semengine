package graphlexical

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/component"
)

// TestRegisterAddsGraphLexical: Register adds the graph-lexical factory, whose schema lists the
// five keys it defines, which asks boot for no model registry, and whose port declarer reports
// the ports the factory builds. A missing NATS client is a construction error.
func TestRegisterAddsGraphLexical(t *testing.T) {
	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	registration, ok := registry.ListFactories()["graph-lexical"]
	if !ok {
		t.Fatal("Register did not add graph-lexical")
	}
	keys := []string{"ports", "text_suffixes", "max_text_len", "startup_attempts", "startup_interval_ms"}
	for _, k := range keys {
		if _, ok := registration.Schema.Properties[k]; !ok {
			t.Errorf("schema has no %q property", k)
		}
	}
	if len(registration.Schema.Properties) != len(keys) {
		t.Errorf("schema properties = %d, want the %d defined keys", len(registration.Schema.Properties), len(keys))
	}
	if len(registration.Dependencies) != 0 {
		t.Errorf("dependencies = %q, want none", registration.Dependencies)
	}

	declared, err := registration.Ports(json.RawMessage(`{}`), "graph-lexical")
	if err != nil {
		t.Fatalf("declare ports: %v", err)
	}
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphLexical: %v", err)
	}
	if want := component.PortConfigFrom(c.InputPorts(), c.OutputPorts()); !reflect.DeepEqual(declared, want) {
		t.Fatalf("declared ports %+v differ from the built component's %+v", declared, want)
	}

	if built, err := CreateGraphLexical(json.RawMessage(`{}`), component.Dependencies{}); err == nil {
		t.Fatalf("CreateGraphLexical without a NATS client built %T", built)
	}
}
