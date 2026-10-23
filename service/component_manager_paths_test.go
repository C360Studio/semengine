package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"
)

// TestFlowPathsTraverseTheRetainedGraph guards the traversal GetFlowPaths runs over the graph the
// kept composition result holds, rather than over a graph rebuilt from the registry. An empty map
// is a well-formed answer, and the route tests only check for 200, so the traversal itself is
// asserted here.
//
// It covers all three origin rules, because each is decided by a different projected fact and
// each could be deleted on its own: "poller" is an origin by its declared component type,
// "listener" by a network-listening input port, and "fetcher" by an outbound HTTP-client input
// port. The ports are declarations only: nothing listens on the network port's number.
func TestFlowPathsTraverseTheRetainedGraph(t *testing.T) {
	poller := &portFactsDiscoverable{
		inputs: []component.Port{{
			Name: "tick", Direction: component.DirectionInput,
			Config: component.TimerPort{Interval: "30s"},
		}},
		outputs: []component.Port{{
			Name: "out", Direction: component.DirectionOutput,
			Config: component.NATSPort{Subject: "paths.test.data"},
		}},
	}
	listener := &portFactsDiscoverable{
		inputs: []component.Port{{
			Name: "wire", Direction: component.DirectionInput,
			Config: component.NetworkPort{Protocol: "udp", Host: "0.0.0.0", Port: 34551},
		}},
		outputs: []component.Port{{
			Name: "out", Direction: component.DirectionOutput,
			Config: component.NATSPort{Subject: "paths.test.data"},
		}},
	}
	fetcher := &portFactsDiscoverable{
		inputs: []component.Port{{
			Name: "feed", Direction: component.DirectionInput,
			Config: component.HTTPClientPort{Method: "GET", URLPattern: "https://example.test/feed"},
		}},
		outputs: []component.Port{{
			Name: "out", Direction: component.DirectionOutput,
			Config: component.NATSPort{Subject: "paths.test.data"},
		}},
	}
	middle := &portFactsDiscoverable{
		inputs: []component.Port{{
			Name: "in", Direction: component.DirectionInput,
			Config: component.NATSPort{Subject: "paths.test.data"},
		}},
		outputs: []component.Port{{
			Name: "out", Direction: component.DirectionOutput,
			Config: component.NATSPort{Subject: "paths.test.derived"},
		}},
	}
	sink := &portFactsDiscoverable{
		inputs: []component.Port{{
			Name: "in", Direction: component.DirectionInput,
			Config: component.NATSPort{Subject: "paths.test.derived"},
		}},
	}

	manager := newProjectionComponentManager(t, component.NewRegistry(),
		projectionComponent{name: "poller", componentType: types.ComponentTypeInput, instance: poller},
		projectionComponent{name: "listener", componentType: types.ComponentTypeProcessor, instance: listener},
		projectionComponent{name: "fetcher", componentType: types.ComponentTypeProcessor, instance: fetcher},
		projectionComponent{name: "middle", componentType: types.ComponentTypeProcessor, instance: middle},
		projectionComponent{name: "sink", componentType: types.ComponentTypeOutput, instance: sink},
	)

	paths, err := manager.GetFlowPaths()
	require.NoError(t, err)

	want := map[string][]string{
		"poller":   {"poller", "middle", "sink"},
		"listener": {"listener", "middle", "sink"},
		"fetcher":  {"fetcher", "middle", "sink"},
	}
	require.Len(t, paths, len(want),
		"paths = %#v; want one entry per origin: a declared input type, a network-listening port, and an "+
			"outbound HTTP-client port. Each is a separate arm of isInputNode and each can be deleted "+
			"independently, so all three are asserted", paths)
	for origin, expected := range want {
		reachable, ok := paths[origin]
		require.True(t, ok, "paths = %#v, want %q recognised as an origin", paths, origin)
		require.Equal(t, expected, reachable,
			"%q must reach the whole downstream chain in depth-first order; a wrong or missing edge shows up here",
			origin)
	}
}
