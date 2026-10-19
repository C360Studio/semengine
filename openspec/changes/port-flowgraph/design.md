# Design: port-flowgraph

Pin `8b99efe9`. Ledger row: `component/flowgraph` (below). Inventory: `inventory.md` in this change.

Status: drafted before `INVENTORY PASS` at the orchestrator's request; void if the inventory review returns a
`BLOCKING` finding.

## Options

- A. Port to `internal/flowgraph` with the required corrections below (recommended). Change 2's destination rule
  moved `pkg/dispatch` to `internal/dispatch` because "no consumer import; no public signature names it"
  (archive `2026-10-12-setup-04a-02-ingest-kernel/design.md:111`); both hold here (inventory, "Who reads it").
  The pin's only reader, `composition`, is in the same module. ADR-100 D3 makes `composition` the one validator and
  D4 gives products `catalog`, `validate` and `graph` through it, so a public graph analyser invites the second
  interpreter ADR-100 removed. The move also settles the over-exported constructors (Surface audit: "moved under
  `internal/`"). Cost: the destination differs from the admission row's path, so the owner rules (Q1); `ledger:check`
  maps the move for `composition`'s files when they land (`internal/harness/pindiff/rewrite.go:16-20`).
- B. The same work at `component/flowgraph`. Cost: a public package nothing outside the module imports;
  `NewFlowGraph` and `ConnectComponentsByPatterns` stay public as a second way to build a graph beside
  `BuildFromDeclarations`, which the pin spec calls "the one construction seam" (`composition-validation/spec.md:283`).
- C. Required build and gate fixes only (R1, R2), surface kept, docs as at the pin, no new tests. Cost: ports dead
  and advertised-absent surface knowingly (Surface audit (a) and (c)); leaves composition's two entry points untested
  until composition lands.
- D. Port it inside composition's PR. Cost: two packages in one PR, against the one-package rule (#159).

## Decision (A), pending Q1

Required corrections, each with its basis:

- R1. `flowgraph.go:264` spells `"graph.mutation."`; take the prefix from `graphmutation.SubjectFamily` as
  `processor/graph-ingest/canonical_mutations.go:120` does. Basis: `TestReservedSubjectsDeclaredOnce` fails on the
  pin file (inventory, Pin probe; graph-transport-boundary spec). Behaviour unchanged.
- R2. The tests' mock implements `Meta()` and `ConfigSchema()` with `component.Metadata`, which this tree's
  `component` deleted (ledger `component` adaptation 9). Remove them and the field; `task lint` must pass. Basis:
  the package's tests must build.
- R3. The graph-mutation check visits components in name order (`g.sortedNodeNames()`, as the rest of the package
  does), so the same declarations give the same refusal text on every run. Basis: a demonstrated defect (inventory,
  behaviour probe: three texts in 50 runs) against the carried contract composition binds, byte-equal results for
  equal inputs (pin `composition-validation/spec.md:101`); the real-world trigger is a configuration with two
  malformed graph-mutation ports.
- R4. Dead surface removed (Surface audit (a); `.agents/protocol.md` Bounded port scope: "truly dead surface is not
  protected"): `GetNodes` with its deep copy and `TestGetNodesReturnsIndependentInterfaceContracts`;
  `EdgeMetadata` and `FlowEdge.Metadata`; `FlowAnalysisResult.ConnectedEdges` (its one assert reads `GetEdges`);
  `StreamRequirementWarning.Severity` and `.Issue`. With `Metadata` gone, `GetEdges` copies hold no shared pointer.
  `TestFlowGraphConstruction`'s `GetNodes` reads become reads of `AnalyzeConnectivity` (a lone node is reported
  disconnected). Kept: `ConnectedComponents`, which the pin spec requires (`composition-validation/spec.md:281`);
  its contract line is the ledger row until composition brings that spec.
- R5. `README.md` and `doc.go` rewritten to what the code does (Surface audit (c): each item in the inventory's list),
  for a working developer, naming `composition` as the reader. Statements about NATS stay the package's own rule
  ("this package's matcher treats `*` as one token and `>` as one or more"), not a claim about the server, so no
  probe is owed (#170). The "subscriber hangs" sentence is dropped, not restated.
- R6. Proof for the two production entry points the package's tests never call (both 0.0% at the pin). The stream
  requirement check guards silent loss, an admission gate (architect contract, Extraction slices), so its proof is
  required qualification, not optional coverage. One new test file, below.

Retained, optional under Bounded port scope (inherited; no binding requirement, gate or ruling names them):
shape findings 1a, 1b, 1c, 1f, the interface-alternative heuristic, the JSON tags, `ConnectedComponents` order,
`NewFlowGraph` and `ConnectComponentsByPatterns` staying exported inside `internal/`, `TestOrphanedPortSeverity`
(not listed as a proving test), and the refusal text naming internals (composition's PR). Costs are recorded here
and in the ledger's `known_risks`.

Not changed: no context, goroutine, I/O or lifecycle; no `GenericJSON`; no spec capability (the contract line is the
ledger row, as for `internal/logforwarderpolicy`; the pin's `composition-validation` and the mutation-topology
requirement of `framework-composition` land with `composition`). Examples suffice over generated checks: the
grammar under test is two small token matchers already pinned case by case (`subject_cover_test.go`).

## ADRs

- ADR-100 D1 and D3: kept; R4's removal of `Severity` matches D3. Not carried here: its main subject is
  `composition` (D3, D4) and it supersedes clauses of ADR-094 and ADR-096, which come with `service`; carry it with
  `composition` (Q2).
- ADR-091 §2: kept unchanged (exactly one provider, any number of requesters); ADR-110 `:69` already records it.
- ADR-063 item 5: kept unchanged (store fan-in edges, store ports never orphans). Not carried: its subject is the
  store resolver (`storage/storeregistry`, ported in change 2 without it). Q2.
- Nothing is changed or dropped.

## Proving tests and mutants (task 2.3)

New, in `internal/flowgraph/declarations_test.go`, oracle = literal expected edges, names and subjects:

- `TestBuildFromDeclarationsConnectsDeclaredPorts`: two declarations, a NATS output on `data.raw` and a NATS input on
  `data.*`, give one stream edge `pub.out -> sub.in` on `data.raw`.
- `TestBuildFromDeclarationsRefusesPortFactMismatch`: a declaration whose port and fact counts differ (one fact and
  no port; one port and no fact) is refused and no graph is returned.
- `TestValidateStreamRequirementsFlagsCoreNATSPublishers`: a JetStream input fed by two core-NATS publishers gives
  one warning naming both; fed by a JetStream output, none.
- `TestGraphMutationRefusalIsTheSameEveryRun`: three malformed mutation ports, 50 builds, one error text, naming the
  first component in name order.

Mutants on the ported file, by pin line; the developer maps lines and runs `task mutate:check` on reached lines only
(a mutant that deletes an import's only use does not build). Predicted detections: M1 `:264` prefix to
`graphmutation.SubjectFamily` (`TestGraphMutationFlowRejectsUntypedLegacyRequester`); M2 R3's loop back to map
order (`TestGraphMutationRefusalIsTheSameEveryRun`); M3 `:286` `!= 1` to `> 1`
(`TestGraphMutationFlowRejectsInvalidProviderTopology/missing`); M4 `:1082` `==` to `!=`
(`TestValidateStreamRequirementsFlagsCoreNATSPublishers`); M5 `:189` condition to `false`
(`TestBuildFromDeclarationsRefusesPortFactMismatch`); M6 `:397` drop `|| len(patternTokens) != len(filterTokens)`
(`TestSubjectCoversIsDirectionalCover`); M7 `:1107` drop the publisher merge (the stream-requirement test).
Predicted survivor: S1 `:272` drop `|| !port.Required` (every fixture's port is required). Results and uncovered
cases go in `notes/2.3.md`, survivors reported as survivors.

## Admission-ledger row

```yaml
- source_path: component/flowgraph
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the flow-graph analysis composition.Analyze runs over component declarations (pin composition/analyze.go:42,
    :59, :81, :125; composition/graph.go:93), ported later in change 3 (setup-04a-foundation design D2 row 3);
    carried ahead of its importer in the owner's order (#9 comment 6270875286); moved under internal/ because no
    consumer imports it and no public signature names it (the setup-04a-02 design D1 rule, as for pkg/dispatch)
  destination: internal/flowgraph
  contract: >-
    as at the pin, except: (1) the graph-mutation prefix comes from graphmutation.SubjectFamily; (2) the
    graph-mutation check visits components in name order, so equal declarations give equal refusals; (3) removed
    as dead: GetNodes, EdgeMetadata and FlowEdge.Metadata, FlowAnalysisResult.ConnectedEdges,
    StreamRequirementWarning.Severity and Issue; (4) README.md and doc.go describe the code. BuildFromDeclarations
    refuses an empty or duplicate instance name, a port/fact count mismatch and a port with no connection identity,
    then derives edges: stream and request by wildcard subject match, watch and read by exact bucket, store as
    fan-in from every provider to every reader; it refuses more than one writer to a KV bucket, more than one
    exclusive bind on a network address, and any graph-mutation topology but exactly one required nats-request
    provider input for semengine.graph.mutation v1 on graph.mutation.> (ADR-091 §2, kept by ADR-110).
    AnalyzeConnectivity reports connected components, disconnected nodes and orphaned ports with an issue type and
    derives no severity; ValidateStreamRequirements reports each JetStream input fed by core-NATS publishers, with
    those publishers; SubjectCovers reports whether one subject filter covers another
  dependencies_and_side_effects: >-
    component, internal/graphmutation, pkg/errs (in-set); stdlib; no goroutine, no I/O
  known_risks: >-
    no importer until composition lands; ConnectedComponents order follows map iteration (nothing reads it);
    orphaned non-required ports with an interface are called interface alternatives by name text; the refusal
    text names internal function names; uncovered cases in notes/2.3.md
  proving_tests: >-
    internal/flowgraph/declarations_test.go (TestBuildFromDeclarationsConnectsDeclaredPorts,
    TestBuildFromDeclarationsRefusesPortFactMismatch, TestValidateStreamRequirementsFlagsCoreNATSPublishers,
    TestGraphMutationRefusalIsTheSameEveryRun), graph_mutation_contract_test.go, subject_cover_test.go,
    flowgraph_test.go, flowgraph_patterns_test.go, flowgraph_validation_test.go, store_federation_test.go;
    internal/harness/contract (TestReservedSubjectsDeclaredOnce)
  owner: port-flowgraph developer
  disposition: adapt
```

## Findings and filing

R3 is fixed here, so nothing is filed for it. Proposed for filing (owner's word, Q2): ADR-063 is cited by
`component` code and governs `storage/storeregistry` and this package, and is not in `docs/adr`.

## Merge order

Either order with #318 (health): the second to merge takes `origin/main` and resolves the ledger append.

## Owner questions

Q1. Where should the package live? SemStreams has it at `component/flowgraph`, public, but nothing outside
SemStreams imports it and its one reader, `composition`, sits in the same module. I recommend `internal/flowgraph`,
as `pkg/dispatch` became `internal/dispatch`: products validate through `composition`, never through this. If you
want the pin path, take option B; the work is the same.

Q2. Three SemStreams decision records govern this package and none is in `docs/adr`: ADR-100 (one validator),
ADR-091 §2 (one mutation provider), ADR-063 item 5 (store edges). This port keeps all three unchanged. I recommend
carrying ADR-100 with `composition`, leaving ADR-091 to ADR-110 (which already records keeping §2), and filing an
issue to carry ADR-063, which `component` code already cites. Agree?

Q3. Optional: the retained findings above (duplicate checks, the name-text heuristic, the test that tests only
itself) stay as they are. Take any now? I recommend none.
