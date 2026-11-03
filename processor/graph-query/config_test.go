package graphquery

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
)

// requiredPort is the one input a configuration may name, written out as an operator writes it.
// It is spelled here rather than built from DefaultConfig so that a change to the default fails
// these tests.
const requiredPort = `{"name":"graph_queries","required":true,"config":{"kind":"nats-request",` +
	`"subject":"graph.query.*","interface":{"type":"graph.query","version":"v1"}}}`

// createFromJSON builds graph-query through CreateGraphQuery from raw, as a composition root would,
// with a NATS client that is never connected: construction opens no connection.
func createFromJSON(t *testing.T, raw string) (*Component, error) {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphQuery(json.RawMessage(raw), component.Dependencies{NATSClient: client})
	if err != nil {
		return nil, err
	}
	return built.(*Component), nil
}

// TestUnknownConfigKeyIsRefused: a key Config does not define is refused at construction and by
// DeclarePorts, naming the key, with and without a ports block. The list holds what the pin
// accepted and read, what it accepted and ignored (P2; MIGRATION.md's deployed keys), and
// the old spelling of the timeout, whose advertised "5s" the pin refused for another reason (P1).
func TestUnknownConfigKeyIsRefused(t *testing.T) {
	for _, entry := range []struct{ key, value string }{
		{"no_such_key", `1`},
		{"max_traversal_depth", `5`},
		{"max_traversal_nodes", `100`},
		{"request_timeout", `"5s"`},
		{"query_timeout", `"5s"`},
		{"query_timeout", `5000000000`},
		{"max_depth", `10`},
		{"recheck_interval", `5000000000`},
		{"min_text_relevance", `0.3`},
	} {
		field := `"` + entry.key + `":` + entry.value
		for name, raw := range map[string]string{
			"without ports": `{` + field + `}`,
			"with ports":    `{"ports":{"inputs":[` + requiredPort + `]},` + field + `}`,
		} {
			t.Run(entry.key+"="+entry.value+"/"+name, func(t *testing.T) {
				if _, err := createFromJSON(t, raw); err == nil {
					t.Fatalf("CreateGraphQuery accepted %s", raw)
				} else if !strings.Contains(err.Error(), `"`+entry.key+`"`) {
					t.Fatalf("CreateGraphQuery error does not name %s: %v", entry.key, err)
				}
				if _, err := DeclarePorts(json.RawMessage(raw), "graph-query"); err == nil {
					t.Fatalf("DeclarePorts accepted %s", raw)
				} else if !strings.Contains(err.Error(), `"`+entry.key+`"`) {
					t.Fatalf("DeclarePorts error does not name %s: %v", entry.key, err)
				}
			})
		}
	}
}

// TestConfigRefusesDataAfterTheObject: strict decoding keeps json.Unmarshal's refusal of anything
// after the configuration object.
func TestConfigRefusesDataAfterTheObject(t *testing.T) {
	for _, raw := range []string{`{} {}`, `{}x`, `{"query_timeout_ms":1}{"query_timeout_ms":2}`} {
		if _, err := createFromJSON(t, raw); err == nil {
			t.Errorf("CreateGraphQuery accepted %q", raw)
		}
	}
}

// TestConfigTimeout: query_timeout_ms defaults to 15000 (design D7), zero means that default as it
// does for graph-index's optional numbers, a value is kept, and a negative one is refused naming
// the key.
func TestConfigTimeout(t *testing.T) {
	for raw, want := range map[string]int{
		``:                         15000,
		`{}`:                       15000,
		`{"query_timeout_ms":0}`:   15000,
		`{"query_timeout_ms":250}`: 250,
	} {
		c, err := createFromJSON(t, raw)
		if err != nil {
			t.Errorf("CreateGraphQuery(%q): %v", raw, err)
			continue
		}
		if c.config.QueryTimeoutMs != want {
			t.Errorf("CreateGraphQuery(%q) timeout = %d, want %d", raw, c.config.QueryTimeoutMs, want)
		}
	}

	_, err := createFromJSON(t, `{"query_timeout_ms":-1}`)
	if err == nil {
		t.Fatal(`CreateGraphQuery accepted {"query_timeout_ms":-1}`)
	}
	if !strings.Contains(err.Error(), "query_timeout_ms") {
		t.Fatalf("negative timeout error does not name the key: %v", err)
	}
	if _, err := createFromJSON(t, `{"query_timeout_ms":"5s"}`); err == nil {
		t.Fatal(`CreateGraphQuery accepted {"query_timeout_ms":"5s"}`)
	}
}

