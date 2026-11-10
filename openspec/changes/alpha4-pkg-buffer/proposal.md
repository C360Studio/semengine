# Proposal: alpha4-pkg-buffer

Approved-by:

Track: `quick` (no spec Requirement and no capability under `openspec/specs/` changes). `design.md` is carried
because a porting PR has a one-page design with its shape sweep, probe and ADR listing (#159).

## Why

SemSource publishes every graph entity through `pkg/buffer` (Block policy with a timeout), and the `output/websocket`
port (#34) uses it for its pending-ack ring. Nothing on `main` offers a bounded queue, so SemSource cannot build on
SemEngine and #34 cannot start. Issue #566.

## What Changes

- New `pkg/buffer`, ported whole from the pin: `Buffer[T]`, `NewCircularBuffer`, the three overflow policies
  (`DropOldest`, `DropNewest`, `Block`), `WithOverflowPolicy`, `WithMetrics`, `WithDropCallback`, `Statistics` and
  `StatsSummary`, `WriteWithTimeout` and `WriteWithContext` on the returned buffer.
- Dropped: `WithStats`, `StatsFromContext`, `ContextKeyStats` (state in context values, #91).
- Metrics: on the injected registry through `metric.RegisterOrGet`, named `semengine_buffer_*`.
- Corrections that binding rules require:
  - `WriteWithTimeout` keeps its signature and result, and invents no context root.
  - `WriteWithContext` leaves no goroutine running when it returns, and refuses a nil context.
  - Drop callbacks run after the buffer's lock is released, as the pin's comments already say.
  - The tests lose `time.Sleep` and wall-clock bounds.
- Declared, unchanged: capacity below 1 becomes 1; `WithMetrics` with a nil registry or empty prefix does nothing.

## Rulings

Owner, 2026-11-10 (#566 comment 6772860154): carry `pkg/buffer` as an Adapt in `v0.1.0-alpha.4`, option 1: own
small PR, own OpenSpec change, port whole, drop the context-value statistics, keep the three overflow policies; ledger
row and `MIGRATION.md` land in this PR; merges before or alongside the websocket port; SemSource keeps `Block` with a
timeout and changes only its import path.

## Owner questions

- Q1. Add `WriteWithTimeout` and `WriteWithContext` to `Buffer[T]`, so SemSource needs no type assertion and cannot
  fall back silently to an unbounded `Write`? No consumer breaks. Assumed here: no, as at the pin.
- Q2. `Statistics.MemoryUsage` always reads 0 (nothing sets it); `internal/cache` dropped the same methods. Assumed
  here: keep and say so in the doc comment.

## Capabilities

None. The contract line is the package `README.md` and the ledger row's `contract` field.

## Impact

- Pin lines: 1,235 production, 1,224 test. One PR.
- Consumer migration (`pkg/buffer/MIGRATION.md`): SemSource changes the import path. Metric names change from
  `semstreams_buffer_*` to `semengine_buffer_*`.
- Merge order: this PR before the `output/websocket` port (#34). No overlap with #588.
- Closes: #566 (task 3.1).
