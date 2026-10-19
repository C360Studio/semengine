package flowgraph

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

// declaration builds a component.Declaration the way the registry does: each
// port beside the facts it projects.
func declaration(t *testing.T, name string, inputs, outputs []component.Port) component.Declaration {
	t.Helper()
	inputFacts, err := testPortFacts(inputs)
	require.NoError(t, err)
	outputFacts, err := testPortFacts(outputs)
	require.NoError(t, err)
	return component.Declaration{
		InstanceName: name,
		InputPorts:   inputs,
		InputFacts:   inputFacts,
		OutputPorts:  outputs,
		OutputFacts:  outputFacts,
	}
}

func natsPort(name string, direction component.Direction, subject string) component.Port {
	return component.Port{Name: name, Direction: direction, Config: component.NATSPort{Subject: subject}}
}

func networkPort(name string, direction component.Direction) component.Port {
	return component.Port{
		Name:      name,
		Direction: direction,
		Config:    component.NetworkPort{Protocol: "tcp", Host: "0.0.0.0", Port: 8080},
	}
}

func TestBuildFromDeclarationsConnectsDeclaredPorts(t *testing.T) {
	tests := []struct {
		name       string
		pubSubject string
		subSubject string
		want       string
	}{
		{name: "wildcard on the subscriber", pubSubject: "data.raw", subSubject: "data.*", want: "data.raw"},
		{name: "wildcard on the publisher", pubSubject: "events.*", subSubject: "events.raw", want: "events.raw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := BuildFromDeclarations([]component.Declaration{
				declaration(t, "pub", nil, []component.Port{natsPort("out", component.DirectionOutput, tt.pubSubject)}),
				declaration(t, "sub", []component.Port{natsPort("in", component.DirectionInput, tt.subSubject)}, nil),
			})
			require.NoError(t, err)
			assert.Equal(t, []FlowEdge{{
				From:         ComponentPortRef{ComponentName: "pub", PortName: "out"},
				To:           ComponentPortRef{ComponentName: "sub", PortName: "in"},
				Pattern:      component.PatternStream,
				ConnectionID: tt.want,
			}}, graph.GetEdges())
		})
	}
}

// TestTailWildcardNeedsATrailingToken checks the documented `>` rule through
// the production seam: `events.>` stands for one or more tokens after
// `events`, so a publisher on `events` itself leaves a required subscriber on
// `events.>` unconnected, while a publisher on `events.created` reaches it.
func TestTailWildcardNeedsATrailingToken(t *testing.T) {
	build := func(t *testing.T, pubSubject string) *FlowGraph {
		t.Helper()
		input := natsPort("in", component.DirectionInput, "events.>")
		input.Required = true
		graph, err := BuildFromDeclarations([]component.Declaration{
			declaration(t, "publisher", nil, []component.Port{
				natsPort("out", component.DirectionOutput, pubSubject),
			}),
			declaration(t, "subscriber", []component.Port{input}, nil),
		})
		require.NoError(t, err)
		return graph
	}

	t.Run("no trailing token", func(t *testing.T) {
		graph := build(t, "events")
		assert.Empty(t, graph.GetEdges())
		assert.Contains(t, graph.AnalyzeConnectivity().OrphanedPorts, OrphanedPort{
			ComponentName: "subscriber",
			PortName:      "in",
			Direction:     component.DirectionInput,
			ConnectionID:  "events.>",
			Pattern:       component.PatternStream,
			Issue:         IssueNoPublishers,
			Required:      true,
		})
	})

	t.Run("one trailing token", func(t *testing.T) {
		graph := build(t, "events.created")
		assert.Equal(t, []FlowEdge{{
			From:         ComponentPortRef{ComponentName: "publisher", PortName: "out"},
			To:           ComponentPortRef{ComponentName: "subscriber", PortName: "in"},
			Pattern:      component.PatternStream,
			ConnectionID: "events.created",
		}}, graph.GetEdges())
		assert.Empty(t, graph.AnalyzeConnectivity().OrphanedPorts)
	})
}

func TestBuildFromDeclarationsRefusesExtraFacts(t *testing.T) {
	facts, err := natsPort("in", component.DirectionInput, "data.raw").Facts()
	require.NoError(t, err)

	graph, err := BuildFromDeclarations([]component.Declaration{{
		InstanceName: "reader",
		InputFacts:   []component.PortFacts{facts},
	}})

	require.EqualError(t, err, `add component "reader" to flow graph: component "reader" input ports: `+
		`retained port/fact count mismatch: 0 ports, 1 facts`)
	assert.Nil(t, graph)
}