// TestConfigTimeoutFitsADuration: the largest accepted value is the last millisecond count a
// time.Duration holds, 2^63-1 nanoseconds, written out here so the test does not take it from the
// code. One more would wrap negative when the value is read as a duration, so it is refused, naming
// the key.
func TestConfigTimeoutFitsADuration(t *testing.T) {
	c, err := createFromJSON(t, `{"query_timeout_ms":9223372036854}`)
	if err != nil {
		t.Fatalf("the largest timeout: %v", err)
	}
	if d := time.Duration(c.config.QueryTimeoutMs) * time.Millisecond; d <= 0 {
		t.Fatalf("the largest accepted timeout reads as the duration %v", d)
	}
	for _, raw := range []string{`{"query_timeout_ms":9223372036855}`, `{"query_timeout_ms":9223372036854775807}`} {
		_, err := createFromJSON(t, raw)
		if err == nil {
			t.Errorf("CreateGraphQuery accepted %s", raw)
		} else if !strings.Contains(err.Error(), "query_timeout_ms") {
			t.Errorf("CreateGraphQuery(%s) error does not name the key: %v", raw, err)
		}
	}
}

// TestConfigWithoutPortsTakesTheRequiredPort: no ports block means one required nats-request input
// named graph_queries on graph.query.*, carrying the graph.query v1 contract, and no outputs. The
// same port written out by hand is accepted and is the same port.
func TestConfigWithoutPortsTakesTheRequiredPort(t *testing.T) {
	for _, raw := range []string{``, `{}`, `{"ports":null}`, `{"ports":{"inputs":[` + requiredPort + `]}}`,
		`{"ports":{"inputs":[` + requiredPort + `],"outputs":[]}}`} {
		c, err := createFromJSON(t, raw)
		if err != nil {
			t.Errorf("CreateGraphQuery(%q): %v", raw, err)
			continue
		}
		if outputs := c.OutputPorts(); len(outputs) != 0 {
			t.Errorf("CreateGraphQuery(%q) outputs = %+v, want none", raw, outputs)
		}
		inputs := c.InputPorts()
		if len(inputs) != 1 {
			t.Errorf("CreateGraphQuery(%q) inputs = %+v, want one", raw, inputs)
			continue
		}
		port := inputs[0]
		if port.Name != "graph_queries" || !port.Required || port.Direction != component.DirectionInput {
			t.Errorf("CreateGraphQuery(%q) input = %+v, want the required graph_queries input", raw, port)
		}
		request, ok := port.Config.(component.NATSRequestPort)
		if !ok {
			t.Errorf("CreateGraphQuery(%q) input config = %T, want NATSRequestPort", raw, port.Config)
			continue
		}
		if request.Subject != "graph.query.*" {
			t.Errorf("CreateGraphQuery(%q) subject = %q", raw, request.Subject)
		}
		if contract := request.Interface; contract == nil || contract.Type != "graph.query" || contract.Version != "v1" {
			t.Errorf("CreateGraphQuery(%q) interface = %+v, want graph.query v1", raw, contract)
		}
	}
}

