package graphclustering

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// createFromJSON builds graph-clustering through CreateGraphClustering from raw, as a
// composition root would, with a NATS client that is never connected: construction opens no
// connection.
func createFromJSON(t *testing.T, raw string) (*Component, error) {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphClustering(json.RawMessage(raw), component.Dependencies{NATSClient: client})
	if err != nil {
		return nil, err
	}
	return built.(*Component), nil
}

// mustCreate is createFromJSON that fails the test on a refusal.
func mustCreate(t *testing.T, raw string) *Component {
	t.Helper()
	c, err := createFromJSON(t, raw)
	if err != nil {
		t.Fatalf("CreateGraphClustering(%s): %v", raw, err)
	}
	return c
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

// requireRefused fails the test unless raw is refused by CreateGraphClustering and by
// DeclarePorts with an invalid-config error whose text holds every one of want.
func requireRefused(t *testing.T, raw string, want ...string) {
	t.Helper()
	_, err := createFromJSON(t, raw)
	if err == nil {
		t.Fatalf("CreateGraphClustering accepted %s", raw)
	}
	if !errs.IsInvalid(err) {
		t.Fatalf("CreateGraphClustering(%s) = %v; want an invalid-config error", raw, err)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Fatalf("CreateGraphClustering(%s) = %v; want it to say %q", raw, err, w)
		}
	}
	if _, err := DeclarePorts(json.RawMessage(raw), key); err == nil {
		t.Fatalf("DeclarePorts accepted %s", raw)
	}
}

// TestConfigRefusesRemovedKeys: each configuration key the pin defined and graph-clustering
// does not is refused at construction and by port declaration, naming the key and saying it was
// removed with what replaces it; the same configuration without it builds (design D3;
// graph-clustering spec, "Removed configuration is refused"). The keys and the words of their
// guidance are written out here, not read from the package's table. A key nobody ever defined
// (a misspelling) is refused naming it too.
func TestConfigRefusesRemovedKeys(t *testing.T) {
	raw, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	mustCreate(t, string(raw))

	cases := []struct {
		key      string
		value    any
		guidance string
	}{
		{"batch_size", 100, "detection_interval"},
		{"enable_llm", true, "tier 2"},
		{"enhancement_workers", 5, "tier 2"},
		{"min_community_size", 3, "ADR-099"},
		{"max_iterations", 100, "ADR-099"},
		{"entity_id_edges", map[string]any{"include_siblings": true}, "ADR-099"},
		{"semantic_edges", map[string]any{"enable_semantic_edges": true}, "ADR-099"},
		{"allow_ungated_reads", true, "readiness gate"},
		{"enable_anomaly_detection", true, "anomaly detection is not ported"},
		{"anomaly_config", map[string]any{"enabled": true}, "anomaly detection is not ported"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			requireRefused(t, defaultConfigWith(t, tc.key, tc.value), `"`+tc.key+`" was removed`, tc.guidance)
		})
	}
	t.Run("every removed key named", func(t *testing.T) {
		requireRefused(t, `{"max_iterations": 100, "batch_size": 100}`, `"batch_size" was removed`,
			`"max_iterations" was removed`)
	})
	t.Run("unknown key", func(t *testing.T) {
		requireRefused(t, defaultConfigWith(t, "detection_intervals", "30s"), `"detection_intervals"`)
	})
}

// TestConfigRefusesDataAfterTheObject: strict decoding keeps json.Unmarshal's refusal of anything
// after the configuration object, and of malformed JSON.
func TestConfigRefusesDataAfterTheObject(t *testing.T) {
	for _, raw := range []string{`{"startup_attempts": 2} {"startup_attempts": 3}`, `{invalid json}`, `[]`} {
		if _, err := createFromJSON(t, raw); !errs.IsInvalid(err) {
			t.Fatalf("CreateGraphClustering(%s) = %v; want an invalid-config refusal", raw, err)
		}
	}
}

// entityWatch is the one ENTITY_STATES input the follower reads.
func entityWatch() component.PortDefinition {
	return component.PortDefinition{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}}
}

// TestConfigRefusesOutputPorts: graph-clustering writes COMMUNITY_INDEX itself and declares no
// output ports, so the pin's default outputs (a graph-mutation request port and a COMMUNITY_INDEX
// write, which semboids' configuration declares) are refused, as is any one of them alone
// (design D3).
func TestConfigRefusesOutputPorts(t *testing.T) {
	mutations := component.PortDefinition{Name: "graph_mutations", Config: component.NATSRequestPort{Subject: "graph.mutation.>"}}
	communities := component.PortDefinition{Name: "communities", Config: component.KVWritePort{Bucket: graph.BucketCommunityIndex}}
	for name, outputs := range map[string][]component.PortDefinition{
		"the pin's defaults": {mutations, communities},
		"COMMUNITY_INDEX":    {communities},
	} {
		t.Run(name, func(t *testing.T) {
			raw := defaultConfigWith(t, "ports", map[string]any{
				"inputs":  []component.PortDefinition{entityWatch()},
				"outputs": outputs,
			})
			requireRefused(t, raw, "no output ports", "remove ports.outputs")
		})
	}
}

