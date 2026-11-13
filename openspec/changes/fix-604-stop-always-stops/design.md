# Design: fix-604-stop-always-stops

## Context

Facts from `inventory.md` (`INVENTORY PASS` at `bfbfd6bc`, pin tidy `1dabaa0ceaf6`): `ComponentManager.stopComponents`
skips remaining started components when `ctx.Err() != nil` (`service/component_manager_lifecycle.go:184-187`; `#604`),
and `service.Manager.stopAll` passes an expired `ctx` to remaining services (`service/service_manager.go:709-715`) whose
`lifecycleguard.Guard.Stop` returns `ctx.Err()` before running `cleanup` (`internal/lifecycleguard/guard.go:109-111`).
The tree splits between Spelling A (`ADR-113` `docs/adr/113-start-context-bounds-start.md:54-61`, `ADR-112`,
`natsclient/client.go:634-637,703-707`, `metric/handler.go:204-254`, `openspec/specs/background-work/spec.md:53-55`,
`processor/graph-ingest/component.go:553-593`, `storage/objectstore/component.go:513-539`) and Spelling B (`guard.go`,
`service_manager.go:677-679`, `message_logger.go:515,529-550`, `metrics.go:152`, `heartbeat.go:188-196`,
`metrics_forwarder.go:190-198`, `storage_observability.go:302-310`, `graph-embedding/component.go:411-435`,
`graph-index/component.go:308-332`, `graph-query/component.go:212-228`, `output/websocket/websocket.go:263-295`,
`lifecycletest/refowner_test.go:149-164`). Zero production structs retain `context.Context` (`inventory.md:154`).
Governing ADRs kept unchanged: `ADR-113`, `ADR-108` (`component.Lifecycle`), `ADR-095` (one-shot lifecycle + retained
failed-`Start` cleanup), `ADR-094` (boot-only composition), and `ADR-112` (`natsclient` close semantics). Open PRs
`#648`, `#598`, `#597` (`inventory.md:102-105`) have zero file or capability overlap; either `#649` or any of them may
merge first (`#648`'s `graph-lexical` already cancels `runCancel` unconditionally in `cleanup`).

## Goals / Non-Goals

Goals: retire `#604`; give every runtime lifecycle owner (`Guard`, `component.Lifecycle`, `service.Manager`,
`ComponentManager`, `config.Manager`, services, components) one observable `Stop(ctx)` / `StopAll(ctx)` contract where
shutdown always initiates and `ctx` bounds only drain/join waits. Non-Goals: replacing `Start`/`Stop` with `Run(ctx)`;
adding `Lifecycle.Go`; changing `natsfixture.Fixture.Stop` (`internal/harness/natsfixture/stop.go:26-28`, test-only
Docker HTTP container removal requiring a live request context, swept after 30m by `nats-fixture` spec); or editing
`semsource` in this repo (`processor/ast-source/component.go:1039-1041` noted for consumer follow-up).

## Decisions

**D1. Ended or expiring `Stop` context: initiate teardown unconditionally; `ctx` bounds only waits (Spelling A1, chosen).**
Observable rules for `Guard.Stop`, `component.Lifecycle.Stop`, `service.Manager.StopAll`, `ComponentManager.Stop`,
`config.Manager.Stop`, and every service/component `Stop`:

1. `ctx == nil` returns an `errs.ErrInvalidData` error and mutates nothing.
2. Before `Start` / `StartAll`, `Stop` / `StopAll` returns `nil` for any non-nil `ctx` when no pre-`Start` handle is
   held, and when one is (`StartHealthListener`, pre-`Start` `MessageLogger` watches, `config.Manager` subscriber
   channels) initiates its close and bounds any join wait by `ctx`. After an earlier `Stop` / `StopAll` (or
   failed-`Start` rollback) joined everything and returned `nil`, every later call returns `nil` for any non-nil `ctx`.
3. While `Start` is still running, `Stop(ctx)` waits up to `ctx` for `Start` to return; if `ctx` ends while `Start` is
   still running, `Stop` returns `ctx.Err()` without running cleanup concurrently with `Start` (`ADR-113`, `ADR-095`).
4. On a started owner not yet stopped, any non-nil `ctx` — live or already ended — clears readiness, fences admissions,
   deregisters provided stores, and initiates shutdown across every child service, component, listener, subscription,
   pool, and run context in reverse start order. `ctx` bounds only waiting for graceful drain and goroutine joins. If
   `ctx` ends before all joins finish, `Stop` returns an error matching `ctx.Err()` / `context.Cause(ctx)` and keeps
   unjoined items for retry on a later `Stop`. If all owned work finishes synchronously during `Stop(endedCtx)` (e.g. an
   idle owner such as `graph-gateway`, `inventory.md:55`), the owner reaches terminal stopped (a later `Stop` returns
   `nil`) while this `Stop(endedCtx)` still returns `ctx.Err()`, satisfying `CheckStopReturnsItsEndedContext`
   (`lifecycletest.go:219-237`) and matching `metric.Server.Stop` (`metric/handler.go:249-251`) and `ADR-112`.

- *Rejected — Do nothing (keep Spelling B):* zero diff, but leaves `#604` open and leaks running components/services and
  registered stores whenever one child exhausts a shared shutdown budget (`inventory.md:161-167`). Forecloses safe
  single-deadline `manager.StopAll(stopCtx)`.
- *Rejected — Per-component detached timeout inside `ComponentManager.stopComponents` only:* 1-file diff, but invents
  hidden fixed timeouts violating caller-bounded `Stop(ctx)` (`ADR-113`, `background-work` spec:53-55), multiplies
  shutdown tail latency by `N * timeout`, and still leaves `service.Manager.stopAll` skipping remaining services once
  `stopCtx` expires. Forecloses caller deadline control.
- *Adopter seam:* composition roots (`semsource` `cmd/semsource/run.go:227-232`) hold one rule: pass a bounded non-nil
  `ctx` to `StopAll`/`Stop`. If they do nothing else and a child stalls past the deadline, every sibling component and
  service still fences admissions, deregisters stores, and cancels its run context before `StopAll` returns.

**D2. Keep `Start(ctx)` / `Stop(ctx)` without adding `Lifecycle.Go` (chosen).**
Preserves `ComponentManager`'s two-phase `StoreProvider` -> consumer startup barrier (`component_manager_lifecycle.go:37-54`),
reverse teardown before store close (`:178-199`), and bounded drain before `runCancel` (`ADR-108`, `ADR-113`), using
stdlib `sync.WaitGroup.Go` + `context.WithCancel`.

- *Rejected — Migrate owners to `Run(ctx) error`:* rewrites every service, component, and `semsource` adapter (`inventory.md`
  §4) and forecloses synchronous startup-readiness return and post-cancel drain budgets without sideband channels.
- *Rejected — Add `Lifecycle.Go(fn)`:* adds framework API with zero caller need (`inventory.md` §4), duplicating
  `sync.WaitGroup.Go` and foreclosing minimal `component.Lifecycle` surface (`ADR-108`).
- *Adopter seam:* zero signature or wiring change; existing consumers compile and gain D1 automatically.

**D3. Multi-resource `cleanup(ctx)` initiates every step and joins errors (chosen).**
`HeartbeatService`, `MetricsForwarder`, `StorageObservabilityService`, `MessageLogger`, `graph-embedding`, `graph-index`,
`graph-query`, and `websocket` (`inventory.md:68-75`) adopt `graph-ingest` / `objectstore`'s pattern
(`graph-ingest/component.go:553-593`): fence admissions (`MessageLogger.watchesClosed`, `Metrics.lifecycleCalled`) on
any non-nil `Stop`, initiate every drain/close/pool-shutdown/`runCancel`/`BaseService.Stop(ctx)` without returning early
on a prior `ctx.Err()`, and join errors so `Guard` retries until all owned work exits.

- *Rejected — Change `Guard` and managers only:* smaller diff, but when `Stop(expiredCtx)` reaches those eight owners,
  `sub.Drain(ctx)` or the first loop wait returns `ctx.Err()` before `runCancel()` / `BaseService.Stop(ctx)`, leaving
  worker goroutines and `BaseService` running (`inventory.md:68-75`). Forecloses D1's guarantee on real components.
- *Adopter seam:* component authors follow one rule in `component/lifecycle.go`: `cleanup(ctx)` initiates every owned
  stop and uses `ctx` only to bound waits; doing nothing in existing `graph-ingest`/`objectstore`/`graph-clustering`/`graph-gateway`
  already conforms.

## Invariants

- **I1 (Nil refusal):** `Stop(nil)` / `StopAll(nil)` returns `errs.ErrInvalidData` and stops nothing (`service-lifecycle`
  modified `Stop`/`StopAll` requirements; `lifecycle-suite` `CheckNilStopContextRefused`).
- **I2 (Unconditional teardown initiation):** On any started unstopped owner, `Stop(ctx)` / `StopAll(ctx)` with any
  non-nil `ctx` (live or ended) clears running/ready status, deregisters provided stores, and initiates stop across all
  started children and owned loops/subscriptions/listeners in reverse order (`component-lifecycle` `The shell is running
  from start's return until cleanup begins`; `service-lifecycle` `StopAll releases services...` and `The component
  manager's Stop...`; `TestPropComponentManagerStopHistory`).
- **I3 (Ended-context error & terminal no-op):** `Stop(endedCtx)` on a started unstopped owner returns an error matching
  `endedCtx.Err()` / `context.Cause(endedCtx)` (`lifecycle-suite` `Stop checks`); once every owned resource has joined,
  every later `Stop(nonNilCtx)` returns `nil` (`service-lifecycle` `A component whose Stop failed is stopped again`).
- **I4 (Retry authority):** Any child or goroutine whose join outlasts `ctx` remains unstopped so a later `Stop(liveCtx)`
  waits for it and returns `nil` only after completion (`service-lifecycle` retry scenarios; `TestPropComponentManagerStopHistory`).

## Risks / Trade-offs

- When `ctx` has already expired, `Subscription.Drain(ctx)` (`natsclient/client.go:984-1006`) starts native `Drain()`
  once and returns `ctx.Err()` immediately, so `cleanup` cancels `runCancel()` without waiting for broker flush — the
  expected trade-off once the caller's graceful-drain budget is exhausted.

## Migration Plan

Single PR (`#649`, `Closes #604`) landing code, failing-first tests, docs, `docs/admission-ledger.yaml` contract-note
updates, and OpenSpec archive. Independent of open PRs `#648`, `#598`, `#597` (either merge order passes `task verify`).

## Open Questions

None beyond owner `Approved-by:` approval of `proposal.md` and `design.md`.
