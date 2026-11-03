package graphquery

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
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

// TestInputPortsSliceIsNew: a caller writing a field of a returned element does not change the
// component's port. A write through the element's Interface pointer would: that is shared.
func TestInputPortsSliceIsNew(t *testing.T) {
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery({}): %v", err)
	}
	c.InputPorts()[0].Name = "changed"
	if name := c.InputPorts()[0].Name; name != "graph_queries" {
		t.Fatalf("changing the returned inputs changed the component's port to %q", name)
	}
}

// TestGraphQueryIsItsVerbsResponder: the real factory's name and declared port satisfy the check the
// graph.QueryVerbs table drives: one instance draws no request_subject_conflict, so a composition
// with graph-query boots, and a second draws one, on the later in instance-name order.
func TestGraphQueryIsItsVerbsResponder(t *testing.T) {
	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	instance := types.ComponentConfig{
		Name: "graph-query", Type: types.ComponentTypeProcessor, Enabled: true, Config: json.RawMessage(`{}`),
	}
	validate := func(components config.ComponentConfigs) []composition.Finding {
		t.Helper()
		result, err := composition.Validate(registry, &config.Config{
			Version:    "1.0.0",
			Platform:   config.PlatformConfig{Org: "test", ID: "composition", Environment: "test"},
			Components: components,
		})
		if err != nil {
			t.Fatalf("Validate: %v", err)
		}
		var conflicts []composition.Finding
		for _, finding := range result.Errors {
			if finding.Type == composition.TypeRequestSubjectConflict {
				conflicts = append(conflicts, finding)
			}
		}
		return conflicts
	}
	if got := validate(config.ComponentConfigs{"query": instance}); len(got) != 0 {
		t.Errorf("one instance drew %+v", got)
	}
	got := validate(config.ComponentConfigs{"query-a": instance, "query-b": instance})
	if len(got) != 1 || got[0].Component != "query-b" || got[0].Port != "graph_queries" {
		t.Errorf("two instances drew %+v, want one on query-b's graph_queries", got)
	}
}