func TestBuildFromDeclarationsRefusesMalformedDeclarations(t *testing.T) {
	port := natsPort("in", component.DirectionInput, "data.raw")
	tests := []struct {
		name  string
		facts []component.PortFacts
		want  string
	}{
		{
			name: "port without facts",
			want: `add component "reader" to flow graph: component "reader" input ports: ` +
				`retained port/fact count mismatch: 1 ports, 0 facts`,
		},
		{
			name:  "facts without a connection identity",
			facts: []component.PortFacts{{}},
			want: `add component "reader" to flow graph: component "reader" input ports: ` +
				`port "in" has no connection identity`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := BuildFromDeclarations([]component.Declaration{{
				InstanceName: "reader",
				InputPorts:   []component.Port{port},
				InputFacts:   tt.facts,
			}})

			require.EqualError(t, err, tt.want)
			assert.Nil(t, graph)
		})
	}
}

func TestBuildFromDeclarationsRefusesNetworkBindConflicts(t *testing.T) {
	tests := []struct {
		name         string
		declarations func(t *testing.T) []component.Declaration
		want         string
	}{
		{
			name: "two outputs",
			declarations: func(t *testing.T) []component.Declaration {
				return []component.Declaration{
					declaration(t, "first", nil, []component.Port{networkPort("listen", component.DirectionOutput)}),
					declaration(t, "second", nil, []component.Port{networkPort("listen", component.DirectionOutput)}),
				}
			},
			want: "network port conflict on tcp:0.0.0.0:8080: multiple components binding",
		},
		{
			name: "one input and one output",
			declarations: func(t *testing.T) []component.Declaration {
				return []component.Declaration{
					declaration(t, "gateway",
						[]component.Port{networkPort("accept", component.DirectionInput)},
						[]component.Port{networkPort("serve", component.DirectionOutput)}),
				}
			},
			want: "network port conflict on tcp:0.0.0.0:8080: component gateway port serve and " +
				"component gateway port accept both trying to bind",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := BuildFromDeclarations(tt.declarations(t))

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Nil(t, graph)
		})
	}
}

func TestValidateStreamRequirementsFlagsCoreNATSPublishers(t *testing.T) {
	jetStreamInput := component.Port{
		Name:      "orders",
		Direction: component.DirectionInput,
		Config:    component.JetStreamPort{StreamName: "ORDERS", Subjects: []string{"orders.>"}},
	}

	t.Run("core NATS publishers", func(t *testing.T) {
		graph, err := BuildFromDeclarations([]component.Declaration{
			declaration(t, "alpha", nil, []component.Port{natsPort("out", component.DirectionOutput, "orders.new")}),
			declaration(t, "bravo", nil, []component.Port{natsPort("out", component.DirectionOutput, "orders.paid")}),
			declaration(t, "ledger", []component.Port{jetStreamInput}, nil),
		})
		require.NoError(t, err)

		warnings := graph.ValidateStreamRequirements()

		require.Len(t, warnings, 1)
		assert.Equal(t, "ledger", warnings[0].SubscriberComp)
		assert.Equal(t, "orders", warnings[0].SubscriberPort)
		assert.Equal(t, []string{"orders.>"}, warnings[0].Subjects)
		assert.Equal(t, []string{"alpha", "bravo"}, warnings[0].PublisherComps)
	})

	t.Run("one publisher with two feeding ports is named once", func(t *testing.T) {
		graph, err := BuildFromDeclarations([]component.Declaration{
			declaration(t, "alpha", nil, []component.Port{
				natsPort("new", component.DirectionOutput, "orders.new"),
				natsPort("paid", component.DirectionOutput, "orders.paid"),
			}),
			declaration(t, "ledger", []component.Port{jetStreamInput}, nil),
		})
		require.NoError(t, err)
		// Both ports feed the input, so a single name is the dedup, not a
		// missing edge.
		require.Len(t, graph.GetEdges(), 2)

		warnings := graph.ValidateStreamRequirements()

		require.Len(t, warnings, 1)
		assert.Equal(t, []string{"alpha"}, warnings[0].PublisherComps)
	})

	t.Run("warnings sort by subscriber component, then port", func(t *testing.T) {
		// "graph-index-spatial" sorts before "graph-index:" as a joined
		// string ('-' < ':'), so only a component-first sort passes.
		second := jetStreamInput
		second.Name = "later"
		graph, err := BuildFromDeclarations([]component.Declaration{
			declaration(t, "alpha", nil, []component.Port{natsPort("out", component.DirectionOutput, "orders.new")}),
			declaration(t, "graph-index-spatial", []component.Port{jetStreamInput}, nil),
			declaration(t, "graph-index", []component.Port{second, jetStreamInput}, nil),
		})
		require.NoError(t, err)

		var got [][2]string
		for _, w := range graph.ValidateStreamRequirements() {
			got = append(got, [2]string{w.SubscriberComp, w.SubscriberPort})
		}

		assert.Equal(t, [][2]string{
			{"graph-index", "later"},
			{"graph-index", "orders"},
			{"graph-index-spatial", "orders"},
		}, got)
	})

	t.Run("JetStream publisher", func(t *testing.T) {
		jetStreamOutput := component.Port{
			Name:      "out",
			Direction: component.DirectionOutput,
			Config:    component.JetStreamPort{StreamName: "ORDERS", Subjects: []string{"orders.>"}},
		}
		graph, err := BuildFromDeclarations([]component.Declaration{
			declaration(t, "alpha", nil, []component.Port{jetStreamOutput}),
			declaration(t, "ledger", []component.Port{jetStreamInput}, nil),
		})
		require.NoError(t, err)
		// The pair is connected, so an empty result is the check passing, not
		// a missing edge.
		require.Len(t, graph.GetEdges(), 1)

		assert.Empty(t, graph.ValidateStreamRequirements())
	})
}

