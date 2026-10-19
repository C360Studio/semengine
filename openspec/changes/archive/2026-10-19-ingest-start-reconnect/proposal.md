# Proposal: ingest-start-reconnect

## Why

Issue #291 (found by the inventory for wave PR 6c, #290; owner's plan note of 2026-10-18: PR 3 of the lifecycle
contract order recorded on #282). graph-ingest's `Start` calls `Connect` whenever the client's status is not
`Connected`. While nats.go is reconnecting, a connection is installed but the status reads `Reconnecting`, so
`Connect` refuses with "already connected" and `Start` fails. The real-world trigger is a fresh component instance
started on a shared client that is reconnecting: a broker restart, a dropped link, or a failed health RTT (graph-ingest
is one-shot, so the "Stop then Start" of #291's body cannot be the same instance; inventory, "The trigger, corrected").

This is PR 3 of the order on #282, after PR 2 (#310, ADR-113, merged `63328c4`), which put the connect block under the
Start context. It retires #291.

## What Changes

- `Start` tolerates `Connect`'s "already connected" refusal (`errs.ErrAlreadyStarted`): a client whose connection is
  installed but reconnecting is waited for under the Start context by the existing `WaitForConnection` call. A Start
  context that ends first fails `Start` with that error and holds nothing (design D1).
- Two integration tests through a test-local relay to the shared broker: `Start` waits for a reconnecting client and
  returns nil once the broker is reachable again; `Start` returns its context's error when it ends during the wait
  (D2).
- `Start`'s doc comment names the wait.

## Capabilities

- None. No spec states `Start`'s connect behaviour (design D6); `skip_specs: true` stays.

## Impact

- `processor/graph-ingest/component.go` (about two lines and a comment) and `lifecycle_integration_test.go`. No
  exported change; natsclient unchanged.
