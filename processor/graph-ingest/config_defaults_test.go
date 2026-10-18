package graphingest

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
)

// Ruling K (#91 comment 6062681355): a configuration with no ports key takes
// DefaultConfig's ports; a ports block that is present is used as written, with no
// merge; Validate only checks, and the ingest-lane clamp is ApplyDefaults'.

// createFromJSON builds graph-ingest through CreateGraphIngest from raw, as a
// composition root would.
func createFromJSON(t *testing.T, raw string) (*Component, error) {
	t.Helper()
	natsClient, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphIngest(json.RawMessage(raw), testDependencies(t, natsClient))
	if err != nil {
		return nil, err
	}
	return built.(*Component), nil
}

func portNames(ports []component.Port) []string {
	names := make([]string, 0, len(ports))
	for _, port := range ports {
		names = append(names, port.Name)
	}
	return names
}

// TestConfigWithoutPortsTakesDefaultPorts: a configuration that omits ports builds
// with DefaultConfig's input and output ports.
func TestConfigWithoutPortsTakesDefaultPorts(t *testing.T) {
	defaults := DefaultConfig()
	for _, raw := range []string{`{}`, `{"enable_hierarchy":true}`, `{"ports":null}`} {
		t.Run(raw, func(t *testing.T) {
			c, err := createFromJSON(t, raw)
			if err != nil {
				t.Fatalf("CreateGraphIngest(%s): %v, want the default ports", raw, err)
			}
			if !reflect.DeepEqual(c.config.Ports, defaults.Ports) {
				t.Fatalf("CreateGraphIngest(%s) ports = %+v, want DefaultConfig's %+v", raw, c.config.Ports, defaults.Ports)
			}
			wantInputs := []string{"entity_stream", "mutations"}
			if got := portNames(c.InputPorts()); !slices.Equal(got, wantInputs) {
				t.Fatalf("CreateGraphIngest(%s) inputs = %v, want %v", raw, got, wantInputs)
			}
			if got := portNames(c.OutputPorts()); !slices.Equal(got, []string{"entity_states"}) {
				t.Fatalf("CreateGraphIngest(%s) outputs = %v, want [entity_states]", raw, got)
			}
		})
	}
}

// TestConfigPresentPortsUsedAsWritten: a ports block that is present is the whole port
// set. One that omits the entity stream builds without it, and an empty one is
// refused, rather than either being filled from the defaults.
func TestConfigPresentPortsUsedAsWritten(t *testing.T) {
	mutationsOnly := `{"ports":{` +
		`"inputs":[{"name":"mutations","required":true,"config":{"kind":"nats-request","subject":"graph.mutation.>",` +
		`"interface":{"type":"semengine.graph.mutation","version":"v1"}}}],` +
		`"outputs":[{"name":"entity_states","config":{"kind":"kv-write","bucket":"ENTITY_STATES"}}]}}`
	c, err := createFromJSON(t, mutationsOnly)
	if err != nil {
		t.Fatalf("CreateGraphIngest(mutations-only ports): %v", err)
	}
	if got := portNames(c.InputPorts()); !slices.Equal(got, []string{"mutations"}) {
		t.Fatalf("inputs = %v, want only the configured [mutations]", got)
	}

	if _, err := createFromJSON(t, `{"ports":{}}`); err == nil {
		t.Fatal(`CreateGraphIngest({"ports":{}}) succeeded, want an empty ports block refused as written`)
	}
}

// TestConfigValidateOnlyChecks: Validate leaves the configuration it checks as it was.
func TestConfigValidateOnlyChecks(t *testing.T) {
	config := DefaultConfig()
	config.IngestLanes = -5
	before := DefaultConfig()
	before.IngestLanes = -5

	_ = config.Validate()

	if !reflect.DeepEqual(config, before) {
		t.Fatalf("Validate changed the configuration: got %+v, want %+v", config, before)
	}
}

// TestConfigApplyDefaultsClampsLanes: ApplyDefaults reads 0 as unset (eight lanes),
// keeps a positive count, and clamps a negative one to a single lane.
func TestConfigApplyDefaultsClampsLanes(t *testing.T) {
	for lanes, want := range map[int]int{0: 8, 1: 1, 3: 3, -1: 1, -5: 1} {
		config := Config{IngestLanes: lanes}
		config.ApplyDefaults()
		if config.IngestLanes != want {
			t.Errorf("ApplyDefaults: ingest_lanes %d became %d, want %d", lanes, config.IngestLanes, want)
		}
	}
}
