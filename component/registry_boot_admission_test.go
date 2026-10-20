package component

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
)

func TestRegistryCapturesPortsOnceAndReturnsDefensiveDeclarationSnapshots(t *testing.T) {
	registry := NewRegistry()
	created := &declarationTestComponent{outputs: []Port{
		declarationTestPort("events.created"),
		{
			Name: "listener", Direction: DirectionOutput, Required: true,
			Config: NetworkPort{Protocol: "tcp", Host: "127.0.0.1", Port: 8080},
		},
	}}
	requireNoError(t, registry.Register(Registration{
		Name: "declaration-test", Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) { return created, nil },
		Ports:   created.declarePorts,
	}))

	got, err := registry.CreateComponent(
		"worker", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)
	if got != created {
		t.Fatal("CreateComponent returned a different component")
	}
	if created.inputCalls != 1 || created.outputCalls != 1 {
		t.Fatalf("port capture calls = input:%d output:%d, want exactly one each", created.inputCalls, created.outputCalls)
	}

	all := registry.Snapshots()
	if len(all) != 1 {
		t.Fatalf("Snapshots length = %d, want 1", len(all))
	}
	assertWorkerDeclarationUnchanged(t, all[0])
	mutateDeclarationSnapshot(all[0])

	rereadAll := registry.Snapshots()
	if len(rereadAll) != 1 {
		t.Fatalf("second Snapshots length = %d, want 1", len(rereadAll))
	}
	assertWorkerDeclarationUnchanged(t, rereadAll[0])
	if created.inputCalls != 1 || created.outputCalls != 1 {
		t.Fatalf("port capture calls after defensive reads = input:%d output:%d, want exactly one each",
			created.inputCalls, created.outputCalls)
	}
}

func mutateDeclarationSnapshot(snapshot Declaration) {
	jetstream := snapshot.OutputPorts[0].Config.(JetStreamPort)
	jetstream.Subjects[0] = "events.mutated"
	snapshot.OutputFacts[0].natsSubjects[0] = "facts.mutated"
	snapshot.OutputFacts[0].stream.subjects[0] = "stream-facts.mutated"
	snapshot.ExclusiveResources[0] = "tcp:mutated:1"
}

func assertWorkerDeclarationUnchanged(t *testing.T, snapshot Declaration) {
	t.Helper()
	if snapshot.InstanceName != "worker" || snapshot.FactoryIdentity != "declaration-test" {
		t.Fatalf("snapshot identity = %q/%q, want worker/declaration-test",
			snapshot.InstanceName, snapshot.FactoryIdentity)
	}
	outputs := snapshot.OutputPorts
	if len(outputs) != 2 {
		t.Fatalf("output ports = %#v, want two retained declarations", outputs)
	}
	jetstream := outputs[0].Config.(JetStreamPort)
	if !reflect.DeepEqual(jetstream.Subjects, []string{"events.created"}) {
		t.Fatalf("port subjects = %v, want events.created", jetstream.Subjects)
	}
	facts := snapshot.OutputFacts
	if got := facts[0].NATSSubjects(); !reflect.DeepEqual(got, []string{"events.created"}) {
		t.Fatalf("fact subjects = %v, want events.created", got)
	}
	stream, ok := facts[0].Stream()
	if !ok || !reflect.DeepEqual(stream.Subjects(), []string{"events.created"}) {
		t.Fatalf("stream fact subjects = %v, present=%v, want events.created", stream.Subjects(), ok)
	}
	if !reflect.DeepEqual(snapshot.ExclusiveResources, []string{"tcp:127.0.0.1:8080"}) {
		t.Fatalf("exclusive resources = %v, want tcp:127.0.0.1:8080", snapshot.ExclusiveResources)
	}
}

func TestRegistryHasNoLiveHandleOrReplacementSurface(t *testing.T) {
	registryType := reflect.TypeOf((*Registry)(nil))
	for _, retiredMethod := range []string{
		"Component", "GetComponent", "ListComponents", "ReplaceComponent",
		"RemoveComponent", "UnregisterInstance", "ReserveReplacement",
	} {
		if _, ok := registryType.MethodByName(retiredMethod); ok {
			t.Errorf("Registry retains retired live-handle method %q", retiredMethod)
		}
	}
}

