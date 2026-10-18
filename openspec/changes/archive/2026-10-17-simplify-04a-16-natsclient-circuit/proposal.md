# Proposal: simplify-04a-16-natsclient-circuit

## Why

PR 6c of epic #223 (#223 comment 6233147857): #222 part c. Part c names three defects; inventory § 1 measured each:

- The `AccountInfo` trip does not exist, and there are three `atomic.Value` fields, not six.
- `testCircuit` does leave a live connection marked `Disconnected`. JetStream calls then refuse with `ErrNotConnected`
  until the health monitor's next tick, or until a reconnect when the monitor is off.
- The breaker counts any caller's errors on one shared client: application errors on a live connection, request
  timeouts, and failed dials (inventory § 2). Fifteen in a row make every call on the client refuse for 1 s to 1 min.

## What Changes

- The connection circuit breaker is removed:
  - its seven fields, the circuit timer and the circuit test;
  - `StatusCircuitOpen`, `natsclient.ErrCircuitOpen` and `pkg/errs.ErrCircuitOpen` (owner question 3);
  - `Client.Failures` and `Client.Backoff`;
  - `Status.FailureCount`, `LastFailureTime` and `Reconnects` (the last is never written).
- A failed call returns its own error and refuses no other call. `Status()` changes only on `Connect`, the connection's
  events, the health monitor and `Close`.
- The status is a typed atomic. The dead `connStatus` is removed.
- The health monitor stays (owner question 2). `Client.RTT` is unchanged (owner question 4).

## Capabilities

- Modified: `transport-client`. Two requirements are modified, one is removed and re-added without `ErrCircuitOpen`,
  and one is added.
- Modified: `projection-mutation`. One requirement drops `ErrCircuitOpen`.

## Impact

- Files:
  - `natsclient/client.go`, `stream.go`, `request.go`, `errors.go`, `consumer_policy.go`, `owned_delivery.go`, plus
    `README.md`, `doc.go` and the tests;
  - `pkg/errs/errs.go` and its tests (Q3);
  - `internal/graphmutation/client.go` (one `errors.Is` term) and its test;
  - comments only: `pkg/projection/mutation_client.go`, `graph/exact_entity.go` and its test,
    `graph/kvcatalog/owned_bucket_retention.go`; two `pkg/projection` test rows;
  - `docs/adr/112-natsclient-contract.md` (amendment); `docs/admission-ledger.yaml` (natsclient, graphmutation, errs
    rows).
- Behaviour that callers can see:
  - after a run of failures, each call waits its own timeout instead of refusing at once;
  - `Connect` dials each time it is called;
  - a graphmutation write that times out is commit-unknown, where after 15 failures it was not-committed.
- Breaking at cutover, found by the compiler:
  - semsource `code-context/http_error.go:69`, `internal/entitypub/classify.go:54` and one test;
  - pin `service/service_manager.go:1752` (`FailureCount`) when `service` is ported.
  - semconnect is unaffected.
- Part c of #222. #222 stays open for 6d. Addresses #223.
