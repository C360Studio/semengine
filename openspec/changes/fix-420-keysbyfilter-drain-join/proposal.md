# Proposal: fix-420-keysbyfilter-drain-join

## Why

Issue #420. `KeysByFilter` and `FilteredKeys` share one listing, `keysByFilter` (`natsclient/kv.go`). After reading
the watcher's snapshot it calls `watcher.Stop()`, then reads `Updates()` until the channel closes, but gives up after
`filteredWatcherDrainTimeout`, a fixed 5 s (`natsclient/kv.go:605`, used at `:700`). When the 5 s run out it returns
`errFilteredWatcherDeliveryIncomplete` while nats.go's delivery goroutine may still be running. A fixed budget in place
of waiting for the work to end is what `openspec/specs/background-work/spec.md` rules out ("No fixed shutdown
timeout"). The follower that copied this cleanup (`graph/follower/follower.go`, `stopWatch`) was changed on PR #414 to
read until the channel closes, with no cutoff; this change gives `natsclient` the same shape.

Nobody has seen the 5 s run out. By #185 that makes the defect MEDIUM at most. It is still a real break of a stated
rule, not a guard against an imagined fault.

**Why the wait always ends (pinned nats.go v1.54.0, source read).** The watcher's update channel is closed only by the
subscription's closed handler (`jetstream/kv.go:1364-1368`, `sub.SetClosedHandler(... close(w.updates))`). The
subscription's delivery goroutine (`nats.go:3744`, `waitForMsgs`) runs that handler once the subscription is marked
closed, which `Stop` (`jetstream/kv.go:1235`, `w.sub.Unsubscribe()`) does through `removeSub`, and which closing the
connection does for every subscription. If `Stop` returns `ErrConnectionClosed` or `ErrBadSubscription`, the
subscription was already closed. The goroutine can be held only while the update callback is blocked sending on the
channel (`jetstream/kv.go:1319`, `:1328`), and the drain loop is what empties the channel. So a drain until close ends
once nats.go's delivery has ended, and not before. `TestIntegration_KVStoreFilteredNativeDeliveryClosure` exercises
this against a real server with a full 256-entry buffer.

Readers: `KeysByFilter` has callers across the tree, `FilteredKeys` too; none reads
`errFilteredWatcherDeliveryIncomplete`, which is unexported. The only references to the constant and the error are
`natsclient/kv.go` and the unit tests in `natsclient/kv_watcher_ownership_test.go`, plus one line of
`docs/admission-ledger.yaml` that describes `kv.go:700` as a bounded drain.

No open PR touches `natsclient/kv.go` or its tests (PR #437 touches `natsclient/client.go` and
`natsclient/owned_delivery.go`).

Light process, owner 2026-10-28: proposal, tasks and spec delta; no design document; Codex reviews the code.

## What Changes

- `keysByFilter` drains `Updates()` until it closes after `Stop`, with no timeout, as `graph/follower`'s `stopWatch`
  does. `filteredWatcherDrainTimeout` and `errFilteredWatcherDeliveryIncomplete` are removed. A `Stop` error that
  says only that the subscription was already gone (`nats.ErrBadSubscription`, `nats.ErrConnectionClosed`) stays
  tolerated, as today once the channel has closed.
- `TestKVStoreFilteredTerminalWindowUsesVirtualClock` is deleted: each of its four cases scripts a watcher whose
  channel never closes after `Stop`, to prove the 5 s bound this change removes. nats.go cannot produce that watcher
  (see above).
- A new unit test, under `testing/synctest`, scripts a watcher whose channel closes only when the test says, past 5 s
  of the bubble's clock: the listing has not returned before the close and returns after it. It fails on the base
  commit, where the listing returns `errFilteredWatcherDeliveryIncomplete` at 5 s.
- The `// spec: nats-kv-keys / ...` citations in `natsclient/kv_watcher_ownership_test.go` and
  `kv_watcher_ownership_integration_test.go` name a spec that does not exist; the tests they sit on prove that the
  listing stops its watcher and waits for its delivery to close, so they cite the modified requirement instead.
- `docs/admission-ledger.yaml` drops the sentence describing `kv.go:700` as a bounded terminal drain.

## Capabilities

### Modified Capabilities

- `transport-client`: "A filtered listing stops its watcher before returning" waits until the update channel closes,
  with no fixed timeout.
