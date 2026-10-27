package graphindex

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// createFromJSON builds graph-index through CreateGraphIndex from raw, as a composition
// root would, with a NATS client that is never connected: construction opens no
// connection.
func createFromJSON(t *testing.T, raw string) (*Component, error) {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphIndex(json.RawMessage(raw), component.Dependencies{NATSClient: client})
	if err != nil {
		return nil, err
	}
	return built.(*Component), nil
}

// defaultConfigWith is DefaultConfig marshalled to JSON with key set to value.
func defaultConfigWith(t *testing.T, key string, value any) string {
	t.Helper()
	raw, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal default config: %v", err)
	}
	fields[key] = value
	out, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal config with %s: %v", key, err)
	}
	return string(out)
}

// TestUnknownConfigKeyIsRefused: a configuration carrying a key graph-index does not
// define is refused at construction, naming the key; the same configuration without it
// builds (design D9; graph-index spec, "A configuration carrying batch_size"). The keys
// are batch_size, which the pin defined and nothing read (S15), and a misspelling.
func TestUnknownConfigKeyIsRefused(t *testing.T) {
	raw, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if _, err := createFromJSON(t, string(raw)); err != nil {
		t.Fatalf("CreateGraphIndex with the default config: %v", err)
	}

	for key, value := range map[string]any{"batch_size": 50, "worker": 2} {
		t.Run(key, func(t *testing.T) {
			_, err := createFromJSON(t, defaultConfigWith(t, key, value))
			if err == nil {
				t.Fatalf("CreateGraphIndex accepted a config with the unknown key %s", key)
			}
			if !strings.Contains(err.Error(), `"`+key+`"`) {
				t.Fatalf("CreateGraphIndex error does not name the unknown key %s: %v", key, err)
			}
			if _, err := DeclarePorts(json.RawMessage(defaultConfigWith(t, key, value)), "graph-index"); err == nil {
				t.Fatalf("DeclarePorts accepted a config with the unknown key %s", key)
			}
		})
	}
}

// TestConfigRefusesDataAfterTheObject: strict decoding keeps json.Unmarshal's refusal of
// anything after the configuration object.
func TestConfigRefusesDataAfterTheObject(t *testing.T) {
	if _, err := createFromJSON(t, `{"workers": 2} {"workers": 3}`); err == nil {
		t.Fatal("CreateGraphIndex accepted data after the configuration object")
	}
	if _, err := createFromJSON(t, `{invalid json}`); err == nil {
		t.Fatal("CreateGraphIndex accepted malformed JSON")
	}
}

// TestConfigWorkersZeroMeansOneLane: "workers": 0 builds and runs one lane, the default
// semsource relies on (design D9; graph-index spec, "workers 0"); an absent key is the
// same.
func TestConfigWorkersZeroMeansOneLane(t *testing.T) {
	for name, raw := range map[string]string{
		"zero":   `{"workers": 0}`,
		"absent": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, err := createFromJSON(t, raw)
			if err != nil {
				t.Fatalf("CreateGraphIndex(%s): %v", raw, err)
			}
			if c.config.Workers != 1 {
				t.Fatalf("CreateGraphIndex(%s) lanes = %d, want 1", raw, c.config.Workers)
			}
		})
	}
}

// TestConfigWorkersRange: workers other than 0 is the lane count, 1 to 16; outside that
// range construction fails as invalid configuration (design D9).
func TestConfigWorkersRange(t *testing.T) {
	for _, workers := range []int{1, 7, 16} {
		c, err := createFromJSON(t, defaultConfigWith(t, "workers", workers))
		if err != nil {
			t.Fatalf("CreateGraphIndex with workers %d: %v", workers, err)
		}
		if c.config.Workers != workers {
			t.Fatalf("CreateGraphIndex with workers %d: lanes = %d", workers, c.config.Workers)
		}
	}
	for _, workers := range []int{-1, 17} {
		_, err := createFromJSON(t, defaultConfigWith(t, "workers", workers))
		if err == nil {
			t.Fatalf("CreateGraphIndex accepted workers %d", workers)
		}
		var classified *errs.ClassifiedError
		if !errors.As(err, &classified) || classified.Class != errs.ErrorInvalid {
			t.Fatalf("CreateGraphIndex with workers %d: error is not classified invalid: %v", workers, err)
		}
	}
}

