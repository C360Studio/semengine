package graphembedding

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// createFromJSON builds graph-embedding through CreateGraphEmbedding from raw, as a
// composition root would, with a NATS client that is never connected: construction opens
// no connection.
func createFromJSON(t *testing.T, raw string) (*Component, error) {
	t.Helper()
	return createWithDeps(t, raw, component.Dependencies{})
}

// createWithDeps is createFromJSON with deps, whose NATS client it fills.
func createWithDeps(t *testing.T, raw string, deps component.Dependencies) (*Component, error) {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	deps.NATSClient = client
	built, err := CreateGraphEmbedding(json.RawMessage(raw), deps)
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

// mustCreate is createFromJSON that fails the test on a refusal.
func mustCreate(t *testing.T, raw string) *Component {
	t.Helper()
	c, err := createFromJSON(t, raw)
	if err != nil {
		t.Fatalf("CreateGraphEmbedding(%s): %v", raw, err)
	}
	return c
}

// TestConfigRefusesUnknownKey: a configuration carrying a key graph-embedding does not
// define is refused at construction and by port declaration, naming the key; the same
// configuration without it builds (graph-embedding spec, "A removed key"). batch_size is
// a key SemSource sends that nothing here reads; cache_ttl is a cache setting with no reader;
// "worker" is a misspelling.
func TestConfigRefusesUnknownKey(t *testing.T) {
	raw, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	mustCreate(t, string(raw))

	for key, value := range map[string]any{"batch_size": 50, "cache_ttl": "1h", "worker": 2} {
		t.Run(key, func(t *testing.T) {
			raw := defaultConfigWith(t, key, value)
			_, err := createFromJSON(t, raw)
			if err == nil {
				t.Fatalf("CreateGraphEmbedding accepted a config with the unknown key %s", key)
			}
			if !errs.IsInvalid(err) || !strings.Contains(err.Error(), `"`+key+`"`) {
				t.Fatalf("CreateGraphEmbedding error is not an invalid config naming %s: %v", key, err)
			}
			if _, err := DeclarePorts(json.RawMessage(raw), key); err == nil {
				t.Fatalf("DeclarePorts accepted a config with the unknown key %s", key)
			}
		})
	}
}

// TestConfigRefusesDataAfterTheObject: strict decoding keeps json.Unmarshal's refusal of
// anything after the configuration object, and of malformed JSON.
func TestConfigRefusesDataAfterTheObject(t *testing.T) {
	for _, raw := range []string{`{"workers": 2} {"workers": 3}`, `{invalid json}`} {
		if _, err := createFromJSON(t, raw); err == nil {
			t.Fatalf("CreateGraphEmbedding accepted %s", raw)
		}
	}
}

// TestConfigRefusesHTTPEmbedder: at tier 0 the only embedder is bm25; http, a
// provider-backed embedder, and any other value are refused at construction naming the
// value and tier 0 (graph-embedding spec, "The HTTP embedder at tier 0").
func TestConfigRefusesHTTPEmbedder(t *testing.T) {
	for _, value := range []string{"http", "openai"} {
		t.Run(value, func(t *testing.T) {
			raw := defaultConfigWith(t, "embedder_type", value)
			_, err := createFromJSON(t, raw)
			if err == nil {
				t.Fatalf("CreateGraphEmbedding accepted embedder_type %q at tier 0", value)
			}
			if !errs.IsInvalid(err) || !strings.Contains(err.Error(), `"`+value+`"`) ||
				!strings.Contains(err.Error(), "tier 0") {
				t.Fatalf("CreateGraphEmbedding error is not an invalid config naming %q and tier 0: %v", value, err)
			}
			if _, err := DeclarePorts(json.RawMessage(raw), "graph-embedding"); err == nil {
				t.Fatalf("DeclarePorts accepted embedder_type %q", value)
			}
		})
	}
}

// TestConfigEmbedderType: an absent embedder_type is bm25, and bm25 is kept.
func TestConfigEmbedderType(t *testing.T) {
	if c := mustCreate(t, `{}`); c.config.EmbedderType != "bm25" {
		t.Fatalf("absent embedder_type = %q, want bm25", c.config.EmbedderType)
	}
	if c := mustCreate(t, `{"embedder_type": "bm25"}`); c.config.EmbedderType != "bm25" {
		t.Fatalf("embedder_type bm25 = %q", c.config.EmbedderType)
	}
}

// TestConfigWorkers: workers is the embedding worker count; absent or 0 is 5, the Worker's
// own default; a set value is kept; a negative one is refused.
func TestConfigWorkers(t *testing.T) {
	for raw, want := range map[string]int{`{}`: 5, `{"workers": 0}`: 5, `{"workers": 3}`: 3} {
		if c := mustCreate(t, raw); c.config.Workers != want {
			t.Fatalf("CreateGraphEmbedding(%s) workers = %d, want %d", raw, c.config.Workers, want)
		}
	}
	if _, err := createFromJSON(t, `{"workers": -1}`); !errs.IsInvalid(err) {
		t.Fatalf("CreateGraphEmbedding with workers -1 = %v, want an invalid config", err)
	}
}

// TestConfigMaxTextLen: max_text_len caps the text embedded; absent or 0 is bm25's 4,000,
// resolved at construction so every reader sees one value; a set value up to
// embedding.MaxSourceTextLenCeiling is kept; a negative value or one past the ceiling is
// refused.
func TestConfigMaxTextLen(t *testing.T) {
	for raw, want := range map[string]int{
		`{}`:                        4000,
		`{"max_text_len": 0}`:       4000,
		`{"max_text_len": 1200}`:    1200,
		`{"max_text_len": 1000000}`: 1_000_000,
	} {
		if c := mustCreate(t, raw); c.config.MaxTextLen != want {
			t.Fatalf("CreateGraphEmbedding(%s) max_text_len = %d, want %d", raw, c.config.MaxTextLen, want)
		}
	}
	if embedding.MaxSourceTextLenCeiling != 1_000_000 {
		t.Fatalf("ceiling moved to %d; update this test's boundary cases", embedding.MaxSourceTextLenCeiling)
	}
	for _, raw := range []string{`{"max_text_len": -1}`, `{"max_text_len": 1000001}`} {
		if _, err := createFromJSON(t, raw); !errs.IsInvalid(err) {
			t.Fatalf("CreateGraphEmbedding(%s) = %v, want an invalid config", raw, err)
		}
	}
}

// TestConfigTextSuffixes: text_suffixes names the predicate suffixes whose values are
// embedded; absent or empty is the nine defaults, resolved at construction; a set list is
// kept as written.
func TestConfigTextSuffixes(t *testing.T) {
	pinDefaults := []string{".title", ".content", ".description", ".summary", ".text", ".name", ".body",
		".abstract", ".subject"}
	for _, raw := range []string{`{}`, `{"text_suffixes": []}`} {
		if c := mustCreate(t, raw); !reflect.DeepEqual(c.config.TextSuffixes, pinDefaults) {
			t.Fatalf("CreateGraphEmbedding(%s) text_suffixes = %q, want %q", raw, c.config.TextSuffixes, pinDefaults)
		}
	}
	c := mustCreate(t, `{"text_suffixes": [".source_code", ".signature"]}`)
	if want := []string{".source_code", ".signature"}; !reflect.DeepEqual(c.config.TextSuffixes, want) {
		t.Fatalf("text_suffixes = %q, want %q", c.config.TextSuffixes, want)
	}
}

// TestConfigStartupWait: the wait for ENTITY_STATES defaults to 30 attempts 500 ms apart,
// and set values are kept.
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
}

