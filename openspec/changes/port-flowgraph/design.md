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

Simple fixes the owner made required on 2026-10-19 ("let's fix them so we do not create debt on simple fixes as
we port"). Each keeps every behaviour `composition` reads at the pin (inventory, shape finding 4) unless it says
otherwise:

- R8 (finding 1c). `matchNATSPattern` already answers both ways (`:450-483`: it swaps the arguments itself when only
  one side has a wildcard and tries both orders when both do), so the four callers ask once:
  `matchNATSPattern(pubConnID, subConnID)` at `:538` and `:653`, and the same at `:678` and `:698`. `SubjectCovers`
  stays beside it: it answers a different, directional question, pinned by `subject_cover_test.go:38-46`.
  Proof: `TestMatchNATSPatternIsSymmetric` checks `matchNATSPattern(a, b) == matchNATSPattern(b, a)` for every
  ordered pair of a fixed set of subjects covering each case the function branches on (equal, literal only, `*`
  and `>` on either or both sides, different lengths); the existing edge tests show no edge changes.
- R9 (finding 1f). One port-map builder: `buildPublisherMap` and `buildSubscriberMap` become one
  `portsByConnection(direction component.Direction)` that picks `node.InputPorts` or `node.OutputPorts` the way
  `exclusivePortRefs` already does (`:790-793`). The four copies of "use the side without a wildcard" become one
  `concreteID(a, b string) string`. Proof: the existing edge tests, which assert each edge's connection ID
  (`flowgraph_patterns_test.go:365`, "Should use concrete subject, not pattern"; `flowgraph_validation_test.go:670`).
- R10 (interface-alternative heuristic). `isInterfaceAlternativePort` (`:991-1025`) and `IssueOptionalInterfaceUnused`
  go: an unconnected input that is not required reports `no_publishers` like any other. Its only effect at the pin is
  composition's suggestion text (pin `composition/analyze.go:195-196`); severity is a warning either way (pin
  `composition/findings.go:73-78`), and no pin spec names the issue. Composition's PR drops that one `case`.
  Proof: `TestFlowGraphInterfaceAlternatives` (`flowgraph_validation_test.go:372-491`) becomes
  `TestUnconnectedOptionalInterfacePortReportsNoPublishers`: the pin's `write-graphable` and `.graphable` ports
  report `no_publishers` with `Required` false.
