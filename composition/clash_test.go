package composition_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

// portsComponent is a constructed component that reports fixed ports, so the
// registry's admission parity check holds by construction. Unlike fakeSpec's
// factories it can be constructed: the clash tests run the same pair through
// Registry.CreateComponent as well as through Validate.
type portsComponent struct{ inputs, outputs []component.Port }

func (c *portsComponent) InputPorts() []component.Port    { return c.inputs }
func (c *portsComponent) OutputPorts() []component.Port   { return c.outputs }
func (c *portsComponent) Health() component.HealthStatus  { return component.HealthStatus{} }
func (c *portsComponent) DataFlow() component.FlowMetrics { return component.FlowMetrics{} }

// registerPortsFactory registers a constructible input factory whose
// component reports inputs and outputs.
func registerPortsFactory(t *testing.T, registry *component.Registry, name string, inputs, outputs []component.Port) {
	t.Helper()
	built := &portsComponent{inputs: inputs, outputs: outputs}
	err := registry.Register(component.Registration{
		Name: name, Type: types.ComponentTypeInput, Protocol: "fake", Domain: "test", Version: "0.0.1",
		Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) { return built, nil },
		Ports: func(json.RawMessage, string) (component.PortConfig, error) {
			return component.PortConfigFrom(inputs, outputs), nil
		},
	})
	if err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

func listener(name string, direction component.Direction, protocol, host string, port int) component.Port {
	return component.Port{
		Name: name, Direction: direction, Required: true,
		Config: component.NetworkPort{Protocol: protocol, Host: host, Port: port},
	}
}

// createBoth admits component a, then component b, through the registry's
// boot-admission seam, and returns b's refusal (nil when admitted).
func createBoth(t *testing.T, registry *component.Registry) error {
	t.Helper()
	deps := component.Dependencies{NATSClient: new(natsclient.Client)}
	if _, err := registry.CreateComponent("a", instance("first-listener", types.ComponentTypeInput), deps, nil); err != nil {
		t.Fatalf("CreateComponent a: %v", err)
	}
	_, err := registry.CreateComponent("b", instance("second-listener", types.ComponentTypeInput), deps, nil)
	return err
}

