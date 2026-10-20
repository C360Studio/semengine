package composition_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

func TestMermaidIsDeterministic(t *testing.T) {
	registry := fakeRegistry(t,
		fakeSpec{name: "src", typ: "input", outputs: []component.PortDefinition{jetStreamOut("out", "DATA", "data.>")}},
		fakeSpec{name: "proc", typ: "processor",
			inputs:  []component.PortDefinition{jetStreamIn("in", "DATA", "data.>", true)},
			outputs: []component.PortDefinition{natsOut("out", "proc.done", nil)}},
		fakeSpec{name: "sink", typ: "output", inputs: []component.PortDefinition{natsIn("in", "proc.done", true, nil)}},
	)
	cfg := compositionOf(config.ComponentConfigs{
		"b-src":  instance("src", types.ComponentTypeInput),
		"a-src":  instance("src", types.ComponentTypeInput),
		"proc":   instance("proc", types.ComponentTypeProcessor),
		"z-sink": instance("sink", types.ComponentTypeOutput),
		"y-sink": instance("sink", types.ComponentTypeOutput),
	})
	var renders []string
	for i := 0; i < 5; i++ {
		result, err := composition.Validate(registry, cfg)
		if err != nil {
			t.Fatal(err)
		}
		renders = append(renders, composition.Mermaid(result.Graph))
	}
	for i := 1; i < len(renders); i++ {
		if renders[0] != renders[i] {
			t.Fatalf("render %d differs from render 0:\n%s\n---\n%s", i, renders[0], renders[i])
		}
	}
	if !strings.HasPrefix(renders[0], "flowchart LR") {
		t.Fatalf("Mermaid output does not start with a flowchart header:\n%s", renders[0])
	}
	for _, instance := range []string{"a-src", "b-src", "proc", "y-sink", "z-sink"} {
		if !strings.Contains(renders[0], instance) {
			t.Errorf("Mermaid output lacks node %s:\n%s", instance, renders[0])
		}
	}
	if strings.Count(renders[0], "-->") != 4 {
		t.Fatalf("Mermaid output renders %d edges, want 4:\n%s", strings.Count(renders[0], "-->"), renders[0])
	}

	// The spec scenario: one projection built by hand, rendered with its nodes
	// and edges supplied in one order and then in the reverse order. The first
	// two edges differ only in their source, so the render is stable only if
	// the edge order reads every key Mermaid prints.
	nodes := []composition.Node{
		{Instance: "a", Factory: "src"}, {Instance: "b", Factory: "src"}, {Instance: "sink", Factory: "sink"},
	}
	edges := []composition.Edge{
		{From: "a", FromPort: "out", To: "sink", ToPort: "in", Pattern: "stream", ConnectionID: "data.x"},
		{From: "b", FromPort: "out", To: "sink", ToPort: "in", Pattern: "stream", ConnectionID: "data.x"},
		{From: "a", FromPort: "alt", To: "sink", ToPort: "in", Pattern: "stream", ConnectionID: "data.y"},
	}
	reversedNodes, reversedEdges := slices.Clone(nodes), slices.Clone(edges)
	slices.Reverse(reversedNodes)
	slices.Reverse(reversedEdges)
	forward := composition.Mermaid(composition.Graph{Nodes: nodes, Edges: edges})
	backward := composition.Mermaid(composition.Graph{Nodes: reversedNodes, Edges: reversedEdges})
	if forward != backward {
		t.Fatalf("render depends on input order:\n%s\n---\n%s", forward, backward)
	}
	if strings.Count(forward, "-->") != len(edges) {
		t.Fatalf("hand-built render has %d edges, want %d:\n%s", strings.Count(forward, "-->"), len(edges), forward)
	}
}
