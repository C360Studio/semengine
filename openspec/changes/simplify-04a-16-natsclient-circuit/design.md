# Design: simplify-04a-16-natsclient-circuit

This design depends on `INVENTORY PASS` for `inventory.md`; premises cite its sections. It ports nothing from the pin,
so no pin probe applies; its pin facts are code and caller reads (§ 2, § 4). It adds no statement on how nats.go times,
orders or fails, and it strikes ADR-112 lines about the breaker (D4).

## D1. The circuit breaker is removed

Premises:

- The breaker counts only errors from calls that passed a connected check, so a real outage is never counted
  (§ 1, § 2).
- What it counts is application errors on a live connection. Four exceptions already exist for exactly that (§ 2).
- Opening it refuses every caller on the client. Closing it marks a live connection `Disconnected` (§ 1).
- Core `Publish`, `Subscribe` and KV calls ignore it (§ 2).
- No spec requirement promises it. Only `ErrCircuitOpen`'s class is specified.

What changes:

- Every call returns the error it got.
- `Connect` dials each time it is called. Every measured caller calls it once (pin `service`; semsource `run.go:465`;
  semconnect `main.go:98`; semteams `main.go:573`).
- Removed: `recordFailure*`, `recordStreamPublishFailure`, `isCircuitNeutralStreamCapacityError`, `resetCircuit*`,
  `armCircuitTimer`, `circuitTimerFired`, `testCircuit`, `workCircuitTest`, and the six fields at `client.go:73`,
  `:86-90` and `:138`.
- `connectFailed` sets `Disconnected` only.
- The async publish error handler keeps its metric and log.

## D2. The status is the connection's

`Status()` is written by `Connect`, `handleDisconnect`, `handleReconnect`, the health monitor and `Close`, as today
less the breaker. It is stored as a typed value, so no load asserts a type. Mechanism is the developer's choice: a typed
atomic or a field under `mu`. `Status()` is called with `mu` held (`ownsStatusLocked`), so a field under `mu` needs
care. Proof: the existing `-race` suites (`TestConcurrentSafety`, the close and lifecycle tests) and compile.

## D3. Callers of the removed names

- `internal/graphmutation/client.go:171` drops `errors.Is(err, natsclient.ErrCircuitOpen)`.
- Comments at `:26`, `:168` and `pkg/projection/mutation_client.go:417` drop it too.
- Test rows drop it: `graphmutation/client_test.go:158`, `projection/classify_mutation_error_test.go:75`,
  `commit_ambiguity_test.go:179`.
- `pkg/errs.ErrCircuitOpen` stays, in another package. A follow-up issue records that it has no producer.

## D4. Text follows the code

- ADR-112 gets an amendment line. It strikes `:164` and `:170-172`, and `:236`'s "and a circuit refusal".
- `README.md`: the circuit sections and mentions (inventory § 3) go. `doc.go` the same. `doc.go:356-359` keeps its
  first sentence on `ReconnectWait`.
- Ledger natsclient row: `consumer_purpose` "connection lifecycle with circuit breaker and health status" becomes
  "connection lifecycle and health status". The contract gains a "Changed surface, simplify-04a-16 (#222)" sentence;
  `:864` drops the two defaults. The graphmutation row `:2028` drops `ErrCircuitOpen`.

## Option A (if owner question 1 is no): repair

- A1: when its backoff ends, `testCircuit` sets the installed connection's own state and zeroes the round's count. This
  uses `connStatus`. Backoff still resets only on a success.
- A2: the health monitor moves only `Connected` and `Reconnecting`, never an open circuit.
- A3: `status`, `lastFailure` and `backoff` become typed.
- A adds a transport-client requirement, "The circuit closes to the connection's state". It keeps every export and
  leaves every caller of an error that is not a connection failure refusing the others.

## Tests

- `TestIntegration_FailedCallsRefuseNoOtherCall`, new, on the package's shared server through the reset:
  - Twenty `PublishToStream` calls to a subject no stream captures each return an error that is not
    `errs.ErrCircuitOpen`.
  - A publish to an existing stream then succeeds, and `Status()` is `Connected`.
  - It fails on base: call 16 returns `ErrCircuitOpen`.
- Deleted, because they test only the breaker:
  - `TestCircuitBreaker_OpensAfterFailures`, `_Reset`, `_ExponentialBackoff`;
  - the "circuit open" subtests in `TestKeyValueBuckets` and `TestStatus_Transitions`;
  - `TestPublishToStreamAsync_CircuitOpen`, `TestAsyncPublishErrHandler_RecordsFailure`;
  - `getkv_circuit_breaker_test.go` and `getstream_circuit_breaker_test.go` tests. `fakeJetStream` moves to a test
    helper file; four other test files use it.
  - `stream_capacity_circuit_test.go`, `stream_capacity_circuit_integration_test.go`;
  - `TestClientCloseStopsCircuitTimer`.
- Edited, with the failure-count assertions dropped: `request_cancel_breaker_test.go`, `nil_context_siblings_test.go`,
  `client_connect_test.go:86`, `client_async_error_test.go:142-171`, `client_test.go:457-464`,
  `kv_key_contract_integration_test.go:74` (status store), and `client_lifecycle_test.go:82`.
- Generated checks: examples suffice. With nothing counted, no input interacts with another call.

## Exports (#190)

| Symbol | Action | Kind | Reader |
| --- | --- | --- | --- |
| `StatusCircuitOpen` | removed (Q1) | framework API by README `:37-41`, no production reader | semsource test |
| `ErrCircuitOpen` | removed (Q1, Q3) | framework API: transport-client, ADR-112 `:164` | graphmutation `:171` (D3); semsource ×2 |
| `Client.Failures` | removed (Q1) | framework API by spec `:126` only | tests |
| `Client.Backoff` | removed (Q1) | looks like framework API, no contract line (doc example) | tests |
| `Status.FailureCount` | removed (Q1) | read by admitted, unported pin `service:1752` | migrates when `service` ports |
| `Status.LastFailureTime` | removed | no contract line | test |
| `Status.Reconnects` | removed | dead: never written or read | none |
| `Status()`, `IsHealthy`, `GetStatus`, `RTT`, `WaitForConnection`, `ConnectionStatus` + four constants, `WithHealthInterval` | kept | plumbing / framework API | inventory § 4 |

No back door.

## ADRs

- Pin: none govern this (§ 6).
- ADR-112: amended (D4). No new library statement.

## Open issues and merge order

- #222: part c is done here (D1-D2). The PR body says "Addresses #222", and 6d closes it.
- #75 and #169 stay for 6d. #114 stays (`model`).
- To file:
  - graph-ingest `Start` calls `Connect` on an installed connection (`component.go:982-984`) and gets
    `ErrAlreadyStarted` during a reconnect.
  - `pkg/errs.ErrCircuitOpen` has no producer.
- #286 (8c) touches `internal/graphmutation/client.go` and the ledger, with no shared hunk. Either merge order works;
  the second to land merges `origin/main` and reruns `task verify`.

## Owner questions

1. Remove the breaker (B) or repair it (A)? Recommended: B.
2. Keep the health monitor in 6c? Recommended: yes.
3. Remove `natsclient.ErrCircuitOpen` with the breaker? Recommended: yes.
