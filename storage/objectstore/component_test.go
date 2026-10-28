package objectstore

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"
)

// buildFromJSON builds the Component through the factory Register registers (the registry's
// listing withholds factories), from raw. Construction opens no connection.
func buildFromJSON(t *testing.T, raw string) (*Component, error) {
	t.Helper()
	built, err := newComponent(json.RawMessage(raw), component.Dependencies{})
	if err != nil {
		return nil, err
	}
	return built.(*Component), nil
}

// registration is the objectstore factory Register adds to a fresh registry.
func registration(t *testing.T) *component.Registration {
	t.Helper()
	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	reg, ok := registry.ListFactories()["objectstore"]
	if !ok {
		t.Fatal("Register did not add objectstore")
	}
	return reg
}

// writeInput and storedOutput are the default ports, spelled as an operator writes them.
const (
	writeInput   = `{"name": "write", "config": {"kind": "nats", "subject": "storage.objectstore.write"}}`
	storedOutput = `{"name": "stored", "config": {"kind": "nats", "subject": "storage.objectstore.stored"}}`
)

// TestComponentConfigRefusesUnknownKey: a configuration carrying a key the Component does not
// define is refused at construction and by the port declarer, naming the key; the same
// configuration without it builds (object-store spec, "A data_cache key"; component-registration
// spec, "Unknown configuration keys are refused"). The keys are data_cache, which the pin read
// and this port does not carry (design D6), and a misspelling of bucket_name.
func TestComponentConfigRefusesUnknownKey(t *testing.T) {
	if _, err := buildFromJSON(t, `{"bucket_name": "CONTENT"}`); err != nil {
		t.Fatalf("a config with bucket_name only: %v", err)
	}
	for _, tc := range []struct{ key, raw string }{
		{"data_cache", `{"bucket_name": "CONTENT", "data_cache": {"enabled": false}}`},
		{"bucket", `{"bucket": "CONTENT"}`},
	} {
		t.Run(tc.key, func(t *testing.T) {
			_, err := buildFromJSON(t, tc.raw)
			if err == nil || !strings.Contains(err.Error(), `"`+tc.key+`"`) {
				t.Fatalf("construction error = %v, want a refusal naming %q", err, tc.key)
			}
			if _, err := registration(t).Ports(json.RawMessage(tc.raw), "objectstore"); err == nil ||
				!strings.Contains(err.Error(), `"`+tc.key+`"`) {
				t.Fatalf("port declarer error = %v, want a refusal naming %q", err, tc.key)
			}
		})
	}
	if _, err := buildFromJSON(t, `{"bucket_name": "A"} {"bucket_name": "B"}`); err == nil {
		t.Fatal("construction accepted data after the configuration object")
	}
}

// TestComponentRefusesOutputOtherThanStored: an output port other than stored is refused at
// construction, naming it, since nothing publishes on it; events, the pin's port, is the case the
// spec names (object-store spec, "An events output").
func TestComponentRefusesOutputOtherThanStored(t *testing.T) {
	for _, name := range []string{"events", "archive"} {
		t.Run(name, func(t *testing.T) {
			raw := `{"ports": {"inputs": [` + writeInput + `], "outputs": [` + storedOutput +
				`, {"name": "` + name + `", "config": {"kind": "nats", "subject": "storage.objectstore.` + name + `"}}]}}`
			_, err := buildFromJSON(t, raw)
			if err == nil || !strings.Contains(err.Error(), `"`+name+`"`) {
				t.Fatalf("construction error = %v, want a refusal naming %q", err, name)
			}
			if _, err := registration(t).Ports(json.RawMessage(raw), "objectstore"); err == nil {
				t.Fatalf("the port declarer accepted output %q", name)
			}
		})
	}
}

// TestComponentRefusesInputWithoutStoredOutput: a configuration with an input and no stored
// output is refused naming stored, since every body would be stored and announced to nobody
// (design D3); a Component with no input needs no stored output (object-store spec, "An input with
// no stored output").
func TestComponentRefusesInputWithoutStoredOutput(t *testing.T) {
	for name, raw := range map[string]string{
		"no outputs": `{"ports": {"inputs": [` + writeInput + `]}}`,
		"jetstream input": `{"ports": {"inputs": [{"name": "store_in", "config": {"kind": "jetstream", ` +
			`"stream_name": "MAPPED", "subjects": ["mapped.messages"]}}], "outputs": []}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := buildFromJSON(t, raw)
			if err == nil || !strings.Contains(err.Error(), `"stored"`) {
				t.Fatalf("construction error = %v, want a refusal naming the missing \"stored\" output", err)
			}
		})
	}
	for name, raw := range map[string]string{
		"no ports":      `{"ports": {}}`,
		"stored only":   `{"ports": {"outputs": [` + storedOutput + `]}}`,
		"input, stored": `{"ports": {"inputs": [` + writeInput + `], "outputs": [` + storedOutput + `]}}`,
		// Pin TestNewComponentAcceptsOrdinaryWriteInputs, with the stored output it now needs.
		"several lanes, stored": `{"ports": {"inputs": [` +
			`{"name": "direct_ingest", "config": {"kind": "nats", "subject": "storage.objectstore.direct"}}, ` +
			`{"name": "store_in", "config": {"kind": "jetstream", "stream_name": "MAPPED", "subjects": ["mapped.messages"]}}` +
			`], "outputs": [` + storedOutput + `]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := buildFromJSON(t, raw); err != nil {
				t.Fatalf("construction refused a valid configuration: %v", err)
			}
		})
	}
}