// TestFlowGraphOutputIsTheSameEveryRun holds the package to the order
// composition reads it in: components visited by instance name, so equal
// declarations give an equal refusal and equal results every run.
func TestFlowGraphOutputIsTheSameEveryRun(t *testing.T) {
	const runs = 50

	t.Run("graph mutation refusal names the first component by name", func(t *testing.T) {
		var declarations []component.Declaration
		for _, name := range []string{"charlie", "alpha", "bravo"} {
			legacy := component.Port{
				Name:      "mutations",
				Direction: component.DirectionOutput,
				Config:    component.NATSRequestPort{Subject: "graph.mutation.entity.create"},
			}
			declarations = append(declarations, declaration(t, name, nil, []component.Port{legacy}))
		}

		for run := range runs {
			_, err := BuildFromDeclarations(declarations)
			require.Error(t, err)
			require.Contains(t, err.Error(), "component alpha port mutations is not a required nats-request",
				"run %d", run)
		}
	})

	t.Run("connected components follow name order", func(t *testing.T) {
		declarations := []component.Declaration{
			declaration(t, "delta", []component.Port{natsPort("in", component.DirectionInput, "b.x")}, nil),
			declaration(t, "charlie", []component.Port{natsPort("in", component.DirectionInput, "a.x")}, nil),
			declaration(t, "bravo", nil, []component.Port{natsPort("out", component.DirectionOutput, "b.x")}),
			declaration(t, "alpha", nil, []component.Port{natsPort("out", component.DirectionOutput, "a.x")}),
		}
		want := [][]string{{"alpha", "charlie"}, {"bravo", "delta"}}

		for run := range runs {
			graph, err := BuildFromDeclarations(declarations)
			require.NoError(t, err)
			require.Equal(t, want, graph.AnalyzeConnectivity().ConnectedComponents, "run %d", run)
		}
	})
}

// TestMatchNATSPatternIsSymmetric checks the one law edge derivation relies on
// when it asks the matcher once per pair of connection IDs: the answer does not
// depend on argument order. The subjects reach each case the matcher branches
// on: equal, literal only, `*` and `>` on either or both sides, and different
// lengths.
func TestMatchNATSPatternIsSymmetric(t *testing.T) {
	subjects := []string{
		"a", "a.b", "a.c", "a.b.c",
		"*", "a.*", "*.b", "a.*.c", "*.*",
		">", "a.>", "a.b.>", "*.>",
	}
	var matched, unmatched int
	for _, a := range subjects {
		for _, b := range subjects {
			forward, reverse := matchNATSPattern(a, b), matchNATSPattern(b, a)
			if forward != reverse {
				t.Errorf("matchNATSPattern(%q, %q) = %v, but matchNATSPattern(%q, %q) = %v",
					a, b, forward, b, a, reverse)
			}
			if a != b && forward {
				matched++
			} else if !forward {
				unmatched++
			}
		}
	}
	// The law is not checked over a set that never matches or always does.
	if matched == 0 || unmatched == 0 {
		t.Fatalf("the set must give both answers: %d distinct pairs matched, %d did not", matched, unmatched)
	}
}

