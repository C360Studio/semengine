# Design: simplify-04a-12-dispatch

This design depends on `INVENTORY PASS` for `inventory.md`. Premises cite its sections; line numbers are at the base
commit.

## D1. One stop path (#131, HIGH)

`NewKeyedPool(ctx, cfg)` passes ctx to `Process` and nowhere else: no lane, and no part of `SubmitBlocking`, reads
it (today `keyed_pool.go:250`, `:358`). `Shutdown(ctx)` is the only stop. Observable:

- `SubmitBlocking` returns nil only for an item that `Process` is called with exactly once before `Shutdown` returns
  nil. A call that begins after the first `Shutdown` returns `ErrStopped`; one already waiting on a full lane may
  still be accepted (unchanged, `TestKeyedPool_ShutdownSubmitRace`, ADR-072 M3).
- After the constructor's context ends, lanes keep running: queued and later-accepted items reach `Process` with the
  ended context. graph-ingest then naks each at once (inventory § 4: `ValidateHeartbeatDeliveryPolicy` refuses an
  ended context, `keyed_ingest.go:218` naks), where today they wait for AckWait.
- `Shutdown` returns nil when every lane has exited, even if its ctx has also ended; `ctx.Err()` when ctx ends first
  (shape sweep item 6, mirroring `coalescing_set.go:203-208`). Nil ctx still refused.

Options:

- (a) Lanes read their queue until `Shutdown` closes it; context to `Process` only. Chosen (owner, 2026-10-16):
  #131's proposal, the `background-work` `Shutdown(ctx)` shape exactly, and it deletes `drainCh`, the nested drain
  loop and the context arm. Cost: a pool nobody shuts down keeps its lanes until the process ends (adopter seams 2);
  change 2 task 3.6's "cancelling the run context leaves nothing running" is dropped.
- (b) (a) plus `context.AfterFunc(ctx, …)` starting the same drain `Shutdown` starts (pattern
  `natsclient/socket.go:54`). Keeps that sentence; costs a second trigger for one stop, which #131 asked to remove.
- (c) `SubmitBlocking` also checks the run context. Not a fix: a lane can still exit after the send wins, and queued
  items are still dropped.
- (d) Do nothing. ADR-072 M3's "no accepted (nil-return) submit can be stranded" (inventory § 6) stays broken.

Why the pool keeps a constructor context. The pool needs a context to hand `Process`, and the developer contract
forbids inventing one (`.agents/contracts/semengine-developer.md:219-221`: no `context.Background` or
`context.WithoutCancel` outside the composition boundary), so it takes its owner's. The context carries no stop
meaning: it is the work's cancellation, which the owner ends. graph-ingest already ends it straight after `Shutdown`
(`component.go:1139-1144`), and `processIngest` was written for that order: "the policy is refused once the lane
context has ended: at Stop, when the pool's Shutdown runs out of time while lanes drain … the input is negatively
acknowledged" (`keyed_ingest.go:212-215`). So when `Shutdown`'s bound runs out, the owner's cancel makes the lanes
finish the queue fast, each item naked, as with `net/http.Server`, whose `Shutdown` returns the error and leaves the
abort to the caller. The pool adds no abort of its own. Whether a component's `Start` context should own its
running work at all (the same two-stop shape one layer up) is #282, for the owner, after PR 7.

ADR-072 kept: M3's stop order is graph-ingest's (`component.go:1136-1144`, unchanged) and its atomic handoff now holds
for both ways a context can end; H2 and B1 unchanged.

Tests (`shutdown_test.go`, each in `synctest.Test`):

- `TestSubmitAfterRunCancelIsProcessed`: #131's reproduction, asserting `Process` saw the item with an ended context.
- `TestRunCancelProcessesQueuedItems`: one item blocked in `Process`, 64 queued (`QueueDepth` 64); cancel; release; all
  64 reach `Process` with the ended context, in order (base processes all 64 with odds 2^-64).
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
reads 0 until a tick. Dropping the gauge is not an option: queue depth is part of ADR-072's decision (pin
`:182-188`).

## D6. Tests on the bubble clock (#136 item a)

Every `keyed_pool_test.go` test that waits runs in `synctest.Test`. The `time.After` watchdogs (`:82`, `:89`, `:201`)
and the 5 s `stopCtx` (`:22`) go; backpressure (`:237`) is shown by `synctest.Wait()` with the third submit still
blocked, then its context cancelled. `ProcessUsesConstructorContext` drops its `context.WithValue` (`:58`); the
identity check proves it. Sensitivity: `task mutate:check` on a mutant that makes `SubmitBlocking` drop instead of
wait must be detected by `TestKeyedPool_Backpressure`.

## D7. Text follows the code

`doc.go` (one stop, the key argument, no `KeyedDeps`), `errors.go:7-11`, the `KeyOf` comment at `component.go:1567`, the
`processIngest` comment (`keyed_ingest.go:212-215`: the context also ends when `Start`'s does), the spec delta
`keyed-dispatch`, and the ledger row (`docs/admission-ledger.yaml:2025-2032`: the D1–D5 contract; "one goroutine per
lane, joined by Shutdown").

## Open issues and merge order

Issues #131 and #136 are retired (tasks 3.1–3.3). #107, #135 and #195 stay (inventory § 3). #273 edits
`component.go:1547`; this change edits `:1610` in the same function. Either order; the second to land merges
`origin/main`.

## Owner rulings

1. Accepted 2026-10-16 (#131 comment): ending the pool's context no longer stops the pool; only `Shutdown` does (D1
   option a), dropping change 2 task 3.6's "cancelling the run context leaves nothing running". The pool keeps its
   constructor context, passed only to `Process`; no root context is invented. The component-level question is #282.
2. FYI, no ruling needed: D5 counts queue depth on submit and pickup instead of #136's suggested `GaugeFunc`, which
   would read the first pool's queues after a restart under the same name.
