package graphgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
)

// requiredPort is the one output a configuration may name, written out as an operator writes it.
// It is spelled here rather than built from DefaultConfig so that a change to the default fails
// these tests.
const requiredPort = `{"name":"graph_queries","required":true,"config":{"kind":"nats-request",` +
	`"subject":"graph.query.*","interface":{"type":"graph.query","version":"v1"}}}`

// createFromJSON builds graph-gateway through CreateGraphGateway from raw, as a composition root
// would, with a NATS client that is never connected: construction opens no connection.
func createFromJSON(t *testing.T, raw string) (*Component, error) {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphGateway(json.RawMessage(raw), component.Dependencies{NATSClient: client})
	if err != nil {
		return nil, err
	}
	return built.(*Component), nil
}

// requireRefusedNaming fails unless both construction and DeclarePorts refuse raw with an error
// holding want.
func requireRefusedNaming(t *testing.T, raw, want string) {
	t.Helper()
	if _, err := createFromJSON(t, raw); err == nil {
		t.Errorf("CreateGraphGateway accepted %s", raw)
	} else if !strings.Contains(err.Error(), want) {
		t.Errorf("CreateGraphGateway(%s) error does not name %s: %v", raw, want, err)
	}
	if _, err := DeclarePorts(json.RawMessage(raw), key); err == nil {
		t.Errorf("DeclarePorts accepted %s", raw)
	} else if !strings.Contains(err.Error(), want) {
		t.Errorf("DeclarePorts(%s) error does not name %s: %v", raw, want, err)
	}
}

// TestConfigRefusesRemovedKeys: every key the pin's Config read (pin gateway/graph-gateway/component.go:73-84)
// but ports is refused at construction and by DeclarePorts, naming the key, with and without a ports
// block, as is a key no version defined.
func TestConfigRefusesRemovedKeys(t *testing.T) {
	for _, entry := range []struct{ key, value string }{
		{"graphql_path", `"/graphql"`},
		{"mcp_path", `"/mcp"`},
		{"bind_address", `":8080"`},
		{"standalone_server", `false`},
		{"enable_playground", `true`},
		{"enable_inference_api", `false`},
		{"query_timeout", `"30s"`},
		{"domain_examples_path", `""`},
		{"enable_embedding_classifier", `false`},
		{"readiness_keys", `[]`},
		{"readiness_path", `"/readiness"`},
		{"no_such_key", `1`},
	} {
		field := `"` + entry.key + `":` + entry.value
		for name, raw := range map[string]string{
			"without ports": `{` + field + `}`,
			"with ports":    `{"ports":{"outputs":[` + requiredPort + `]},` + field + `}`,
		} {
			t.Run(entry.key+"/"+name, func(t *testing.T) {
				requireRefusedNaming(t, raw, `"`+entry.key+`"`)
			})
		}
	}
}

// TestConfigRefusesExtraPorts: the one output is the required graph_queries nats-request port on
// graph.query.*, graph.query v1; an input, a second output, no output, or one that says anything
// else under its name, required flag or config is refused.
func TestConfigRefusesExtraPorts(t *testing.T) {
	other := `{"name":"other","required":true,"config":{"kind":"nats-request","subject":"graph.query.*",` +
		`"interface":{"type":"graph.query","version":"v1"}}}`
	for name, ports := range map[string]string{
		"an input":             `{"inputs":[` + requiredPort + `],"outputs":[` + requiredPort + `]}`,
		"a second output":      `{"outputs":[` + requiredPort + `,` + other + `]}`,
		"no output":            `{"outputs":[]}`,
		"another name":         `{"outputs":[` + other + `]}`,
		"not required":         `{"outputs":[` + strings.Replace(requiredPort, `"required":true`, `"required":false`, 1) + `]}`,
		"another subject":      `{"outputs":[` + strings.Replace(requiredPort, `graph.query.*`, `graph.index.query.*`, 1) + `]}`,
		"another interface":    `{"outputs":[` + strings.Replace(requiredPort, `"v1"`, `"v2"`, 1) + `]}`,
		"no interface":         `{"outputs":[` + strings.Replace(requiredPort, `,"interface":{"type":"graph.query","version":"v1"}`, ``, 1) + `]}`,
		"a timeout of its own": `{"outputs":[` + strings.Replace(requiredPort, `"subject"`, `"timeout":"5s","subject"`, 1) + `]}`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := `{"ports":` + ports + `}`
			if _, err := createFromJSON(t, raw); err == nil {
				t.Errorf("CreateGraphGateway accepted %s", raw)
			}
			if _, err := DeclarePorts(json.RawMessage(raw), key); err == nil {
				t.Errorf("DeclarePorts accepted %s", raw)
			}
		})
	}
	if _, err := createFromJSON(t, `{"ports":{"outputs":[`+requiredPort+`]}}`); err != nil {
		t.Errorf("the required port written out is refused: %v", err)
	}
}

