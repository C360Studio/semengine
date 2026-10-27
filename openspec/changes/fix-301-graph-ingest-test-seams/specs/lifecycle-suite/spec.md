# lifecycle-suite

## MODIFIED Requirements

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
- **THEN** an input published to its input stream stays pending, a request on each of its request subjects finds no
  responder, and the adapter lists nothing for either
