# Tasks: port-flowgraph

Each task names its outcome and the gate that proves it. Tests come first. Each stop's task ends only when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner, with Q1-Q3 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Port

- [ ] 2.1 Tests first: the pin's seven test files in the destination with the mock fitted (R2), the `GetNodes`
  test and reads and the `ConnectedEdges` assert changed as the design says (R4), and the new
  `declarations_test.go` (R6, R3's test); then the pin's three `.go` files unchanged, the run recorded:
  `TestGraphMutationRefusalIsTheSameEveryRun` and `TestReservedSubjectsDeclaredOnce` red. Then R1 and R3, both
  green; the ledger row and a `docs/repository-map.md` row. Gate: `go test -race ./internal/flowgraph
  ./internal/harness/contract`, `task lint` and `task ledger:check`.
- [ ] 2.2 R4's removals and R5's `README.md` and `doc.go`. Gate: `task docs:check` and
  `go test -race ./internal/flowgraph`.
- [ ] 2.3 `task mutate:check` for M1-M7 (detections) and S1 (survivor recorded) in `notes/2.3.md`, with the
  uncovered cases; `task verify` passes on a clean tree.
- [ ] 2.4 Cross-agent review stop (Codex); `reviewed-by:` in the PR body.

## 3. Close

- [ ] 3.1 Archive (`openspec archive port-flowgraph`) as the last content commit. Gate: `task spec:check` and
  `task docs:check`.
