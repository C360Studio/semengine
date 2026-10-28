# Proposal: fix-456-guard-watcher-drain

## Why

Issue #456. graph-ingest's boot sweep of `ENTITY_STATES` (`startEntityStateGuard`,
`processor/graph-ingest/poison_inventory.go:305-343`) runs from `Start` under `Start`'s context
(`processor/graph-ingest/component.go:457`). It opens a watch with `KVStore.Watch(ctx, ">")`, reads the snapshot to
its end-of-snapshot marker, and then calls `stopEntityStateGuardWatcher` (`poison_inventory.go:352-362`), which stops
the watch and reads the update channel until it closes, with no bound. It does the same when `Start`'s context ends
mid-sweep.

At the pinned nats.go v1.54.0 the channel can stay open for good. This is the window #454 bounded in `graph/follower`
(PR #455) and #420 described for `natsclient`:

- `WatchAll` subscribes first and only then installs the handler that closes the update channel
  (`jetstream/kv.go:1356-1368`).
- `Subscribe` has already started a goroutine that unsubscribes when the watch's context ends (`js.go:2063-2068`).
- The subscription's delivery goroutine reads the closed handler once, when it exits (`nats.go:3830-3834`);
  `SetClosedHandler` only stores it (`nats.go:5493-5497`).

So if the watch's context ends, or its connection closes, between the subscribe and the handler install, nothing ever
closes the channel. Real-world triggers: shutdown during boot cancelling `Start`'s context inside that window, after
which `Start` never returns (ADR-113 rules that out); or the connection closing in the window on an empty bucket, where
nats.go still sends the end-of-snapshot marker (`kv.go:1373`), so the sweep reaches the drain on its success path and
waits for good. The window is microseconds wide and has not been reproduced; by #185 that makes this MEDIUM at most.

When the handler was installed, the wait ends as soon as nats.go's delivery ends, so the bound is never reached on that
path. When it was not, the delivery goroutine has already exited, so giving up leaves nothing running.

Fix shape, as #455 did and as `natsclient`'s `keysByFilter` keeps (`natsclient/kv.go:603-605`, kept on #420 by the
owner's choice on 2026-10-28): drain under `context.WithTimeout(context.WithoutCancel(ctx), budget)` with a named 5 s
budget. That is the shape the developer and reviewer contracts admit for terminal cleanup after an owner's cancellation
(`.agents/contracts/semengine-reviewer.md:251-255`); it does not stand in for a join (`openspec/specs/background-work/
spec.md`, "No fixed shutdown timeout"), for the reason above.

When the bound runs out the sweep fails, classified transient, so `Start` fails rather than run on with a watch whose
delivery it could not see end. On the success path this means the poison found by the sweep is not recorded; `Start`
fails anyway, so nothing reads it.

No open PR touches `processor/graph-ingest` or `openspec/specs/graph-ingest-recovery` (PR #452 and PR #458 checked on
2026-10-28).

Light process, as on #454: proposal, tasks and spec delta; no design document; Codex reviews the code.

## What Changes

- `stopEntityStateGuardWatcher` takes the sweep's context and returns an error. It drains the update channel under
  `context.WithTimeout(context.WithoutCancel(ctx), budget)` with a named 5 s budget and, when the budget runs out,
  returns an error saying the snapshot watch's delivery did not close after `Stop`. Its doc comment names the bound and
  the nats.go window.
- `startEntityStateGuard` returns that error, classified transient: joined with the context's error on the cancelled
  path, and in place of recording the sweep's poison on the marker path.
- A unit test, `TestEntityStateGuardGivesUpOnADeliveryThatNeverCloses` in
  `processor/graph-ingest/query_contract_guard_test.go`, runs the sweep under `testing/synctest` on a scripted watcher
  whose channel never closes, in two cases: after the end-of-snapshot marker, and with `Start`'s context cancelled
  mid-sweep. In each the sweep returns a transient error naming the open delivery (and, when cancelled, wrapping
  `context.Canceled`) after exactly the named budget of the bubble's clock, records no poison, and has stopped the
  watch. It cannot pass on the base commit, where the sweep never returns (synctest reports the deadlock).
- `TestEntityStateGuardDeliberateStopDrainsToCloseWithoutMisclassification` is unchanged; it still proves the drain
  reads post-marker entries to the close.

## Not covered

- #457 (the follower's feed never declared lost in the same window while its context is live) stays open.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `graph-ingest-recovery`: adds "The boot sweep waits at most 5 s for its watch to close".
