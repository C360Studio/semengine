# Tasks: fix-442-graph-ingest-consumer-name

Light process, owner 2026-10-28: proposal, tasks and spec delta; no design document; Codex reviews the code.

## 1. Proposal

- [x] 1.1 Proposal, tasks and spec delta committed. Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [x] 2.1 `TestConfigRefusesDeclaredConsumerName` in `processor/graph-ingest/config_strict_test.go`, failing on the
  base commit: a JetStream input declaring `consumer_name` is accepted by `DeclarePorts`, `CreateGraphIngest` and
  `Validate`. Gate: `go test -run TestConfigRefusesDeclaredConsumerName ./processor/graph-ingest/` fails on base.
- [ ] 2.2 `Config.Validate` refuses a JetStream input declaring `consumer_name`, naming the port and the key. Gate:
  the test in 2.1 passes; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record on the PR. The PR body carries `implemented-by: claude (opus)` and
  `reviewed-by: codex`.
  Hold: Codex reviews the code; the owner starts Codex.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`, syncing the added requirement into
  `openspec/specs/graph-ingest-recovery/spec.md`). Gate: `task spec:check` and `task docs:check`.
