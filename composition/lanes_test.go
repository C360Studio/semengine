package composition_test

import (
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

func contract(typ string) *component.InterfaceContract {
	if typ == "" {
		return nil
	}
	return &component.InterfaceContract{Type: typ}
}

// TestValidateKeepsSameNamedLanesApart is Codex's #346 probe: a processor may
// name an input and an output alike, and the output's interface contract must
// not stand in for the input's. The row with the output renamed `out` is the
// control: it gives the same judgment, so each assertion is shown to run.
func TestValidateKeepsSameNamedLanesApart(t *testing.T) {
	cases := []struct {
		name         string
		sourceType   string // what src's output "out" provides
		inputType    string // what proc's input "data" requires
		outputName   string // proc's output, on another subject
		outputType   string
		wantMismatch string // the literal interface_mismatch message, or "" for none
		wantMissing  int
	}{
		{
			name: "mismatch beside a same-named output", sourceType: "a.v1", inputType: "b.v1",
			outputName: "data", outputType: "a.v1",
			wantMismatch: "Interface mismatch: source port 'out' provides 'a.v1' but target port 'data' requires 'b.v1'",
		},
		{
			name: "mismatch with the output renamed out", sourceType: "a.v1", inputType: "b.v1",
			outputName: "out", outputType: "a.v1",
			wantMismatch: "Interface mismatch: source port 'out' provides 'a.v1' but target port 'data' requires 'b.v1'",
		},
		{
			name: "compatible beside a same-named output", sourceType: "a.v1", inputType: "a.v1",
			outputName: "data", outputType: "b.v1",
		},
		{
			name: "missing interface beside a same-named output", sourceType: "", inputType: "b.v1",
			outputName: "data", outputType: "",
			wantMissing: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registry := fakeRegistry(t,
				fakeSpec{name: "src", typ: types.ComponentTypeProcessor, outputs: []component.PortDefinition{
					natsOut("out", "data.raw", contract(tc.sourceType)),
				}},
				fakeSpec{name: "proc", typ: types.ComponentTypeProcessor,
					inputs:  []component.PortDefinition{natsIn("data", "data.raw", true, contract(tc.inputType))},
					outputs: []component.PortDefinition{natsOut(tc.outputName, "data.done", contract(tc.outputType))},
				},
			)
			result := validateRoundTrip(t, registry, compositionOf(config.ComponentConfigs{
				"src":  instance("src", types.ComponentTypeProcessor),
				"proc": instance("proc", types.ComponentTypeProcessor),
			}))

			edgeFound := false
			for _, edge := range result.Graph.Edges {
				if edge.From == "src" && edge.FromPort == "out" && edge.To == "proc" && edge.ToPort == "data" {
					edgeFound = true
				}
			}
			if !edgeFound {
				t.Fatalf("no edge src/out -> proc/data; edges = %+v", result.Graph.Edges)
			}

			mismatch := findingsOfType(result.Errors, composition.TypeInterfaceMismatch)
			switch {
			case tc.wantMismatch == "" && len(mismatch) != 0:
				t.Fatalf("interface_mismatch = %+v, want none", mismatch)
			case tc.wantMismatch != "" && (len(mismatch) != 1 || mismatch[0].Message != tc.wantMismatch):
				t.Fatalf("interface_mismatch = %+v, want one with message %q", mismatch, tc.wantMismatch)
			}
			if missing := findingsOfType(result.Warnings, composition.TypeMissingInterface); len(missing) != tc.wantMissing {
				t.Fatalf("missing_interface = %+v, want %d", missing, tc.wantMissing)
			}
		})
	}
}

// TestValidateReportsOrphanBesideSameNamedOutput — a required stream input
// with no publisher is an orphaned_port error even when the same component's
// output of the same name is connected.
func TestValidateReportsOrphanBesideSameNamedOutput(t *testing.T) {
	registry := fakeRegistry(t,
		fakeSpec{name: "proc", typ: types.ComponentTypeProcessor,
			inputs:  []component.PortDefinition{jetStreamIn("data", "DATA", "data.in", true)},
			outputs: []component.PortDefinition{natsOut("data", "data.out", nil)},
		},
		fakeSpec{name: "sink", typ: types.ComponentTypeOutput,
			inputs: []component.PortDefinition{natsIn("in", "data.out", true, nil)},
		},
	)
	result := validateRoundTrip(t, registry, compositionOf(config.ComponentConfigs{
		"proc": instance("proc", types.ComponentTypeProcessor),
		"sink": instance("sink", types.ComponentTypeOutput),
	}))
	orphans := findingsOfType(result.Errors, composition.TypeOrphanedPort)
	if len(orphans) != 1 || orphans[0].Component != "proc" || orphans[0].Port != "data" ||
		!strings.HasPrefix(orphans[0].Message, "input port 'data'") {
		t.Fatalf("orphaned_port errors = %+v, want one naming proc's input data", orphans)
	}
}
