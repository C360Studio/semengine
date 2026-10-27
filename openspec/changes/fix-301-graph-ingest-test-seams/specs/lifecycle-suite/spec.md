# lifecycle-suite

## MODIFIED Requirements

### Requirement: Failed-start check

The failed-start check SHALL require that Start returns a non-nil error, that the owner then reports nothing
unresolved, and that a following Stop returns nil and changes no call count. For an adapter that shows cleanup on the
NATS server instead of counting calls ("Observe adapter contract"), the call count is empty, so for the following Stop
this check proves no more than that it returns nil; what the server shows held after that Stop is proved by the
owner's own post-Stop check, not by this one. It judges a failed Start whose own cleanup succeeded; a failed Start
whose own cleanup also fails is governed by "Failed start whose own cleanup fails" and is not judged by this check.

#### Scenario: Failed start that holds a resource

- **WHEN** the `mustFail` factory's owner returns an error from Start but keeps a goroutine or handle
- **THEN** the failed-start check fails naming the unresolved item

#### Scenario: mustFail factory whose Start succeeds

- **WHEN** the `mustFail` factory's owner returns nil from Start
- **THEN** the failed-start check fails stating that the factory did not fail

### Requirement: Observe adapter contract

A service ported from the pin SHALL be run through the suite via a test-side adapter in its package, not by adding
Observe to production code. The adapter's Observation SHALL list, under Unresolved, every retained kind the service
holds while started — connections, subscriptions, consumers, key-value watchers, listeners, goroutines, tickers and
timers — by a stable name each. It SHALL count every cleanup or external call the service makes so that a no-op Stop
is provable, or SHALL show that cleanup on the NATS server instead. Showing it on the server means listing under
Unresolved what the server still shows the service holding, read without a test-only field on the service (#301).

#### Scenario: Adapter omits a retained kind (review check, not a test)

- **WHEN** an owner holds a subscription after a Stop that returned nil and its adapter does not list subscriptions
- **THEN** the port review rejects the adapter against the owner's retained kinds; the suite cannot detect the
  omission by itself

#### Scenario: Started owner reports its handles

- **WHEN** an adapted owner has started against the fixture
- **THEN** Observe lists each connection, subscription and goroutine the owner holds, and after a nil Stop lists none

#### Scenario: Cleanup shown on the NATS server

- **WHEN** graph-ingest's adapter observes it after a Stop that returned nil
- **THEN** a request on each of its request subjects finds no responder, and the server shows no outstanding pull on
  its durable consumer
- **AND** an input published after that read is counted pending on that consumer with nothing ack-pending, and the
  adapter lists nothing for either
