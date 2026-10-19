# Design: port-flowgraph

Pin `8b99efe9`. Ledger row: `component/flowgraph` (below). Inventory: `inventory.md` in this change (`INVENTORY
PASS`, round 1).

## Destination

`internal/flowgraph`, by the owner's public-signature rule (#9 comment 5953295358, 2026-10-02): a package is
public "if a consumer imports it, or if an exported signature in a public package names one of its types", and the
rule "is meant to stand for later changes". Both are measured false (inventory, "Who reads it"): no named consumer
imports it, and every pin use in `composition` is unexported. SETUP 03B D16 (Q17) sets the same default, "internal
by default". `task ledger:check` maps the move for `composition`'s files when they land
(`internal/harness/pindiff/ledger.go:90-112`). This is a notice to the owner, not a question.

## Options

- A. Port to `internal/flowgraph` with the required corrections below (recommended; the ruled destination).
- B. The same work at `component/flowgraph`. A departure from the 2026-10-02 ruling: a public package nothing
  outside the module imports, with `NewFlowGraph` and `ConnectComponentsByPatterns` as a second public way to build
  a graph beside "the one construction seam" (pin `composition-validation/spec.md:283`).
- C. Required build and gate fixes only (R1, R2), surface kept, docs as at the pin, no new tests. Cost: ports dead
  and advertised-absent surface knowingly (Surface audit (a) and (c)); leaves composition's two entry points untested
  until composition lands.
- D. Port it inside composition's PR. Cost: two packages in one PR, against the one-package rule (#159).

## Decision (A)

Required corrections, each with its basis:

- R1. `flowgraph.go:264` spells `"graph.mutation."`; take the prefix from `graphmutation.SubjectFamily` as
  `processor/graph-ingest/canonical_mutations.go:120` does. Basis: `TestReservedSubjectsDeclaredOnce` fails on the
  pin file (inventory, Pin probe; graph-transport-boundary spec). Behaviour unchanged.
- R2. The tests' mock uses `component.Metadata`, which this tree's `component` deleted (ledger `component`
  adaptation 9): drop the `metadata` field and the `Meta()` and `ConfigSchema()` methods
  (`flowgraph_test.go:489-492`, `:500`, `:505-507`, `:517-519`) and the field literal at
  `flowgraph_patterns_test.go:375-377`. The parameters that leaves unused,
  `componentType` and `name` at `flowgraph_test.go:485` and `name` at `flowgraph_patterns_test.go:373`, are renamed
  `_` (the smallest change; callers stay as they are). Basis: the package's tests must build and `task lint` pass.
- R3. Output in name order: the graph-mutation check (`:262`) and `findConnectedComponents` (`:868`) range over
  `g.sortedNodeNames()`, as the rest of the package does. Equal declarations then give the same refusal text and
  the same `ConnectedComponents` on every run. Basis: a demonstrated defect (inventory, behaviour probe: three texts
  in 50 runs) against the carried contract composition binds, "Components SHALL be visited in instance-name order"
  and byte-equal results (pin `composition-validation/spec.md:100-101`); the real-world trigger is a configuration
  with two malformed graph-mutation ports.
- R4. Dead surface removed (Surface audit (a); `.agents/protocol.md` Bounded port scope: "truly dead surface is not
  protected"): `GetNodes` with its deep copy and `TestGetNodesReturnsIndependentInterfaceContracts`;
  `EdgeMetadata` and `FlowEdge.Metadata`; `FlowAnalysisResult.ConnectedEdges` (its one assert reads `GetEdges`);
  `StreamRequirementWarning.Severity` and `.Issue`. With `Metadata` gone, `GetEdges` copies hold no shared pointer.
  With `GetNodes` gone, `ComponentNode` and `PortInfo` are named by no exported function or field, so they become
  `componentNode` and `portInfo` (in-package renames only; their fields keep their names).
  `TestFlowGraphConstruction`'s `GetNodes` reads become reads of `AnalyzeConnectivity` (a lone node is reported
  disconnected). Kept: `ConnectedComponents`, which the pin spec requires (`composition-validation/spec.md:281`);
  its contract line is the ledger row until composition brings that spec.
- R5. Each item in the inventory's "behaviour described and not implemented" list is corrected (Surface audit (c)):
  `README.md` and `doc.go` describe what the code does, for a working developer, naming `composition` as the
  reader, and the `component_node_test.go:11` comment names `BuildFromDeclarations`, not `BuildFromRegistry`.
  Statements about NATS stay the package's own rule ("this package's matcher treats `*` as one token and `>` as one
  or more"), not a claim about the server, so no probe is owed (#170). The sentence that a subscriber "hangs" is
  dropped, not restated.