// TestConfigWithoutPortsTakesDefaultPorts: a configuration with no ports block takes the one
// ENTITY_STATES watch and no outputs; the returned slice is a copy.
func TestConfigWithoutPortsTakesDefaultPorts(t *testing.T) {
	c := mustCreate(t, `{}`)
	inputs := c.InputPorts()
	if len(inputs) != 1 {
		t.Fatalf("inputs = %+v, want the entity watch", inputs)
	}
	if watch, ok := inputs[0].Config.(component.KVWatchPort); !ok || watch.Bucket != graph.BucketEntityStates ||
		len(watch.Keys) != 0 || watch.History || inputs[0].Direction != component.DirectionInput {
		t.Fatalf("input %+v is not a kv-watch input of every %s key", inputs[0], graph.BucketEntityStates)
	}
	if outputs := c.OutputPorts(); len(outputs) != 0 {
		t.Fatalf("outputs = %+v, want none", outputs)
	}
	inputs[0].Name = "changed"
	if c.InputPorts()[0].Name != "entity_watch" {
		t.Fatal("changing the returned inputs changed the component's ports")
	}
}

// TestConfigRefusesInputsTheFollowerDoesNotRead: the one input is the follower's watch of every
// ENTITY_STATES key with no history; any other input, a second input, or none is refused, naming
// the port where there is one, so a composition never advertises a feed graph-clustering does not
// read (#424, as graph-index). The pin's default also read ENTITY_STATES, OUTGOING_INDEX and
// INCOMING_INDEX through kv-read ports; those are refused.
func TestConfigRefusesInputsTheFollowerDoesNotRead(t *testing.T) {
	cases := map[string]struct {
		inputs []component.PortDefinition
		want   string
	}{
		"other bucket": {[]component.PortDefinition{{Name: "w", Config: component.KVWatchPort{Bucket: graph.BucketCommunityIndex}}}, `"w"`},
		"some keys":    {[]component.PortDefinition{{Name: "w", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates, Keys: []string{"acme.>"}}}}, `"w"`},
		"history":      {[]component.PortDefinition{{Name: "w", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates, History: true}}}, `"w"`},
		"kv read":      {[]component.PortDefinition{{Name: "outgoing_index", Config: component.KVReadPort{Bucket: graph.BucketOutgoingIndex}}}, `"outgoing_index"`},
		"second input": {[]component.PortDefinition{entityWatch(), {Name: "entity_states", Config: component.KVReadPort{Bucket: graph.BucketEntityStates}}}, "exactly one input"},
		"none":         {[]component.PortDefinition{}, "exactly one input"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			requireRefused(t, defaultConfigWith(t, "ports", map[string]any{"inputs": tc.inputs}), tc.want)
		})
	}
}

// TestConfigStartupWait: the wait for ENTITY_STATES defaults to 30 attempts 500 ms apart, and set
// values are kept (pin ApplyDefaults); a negative value is refused.
func TestConfigStartupWait(t *testing.T) {
	c := mustCreate(t, `{}`)
	if c.config.StartupAttempts != 30 || c.config.StartupInterval != 500 {
		t.Fatalf("defaults: startup_attempts %d, startup_interval_ms %d; want 30, 500",
			c.config.StartupAttempts, c.config.StartupInterval)
	}
	c = mustCreate(t, `{"startup_attempts": 3, "startup_interval_ms": 20}`)
	if c.config.StartupAttempts != 3 || c.config.StartupInterval != 20 {
		t.Fatalf("set values: startup_attempts %d, startup_interval_ms %d; want 3, 20",
			c.config.StartupAttempts, c.config.StartupInterval)
	}
	requireRefused(t, `{"startup_attempts": -1}`, "startup_attempts")
	requireRefused(t, `{"startup_interval_ms": -1}`, "startup_interval_ms")
}

