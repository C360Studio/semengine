# Tasks: simplify-04a-12-dispatch

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`. Tasks 2.1–2.3 are written against the base API and fail by assertion, recorded before 2.4
changes the signatures; only 2.4 counts a compile failure as failing.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner, with the design's owner question answered (owner 2026-10-16, design "Owner rulings" 1). Gate: `task
  spec:check` and `task docs:check`.

## 2. Failing tests

- [x] 2.1 `TestSubmitAfterRunCancelIsProcessed` and `TestRunCancelProcessesQueuedItems` in `shutdown_test.go`
  (`synctest`): an item accepted after, or queued at, the run context's end reaches `Process` with the ended context,
  in order, and `Shutdown` returns nil (D1); the queued case uses 64 items. Gate: fails by assertion on the base
  commit. Evidence: both fail by assertion at base `185441e` (commit `39c8d13`, `notes/2.1.md`).
- [x] 2.2 `TestKeyedPoolRunContextCancelLeavesNothingRunning` becomes
  `TestKeyedPoolRunContextCancelThenShutdownLeavesNothingRunning`; new
  `TestShutdownAfterJoinReturnsNilUnderEndedContext` (64 calls, each nil) (D1). Gate: the second fails on the base
  commit. Evidence: it fails by assertion at base `185441e`; the renamed test passes there (commit `39c8d13`,
  `notes/2.2.md`).
- [x] 2.3 `TestKeyedPoolQueueDepthCountsWaitingItems` (`synctest`): one item in `Process`, three queued, no clock
  advance, gauge 3; after `Shutdown`, 0; and `TestKeyedPoolQueueDepthSumsPoolsOfOneName` (`synctest`): two pools of
  one name on one registry, both queues occupied, combined depth; one drained, the other's count stays; a replacement
  pool of that name then counts alone; 0 after all drain (D5). Gate: both fail by assertion on the base commit.
  Evidence: both fail by assertion at base `49327b7` (commit `1df03ef`, `notes/2.3.md`).
- [x] 2.4 Every caller uses `SubmitBlocking(ctx, key, work)`, `NewKeyedPool(ctx, cfg)` with `Logger` and
  `MetricsRegistry` in `KeyedConfig`, and a `Process` without an error: `keyed_pool_test.go`, `shutdown_test.go`,
  `processor/graph-ingest/lifecycle_owner_test.go`, `interrupted_work_test.go`; the "nil KeyOf" case leaves
  `TestKeyedPool_RejectsBadConfig` (D2–D4). Gate: fails to compile on the base commit. Evidence: 53 compile errors,
  all in those test files, at base `1df03ef` (commit `6155380`, `notes/2.4.md`).
- [x] 2.5 `keyed_pool_test.go` runs on the bubble clock: no `time.After` watchdog, no 100 ms or 5 s deadline, no
  `context.WithValue`; backpressure shown by `synctest.Wait()` with the submit still blocked (D6). Gate: the package
  tests pass after 3.3; `notes/2.5.md` records the `task mutate:check` run on a `SubmitBlocking` that drops instead of
  waits, detected by `TestKeyedPool_Backpressure`. Evidence: rewrite in commit `6155380`; `task mutate:check` verdict
  detection, a `default: return nil` arm caught at `keyed_pool_test.go:227` (commit `6574e16`, `notes/2.5.md`).

## 3. Implementation

- [x] 3.1 One stop path: lanes read their queue until `Shutdown` ends it, the constructor's context reaches only
  `Process`, and `Shutdown` returns nil once the lanes have exited whatever its context (D1, #131). Gate: 2.1, 2.2;
  the other `shutdown_test.go` tests and `TestKeyedPool_ShutdownSubmitRace` pass. Evidence: commit `32960aa`; all pass
  under `go test -race -count=3`; `task mutate:check` detection, lanes given back their context arm, caught by
  `TestSubmitAfterRunCancelIsProcessed` (`notes/3.1.md`).
- [x] 3.2 The key argument, one config struct, and `Process` without an error; graph-ingest's `buildIngestPool` logs
  `processIngest`'s error at Debug with the entity ID, and `component.go:1610` passes `work.entityID` (D2–D4, #136
  items 2–4). Gate: 2.4; `task test:unit`. Evidence: commit `6155380` (the submit is at `component.go:1595` after
  the merge of `origin/main`); `go test -race -count=3` of `internal/dispatch` and `processor/graph-ingest/...`
  pass, and `task test:unit` passes inside `task verify` on the commit that ticks this task (`notes/3.2.md`).
- [x] 3.3 `metricsUpdater` removed; the queue-depth gauge counts waiting items (D5, #136 item 1). Gate: 2.3;
  `TestKeyedPool_MetricsMove`, `TestKeyedPoolWritesTheRegisteredCollector` and
  `TestKeyedPoolReturnsARegistrationRefusal` pass. Evidence: commit `6155380`; all five pass under
  `go test -race -count=3` (`notes/3.3.md`).

## 4. Documentation

- [x] 4.1 `internal/dispatch/doc.go` and `errors.go`, the `KeyOf` comment at `component.go:1567`, the `processIngest`
  comment (`keyed_ingest.go:212-215`), and the
  `internal/dispatch` ledger row's contract and `dependencies_and_side_effects` state D1–D5 (D7). Gate:
  `task ledger:check`, `task docs:check`. Evidence: commit `60b0605` (the `KeyOf` comment is at `component.go:1552`
  after the merge of `origin/main`; `Shutdown`'s comment is fixed there too); both gates pass, also inside
  `task verify` on the commit that ticks this task.

## 5. Verify, review and close

- [x] 5.1 `task verify` passes on a clean tree at the implementation head (exit 0 at `ee73ec1`).
- [x] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body (Codex
  APPROVE at `566f667`, PR comment 6225683944; batch 1 at `d614c71`, comment 6224927071).
- [x] 5.3 Archive the change (`openspec archive simplify-04a-12-dispatch`) as the last content commit. Gate:
  `task spec:check`.