// TestConfigRefusesEveryOtherInputShape: a ports block is used as written, never merged with the
// defaults, and anything but the one required port is refused. graph-query answers only on its
// verbs, so a port that says otherwise advertises something nothing serves. The first two cases are
// the input forms the pin's doc.go advertised and its Validate refused (P3).
func TestConfigRefusesEveryOtherInputShape(t *testing.T) {
	const contract = `"interface":{"type":"graph.query","version":"v1"}`
	entity := `{"name":"query_entity","config":{"kind":"nats-request","subject":"graph.query.entity"}}`
	for name, ports := range map[string]string{
		"the pin's doc.go, three ports": `{"inputs":[` + entity +
			`,{"name":"query_relationships","config":{"kind":"nats-request","subject":"graph.query.relationships"}}` +
			`,{"name":"query_path_search","config":{"kind":"nats-request","subject":"graph.query.pathSearch"}}]}`,
		"the pin's doc.go, one port": `{"inputs":[` + entity + `]}`,
		"empty ports block":          `{}`,
		"no inputs":                  `{"inputs":[]}`,
		"two ports":                  `{"inputs":[` + requiredPort + `,` + requiredPort + `]}`,
		"another name": `{"inputs":[{"name":"queries","required":true,"config":{"kind":"nats-request",` +
			`"subject":"graph.query.*",` + contract + `}}]}`,
		"not required": `{"inputs":[{"name":"graph_queries","config":{"kind":"nats-request",` +
			`"subject":"graph.query.*",` + contract + `}}]}`,
		"one verb's subject": `{"inputs":[{"name":"graph_queries","required":true,"config":{"kind":"nats-request",` +
			`"subject":"graph.query.entity",` + contract + `}}]}`,
		"another subject family": `{"inputs":[{"name":"graph_queries","required":true,"config":{"kind":"nats-request",` +
			`"subject":"graph.ingest.query.*",` + contract + `}}]}`,
		"another kind": `{"inputs":[{"name":"graph_queries","required":true,"config":{"kind":"nats",` +
			`"subject":"graph.query.*"}}]}`,
		"no interface": `{"inputs":[{"name":"graph_queries","required":true,"config":{"kind":"nats-request",` +
			`"subject":"graph.query.*"}}]}`,
		"another interface": `{"inputs":[{"name":"graph_queries","required":true,"config":{"kind":"nats-request",` +
			`"subject":"graph.query.*","interface":{"type":"graph.query","version":"v2"}}}]}`,
		"a timeout nothing reads": `{"inputs":[{"name":"graph_queries","required":true,"config":{"kind":"nats-request",` +
			`"subject":"graph.query.*","timeout":"5s",` + contract + `}}]}`,
		"retries nothing reads": `{"inputs":[{"name":"graph_queries","required":true,"config":{"kind":"nats-request",` +
			`"subject":"graph.query.*","retries":3,` + contract + `}}]}`,
		"an output": `{"inputs":[` + requiredPort + `],"outputs":[{"name":"out","config":{"kind":"nats",` +
			`"subject":"graph.query.out"}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := `{"ports":` + ports + `}`
			if _, err := createFromJSON(t, raw); err == nil {
				t.Errorf("CreateGraphQuery accepted %s", raw)
			}
			if _, err := DeclarePorts(json.RawMessage(raw), "graph-query"); err == nil {
				t.Errorf("DeclarePorts accepted %s", raw)
			}
		})
	}
}

// TestConfigKeepsWhatAnOperatorMayStateOfThePort: a description, and external (the operator's
// statement that requests come from outside the composition, ADR-100), change nothing graph-query
// serves, so the required port may carry them, and they are reported.
func TestConfigKeepsWhatAnOperatorMayStateOfThePort(t *testing.T) {
	c, err := createFromJSON(t, `{"ports":{"inputs":[{"name":"graph_queries","required":true,"external":true,`+
		`"description":"lookups from the gateway","config":{"kind":"nats-request","subject":"graph.query.*",`+
		`"interface":{"type":"graph.query","version":"v1"}}}]}}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	if port := c.InputPorts()[0]; !port.External || port.Description != "lookups from the gateway" {
		t.Fatalf("input = %+v, want the description and external kept", port)
	}
}

// TestDefaultConfigIsValid: DefaultConfig, written out and read back, is the configuration an empty
// one becomes.
func TestDefaultConfigIsValid(t *testing.T) {
	defaults := DefaultConfig()
	if err := defaults.Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate(): %v", err)
	}
	raw, err := json.Marshal(defaults)
	if err != nil {
		t.Fatalf("marshal DefaultConfig: %v", err)
	}
	c, err := createFromJSON(t, string(raw))
	if err != nil {
		t.Fatalf("CreateGraphQuery(%s): %v", raw, err)
	}
	if c.config.QueryTimeoutMs != 15000 {
		t.Fatalf("timeout = %d, want 15000", c.config.QueryTimeoutMs)
	}
}