func TestRegistryManagedPreparationFailurePublishesNothing(t *testing.T) {
	registry := NewRegistry()
	requireNoError(t, registry.Register(Registration{
		Name: "declaration-test", Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) {
			return &declarationTestComponent{}, nil
		},
		Ports: noPorts,
	}))
	want := errors.New("initialize failed")
	_, err := registry.CreateComponent(
		"worker", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(),
		func(Discoverable) error { return want },
	)
	if !errors.Is(err, want) {
		t.Fatalf("CreateComponent error = %v, want %v", err, want)
	}
	if _, ok := admittedDeclaration(registry, "worker"); ok {
		t.Fatal("failed preparation published a declaration")
	}
}

func TestRegistrySnapshotReportsCompleteBootSetAndSealRejectsLaterAdmission(t *testing.T) {
	registry := NewRegistry()
	factoryCalls := 0
	requireNoError(t, registry.Register(Registration{
		Name: "declaration-test", Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) {
			factoryCalls++
			return &declarationTestComponent{}, nil
		},
		Ports: noPorts,
	}))
	_, err := registry.CreateComponent(
		"worker-a", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil,
	)
	requireNoError(t, err)
	_, err = registry.CreateComponent(
		"worker-b", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil,
	)
	requireNoError(t, err)
	admitted := registry.Snapshots()
	if len(admitted) != 2 || admitted[0].InstanceName != "worker-a" || admitted[1].InstanceName != "worker-b" {
		t.Fatalf("boot declaration snapshot = %#v, want complete worker-a/worker-b set", admitted)
	}

	registry.SealComposition()
	_, err = registry.CreateComponent(
		"late", declarationTestConfig("declaration-test", `{}`), declarationTestDeps(), nil,
	)
	if err == nil {
		t.Fatal("post-seal component admission succeeded")
	}
	if factoryCalls != 2 {
		t.Fatalf("factory calls after rejected post-seal admission = %d, want 2", factoryCalls)
	}
	if snapshots := registry.Snapshots(); len(snapshots) != 2 {
		t.Fatalf("post-seal snapshots = %#v, want unchanged two-declaration set", snapshots)
	}
}

// registerPortsFactory registers a factory whose component reports exactly
// these ports, with a declarer that declares the same ones.
func registerPortsFactory(t *testing.T, registry *Registry, factory string, inputs, outputs []Port) {
	t.Helper()
	built := &declarationTestComponent{inputs: inputs, outputs: outputs}
	requireNoError(t, registry.Register(Registration{
		Name: factory, Type: "processor",
		Factory: func(json.RawMessage, Dependencies) (Discoverable, error) { return built, nil },
		Ports:   built.declarePorts,
	}))
}

func networkTestPort(name string, direction Direction, port int) Port {
	return Port{
		Name: name, Direction: direction, Required: true,
		Config: NetworkPort{Protocol: "tcp", Host: "127.0.0.1", Port: port},
	}
}

func admittedNames(registry *Registry) []string {
	names := []string{}
	for _, declaration := range registry.Snapshots() {
		names = append(names, declaration.InstanceName)
	}
	return names
}

func TestRegistryRefusesASecondComponentOnATakenAddress(t *testing.T) {
	registry := NewRegistry()
	registerPortsFactory(t, registry, "listener", nil, []Port{networkTestPort("listen", DirectionOutput, 8080)})
	registerPortsFactory(t, registry, "acceptor", []Port{networkTestPort("accept", DirectionInput, 8080)}, nil)
	_, err := registry.CreateComponent("first", declarationTestConfig("listener", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)

	_, err = registry.CreateComponent("second", declarationTestConfig("acceptor", `{}`), declarationTestDeps(), nil)

	const want = "resource conflict: component 'second' claims tcp:127.0.0.1:8080, " +
		"held by component 'first' as tcp:127.0.0.1:8080"
	if err == nil || !errs.IsInvalid(err) || !strings.Contains(err.Error(), want) {
		t.Fatalf("second component on a taken address: error = %v, want an invalid-class refusal containing %q",
			err, want)
	}
	if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("admitted after the refusal = %v, want [first]", got)
	}
}

