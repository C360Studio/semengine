package websocket

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// testDeps is the Dependencies a composition root would pass, with a NATS client that is never
// connected: construction opens no connection.
func testDeps(t *testing.T) component.Dependencies {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	return component.Dependencies{NATSClient: client}
}

// registration is the websocket Registration as Register adds it, so the tests reach the port
// declarer the way the component manager does. The registry lists no factory, so tests call create.
func registration(t *testing.T) component.Registration {
	t.Helper()
	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	reg, ok := registry.ListFactories()["websocket"]
	if !ok {
		t.Fatal("Register did not add websocket")
	}
	return *reg
}

// mustResolve resolves raw through the factory's own derivation and fails the test on a refusal.
func mustResolve(t *testing.T, raw string) settings {
	t.Helper()
	s, err := newSettings(json.RawMessage(raw), testDeps(t))
	if err != nil {
		t.Fatalf("newSettings(%s): %v", raw, err)
	}
	return s
}

// requireRefused fails the test unless the factory and the registered port declarer both refuse
// raw with an invalid-config error whose text holds every one of want.
func requireRefused(t *testing.T, deps component.Dependencies, raw string, want ...string) {
	t.Helper()
	reg := registration(t)
	built, err := create(json.RawMessage(raw), deps)
	if err == nil {
		t.Fatalf("factory accepted %s and built %T", raw, built)
	}
	if !errs.IsInvalid(err) {
		t.Fatalf("factory(%s) = %v; want an invalid-config error", raw, err)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Fatalf("factory(%s) = %v; want it to say %q", raw, err, w)
		}
	}
	if deps.NATSClient == nil {
		return // the declarer takes no dependencies, so a missing client is the factory's alone
	}
	if _, err := reg.Ports(json.RawMessage(raw), "websocket"); err == nil {
		t.Fatalf("port declarer accepted %s", raw)
	}
}

// withPorts is a configuration whose ports block holds exactly inputs and outputs.
func withPorts(t *testing.T, inputs, outputs []component.PortDefinition) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"ports": component.PortConfig{Inputs: inputs, Outputs: outputs}})
	if err != nil {
		t.Fatalf("marshal ports: %v", err)
	}
	return string(raw)
}

func entityWatch() component.PortDefinition {
	return component.PortDefinition{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}}
}

func server(port int) component.PortDefinition {
	return component.PortDefinition{
		Name:   "websocket_server",
		Config: component.NetworkPort{Protocol: "http", Host: "127.0.0.1", Port: port},
	}
}

