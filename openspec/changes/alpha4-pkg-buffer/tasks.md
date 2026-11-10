# Tasks: alpha4-pkg-buffer

Quick track, with `design.md` for the port rules (#159); no spec delta. Tests come first. Each code task ends at a
clean-tree local `task verify` (#171). "Shown to fail" is failing-first output or `task mutate:check`, recorded in
`notes/<task>.md`. No `time.Sleep`, no `Skip`, no test deadline of its own, no `context.WithValue`. Review stops:
after 2.2, before archive.

## 1. Design

- [ ] 1.1 Inventory review passes (`INVENTORY PASS`), then pre-owner design review; the owner answers Q1 and Q2 and
  fills `Approved-by:`. Gate: `task spec:check`, `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Port

- [ ] 2.1 Port `pkg/buffer` with only the module path changed, then the failing-first tests (design D2, R3 to R6):
  `TestWriteWithTimeoutAssertable`, `TestWriteWithTimeoutExpires`, `TestWriteWithTimeoutSucceedsWhenSpaceFrees`,
  `TestWriteWithContextLeavesNothing`, `TestWriteWithContextRefusesNilContext`, `TestDropCallbackMayUseBuffer`,
  `TestMetricsOnInjectedRegistry`, `TestCapacityBelowOneIsOne`. Each fails or hangs on that copy; output in
  `notes/2.1.md`. Gate: the failures are recorded.
- [ ] 2.2 Make them pass:
  - Drop `WithStats`, `StatsFromContext` and `ContextKeyStats` (R1).
  - Register metrics through `metric.RegisterOrGet` under `semengine` (R2).
  - R3 to R6 as the design states them.
  - Rewrite the pin tests at `buffer_test.go:445-447`, `:467`, `:480-482`, `:508` and `:643-703` in `synctest`
    without sleeps or elapsed bounds (R7).
  - Doc comments for R9 and O4.

  Gate: `task test:unit`, `task test:repeat`, `task lint`, `task verify` on a clean tree.

## 3. Close-out

- [ ] 3.1 Ledger row `pkg/buffer` (`adapt`) in `docs/admission-ledger.yaml`. PR body: `Closes #566`. Gate:
  `task ledger:diff -- pkg/buffer`, the ledger contract test.
- [ ] 3.2 `pkg/buffer/README.md` and `doc.go`: the module path; examples that compile, with the type assertion (R8);
  the dropped context helpers removed; markdownlint clean. Also `pkg/buffer/MIGRATION.md`, and a `pkg/buffer/` row in
  `docs/repository-map.md`. Gate: `task docs:check`, `go test -run Example ./pkg/buffer/`.
- [ ] 3.3 Archive (`openspec archive alpha4-pkg-buffer`) as the last content commit. Gate: `task spec:check`.