// TestRegistryAdmitsComponentsOnDistinctOrSharedResources is the sensitivity
// pair of the refusal above: what is not one exclusive address is admitted.
func TestRegistryAdmitsComponentsOnDistinctOrSharedResources(t *testing.T) {
	tests := []struct {
		name          string
		first, second Port
	}{
		{
			name:   "two different addresses",
			first:  networkTestPort("listen", DirectionOutput, 8080),
			second: networkTestPort("listen", DirectionOutput, 8081),
		},
		{
			name:   "two file ports on one path",
			first:  Port{Name: "file", Direction: DirectionOutput, Config: FilePort{Path: "/shared/events.jsonl"}},
			second: Port{Name: "file", Direction: DirectionOutput, Config: FilePort{Path: "/shared/events.jsonl"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			registerPortsFactory(t, registry, "first-factory", nil, []Port{tt.first})
			registerPortsFactory(t, registry, "second-factory", nil, []Port{tt.second})
			_, err := registry.CreateComponent(
				"first", declarationTestConfig("first-factory", `{}`), declarationTestDeps(), nil)
			requireNoError(t, err)
			_, err = registry.CreateComponent(
				"second", declarationTestConfig("second-factory", `{}`), declarationTestDeps(), nil)
			requireNoError(t, err)
			if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"first", "second"}) {
				t.Fatalf("admitted = %v, want [first second]", got)
			}
		})
	}
}

// TestRegistryRefusesOneComponentClaimingAnAddressTwice: a socket that both
// reads and writes is one port, so two ports of one component on one exclusive
// address are refused, through Declare and through CreateComponent (owner
// ruling, #324 comment 6278113073).
func TestRegistryRefusesOneComponentClaimingAnAddressTwice(t *testing.T) {
	const sameText = "resource conflict: component 'gateway' claims tcp:0.0.0.0:8080, " +
		"held by another of its ports as tcp:0.0.0.0:8080"
	bind := func(name string, direction Direction) Port {
		return networkClaimPort(name, direction, "tcp", "0.0.0.0", 8080)
	}
	tests := []struct {
		name            string
		inputs, outputs []Port
		want            string
	}{
		{
			name:    "an input and an output",
			inputs:  []Port{bind("accept", DirectionInput)},
			outputs: []Port{bind("serve", DirectionOutput)},
			want:    sameText,
		},
		{
			name:   "two inputs",
			inputs: []Port{bind("accept", DirectionInput), bind("accept2", DirectionInput)},
			want:   sameText,
		},
		{
			name:    "two outputs",
			outputs: []Port{bind("serve", DirectionOutput), bind("serve2", DirectionOutput)},
			want:    sameText,
		},
		{
			name:    "an http input and a ws output",
			inputs:  []Port{networkClaimPort("accept", DirectionInput, "http", "0.0.0.0", 8080)},
			outputs: []Port{networkClaimPort("serve", DirectionOutput, "ws", "0.0.0.0", 8080)},
			want: "resource conflict: component 'gateway' claims ws:0.0.0.0:8080, " +
				"held by another of its ports as http:0.0.0.0:8080",
		},
	}
	for _, tt := range tests {
		want := tt.want
		t.Run(tt.name+" through Declare", func(t *testing.T) {
			registry := NewRegistry()
			registerPortsFactory(t, registry, "gateway-factory", tt.inputs, tt.outputs)

			_, err := registry.Declare("gateway", declarationTestConfig("gateway-factory", `{}`))

			if err == nil || !errs.IsInvalid(err) || !strings.Contains(err.Error(), want) {
				t.Fatalf("Declare: error = %v, want an invalid-class refusal containing %q", err, want)
			}
		})
		t.Run(tt.name+" through CreateComponent", func(t *testing.T) {
			registry := NewRegistry()
			registerPortsFactory(t, registry, "gateway-factory", tt.inputs, tt.outputs)

			_, err := registry.CreateComponent(
				"gateway", declarationTestConfig("gateway-factory", `{}`), declarationTestDeps(), nil)

			if err == nil || !errs.IsInvalid(err) || !strings.Contains(err.Error(), want) {
				t.Fatalf("CreateComponent: error = %v, want an invalid-class refusal containing %q", err, want)
			}
			if got := admittedNames(registry); len(got) != 0 {
				t.Fatalf("admitted after the refusal = %v, want none", got)
			}
		})
	}
}

func networkClaimPort(name string, direction Direction, protocol, host string, port int) Port {
	return Port{
		Name: name, Direction: direction, Required: true,
		Config: NetworkPort{Protocol: protocol, Host: host, Port: port},
	}
}