// TestConfigDefaults: the startup wait defaults to 30 attempts 500 ms apart, coalescing
// is off, and each set value is kept (pin ApplyDefaults, less batch_size).
func TestConfigDefaults(t *testing.T) {
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphIndex({}): %v", err)
	}
	if c.config.StartupAttempts != 30 || c.config.StartupInterval != 500 || c.config.CoalesceMs != 0 {
		t.Fatalf("defaults: startup_attempts %d, startup_interval_ms %d, coalesce_ms %d; want 30, 500, 0",
			c.config.StartupAttempts, c.config.StartupInterval, c.config.CoalesceMs)
	}

	c, err = createFromJSON(t, `{"startup_attempts": 3, "startup_interval_ms": 20, "coalesce_ms": 15}`)
	if err != nil {
		t.Fatalf("CreateGraphIndex with set values: %v", err)
	}
	if c.config.StartupAttempts != 3 || c.config.StartupInterval != 20 || c.config.CoalesceMs != 15 {
		t.Fatalf("set values: startup_attempts %d, startup_interval_ms %d, coalesce_ms %d; want 3, 20, 15",
			c.config.StartupAttempts, c.config.StartupInterval, c.config.CoalesceMs)
	}
}

// indexOutputs is the four kv-write outputs graph-index requires.
func indexOutputs() []component.PortDefinition {
	return []component.PortDefinition{
		{Name: "outgoing_index", Config: component.KVWritePort{Bucket: graph.BucketOutgoingIndex}},
		{Name: "incoming_index", Config: component.KVWritePort{Bucket: graph.BucketIncomingIndex}},
		{Name: "alias_index", Config: component.KVWritePort{Bucket: graph.BucketAliasIndex}},
		{Name: "predicate_index", Config: component.KVWritePort{Bucket: graph.BucketPredicateIndex}},
	}
}

// TestConfigValidateOutputs: the four graph-index buckets are required and the only
// outputs permitted, each a kv-write port (pin component_test.go:281, :330 and
// Validate's ownership check).
func TestConfigValidateOutputs(t *testing.T) {
	entityWatch := []component.PortDefinition{
		{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}},
	}
	valid := Config{Ports: &component.PortConfig{Inputs: entityWatch, Outputs: indexOutputs()}, Workers: 4}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate refused the four index outputs: %v", err)
	}

	cases := map[string]Config{
		"no ports":      {},
		"no inputs":     {Ports: &component.PortConfig{Outputs: indexOutputs()}},
		"no outputs":    {Ports: &component.PortConfig{Inputs: entityWatch}},
		"missing three": {Ports: &component.PortConfig{Inputs: entityWatch, Outputs: indexOutputs()[:1]}},
		"another owner's bucket": {Ports: &component.PortConfig{Inputs: entityWatch, Outputs: append(indexOutputs(),
			component.PortDefinition{Name: "entities", Config: component.KVWritePort{Bucket: graph.BucketEntityStates}})}},
		"off the catalog": {Ports: &component.PortConfig{Inputs: entityWatch, Outputs: append(indexOutputs(),
			component.PortDefinition{Name: "typo", Config: component.KVWritePort{Bucket: "OUTGOING_INDX"}})}},
		"not a kv-write": {Ports: &component.PortConfig{Inputs: entityWatch, Outputs: append(indexOutputs(),
			component.PortDefinition{Name: "events", Config: component.NATSPort{Subject: "graph.index.events"}})}},
	}
	for name, config := range cases {
		t.Run(name, func(t *testing.T) {
			if err := config.Validate(); err == nil {
				t.Fatalf("Validate accepted %s", name)
			}
		})
	}
}