- R6. Tests for what the ledger row's `contract` claims and no test in the tree reaches: `BuildFromDeclarations`
  and `ValidateStreamRequirements` (0.0% at the pin; the pin proves them only through `composition`'s tests), and
  three refusals with no hit in the pin's coverage: a port with no connection identity (`:197`), two exclusive
  output binds on one network address (`:754`), and an input and an output binding the same address (`:770`).
  Basis: every behaviour a ledger `contract` names has a proving test (`proving_tests`), and none exists here.
- R7. `TestGraphMutationFlowRejectsInvalidProviderTopology` gains a provider row with `Required: false`, so the
  required-port condition (`:272`), which carries ADR-091 §2's one-provider rule, is shown able to fail.

Retained, optional under Bounded port scope (inherited; no binding requirement, gate or ruling names them):
shape findings 1a, 1b, 1c, 1f, the interface-alternative heuristic, the JSON tags, `NewFlowGraph` and
`ConnectComponentsByPatterns` staying exported inside `internal/`, `TestOrphanedPortSeverity` (not listed as a
proving test), and the refusal text naming internals (composition's PR). Costs are recorded here and in the
ledger's `known_risks`.

Not changed: no context, goroutine, I/O or lifecycle; no `GenericJSON`; no spec capability (the contract line is the
ledger row, as for `internal/logforwarderpolicy`; the pin's `composition-validation` and the mutation-topology
requirement of `framework-composition` land with `composition`). Examples suffice over generated checks: this change
alters neither subject matcher (`SubjectCovers`, `matchNATSPattern`), and the pin's cases for them run unchanged.

## ADRs

- ADR-100 D1 and D3: kept; R4's removal of `Severity` matches D3. Not carried here: its main subject is
  `composition` (D3, D4) and it supersedes clauses of ADR-094 and ADR-096, which come with `service`; carry it with
  `composition` (Q1).
- ADR-091 §2: kept unchanged (exactly one provider, any number of requesters); ADR-110 `:69` already records it.
- ADR-063 item 5: kept unchanged (store fan-in edges, store ports never orphans). Not carried: its subject is the
  store resolver (`storage/storeregistry`, ported in change 2 without it). Q1.
- Nothing is changed or dropped.

## Proving tests and mutants (task 2.3)

New, in `internal/flowgraph/declarations_test.go`, oracle = literal expected edges, names and subjects:

- `TestBuildFromDeclarationsConnectsDeclaredPorts`: two declarations, a NATS output on `data.raw` and a NATS input on
  `data.*`, give one stream edge `pub.out -> sub.in` on `data.raw`.
- `TestBuildFromDeclarationsRefusesExtraFacts`: a declaration with one input fact and no input port is refused,
  and no graph is returned.
- `TestBuildFromDeclarationsRefusesMalformedDeclarations`: one port and no fact, and a port whose facts carry no
  connection identity, are each refused, and no graph is returned. (Kept apart from the test above because a
  weakened count check panics on this input at `:195` instead of failing an assert.)
- `TestBuildFromDeclarationsRefusesNetworkBindConflicts`: two outputs, and one input plus one output, on the same
  network address are each refused, naming the address.
- `TestValidateStreamRequirementsFlagsCoreNATSPublishers`: a JetStream input fed by two core-NATS publishers gives
  one warning naming both; fed by a JetStream output, none.
- `TestFlowGraphOutputIsTheSameEveryRun`: three malformed mutation ports, 50 builds, one error text naming the first
  component in name order; and a graph of two clusters gives the same `ConnectedComponents`, in name order, on
  every one of 50 builds.

Mutants on the ported file, by pin line; the developer maps lines and runs `task mutate:check` on reached lines only
(a mutant that deletes an import's only use does not build). Predicted detections: M1 `:264` prefix to
`graphmutation.SubjectFamily` (`TestGraphMutationFlowRejectsUntypedLegacyRequester`); M2 R3's mutation loop back
to map order and M3 R3's components loop back to map order (`TestFlowGraphOutputIsTheSameEveryRun`); M4 `:286`
`!= 1` to `> 1` (`TestGraphMutationFlowRejectsInvalidProviderTopology/missing`); M5 `:272` drop `||
!port.Required` (R7's row); M6 `:1082` `==` to `!=` (`TestValidateStreamRequirementsFlagsCoreNATSPublishers`); M7
`:189` condition to `false` (`TestBuildFromDeclarationsRefusesExtraFacts`); M9 `:754` `> 1` to `> 2` and M10
`:770` `exists` to `exists && len(existing) == 0` (a bare `false` leaves `existing` unused and does not build)
(`TestBuildFromDeclarationsRefusesNetworkBindConflicts`); M11 `:397` drop `|| len(patternTokens) !=
len(filterTokens)` (`TestSubjectCoversIsDirectionalCover`); M12 `:1107` drop the publisher merge (the
stream-requirement test). No clean mutant exists for `:197`: any weakening reaches `connections[0]` at `:207`
and panics, so `TestBuildFromDeclarationsRefusesMalformedDeclarations` is its proof and M8 is recorded as not run.
No survivor is predicted. Results, any survivors (reported as survivors) and uncovered cases go in
`notes/2.3.md`.

## Admission-ledger row

```yaml
- source_path: component/flowgraph
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the flow-graph analysis composition.Analyze runs over component declarations (pin composition/analyze.go:42,
    :59, :81, :125; composition/graph.go:93), ported later in change 3 (setup-04a-foundation design D2 row 3);
    carried ahead of its importer in the owner's order (#9 comment 6270875286); internal because no consumer
    imports it and no exported signature in a public package names its types (#9 comment 5953295358)
  destination: internal/flowgraph
  contract: >-
    as at the pin, except: (1) the graph-mutation prefix comes from graphmutation.SubjectFamily; (2) the
    graph-mutation check and the connected-component walk visit components in name order, so equal declarations
    give equal refusals and equal results; (3) removed as dead: GetNodes, EdgeMetadata and FlowEdge.Metadata,
    FlowAnalysisResult.ConnectedEdges, StreamRequirementWarning.Severity and Issue; ComponentNode and PortInfo
    unexported; (4) README.md and doc.go describe the code. BuildFromDeclarations refuses an empty or duplicate
    instance name, a port/fact count mismatch and a port with no connection identity, then derives edges: stream
    and request by wildcard subject match, watch and read by exact bucket, store as fan-in from every provider to
    every reader; it refuses more than one writer to a KV bucket, more than one exclusive bind on a network
    address, and any graph-mutation topology but exactly one required nats-request provider input for
    semengine.graph.mutation v1 on graph.mutation.> (ADR-091 §2, kept by ADR-110). AnalyzeConnectivity reports
    connected components, disconnected nodes and orphaned ports with an issue type and derives no severity;
    ValidateStreamRequirements reports each JetStream input fed by core-NATS publishers, with those publishers;
    SubjectCovers reports whether one subject filter covers another
  dependencies_and_side_effects: >-
    component, internal/graphmutation, pkg/errs (in-set); stdlib; no goroutine, no I/O
  known_risks: >-
    no importer until composition lands; orphaned non-required ports with an interface are called interface
    alternatives by name text; the refusal text names internal function names; uncovered cases in notes/2.3.md
  proving_tests: >-
    internal/flowgraph/declarations_test.go (TestBuildFromDeclarationsConnectsDeclaredPorts,
    TestBuildFromDeclarationsRefusesExtraFacts, TestBuildFromDeclarationsRefusesMalformedDeclarations,
    TestBuildFromDeclarationsRefusesNetworkBindConflicts,
    TestValidateStreamRequirementsFlagsCoreNATSPublishers, TestFlowGraphOutputIsTheSameEveryRun),
    graph_mutation_contract_test.go, subject_cover_test.go, flowgraph_test.go, flowgraph_patterns_test.go,
    flowgraph_validation_test.go, store_federation_test.go; internal/harness/contract
    (TestReservedSubjectsDeclaredOnce)
  owner: port-flowgraph developer
  disposition: adapt
```

## Findings and filing

R3 is fixed here, so nothing is filed for it. Proposed for filing (owner's word, Q1): ADR-063 is cited by
`component` code and governs `storage/storeregistry` and this package, and is not in `docs/adr`.

## Merge order

Either order with #318 (health): the second to merge takes `origin/main` and resolves the ledger append.

## Owner questions

Notice (no answer needed): the package moves to `internal/flowgraph` under your 2026-10-02 rule, because nothing
outside the module uses it and no public function names its types.

Q1. Three SemStreams decision records govern this package and none is in `docs/adr`: ADR-100 (one validator),
ADR-091 §2 (one mutation provider), ADR-063 item 5 (store edges). This port keeps all three unchanged. I recommend
carrying ADR-100 with `composition`, leaving ADR-091 to ADR-110 (which already records keeping §2), and filing an
issue to carry ADR-063, which `component` code already cites. Agree?

Q2. Optional: the retained findings above (duplicate checks, the name-text heuristic, the test that tests only
itself) stay as they are. Take any now? I recommend none.