// TestConfigCoalesceMs: coalescing is off by default, and a set window is kept.
func TestConfigCoalesceMs(t *testing.T) {
	if c := mustCreate(t, `{}`); c.config.CoalesceMs != 0 {
		t.Fatalf("default coalesce_ms = %d, want 0", c.config.CoalesceMs)
	}
	if c := mustCreate(t, `{"coalesce_ms": 25}`); c.config.CoalesceMs != 25 {
		t.Fatalf("coalesce_ms = %d, want 25", c.config.CoalesceMs)
	}
}

// entityWatch is the one ENTITY_STATES input the follower reads.
func entityWatch() component.PortDefinition {
	return component.PortDefinition{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}}
}

// TestConfigWithoutPortsTakesDefaultPorts: a configuration with no ports block takes the
// default inputs, the ENTITY_STATES watch and the MESSAGES store read, and no outputs.
func TestConfigWithoutPortsTakesDefaultPorts(t *testing.T) {
	c := mustCreate(t, `{}`)
	inputs := c.InputPorts()
	if len(inputs) != 2 {
		t.Fatalf("inputs = %+v, want the entity watch and the content store", inputs)
	}
	if watch, ok := inputs[0].Config.(component.KVWatchPort); !ok || watch.Bucket != graph.BucketEntityStates ||
		inputs[0].Direction != component.DirectionInput {
		t.Fatalf("first input %+v is not a kv-watch input on %s", inputs[0], graph.BucketEntityStates)
	}
	if read, ok := inputs[1].Config.(component.StoreReadPort); !ok || read.Bucket != "MESSAGES" {
		t.Fatalf("second input %+v is not a store read of MESSAGES", inputs[1])
	}
	if outputs := c.OutputPorts(); len(outputs) != 0 {
		t.Fatalf("outputs = %+v, want none", outputs)
	}
	inputs[0].Name = "changed"
	if c.InputPorts()[0].Name != "entity_watch" {
		t.Fatal("changing the returned inputs changed the component's ports")
	}
}

