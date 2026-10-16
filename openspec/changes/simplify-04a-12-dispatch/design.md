# Design: simplify-04a-12-dispatch

This design depends on `INVENTORY PASS` for `inventory.md`. Premises cite its sections; line numbers are at the base
commit.

## D1. One stop path (#131, HIGH)

`NewKeyedPool(ctx, cfg)` passes ctx to `Process` and nowhere else: no lane, and no part of `SubmitBlocking`, reads
it (today `keyed_pool.go:250`, `:358`). `Shutdown(ctx)` is the only stop. Observable:

- `SubmitBlocking` returns nil only for an item that `Process` is called with exactly once before `Shutdown` returns
  nil. From the first `Shutdown`, it returns `ErrStopped` (unchanged).
- After the constructor's context ends, lanes keep running: queued and later-accepted items reach `Process` with the
  ended context. graph-ingest then naks each at once (inventory § 4: `ValidateHeartbeatDeliveryPolicy` refuses an
  ended context, `keyed_ingest.go:218` naks), where today they wait for AckWait.
- `Shutdown` returns nil when every lane has exited, even if its ctx has also ended; `ctx.Err()` when ctx ends first
  (shape sweep item 6, mirroring `coalescing_set.go:203-208`). Nil ctx still refused.

Options:

- (a) Lanes read their queue until `Shutdown` closes it; context to `Process` only. Recommended: #131's proposal, the
  `background-work` `Shutdown(ctx)` shape exactly, and it deletes `drainCh`, the nested drain loop and the context
  arm. Cost: a pool nobody shuts down keeps its lanes until the process ends (adopter seams 2); change 2 task 3.6's
  "cancelling the run context leaves nothing running" is dropped (owner question).
- (b) (a) plus `context.AfterFunc(ctx, …)` starting the same drain `Shutdown` starts (pattern
  `natsclient/socket.go:54`). Keeps that sentence; costs a second trigger for one stop, which #131 asked to remove.
- (c) `SubmitBlocking` also checks the run context. Not a fix: a lane can still exit after the send wins, and queued
  items are still dropped.
- (d) Do nothing. ADR-072 M3's "no accepted (nil-return) submit can be stranded" (inventory § 6) stays broken.

ADR-072 kept: M3's stop order is graph-ingest's (`component.go:1136-1144`, unchanged) and its atomic handoff now holds
for both ways a context can end; H2 and B1 unchanged.

Tests (`shutdown_test.go`, each in `synctest.Test`):

- `TestSubmitAfterRunCancelIsProcessed`: #131's reproduction, asserting `Process` saw the item with an ended context.
- `TestRunCancelProcessesQueuedItems`: one item blocked in `Process`, N queued; cancel; release; all N reach `Process`
  with the ended context, in order.
- `TestKeyedPoolRunContextCancelLeavesNothingRunning` becomes `…CancelThenShutdownLeavesNothingRunning` (+`Shutdown`).
- `TestShutdownAfterJoinReturnsNilUnderEndedContext`: 64 calls after the join, each nil (base passes all 64 with odds
  2^-64).
- `TestKeyedPool_ShutdownSubmitRace` and the three other shutdown tests pass unedited in substance.

## D2. The key is an argument (#136 item 3)

`SubmitBlocking(ctx context.Context, key string, work W) error`; `KeyOf` goes, and with it the "missing KeyOf"
refusal. graph-ingest passes `work.entityID` at `component.go:1610`. Observable: unchanged routing for one key
(`TestKeyedPool_SameKeyOrdered`, `…SameKeySameLane`, `…DifferentKeysConcurrent` adapt; they fail to compile on base).

## D3. One config struct (#136 item 4)

`KeyedConfig` gains `Logger *slog.Logger` and `MetricsRegistry *metric.MetricsRegistry` (both optional, as today);
`NewKeyedPool(ctx, cfg)`. Observable: unchanged.

## D4. `Process` returns nothing (#136 item 2)

`Process func(ctx context.Context, lane int, work W)`. The pool's debug log (`keyed_pool.go:298-303`) moves to its
one owner: `buildIngestPool` passes a function that calls `processIngest` and logs a returned error at Debug with the
entity ID. `processIngest` keeps its signature, so its 25 test readers (inventory § 1) are untouched. Option: drop
`processIngest`'s error and rewrite those tests to read settlements; larger, and outside `internal/dispatch`.

## D5. Queue depth without a sampler (#136 item 1)

`metricsUpdater` goes; the pool starts no goroutine besides its lanes. Observable: `dispatch_queue_depth` equals the
items accepted and not yet passed to `Process`, summed over pools of one `Name` on a registry. Rejected:
`prometheus.NewGaugeFunc` as #136 proposed. `RegisterOrGet` returns the first collector for a key
(`metric/registry.go:39-50`), so a second pool of that name, such as a new graph-ingest after a failed `Start`, would
report the first pool's queues for the registry's lifetime. Test: `TestKeyedPoolQueueDepthCountsWaitingItems`
(bubble; one item in `Process`, three queued, no clock advance: 3; after `Shutdown`: 0). It fails on base, which
reads 0 until a tick.

## D6. Tests on the bubble clock (#136 item a)

Every `keyed_pool_test.go` test that waits runs in `synctest.Test`. The `time.After` watchdogs (`:82`, `:89`, `:201`)
and the 5 s `stopCtx` (`:22`) go; backpressure (`:237`) is shown by `synctest.Wait()` with the third submit still
blocked, then its context cancelled. `ProcessUsesConstructorContext` drops its `context.WithValue` (`:58`); the
identity check proves it. Sensitivity: `task mutate:check` on a mutant that makes `SubmitBlocking` drop instead of
wait must be detected by `TestKeyedPool_Backpressure`.

## D7. Text follows the code

`doc.go` (one stop, the key argument, no `KeyedDeps`), `errors.go:7-11`, the `processIngest` comment
(`keyed_ingest.go:212-215`: the context also ends when `Start`'s does), the spec delta `keyed-dispatch`, and the ledger
row (`docs/admission-ledger.yaml:2025-2032`: the D1–D5 contract; "one goroutine per lane, joined by Shutdown").

## Open issues and merge order

Issues #131 and #136 are retired (tasks 3.1–3.3). #107, #135 and #195 stay (inventory § 3). #273 edits
`component.go:1547`; this change edits `:1610` in the same function. Either order; the second to land merges
`origin/main`.

## Owner questions

1. Ending the pool's context no longer stops the pool; only `Shutdown` does (D1 option a), dropping change 2 task
   3.6's "cancelling the run context leaves nothing running". Recommended: accept. It is #131's own proposal, and
   graph-ingest already calls `Shutdown` on every stop and failed start.