// TestValidateExclusiveClashMatchesAdmission: the offline judgment of two
// components' network claims is the registry's. Each clashing pair is one
// exclusive_resource_conflict on the later component, whose message equals
// the literal text below, and CreateComponent refuses the same pair with an
// error containing it. The controls are admitted by both. Expected text is
// written as literals, never built by the code under test.
func TestValidateExclusiveClashMatchesAdmission(t *testing.T) {
	tests := []struct {
		name          string
		first, second component.Port
		want          string // "" when the pair can both bind
	}{
		{
			name:   "http and websocket on one TCP port",
			first:  listener("listen", component.DirectionOutput, "http", "0.0.0.0", 8080),
			second: listener("listen", component.DirectionOutput, "websocket", "0.0.0.0", 8080),
			want: "resource conflict: component 'b' claims websocket:0.0.0.0:8080, " +
				"held by component 'a' as http:0.0.0.0:8080",
		},
		{
			name:   "IPv4 and IPv6 wildcards on one port",
			first:  listener("listen", component.DirectionOutput, "tcp", "0.0.0.0", 8080),
			second: listener("listen", component.DirectionOutput, "tcp", "::", 8080),
			want: "resource conflict: component 'b' claims tcp::::8080, " +
				"held by component 'a' as tcp:0.0.0.0:8080",
		},
		{
			name:   "UDP and udp on one port",
			first:  listener("listen", component.DirectionInput, "UDP", "0.0.0.0", 14550),
			second: listener("listen", component.DirectionInput, "udp", "0.0.0.0", 14550),
			want: "resource conflict: component 'b' claims udp:0.0.0.0:14550, " +
				"held by component 'a' as UDP:0.0.0.0:14550",
		},
		{
			name:   "wildcard and loopback on one port",
			first:  listener("listen", component.DirectionOutput, "tcp", "0.0.0.0", 8080),
			second: listener("listen", component.DirectionOutput, "tcp", "127.0.0.1", 8080),
			want: "resource conflict: component 'b' claims tcp:127.0.0.1:8080, " +
				"held by component 'a' as tcp:0.0.0.0:8080",
		},
		{
			name:   "one address spelled the same twice",
			first:  listener("listen", component.DirectionOutput, "tcp", "0.0.0.0", 8080),
			second: listener("listen", component.DirectionOutput, "tcp", "0.0.0.0", 8080),
			want: "resource conflict: component 'b' claims tcp:0.0.0.0:8080, " +
				"held by component 'a' as tcp:0.0.0.0:8080",
		},
		{
			name:   "control: udp and http on one port",
			first:  listener("listen", component.DirectionInput, "udp", "0.0.0.0", 8080),
			second: listener("listen", component.DirectionOutput, "http", "0.0.0.0", 8080),
		},
		{
			name:   "control: one host on two ports",
			first:  listener("listen", component.DirectionOutput, "tcp", "0.0.0.0", 8080),
			second: listener("listen", component.DirectionOutput, "tcp", "0.0.0.0", 9090),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := component.NewRegistry()
			lane := func(port component.Port) (inputs, outputs []component.Port) {
				if port.Direction == component.DirectionInput {
					return []component.Port{port}, nil
				}
				return nil, []component.Port{port}
			}
			firstIn, firstOut := lane(tt.first)
			secondIn, secondOut := lane(tt.second)
			registerPortsFactory(t, registry, "first-listener", firstIn, firstOut)
			registerPortsFactory(t, registry, "second-listener", secondIn, secondOut)

			result := validateRoundTrip(t, registry, compositionOf(config.ComponentConfigs{
				"a": instance("first-listener", types.ComponentTypeInput),
				"b": instance("second-listener", types.ComponentTypeInput),
			}))
			conflicts := findingsOfType(result.Errors, composition.TypeExclusiveResourceConflict)
			instances := make([]string, 0, len(result.Graph.Nodes))
			for _, node := range result.Graph.Nodes {
				instances = append(instances, node.Instance)
			}
			admission := createBoth(t, registry)

			if tt.want == "" {
				if len(result.Errors) != 0 {
					t.Fatalf("Validate errors = %+v, want none", result.Errors)
				}
				if !reflect.DeepEqual(instances, []string{"a", "b"}) {
					t.Fatalf("graph nodes = %v, want [a b]", instances)
				}
				if admission != nil {
					t.Fatalf("CreateComponent b = %v, want admitted", admission)
				}
				return
			}
			if len(result.Errors) != 1 || len(conflicts) != 1 {
				t.Fatalf("Validate errors = %+v, want exactly one exclusive_resource_conflict", result.Errors)
			}
			if conflicts[0].Component != "b" || conflicts[0].Message != tt.want {
				t.Fatalf("conflict = %q on %q, want %q on b", conflicts[0].Message, conflicts[0].Component, tt.want)
			}
			if !reflect.DeepEqual(instances, []string{"a"}) {
				t.Fatalf("graph nodes = %v, want [a]: a refused declaration stays out of the graph", instances)
			}
			if admission == nil || !errs.IsInvalid(admission) || !strings.Contains(admission.Error(), tt.want) {
				t.Fatalf("CreateComponent b = %v, want an invalid-class refusal containing %q", admission, tt.want)
			}
		})
	}
}

// TestValidateSelfClashIsPortDeclarationError: a component whose own claims
// clash is refused by Registry.Declare, so Validate reports the refusal as
// one port_declaration_error and never an exclusive_resource_conflict. At the
// SemStreams pin the same composition gave a connection_pattern_error from the
// flow graph's bind refusal, which SemEngine removed (#324).
func TestValidateSelfClashIsPortDeclarationError(t *testing.T) {
	registry := component.NewRegistry()
	registerPortsFactory(t, registry, "gateway",
		[]component.Port{listener("accept", component.DirectionInput, "tcp", "0.0.0.0", 8080)},
		[]component.Port{listener("serve", component.DirectionOutput, "tcp", "0.0.0.0", 8080)})

	result := validateRoundTrip(t, registry, compositionOf(config.ComponentConfigs{
		"gw": instance("gateway", types.ComponentTypeInput),
	}))
	if len(result.Errors) != 1 {
		t.Fatalf("Validate errors = %+v, want exactly one", result.Errors)
	}
	finding := result.Errors[0]
	if finding.Type != composition.TypePortDeclarationError || finding.Component != "gw" {
		t.Fatalf("finding = %+v, want a port_declaration_error on gw", finding)
	}
	const want = "resource conflict: component 'gw' claims tcp:0.0.0.0:8080, " +
		"held by another of its ports as tcp:0.0.0.0:8080"
	if !strings.Contains(finding.Message, want) {
		t.Fatalf("message = %q, want it to contain %q", finding.Message, want)
	}
}