// TestConfigPresentPortsUsedAsWritten: a ports block that is present is used as written,
// never merged with the defaults: one input is one input, and an empty input list is
// refused rather than filled in.
func TestConfigPresentPortsUsedAsWritten(t *testing.T) {
	raw := defaultConfigWith(t, "ports", map[string]any{"inputs": []component.PortDefinition{entityWatch()}})
	if inputs := mustCreate(t, raw).InputPorts(); len(inputs) != 1 || inputs[0].Name != "entity_watch" {
		t.Fatalf("CreateGraphEmbedding(%s) inputs = %+v, want the one declared", raw, inputs)
	}
	raw = defaultConfigWith(t, "ports", map[string]any{"inputs": []component.PortDefinition{}})
	if c, err := createFromJSON(t, raw); err == nil {
		t.Fatalf("CreateGraphEmbedding(%s) built with inputs %+v, want an empty input list refused", raw, c.InputPorts())
	}
}

// TestConfigRefusesOutputs: graph-embedding writes its buckets itself and declares no
// output ports; a declared output is refused.
func TestConfigRefusesOutputs(t *testing.T) {
	raw := defaultConfigWith(t, "ports", map[string]any{
		"inputs":  []component.PortDefinition{entityWatch()},
		"outputs": []component.PortDefinition{{Name: "embeddings", Config: component.KVWritePort{Bucket: "EMBEDDINGS_CACHE"}}},
	})
	if _, err := createFromJSON(t, raw); !errs.IsInvalid(err) || !strings.Contains(err.Error(), "no output ports") {
		t.Fatalf("CreateGraphEmbedding(%s) = %v, want an invalid config saying it declares no output ports", raw, err)
	}
}

// TestConfigRefusesInputsTheFollowerDoesNotRead: an input is the follower's full watch of
// ENTITY_STATES or a store read for offloaded bodies; anything else is refused naming the
// port, so a composition never advertises a feed graph-embedding does not read, as
// graph-index refuses one. A configuration with no ENTITY_STATES watch is refused too.
func TestConfigRefusesInputsTheFollowerDoesNotRead(t *testing.T) {
	cases := map[string]component.Portable{
		"another bucket": component.KVWatchPort{Bucket: "OTHER_STATES"},
		"a key filter":   component.KVWatchPort{Bucket: graph.BucketEntityStates, Keys: []string{"c360.*.>"}},
		"history":        component.KVWatchPort{Bucket: graph.BucketEntityStates, History: true},
		"a KV read":      component.KVReadPort{Bucket: graph.BucketEntityStates},
		"a NATS subject": component.NATSPort{Subject: "graph.ingest.events"},
	}
	for name, portConfig := range cases {
		t.Run(name, func(t *testing.T) {
			inputs := []component.PortDefinition{entityWatch(), {Name: "unread_feed", Config: portConfig}}
			raw := defaultConfigWith(t, "ports", map[string]any{"inputs": inputs})
			if c, err := createFromJSON(t, raw); err == nil {
				t.Fatalf("CreateGraphEmbedding(%s) built advertising %+v", raw, c.InputPorts())
			} else if !errs.IsInvalid(err) || !strings.Contains(err.Error(), `"unread_feed"`) {
				t.Fatalf("CreateGraphEmbedding(%s) = %v, want an invalid config naming the port", raw, err)
			}
			if _, err := DeclarePorts(json.RawMessage(raw), ""); err == nil {
				t.Fatalf("DeclarePorts(%s) accepted the input", raw)
			}
		})
	}

	storeOnly := []component.PortDefinition{{Name: "content_store", Config: component.StoreReadPort{Bucket: "MESSAGES"}}}
	raw := defaultConfigWith(t, "ports", map[string]any{"inputs": storeOnly})
	if _, err := createFromJSON(t, raw); !errs.IsInvalid(err) || !strings.Contains(err.Error(), graph.BucketEntityStates) {
		t.Fatalf("CreateGraphEmbedding(%s) = %v, want an invalid config naming the missing %s watch",
			raw, err, graph.BucketEntityStates)
	}
}

