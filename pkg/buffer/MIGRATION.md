# Consumer migration: `pkg/buffer`

This is for SemSource's entity publisher (`internal/entitypub/publisher.go`) and for anyone else moving from
SemStreams' `pkg/buffer` to SemEngine's. It lists what you change at your cutover and what you can leave alone. It was
not run against your code. It comes from reading the SemEngine package and SemStreams.

## Changes that stop your build (loud)

- Import path. `github.com/c360studio/semstreams/pkg/buffer` becomes `github.com/c360studio/semengine/pkg/buffer`.
- `WithStats`, `StatsFromContext` and `ContextKeyStats` are gone. SemEngine does not carry state in context values.
  Read statistics from `buf.Stats()`. SemSource calls none of these.
- `Statistics.MemoryUsage`, `UpdateMemoryUsage` and `StatsSummary.MemoryUsage` are gone. Nothing ever set them, so
  they always read 0.
- `WithMetrics` takes SemEngine's `*metric.MetricsRegistry` (`github.com/c360studio/semengine/metric`), not the
  SemStreams one.
- A type that implements `Buffer[T]` without embedding it must add `WriteWithTimeout(item T, timeout time.Duration)
  error` and `WriteWithContext(ctx context.Context, item T) error`. `Buffer[T]` now declares both.

## Changes that do not stop your build (silent)

- Metric names: `semstreams_buffer_*` becomes `semengine_buffer_*`: `writes_total`, `reads_total`, `peeks_total`,
  `overflows_total`, `drops_total`, `size`, `utilization`, each labelled `component=<prefix>`. Do: update dashboards
  and alerts.
- Two buffers with one metrics prefix share one series. In SemStreams the second buffer's counters were never gathered.
  Do: give each buffer its own prefix.
- `WriteWithContext` refuses a nil context with an `ErrorInvalid` error under every policy. In SemStreams a nil context
  panicked under `Block` and was ignored under the other policies.
- Drop callbacks run after the buffer's lock is released. In SemStreams they ran under it, so a callback that touched the
  buffer deadlocked. SemSource sets no drop callback.
- `NewCircularBuffer` returns a true nil `Buffer[T]` with its error. In SemStreams it returned a non-nil interface
  holding a nil pointer, so a `buf != nil` check passed on a failed construction. Check the error, as before.
- `semengine_buffer_reads_total` counts the items `ReadBatch` removes. In SemStreams it counted only `Read`, so a
  publisher that drains with `ReadBatch`, as SemSource's does, reported zero reads. `Stats().Reads()` counted both
  then and now. Do: expect the reads series to rise with batch drains.
- `StatsSummary` has no `memory_usage` field in its JSON.
- A wrapper that embeds `Buffer[T]` (SemSource's `notifyingBuffer` in `stop_settlement_test.go`) now keeps the
  bounded writes, because they are part of the interface. In SemStreams the embedded interface hid them.

## What you can leave alone

- `NewCircularBuffer`, `WithOverflowPolicy(buffer.Block)`, `WithMetrics(registry, prefix)`; `Write`, `ReadBatch`,
  `Size`, `Close`; `Stats() *buffer.Statistics`.
- `WriteWithTimeout(item, timeout)`: same signature, and it still returns `context.DeadlineExceeded` when the buffer
  stays full past the timeout, with nothing written.
- Error classes: a write after `Close` is `ErrorInvalid` wrapping `errs.ErrAlreadyStopped`.
- SemSource's `timeoutWriter` assertion still compiles and still matches, but you can drop it and call
  `WriteWithTimeout` on the `Buffer[T]` directly.

## Worth doing

- `WriteWithTimeout(item, 0)` and `WriteWithContext` with an ended context refuse under `Block` even when there is
  space, as in SemStreams. Keep the send timeout positive.
