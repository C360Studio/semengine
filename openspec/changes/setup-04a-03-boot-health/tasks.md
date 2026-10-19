# Tasks: setup-04a-03-boot-health

Each task names its outcome and the gate that proves it. Tests come first; record in `notes/<task>.md` how each
fails on the base commit, or which mutant it detects. Review stops (Codex) after 2.2 and 3.1. Each stop's task ends
only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are
  accepted by the owner, with Q3 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Port (D1, D2)

- [x] 2.1 Tests first, in `health/`: the pin's `helpers_test.go`, `status_test.go` and `security_test.go` with the
  module path rewritten and D1's dropped tests removed; `helpers_test.go:30` checks the `Status` word; new rows in
  `TestFromComponentHealth` (`Healthy=false`, `Status:"degraded"` gives `unhealthy`) and `TestAggregate` (an unknown
  word and `""` give healthy); the row at the pin's `status_test.go:208-218` expects "Component unhealthy" (Q3, fix).
  Gate: the tests fail on the base (the package does not exist; note the compile error).
- [x] 2.2 `health/status.go`, `health/helpers.go` and `health/doc.go` from the pin with the module path rewritten;
  `monitor.go` not ported; `WithMetrics`, `WithSubStatus`, `IsHealthy` and `Metrics.MessagesProcessed` removed;
  the Q3 message change. Gate: `go test -race ./health/`; `task mutate:check` detects each
  mutant D2's Tests line names; `task verify` passes on a clean tree.

## 3. Text and ledger (D3, D4)

- [ ] 3.1 `health/doc.go` and `health/README.md` rewritten per D3; the `health` row in `docs/admission-ledger.yaml`
  per D4, its `known_risks` naming the Q3 message change. Gate: `go doc ./health` lists only the kept surface;
  `task verify` passes on a clean tree, which runs `ledger:check` and `docs:check` (review stop).

## 4. Verify, review and close

- [ ] 4.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 4.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 4.3 Any defect found and not fixed is filed; then the change is archived
  (`openspec archive setup-04a-03-boot-health`) as the last content commit. Gate: `task spec:check`.