// TestStoreReadInputAdmitsStoreRegistry: the injected store registry, through which
// offloaded bodies are read (ADR-063), is kept only when a store-read input is declared;
// without one the component holds none, and no store-read port is invented.
func TestStoreReadInputAdmitsStoreRegistry(t *testing.T) {
	registry := storeregistry.New()
	c, err := createWithDeps(t, `{}`, component.Dependencies{StoreRegistry: registry})
	if err != nil {
		t.Fatalf("CreateGraphEmbedding({}): %v", err)
	}
	if c.storeRegistry != registry {
		t.Fatal("the default config declares a store read, but the store registry was not kept")
	}

	raw := defaultConfigWith(t, "ports", map[string]any{"inputs": []component.PortDefinition{entityWatch()}})
	c, err = createWithDeps(t, raw, component.Dependencies{StoreRegistry: registry})
	if err != nil {
		t.Fatalf("CreateGraphEmbedding(%s): %v", raw, err)
	}
	if c.storeRegistry != nil {
		t.Fatal("the store registry was kept without a store-read input")
	}
	for _, port := range c.InputPorts() {
		if _, ok := port.Config.(component.StoreReadPort); ok {
			t.Fatalf("a store-read input %q was invented", port.Name)
		}
	}
}

// FuzzResolveConfig: the factory and DeclarePorts take operator bytes. Whatever they are,
// resolving them does not panic, and whatever is accepted is tier-0 and reads only what
// graph-embedding reads: embedder bm25, at least one worker, a text cap from 1 to the
// ceiling, a non-empty suffix list, no outputs, and inputs that are full ENTITY_STATES
// watches (at least one) or store reads. CreateGraphEmbedding and DeclarePorts agree on
// every input. The seeds cover each class: empty, the defaults, each kept key, each
// refusal (an unknown key, trailing data, http, a negative or oversized value, an output,
// an unread input, no watch), not JSON.
func FuzzResolveConfig(f *testing.F) {
	for _, seed := range []string{
		``,
		`{}`,
		`null`,
		`{"embedder_type":"bm25","workers":3,"max_text_len":1200,"text_suffixes":[".code"],"startup_attempts":2,"startup_interval_ms":10,"coalesce_ms":5}`,
		`{"ports":{"inputs":[{"name":"w","config":{"kind":"kv-watch","bucket":"ENTITY_STATES"}}]}}`,
		`{"batch_size":50}`,
		`{} {}`,
		`{"embedder_type":"http"}`,
		`{"workers":-1}`,
		`{"max_text_len":1000001}`,
		`{"ports":{"inputs":[{"name":"w","config":{"kind":"kv-watch","bucket":"ENTITY_STATES"}}],"outputs":[{"name":"o","config":{"kind":"kv-write","bucket":"X"}}]}}`,
		`{"ports":{"inputs":[{"name":"w","config":{"kind":"kv-watch","bucket":"ENTITY_STATES","history":true}}]}}`,
		`{"ports":{"inputs":[{"name":"s","config":{"kind":"store-read","bucket":"MESSAGES"}}]}}`,
		`{`,
		`[]`,
	} {
		f.Add([]byte(seed))
	}
	client, err := natsclient.NewClient("")
	if err != nil {
		f.Fatalf("natsclient.NewClient: %v", err)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		config, inputs, outputs, err := resolveConfig(raw)
		_, declareErr := DeclarePorts(raw, "graph-embedding")
		_, createErr := CreateGraphEmbedding(raw, component.Dependencies{NATSClient: client})
		if (err == nil) != (declareErr == nil) || (err == nil) != (createErr == nil) {
			t.Fatalf("resolve %v, DeclarePorts %v, CreateGraphEmbedding %v disagree on %q", err, declareErr, createErr, raw)
		}
		if err != nil {
			if !errs.IsInvalid(err) {
				t.Fatalf("resolveConfig(%q) refused with an error not classified invalid: %v", raw, err)
			}
			return
		}
		if config.EmbedderType != "bm25" || config.Workers < 1 || config.MaxTextLen < 1 ||
			config.MaxTextLen > embedding.MaxSourceTextLenCeiling || len(config.TextSuffixes) == 0 {
			t.Fatalf("resolveConfig(%q) accepted %+v", raw, config)
		}
		if len(outputs) != 0 {
			t.Fatalf("resolveConfig(%q) accepted outputs %+v", raw, outputs)
		}
		watched := false
		for _, input := range inputs {
			switch port := input.Config.(type) {
			case component.KVWatchPort:
				if port.Bucket != graph.BucketEntityStates || len(port.Keys) != 0 || port.History {
					t.Fatalf("resolveConfig(%q) accepted input %+v", raw, input)
				}
				watched = true
			case component.StoreReadPort:
			default:
				t.Fatalf("resolveConfig(%q) accepted input %+v", raw, input)
			}
		}
		if !watched {
			t.Fatalf("resolveConfig(%q) accepted inputs %+v with no ENTITY_STATES watch", raw, inputs)
		}
	})
}
