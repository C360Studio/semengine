package component

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/c360studio/semengine/types"
)

// declareFixture registers one factory, "declared", whose declarer reports the
// ports its component is built with, and counts both calls.
type declareFixture struct {
	registry      *Registry
	built         *declarationTestComponent
	factoryCalls  int
	declarerCalls int
}

func newDeclareFixture(t *testing.T) *declareFixture {
	t.Helper()
	f := &declareFixture{
		registry: NewRegistry(),
		built: &declarationTestComponent{
			inputs: []Port{{
				Name: "feed", Direction: DirectionInput, Required: true,
				Config: JetStreamPort{StreamName: "EVENTS", Subjects: []string{"events.>"}},
			}},
			// The listener is declared, never bound: it makes the
			// declaration's exclusive resources non-empty.
			outputs: []Port{{
				Name: "state", Direction: DirectionOutput, Required: true,
				Config: KVWritePort{Bucket: "STATE"},
			}, {
				Name: "listener", Direction: DirectionOutput, Required: true,
				Config: NetworkPort{Protocol: "tcp", Host: "127.0.0.1", Port: 8080},
			}},
		},
	}
	requireNoError(t, f.registry.RegisterWithConfig(RegistrationConfig{
		Name: "declared", Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) {
			f.factoryCalls++
			return f.built, nil
		},
		Ports: func(raw json.RawMessage, instance string) (PortConfig, error) {
			f.declarerCalls++
			return f.built.declarePorts(raw, instance)
		},
	}))
	return f
}

// TestDeclareReturnsTheAdmittedDeclarationWithoutConstructing proves Declare is
// the offline half of admission: for a configured instance it returns the
// declaration the Registry retains when that instance is admitted (its name,
// factory, type, ports, port facts and exclusive resources), without calling
// the factory and without admitting the instance.
func TestDeclareReturnsTheAdmittedDeclarationWithoutConstructing(t *testing.T) {
	f := newDeclareFixture(t)
	config := declarationTestConfig("declared", `{}`)

	declared, err := f.registry.Declare("worker", config)
	requireNoError(t, err)
	if f.factoryCalls != 0 {
		t.Fatalf("Declare called the factory %d times, want none", f.factoryCalls)
	}
	if _, admitted := f.registry.declaration("worker"); admitted {
		t.Fatal("Declare admitted the instance")
	}
	if declared.InstanceName != "worker" || declared.FactoryIdentity != "declared" ||
		declared.ComponentType != types.ComponentTypeProcessor {
		t.Fatalf("declaration names %q/%q/%q, want worker/declared/processor",
			declared.InstanceName, declared.FactoryIdentity, declared.ComponentType)
	}
	if len(declared.InputPorts) != 1 || declared.InputPorts[0].Name != "feed" ||
		declared.InputPorts[0].Direction != DirectionInput || len(declared.InputFacts) != 1 {
		t.Fatalf("declared inputs = %+v, want the one input feed with its facts", declared.InputPorts)
	}
	if len(declared.OutputPorts) != 2 || declared.OutputPorts[0].Name != "state" ||
		declared.OutputPorts[1].Name != "listener" || declared.OutputPorts[1].Direction != DirectionOutput ||
		len(declared.OutputFacts) != 2 {
		t.Fatalf("declared outputs = %+v, want state and listener with their facts", declared.OutputPorts)
	}
	if want := []string{"tcp:127.0.0.1:8080"}; !reflect.DeepEqual(declared.ExclusiveResources, want) {
		t.Fatalf("exclusive resources = %v, want %v", declared.ExclusiveResources, want)
	}

	_, err = f.registry.CreateComponent("worker", config, declarationTestDeps(), nil)
	requireNoError(t, err)
	admitted, ok := f.registry.declaration("worker")
	if !ok {
		t.Fatal("CreateComponent retained no declaration")
	}
	if !reflect.DeepEqual(declared, admitted) {
		t.Fatalf("Declare returned\n%#v\nadmission retained\n%#v", declared, admitted)
	}
}

