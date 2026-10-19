# lifecycle-suite

## MODIFIED Requirements

### Requirement: Portable floor

`lifecycletest` SHALL provide error-returning checks over `Owner{Start(ctx) error; Stop(ctx) error; Observe()
Observation}`, where Observe, reporting the owner's unresolved resources and per-operation call counts, is required at
compile time so that no completion check can be skipped or fail on an owner that cannot report its state, for: nil Start
and Stop contexts refused; pre-cancelled and pre-expired Start refused with the matching context error; Stop before
Start safe; the Start context ending after Start returned stopping nothing, so that every item the owner reported
unresolved just after Start is still reported once a settle window after that context ended has passed, and a following
Stop under fresh finite authority returns nil and leaves nothing unresolved; a Stop whose own context has already ended
returning an error that carries that context's error or cause; repeated Stop as a no-op within its bound; second Start
refused unless the factory promises restart, in which case a full second cycle SHALL succeed; and a failed Start holding
nothing. The settle window is the suite's only wait on a healthy owner: an owner whose reported items last until
Stop passes for any window length, and an owner is judged only by what its Observe reports. `Run(t, factory, mustFail,
promise)` SHALL drive them as subtests, where `mustFail` is a required factory returning an owner whose Start returns
an error; Run SHALL fail before any check, naming the argument, when `mustFail` is nil. The failed-start check SHALL
require that Start returns a non-nil error,
that the owner then reports nothing unresolved, and that a following Stop returns nil and changes no call count. No
check SHALL count a panic as a refusal. The suite SHALL NOT use aggregate goroutine or memory counts as proof of joins,
and SHALL NOT read readiness from the owner. The failed-start check judges a failed Start whose own cleanup succeeded; a
failed Start whose own cleanup also fails is governed by "Failed start whose own cleanup fails" and is not judged by this
check.

#### Scenario: Suite detects each violation

- **WHEN** Run executes against the in-package failpoint double with one failpoint enabled
- **THEN** exactly the corresponding check fails, and all checks pass against the clean double, the failed-start
  check being given the clean double in its must-fail construction mode

#### Scenario: Owner whose work ends with its Start context

- **WHEN** an owner's continuing work ends once the context given to Start ends, and its Observe stops listing that
  work before the settle window has passed
- **THEN** the Start-context check fails naming what the owner no longer holds

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

### Requirement: Complete sensitivity matrix

The in-package failpoint double SHALL declare every failpoint in one table that gives the check it must trip, and
every sensitivity test of the suite SHALL take its cases from that table. The suite's own tests SHALL fail when a
declared failpoint has no expected check.

A worker is signalled when a Stop has told it to exit. The helper that ends a double after a check SHALL call Stop as
Run does after each check and SHALL then require that the double holds nothing, for every failpoint. It SHALL NOT wait
for a signalled worker before it checks that requirement, so that a Stop that returned nil ahead of its worker's exit
is reported and not hidden by the helper's own wait. A worker that no Stop has signalled SHALL be ended by cancelling
the double's own run context and joined before the requirement is checked. The helper SHALL join every worker the
double started before it returns, whether or not the requirement held.

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