// networkClaimRow is two network claims and their resource texts, written as
// literals so the expected refusal text never comes from the code under test.
type networkClaimRow struct {
	name           string
	firstProtocol  string
	firstHost      string
	firstPort      int
	firstID        string
	secondProtocol string
	secondHost     string
	secondPort     int
	secondID       string
}

func (row networkClaimRow) firstClaim(name string, direction Direction) Port {
	return networkClaimPort(name, direction, row.firstProtocol, row.firstHost, row.firstPort)
}

func (row networkClaimRow) secondClaim(name string, direction Direction) Port {
	return networkClaimPort(name, direction, row.secondProtocol, row.secondHost, row.secondPort)
}

// createHolderThenClaimant admits component first with holder's port, then
// creates component second with claimant's port, and returns the second
// creation's error.
func createHolderThenClaimant(t *testing.T, holder, claimant Port) (*Registry, error) {
	t.Helper()
	registry := NewRegistry()
	registerPortsFactory(t, registry, "first-factory", nil, []Port{holder})
	registerPortsFactory(t, registry, "second-factory", nil, []Port{claimant})
	_, err := registry.CreateComponent("first", declarationTestConfig("first-factory", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)
	_, err = registry.CreateComponent(
		"second", declarationTestConfig("second-factory", `{}`), declarationTestDeps(), nil)
	return registry, err
}

// declareGateway declares component gateway with input as its input port and
// output as its output port.
func declareGateway(t *testing.T, input, output Port) error {
	t.Helper()
	registry := NewRegistry()
	registerPortsFactory(t, registry, "gateway-factory", []Port{input}, []Port{output})
	_, err := registry.Declare("gateway", declarationTestConfig("gateway-factory", `{}`))
	return err
}

func requireInvalidRefusal(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !errs.IsInvalid(err) || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want an invalid-class refusal containing %q", err, want)
	}
}

