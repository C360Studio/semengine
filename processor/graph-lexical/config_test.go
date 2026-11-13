package graphlexical

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// createFromJSON builds graph-lexical through CreateGraphLexical from raw, as a composition root
// would, with a NATS client that is never connected: construction opens no connection.
func createFromJSON(t *testing.T, raw string) (*Component, error) {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphLexical(json.RawMessage(raw), component.Dependencies{NATSClient: client})
	if err != nil {
		return nil, err
	}
	return built.(*Component), nil
}

// requireRefused fails the test unless raw is refused by CreateGraphLexical and by DeclarePorts
// with an invalid-config error whose text holds every one of want.
func requireRefused(t *testing.T, raw string, want ...string) {
	t.Helper()
	_, err := createFromJSON(t, raw)
	if err == nil {
		t.Fatalf("CreateGraphLexical accepted %s", raw)
	}
	if !errs.IsInvalid(err) {
		t.Fatalf("CreateGraphLexical(%s) = %v; want an invalid-config error", raw, err)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Fatalf("CreateGraphLexical(%s) = %v; want it to say %q", raw, err, w)
		}
	}
	if _, err := DeclarePorts(json.RawMessage(raw), key); err == nil {
		t.Fatalf("DeclarePorts accepted %s", raw)
	}
}

// TestConfigRefusesUnknownKey: a key graph-lexical does not define is refused at construction,
// naming it; graph-embedding's workers is one. The keys it does define build.
func TestConfigRefusesUnknownKey(t *testing.T) {
	requireRefused(t, `{"workers": 5}`, `"workers"`)
	requireRefused(t, `{"max_text_len": 100, "text_suffix": [".name"]}`, `"text_suffix"`)

	c, err := createFromJSON(t, `{"text_suffixes": [".name", ".body"], "max_text_len": 100,
		"startup_attempts": 2, "startup_interval_ms": 10}`)
	if err != nil {
		t.Fatalf("CreateGraphLexical with every defined key: %v", err)
	}
	if c.config.MaxTextLen != 100 || len(c.config.TextSuffixes) != 2 {
		t.Fatalf("config = %+v; want max_text_len 100 and the two suffixes", c.config)
	}
}

// TestConfigMaxTextLen: max_text_len defaults to 16,000 characters, and a negative value or one
// above 1,000,000 is refused naming the key.
func TestConfigMaxTextLen(t *testing.T) {
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphLexical: %v", err)
	}
	if c.config.MaxTextLen != 16000 {
		t.Fatalf("default max_text_len = %d; want 16000", c.config.MaxTextLen)
	}
	requireRefused(t, `{"max_text_len": -1}`, "max_text_len")
	requireRefused(t, `{"max_text_len": 1000001}`, "max_text_len")
	if _, err := createFromJSON(t, `{"max_text_len": 1000000}`); err != nil {
		t.Fatalf("max_text_len 1000000: %v", err)
	}
}

// TestConfigRefusesDataAfterTheObject: strict decoding refuses anything after the configuration
// object, and malformed JSON.
func TestConfigRefusesDataAfterTheObject(t *testing.T) {
	for _, raw := range []string{`{"startup_attempts": 2} {"startup_attempts": 3}`, `{invalid json}`, `[]`} {
		requireRefused(t, raw)
	}
}

// TestConfigRefusesPortsItDoesNotRead: graph-lexical reads one input, a kv-watch of every
// ENTITY_STATES key with no history, and writes through no output port; any other port, a second
// input or none is refused, naming the port where there is one.
func TestConfigRefusesPortsItDoesNotRead(t *testing.T) {
	watch := component.PortDefinition{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}}
	cases := map[string]struct {
		ports map[string]any
		want  string
	}{
		"output": {map[string]any{"inputs": []component.PortDefinition{watch}, "outputs": []component.PortDefinition{
			{Name: "out", Config: component.KVWritePort{Bucket: graph.BucketCommunityIndex}}}}, "no output ports"},
		"other bucket": {map[string]any{"inputs": []component.PortDefinition{
			{Name: "w", Config: component.KVWatchPort{Bucket: graph.BucketCommunityIndex}}}}, `"w"`},
		"history": {map[string]any{"inputs": []component.PortDefinition{
			{Name: "w", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates, History: true}}}}, `"w"`},
		"second input": {map[string]any{"inputs": []component.PortDefinition{watch,
			{Name: "entity_states", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}}}}, "exactly one input"},
		"none": {map[string]any{"inputs": []component.PortDefinition{}}, "exactly one input"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"ports": tc.ports})
			if err != nil {
				t.Fatalf("marshal ports: %v", err)
			}
			requireRefused(t, string(raw), tc.want)
		})
	}
}
