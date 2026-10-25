# Proposal: setup-04a-04-graph-roots-processor-shell

## Why

Change 4 (`setup-04a-04-graph-roots`) of epic #9 ports eight graph packages, one per pull request. The owner set the
order on 2026-10-24 (#9 comment 6386856657): the processor shell (#109) is PR 2, with graph-ingest moving onto it,
before graph-index. ADR-108 (accepted 2026-10-10) puts the shell in `component`, public, composing the lifecycle
state machine `internal/lifecycleguard`, so that starting once, stopping, cleaning up a failed start, and answering
`Health()` and `DataFlow()` are written and proven once instead of once per component. Today graph-ingest carries
that code inline (`processor/graph-ingest/component.go:841-950`), and graph-index, graph-query and objectstore each
bring their own copy from the pin.

## What Changes

- Add `component.Shell`: one exported type, zero value ready, held as a named field. It runs `Initialize`, `Start`
  and `Stop` through the guard and answers `Health` and `DataFlow` from its running state, its counters, and the
  stuck states the component passes in (ADR-108 D3, ADR-109 D2). It holds no context (ADR-113).
- `component.FlowMetrics` gains `Messages` and `Bytes`, the exact totals its rates are derived from; the status
  routes serve them, and graph-ingest's exact-count tests read them.
- Move graph-ingest onto it. Its `Initialize` stops re-validating configuration; `DataFlow`'s `LastActivity` is the
  zero time before the first input; everything else its tests assert stays the same.
- Fix #306: graph-ingest's entity cache handle stays valid for readers while `Stop` runs; `internal/cache` states,
  and tests, that a cache stays usable after `Close`.
- Update the `component`, `processor/graph-ingest` and `pkg/cache` ledger rows, the `LifecycleComponent` doc
  comment, `component/README.md`, `docs/repository-map.md`, a one-line note on ADR-108, and the four rule lines that
  send a component to the guard (`AGENTS.md`, `.agents/rules-index.md`, the developer and reviewer contracts).

## Capabilities

- New capability `component-shell`: the shell's lifecycle and its two reads.
- Modified capability `graph-ingest-recovery`: the entity cache stays usable through `Stop`.

## Impact

- `component` (about 190 new production lines, two new imports, no graph package, two new `FlowMetrics` fields),
  `processor/graph-ingest` (about 110 lines removed), test setup lines in 14 graph-ingest test files. No service file
  changes; the status routes gain `data_flow.messages` and `data_flow.bytes`. No new subject, bucket, config key or
  metric. Retires #306; addresses #109 and #196.
