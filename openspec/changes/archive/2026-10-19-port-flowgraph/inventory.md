# Inventory: port-flowgraph

base: `1ba011ba5f3922c9b0c8c636c7cd19d91d79aea5` (origin/main `4621b4b` plus the claim). Pin paths at SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read from the pin tarball
(`gh api repos/C360Studio/semstreams/tarball/8b99efe9`). "Scratch copy" below means this tree at the base
(`git archive`) with the pin's `component/flowgraph` copied in and the module path rewritten, outside the repository.

Status: INVENTORY PASS (semengine-reviewer, round 1, at `8783daa`); I1 and I2 applied.

## Admission

- `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:216`: `composition` (914) +
  `component/flowgraph` (1,324), disposition carry, "ADR-100 boot validation".
- `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:165`: change 3 lists `component/flowgraph`.
- Same 03B design, D15 (`:635`): "Metric names and the flowgraph response are tested contracts". The response is the
  `<components>/flowgraph` endpoint, which serves `composition.Graph` (pin `service/component_manager_http.go:581`),
  not this package's types; it arrives with `service`.
- Same 03B design, D16 (Q17, `:651-660`): packages are "internal by default and public only where a starter
  consumer imports them"; the owner's public-signature rule (#9 comment 5953295358, 2026-10-02) adds "or if an
  exported signature in a public package names one of its types", and is "meant to stand for later changes".
