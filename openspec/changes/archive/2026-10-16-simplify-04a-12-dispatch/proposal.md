# Proposal: simplify-04a-12-dispatch

## Why

PR 8b of epic #223 (owner, 2026-10-16, #223 comment 6216623583: "**8b — `internal/dispatch`**: #131 (work accepted
after cancellation never runs), #136.").

`KeyedPool` has two ways to stop. Cancelling the context given to `NewKeyedPool` makes the lanes exit at once, but
`SubmitBlocking` does not know about it: it keeps putting items into lane queues that nothing reads and returns nil,
so the caller believes the work was accepted (#131). Items already queued at that moment are dropped as well. For
graph-ingest, each such message is neither acknowledged nor rejected until the server redelivers it. Beside that, the
pool runs a ticker goroutine only to sample queue length, takes an error from `Process` that only reaches a debug
log, takes a key callback where an argument would do, and has a second options struct for its one constructor (#136).

## What Changes

- One stop path: `Shutdown(ctx)`. The constructor's context reaches only `Process`. Every item `SubmitBlocking`
  accepts is passed to `Process` exactly once; after the context has ended, `Process` gets the ended context, which
  graph-ingest already answers with a Nak.
- `Shutdown` returns nil whenever the lanes have exited, even if its own context has also ended.
- `SubmitBlocking(ctx, key, work)` takes the key; `KeyOf` goes.
- `KeyedDeps` folds into `KeyedConfig` (`Logger`, `MetricsRegistry`); `NewKeyedPool(ctx, cfg)`.
- `Process` returns nothing; graph-ingest logs its own error where it is made.
- `dispatch_queue_depth` counts waiting items exactly, with no sampler goroutine.
- The package tests run on the `synctest` clock.

## Capabilities

- Added: `keyed-dispatch` (same-key order, every accepted item processed and one stop path, the queue-depth gauge).

## Impact

- Files: `internal/dispatch` (all five files); `processor/graph-ingest/keyed_ingest.go` (`buildIngestPool`, the
  `processIngest` comment), `component.go:1610`, `lifecycle_owner_test.go`, `interrupted_work_test.go`;
  `docs/admission-ledger.yaml`.
- Exported surface: `KeyedConfig.KeyOf` and `KeyedDeps` removed; `KeyedConfig` gains `Logger` and
  `MetricsRegistry`; `SubmitBlocking` gains `key`; `Process` loses its error. The package is internal, so no consumer
  imports it.
- Behaviour change: ending the pool's context no longer stops it; the owner calls `Shutdown`, as graph-ingest
  already does (`component.go:1139-1141`).
- Closes #131, #136. Addresses #223.
