# Tasks: simplify-04a-12-dispatch

Each task names its outcome and the gate that proves it. Tests come first. Record each one failing on the base commit
in `notes/<task>.md`. Tasks 2.1–2.3 are written against the base API and fail by assertion, recorded before 2.4
changes the signatures; only 2.4 counts a compile failure as failing.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal, design and spec delta pass pre-owner review and are accepted
  by the owner, with the design's owner question answered. Gate: `task spec:check` and `task docs:check`.

## 2. Failing tests

- [ ] 2.1 `TestSubmitAfterRunCancelIsProcessed` and `TestRunCancelProcessesQueuedItems` in `shutdown_test.go`
  (`synctest`): an item accepted after, or queued at, the run context's end reaches `Process` with the ended context,
  in order, and `Shutdown` returns nil (D1); the queued case uses 64 items. Gate: fails by assertion on the base
  commit.
- [ ] 2.2 `TestKeyedPoolRunContextCancelLeavesNothingRunning` becomes
  `TestKeyedPoolRunContextCancelThenShutdownLeavesNothingRunning`; new
  `TestShutdownAfterJoinReturnsNilUnderEndedContext` (64 calls, each nil) (D1). Gate: the second fails on the base
  commit.
- [ ] 2.3 `TestKeyedPoolQueueDepthCountsWaitingItems` (`synctest`): one item in `Process`, three queued, no clock
  advance, gauge 3; after `Shutdown`, 0 (D5). Gate: fails by assertion on the base commit.
- [ ] 2.4 Every caller uses `SubmitBlocking(ctx, key, work)`, `NewKeyedPool(ctx, cfg)` with `Logger` and
  `MetricsRegistry` in `KeyedConfig`, and a `Process` without an error: `keyed_pool_test.go`, `shutdown_test.go`,
  `processor/graph-ingest/lifecycle_owner_test.go`, `interrupted_work_test.go`; the "nil KeyOf" case leaves
  `TestKeyedPool_RejectsBadConfig` (D2–D4). Gate: fails to compile on the base commit.
- [ ] 2.5 `keyed_pool_test.go` runs on the bubble clock: no `time.After` watchdog, no 100 ms or 5 s deadline, no
  `context.WithValue`; backpressure shown by `synctest.Wait()` with the submit still blocked (D6). Gate: the package
  tests pass after 3.3; `notes/2.5.md` records the `task mutate:check` run on a `SubmitBlocking` that drops instead of waits,
  detected by `TestKeyedPool_Backpressure`.

## 3. Implementation

- [ ] 3.1 One stop path: lanes read their queue until `Shutdown` ends it, the constructor's context reaches only
  `Process`, and `Shutdown` returns nil once the lanes have exited whatever its context (D1, #131). Gate: 2.1, 2.2;
  the other `shutdown_test.go` tests and `TestKeyedPool_ShutdownSubmitRace` pass.
- [ ] 3.2 The key argument, one config struct, and `Process` without an error; graph-ingest's `buildIngestPool` logs
  `processIngest`'s error at Debug with the entity ID, and `component.go:1610` passes `work.entityID` (D2–D4, #136
  items 2–4). Gate: 2.4; `task test:unit`.
- [ ] 3.3 `metricsUpdater` removed; the queue-depth gauge counts waiting items (D5, #136 item 1). Gate: 2.3;
  `TestKeyedPool_MetricsMove`, `TestKeyedPoolWritesTheRegisteredCollector` and
  `TestKeyedPoolReturnsARegistrationRefusal` pass.

## 4. Documentation

- [ ] 4.1 `internal/dispatch/doc.go` and `errors.go`, the `KeyOf` comment at `component.go:1567`, the `processIngest`
  comment (`keyed_ingest.go:212-215`), and the
  `internal/dispatch` ledger row's contract and `dependencies_and_side_effects` state D1–D5 (D7). Gate:
  `task ledger:check`, `task docs:check`.

## 5. Verify, review and close

- [ ] 5.1 `task verify` passes on a clean tree at the implementation head.
- [ ] 5.2 Cross-agent review of record on the implementation commit; `reviewed-by:` in the PR body.
- [ ] 5.3 Archive the change (`openspec archive simplify-04a-12-dispatch`) as the last content commit. Gate:
  `task spec:check`.
