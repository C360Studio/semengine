# Design: simplify-04a-16-natsclient-circuit

This design depends on `INVENTORY PASS` for `inventory.md`; premises cite its sections. It ports nothing from the pin,
so no pin probe applies; its pin facts are code and caller reads (§ 2, § 4). It adds no statement on how nats.go times,
orders or fails, and it strikes ADR-112 lines about the breaker (D4). Edits outside `natsclient`, in `pkg/errs`,
`internal/graphmutation`, `pkg/projection`, `graph` and `graph/kvcatalog`, are needed for the build or are comments
only (#159).

## D1. The circuit breaker is removed

Premises, each measured:

- **What it counts (§ 2):** any caller's errors on one shared client, namely
  - application errors on a live connection (`CreateStream`, a publish no stream captures, consumer `Info`);
  - request timeouts and no-responders (`request.go:241`);
  - failed `Connect` dials while no connection is installed (`client.go:721`, `:726`, `:757`).
- **Earlier exceptions:** four already exist, for the application-error case (§ 2).
- **What opening it does:** every JetStream, request and `Connect` call on the client refuses, for 1 s doubling to
  1 min. The health monitor ends that within one tick on a live connection (`client.go:2262`).
- **What closing it does:** the circuit test marks a live connection `Disconnected` (§ 1).
- **What ignores it:** core `Publish`, `Subscribe` and KV calls (§ 2).
- **What specs promise:** only `ErrCircuitOpen`'s class.

Behaviour dropped, stated for the owner (question 1):

- **(a) `Connect` after 15 failed dials** returned `ErrCircuitOpen` without dialling (`client.go:689`, `:727`).
  - Under B it dials each time it is called, bounded by its context.
  - Nothing measured relies on this. Each caller calls `Connect` once, at boot:
    - pin `internal/bootstrapobservability/bootstrap.go:139`;
    - semsource `cmd/semsource/run.go:465`, semconnect `cmd/cs-api-server/main.go:98`, semteams
      `cmd/semteams/main.go:573`;
    - in this repo, `processor/graph-ingest/component.go:983`.
- **(b) After 15 request timeouts or no-responders**, later JetStream and request calls failed at once with a
  transient `ErrCircuitOpen`. Under B each one waits for its own timeout or answer.
  - A graphmutation write that times out was not-committed (`TransportUnavailable`,
    `internal/graphmutation/client.go:171`) once the breaker had opened, and commit-unknown before that. Under B it is
    always commit-unknown, which is what it is: the request was sent.

What changes:

- Every call returns the error it got.
- Removed: `recordFailure*`, `recordStreamPublishFailure`, `isCircuitNeutralStreamCapacityError`, `resetCircuit*`,
  `armCircuitTimer`, `circuitTimerFired`, `testCircuit`, `workCircuitTest`, and the seven fields (`client.go:73`,
  `:86-90`, `:138`).
- `connectFailed` sets `Disconnected` only.
- The async publish error handler keeps its metric and log.

## D2. The status is the connection's

`Status()` is written by `Connect`, `handleDisconnect`, `handleReconnect`, the health monitor and `Close`, as today
less the breaker. The status is held in a typed atomic, `atomic.Int32` holding the `ConnectionStatus`. `Status()` is
callable with or without `mu` held: the health monitor (`client.go:2260-2263`) calls it while holding `mu`, so a
`Status()` that took `mu` would hang the monitor's first tick and then `Close`.

Proof:

- `TestHealthMonitorTickReturnsStatusAndClose` (Tests).
- The existing `-race` suites (`TestConcurrentSafety`, the close and lifecycle tests).

## D3. Callers of the removed names

- `pkg/errs`: `ErrCircuitOpen` (`errs.go:75`), its `IsTransient` term (`:169`) and its test rows
  (`classify_by_type_test.go:29`, `errs_test.go:44`, `:394`) go (Q3).
- `internal/graphmutation/client.go:171`: the `errors.Is(err, natsclient.ErrCircuitOpen)` term goes. So do the
  comments `:26` and `:168`, and the test row `client_test.go:158`.
- `pkg/projection`: the comment `mutation_client.go:417` and the test rows `classify_mutation_error_test.go:75` and
  `commit_ambiguity_test.go:179` drop it.
- Comments: `graph/exact_entity.go:43-46` and `exact_entity_test.go:112` say "fail while the caller's context is live",
  without the breaker. `graph/kvcatalog/owned_bucket_retention.go:62-63` drops "(exempted from the shared circuit
  breaker)".

## D4. Text follows the code

