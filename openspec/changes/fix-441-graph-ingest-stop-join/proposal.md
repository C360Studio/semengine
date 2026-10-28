# Proposal: fix-441-graph-ingest-stop-join

## Why

Issue #441. graph-ingest's `cleanup` (`processor/graph-ingest/component.go:551-595`) drains each JetStream input and
then waits on the consume loop's native `Closed()` (`component.go:560-566`) as if it proved every consume callback had
returned. It does not: once the connection is lost for good, `Closed()` can fire while a callback is still running
(`natsclient/owned_delivery.go:217-225`, citing `jetstream/pull.go:822-837` at nats.go v1.54.0; #83). `cleanup` then
shuts the ingest pool down and `Stop` can return nil with a callback still running.

The callback is `inputLane.deliver` (`processor/graph-ingest/consume.go:236-`): it decodes the message, then submits it
to the ingest pool or settles it (Ack, Term, Nak) itself. Real-world trigger: a broker outage that ends the client's
reconnects while a delivery is being decoded or submitted, the trigger of #439. The impact is smaller than in
objectstore: the pool is already shut down, so a late submit is refused and the message Nak'd; what is broken is the
promise that a nil `Stop` means nothing of the component still runs.

The fix is the owner's #443 ruling, recorded for this issue on #441 (PR #437 comment 6467701324): one owner for
handler completion, `natsclient.Client.WaitConsumerHandlers` (`natsclient/owned_delivery.go:217-261`). It returns nil
once delivery on the consumer has ended and every callback has returned, and an error matching `ctx.Err()` if `ctx`
ends first. `storage/objectstore`'s `cleanup` already uses it (`storage/objectstore/component.go:506-535`, PR #437).

`release` (`component.go:540-547`) already keeps every handle when `cleanup` fails, so a `Stop` whose context ends
first keeps the inputs for a later `Stop`, as the ruling asks.

No open PR touches `processor/graph-ingest` or `openspec/specs/graph-ingest-recovery` (PR #458 checked on 2026-10-28).

Light process, as on #454 and #456: proposal, tasks and spec delta; no design document; Codex reviews the code.

## What Changes

- `cleanup` waits for each consumer through `c.natsClient.WaitConsumerHandlers(ctx, handle)` in place of the
  `Closed()` select, at the same point: after every Drain is issued and before the pool shuts down. Its doc comment
  says why `Closed()` is not the wait.
- An integration test, `TestStopJoinsCallbackAfterConnectionLoss` in a new
  `processor/graph-ingest/stop_join_integration_test.go`, mirrors objectstore's `TestStopJoinsHandlerAfterConnectionLoss`
  on its own server, with a client that does not reconnect. It holds a delivery callback by pausing graph-ingest's
  logger on the poison-message warning `deliver` logs (the `pauseOnRecord` helper), takes the broker away and waits
  for the client's connection to close and the consume loop's `Closed()` to fire. A `Stop` under a one-second context
  must then fail with that context's error and keep the inputs; once released, a later `Stop` returns nil and holds
  nothing. On the base commit the first `Stop` returns without waiting for the held callback.

## Not covered

- graph-ingest's core-NATS query subscriptions: their `Drain(ctx)` already waits for their handlers.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `graph-ingest-recovery`: adds "Stop returns only after every input callback has returned".