// TestConfigDetectionInterval: detection_interval defaults to the pin's 30 s and a set duration is
// kept; where the pin fell back to 30 s on a value it could not parse, a value that is not a
// duration, or is not above zero, is refused naming the key. The ticker that reads it is
// TestDetectionIntervalIsRead's (task 2.3).
func TestConfigDetectionInterval(t *testing.T) {
	for raw, want := range map[string]string{`{}`: "30s", `{"detection_interval": "5m"}`: "5m0s"} {
		d, err := mustCreate(t, raw).config.interval()
		if err != nil || d.String() != want {
			t.Fatalf("%s: interval = %v, %v; want %s", raw, d, err, want)
		}
	}
	for _, value := range []string{"soon", "30", "0s", "-1s"} {
		requireRefused(t, defaultConfigWith(t, "detection_interval", value), "detection_interval", `"`+value+`"`)
	}
}

// FuzzResolveConfig: for any bytes, resolveConfig, DeclarePorts and CreateGraphClustering agree
// on accepting or refusing, a refusal is classified invalid, and whatever is accepted carries no
// removed key and reads only what graph-clustering reads: a detection interval above zero, a
// startup wait of at least one attempt, no outputs, and one input that is a full ENTITY_STATES
// watch with no history. The seeds cover each class: empty, null, the defaults, each kept key, a
// removed key, an unknown key, trailing data, a bad or non-positive interval, a negative startup
// value, an output, an unread input, two inputs, no input, not JSON, not an object.
func FuzzResolveConfig(f *testing.F) {
	for _, seed := range []string{
		``,
		`null`,
		`{}`,
		`{"detection_interval":"1m","startup_attempts":2,"startup_interval_ms":10}`,
		`{"ports":{"inputs":[{"name":"w","config":{"kind":"kv-watch","bucket":"ENTITY_STATES"}}]}}`,
		`{"entity_id_edges":{"include_siblings":false}}`,
		`{"enable_anomaly_detection":true}`,
		`{"detection_intervals":"30s"}`,
		`{} {}`,
		`{"detection_interval":"soon"}`,
		`{"detection_interval":"0s"}`,
		`{"startup_attempts":-1}`,
		`{"ports":{"inputs":[{"name":"w","config":{"kind":"kv-watch","bucket":"ENTITY_STATES"}}],"outputs":[{"name":"o","config":{"kind":"kv-write","bucket":"COMMUNITY_INDEX"}}]}}`,
		`{"ports":{"inputs":[{"name":"w","config":{"kind":"kv-watch","bucket":"ENTITY_STATES","history":true}}]}}`,
		`{"ports":{"inputs":[{"name":"w","config":{"kind":"kv-watch","bucket":"ENTITY_STATES"}},{"name":"r","config":{"kind":"kv-read","bucket":"ENTITY_STATES"}}]}}`,
		`{"ports":{"inputs":[]}}`,
		`{`,
		`[]`,
	} {
		f.Add([]byte(seed))
	}
	client, err := natsclient.NewClient("")
	if err != nil {
		f.Fatalf("natsclient.NewClient: %v", err)
	}
	removed := []string{"batch_size", "enable_llm", "enhancement_workers", "min_community_size", "max_iterations",
		"entity_id_edges", "semantic_edges", "allow_ungated_reads", "enable_anomaly_detection", "anomaly_config"}
	f.Fuzz(func(t *testing.T, raw []byte) {
		config, inputs, err := resolveConfig(raw)
		_, declareErr := DeclarePorts(raw, key)
		_, createErr := CreateGraphClustering(raw, component.Dependencies{NATSClient: client})
		if (err == nil) != (declareErr == nil) || (err == nil) != (createErr == nil) {
			t.Fatalf("resolve %v, DeclarePorts %v, CreateGraphClustering %v disagree on %q", err, declareErr, createErr, raw)
		}
		if err != nil {
			if !errs.IsInvalid(err) {
				t.Fatalf("resolveConfig(%q) refused with an error not classified invalid: %v", raw, err)
			}
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil {
			for _, k := range removed {
				if _, ok := fields[k]; ok {
					t.Fatalf("resolveConfig(%q) accepted the removed key %s", raw, k)
				}
			}
		}
		if d, err := config.interval(); err != nil || d <= 0 || config.StartupAttempts < 1 || config.StartupInterval < 1 {
			t.Fatalf("resolveConfig(%q) accepted %+v (interval %v, %v)", raw, config, d, err)
		}
		if len(config.Ports.Outputs) != 0 || len(inputs) != 1 {
			t.Fatalf("resolveConfig(%q) accepted ports %+v", raw, config.Ports)
		}
		if port, ok := inputs[0].Config.(component.KVWatchPort); !ok || port.Bucket != graph.BucketEntityStates ||
			len(port.Keys) != 0 || port.History {
			t.Fatalf("resolveConfig(%q) accepted input %+v", raw, inputs[0])
		}
	})
}