// TestDefaultConfigIsValid: DefaultConfig validates and names the four index buckets
// (pin component_test.go:482).
func TestDefaultConfigIsValid(t *testing.T) {
	config := DefaultConfig()
	if err := config.Validate(); err != nil {
		t.Fatalf("DefaultConfig does not validate: %v", err)
	}
	if config.Workers != 1 {
		t.Fatalf("DefaultConfig workers = %d, want 1", config.Workers)
	}
	buckets := map[string]bool{}
	for _, definition := range config.Ports.Outputs {
		port, err := definition.Resolve(component.DirectionOutput)
		if err != nil {
			t.Fatalf("resolve %s: %v", definition.Name, err)
		}
		facts, err := port.Facts()
		if err != nil {
			t.Fatalf("facts of %s: %v", definition.Name, err)
		}
		buckets[strings.TrimPrefix(facts.ResourceID(), "kv:")] = true
	}
	for _, bucket := range []string{graph.BucketOutgoingIndex, graph.BucketIncomingIndex, graph.BucketAliasIndex, graph.BucketPredicateIndex} {
		if !buckets[bucket] {
			t.Errorf("DefaultConfig has no output on %s", bucket)
		}
	}
}

// TestConfigWithoutPortsTakesDefaultPorts: a configuration with no ports key builds with
// DefaultConfig's ports (ruling K, #91 comment 6062681355, applied to graph-index by PR
// #414 comment 6431777824).
func TestConfigWithoutPortsTakesDefaultPorts(t *testing.T) {
	for _, raw := range []string{`{}`, `{"workers": 2}`, `{"ports": null}`} {
		t.Run(raw, func(t *testing.T) {
			c, err := createFromJSON(t, raw)
			if err != nil {
				t.Fatalf("CreateGraphIndex(%s): %v, want the default ports", raw, err)
			}
			if len(c.InputPorts()) != 1 || c.InputPorts()[0].Name != "entity_watch" {
				t.Fatalf("CreateGraphIndex(%s) inputs = %+v, want the default entity_watch", raw, c.InputPorts())
			}
			want := []string{"outgoing_index", "incoming_index", "alias_index", "predicate_index"}
			got := make([]string, 0, len(c.OutputPorts()))
			for _, port := range c.OutputPorts() {
				got = append(got, port.Name)
			}
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("CreateGraphIndex(%s) outputs = %v, want %v", raw, got, want)
			}
		})
	}
}

// TestConfigPresentPortsUsedAsWritten: a ports block that is present is the whole set,
// never merged with the defaults, so an empty inputs or outputs list is no inputs or no
// outputs and construction refuses it (ruling K, PR #414 comment 6431777824; the pin
// filled an empty side from the defaults).
func TestConfigPresentPortsUsedAsWritten(t *testing.T) {
	entityWatch := []component.PortDefinition{
		{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}},
	}
	// The empty list is spelled out as [] in the JSON, as a consumer would write it.
	cases := map[string]struct {
		ports map[string]any
		want  string
	}{
		"empty inputs":  {map[string]any{"inputs": []any{}, "outputs": indexOutputs()}, "at least one input port required"},
		"empty outputs": {map[string]any{"inputs": entityWatch, "outputs": []any{}}, "at least one output port required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			raw := defaultConfigWith(t, "ports", tc.ports)
			c, err := createFromJSON(t, raw)
			if err == nil {
				t.Fatalf("CreateGraphIndex(%s) built with inputs %+v and outputs %+v, want only the configured ports, refused: %s",
					raw, c.InputPorts(), c.OutputPorts(), tc.want)
			}
			if !errs.IsInvalid(err) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CreateGraphIndex(%s) = %v, want an invalid-configuration error naming %q", raw, err, tc.want)
			}
		})
	}
}
