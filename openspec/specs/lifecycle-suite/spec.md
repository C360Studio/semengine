# lifecycle-suite Specification

## Purpose
The lifecycle suite is the portable owner-lifecycle floor every SemEngine owner is tested against, plus the probes
that let a test observe joins and retained state instead of inferring them from timing or goroutine counts.

## Requirements

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

### Requirement: Bound is not a join

Checks SHALL treat a Stop that returns after its context ended as incomplete, SHALL report which observation was
missing, and SHALL NOT invent a second Stop or a replacement context.

#### Scenario: Owner ignores the Stop deadline

- **WHEN** Stop does not return before the check's bound
- **THEN** the check reports the bound and the retained state without hanging the test

### Requirement: Probes retain observable state

`probe.Callback` SHALL expose entered, release, and joined channels; `probe.ObservedContext` SHALL signal the first
Done() observation and optionally hold it; `probe.Await` SHALL poll under the caller's context and return the last
observation's value and its error, if it had one, on failure.

#### Scenario: Await reports the last observation

- **WHEN** the observed condition never holds
- **THEN** Await returns after the context ends with the last observation's value and its error, if it had one,
  in the message

### Requirement: Complete sensitivity matrix

The in-package failpoint double SHALL declare every failpoint in one table that gives the check it must trip, and
every sensitivity test of the suite SHALL take its cases from that table. The suite's own tests SHALL fail when a
declared failpoint has no expected check.

A worker is signalled when a Stop has told it to exit. The helper that ends a double after a check SHALL call Stop as
Run does after each check and SHALL then require that the double holds nothing, for every failpoint. It SHALL NOT wait
for a signalled worker before it checks that requirement, so that a Stop that returned nil ahead of its worker's exit
is reported and not hidden by the helper's own wait. A worker that no Stop has signalled SHALL be ended through its
Start context and joined before the requirement is checked. The helper SHALL join every worker the double started
before it returns, whether or not the requirement held.

#### Scenario: Failpoint declared without a check

- **WHEN** a failpoint is declared and the table gives it no expected check
- **THEN** a `lifecycletest` test fails naming the failpoint

#### Scenario: Worker of a double that never stops it

- **WHEN** the helper ends a double whose Stop returned nil without signalling its worker
- **THEN** the helper ends and joins that worker, the double then reports nothing unresolved, and the worker has
  exited before the helper returns

#### Scenario: Stop returned ahead of its worker's exit

- **WHEN** the helper ends a double whose Stop signalled its worker and returned nil, and the worker has not exited
- **THEN** the helper fails the test naming what the double still holds, and the worker has exited before the helper
  returns

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
