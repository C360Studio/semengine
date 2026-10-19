# Tasks: port-flowgraph

Each task names its outcome and the gate that proves it. Tests come first. Each stop's task ends only when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner (accepted 2026-10-19; Q1 and Q2 answered; Q3 and Q4 answered 2026-10-19 as #323 and #324). Gate: `task spec:check`
  and `task docs:check`.

## 2. Port

- [x] 2.1 Tests first: the pin's seven test files in `internal/flowgraph` with the mock fitted (R2), the `GetNodes`
  test and reads and the `ConnectedEdges` assert changed as the design says (R4), R7's provider row, and the new
  `declarations_test.go` (R6, R3's test); then the pin's three `.go` files unchanged, the run recorded:
  `TestFlowGraphOutputIsTheSameEveryRun` and `TestReservedSubjectsDeclaredOnce` red. Then R1 and R3, both green.
  Gate: `go test -race ./internal/flowgraph ./internal/harness/contract` and `task lint`.
- [x] 2.2 Dead surface and docs: R4's removals and renames, R5's corrections (`README.md`, `doc.go`, the
  `component_node_test.go` comment). Gate: `task docs:check`, `go test -race ./internal/flowgraph` and `task lint`.
- [x] 2.3 Same behaviour, less code: R8 (`TestMatchNATSPatternIsSymmetric` first, green on the pin's matcher, then
  each caller asks once), R9 (the `events.*` case of `TestBuildFromDeclarationsConnectsDeclaredPorts` first, then
  one port-map builder and one `concreteID`), R11 (tags removed) and R12 (constructors unexported). The pin's edge
  tests pass unchanged except for R12's renames. Gate: `go test -race ./internal/flowgraph` and `task lint`.
- [x] 2.4 Owner-ruled behaviour changes: R10 (`TestUnconnectedOptionalInterfacePortReportsNoPublishers` red, then
  the heuristic and its constant removed), R13 (the severity test and its helper removed) and R14
  (`TestRefusalsNameNoInternalFunction` red, then plain-text refusals); then the ledger row and a
  `docs/repository-map.md` row, now true of the code. Gate: `task ledger:check`, `task docs:check` and
  `go test -race ./internal/flowgraph ./internal/harness/contract`.
- [x] 2.5 `task mutate:check` for M1-M7 and M9-M16 (detections; M8 recorded as not run, with its reason) in
  `notes/2.5.md`, with any survivors and the uncovered cases; `task verify` passes on a clean tree.
- [x] 2.6 Cross-agent review stop (Codex); `reviewed-by:` in the PR body. Round 1 CHANGES REQUESTED (F1, #326; F2),
  fixed at 56bb269; round 2 APPROVE at 56bb269 (PR #320 comment 6277239726).

## 3. Close

- [x] 3.1 Archive (`openspec archive port-flowgraph`) as the last content commit. Gate: `task spec:check` and
  `task docs:check`.
