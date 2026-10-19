# Proposal: start-context

## Why

ADR-113 (`docs/adr/113-start-context-bounds-start.md`, accepted; owner ruling #282 comment 6249934379, watcher
correction 6250135029) decides that the context given to a component's `Start` bounds only `Start`. Ending it after
`Start` returns stops nothing; `Stop(ctx)` is the only stop. Today the lifecycle suite, the `LifecycleComponent` doc
comment, `metric.Server` and graph-ingest's `start` all state or implement the older rule, under which the `Start`
context owned the component's running lifetime. This is PR 2 of the order recorded on #282 (point 5), after the
lifecycleguard tidy (#308, merged `1dc132b`). It retires #282.

## What Changes

- **The suite.** Two of the eight lifecycle-suite checks are replaced: `StartContextEndStopsNothing` (cancel the
  Start context after `Start` returns; the owner must still hold everything it reported; `Stop` then releases it all)
  and `StopReturnsItsEndedContext` (a `Stop` whose own context has ended returns that context's error or cause; the
  Start context is no longer cancelled first). The sensitivity double gains one failpoint, an owner whose work dies
  with its Start context, and its clean form adopts the contract (design D1-D4).
- **Adopters, same PR.** `metric.Server` stops handing the Start context to served requests (D5). graph-ingest's
  `start` runs its startup work (connect, buckets, the snapshot sweep, consumer setup) under the Start context and its
  continuing work (pool, subscriptions, request handlers, the entity cache's expiry sweep, the status loop) under a
  run context it creates and `release` ends (D6). Its lifecycle adapter reports the status loop while it runs, which
  is what makes the new check fail on today's code. The guard's test owner and natsclient pass unchanged (D7).
- **Every statement of the old rule is rewritten** (D8, D9): the `LifecycleComponent` doc comment, the
  `lifecycle-suite` spec (D10), the developer, reviewer and architect contracts, the `AGENTS.md` row and its
  rules-index row, `docs/setup-plan.md`, the ledger's `lifecycletest` row, `pkg/errs/shutdown.go`, graph-ingest's
  comments, and a dated note on ADR-108.
- **Not changed:** `internal/lifecycleguard/guard.go`, natsclient, exported signatures.

## Capabilities

- `lifecycle-suite`: modified requirements "Portable floor" and "Complete sensitivity matrix".

## Impact

- `internal/harness/lifecycletest`, `metric`, `processor/graph-ingest` (and its lifecycle adapter test), `component`
  (comment), `pkg/errs` (comment), docs and role contracts. About 120 non-test lines and 150 test lines.
- Three defects found and filed, not fixed here (inventory, "Issues to file"): natsclient's `SubscribeForRequests`
  parents handlers on its setup context; graph-ingest's test owner encodes the retired rule and cites a spec that
  does not exist; the `metric` README examples teach the retired shape.