// TestComponentRefusesUnsupportedInputs: an input named api (the pin's removed request lane), a
// request-reply input, two ports of one name and a port on two subjects are refused at
// construction, each naming the port (pin TestNewComponentRejectsRemovedRequestAPIInputs).
func TestComponentRefusesUnsupportedInputs(t *testing.T) {
	for _, tc := range []struct{ name, inputs, want string }{
		{"api name", `{"name": "api", "config": {"kind": "nats", "subject": "storage.objectstore.write"}}`, `"api"`},
		{"request kind", `{"name": "legacy", "config": {"kind": "nats-request", "subject": "storage.legacy.request"}}`, `"legacy"`},
		{"duplicate name", writeInput + `, {"name": "write", "config": {"kind": "nats", "subject": "storage.other"}}`, `"write"`},
		{"two subjects", `{"name": "store_in", "config": {"kind": "jetstream", "stream_name": "MAPPED", ` +
			`"subjects": ["mapped.a", "mapped.b"]}}`, `"store_in"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"ports": {"inputs": [` + tc.inputs + `], "outputs": [` + storedOutput + `]}}`
			if _, err := buildFromJSON(t, raw); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("construction error = %v, want a refusal naming %s", err, tc.want)
			}
		})
	}
}

// TestDeclarePortsMatchesComponentPorts: the registered port declarer reports exactly the ports
// the built Component reports, for the default and for a configured set; the defaults are the
// write input, the stored output carrying storage.stored.v1, and store-provide for the one
// instance name objectstore (design D6, D7).
func TestDeclarePortsMatchesComponentPorts(t *testing.T) {
	for name, raw := range map[string]string{
		"default": `{}`,
		"configured": `{"bucket_name": "CONTENT", "ports": {"inputs": [{"name": "store_in", "config": {"kind": "jetstream", ` +
			`"stream_name": "MAPPED", "subjects": ["mapped.messages"]}}], "outputs": [` + storedOutput + `]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			declared, err := registration(t).Ports(json.RawMessage(raw), "any-instance")
			if err != nil {
				t.Fatalf("declare ports: %v", err)
			}
			c, err := buildFromJSON(t, raw)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if want := component.PortConfigFrom(c.InputPorts(), c.OutputPorts()); !reflect.DeepEqual(declared, want) {
				t.Fatalf("declared ports %+v differ from the built component's %+v", declared, want)
			}
		})
	}

	c, err := buildFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("build default: %v", err)
	}
	inputs, outputs := c.InputPorts(), c.OutputPorts()
	if len(inputs) != 1 || inputs[0].Name != "write" ||
		!reflect.DeepEqual(inputs[0].Config, component.NATSPort{Subject: "storage.objectstore.write"}) {
		t.Fatalf("default inputs = %+v, want write on storage.objectstore.write", inputs)
	}
	wantOutputs := map[string]component.Portable{
		"stored": component.NATSPort{
			Subject:   "storage.objectstore.stored",
			Interface: &component.InterfaceContract{Type: "storage.stored.v1"},
		},
		"store-provide": component.StoreProvidePort{Instance: "objectstore"},
	}
	if len(outputs) != len(wantOutputs) {
		t.Fatalf("default outputs = %+v, want stored and store-provide", outputs)
	}
	for _, port := range outputs {
		if want, ok := wantOutputs[port.Name]; !ok || !reflect.DeepEqual(port.Config, want) {
			t.Errorf("default output %s = %+v, want %+v", port.Name, port.Config, want)
		}
	}
}

// TestRegisterRegistersObjectstore: Register adds the objectstore factory as a storage component
// whose schema lists bucket_name and ports and nothing the port does not carry, and a second
// Register on the same registry is refused.
func TestRegisterRegistersObjectstore(t *testing.T) {
	reg := registration(t)
	if reg.Type != types.ComponentTypeStorage {
		t.Errorf("Type = %q, want %q", reg.Type, types.ComponentTypeStorage)
	}
	keys := make([]string, 0, len(reg.Schema.Properties))
	for key := range reg.Schema.Properties {
		keys = append(keys, key)
	}
	if len(keys) != 2 || reg.Schema.Properties["bucket_name"].Type == "" || reg.Schema.Properties["ports"].Type == "" {
		t.Errorf("schema properties = %v, want exactly bucket_name and ports", keys)
	}
	if got := reg.Schema.Properties["bucket_name"].Default; got != "MESSAGES" {
		t.Errorf("bucket_name default = %v, want MESSAGES", got)
	}

	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := Register(registry); err == nil {
		t.Fatal("a second Register on one registry was accepted")
	}
}
