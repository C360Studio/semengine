# Proposal: port-flowgraph

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. This change ports the third, `component/flowgraph` (1,324 pin lines
at SemStreams `8b99efe9`): the graph analysis `composition` runs over component declarations. `health` is another
session's draft (#318); `internal/logforwarderpolicy` merged in #319.

## What Changes

- Port the package to `internal/flowgraph` (owner question Q1; `component/flowgraph` if the owner keeps the pin
  path), ahead of its importer `composition`, with one `adapt` ledger row.
- Required corrections (design R1-R6): the graph-mutation subject prefix taken from its one declaration; the tests'
  mock fitted to this tree's `component.Discoverable`; the graph-mutation refusal made the same on every run;
  dead surface removed (`GetNodes`, `EdgeMetadata`, `FlowAnalysisResult.ConnectedEdges`,
  `StreamRequirementWarning.Severity` and `.Issue`); `README.md` and `doc.go` rewritten to what the code does; new
  tests for `BuildFromDeclarations` and `ValidateStreamRequirements`, which the pin's package tests never call.

## Capabilities

- None changed. The contract line is the ledger row; the requirements land with `composition`.

## Impact

- New package `internal/flowgraph` (fewer than the pin's 1,324 production lines after the removals), its ledger
  row and a `docs/repository-map.md` row. No importer until `composition`.
