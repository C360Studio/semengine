# Inventory: refactor-363-flow-paths-composition

base: 39a17a959216fe4f82564b89caf6df5ea671c889 (main; claim commit 43c1f14 is empty)

Port-refactor #363 of `service`, carried from the SemStreams pin `8b99efe9`. No new package is carried, so no pin
probe runs; pin statements cite pin files read through `gh api contents?ref=8b99efe9`.

## 1. The claimed gap: definitions, callers, tests

- `service/component_manager.go:603` — `func (cm *ComponentManager) GetFlowPaths() (map[string][]string, error) {`
  (body `:604-623`: reads `cm.bootCompositionResult()`, builds `adjacency` from `result.Graph.Edges` at `:612-615`,
  keeps nodes passing `isInputNode` at `:617-622`). Doc comment `:595-602`.
- `service/component_manager.go:631` — `func isInputNode(node composition.Node) bool {` (`:632` type is
  `types.ComponentTypeInput`; `:636-638` an input port of `component.PatternNetwork` or `component.PatternHTTPClient`).
- `service/component_manager.go:647` — `func reachableFrom(start string, adjacency map[string][]string) []string {`
  (depth-first, start first, order taken from the projection's sorted edges).
- Route: `service/component_manager_http.go:79` — `mux.HandleFunc(prefix+"paths", cm.handleFlowPaths)`; handler
  `:626` — `func (cm *ComponentManager) handleFlowPaths(w http.ResponseWriter, r *http.Request) {`, which calls
  `GetFlowPaths` at `:632` and then computes `statistics` (`:642-666`: `input_component_count`, `max_path_length`,
  `avg_path_length`, `total_reachable`) plus `"timestamp": time.Now().UTC()` (`:659`). OpenAPI entry `:257-268`
  (`"/paths"`, no `SchemaRef`).
- Production callers of `GetFlowPaths`: only `service/component_manager_http.go:632` (`git grep -n GetFlowPaths`).
  `isInputNode` and `reachableFrom` are called only from `GetFlowPaths` (`:618`, `:621`).
- Tests:
  - `service/component_manager_paths_test.go:21` — `func TestFlowPathsTraverseTheRetainedGraph(t *testing.T) {`
    (all three origin arms, exact depth-first answers, `:77-95`).
  - `service/component_manager_port_facts_test.go:157` — `paths, err := manager.GetFlowPaths()` (subtest
    `EmptyComposition` of `TestComponentManagerProjectionCarriesOnlyAdmittedInstances`, `:134`).
  - `service/component_manager_port_facts_test.go:207` — `"paths": {manager.handleFlowPaths, func(*testing.T, []byte) {}},`
    (`TestComponentManagerFlowReportingUsesRetainedPortsAfterComponentMutation`, `:167`; checks the status only).
  - Integration: `service/component_manager_integration_test.go:70` (`TestComponentManagerWithRealNATS`, `:59`) and
    `:91` (`TestComponentManagerCompositionProjection`, `:84`); both use an empty composition.
- Golden files: `service/testdata/flowgraph_fixture.golden.json` and `.golden.mmd`, read by
  `service/component_manager_http_test.go:371` — `func TestFlowGraphMatchesGoldenForFixtureComposition`. They cover
  `/components/flowgraph` only. No golden file or test asserts the `/components/paths` body
  (`git grep -n 'input_component_count\|total_reachable' -- '*_test.go'`: empty).

## 2. What `composition` exposes, and how it overlaps with other spellings

- `composition/findings.go:92` — `type Result struct {` with `Graph Graph` at `:96`. `composition/graph.go:14`
  — `type Graph struct {` (`Nodes []Node`, `Edges []Edge`); `Node` `:20-26` (`Instance`, `Type`, `Inputs []PortView`);
  `PortView.Pattern` `:36`; `Edge.From`/`To` `:43,45`.
- `composition/analyze.go:28` —
  `func Analyze(declarations []component.Declaration, streams config.StreamConfigs) *Result {`.
  Edges are sorted once by `compareEdges` (`composition/graph.go:102,108`), which `GetFlowPaths` relies on for order.
- Closest existing function over the retained graph: `composition/mermaid.go:13` — `func Mermaid(graph Graph) string {`,
  served by `service/component_manager_http.go:584`.
- Does the service traverse exactly what composition keeps? Yes. `service/component_manager.go:357-358` —
  `result := composition.Analyze(cm.registry.Snapshots(), cm.bootStreams)` / `cm.bootFindings = result`. That is
  the only write (`grep -n bootFindings service/*.go`). `bootCompositionResult` (`:388-390`) returns it, and the
  traversal reads nothing else.
- Hidden dependencies: the only service state the traversal reads is that one field. The traversal also needs
  `types.ComponentTypeInput` and `component.PatternNetwork`/`PatternHTTPClient`. `composition` already imports both
  packages (`composition/catalog.go:8` `types`, `composition/graph.go:7` `component`), so no new import edge, and
  `composition` does not import `service`.
- The early return on a failed graph build (`composition/analyze.go:58-71`) leaves edges empty, but it records
  `connection_pattern_error`, which refuses boot. A manager cannot serve that result.
- Nil result: the ledger records "GetFlowPaths keeps its nil-result refusal, now unreachable"
  (`docs/admission-ledger.yaml:3012-3013`).
- Other spellings of "this input is fed from outside" (an inherited cost; whether to consolidate is a Bounded port
  scope question for the design):
  - `isInputNode`: type `input`, or a Network/HTTPClient input.
  - `internal/flowgraph/flowgraph.go:786`: orphan skip for Network/HTTPClient/Store/Timer.
  - `composition/analyze.go:290` — `func externalInputs(...)`: `port.External`.
- Other traversal: `internal/flowgraph/flowgraph.go:720,747` (`findConnectedComponents`/`dfs`) treats edges as
  undirected for connectivity. That is a different fact, not a second reachability spelling.

## 3. Does the premise still hold on main?

Yes, measured. The body at `service/component_manager.go:603-665` matches pin `service/component_manager.go:1405-1475`
line for line, apart from the doc comment. The issue's pin citation is accurate.

`openspec/specs/operator-surface/spec.md:273-274` says `/components/paths` "SHALL project the composition result ... and
SHALL compute nothing". Today the route computes two things: reachability (`GetFlowPaths`) and the statistics
(`handleFlowPaths:642-656`). So that spec line is currently untrue for `/paths`. ADR-100 decision 3
(`docs/adr/100-compositions-are-validated-diagrams-are-projections.md:61-63`: "the HTTP handler ... stop
re-interpreting the analysis; the library is the one home") is the binding basis.

The issue's precondition "after #30" holds: #30 is CLOSED.

Two corrections to the issue's Expected:

- The flowgraph golden test never runs the paths code, so it proves nothing about this move.
- The `/paths` body carries `time.Now()`, so it cannot be byte-identical across runs, and no test asserts it today.

## 4. Adjacent claims

- Open PRs (`gh pr list --state open --json number,title,isDraft,changedFiles,files`):
  - #413 is this claim (draft, 0 files).
  - #414 (draft, graph-index, change 4 PR 4 of 9) has 0 files so far, so no overlap can be measured yet. Re-check
    before the design states merge order.
- Open issues (`gh issue list --state open --search` on `composition`, `GetFlowPaths`, `components/paths`,
  `component_manager.go`, `flowpaths`, `flow paths`, `isInputNode`, `reachableFrom`):
  - #363: retired here (`Closes #363` in #413).
  - #362: stays, serial after this PR (owner, #363 comment 6384691294).
  - #196: stays. It cites `service/component_manager.go:102` for the lifecycle guard, not this code.
  - #280: stays. Item 2 (composition reads port lists in pairs) was settled by service PR 4; items 1 and 3 are not
    this code.
  - #16: stays. The reserved-subject work in `composition` was done in an earlier change; #16 is not this code.
  - #361, #106, #195, #24, #96, #9, #7: use the word, not this package or route.
- Specs:
  - `openspec/specs/operator-surface/spec.md:273` (requirement "The composition projections serve the result kept at
    construction"). No scenario covers `/paths`; scenarios exist only for flowgraph (`:277-281`) and validate
    (`:283-287`).
  - `openspec/specs/operator-surface/spec.md:230-231` lists the route.
  - `openspec/specs/composition-validation/spec.md:141-145` ("The catalog and the projection are read-only views":
    `Catalog`, `Mermaid`) is the requirement a composition-side function would sit beside.
  - Purpose line `:3-9` names `Validate`, `Analyze`, `Catalog`, `Mermaid`.
- Docs naming the route or call:
  - `service/README.md:335,337-338` ("`GetFlowPaths` is the same projection as a call").
  - `docs/contract.md:138`.
  - Stale, found on the way: `composition/doc.go:19` — "The boot caller is service's component manager, not yet
    ported". It is a defect in repo code, so the design files it or fixes it.
- Ledger:
  - `service` row (`docs/admission-ledger.yaml:2837`, disposition `adapt` `:3098`): `:2883` "read inside the package
    and kept as at the pin: ... GetFlowPaths"; `:3012-3013` the nil-result refusal.
  - `composition` row (`:3219`, disposition `repair-before-port` `:3263`): contract `:3238` "Analyze, Catalog and
    Mermaid otherwise as at the pin"; proving tests `:3254-3259`.
  - The move flips neither disposition. Both rows' contract/known_risks text needs an edit, and `composition`'s
    proving_tests gains the moved test.
- Coverage: `scripts/cover-check.sh:48` — `"composition unit"`; `:45` `"service merged"`; threshold 80 (`:18`).

## 5. The problem shape

A read-only projection over the retained `composition.Graph`, computed by the library and served verbatim by a
`service` handler. Closest instance: `composition.Mermaid(result.Graph)` at `service/component_manager_http.go:584`
(spec `composition-validation:141-145`, `TestMermaidIsDeterministic`).

## 6. Pin decision records

ADR-100 governs (decisions 1 and 3; ported at `docs/adr/100-*.md`, ledger row after `:3263`). The 85 pin ADRs were
searched for `GetFlowPaths`, `components/paths`, `isInputNode`, `reachableFrom`, `FlowPaths`: no match. `flow path` and
`reachab` hit 17 records (ADR-075 `:90` "reachability contracts", ADR-098 `:27,98`, and others), all using the word for
other subjects.

## Who it is for (category 4)

The inventory introduces no surface. Any new exported `composition` symbol the design proposes needs a named
caller. Production candidate: `service/component_manager.go:603`. Otherwise it needs a contract line in
`composition-validation`.

## Adopter seam inventory

Reached from outside:

- the `/components/paths` JSON (route, keys `paths`, `statistics.*`, `timestamp`);
- the exported `ComponentManager.GetFlowPaths` (ledger `:2883`; ADR-100 decision 4 makes the projection a
  cross-repo contract).

Consumer callers of `GetFlowPaths` were not measured: `docs/inventory-scope.md:14-17` scopes semsource and semconnect
to closure and imports, and semteams to rule, lifecycle and agentic symbols. The ledger records that semteams reflects
`composition.Result` and `Graph` in its OpenAPI generator test (`:3224`).

If the route body and `GetFlowPaths` keep their signature and answers, an adopter needs to know nothing new. A new
exported `composition` function is a new framework surface, with its own one-line contract to state.
