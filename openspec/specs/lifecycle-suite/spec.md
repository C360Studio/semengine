# lifecycle-suite Specification

## Purpose
The lifecycle suite is the portable owner-lifecycle floor every SemEngine owner is tested against, plus the probes
that let a test observe joins and retained state instead of inferring them from timing or goroutine counts. It also
covers a failed `Start` whose own cleanup fails: a component this module ports may then keep what it could not
release, and proves with its own test that a later `Stop` releases it. And it covers
`pkg/lifecyclecleanup.RollbackFailedStart`, which runs a failed `Start`'s rollback under a fresh five-second
deadline that keeps the caller's context values.

## Requirements

### Requirement: Portable floor

`lifecycletest` SHALL provide error-returning checks over `Owner{Start(ctx) error; Stop(ctx) error; Observe()
Observation}`, where Observe, reporting the owner's unresolved resources and per-operation call counts, is required at
compile time so that no completion check can be skipped or fail on an owner that cannot report its state, for: nil Start
and Stop contexts refused; pre-cancelled and pre-expired Start refused with the matching context error; and Stop before
Start safe.

#### Scenario: Suite detects each violation

- **WHEN** Run executes against the in-package failpoint double with one failpoint enabled
- **THEN** exactly the corresponding check fails, and all checks pass against the clean double, the failed-start
  check being given the clean double in its must-fail construction mode

### Requirement: Stop checks

`lifecycletest` SHALL also provide error-returning checks that a Stop whose own context has already ended returns an
error that carries that context's error or cause, and that a repeated Stop is a no-op within its bound.

#### Scenario: Stop with an ended context

- **WHEN** Stop is called with a context that has already ended
- **THEN** the check requires an error that carries that context's error or cause

#### Scenario: Repeated Stop

- **WHEN** Stop is called again after a Stop that returned nil
- **THEN** the check requires it to be a no-op within its bound

### Requirement: Second Start check

`lifecycletest` SHALL also provide an error-returning check that a second Start is refused unless the factory promises
restart, in which case a full second cycle SHALL succeed.

#### Scenario: Second Start without a restart promise

- **WHEN** Start is called a second time on an owner whose factory does not promise restart
- **THEN** the check requires Start to refuse

#### Scenario: Second Start with a restart promise

- **WHEN** the factory promises restart
- **THEN** a full second cycle of Start and Stop succeeds

### Requirement: Start context ending stops nothing

`lifecycletest` SHALL also provide an error-returning check that the Start context ending after Start returned stops
nothing, so that every item the owner reported unresolved just after Start is still reported once a settle window after
that context ended has passed, and a following Stop under fresh finite authority returns nil and leaves nothing
unresolved.

#### Scenario: Owner whose work ends with its Start context

- **WHEN** an owner's continuing work ends once the context given to Start ends, and its Observe stops listing that
  work before the settle window has passed
- **THEN** the Start-context check fails naming what the owner no longer holds

### Requirement: Owners are judged only by what Observe reports

The settle window is the suite's only wait on a healthy owner: an owner whose reported items last until Stop passes for
any window length, and an owner is judged only by what its Observe reports. The suite SHALL NOT use aggregate goroutine
or memory counts as proof of joins, and SHALL NOT read readiness from the owner.

#### Scenario: Reported items last until Stop

- **WHEN** an owner's reported items last until Stop
- **THEN** the Start-context check passes for any settle window length

### Requirement: Run drives the checks and requires a failing factory

`Run(t, factory, mustFail, promise)` SHALL drive the checks as subtests, where `mustFail` is a required factory
returning an owner whose Start returns an error; Run SHALL fail before any check, naming the argument, when `mustFail`
is nil.

#### Scenario: Nil mustFail factory

- **WHEN** Run is called with a nil `mustFail`
- **THEN** Run fails before any check, naming the `mustFail` argument

### Requirement: Failed-start check

The failed-start check SHALL require that Start returns a non-nil error, that the owner then reports nothing
unresolved, and that a following Stop returns nil and changes no call count. It judges a failed Start whose own cleanup
succeeded; a failed Start whose own cleanup also fails is governed by "Failed start whose own cleanup fails" and is not
judged by this check.

#### Scenario: Failed start that holds a resource

- **WHEN** the `mustFail` factory's owner returns an error from Start but keeps a goroutine or handle
- **THEN** the failed-start check fails naming the unresolved item

#### Scenario: mustFail factory whose Start succeeds

- **WHEN** the `mustFail` factory's owner returns nil from Start
- **THEN** the failed-start check fails stating that the factory did not fail

### Requirement: A panic is not a refusal

No check SHALL count a panic as a refusal.

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

