# Tasks: fix-441-graph-ingest-stop-join

Light process, as on #454 and #456: proposal, tasks and spec delta; no design document; Codex reviews the code.

## 1. Proposal

- [ ] 1.1 Proposal, tasks and spec delta committed; Claude inventory review of the proposal. Gate: `task spec:check`
  and `task docs:check`.

## 2. Code

- [ ] 2.1 `TestStopJoinsCallbackAfterConnectionLoss` in `processor/graph-ingest/stop_join_integration_test.go`, failing
  on the base commit: with a delivery callback held after the connection closed for good and `Closed()` fired, `Stop`
  returns without waiting for it. Gate: the test fails on base.
- [ ] 2.2 `cleanup` waits through `WaitConsumerHandlers` in place of `Closed()`; its doc comment says why. Gate: the
  test in 2.1, `TestGraphIngestLifecycleSuite` and the package's integration tests pass; `task verify` passes on a
  clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record; the PR body carries `implemented-by: claude (opus)` and `reviewed-by: codex`.
  Hold: Codex reviews the code (owner starts Codex).

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`, syncing the added requirement into
  `openspec/specs/graph-ingest-recovery/spec.md`). Gate: `task spec:check` and `task docs:check`.
