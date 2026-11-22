# Proposal: fix-564-embedding-clean-stop-warning

Approved-by: cglusky (2026-11-22) <https://github.com/C360Studio/semengine/issues/564#issuecomment-7079557621> sha256:df5dcefc1719

Track: `quick`. No capability changes: the `keyed-dispatch` spec says nothing about the warning, and no requirement is
added or changed. One package (`internal/dispatch`).

## Why

graph-embedding logs `dispatch: Shutdown's context ended before accepted work drained` on every clean stop (#564).
The embedding worker begins the pool's drain with `pool.Shutdown(ctx)` after its run context has ended, then joins
with `pool.Wait()` (`graph/embedding/worker.go:306`, `:319`). That is the pool's documented way to drain when the
caller has no live context left (`KeyedPool.Wait`'s comment: "the join for a caller whose Shutdown(ctx) returned
ctx.Err() because ctx had already ended"). No accepted work is lost, but the pool warns each time
(`internal/dispatch/keyed_pool.go:305`), so an operator sees a drain failure that did not happen, and a real one reads
the same.

## What Changes

- `Shutdown` warns only when its context ends while it is waiting for the drain. Called with a context that has
  already ended, it begins the drain and returns `ctx.Err()` without a warning: the caller is starting the drain and
  joining with `Wait`, as the worker does. Return values and the drain itself are unchanged, so every `keyed-dispatch`
  requirement holds as written.
- The comment on `Shutdown` says when it warns.
- A test in `internal/dispatch`: `Shutdown` under an already-ended context, then `Wait`, logs no warning; a context
  that ends while `Shutdown` waits on a busy lane still logs it. The second half shows the test can fail.

Considered and rejected: changing the worker to call `Shutdown` with a live context. The worker's only context is its
run context, which has ended by then, and the developer contract forbids a `context.WithoutCancel` descendant outside
the one run context.

## Non-goals

- The roughly 5 s slow stop seen on 2026-11-16 when the embedding provider is gone (#564 comment). It is a separate
  cause (an in-flight provider call), not the log line; filed separately if it reproduces.
- graph-index, graph-ingest and graph-embedding's coalescer call `Shutdown` with a live stop context; they are
  unchanged.

## Impact

`internal/dispatch/keyed_pool.go` and its test. One PR, `Closes #564`.