- Order: #9 comment 6270875286 (third of seven); next after #319 per #9 comment 6273449243. No ledger row exists
  (`git grep -n flowgraph docs/admission-ledger.yaml` gives only `:2274`, the `component` row's "not ported").
- Size at the pin: `flowgraph.go` 1,119, `doc.go` 165, `flowgraph_analysis.go` 40 = 1,324 non-test lines; seven
  test files, 1,981 lines; `README.md` 235 lines. One package, no lifecycle owner, under the ~3,000 bound (#159).

## Imports (pin) and presence here

`flowgraph.go:10-12`: `component`, `internal/graphmutation`, `pkg/errs`; stdlib `fmt`, `maps`, `slices`,
`strings`. Tests add testify (already in `go.mod`). All three are in the tree with `adapt` rows
(`docs/admission-ledger.yaml:437`, `:2073`, `:2245`). On the scratch copy `go build ./component/flowgraph` passes
unchanged; `go vet` fails only in tests, stopping at the first error, `flowgraph_test.go:500:24: undefined:
component.Metadata` (the mock's `Meta()` and `ConfigSchema()`, removed from `Discoverable` by the `component` row's
adaptation 9). The same type is used at `flowgraph_patterns_test.go:375`, which breaks the same way.

## Pin probe

go1.26.4 darwin/arm64. No lifecycle owner, so the lifecycle suite does not apply.

- Pin copy: `go test -race -count=1 -p 1 -cpu 1 ./component/flowgraph` ok; `go test -count=5 -cpu 1 -p 1
  -shuffle=on` ok (seed 1792432337450547000). Coverage 78.3%; `BuildFromDeclarations` (`flowgraph.go:130`) and
  `ValidateStreamRequirements` (`:1033`) are 0.0%: the package's tests never call either production entry point
  (the pin proves them through `composition/validate_test.go`).
- Test-text rules (`time.Sleep`, Skip, build tags, `t.Parallel`): grep empty. No `context`, goroutine, `select`.
- Scratch copy, tests' mock trimmed of `Meta`/`ConfigSchema`: race and five shuffled runs ok, coverage 78.3%.
  `go test ./internal/harness/contract/` fails once:
  `component/flowgraph/flowgraph.go:264: "graph.mutation." spells graph.mutation.; take it from graph.QueryVerbs or
  internal/graphmutation (graph-transport-boundary › Reserved request subjects have one declaration)`.
  `task lint`: production code clean (the three warnings are the trimmed mock's now-unused parameters).
- Behaviour probe (throwaway test on the scratch copy, removed): three components each with a malformed
  graph-mutation port, `ConnectComponentsByPatterns` run 50 times, gave three different error texts (36 naming
  `alpha`, 7 `bravo`, 7 `charlie`).

## Shape sweep (pin `component/flowgraph`)

1. Second spelling.
   - a. The graph-mutation port shape is checked twice: `flowgraph.go:264` (`isMutation := (port.Pattern ==
     component.PatternRequest && strings.HasPrefix(port.ConnectionID, "graph.mutation.")) ||`) across the graph,
     and `processor/graph-ingest/canonical_mutations.go:120` in this tree, per component, which already takes the
     prefix from `strings.TrimSuffix(graphmutation.SubjectFamily, ">")` (the pin spelled it at `:174`).
   - b. Exclusive-resource conflicts: `flowgraph.go:745` (`func (g *FlowGraph) validateNetworkPorts(publishers,
     subscribers map[string][]ComponentPortRef) []string {`) and `component/registry.go:402`
     (`checkResourceConflictsLocked`) at admission.
   - c. Two subject matchers in the package: `SubjectCovers` (`:389`) and `matchNATSPattern` (`:450`). Deliberate:
     `subject_cover_test.go:38-46` pins that they answer different questions. `matchNATSPattern` is bidirectional by
     its own doc (`:449`), yet every caller asks both ways: `:538`, `:653`, `:678`, `:698` (`if
     matchNATSPattern(pubConnID, subConnID) || matchNATSPattern(subConnID, pubConnID) {`).
   - d. Edges exposed twice: `GetEdges` (`:119`, a copy) and `FlowAnalysisResult.ConnectedEdges`
     (`:813`, `ConnectedEdges:      g.edges,`, the internal slice itself).
   - e. A severity outside composition: `:1088` (`Severity:       "critical",`) beside the package's own statement
     that it "derives no severity" (`:804-810`, citing ADR-100 D3) and the pin's `composition/findings.go:69`.
   - f. `buildPublisherMap` (`:297`) and `buildSubscriberMap` (`:341`) are the same 40 lines over outputs and
     inputs; the "more concrete connection ID" choice is written four times (`:540`, `:655`, `:679`, `:699`).
2. Copied shell: none (no `Start`, `Stop`, `Health` or watch loop).
3. Layered interface: none (`grep -n 'interface {'` over the package is empty).
4. Exported surface with no reader. Pin-wide search outside the package, production and tests, per exported name:
   readers are only `composition/analyze.go:42` (`BuildFromDeclarations`), `:59` (`AnalyzeConnectivity`; reads
   `DisconnectedNodes` and `OrphanedPorts`), `:81` (`ValidateStreamRequirements`; reads `SubscriberComp`,
   `SubscriberPort`, `Subjects`, `PublisherComps`), `:125` (`SubjectCovers`), `:169-197` and
   `composition/findings.go:74` (`OrphanedPort`, the five `Issue*` constants), `:231` and `composition/graph.go:93`
   (`GetEdges`; reads `From`, `To`, `Pattern`, `ConnectionID`). No reader anywhere:
   - `GetNodes` (`:82`, `func (g *FlowGraph) GetNodes() map[string]*ComponentNode {`): tests only
     (`flowgraph_test.go:18`, `:29`, `:63`, `:67`); `ComponentNode` and `PortInfo` are exported only through it.
   - `EdgeMetadata` (`:66`) and `FlowEdge.Metadata` (`:47`, "Pattern-specific validation data"): every writer sets
     `EdgeMetadata{}` (`:561`, `:600`, `:641`, `:735`); nothing reads it.
   - `FlowAnalysisResult.ConnectedEdges` (`flowgraph_analysis.go:9`): one test (`flowgraph_test.go:258`).
   - `StreamRequirementWarning.Severity` (`flowgraph_analysis.go:34`) and `.Issue` (`:39`).
   - `FlowAnalysisResult.ConnectedComponents` (`flowgraph_analysis.go:8`): tests only, but the pin spec requires it:
     `openspec/specs/composition-validation/spec.md:281` ("SHALL report connected components, disconnected nodes,
     and orphaned ports and SHALL derive no status of its own").
   - Over-exported, read in the package: `NewFlowGraph` (`:131`) and `ConnectComponentsByPatterns` (`:141`), both
     called only by `BuildFromDeclarations`; tests name them on 42 and 35 lines.
   - JSON tags on every result type: no marshaller. `<components>/flowgraph` serves `composition.Graph`
     (pin `service/component_manager_http.go:581`), not these types.
   - Behaviour described and not implemented: `README.md:101`, `:173-178`, `:201` (`ValidationStatus`, no such
     field); `:94`, `:192` (`componentManager.GetFlowGraph()`, which the pin spec makes absent, `:288`); `:45`
     ("four interaction patterns"; there are eight, `component/port.go:40-47`); `:50` (request matching "Exact
     subject"; code matches wildcards, `:608-611`); `:20` and `:206-221` (severity classification); `:79-83`
     (`service.ComponentManager` builds and caches the graph); `doc.go:7` ("five interaction patterns"), `:11`
     (`//   - Orphaned port detection with severity classification`), `:67-72` and `:153` (critical severity),
     `:164` (`service` uses flowgraph; `composition` does); `component_node_test.go:11` (`BuildFromRegistry`).
5. Classification by error text: none in production. Tests match messages (`graph_mutation_contract_test.go:45`,
   `:65`). Related: `isInterfaceAlternativePort` (`:991`) classifies by port-name and subject text (`:1007`, `if
   strings.Contains(port.Name, "-") {`; `:1018`); it only picks `optional_interface_unused` for a non-required port,
   which composition rates a warning either way (pin `composition/findings.go:73-78`).

Other findings from the read:

- Map-order output. `:262` (`for componentName, node := range g.nodes {`) makes the graph-mutation refusal name
  whichever malformed port the map yields first (probe above). Composition puts that text in the finding message
  (pin `composition/analyze.go:47`, `Message:   err.Error(),`), and the pin spec requires "two runs over equal
  inputs produce byte-equal JSON" (`composition-validation/spec.md:101`). `:868` (`for componentName := range
  g.nodes {`) does the same to `ConnectedComponents`, which nothing reads.
- `TestOrphanedPortSeverity` (`flowgraph_validation_test.go:306`) tests a severity table written in the test
  (`:355`, `getOrphanedPortSeverity`); no production line can fail it.
- The refusal text names internals: `FlowGraph.validateGraphMutationProviders: ... failed: invalid configuration`
  (probe output) reaches an operator through composition's message. Composition's seam.

## Who reads it (#190)

- Pin importers: `composition/graph.go:7`, `composition/findings.go:7`, `composition/analyze.go:9`; nothing else,
  production or test. No pin consumer adapter imports it.
- Named consumers (local checkouts, read without git; heads read from `.git`): semsource `4093d3c`, semconnect
  `dff1265`, semteams `30ce863`, semboids `37dbdb0`, semvocab `c8a07be`, semsparql `51ac535`. `grep -rl
  'component/flowgraph'` over `*.go`: 0 files in each. (semteams and semconnect import `composition`; it is ported
  later and names no flowgraph type in an exported signature: every pin `flowgraph.` use in `composition` is in an
  unexported function or a local.)
- In this tree: no importer until `composition` lands, sixth in the order.

## Pin ADRs (#157)

All 85 records in the pin's `docs/adr` were searched (`flowgraph`, `FlowGraph`, `flow graph`,
`BuildFromDeclarations`, `SubjectCovers`, `AnalyzeConnectivity`, `ValidateStreamRequirements`, `OrphanedPort`,
`orphaned port`): hits in ADR-026, ADR-045, ADR-063. Read by title: ADR-091, ADR-094, ADR-096, ADR-100.

- ADR-100 (compositions are validated; diagrams are projections). D1: connections are derived from port
  declarations, never authored; D3: one validator, one findings vocabulary, the library is the one home. Binding:
  `BuildFromDeclarations` and the `connect*` passes implement D1; `:804-810` implements D3's "no severity here". Not
  in `docs/adr`; cited in this tree by `simplify-04a-09-component` (archive `design.md:12`).
- ADR-091 (graph mutation authority) §2 (`091-graph-mutation-authority-without-semantic-ownership.md:55`): "The
  component flow validates exactly one declared provider and any number of requesters". Binding on
  `validateGraphMutationProviders` (`:259-291`). Not in `docs/adr`; this repository's ADR-110 records keeping §2
  (`docs/adr/110-write-seam.md:69`).
- ADR-063 (store substrate) item 5 (`063-store-substrate-and-resolver.md:385-392`): store edges are advisory
  fan-in, and `findOrphanedPorts` skips `PatternStore`. Binding on `connectStorePorts` (`:577`) and `:927`, `:961`.
  Not in `docs/adr`, though `component/dependencies.go:17`, `:52` and `component/port_provide.go:6` already cite it.
- ADR-026 (`:189`) and ADR-045 (`:409`) name flowgraph in context only; ADR-094 and ADR-096 govern `service`
  (#9 comment 6099868722 carries them with change 3's services); ADR-100 supersedes ADR-096's flow clauses.
- This repository's records: ADR-110 (`:183`, cited by change 3) — flowgraph writes no entity state; ADR-108 and
  ADR-113 govern lifecycle owners — none here.
- Guidance: no pin contract section or skill names the package; the one hit, the pin developer contract `:80`,
  uses FlowGraph as an example of a classification not to re-derive. "State ownership and component wiring" is
  already carried (`.agents/contracts/semengine-developer.md:299`).

## Open issues (#160)

One `gh issue list --state open --search` per word: `flowgraph`, `FlowGraph`, `"component/flowgraph"` give #9
(the epic; stays, `Addresses #9`). `"flow graph"`, `SubjectCovers`, `BuildFromDeclarations`, `AnalyzeConnectivity`,
`ValidateStreamRequirements`, `OrphanedPort`, `orphaned`, `matchNATSPattern`, `validateGraphMutationProviders`,
`flowgraph.go`, `SubjectFamily`, `"mutation provider"`, `ADR-063`, `ADR-100` give nothing else naming the package.
`composition` gives #16, #24, #96, #7: each names the word, not the package (#16 has no `flowgraph` hit in body or
comments).

## Overlapping PRs

`gh pr list --state open`: #318 (health, draft, 15 files) shares only `docs/admission-ledger.yaml` (an append at
the end, as here). No capability overlap: its delta is `specs/health-status/`; this change adds none.

## Problem shape

Static analysis over declarations: admit or refuse a topology, then report facts without judging them. The closest
instance here is `processor/graph-ingest/canonical_mutations.go:104-140` (one component's mutation port, refused
before start); the port adopts its prefix spelling. No new primitive, so no adoption sweep; no durable,
communication or coordination primitive, so no collision table. No capability changes membership, so no intent
table.

## Adopter seam

No consumer imports the package. Its outside reach is through `composition`: an operator reads flowgraph's refusal
text inside a `connection_pattern_error` finding, and a product calls composition's `Validate`, never flowgraph.
Composition's PR inventories that seam (the internal names in the refusal text, above).
