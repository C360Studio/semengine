# composition-validation delta: refactor-363-flow-paths-composition

## ADDED Requirements

### Requirement: FlowPaths answers reachability from each origin

`composition.FlowPaths(graph)` SHALL return a non-nil map with one key per origin: a node of type `input`, or a node
with an input port of pattern `network` or `http-client`. Each value SHALL be the origin followed by every node
reachable from it along the graph's edges, each node once, in depth-first order over the edges in the order the graph
holds them. It SHALL store nothing and read nothing but its argument.

#### Scenario: Each origin rule alone makes an origin

- **WHEN** a graph holds a node of type `input`, a processor with a `network` input, a processor with an
  `http-client` input, and a processor with only NATS inputs
- **THEN** the first three are keys and the fourth is not
- **AND** the test that verifies this is `TestFlowPathsKeysEachOrigin`

#### Scenario: Depth-first in edge order

- **WHEN** an origin's edges branch to two nodes and the first branch continues further
- **THEN** its value lists the origin, the whole first branch, then the second branch
- **AND** the test that verifies this is `TestFlowPathsWalksDepthFirstInEdgeOrder`

#### Scenario: A cycle

- **WHEN** an edge leads back to the origin
- **THEN** the walk ends and each node appears once
- **AND** the test that verifies this is `TestFlowPathsVisitsEachNodeOnce`

#### Scenario: An empty graph

- **WHEN** the graph has no nodes
- **THEN** the answer is a non-nil, empty map
- **AND** the test that verifies this is `TestFlowPathsOverAnEmptyGraph`
