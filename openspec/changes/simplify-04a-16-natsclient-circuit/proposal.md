# Proposal: simplify-04a-16-natsclient-circuit

## Why

PR 6c of epic #223 (#223 comment 6233147857): #222 part c. Part c names three defects; inventory § 1 measured each:

- The `AccountInfo` trip does not exist, and there are three `atomic.Value` fields, not six.
- `testCircuit` does leave a live connection marked `Disconnected`. JetStream calls then refuse with `ErrNotConnected`
  until the health monitor's next tick, or until a reconnect when the monitor is off.
- The breaker counts only errors from calls that reached a live connection. So one caller's application errors refuse
  every caller that shares the client (inventory § 2).

## What Changes

- The connection circuit breaker is removed:
  - the failure count, backoff, circuit timer and circuit test;
  - `StatusCircuitOpen`, `ErrCircuitOpen`, `Client.Failures` and `Client.Backoff`;
  - `Status.FailureCount`, `LastFailureTime` and `Reconnects` (the last is never written).
- A failed call returns its own error and refuses no other call. `Status()` changes only on `Connect`, the connection's
  events, the health monitor and `Close`.
- The remaining `atomic.Value`, the status, becomes typed.
- The dead `connStatus` is removed.
- The health monitor stays (owner question 2).

## Capabilities

- Modified: `transport-client`. Two requirements are modified, one is removed and re-added without `ErrCircuitOpen`,
  and one is added.
- Modified: `projection-mutation`. One requirement drops `ErrCircuitOpen`.

## Impact

- Files:
  - `natsclient/client.go`, `stream.go`, `request.go`, `errors.go`, `consumer_policy.go`, `owned_delivery.go`, plus
    `README.md`, `doc.go` and the tests;
  - `internal/graphmutation/client.go` (one `errors.Is` term, two comments) and its test;
  - `pkg/projection` (one comment, two test rows);
  - `docs/adr/112-natsclient-contract.md` (amendment); `docs/admission-ledger.yaml` (natsclient and graphmutation rows).
- Breaking at cutover, found by the compiler:
  - semsource `code-context/http_error.go:69`, `internal/entitypub/classify.go:54` and one test (`ErrCircuitOpen`,
    `StatusCircuitOpen`);
  - pin `service/service_manager.go:1752` (`FailureCount`) when `service` is ported.
  - semconnect is unaffected.
- Part c of #222. #222 stays open for 6d. Addresses #223.