// TestConfigRefusals: each configuration the component cannot honour is refused at construction
// and by the port declarer, naming the key or port at fault (design D1, D5; output-websocket spec,
// "One input per instance"). The keys, port names and words of the guidance are written out here,
// not read from the package's tables.
func TestConfigRefusals(t *testing.T) {
	deps := testDeps(t)
	// The baseline the cases perturb resolves.
	mustResolve(t, withPorts(t, []component.PortDefinition{entityWatch()}, []component.PortDefinition{server(9001)}))

	removed := []struct {
		key   string
		value any
		hint  string
	}{
		{"delivery_mode", "at-least-once", "at most once"},
		{"ack_timeout", "5s", "nothing is acknowledged"},
		{"passthrough", true, "unchanged"},
	}
	for _, tc := range removed {
		t.Run("removed "+tc.key, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{tc.key: tc.value})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			requireRefused(t, deps, string(raw), `"`+tc.key+`" was removed`, tc.hint)
		})
	}
	t.Run("every removed key named", func(t *testing.T) {
		requireRefused(t, deps, `{"passthrough": true, "delivery_mode": "at-most-once"}`,
			`"delivery_mode" was removed`, `"passthrough" was removed`)
	})
	t.Run("unknown key", func(t *testing.T) {
		requireRefused(t, deps, `{"ping_intervals": "30s"}`, `"ping_intervals"`)
	})
	t.Run("data after the object", func(t *testing.T) {
		requireRefused(t, deps, `{"path": "/ws"} {}`)
	})

	inputs := map[string]struct {
		inputs []component.PortDefinition
		want   []string
	}{
		"jetstream": {
			[]component.PortDefinition{{Name: "events", Config: component.JetStreamPort{
				StreamName: "EVENTS", Subjects: []string{"events.>"}, ConsumerName: "ws",
			}}},
			[]string{`"events"`, "jetstream", "kv-watch of every ENTITY_STATES key"},
		},
		"kv-watch of another bucket": {
			[]component.PortDefinition{{Name: "communities", Config: component.KVWatchPort{Bucket: graph.BucketCommunityIndex}}},
			[]string{`"communities"`},
		},
		"kv-watch with keys": {
			[]component.PortDefinition{{Name: "some", Config: component.KVWatchPort{
				Bucket: graph.BucketEntityStates, Keys: []string{"acme.>"},
			}}},
			[]string{`"some"`},
		},
		"kv-watch with history": {
			[]component.PortDefinition{{Name: "past", Config: component.KVWatchPort{
				Bucket: graph.BucketEntityStates, History: true,
			}}},
			[]string{`"past"`},
		},
		"subject with a queue group": {
			[]component.PortDefinition{{Name: "shared", Config: component.NATSPort{Subject: "boids.frames", Queue: "q"}}},
			[]string{`"shared"`, "queue"},
		},
		"another kind": {
			[]component.PortDefinition{{Name: "entity_read", Config: component.KVReadPort{Bucket: graph.BucketEntityStates}}},
			[]string{`"entity_read"`},
		},
		"two inputs": {
			[]component.PortDefinition{entityWatch(), {Name: "frames", Config: component.NATSPort{Subject: "boids.frames"}}},
			[]string{"exactly one input", "got 2"},
		},
		"no input": {
			[]component.PortDefinition{},
			[]string{"exactly one input", "got 0"},
		},
	}
	for name, tc := range inputs {
		t.Run("input "+name, func(t *testing.T) {
			requireRefused(t, deps, withPorts(t, tc.inputs, []component.PortDefinition{server(9001)}), tc.want...)
		})
	}

	outputs := map[string]struct {
		outputs []component.PortDefinition
		want    []string
	}{
		"port below 1024": {[]component.PortDefinition{server(1023)}, []string{`"websocket_server"`, "port 1023", "1024-65535"}},
		"not http": {
			[]component.PortDefinition{{Name: "raw", Config: component.NetworkPort{Protocol: "udp", Host: "127.0.0.1", Port: 9001}}},
			[]string{`"raw"`, "http"},
		},
		"not network": {
			[]component.PortDefinition{{Name: "out", Config: component.NATSPort{Subject: "out"}}},
			[]string{`"out"`, "network"},
		},
		"two outputs": {
			[]component.PortDefinition{server(9001), {Name: "second", Config: component.NetworkPort{
				Protocol: "http", Host: "127.0.0.1", Port: 9002,
			}}},
			[]string{"exactly one output", "got 2"},
		},
		"no output": {[]component.PortDefinition{}, []string{"exactly one output", "got 0"}},
	}
	for name, tc := range outputs {
		t.Run("output "+name, func(t *testing.T) {
			requireRefused(t, deps, withPorts(t, []component.PortDefinition{entityWatch()}, tc.outputs), tc.want...)
		})
	}
	// Port 0 and ports above 65535 cannot be marshalled from a PortDefinition, so these are written
	// as JSON; the port codec refuses them while decoding, naming the port and its field.
	for _, number := range []string{"0", "65536"} {
		t.Run("output port "+number, func(t *testing.T) {
			raw := `{"ports": {"inputs": [{"name": "entity_watch", "config": {"kind": "kv-watch", "bucket": "ENTITY_STATES"}}],
				"outputs": [{"name": "websocket_server", "config": {"kind": "network", "protocol": "http", "host": "127.0.0.1", "port": ` +
				number + `}}]}}`
			requireRefused(t, deps, raw, `"websocket_server"`, `field "port"`)
		})
	}

	for name, value := range map[string]string{"empty": "", "not a duration": "soon", "zero": "0s", "negative": "-1s"} {
		t.Run("ping_interval "+name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"ping_interval": value})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			requireRefused(t, deps, string(raw), "ping_interval", `"`+value+`"`)
		})
	}

	// ServeMux accepts a host-qualified pattern such as example.com/ws; the path must be path-only.
	paths := map[string]struct{ value, want string }{
		"empty":     {"", "path cannot be empty"},
		"with host": {"example.com/ws", "must begin with /"},
		"space":     {"/a b", "whitespace"},
		"control":   {"/ws\x7f", "control character"},
	}
	for name, tc := range paths {
		t.Run("path "+name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"path": tc.value})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			requireRefused(t, deps, string(raw), "path", tc.want)
		})
	}

	t.Run("nil NATS client", func(t *testing.T) {
		requireRefused(t, component.Dependencies{}, `{}`, "NATS client")
	})
}

