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
SHALL NOT use aggregate goroutine or memory counts as proof of joins, and SHALL NOT read readiness from the owner. The
failed-start check judges a failed Start whose own cleanup succeeded; a failed Start whose own cleanup also fails is
governed by "Failed start whose own cleanup fails" and is not judged by this check.

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

### Requirement: Failed start whose own cleanup fails

A component this module ports MAY return from a failed Start still holding what its own cleanup could not release,
provided that Start's error reports both the start failure and the cleanup failure, and that the component keeps what is
left on record so that a later Stop tries again to release it. A Stop that then completes the cleanup SHALL return nil
and leave nothing unresolved. Each such component SHALL prove this branch with its own test, named in that component's
admission-ledger row under `proving_tests`; the shared failed-start check of "Portable floor" is not run on it. This
requirement binds the components this module ports; a component outside this module is proven by its own tests (#77
ruling, comment 6035317931).

#### Scenario: graph-ingest's cleanup fails after a failed start

- **WHEN** graph-ingest's Start fails after acquiring resources and the cleanup it runs then also fails
- **THEN** Start returns an error that reports both failures, the adapter lists what is still held, and a following
  Stop that can complete the cleanup returns nil and leaves nothing listed, as
  `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` proves

#### Scenario: Cleanup succeeds after a failed start

- **WHEN** graph-ingest's failing factory makes Start fail and the cleanup it runs succeeds
- **THEN** the shared failed-start check passes: Start returned an error and nothing is unresolved

### Requirement: Failed-start rollback helper

`pkg/lifecyclecleanup.RollbackFailedStart(parent, rollback)` SHALL run `rollback` synchronously, before it returns,
under a context that keeps the parent's values, does not inherit the parent's cancellation or deadline, and has a
deadline five seconds after the call. It SHALL return the rollback's error joined with that context's error, and nil
only when the rollback returned nil within the budget. It SHALL return an error, without running anything, when the
parent is nil or the rollback is nil. It SHALL start no goroutine that outlives the call and import only the standard
library.

#### Scenario: Cancelled parent

- **WHEN** the parent context is already cancelled and carries a value, and the rollback reads its own context
- **THEN** the rollback sees a live context with a deadline and the parent's value, and its nil result makes the
  helper return nil

#### Scenario: Expired parent

- **WHEN** the parent context's deadline has already passed and it carries a value, and the rollback reads its own
  context
- **THEN** the rollback sees a live context with a fresh deadline and the parent's value, and its nil result makes the
  helper return nil

#### Scenario: Rollback outlives the budget

- **WHEN** the rollback returns only after its context's deadline has passed
- **THEN** the helper returns an error that includes the deadline error

#### Scenario: Nil rollback

- **WHEN** the helper is called with a live parent and a nil rollback
- **THEN** it returns an error naming the nil rollback

#### Scenario: Nil parent

- **WHEN** the helper is called with a nil parent
- **THEN** it returns an error and does not call the rollback
