# Proposal: start-context

## Why

ADR-113 (`docs/adr/113-start-context-bounds-start.md`, accepted; owner ruling #282 comment 6249934379, watcher
correction 6250135029) decides that the context given to a component's `Start` bounds only `Start`. Ending it after
`Start` returns stops nothing; `Stop(ctx)` is the only stop. Today the lifecycle suite, the `LifecycleComponent` doc
comment, `metric.Server` and graph-ingest's `start` all state or implement the older rule, under which the `Start`
context owned the component's running lifetime. This is PR 2 of the order recorded on #282 (point 5), after the
lifecycleguard tidy (#308, merged `1dc132b`). It retires #282.

## What Changes

Claim skeleton. The design fills this section once the inventory passes review. Expected, from ADR-113's
Consequences:

- Two of the eight lifecycle-suite checks change, and the sensitivity double gains one failpoint (an owner whose
  work dies with its `Start` context).
- `metric.Server` and graph-ingest's `start` create their one run context inside `Start` and end it inside `Stop`.
- Every place that states the old rule is rewritten: the `LifecycleComponent` doc comment, the `lifecycle-suite`
  spec, the role contracts, the rules index, and the docs ADR-113 lists.

## Capabilities

- `lifecycle-suite` (modified requirement: "Portable floor").

## Impact

- `internal/harness/lifecycletest`, `component`, `metric`, `processor/graph-ingest`, `internal/lifecycleguard`
  (test owner only), docs and role contracts. No exported signature changes are expected.