// TestConfigKeysAreRead: every kept key changes what the factory resolves, read against values
// written out here (design D5).
func TestConfigKeysAreRead(t *testing.T) {
	frames := component.PortDefinition{Name: "frames", Config: component.NATSPort{Subject: "boids.>"}}
	raw, err := json.Marshal(map[string]any{
		"path":          "/stream",
		"ping_interval": "250ms",
		"ports":         component.PortConfig{Inputs: []component.PortDefinition{frames}, Outputs: []component.PortDefinition{server(9001)}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := mustResolve(t, string(raw))
	if s.path != "/stream" {
		t.Errorf("path = %q, want /stream", s.path)
	}
	if s.pingInterval != 250*time.Millisecond {
		t.Errorf("ping interval = %v, want 250ms", s.pingInterval)
	}
	if s.subject != "boids.>" {
		t.Errorf("subject = %q, want boids.>", s.subject)
	}
	if s.input.Name != "frames" {
		t.Errorf("input port = %q, want frames", s.input.Name)
	}
	if want := (component.NetworkPort{Protocol: "http", Host: "127.0.0.1", Port: 9001}); s.output.Config != want {
		t.Errorf("output = %+v, want %+v", s.output.Config, want)
	}

	// A ports block is used as written, never merged with the default ports: an applied-state
	// input given with its own output keeps that output.
	applied := mustResolve(t, withPorts(t, []component.PortDefinition{entityWatch()}, []component.PortDefinition{server(9002)}))
	if applied.subject != "" {
		t.Errorf("applied-state input resolved subject %q, want none", applied.subject)
	}
	if want := (component.NetworkPort{Protocol: "http", Host: "127.0.0.1", Port: 9002}); applied.output.Config != want {
		t.Errorf("output = %+v, want %+v", applied.output.Config, want)
	}
}

// TestDefaultConfig: an empty configuration and {} both stream applied state from ENTITY_STATES on
// 0.0.0.0:8081 at /ws, pinging every 30s (design D5).
func TestDefaultConfig(t *testing.T) {
	for _, raw := range []string{"", `{}`} {
		s := mustResolve(t, raw)
		if want := (component.KVWatchPort{Bucket: graph.BucketEntityStates}); !reflect.DeepEqual(s.input.Config, want) {
			t.Errorf("%q: input = %+v, want %+v", raw, s.input.Config, want)
		}
		if s.subject != "" {
			t.Errorf("%q: subject = %q, want none", raw, s.subject)
		}
		if want := (component.NetworkPort{Protocol: "http", Host: "0.0.0.0", Port: 8081}); s.output.Config != want {
			t.Errorf("%q: output = %+v, want %+v", raw, s.output.Config, want)
		}
		if s.path != "/ws" {
			t.Errorf("%q: path = %q, want /ws", raw, s.path)
		}
		if s.pingInterval != 30*time.Second {
			t.Errorf("%q: ping interval = %v, want 30s", raw, s.pingInterval)
		}
	}
}

// TestRegisterDeclaresWhatTheFactoryResolves: Register adds the websocket factory, whose schema
// lists the three kept keys and no removed one, and whose port declarer reports the ports the
// factory resolves for each configuration.
func TestRegisterDeclaresWhatTheFactoryResolves(t *testing.T) {
	reg := registration(t)
	kept := []string{"ports", "path", "ping_interval"}
	for _, key := range kept {
		if _, ok := reg.Schema.Properties[key]; !ok {
			t.Errorf("schema has no %q property", key)
		}
	}
	if len(reg.Schema.Properties) != len(kept) {
		t.Errorf("schema properties = %d, want the %d kept keys", len(reg.Schema.Properties), len(kept))
	}

	frames := component.PortDefinition{Name: "frames", Config: component.NATSPort{Subject: "boids.frames"}}
	for _, raw := range []string{
		`{}`,
		withPorts(t, []component.PortDefinition{frames}, []component.PortDefinition{server(9001)}),
		withPorts(t, []component.PortDefinition{entityWatch()}, []component.PortDefinition{server(9002)}),
	} {
		declared, err := reg.Ports(json.RawMessage(raw), "websocket")
		if err != nil {
			t.Fatalf("declare ports for %s: %v", raw, err)
		}
		s := mustResolve(t, raw)
		want := component.PortConfigFrom([]component.Port{s.input}, []component.Port{s.output})
		if !reflect.DeepEqual(declared, want) {
			t.Fatalf("declared ports for %s = %+v, want the factory's %+v", raw, declared, want)
		}
	}
}

// TestFactoryBuildsNothingYet: until the server exists (task 3.1), a configuration that passes
// every check is still refused, so no composition admits a websocket instance that would never
// serve. The refusal is not a configuration fault.
func TestFactoryBuildsNothingYet(t *testing.T) {
	built, err := create(json.RawMessage(`{}`), testDeps(t))
	if built != nil || !errors.Is(err, errNotBuilt) {
		t.Fatalf("factory({}) = %v, %v; want no component and errNotBuilt", built, err)
	}
	if errs.IsInvalid(err) {
		t.Fatalf("factory({}) = %v; want a refusal that is not an invalid-config error", err)
	}
}