- R11 (JSON tags). The `json:"..."` tags on `FlowEdge`, `ComponentPortRef`, `FlowAnalysisResult`,
  `DisconnectedNode`, `OrphanedPort` and `StreamRequirementWarning` go: nothing marshals these types (the endpoint
  serves `composition.Graph`). Nothing observable changes, so there is no test; the reviewer's check is that `git
  grep -n 'json:"' -- internal/flowgraph` is empty.
- R12 (constructors). `NewFlowGraph` and `ConnectComponentsByPatterns` become `newFlowGraph` and
  `connectComponentsByPatterns`: their only callers are `BuildFromDeclarations` and the package's tests, and
  `BuildFromDeclarations` stays "the one construction seam" (pin `composition-validation/spec.md:283`). In-package
  renames only.
- R13 (`TestOrphanedPortSeverity`). The test, its helper `getOrphanedPortSeverity`
  (`flowgraph_validation_test.go:305-370`) and the one assert that calls the helper (`:436`) go: no production line
  can fail them, and severity is composition's, with its own tests at the pin.
- R14 (refusal text). The five refusals stop naming internal functions: `errs.WrapInvalid(errs.ErrInvalidConfig,
  "FlowGraph", "<method>", msg)` at `:161`, `:164`, `:250`, `:273`, `:287` becomes `fmt.Errorf("<msg>: %w",
  errs.ErrInvalidConfig)`, the plain-text shape `processor/graph-ingest/canonical_mutations.go:104-140` uses for the
  same port check. The text an operator reads in composition's finding becomes, for example, "graph mutation flow
  requires exactly one compatible provider, found 2: invalid configuration". Nothing at the pin reads the error's
  class: `composition/analyze.go:43` checks only `err != nil`. Proof: `TestRefusalsNameNoInternalFunction` drives
  each of the five refusals and checks that `errors.Is(err, errs.ErrInvalidConfig)` holds and that the text holds
  none of `FlowGraph.`, `AddComponentNode`, `ConnectComponentsByPatterns`, `validateGraphMutationProviders`.

Not designed, because the fix crosses the one-package boundary (owner questions Q3 and Q4 below):

- Finding 1a, the graph-mutation port shape checked in two places. `flowgraph`'s check stays in any case: it alone
  sees the whole graph, so it alone can count providers (ADR-091 §2). One home for "is this a valid provider port"
  would be a function in `internal/graphmutation` that both `flowgraph` and `processor/graph-ingest` call, which
  edits two other packages.
- Finding 1b, exclusive network addresses checked in two places (`flowgraph.go:745-802` and
  `component/registry.go:571-586`). The registry's check is the one to keep: it runs at admission, covers every
  exclusive port, and its output (`Declaration.ExclusiveResources`) is what composition's offline check reads (pin
  `composition/validate.go:139`). Removing `flowgraph`'s copy is a simple deletion. But no test in this tree shows
  the registry refusing a second component on a taken address (`grep -n 'already used by' component/*_test.go` is
  empty), so the remaining copy needs a test in `component`. And one behaviour changes: a single component that
  lists the same address on an input and an output is refused by `flowgraph` (`:770`) and accepted by the registry,
  which counts it once (`exclusiveResources`, `component/registry.go:611-619`).

Not changed: no context, goroutine, I/O or lifecycle; no `GenericJSON`; no spec capability (the contract line is the
ledger row, as for `internal/logforwarderpolicy`; the pin's `composition-validation` and the mutation-topology
requirement of `framework-composition` land with `composition`). Examples suffice over generated checks: the
matchers themselves do not change (R8 changes only their callers), and R8's one law, symmetry, is checked over every
ordered pair of a set chosen to reach each branch, which a generator would add nothing to.

## ADRs

- ADR-100 D1 and D3: kept; R4's removal of `Severity` matches D3. Not carried here: its main subject is
  `composition` (D3, D4) and it supersedes clauses of ADR-094 and ADR-096, which come with `service`; carry it with
  `composition` (Q1, answered).
- ADR-091 §2: kept unchanged (exactly one provider, any number of requesters); ADR-110 `:69` already records it.
- ADR-063 item 5: kept unchanged (store fan-in edges, store ports never orphans). Not carried: its subject is the
  store resolver (`storage/storeregistry`, ported in change 2 without it); #321 carries it (Q1).
- Nothing is changed or dropped.

## Proving tests and mutants (task 2.5)

New, in `internal/flowgraph/declarations_test.go`, oracle = literal expected edges, names and subjects:

- `TestBuildFromDeclarationsConnectsDeclaredPorts`: two declarations, a NATS output on `data.raw` and a NATS input on
  `data.*`, give one stream edge `pub.out -> sub.in` on `data.raw`; and a NATS output on `events.*` with a NATS input
  on `events.raw` give one edge on `events.raw`. No pin test has the wildcard on the publisher's side, so this case
  is the one that reaches R9's `concreteID` choice.
- `TestBuildFromDeclarationsRefusesExtraFacts`: a declaration with one input fact and no input port is refused,
  and no graph is returned.
- `TestBuildFromDeclarationsRefusesMalformedDeclarations`: one port and no fact, and a port whose facts carry no
  connection identity, are each refused, and no graph is returned. (Kept apart from the test above because a
  weakened count check panics on this input at `:195` instead of failing an assert.)
- `TestBuildFromDeclarationsRefusesNetworkBindConflicts`: two outputs, and one input plus one output, on the same
  network address are each refused, naming the address.
- `TestValidateStreamRequirementsFlagsCoreNATSPublishers`: a JetStream input fed by two core-NATS publishers gives
  one warning naming both; fed by a JetStream output, none.
- `TestMatchNATSPatternIsSymmetric` (R8) and `TestRefusalsNameNoInternalFunction` (R14), as R8 and R14 say;
  `TestUnconnectedOptionalInterfacePortReportsNoPublishers` (R10) replaces `TestFlowGraphInterfaceAlternatives`.
- `TestFlowGraphOutputIsTheSameEveryRun`: three malformed mutation ports, 50 builds, one error text naming the first
  component in name order; and a graph of two clusters gives equal `ConnectedComponents` on every one of 50 builds
  (each walk starts at the first unvisited name; members follow walk order).

Mutants on the ported file, by pin line; the developer maps lines and runs `task mutate:check` on reached lines only (a
mutant that deletes an import's only use does not build). Predicted detections: M1 `:264` prefix to
`graphmutation.SubjectFamily` (`TestGraphMutationFlowRejectsUntypedLegacyRequester`); M2 R3's mutation loop back to map
order and M3 R3's components loop back to map order (`TestFlowGraphOutputIsTheSameEveryRun`); M4 `:286` `!= 1` to `> 1`
(`TestGraphMutationFlowRejectsInvalidProviderTopology/missing`); M5 `:272` drop `|| !port.Required` (R7's row); M6
`:1082` `==` to `!=` (`TestValidateStreamRequirementsFlagsCoreNATSPublishers`); M7 `:189` condition to `false`
(`TestBuildFromDeclarationsRefusesExtraFacts`); M9 `:754` `> 1` to `> 2` and M10 `:770` `exists` to `exists &&
len(existing) == 0` (a bare `false` leaves `existing` unused and does not build)
(`TestBuildFromDeclarationsRefusesNetworkBindConflicts`); M11 `:397` drop `|| len(patternTokens) != len(filterTokens)`
(`TestSubjectCoversIsDirectionalCover`); M12 `:1107` drop the publisher merge (the stream-requirement test); M13
`concreteID` returns its first argument (the `events.*` case of `TestBuildFromDeclarationsConnectsDeclaredPorts`); M14
one of R14's five refusals back to `errs.WrapInvalid(..., "FlowGraph", ...)` (`TestRefusalsNameNoInternalFunction`); M15
`:482` (`return matchTokens(patternTokens, subjectTokens)`) to `return matchTokens(subjectTokens, patternTokens)`
(`TestMatchNATSPatternIsSymmetric`); M16 `portsByConnection` always reads `node.InputPorts`
(`TestStreamPatternConnections`). No clean mutant exists for `:197`: any weakening reaches `connections[0]` at `:207`
and panics, so `TestBuildFromDeclarationsRefusesMalformedDeclarations` is its proof and M8 is recorded as not run. No
survivor is predicted. Results, any survivors (reported as survivors) and uncovered cases go in `notes/2.5.md`.

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
    FlowAnalysisResult.ConnectedEdges, StreamRequirementWarning.Severity and Issue; ComponentNode, PortInfo,
    NewFlowGraph and ConnectComponentsByPatterns unexported; no JSON tags; (4) README.md and doc.go describe the
    code; (5) one port-map builder, one concrete-subject choice, and matchNATSPattern asked once per pair, which
    it answers both ways; (6) the interface-alternative heuristic and IssueOptionalInterfaceUnused removed, so an
    unconnected input that is not required reports no_publishers; (7) refusals are plain text wrapping
    errs.ErrInvalidConfig and name no internal function. BuildFromDeclarations refuses an empty or duplicate
    instance name, a port/fact count mismatch and a port with no connection identity, then derives edges: stream
    and request by wildcard subject match, on the side without a wildcard, watch and read by exact bucket, store
    as fan-in from every provider to every reader; it refuses more than one writer to a KV bucket, more than one
    exclusive bind on a network address, and any graph-mutation topology but exactly one required nats-request
    provider input for semengine.graph.mutation v1 on graph.mutation.> (ADR-091 §2, kept by ADR-110).
    AnalyzeConnectivity reports connected components, disconnected nodes and orphaned ports with an issue type and
    derives no severity; ValidateStreamRequirements reports each JetStream input fed by core-NATS publishers, with
    those publishers; SubjectCovers reports whether one subject filter covers another
  dependencies_and_side_effects: >-
    component, internal/graphmutation, pkg/errs (in-set); stdlib; no goroutine, no I/O
  known_risks: >-
    no importer until composition lands, whose port drops its IssueOptionalInterfaceUnused case; the
    graph-mutation port shape is also checked in processor/graph-ingest and exclusive network addresses also in
    component.Registry (design Q3, Q4); refusals are no longer errs-classified invalid (nothing reads the class);
    uncovered cases in notes/2.5.md
  proving_tests: >-
    internal/flowgraph/declarations_test.go (TestBuildFromDeclarationsConnectsDeclaredPorts,
    TestBuildFromDeclarationsRefusesExtraFacts, TestBuildFromDeclarationsRefusesMalformedDeclarations,
    TestBuildFromDeclarationsRefusesNetworkBindConflicts, TestValidateStreamRequirementsFlagsCoreNATSPublishers,
    TestFlowGraphOutputIsTheSameEveryRun, TestMatchNATSPatternIsSymmetric, TestRefusalsNameNoInternalFunction),
    flowgraph_validation_test.go (TestUnconnectedOptionalInterfacePortReportsNoPublishers and the pin's tests),
    graph_mutation_contract_test.go, subject_cover_test.go, flowgraph_test.go, flowgraph_patterns_test.go,
    store_federation_test.go; internal/harness/contract (TestReservedSubjectsDeclaredOnce)
  owner: port-flowgraph developer
  disposition: adapt
```

## Findings and filing

R3 is fixed here, so nothing is filed for it. ADR-063 not being in `docs/adr` is #321, filed on the owner's word of
2026-10-19. Findings 1a and 1b wait on Q3 and Q4.

## Merge order

The health port (#318) merged as `0ec8f5a`; this PR merges `origin/main` before its ledger row lands and appends
after it.

## Owner decisions

Accepted by the owner on 2026-10-19: the design ("Accept").

- Notice: the package moves to `internal/flowgraph` under the 2026-10-02 rule (#9 comment 5953295358).
- Q1 (decision records), answered 2026-10-19: "Yes, and file the ADR-063 issue". ADR-100 comes with
  `composition`, ADR-091 stays covered by ADR-110, and ADR-063 is #321.
- Q2 (optional findings), answered 2026-10-19: "let's fix them so we do not create debt on simple fixes as we
  port". R8 to R14 are those fixes; Q3 and Q4 are the two that are not simple inside this package.

Open, in plain words:

- Q3. The check that a graph-mutation port has the right shape is written twice: once here, across the whole
  graph, and once in graph-ingest, for its own port. Making it one function means editing `internal/graphmutation`
  and graph-ingest as well as this package. Do it in this PR, or file an issue and do it in its own small PR? I
  recommend the issue: it keeps this PR to one package.
- Q4. The check that two components do not claim the same network address is written twice: here and in the
  component registry, which runs first, when components are admitted. I would keep the registry's and delete this
  one, but that needs a new test in `component` (none shows the registry refusing today), and it would start
  accepting one component that lists the same address as both an input and an output, which this package refuses
  today. Do it here, as its own issue, or keep both? I recommend its own issue, with the test added in `component`
  first.
