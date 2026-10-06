# lifecycle-suite

## MODIFIED Requirements

### Requirement: Portable floor

`lifecycletest` SHALL provide error-returning checks over `Owner{Start(ctx) error; Stop(ctx) error; Observe()
Observation}`, where Observe, reporting the owner's unresolved resources and per-operation call counts, is required at
compile time so that no completion check can be skipped or fail on an owner that cannot report its state, for: nil Start
and Stop contexts refused; pre-cancelled and pre-expired Start refused with the matching context error; Stop before
Start safe; controlled Stop completing under live Start authority with fresh finite cleanup authority; abort Stop
preserving the caller's context cause; repeated Stop as a no-op within its bound; second Start refused unless the
factory promises restart, in which case a full second cycle SHALL succeed; and a failed Start holding nothing. `Run(t,
factory, mustFail, promise)` SHALL drive them as subtests, where `mustFail` is a required factory returning an owner
whose Start returns an error; Run SHALL fail before any check, naming the argument, when `mustFail` is nil. The
failed-start check SHALL require that Start returns a non-nil error, that the owner then reports nothing unresolved,
and that a following Stop returns nil and changes no call count. No check SHALL count a panic as a refusal. The suite
SHALL NOT use aggregate goroutine or memory counts as proof of joins, and SHALL NOT read readiness from the owner.

#### Scenario: Suite detects each violation

- **WHEN** Run executes against the in-package failpoint double with one failpoint enabled
- **THEN** exactly the corresponding check fails, and all checks pass against the clean double, the failed-start
  check being given the clean double in its must-fail construction mode

#### Scenario: Failed start that holds a resource

- **WHEN** the `mustFail` factory's owner returns an error from Start but keeps a goroutine or handle
- **THEN** the failed-start check fails naming the unresolved item

#### Scenario: Nil mustFail factory

- **WHEN** Run is called with a nil `mustFail`
- **THEN** Run fails before any check, naming the `mustFail` argument

#### Scenario: mustFail factory whose Start succeeds

- **WHEN** the `mustFail` factory's owner returns nil from Start
- **THEN** the failed-start check fails stating that the factory did not fail

#### Scenario: Start panics on a nil context

- **WHEN** an owner's `Start(nil)` panics instead of returning an error
- **THEN** the nil-context check fails naming the panic

## ADDED Requirements

### Requirement: Observe adapter contract

A service ported from the pin SHALL be run through the suite via a test-side adapter in its package, not by adding
Observe to production code. The adapter's Observation SHALL list, under Unresolved, every retained kind the service
holds while started — connections, subscriptions, consumers, key-value watchers, listeners, goroutines, tickers and
timers — by a stable name each, and SHALL count every cleanup or external call the service makes so that a no-op Stop
is provable.

#### Scenario: Adapter omits a retained kind (review check, not a test)

- **WHEN** an owner holds a subscription after a Stop that returned nil and its adapter does not list subscriptions
- **THEN** the port review rejects the adapter against the owner's retained kinds; the suite cannot detect the
  omission by itself

#### Scenario: Started owner reports its handles

- **WHEN** an adapted owner has started against the fixture
- **THEN** Observe lists each connection, subscription and goroutine the owner holds, and after a nil Stop lists none