- **ADR-112:** an amendment line. It strikes `:164` and `:170-172`, and `:236`'s "and a circuit refusal".
- **`README.md` and `doc.go`:** the circuit sections and mentions (inventory § 3) go. `doc.go:356-359` keeps its first
  sentence on `ReconnectWait`.
- **Ledger, natsclient row:** `consumer_purpose` "connection lifecycle with circuit breaker and health status" becomes
  "connection lifecycle and health status". The contract gains a "Changed surface, simplify-04a-16 (#222)" sentence,
  and `:864` drops the two defaults.
- **Ledger, graphmutation row:** `:2028` drops `ErrCircuitOpen`.
- **Ledger, errs row:** gains a changed-surface sentence (Q3).

## Options

- **A, repair:**
  - when its backoff ends, `testCircuit` sets the installed connection's own state (`connStatus`) and zeroes the
    round's count; backoff still resets only on a success;
  - the health monitor moves only `Connected` and `Reconnecting`, never an open circuit;
  - `status`, `lastFailure` and `backoff` become typed atomics.

  A adds a requirement, "The circuit closes to the connection's state", and keeps every export. It keeps one caller's
  failures refusing every other caller.
- **B, remove:** this design.
- **Do nothing:** the three named items are recorded as measured on #222. The live connection marked `Disconnected`
  stays: stream calls refuse after each circuit test until the next tick, or until a reconnect with the monitor off.

## Tests

New:

- **`TestIntegration_FailedCallsRefuseNoOtherCall`**
  - Setup: the package's shared server, reached through the reset, with `WithHealthInterval(0)`. Otherwise the monitor
    sets `Connected` over an open circuit at its tick (`client.go:2262`).
  - Twenty `PublishToStream` calls to a subject no stream captures each return that publish's own failure (no stream
    for the subject), never a circuit refusal. A publish to an existing stream then succeeds, and `Status()` is
    `Connected`. The assertion names the no-stream error, not `ErrCircuitOpen`, which this change removes.
  - It fails on base: call 16 returns `ErrCircuitOpen`.
- **`TestHealthMonitorTickReturnsStatusAndClose`**
  - Setup: an in-package test on a shared-server client with a health interval of a few milliseconds. The test stores
    `StatusDisconnected` into the status field, as `kv_key_contract_integration_test.go:74` already does to private
    state. That adds no production hook.
  - It awaits `Status() == Connected`, which only a tick writes on a healthy connection, through `internal/harness`
    polling with a deadline and no sleep. `Close` then returns nil within its context.
  - It fails on a `Status()` that takes `mu`, because the tick hangs. It fails on its own deadline before its cleanup
    `Close` runs, since that `Close` would hang too.

Kept behaviour that only `Failures()` proved, with a new oracle:

- **`request_integration_test.go:366`** (`TestIntegration_RequestWithRetryClassified_NoResponder`): `MaxAttempts`
  4 makes four attempts.
  - New oracle: the delta of `client.GetConnection().Stats().OutMsgs` across the call is 4.
  - This rests on nats.go counting a request that gets no-responders as one sent message. The test is the probe at the
    pinned nats.go.
  - If it does not hold, the developer switches to a subscriber that never replies plus a short per-attempt timeout,
    and the subscriber's receive count is 4.
- **`errors_integration_test.go:326`** (`TestIntegration_RequestWithRetryClassified_RetriesThenSucceeds`): the
  responder must appear only after the first attempt failed.
  - New oracle: the responder is subscribed from the start. It does not reply to its first request and answers the
    second, with a short per-attempt timeout.
  - Its handler count is 2 and the call returns the reply. That proves a retry from the responder's side.

Deleted, because they test only the breaker:

- `client_test.go`:
  - `TestCircuitBreaker_OpensAfterFailures`, `_Reset`, `_ExponentialBackoff`;
  - the circuit-open rows of `TestStatus_Transitions` and `TestKeyValueBuckets`;
  - the `FailureCount` and `LastFailureTime` assertions in `TestMetrics` (`:457-464`).
- `getkv_circuit_breaker_test.go` and `getstream_circuit_breaker_test.go`: the six circuit tests. `fakeJetStream` moves
  to a test helper file, because `client_close_final_test.go`, `client_lifecycle_test.go`, `consume_context_test.go`
  and `stream_visibility_test.go` use it.
- `stream_capacity_circuit_test.go`, `stream_capacity_circuit_integration_test.go`.
- `publish_async_test.go`: `TestPublishToStreamAsync_CircuitOpen` and `TestAsyncPublishErrHandler_RecordsFailure`;
  `:43`'s `Failures()` line (the test keeps `ErrorIs(context.Canceled)`).
