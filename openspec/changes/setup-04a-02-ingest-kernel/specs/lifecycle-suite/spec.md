# lifecycle-suite

## MODIFIED Requirements

### Requirement: Observe adapter contract

A service ported from the pin SHALL be run through the suite via a test-side adapter in its package, not by adding
Observe to production code. A component ported from the pin is a service; its failing factory SHALL make a real
dependency fail (a broker that refuses the connection, a missing stream, or a key-value fault), never a stub of the
component. The adapter's Observation SHALL list, under Unresolved, every retained kind the service holds while started
— connections, subscriptions, consumers, key-value watchers, listeners, goroutines, tickers and timers — by a stable
name each, and SHALL count every cleanup or external call the service makes so that a no-op Stop is provable.

#### Scenario: Adapter omits a retained kind (review check, not a test)

- **WHEN** an owner holds a subscription after a Stop that returned nil and its adapter does not list subscriptions
- **THEN** the port review rejects the adapter against the owner's retained kinds; the suite cannot detect the
  omission by itself

#### Scenario: Started owner reports its handles

- **WHEN** an adapted owner has started against the fixture
- **THEN** Observe lists each connection, subscription and goroutine the owner holds, and after a nil Stop lists none

#### Scenario: graph-ingest under the suite

- **WHEN** graph-ingest, built through its factory against the fixture with its input stream present, is run through
  `lifecycletest.Run` with a failing factory whose broker refuses the connection
- **THEN** every check passes, and the adapter lists graph-ingest's consumers, request subscriptions, ingest lanes and
  status loop while started and none after a nil Stop

#### Scenario: Failed start whose cleanup fails (not a suite check)

- **WHEN** graph-ingest's Start fails and the cleanup it runs then also fails
- **THEN** Start returns both errors, the adapter lists what is still held, and a following Stop that can complete
  its cleanup returns nil and leaves nothing listed
