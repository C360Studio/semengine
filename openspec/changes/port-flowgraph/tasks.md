# Tasks: port-flowgraph

Each task names its outcome and the gate that proves it. Tests come first. Each stop's task ends only when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner, with Q1 and Q2 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Port

- [ ] 2.1 Tests first: the pin's seven test files in `internal/flowgraph` with the mock fitted (R2), the `GetNodes`
  test and reads and the `ConnectedEdges` assert changed as the design says (R4), R7's provider row, and the new
  `declarations_test.go` (R6, R3's test); then the pin's three `.go` files unchanged, the run recorded:
  `TestFlowGraphOutputIsTheSameEveryRun` and `TestReservedSubjectsDeclaredOnce` red. Then R1 and R3, both green.
  Gate: `go test -race ./internal/flowgraph ./internal/harness/contract` and `task lint`.
- [ ] 2.2 R4's removals and renames, R5's corrections (`README.md`, `doc.go`, the `component_node_test.go`
  comment); then the ledger row and a `docs/repository-map.md` row, now true of the code. Gate:
  `task ledger:check`, `task docs:check`, `go test -race ./internal/flowgraph`.
- [ ] 2.3 `task mutate:check` for M1-M7 and M9-M12 (detections; M8 recorded as not run, with its reason) in
  `notes/2.3.md`, with any survivors and the uncovered cases; `task verify` passes on a clean tree.
- [ ] 2.4 Cross-agent review stop (Codex); `reviewed-by:` in the PR body.

## 3. Close

- [ ] 3.1 Archive (`openspec archive port-flowgraph`) as the last content commit. Gate: `task spec:check` and
  `task docs:check`.
