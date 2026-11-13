# Tasks: fix-604-stop-always-stops

Tests come first. Each code task ends at a clean-tree local verification gate (`preflight` skill), with full
`task verify` before push. Premises cite `inventory.md` (`INVENTORY PASS` at `bfbfd6bc`, pin tidy `1dabaa0ceaf6`) and
`design.md`. Single PR (`#649`, `Closes #604`).

## 1. Design

- [ ] 1.1 Architect inventory and design (`inventory.md`, `proposal.md`, `design.md`, `specs/`): unify `Stop(ctx)` /
  `StopAll(ctx)` teardown initiation under bounded and ended contexts (`ADR-113`, `ADR-112`, `#604`). Pre-owner design
  review passes (`DESIGN REVIEW PASS`); `task spec:check` and `task docs:check` pass.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Code

- [ ] 2.1 `internal/lifecycleguard`, `component.Lifecycle`, and `lifecycletest` reference double (design D1, I1-I4).
  Premise: `Guard.Stop` refuses ended `ctx` before `cleanup` (`internal/lifecycleguard/guard.go:109-111`) and does not
  re-check `startDone` if `Start` finishes concurrently with `ctx.Done()` (`:123-130`); `component.Lifecycle` wraps
  `Guard` (`component/lifecycle_type.go:73-77`, doc at `component/lifecycle.go:77-79`); `refowner.Stop` refuses ended
  `ctx` before `stopOnce` (`internal/harness/lifecycletest/refowner_test.go:149-164`); `CheckStopReturnsItsEndedContext`
  (`internal/harness/lifecycletest/lifecycletest.go:219-237`) requires `Stop(endedCtx)` on a started owner to return
  `endedCtx`'s error/cause even when synchronous cleanup returns `nil`. Accept: failing-first tests
  `TestGuardStopInitiatesCleanupWithEndedContext` (covers held cleanup retry, synchronous `nil` cleanup becoming
  terminal while returning `ctx.Err()`, `Stop` before `Start` with ended `ctx` returning `nil`, and `Start` finishing
  just before `Stop(endedCtx)` running `cleanup`) and `TestLifecycleStopWithEndedContextBeginsCleanup` pass;
  `TestGuardRefusesNilAndEndedContexts` updated for nil-only refusal; `internal/harness/lifecycletest` tests pass; a
  mutant restoring `if err := ctx.Err(); err != nil { return err }` at the top of `Guard.Stop` is shown to fail via
  `task mutate:check`.
- [ ] 2.2 `service` managers and services (`BaseService`, `LogForwarder`, `Manager`, `ComponentManager`, `Metrics`,
  `MessageLogger`, `HeartbeatService`, `MetricsForwarder`, `StorageObservabilityService`) (design D1, D3, I1-I4;
  `Closes #604`). Premise: `Manager.StopAll` gates on `ctx.Err()` (`service/service_manager.go:677-679`);
  `ComponentManager.stopComponents` skips remaining components and store deregistration on `ctx.Err()`
  (`service/component_manager_lifecycle.go:184-187`); `MessageLogger.Stop` (`service/message_logger.go:515`) and
  `Metrics.Stop` (`service/metrics.go:152`) gate admission fences on `ctx.Err() == nil`; `HeartbeatService`
  (`service/heartbeat.go:188-196`), `MetricsForwarder` (`service/metrics_forwarder.go:190-198`),
  `StorageObservabilityService` (`service/storage_observability.go:302-310`), and `MessageLogger.cleanup`
  (`service/message_logger.go:529-550`) return early before `BaseService.Stop` / `runCancel` / `endWatchStreams`;
  existing tests `TestStopWithAnEndedContextStopsNothing` (`service/base_lifecycle_test.go:103`),
  `TestStopAllWithAnEndedContextStopsNothing` (`service/service_manager_test.go:506`), and
  `TestPropComponentManagerStopHistory` (`service/component_manager_prop_test.go:88-92,489-499`) encode Spelling B.
  Accept: failing-first tests `TestComponentManagerStopTimeoutStillStopsRemainingComponents` (using `component.Lifecycle`
  owners so the expired-`ctx` pass initiates cleanup + store deregistration on all remaining components and returns
  joined `ctx` errors, and the live-`ctx` retry returns `nil` without re-running already-finished cleanups),
  `TestStopWithAnEndedContextBeginsCleanupAndReportsContextError`,
  `TestStopAllWithAnEndedContextBeginsTeardownAndReportsContextError`,
  `TestMessageLoggerStopWithEndedContextInitiatesTeardown`, and `BaseService`-stopped assertions on cut-short `Stop` in
  `HeartbeatService`, `MetricsForwarder`, and `StorageObservabilityService` pass; updated
  `TestPropComponentManagerStopHistory` passes; every `lifecycle-suite` test in `service` and `config` passes; a mutant
  restoring the `ctx.Err()` `continue` in `stopComponents` fails `TestComponentManagerStopTimeoutStillStopsRemainingComponents`
  via `task mutate:check`.
- [ ] 2.3 Multi-resource component `cleanup(ctx)` alignment (`graph-embedding`, `graph-index`, `graph-query`,
  `websocket`, and `graph-ingest` timeout test) (design D1, D3, I2-I4). Premise: `graph-embedding`
  (`processor/graph-embedding/component.go:411-435`), `graph-index` (`processor/graph-index/component.go:308-332`),
  `graph-query` (`processor/graph-query/component.go:212-228`), and `websocket` (`output/websocket/websocket.go:263-295`)
  return early when `sub.Drain(ctx)` or an earlier channel wait returns `ctx.Err()`, skipping `runCancel()`,
  pool/coalescer shutdown, or client close; `processor/graph-ingest/component_integration_test.go:119`
  (`TestComponent_Stop_Timeout`) and `processor/graph-index/lifecycle_integration_test.go:277-285` expect Spelling B;
  `gateway/graph-gateway/component.go:132-135` passes `lifecycle-suite` via D1's `Guard.Stop` rule. Accept: failing-first
  unit/integration tests proving `Stop` with an expired/cancelled `ctx` still cancels `runCancel` (and closes websocket
  clients / initiates pool & coalescer shutdown) in `graph-embedding`, `graph-index`, `graph-query`, and `websocket`,
  followed by a clean retry `Stop(liveCtx)`, pass; `TestComponent_Stop_Timeout` in `graph-ingest` updated to assert
  teardown initiated and retry succeeds; `gateway/graph-gateway` lifecycle suite passes.
- [ ] 2.4 Docs, Go doc comments, admission ledger notes, and OpenSpec archive (design Migration Plan). Premise:
  Spelling B prose in `service/README.md:54-56,153-155,275-281,463-464,811-813`,
  `service/doc.go:47-49,133-136,237-245,344-346`, `service/base.go:215-216`, `service/service_manager.go:659-660`,
  `service/component_manager_lifecycle.go:132-133,141-143,174-175`, `config/README.md:211`, `config/doc.go:94`,
  `config/manager.go:535-538`, `component/lifecycle.go:77-79`, and `docs/admission-ledger.yaml:3158-3159,3172,3291`.
  Accept: every listed comment, README, doc.go, and ledger contract note reflects D1/D3; `openspec archive
  fix-604-stop-always-stops --yes` promotes the spec deltas cleanly; full `task verify` passes on a clean tree.
