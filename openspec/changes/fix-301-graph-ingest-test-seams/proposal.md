# Proposal: fix-301-graph-ingest-test-seams

## Why

graph-ingest's production `Component` has four fields that exist only so tests can fake NATS: `waitForStreamInput`,
`consumeStream`, `watchEntityStates` and `subscribeRequests` (`processor/graph-ingest/component.go:67-84`). Production
never sets them. The repository rule is that production code carries no back door (#186, #190).

The owner rescoped #301 on 2026-10-27 (#301 comment 6453844541), asking: "is it an extreme edge and back to
outsmarting nats?" The ruling has four parts:

- Remove the fields.
- Move the consume callback's per-message body into a plain method.
- Keep the tests that prove real behaviour, through that method or one pause on graph-ingest's own log line.
- Delete the tests that reach edges where production only returns an error, or Naks and NATS redelivers.

Ruling 1 of comment 6453410228 stands: the straddle pair becomes a unit test.

## What Changes

- **A plain method.** The consume callback's per-message body becomes `inputLane.deliver`. Setup and its checks stay
  in `setupJetStreamConsumer`.
- **Kept tests.** They reach their case through `deliver`, or through a test logger that pauses on a graph-ingest log
  line.
- **Deleted tests.** Seven tests are deleted (design D3). The production branches they reached stay.
- **Fields removed.** The four fields go.
- **Spec.** `graph-transport-boundary`'s two "during a failed Start" scenarios are reworded to the case the
    kept test proves: the request arrives after registration, and Start then fails.
- **No other change.** No natsclient change (#170, ADR-112), no new exported surface, and no lifecycle-suite change.

## Capabilities

### Modified Capabilities

- `graph-transport-boundary`: modified "graph-ingest does no request work before Start completes".

## Impact

- Code: `processor/graph-ingest/component.go`, `consume.go`, `poison_inventory.go`.
- Tests: graph-ingest's lane, config, lifecycle, readiness, long-apply, settlement and failed-start test files.
- Retires #301 (`Closes #301`).
