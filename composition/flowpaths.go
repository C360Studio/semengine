package composition

import (
	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"
)

// FlowPaths reports, for each origin in the projection, every node reachable
// from it. An origin is a node of type input, or a node with an input port
// whose pattern is network or http-client. Each value is the origin followed
// by every node reachable from it along the graph's edges, each node once, in
// depth-first order over the edges in the order the graph holds them. The map
// is never nil; it is empty when the graph has no origin. FlowPaths stores
// nothing and reads nothing but its argument.
func FlowPaths(graph Graph) map[string][]string {
	adjacency := make(map[string][]string, len(graph.Nodes))
	for _, edge := range graph.Edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}

	paths := make(map[string][]string)
	for _, node := range graph.Nodes {
		if !isInputNode(node) {
			continue
		}
		paths[node.Instance] = reachableFrom(node.Instance, adjacency)
	}
	return paths
}

// isInputNode reports whether a projected node originates data for the graph.
// Both facts come off the projection itself: the declared component type, and
// an input port whose interaction pattern reaches outside the composition —
// PatternNetwork binds a local listener, PatternHTTPClient initiates an
// outbound poll. Either makes the component a data source.
func isInputNode(node Node) bool {
	if node.Type == string(types.ComponentTypeInput) {
		return true
	}
	for _, port := range node.Inputs {
		switch component.InteractionPattern(port.Pattern) {
		case component.PatternNetwork, component.PatternHTTPClient:
			return true
		}
	}
	return false
}

// reachableFrom returns start followed by every node reachable from it, in
// depth-first order over adjacency, which holds each node's targets in the
// graph's edge order. The visited set is this walk's own.
func reachableFrom(start string, adjacency map[string][]string) []string {
	visited := make(map[string]bool)
	var order []string

	var visit func(string)
	visit = func(node string) {
		if visited[node] {
			return
		}
		visited[node] = true
		order = append(order, node)
		for _, next := range adjacency[node] {
			visit(next)
		}
	}
	visit(start)
	return order
}
