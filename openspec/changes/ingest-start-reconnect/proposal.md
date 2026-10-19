# Proposal: ingest-start-reconnect

## Why

Issue #291 (found by the inventory for wave PR 6c, #290; owner's plan note of 2026-10-18: PR 3 of the lifecycle
contract order recorded on #282). graph-ingest's `Start` calls `Connect` whenever the client's status is not
`Connected`. While nats.go is reconnecting, a connection is installed but the status reads `Reconnecting`, so
`Connect` refuses with "already connected" and `Start` fails. The real-world trigger is a component restart while the
broker is restarting or the link is down long enough for nats.go to be reconnecting.

This is PR 3 of the order on #282, after PR 2 (#310, ADR-113, merged `63328c4`), which put the connect block under the
Start context. It retires #291.

## What Changes

Claim skeleton. The design fills this section once the inventory passes review. Expected, from #291 and its plan
note: `Start` calls `Connect` only when no connection is installed, and otherwise waits for the connection under the
Start context.

## Capabilities

- None expected to change; the design says if a `graph-ingest-recovery` delta is needed.

## Impact

- `processor/graph-ingest` (the connect block of `start`) and its tests. No exported signature changes are expected.