// TestDeclareRefusesWhatAdmissionWouldRefuse proves each refusal Declare makes:
// an invalid instance or factory name, a missing component type, a
// configuration the security gate refuses, an unknown factory, a factory of
// another type, a declarer that fails, and a declared input or output the port
// resolver refuses. Each is refused as invalid with no declaration, the
// factory is never called, and nothing is admitted; the refusals made before
// the factory lookup do not run the declarer.
func TestDeclareRefusesWhatAdmissionWouldRefuse(t *testing.T) {
	errDeclarer := errors.New("declarer failed")
	tests := []struct {
		name             string
		instance         string
		config           func(types.ComponentConfig) types.ComponentConfig
		ports            func(json.RawMessage, string) (PortConfig, error) // registered as "other" when set
		beforeDeclarer   bool
		wantDeclarerCall bool
	}{
		{name: "empty instance name", instance: "", beforeDeclarer: true},
		{name: "instance name with a slash", instance: "a/b", beforeDeclarer: true},
		{
			name: "no component type", instance: "worker", beforeDeclarer: true,
			config: func(c types.ComponentConfig) types.ComponentConfig { c.Type = ""; return c },
		},
		{
			name: "factory name with a space", instance: "worker", beforeDeclarer: true,
			config: func(c types.ComponentConfig) types.ComponentConfig { c.Name = "de clared"; return c },
		},
		{
			name: "configuration with a control character", instance: "worker", beforeDeclarer: true,
			config: func(c types.ComponentConfig) types.ComponentConfig {
				c.Config = json.RawMessage(`{"note":"\u0007"}`)
				return c
			},
		},
		{
			name: "unknown factory", instance: "worker", beforeDeclarer: true,
			config: func(c types.ComponentConfig) types.ComponentConfig { c.Name = "missing"; return c },
		},
		{
			name: "factory of another type", instance: "worker", beforeDeclarer: true,
			config: func(c types.ComponentConfig) types.ComponentConfig {
				c.Type = types.ComponentTypeOutput
				return c
			},
		},
		{
			name: "declarer fails", instance: "worker",
			ports: func(json.RawMessage, string) (PortConfig, error) { return PortConfig{}, errDeclarer },
		},
		{
			name: "input the resolver refuses", instance: "worker",
			ports: func(json.RawMessage, string) (PortConfig, error) {
				return PortConfig{Inputs: []PortDefinition{{Name: "p", Config: KVWritePort{Bucket: "B"}}}}, nil
			},
		},
		{
			name: "output the resolver refuses", instance: "worker",
			ports: func(json.RawMessage, string) (PortConfig, error) {
				return PortConfig{Outputs: []PortDefinition{{Config: NATSPort{Subject: "a"}}}}, nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDeclareFixture(t)
			config := declarationTestConfig("declared", `{}`)
			otherCalls := 0
			if tt.ports != nil {
				requireNoError(t, f.registry.RegisterWithConfig(RegistrationConfig{
					Name: "other", Type: "processor",
					Factory: func(json.RawMessage, Dependencies) (Discoverable, error) {
						f.factoryCalls++
						return f.built, nil
					},
					Ports: func(raw json.RawMessage, instance string) (PortConfig, error) {
						otherCalls++
						return tt.ports(raw, instance)
					},
				}))
				config.Name = "other"
			}
			if tt.config != nil {
				config = tt.config(config)
			}

			declared, err := f.registry.Declare(tt.instance, config)
			requireInvalid(t, err)
			if !reflect.DeepEqual(declared, Declaration{}) {
				t.Fatalf("refused Declare returned a declaration: %#v", declared)
			}
			if f.factoryCalls != 0 {
				t.Fatalf("factory called %d times, want none", f.factoryCalls)
			}
			if tt.beforeDeclarer && f.declarerCalls != 0 {
				t.Fatalf("declarer ran %d times before a refusal it should precede", f.declarerCalls)
			}
			if tt.ports != nil && otherCalls != 1 {
				t.Fatalf("declarer ran %d times, want once", otherCalls)
			}
			if _, admitted := f.registry.declaration(tt.instance); admitted {
				t.Fatal("a refused Declare admitted the instance")
			}
		})
	}
}
