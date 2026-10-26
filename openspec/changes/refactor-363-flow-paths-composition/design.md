# Design: refactor-363-flow-paths-composition

Base `39a17a9`. Inventory: `inventory.md` (INVENTORY PASS, commit `2205025`). Retires #363 (`Closes #363`, tasks
2.2-2.3).

## Context

- The component manager runs `composition.Analyze` once, at construction, and keeps the result
  (`service/component_manager.go:357-358`). Nothing writes that result afterwards.
- `/components/validate` serves the result as is (`service/component_manager_http.go:606`). `/components/flowgraph`
  serves its graph, or `composition.Mermaid(result.Graph)` with `format=mermaid` (`:584`).
- `/components/paths` instead reads the graph and decides two things inside `service`:
  - which nodes are origins: `isInputNode`, `service/component_manager.go:631-642`, true for type `input` or an input
    port of pattern `network` or `http-client`;
  - what each origin reaches: `reachableFrom`, `:647-664`, depth-first in the projection's sorted edge order
    (`composition/graph.go:102`).
- The handler then counts that answer (`service/component_manager_http.go:642-656`) and sends it with a timestamp
  (`:658-666`).
- The traversal needs only `composition.Graph` plus the constants `types.ComponentTypeInput`,
  `component.PatternNetwork` and `component.PatternHTTPClient`. `composition` already imports both of those packages
  (`composition/catalog.go:8`, `composition/graph.go:7`), so the move adds no import edge.

## Options

- **A. Do nothing.** The cost: ADR-100 decision 3 stays unmet for one route, and four statements stay untrue:
  - `openspec/specs/operator-surface/spec.md:273-274`;
  - `service/component_manager_http.go:623-625`;
  - `service/component_manager.go:53-56`;
  - `service/README.md:337`.
  Fixing only the wording would make them true while leaving the work in the wrong package.
- **B. `composition.FlowPaths(graph Graph) map[string][]string`; the handler keeps the counts (recommended).**
  - Mirrors `Mermaid(graph)`: a pure function of the projection, called by the route.
  - Adds one exported function and no type.
  - `GetFlowPaths` keeps its signature, so no caller changes.
  - The counts stay in the handler. They are `len` and `max` over the answer `composition` returned, not a reading
    of the analysis, so the reworded statements say exactly that.
- **C. As B, but the counts move too:** `composition.FlowPaths(graph) FlowPathsReport` with `Paths` and `Statistics`
  fields, JSON-tagged.
  - Every number in the body would then come from the library.
  - The costs: a new exported type whose JSON tags become `composition`'s contract, a changed or wrapped
    `GetFlowPaths` signature, and the timestamp still added in `service`.
  - It is more surface for about 15 lines of arithmetic.
- **D. As B, and `GetFlowPaths` goes; the handler calls `composition` directly.** This removes an exported method the
  ledger keeps (`docs/admission-ledger.yaml:2883`) and that four tests call
  (`service/component_manager_paths_test.go:77`, `service/component_manager_port_facts_test.go:157`,
  `service/component_manager_integration_test.go:70,91`), which saves one line. Removing exported surface is outside
  a port-refactor's scope without an owner ruling.

Recommendation: **B**, the smallest change mirroring a shape the code already accepts.

## D1. `composition.FlowPaths` answers reachability from each origin

- `FlowPaths(graph)` returns a non-nil map with one key per origin. An origin is a node of type `input`, or a node with
  an input port whose pattern is `network` or `http-client`.
- Each value is the origin followed by every node reachable from it along the graph's edges, each node once, in
  depth-first order over the edges in the order the graph holds them.
- It stores nothing and reads nothing but its argument.
- The origin rule and the walk are the pin's, moved unchanged. Their unexported helpers live in a new
  `composition/flowpaths.go` beside `mermaid.go`.
- Who it is for: the production caller is `service/component_manager.go` (`GetFlowPaths`). The contract line is the
  ADDED `composition-validation` requirement. Offline callers such as `composition/cli` (change 7) can also use it, but
  that does not justify it here.

Invariants, each from the ADDED requirement:

- every key is an origin and every origin is a key;
- each value begins with its key;
- no value repeats a node;
- every node in a value is reachable from the key;
- equal graphs give equal answers.

Example tests suffice here, with no generated checks (`docs/testing.md`). The input is one graph value with no history
and no interacting inputs beyond the cases below, and the oracle is a hand-written expected map.

Proving tests (new, `composition/flowpaths_test.go`), over hand-built `Graph` values:

- `TestFlowPathsKeysEachOrigin`: each of the three origin rules alone makes an origin; a processor with only NATS
  inputs does not.
- `TestFlowPathsWalksDepthFirstInEdgeOrder`: a branching graph gives the exact order.
- `TestFlowPathsVisitsEachNodeOnce`: a cycle back to the origin ends, with each node listed once.
- `TestFlowPathsOverAnEmptyGraph`: the answer is non-nil and empty.

Kept unchanged in `service`:

- `TestFlowPathsTraverseTheRetainedGraph` (`service/component_manager_paths_test.go:21`), which now proves that
  `GetFlowPaths` reads the kept graph;
