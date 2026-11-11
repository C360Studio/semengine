# Tasks: alpha4-pkg-buffer

Quick track, with `design.md` for the port rules (#159); no spec delta. Tests come first. Each code task ends at a
clean-tree local `task verify` (#171). "Shown to fail" is failing-first output or `task mutate:check`, recorded in
`notes/<task>.md`. No `time.Sleep`, no `Skip`, no test deadline of its own, no `context.WithValue`. Review stops:
after 2.2, before archive.

## 1. Design

- [x] 1.1 Inventory review passes (`INVENTORY PASS`), then pre-owner design review; the owner answers Q1 and
  fills `Approved-by:`. Gate: `task spec:check`, `task docs:check`.

## 2. Port

- [x] 2.1 Port `pkg/buffer` under the SemEngine module path with R1 (drop `WithStats`, `StatsFromContext`,
  `ContextKeyStats`), R2 (metrics through `metric.RegisterOrGet` under `semengine`) and R10 (drop the `MemoryUsage`
  family), so it builds; `TestMetricsOnInjectedRegistry`. Then the failing-first tests
  `TestWriteWithContextRefusesNilContext` (R5) and `TestDropCallbackMayUseBuffer` (R6, a hang to `go test -timeout`),
  output in `notes/2.1.md`. Gate: `task build`; the two failures are recorded.
- [x] 2.2 Fix R3 to R6 and add R11 (the two bounded writes on `Buffer[T]`) as the design states them, and add
  the behaviour pins `TestWriteWithTimeoutAssertable`,
  `TestWriteWithTimeoutExpires`, `TestWriteWithTimeoutSucceedsWhenSpaceFrees`, `TestWriteWithContextLeavesNothing`
  and `TestCapacityBelowOneIsOne`. Show each pin able to fail with `task mutate:check`, recorded in `notes/2.2.md`.
  Remove the root; no goroutine outlives the call (joined or removed); the review checks both by reading
  `circular.go`.
  - Rewrite the pin tests at `buffer_test.go:438-449`, `:467`, `:471-482`, `:508` and `:643-703` in `synctest`
    without sleeps or elapsed bounds (R7).
  - Doc comments for R9.

  Gate: `task test:unit`, `task test:repeat`, `task lint`, `task verify` on a clean tree.

## 3. Close-out

- [x] 3.1 Ledger row `pkg/buffer` (`adapt`) in `docs/admission-ledger.yaml`. PR body: `Closes #566`. Gate:
  `task ledger:diff -- pkg/buffer`, the ledger contract test.
- [x] 3.2 `pkg/buffer/README.md` and `doc.go`: the module path; examples that compile, calling the bounded writes
  on `Buffer[T]` (R8, R11);
  the dropped context helpers removed; markdownlint clean. Also `pkg/buffer/MIGRATION.md`, and a `pkg/buffer/` row in
  `docs/repository-map.md`. Gate: `task docs:check`, `go test -run Example ./pkg/buffer/`.
- [ ] 3.3 Archive (`openspec archive alpha4-pkg-buffer`) as the last content commit. Gate: `task spec:check`.
