package composition_test

import (
	"reflect"
	"testing"

	"github.com/c360studio/semengine/composition"
)

// processor is a processor node whose input ports carry the given patterns.
func processor(instance string, patterns ...string) composition.Node {
	node := composition.Node{Instance: instance, Type: "processor"}
	for _, pattern := range patterns {
		node.Inputs = append(node.Inputs, composition.PortView{Name: "in-" + pattern, Pattern: pattern})
	}
	return node
}

// source is a node of type input with no input ports.
func source(instance string) composition.Node {
	return composition.Node{Instance: instance, Type: "input"}
}

// edge is a stream connection from one port to another.
func edge(from, fromPort, to string) composition.Edge {
	return composition.Edge{From: from, FromPort: fromPort, To: to, ToPort: "in", Pattern: "stream"}
}

func assertFlowPaths(t *testing.T, got, want map[string][]string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FlowPaths = %v, want %v", got, want)
	}
}

func TestFlowPathsKeysEachOrigin(t *testing.T) {
	// No edges, so edge order plays no part: each origin reaches only itself.
	graph := composition.Graph{Nodes: []composition.Node{
		source("feed"),
		processor("listener", "network"),
		processor("poller", "http-client"),
		processor("worker", "stream", "request"),
	}}

	assertFlowPaths(t, composition.FlowPaths(graph), map[string][]string{
		"feed":     {"feed"},
		"listener": {"listener"},
		"poller":   {"poller"},
	})
}

func TestFlowPathsWalksDepthFirstInEdgeOrder(t *testing.T) {
	// Edges are in the order the projection sorts them (source, then source
	// port): feed's out-1 edge to "zeta" comes before its out-2 edge to "beta",
	// though "beta" sorts first by name. Depth-first in that order lists the
	// whole zeta branch before beta; breadth-first, or a walk by name, would not.
	graph := composition.Graph{
		Nodes: []composition.Node{
			source("feed"),
			processor("beta", "stream"),
			processor("yank", "stream"),
			processor("zeta", "stream"),
		},
		Edges: []composition.Edge{
			edge("feed", "out-1", "zeta"),
			edge("feed", "out-2", "beta"),
			edge("zeta", "out", "yank"),
		},
	}

	assertFlowPaths(t, composition.FlowPaths(graph), map[string][]string{
		"feed": {"feed", "zeta", "yank", "beta"},
	})
}

func TestFlowPathsVisitsEachNodeOnce(t *testing.T) {
	// A diamond a -> {b, c} -> d with d leading back to the origin a, and a
	// second origin e joining at d. Edges are in the projection's sorted order.
	// Each origin's walk keeps its own visited set, so e reaches the whole
	// cycle while a lists d once and stops at the edge back to itself.
	graph := composition.Graph{
		Nodes: []composition.Node{
			source("a"),
			processor("b", "stream"),
			processor("c", "stream"),
			processor("d", "stream"),
			source("e"),
		},
		Edges: []composition.Edge{
			edge("a", "out", "b"),
			edge("a", "out", "c"),
			edge("b", "out", "d"),
			edge("c", "out", "d"),
			edge("d", "out", "a"),
			edge("e", "out", "d"),
		},
	}

	assertFlowPaths(t, composition.FlowPaths(graph), map[string][]string{
		"a": {"a", "b", "d", "c"},
		"e": {"e", "d", "a", "b", "c"},
	})
}

func TestFlowPathsOverAnEmptyGraph(t *testing.T) {
	got := composition.FlowPaths(composition.Graph{})
	if got == nil {
		t.Fatal("FlowPaths over an empty graph = nil, want a non-nil empty map")
	}
	if len(got) != 0 {
		t.Fatalf("FlowPaths over an empty graph = %v, want empty", got)
	}
}