// TestRefusalsNameNoInternalFunction drives each of the package's five
// configuration refusals through BuildFromDeclarations. Each one wraps
// errs.ErrInvalidConfig, and its text says what is wrong in the operator's
// words, naming no function of this package.
func TestRefusalsNameNoInternalFunction(t *testing.T) {
	legacyRequester := component.Port{
		Name:      "legacy",
		Direction: component.DirectionOutput,
		Config:    component.NATSRequestPort{Subject: "graph.mutation.entity.create"},
	}
	tests := []struct {
		name         string
		declarations func(t *testing.T) []component.Declaration
		want         string
	}{
		{
			name: "empty instance name",
			declarations: func(t *testing.T) []component.Declaration {
				return []component.Declaration{declaration(t, "", nil, nil)}
			},
			want: `add component "" to flow graph: component name cannot be empty: invalid configuration`,
		},
		{
			name: "duplicate instance name",
			declarations: func(t *testing.T) []component.Declaration {
				return []component.Declaration{declaration(t, "twin", nil, nil), declaration(t, "twin", nil, nil)}
			},
			want: `add component "twin" to flow graph: component twin already exists: invalid configuration`,
		},
		{
			name: "network bind conflict",
			declarations: func(t *testing.T) []component.Declaration {
				return []component.Declaration{
					declaration(t, "first", nil, []component.Port{networkPort("listen", component.DirectionOutput)}),
					declaration(t, "second", nil, []component.Port{networkPort("listen", component.DirectionOutput)}),
				}
			},
			want: "connect component flow graph: network port conflict on tcp:0.0.0.0:8080: " +
				"multiple components binding: component first port listen, component second port listen: " +
				"invalid configuration",
		},
		{
			name: "two conflicts at once",
			declarations: func(t *testing.T) []component.Declaration {
				writer := func(name string) component.Port {
					return component.Port{
						Name: name, Direction: component.DirectionOutput,
						Config: component.KVWritePort{Bucket: "entity_states"},
					}
				}
				return []component.Declaration{
					declaration(t, "first", nil, []component.Port{networkPort("listen", component.DirectionOutput)}),
					declaration(t, "second", nil, []component.Port{networkPort("listen", component.DirectionOutput)}),
					declaration(t, "writer1", nil, []component.Port{writer("state")}),
					declaration(t, "writer2", nil, []component.Port{writer("state")}),
				}
			},
			want: "connect component flow graph: multiple writers to KV bucket kv:entity_states: " +
				"component writer1 port state, component writer2 port state; " +
				"network port conflict on tcp:0.0.0.0:8080: " +
				"multiple components binding: component first port listen, component second port listen: " +
				"invalid configuration",
		},
		{
			name: "malformed graph mutation port",
			declarations: func(t *testing.T) []component.Declaration {
				return []component.Declaration{declaration(t, "requester", nil, []component.Port{legacyRequester})}
			},
			want: "connect component flow graph: component requester port legacy is not a required nats-request " +
				"semengine.graph.mutation v1 port on graph.mutation.>: invalid configuration",
		},
		{
			name: "no graph mutation provider",
			declarations: func(t *testing.T) []component.Declaration {
				return []component.Declaration{
					declaration(t, "requester", nil, []component.Port{mutationPort(component.DirectionOutput)}),
				}
			},
			want: "connect component flow graph: graph mutation flow requires exactly one compatible provider, " +
				"found 0: invalid configuration",
		},
	}
	internalNames := []string{
		"FlowGraph.", "AddComponentNode", "addComponentNode", "ConnectComponentsByPatterns",
		"connectComponentsByPatterns", "validateGraphMutationProviders",
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := BuildFromDeclarations(tt.declarations(t))

			require.Error(t, err)
			assert.Nil(t, graph)
			assert.True(t, errors.Is(err, errs.ErrInvalidConfig), "error %q does not wrap errs.ErrInvalidConfig", err)
			for _, internal := range internalNames {
				assert.False(t, strings.Contains(err.Error(), internal), "error %q names %s", err, internal)
			}
			assert.EqualError(t, err, tt.want)
		})
	}
}
