# Proposal: fix-454-follower-watch-bound

## Why

Issue #454. `stopWatch` (`graph/follower/follower.go:245-250`) stops the `ENTITY_STATES` watch, then reads `Updates()`
until the channel closes, with no bound. PR #414 removed a 5 s bound copied from `natsclient`, after review read it as
a fixed timeout standing in for a join.

At the pinned nats.go v1.54.0 the channel can stay open for good (source read, confirmed on #420 by the inventory
review of withdrawn PR #453):

- `WatchAll` subscribes first and only then installs the handler that closes the update channel
  (`jetstream/kv.go:1356-1368`, `sub.SetClosedHandler(... close(w.updates))`).
- `Subscribe` has already started a goroutine that runs `<-ctx.Done(); sub.Unsubscribe()` on the watch's context
  (`js.go:2063-2068`).
- The subscription's delivery goroutine reads the closed handler once, when it exits (`nats.go:3830-3834`);
  `SetClosedHandler` only stores it (`nats.go:5493-5497`).

So if `Run`'s context ends, or the connection closes, between the subscribe and the handler install, the delivery
goroutine exits with no handler and `Updates()` never closes. `Run` then blocks in `stopWatch` for good. Real-world
trigger: graph-index's `Stop`, or process shutdown, cancelling `Run` (`processor/graph-index/component.go:272`) while
the follower is setting up its watch (`follower.go:202`). The window is microseconds wide and has not been reproduced;
by #185 that makes this MEDIUM at most. Its effect is a component `Stop` that never returns.

In that case nats.go's delivery goroutine has already exited, so giving up the wait leaves nothing running. When the
handler is installed, the wait ends as soon as nats.go's delivery has ended; the bound is never reached on that path.

`natsclient`'s `keysByFilter` keeps a 5 s bound for this same case (`natsclient/kv.go:603-605`), and the owner chose
on 2026-10-28 to keep it there and close #420. The follower gets the same bound, named for this case.

The bound is the shape the developer and reviewer contracts admit for terminal cleanup after an owner's
cancellation (`.agents/contracts/semengine-reviewer.md:251-255`): `context.WithTimeout(context.WithoutCancel(ctx),
budget)`, as `natsclient/kv.go:700` does and `docs/admission-ledger.yaml:1155-1156` records. It does not stand in for a
join (`openspec/specs/background-work/spec.md`, "No fixed shutdown timeout"): on the path where nats.go installed its
handler, the wait ends when nats.go's delivery ends, and on the path where it did not, no goroutine is left to join.

Callers: `processor/graph-index/component.go:272` is the only caller of `Run`; it logs a non-nil error as "ENTITY_STATES
watch did not stop cleanly" and goes on, so an expired bound is reported and `Stop` returns.

No open PR touches `graph/follower` or `openspec/specs/entity-state-follower` (PR #437 and PR #452 checked).

Light process, owner 2026-10-28: proposal, tasks and spec delta; no design document; Codex reviews the code.

## What Changes

- `stopWatch` takes `Run`'s context and drains `Updates()` under `context.WithTimeout(context.WithoutCancel(ctx),
  budget)`, with a named 5 s budget. When the budget runs out it returns an error saying the watch's delivery did not
  close after `Stop`. A `Stop` error is still tolerated, as today, when it says only that the watch was already gone.
- The doc comments of `stopWatch` (`follower.go:240-244`, which calls returning "a join") and `Run`
  (`follower.go:183-186`, "its delivery closed") name the bound and the nats.go window.
- A unit test, `TestStopWatchGivesUpOnADeliveryThatNeverCloses` in `graph/follower/follower_test.go`, runs
  `stopWatch` under `testing/synctest`, with a cancelled context, on a scripted watcher whose channel never closes: it
  returns the error after exactly the named budget of the bubble's clock (`time.Since` on the bubble's clock, no
  `time.Sleep`). It cannot pass on the base commit, where `stopWatch` never
  returns (synctest reports the deadlock).
- `TestFollowerRunReturnsAndLeavesNothing` is unchanged; it still proves the backlogged-delivery join on a real server.

## Not covered

- graph-ingest's guard watcher (`processor/graph-ingest/poison_inventory.go:352-362`) drains with no bound in the same
  window; filed as #456 (one package per PR).
- A connection close in the window while `Run`'s context is live leaves `follow` waiting with the feed never declared
  lost; filed as #457. This change only bounds the wait after the context ends.

## Capabilities

### Modified Capabilities

- `entity-state-follower`: "Run returns only after the watch's delivery closes" waits at most 5 s and names why.