#### Scenario: Failpoint declared without a check

- **WHEN** a failpoint is declared and the table gives it no expected check
- **THEN** a `lifecycletest` test fails naming the failpoint

### Requirement: The sensitivity helper does not hide an early Stop

A worker is signalled when a Stop has told it to exit. The helper that ends a double after a check SHALL call Stop as
Run does after each check and SHALL then require that the double holds nothing, for every failpoint. It SHALL NOT wait
for a signalled worker before it checks that requirement, so that a Stop that returned nil ahead of its worker's exit
is reported and not hidden by the helper's own wait.

#### Scenario: Stop returned ahead of its worker's exit

- **WHEN** the helper ends a double whose Stop signalled its worker and returned nil, and the worker has not exited
- **THEN** the helper fails the test naming what the double still holds, and the worker has exited before the helper
  returns

### Requirement: The sensitivity helper joins every worker

A worker that no Stop has signalled (told to exit) SHALL be ended by cancelling the double's own run context and joined
before the requirement is checked. The helper SHALL join every worker the double started before it returns, whether or
not the requirement held.

#### Scenario: Worker of a double that never stops it

- **WHEN** the helper ends a double whose Stop returned nil without signalling its worker
- **THEN** the helper ends and joins that worker, the double then reports nothing unresolved, and the worker has
  exited before the helper returns

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

### Requirement: Failed start whose own cleanup fails

A component this module ports MAY return from a failed Start still holding what its own cleanup could not release,
provided that Start's error reports both the start failure and the cleanup failure, and that the component keeps what
is left on record so that a later Stop tries again to release it. A Stop that then completes the cleanup SHALL return
nil and leave nothing unresolved.

#### Scenario: graph-ingest's cleanup fails after a failed start

- **WHEN** graph-ingest's Start fails after acquiring resources and the cleanup it runs then also fails
- **THEN** Start returns an error that reports both failures, the adapter lists what is still held, and a following
  Stop that can complete the cleanup returns nil and leaves nothing listed, as
  `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` proves

#### Scenario: Cleanup succeeds after a failed start

- **WHEN** graph-ingest's failing factory makes Start fail and the cleanup it runs succeeds
- **THEN** the shared failed-start check passes: Start returned an error and nothing is unresolved

### Requirement: Each ported component proves its failed-cleanup branch

Each ported component that may return from a failed Start holding resources SHALL prove this branch with its own test,
named in that component's admission-ledger row under `proving_tests`; the shared failed-start check of "Portable floor"
is not run on it. This requirement binds the components this module ports; a component outside this module is proven by
its own tests (#77 ruling, comment 6035317931).

#### Scenario: The proving test is named in the ledger

- **WHEN** a ported component can return from a failed Start still holding what its cleanup could not release
- **THEN** its admission-ledger row names the test that proves the branch under `proving_tests`

### Requirement: Failed-start rollback helper

`pkg/lifecyclecleanup.RollbackFailedStart(parent, rollback)` SHALL run `rollback` synchronously, before it returns,
under a context that keeps the parent's values, does not inherit the parent's cancellation or deadline, and has a
deadline five seconds after the call.

#### Scenario: Cancelled parent

- **WHEN** the parent context is already cancelled and carries a value, and the rollback reads its own context
- **THEN** the rollback sees a live context with a deadline and the parent's value, and its nil result makes the
  helper return nil

#### Scenario: Expired parent

- **WHEN** the parent context's deadline has already passed and it carries a value, and the rollback reads its own
  context
- **THEN** the rollback sees a live context with a fresh deadline and the parent's value, and its nil result makes the
  helper return nil

### Requirement: The rollback helper reports the rollback's error and the budget

`RollbackFailedStart` SHALL return the rollback's error joined with the budget context's error, and nil only when the
rollback returned nil within the budget.

#### Scenario: Rollback outlives the budget

- **WHEN** the rollback returns only after its context's deadline has passed
- **THEN** the helper returns an error that includes the deadline error

### Requirement: The rollback helper refuses nil arguments

`RollbackFailedStart` SHALL return an error, without running anything, when the parent is nil or the rollback is nil.

#### Scenario: Nil rollback

- **WHEN** the helper is called with a live parent and a nil rollback
- **THEN** it returns an error naming the nil rollback

#### Scenario: Nil parent

- **WHEN** the helper is called with a nil parent
- **THEN** it returns an error and does not call the rollback

### Requirement: The rollback helper leaves nothing running and imports only the standard library

`RollbackFailedStart` SHALL start no goroutine that outlives the call and import only the standard library.

#### Scenario: The helper returns

- **WHEN** the helper returns
- **THEN** it has started no goroutine that is still running, and its package imports only the standard library