- the `EmptyComposition` subtest of `TestComponentManagerProjectionCarriesOnlyAdmittedInstances`;
- `TestComponentManagerFlowReportingUsesRetainedPortsAfterComponentMutation`;
- `TestComponentManagerWithRealNATS` and `TestComponentManagerCompositionProjection`.

## D2. The paths route serves the library's answer, counted

- `GetFlowPaths` returns `composition.FlowPaths(result.Graph)`. It keeps its transient refusal when no result is kept,
  which is unreachable (ledger `:3012-3013`).
- `/components/paths` keeps its body:
  - `paths`;
  - `statistics`: `input_component_count` is the number of keys; `max_path_length` is the longest value;
    `total_reachable` is the sum of value lengths; `avg_path_length` is that sum divided by the key count, or 0 with
    no keys;
  - `timestamp`.
- The route derives no graph, origin, reachability or severity of its own.
- The four statements become true by rewording, not by moving the counts. Each says that `validate` serves the kept
  result as is, `flowgraph` serves its graph (as JSON or as `composition.Mermaid`), and `paths` serves
  `composition.FlowPaths` of its graph with counts of that answer. They are:
  - spec `operator-surface:273-274` (the MODIFIED requirement);
  - the handler comment `service/component_manager_http.go:623-625`;
  - the field comment `service/component_manager.go:53-56`;
  - `service/README.md:337-338`.

Proving test (new, task 2.1): `TestFlowPathsRouteServesTheLibraryAnswer`.

- It asks the handler, over a composition whose origins reach paths of different lengths, so the average is not a
  whole number.
- It asserts the `paths` object and all four `statistics` values against hand-written numbers, and that `timestamp`
  parses as RFC 3339; it does not compare the timestamp's value.
- It is written and passes on the base, before `FlowPaths` exists, so it proves the answer is unchanged across the
  move.

The flowgraph golden test (`TestFlowGraphMatchesGoldenForFixtureComposition`) never runs this code. It stays green
because nothing it reads changes, but it is not this change's proof.

## D3. The three "fed from outside" spellings stay

Three places decide whether an input comes from outside the composition:

| Spelling | Where | What it decides |
| --- | --- | --- |
| `isInputNode`, now `FlowPaths`'s origin rule | `service/component_manager.go:631` | where a data path starts: type `input`, or a `network` or `http-client` input |
| flowgraph's orphan skip | `internal/flowgraph/flowgraph.go:786` | which unmatched inputs are not orphaned: those two patterns plus `store` and `timer` |
| `externalInputs` | `composition/analyze.go:290` | which ports are declared `External` and exempt from a no-publisher finding |

They answer three different questions, and their pattern lists differ on purpose: a timer input is not a data source,
yet it is not an orphan either. Under Bounded port scope (`.agents/protocol.md`) consolidating them is optional
inherited maintenance with no binding basis: no admission gate, ruling or spec line requires one rule. Moving the
origin rule into `composition` puts the first and third in one package, which is as close as this change needs.

This change does not file an issue, because this is a choice, not a defect. Changing which nodes count as origins
(for example, a port declared `External`) is a non-goal of this change (`proposal.md`, Non-goals).

## Ledger

`service` row:

- `docs/admission-ledger.yaml:2883`: "GetFlowPaths" becomes "GetFlowPaths (which returns composition.FlowPaths of the
  kept graph, #363)".
- `:3012-3013`: unchanged.

`composition` row:

- contract `:3239`: "Analyze, Catalog and Mermaid otherwise as at the pin" becomes "FlowPaths answers, for each origin
  of a projection, every node reachable from it, moved from service's GetFlowPaths with its answers unchanged (#363);
  Analyze, Catalog and Mermaid otherwise as at the pin".
- `proving_tests` gains "flowpaths_test.go (TestFlowPathsKeysEachOrigin, TestFlowPathsWalksDepthFirstInEdgeOrder,
  TestFlowPathsVisitsEachNodeOnce, TestFlowPathsOverAnEmptyGraph)".
- Neither row's disposition changes (`service` adapt, `composition` repair-before-port). Neither is a `carry` row, so
  `ledger:check` compares neither with the pin.

## ADR-100 and ADR-113

- ADR-100 decision 3 is kept, and now met for `/paths`.
- Decision 4 (`:68`) names only `catalog`, `validate` and `graph` as cross-repo verbs. Paths is not one, so no
  cross-repo contract changes. The constraint on `GetFlowPaths` is that it is exported and listed in the ledger
  (`:2883`), and it is kept.
- ADR-113: no context is created, held or passed; no `Start` or `Stop` changes.

## Merge order

- This PR lands before #362 (owner, #363 comment 6384691294).
- Draft #414 (graph-index, change 4 PR 4) has no files yet. It will probably edit `docs/admission-ledger.yaml` and
  perhaps `scripts/cover-check.sh`, so only text conflicts are expected: whichever lands second merges `origin/main`.

## Adoption

Not an establishing change: `FlowPaths` follows the existing shape of `Mermaid`. There is no primitive for other
packages to adopt.