// TestRegistryRefusesNetworkClaimsThatCannotBothBind: two listeners that
// cannot both bind are refused, whichever comes first and within one
// component (owner rulings, #328 and comment 6290643374; probe in
// openspec/changes/one-network-address-clash/notes/probe.md).
func TestRegistryRefusesNetworkClaimsThatCannotBothBind(t *testing.T) {
	rows := []networkClaimRow{
		{"R1 http and websocket on one TCP port",
			"http", "0.0.0.0", 8080, "http:0.0.0.0:8080", "websocket", "0.0.0.0", 8080, "websocket:0.0.0.0:8080"},
		{"R2 http and tcp on one TCP port",
			"http", "0.0.0.0", 8080, "http:0.0.0.0:8080", "tcp", "0.0.0.0", 8080, "tcp:0.0.0.0:8080"},
		{"R3 UDP and udp",
			"UDP", "0.0.0.0", 8080, "UDP:0.0.0.0:8080", "udp", "0.0.0.0", 8080, "udp:0.0.0.0:8080"},
		{"R4 tcp wildcard and loopback",
			"tcp", "0.0.0.0", 8080, "tcp:0.0.0.0:8080", "tcp", "127.0.0.1", 8080, "tcp:127.0.0.1:8080"},
		{"R5 udp wildcard and loopback",
			"udp", "0.0.0.0", 14550, "udp:0.0.0.0:14550", "udp", "127.0.0.1", 14550, "udp:127.0.0.1:14550"},
		{"R6 IPv4 and IPv6 wildcards",
			"tcp", "0.0.0.0", 8080, "tcp:0.0.0.0:8080", "tcp", "::", 8080, "tcp::::8080"},
		{"R7 IPv6 wildcard and IPv6 loopback",
			"tcp", "::", 8080, "tcp::::8080", "tcp", "::1", 8080, "tcp:::1:8080"},
		{"R8 IPv4 wildcard and IPv6 loopback",
			"tcp", "0.0.0.0", 8080, "tcp:0.0.0.0:8080", "tcp", "::1", 8080, "tcp:::1:8080"},
		{"R9 IPv4-mapped and IPv4 loopback",
			"tcp", "::ffff:127.0.0.1", 8080, "tcp:::ffff:127.0.0.1:8080", "tcp", "127.0.0.1", 8080, "tcp:127.0.0.1:8080"},
		{"R10 two spellings of IPv6 loopback",
			"tcp", "0:0:0:0:0:0:0:1", 8080, "tcp:0:0:0:0:0:0:0:1:8080", "tcp", "::1", 8080, "tcp:::1:8080"},
		{"R11 wildcard and a host name",
			"tcp", "0.0.0.0", 8080, "tcp:0.0.0.0:8080", "tcp", "localhost", 8080, "tcp:localhost:8080"},
		{"R12 one host name twice",
			"tcp", "localhost", 8080, "tcp:localhost:8080", "tcp", "localhost", 8080, "tcp:localhost:8080"},
		{"R13 IPv4-mapped wildcard and loopback",
			"tcp", "::ffff:0.0.0.0", 8080, "tcp:::ffff:0.0.0.0:8080", "tcp", "127.0.0.1", 8080, "tcp:127.0.0.1:8080"},
		{"R14 loopback with and without a zone",
			"tcp", "::1", 8080, "tcp:::1:8080", "tcp", "::1%lo0", 8080, "tcp:::1%lo0:8080"},
		{"R15 loopback zoned by name and by index",
			"tcp", "::1%lo0", 8080, "tcp:::1%lo0:8080", "tcp", "::1%1", 8080, "tcp:::1%1:8080"},
		{"R16 udp loopback on two zones",
			"udp", "::1%lo0", 8080, "udp:::1%lo0:8080", "udp", "::1%en0", 8080, "udp:::1%en0:8080"},
		{"R17 one link-local address and zone twice",
			"tcp", "fe80::1%lo0", 8080, "tcp:fe80::1%lo0:8080", "tcp", "fe80::1%lo0", 8080, "tcp:fe80::1%lo0:8080"},
		{"R18 one link-local address on two interfaces",
			"tcp", "fe80::1%en0", 8080, "tcp:fe80::1%en0:8080", "tcp", "fe80::1%en1", 8080, "tcp:fe80::1%en1:8080"},
		{"R19 link-local zoned by name and by index",
			"tcp", "fe80::1%lo0", 8080, "tcp:fe80::1%lo0:8080", "tcp", "fe80::1%1", 8080, "tcp:fe80::1%1:8080"},
		{"R20 zoned wildcard and loopback",
			"tcp", "::%lo0", 8080, "tcp:::%lo0:8080", "tcp", "127.0.0.1", 8080, "tcp:127.0.0.1:8080"},
	}
	for _, row := range rows {
		t.Run(row.name+" (a) first claim held", func(t *testing.T) {
			registry, err := createHolderThenClaimant(t,
				row.firstClaim("listen", DirectionOutput), row.secondClaim("listen", DirectionOutput))
			requireInvalidRefusal(t, err, "resource conflict: component 'second' claims "+row.secondID+
				", held by component 'first' as "+row.firstID)
			if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"first"}) {
				t.Fatalf("admitted after the refusal = %v, want [first]", got)
			}
		})
		t.Run(row.name+" (b) second claim held", func(t *testing.T) {
			registry, err := createHolderThenClaimant(t,
				row.secondClaim("listen", DirectionOutput), row.firstClaim("listen", DirectionOutput))
			requireInvalidRefusal(t, err, "resource conflict: component 'second' claims "+row.firstID+
				", held by component 'first' as "+row.secondID)
			if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"first"}) {
				t.Fatalf("admitted after the refusal = %v, want [first]", got)
			}
		})
		t.Run(row.name+" (c) one component", func(t *testing.T) {
			err := declareGateway(t, row.firstClaim("accept", DirectionInput), row.secondClaim("serve", DirectionOutput))
			requireInvalidRefusal(t, err, "resource conflict: component 'gateway' claims "+row.secondID+
				", held by another of its ports as "+row.firstID)
		})
	}
}

