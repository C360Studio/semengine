# Tasks: fix-442-graph-ingest-consumer-name

Light process, owner 2026-10-28: proposal, tasks and spec delta; no design document; Codex reviews the code.

## 1. Proposal

- [x] 1.1 Proposal, tasks and spec delta committed. Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [x] 2.1 `TestConfigRefusesDeclaredConsumerName` in `processor/graph-ingest/config_strict_test.go`, failing on the
  base commit: a JetStream input declaring `consumer_name` is accepted by `DeclarePorts`, `CreateGraphIngest` and
  `Validate`. Gate: `go test -run TestConfigRefusesDeclaredConsumerName ./processor/graph-ingest/` fails on base.
- [x] 2.2 `Config.Validate` refuses a JetStream input declaring `consumer_name`, naming the port and the key. Gate:
  the test in 2.1 passes; `task verify` passes on a clean tree.

## 3. Code review

- [x] 3.1 Codex code review of record on PR #445: APPROVE, no actionable findings at
  `cd82c17aed7114bf5e5e38157fbb04a365c81c16` (PR comment 6470980375, 2026-10-28T13:58:54Z). The PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`.

## 4. Archive

- [x] 4.1 Archive as the last content commit (`openspec archive`, syncing the added requirement into
  `openspec/specs/graph-ingest-recovery/spec.md`). Gate: `task spec:check` and `task docs:check`.
