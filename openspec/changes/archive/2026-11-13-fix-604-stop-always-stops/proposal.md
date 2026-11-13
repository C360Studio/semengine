# Proposal: fix-604-stop-always-stops

Approved-by: cglusky (2026-11-13) sha256:9f4e3a62e187

Track: `design`. Capabilities modified: `service-lifecycle`, `component-lifecycle`.

## Why

Issue `#604` reports that when one component exhausts the stop context budget during `ComponentManager.Stop(ctx)`,
`stopComponents` skips every remaining started component (`service/component_manager_lifecycle.go:184-187`), leaving
their stores registered and their goroutines running. `inventory.md` (`INVENTORY PASS` at `bfbfd6bc`) shows `#604` is
one symptom of two incompatible teardown contracts in the tree: `ADR-113` (`docs/adr/113-start-context-bounds-start.md:54-61`),
`ADR-112`, `natsclient.Client.Close`, `metric.Server.Stop`, and `background-work` initiate teardown unconditionally and
use `ctx` only to bound waiting, while `lifecycleguard.Guard`, `service.Manager.StopAll`, `ComponentManager`, four
services, and four components treat an ended `ctx` as "stop nothing" or return early before canceling remaining work.

## What Changes

- Unify `lifecycleguard.Guard.Stop`, `component.Lifecycle.Stop`, `service.Manager.StopAll`, `ComponentManager.Stop`,
  `config.Manager.Stop`, and service/component `Stop` on `ADR-113`'s rule: `Stop(nil)` / `StopAll(nil)` refuses with
  `errs.ErrInvalidData` and stops nothing; before `Start` (closing any pre-`Start` handle under `ctx`'s bound, or
  returning `nil` immediately when none is held) or after a `nil`-returning `Stop`, `Stop(ctx)` returns `nil` once
  nothing remains held; during an in-flight `Start`, `Stop(ctx)` waits up to `ctx` for `Start` to return; on a started
  owner not yet stopped, any non-nil `ctx` (live or already ended) initiates teardown across every owned admission,
  store registration, listener, subscription, pool, and run context, uses `ctx` solely to bound graceful drain and
  goroutine joins, returns `ctx.Err()` when `ctx` ends first or had already ended, and retains unjoined work for retry
  on a later `Stop`.
- Remove the `ctx.Err()` skip in `ComponentManager.stopComponents` (`Closes #604`), the `ctx.Err()` gate in
  `service.Manager.StopAll`, the `ctx.Err() == nil` fence gates in `MessageLogger.Stop` and `Metrics.Stop`, and the
  sequential early-return skips in `HeartbeatService`, `MetricsForwarder`, `StorageObservabilityService`,
  `MessageLogger`, `graph-embedding`, `graph-index`, `graph-query`, and `websocket`.
- Update `service`, `config`, and `component` docs/comments, `lifecycletest` reference-owner tests, and
  `docs/admission-ledger.yaml` contract notes (`:3158-3159,3172,3291`).

## Non-goals

Replacing `Start(ctx)` / `Stop(ctx)` with `Run(ctx)` on services or components; adding a `Lifecycle.Go` helper;
changing `natsfixture.Fixture.Stop` (`internal/harness/natsfixture/stop.go:26-28`, external Docker HTTP removal in the
test harness); or editing `semsource` consumer repositories in this pull request.

## Impact

Zero new or removed exported symbols, signatures, subjects, buckets, or config keys (`inventory.md` §4). Touches
`internal/lifecycleguard`, `component`, `service`, `config` (inherited `Guard` behavior + docs),
`processor/graph-{embedding,index,query}`, `output/websocket`, `internal/harness/lifecycletest`, and
`docs/admission-ledger.yaml`. Single pull request (`#649`, `Closes #604`).

## Rulings

Covered (owner, conversation 2026-11-13): keep `Start(ctx)` / `Stop(ctx)` without `Lifecycle.Go`; drop "ended `ctx`
stops nothing" across `Guard`, managers, services, and components in PR `#649`. Open for owner: `Approved-by:` stamp.