// TestValidateRefusesHandBuiltConfigs: Validate only checks, so a Config built in Go without ports,
// or with a port that has no config, is refused and does not panic.
func TestValidateRefusesHandBuiltConfigs(t *testing.T) {
	for name, config := range map[string]Config{
		"no ports":         {},
		"port, no config":  {Ports: &component.PortConfig{Inputs: []component.PortDefinition{{Name: "graph_queries", Required: true}}}},
		"negative timeout": {Ports: DefaultConfig().Ports, QueryTimeoutMs: -1},
	} {
		if err := config.Validate(); err == nil {
			t.Errorf("%s: Validate returned nil", name)
		}
	}
}

// FuzzDeclarePorts: DeclarePorts takes operator bytes. Whatever they are it does not panic, and
// whatever it accepts is exactly the one input, every field compared but the description and
// external statement, which are the operator's, and no output. The seeds cover each class of the
// grammar: empty, the defaults, a timeout, the written-out port, and each refusal (an unknown key,
// trailing data, a negative or oversized timeout, another port, a port one config field off, not JSON).
func FuzzDeclarePorts(f *testing.F) {
	for _, seed := range []string{
		``,
		`{}`,
		`null`,
		`{"query_timeout_ms":250}`,
		`{"ports":{"inputs":[` + requiredPort + `]}}`,
		`{"no_such_key":1}`,
		`{} {}`,
		`{"query_timeout_ms":-1}`,
		`{"query_timeout_ms":"5s"}`,
		`{"query_timeout_ms":9223372036854775807}`,
		`{"ports":{"inputs":[` + strings.Replace(requiredPort, `"kind"`, `"timeout":"5s","kind"`, 1) + `]}}`,
		`{"ports":{"inputs":[` + strings.Replace(requiredPort, `"kind"`, `"retries":3,"kind"`, 1) + `]}}`,
		`{"ports":{"inputs":[` + strings.Replace(requiredPort, `"v1"`, `"v2"`, 1) + `]}}`,
		`{"ports":{"inputs":[{"name":"x","config":{"kind":"nats-request","subject":"graph.query.x"}}]}}`,
		`{"ports":{"outputs":[{"name":"x","config":{"kind":"nats","subject":"x"}}]}}`,
		`{`,
		`[]`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		declared, err := DeclarePorts(raw, "graph-query")
		if err != nil {
			return
		}
		if len(declared.Inputs) != 1 || len(declared.Outputs) != 0 {
			t.Fatalf("DeclarePorts(%q) accepted %+v", raw, declared)
		}
		input := declared.Inputs[0]
		want := component.PortDefinition{
			Name:        "graph_queries",
			Required:    true,
			Description: input.Description,
			External:    input.External,
			Config: component.NATSRequestPort{
				Subject:   "graph.query.*",
				Timeout:   "1s", // what resolving the port fills in when none is written
				Interface: &component.InterfaceContract{Type: "graph.query", Version: "v1"},
			},
		}
		if !reflect.DeepEqual(input, want) {
			t.Fatalf("DeclarePorts(%q) accepted input %+v, want %+v", raw, input, want)
		}
	})
}