// TestConfigTimeoutDefaultAndRange: query_timeout_ms defaults to the pin's 60000 (design D5), zero
// means that default, a value is kept, and a negative one or one past a duration is refused naming
// the key.
func TestConfigTimeoutDefaultAndRange(t *testing.T) {
	for raw, want := range map[string]int{
		``:                        60000,
		`{}`:                      60000,
		`{"query_timeout_ms":0}`:  60000,
		`{"query_timeout_ms":50}`: 50,
	} {
		c, err := createFromJSON(t, raw)
		if err != nil {
			t.Errorf("CreateGraphGateway(%q): %v", raw, err)
			continue
		}
		if c.config.QueryTimeoutMs != want {
			t.Errorf("CreateGraphGateway(%q) timeout = %d, want %d", raw, c.config.QueryTimeoutMs, want)
		}
	}
	requireRefusedNaming(t, `{"query_timeout_ms":-1}`, "query_timeout_ms")
	requireRefusedNaming(t, `{"query_timeout_ms":9223372036855}`, "query_timeout_ms")
	if _, err := createFromJSON(t, `{} {}`); err == nil {
		t.Error("CreateGraphGateway accepted data after the configuration object")
	}
}

// TestCreateGraphGatewayRequiresNATSClient: a missing NATS client is a construction error.
func TestCreateGraphGatewayRequiresNATSClient(t *testing.T) {
	if built, err := CreateGraphGateway(json.RawMessage(`{}`), component.Dependencies{}); err == nil {
		t.Fatalf("CreateGraphGateway without a NATS client built %T", built)
	}
}

// TestRegisterAddsGraphGateway: Register adds the factory, whose schema lists exactly the two keys
// and whose port declarer reports the ports the factory builds. Importing the package registers
// nothing.
func TestRegisterAddsGraphGateway(t *testing.T) {
	registry := component.NewRegistry()
	if len(registry.ListFactories()) != 0 {
		t.Fatalf("a new registry lists %v", registry.ListFactories())
	}
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	registration, ok := registry.ListFactories()["graph-gateway"]
	if !ok {
		t.Fatal("Register did not add graph-gateway")
	}
	if got := len(registration.Schema.Properties); got != 2 {
		t.Errorf("schema properties = %v, want ports and query_timeout_ms", registration.Schema.Properties)
	}
	for _, name := range []string{"ports", "query_timeout_ms"} {
		if _, ok := registration.Schema.Properties[name]; !ok {
			t.Errorf("schema has no %q property", name)
		}
	}
	declared, err := registration.Ports(json.RawMessage(`{}`), "graph-gateway")
	if err != nil {
		t.Fatalf("declare ports: %v", err)
	}
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphGateway({}): %v", err)
	}
	if want := component.PortConfigFrom(c.InputPorts(), c.OutputPorts()); !reflect.DeepEqual(declared, want) {
		t.Fatalf("declared ports %+v differ from the built component's %+v", declared, want)
	}
	if len(declared.Inputs) != 0 || len(declared.Outputs) != 1 {
		t.Fatalf("declared ports %+v, want no input and one output", declared)
	}
}

// serve answers one request through the route RegisterHTTPHandlers mounts at /graph-gateway, as the
// service manager mounts it.
func serve(t *testing.T, c *Component, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("/graph-gateway", mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, "/graph-gateway/graphql", strings.NewReader(body)))
	return rec
}

// TestRefusedOutsideStartStop: before Start, after a Stop before Start, and once Stop has run, every
// request is 503 graph-gateway is stopping and reaches no resolver; while running a request is
// served (a GET is answered 405, which needs no NATS).
func TestRefusedOutsideStartStop(t *testing.T) {
	newGateway := func(t *testing.T) *Component {
		t.Helper()
		c, err := createFromJSON(t, `{}`)
		if err != nil {
			t.Fatalf("CreateGraphGateway: %v", err)
		}
		if err := c.Initialize(); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		return c
	}
	query := `{"query":"{ entity(id: \"c360.test.query.system.drone.001\") { kvRevision } }"}`
	requireStopping := func(t *testing.T, c *Component, when string) {
		t.Helper()
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			rec := serve(t, c, method, query)
			if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "graph-gateway is stopping") {
				t.Errorf("%s %s: %d %q, want 503 graph-gateway is stopping", when, method, rec.Code, rec.Body)
			}
		}
	}

	t.Run("before Start", func(t *testing.T) {
		requireStopping(t, newGateway(t), "before Start")
	})
	t.Run("Stop before Start", func(t *testing.T) {
		c := newGateway(t)
		if err := c.Stop(t.Context()); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		requireStopping(t, c, "after a Stop before Start")
	})
	t.Run("running, then stopped", func(t *testing.T) {
		c := newGateway(t)
		if err := c.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		rec := serve(t, c, http.MethodGet, "")
		if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != http.MethodPost {
			t.Errorf("GET while running: %d, Allow %q, want 405 Allow POST", rec.Code, rec.Header().Get("Allow"))
		}
		var body struct {
			Errors []struct{ Message string } `json:"errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Errors) != 1 {
			t.Errorf("GET while running: body %q, want one GraphQL error (%v)", rec.Body, err)
		}
		if err := c.Stop(t.Context()); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		requireStopping(t, c, "after Stop")
	})
}

// TestRequestBodyRefused: a body that is not one GraphQL request object is HTTP 400 with an errors
// entry and no data.
func TestRequestBodyRefused(t *testing.T) {
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphGateway: %v", err)
	}
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	for _, body := range []string{``, `not json`, `{"query":"{ __typename }"} {}`, `{"query":1}`} {
		rec := serve(t, c, http.MethodPost, body)
		var got map[string]any
		if rec.Code != http.StatusBadRequest || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got["errors"] == nil || got["data"] != nil {
			t.Errorf("body %q: %d %q, want 400 with errors and no data", body, rec.Code, rec.Body)
		}
	}
}
