# Tasks: simplify-04a-01-timestamp-ledger

Each task names the outcome and the gate that proves it.

## 1. Design

- [x] 1.1 Clarify scope and overlap boundaries with open PR #206, and scaffold `.openspec.yaml`, `proposal.md`,
  `design.md`, and `tasks.md`. Gate: `task spec:check` and `task docs:check`.

## 2. Implementation and tests

- [ ] 2.1 Remove dead `timestamp.Parse(input any) int64` from `internal/timestamp/timestamp.go`, its doc-comment
  passages in `internal/timestamp/timestamp.go` and `internal/timestamp/doc.go`, `ExampleParse` in
  `internal/timestamp/example_test.go`, and `TestParse*` / `BenchmarkParse*` in `internal/timestamp/timestamp_test.go`.
  Gate: `task test:unit`.
- [ ] 2.2 Convert `pkg/timestamp` from `carry` to `adapt` in `docs/admission-ledger.yaml`, trim verbose `adapt` /
  `repair-before-port` rows across `docs/admission-ledger.yaml` (leaving the three entries touched by PR #206
  byte-identical), and update `docs/provenance.md:75-80`. Gate: `task ledger:check` and `task verify`.

## 3. Review and close

- [ ] 3.1 Cross-agent review of record on the implementation commit; record `reviewed-by:` in the PR body once the
  reviewing agent's newest review record approves.
- [ ] 3.2 Archive the change (`openspec archive simplify-04a-01-timestamp-ledger`) as the last content commit. Gate:
  `task spec:check`.