// TestRegistryAdmitsNetworkClaimsThatCanBothBind is the sensitivity pair of
// the refusals above: listeners that both bind are admitted.
func TestRegistryAdmitsNetworkClaimsThatCanBothBind(t *testing.T) {
	rows := []networkClaimRow{
		{"A1 tcp and udp wildcards",
			"tcp", "0.0.0.0", 8080, "tcp:0.0.0.0:8080", "udp", "0.0.0.0", 8080, "udp:0.0.0.0:8080"},
		{"A2 http and udp wildcards",
			"http", "0.0.0.0", 8080, "http:0.0.0.0:8080", "udp", "0.0.0.0", 8080, "udp:0.0.0.0:8080"},
		{"A3 tcp IPv4 and IPv6 loopback",
			"tcp", "127.0.0.1", 8080, "tcp:127.0.0.1:8080", "tcp", "::1", 8080, "tcp:::1:8080"},
		{"A4 udp IPv4 and IPv6 loopback",
			"udp", "127.0.0.1", 8080, "udp:127.0.0.1:8080", "udp", "::1", 8080, "udp:::1:8080"},
		{"A5 two IPv4 loopback addresses",
			"tcp", "127.0.0.1", 8080, "tcp:127.0.0.1:8080", "tcp", "127.0.0.2", 8080, "tcp:127.0.0.2:8080"},
		{"A6 two ports",
			"tcp", "0.0.0.0", 8080, "tcp:0.0.0.0:8080", "tcp", "0.0.0.0", 8081, "tcp:0.0.0.0:8081"},
		{"A7 a host name is not resolved",
			"tcp", "localhost", 8080, "tcp:localhost:8080", "tcp", "127.0.0.1", 8080, "tcp:127.0.0.1:8080"},
		{"A8 UDP and tcp wildcards",
			"UDP", "0.0.0.0", 8080, "UDP:0.0.0.0:8080", "tcp", "0.0.0.0", 8080, "tcp:0.0.0.0:8080"},
	}
	for _, row := range rows {
		t.Run(row.name+" (a) first claim held", func(t *testing.T) {
			registry, err := createHolderThenClaimant(t,
				row.firstClaim("listen", DirectionOutput), row.secondClaim("listen", DirectionOutput))
			requireNoError(t, err)
			assertAdmittedBoth(t, registry, row)
		})
		t.Run(row.name+" (b) second claim held", func(t *testing.T) {
			registry, err := createHolderThenClaimant(t,
				row.secondClaim("listen", DirectionOutput), row.firstClaim("listen", DirectionOutput))
			requireNoError(t, err)
			assertAdmittedBoth(t, registry, row)
		})
		t.Run(row.name+" (c) one component", func(t *testing.T) {
			requireNoError(t, declareGateway(t,
				row.firstClaim("accept", DirectionInput), row.secondClaim("serve", DirectionOutput)))
		})
	}
}

func assertAdmittedBoth(t *testing.T, registry *Registry, row networkClaimRow) {
	t.Helper()
	if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("admitted = %v, want [first second]", got)
	}
	var resources []string
	for _, declaration := range registry.Snapshots() {
		resources = append(resources, declaration.ExclusiveResources...)
	}
	if want := []string{row.firstID, row.secondID}; !reflect.DeepEqual(slices.Sorted(slices.Values(resources)),
		slices.Sorted(slices.Values(want))) {
		t.Fatalf("exclusive resources = %v, want %v", resources, want)
	}
}

// TestRegistryNamesTheFirstClashingHolderByName: when several admitted
// components clash with a claim, the refusal names the first in instance-name
// order, on every attempt. A refusal leaves the admitted set unchanged, so
// each attempt meets the same two holders.
func TestRegistryNamesTheFirstClashingHolderByName(t *testing.T) {
	const want = "resource conflict: component 'gamma' claims tcp:0.0.0.0:8080, " +
		"held by component 'alpha' as tcp:::1:8080"
	registry := NewRegistry()
	registerPortsFactory(t, registry, "beta-factory", nil,
		[]Port{networkClaimPort("listen", DirectionOutput, "tcp", "127.0.0.1", 8080)})
	registerPortsFactory(t, registry, "alpha-factory", nil,
		[]Port{networkClaimPort("listen", DirectionOutput, "tcp", "::1", 8080)})
	registerPortsFactory(t, registry, "gamma-factory", nil,
		[]Port{networkClaimPort("listen", DirectionOutput, "tcp", "0.0.0.0", 8080)})
	_, err := registry.CreateComponent("beta", declarationTestConfig("beta-factory", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)
	_, err = registry.CreateComponent("alpha", declarationTestConfig("alpha-factory", `{}`), declarationTestDeps(), nil)
	requireNoError(t, err)

	for attempt := range 20 {
		_, err := registry.CreateComponent(
			"gamma", declarationTestConfig("gamma-factory", `{}`), declarationTestDeps(), nil)
		if err == nil || !errs.IsInvalid(err) || !strings.Contains(err.Error(), want) {
			t.Fatalf("attempt %d: error = %v, want an invalid-class refusal containing %q", attempt, err, want)
		}
	}
	if got := admittedNames(registry); !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("admitted after the refusals = %v, want [alpha beta]", got)
	}
}
