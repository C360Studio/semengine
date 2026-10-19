# Proposal: port-flowgraph

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. This change ports the third, `component/flowgraph` (1,324 pin lines
at SemStreams `8b99efe9`): the graph analysis `composition` runs over component declarations. `health` is another
session's draft (#318); `internal/logforwarderpolicy` merged in #319.

## What Changes

- Port the package to `internal/flowgraph`, as the owner's public-signature rule places it (#9 comment
  5953295358: no consumer imports it, no public signature names its types), ahead of its importer `composition`,
  with one `adapt` ledger row.
- Required corrections (design R1-R7): the graph-mutation subject prefix taken from its one declaration; the
  tests' mock fitted to this tree's `component.Discoverable`; the graph-mutation refusal and the connected
  components made the same on every run; dead surface removed (`GetNodes`, `EdgeMetadata`,
  `FlowAnalysisResult.ConnectedEdges`, `StreamRequirementWarning.Severity` and `.Issue`; `ComponentNode` and
  `PortInfo` unexported); each wrong statement in `README.md`, `doc.go` and one test comment corrected; new tests
  for `BuildFromDeclarations`, `ValidateStreamRequirements`, three unreached refusals and the non-required
  mutation provider.
- The simple fixes the owner asked for on 2026-10-19 (design R8-R14): the subject matcher asked once per pair; one
  port-map builder and one concrete-subject choice; the port-name heuristic for "interface alternatives" removed;
  JSON tags nothing reads removed; `NewFlowGraph` and `ConnectComponentsByPatterns` unexported; the test that only
  tested its own helper removed; refusals in plain words, without internal function names. Two duplicates whose
  fix reaches other packages wait on the owner (design Q3, Q4).

## Capabilities

- None changed. The contract line is the ledger row; the requirements land with `composition`.

## Impact

- New package `internal/flowgraph`: about 1,150 production lines, from the pin's 1,324 (an estimate; `doc.go`'s
  length depends on R5), its ledger row and a `docs/repository-map.md` row. No importer until `composition`, whose
  port drops its one `IssueOptionalInterfaceUnused` case.