- `publish_async_integration_test.go:290-302`, the enqueue-resets-the-count test.
- `client_close_final_test.go:624-636`, `TestClientFailuresAfterCloseLeaveStatusDisconnected`.
- `client_lifecycle_test.go`: `TestClientCloseStopsCircuitTimer` (`:424-450`) and the `circuitTimer` line of the
  held-work list (`:82-83`).
- `client_sentinels_test.go:21`, the `ErrCircuitOpen` row.

Edited, with the failure-count assertions dropped and each test keeping its other checks:

- `client_close_final_test.go:670-682`: `TestClientAsyncPublishErrorAfterCloseRecordsMetricOnly` keeps the metric
  check.
- `request_cancel_breaker_test.go`: the in-flight `context.Canceled` check stays.
- `nil_context_siblings_test.go:128`, `:134`; `client_connect_test.go:86`; `errors_integration_test.go`;
  `request_integration_test.go`; `integration_test.go`; `client_async_error_test.go:142-171`;
  `kv_key_contract_integration_test.go:74` (typed store).
- `pkg/errs`, `internal/graphmutation` and `pkg/projection` test rows (D3).

Generated checks: examples suffice. With nothing counted, no input interacts with another call.

## Exports (#190)

| Symbol | Action | Kind | Reader |
| --- | --- | --- | --- |
| `StatusCircuitOpen` | removed (Q1) | framework API, contract line README `:37-41` | semsource test only |
| `natsclient.ErrCircuitOpen` | removed (Q1, Q3) | framework API: transport-client, ADR-112 `:164` | graphmutation `:171` (D3); semsource ×2 |
| `pkg/errs.ErrCircuitOpen` | removed (Q3) | framework API (named by transport-client until this change); no producer once B lands | `errs.go:169` only |
| `Client.Failures` | removed (Q1) | framework API, contract line spec `:126` | tests only |
| `Client.Backoff` | removed (Q4) | no contract line (a `doc.go` example only): owner decides | tests only |
| `Status.FailureCount` | removed (Q1) | plumbing for admitted, unported pin `service_manager.go:1752`; removing it changes that port | none here |
| `Status.LastFailureTime` | removed (Q4) | no contract line: owner decides | `client_test.go:459` |
| `Status.Reconnects` | removed | dead: never written or read | none |
| `Client.Status`, `ConnectionStatus`, its four other constants | kept | plumbing | `graph-ingest/component.go:982` |
| `Client.IsHealthy`, `WaitForConnection` | kept | plumbing | pin `service/base.go:434`, rule `processor.go:1041`; `graph-ingest/component.go:989` |
| `Client.GetStatus`, `Status.Status`, `Status.RTT` | kept | plumbing for admitted pin `service_manager.go:1745-1748` | none here yet |
| `WithHealthInterval` | kept | framework API (option list, README) | semsource `run.go:451` |
| `Client.RTT` | kept, unchanged (Q4) | no production reader; README `:235` only names it | tests only |

No back door.

## ADRs

- Pin: none govern this (§ 6).
- ADR-112: amended (D4). No new library statement.

## Open issues and merge order

- #222: part c is done here (D1-D2). The PR body says "Addresses #222", and 6d closes it.
- #75 and #169 stay for 6d. #114 stays (`model`).
- Filed: #291, graph-ingest `Start` calls `Connect` when the status is not `Connected` (`component.go:982-984`). When a
  component restarts while NATS reconnects, `Connect` returns `ErrAlreadyStarted` and `Start` fails.
- #286 (8c) shares 13 files with this change (inventory § 3), with no circuit hunk. Either merge order works; the second
  to land merges `origin/main` and reruns `task verify`.

## Owner questions

1. Remove the shared circuit breaker, repair it, or leave it? Recommended: remove. What goes: after 15 failures in a
   row on one client, every JetStream and request call on that client fails at once for 1 second up to 1 minute.
   Without the breaker, each call waits for its own answer or timeout, and a mutation write that times out is reported
   as "may have been written" instead of "not written". Nothing we measured relies on the fast failure.
2. Keep the health monitor in 6c? Recommended: yes. Its remaining job without the breaker is to mark a connection
   whose ping goes unanswered as `Reconnecting`. semsource sets its interval.
3. Only if 1 is B: remove `natsclient.ErrCircuitOpen` and `pkg/errs.ErrCircuitOpen`? Recommended: yes, both. Nothing
   returns either. semsource checks for this error in two places; those checks stop compiling at cutover and simply
   drop the term.
4. Exports with no contract line: remove `Backoff` and `Status.LastFailureTime` with the breaker, and keep `Client.RTT`
   unchanged? Recommended: yes.
