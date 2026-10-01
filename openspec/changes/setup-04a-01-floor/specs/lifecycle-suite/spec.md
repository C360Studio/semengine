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
factory, mustFail, promise)` SHALL drive them as subtests, where `mustFail` is a required value of a dedicated
`StartFailure` type whose values are built only by `MustFail(factory)` — a factory returning an owner whose Start
returns an error; `MustFail(nil)` SHALL panic at the call site naming the argument — or the function
`NoFallibleStart()`; the type's zero value SHALL be rejected by Run with a failure
naming the argument, and a plain factory SHALL NOT be accepted in its place. The failed-start check SHALL require
that Start returns a non-nil error, that the owner then reports nothing unresolved, and that a following Stop
returns nil and changes no call count. For an owner whose start cannot fail from any input, the caller SHALL pass
`NoFallibleStart()`; Run SHALL then run no failed-start subtest, SHALL call no skip and SHALL write no log line in
its place, and SHALL run every other check; the owner is reported by the pinned list of owners without a fallible
start. Omitting the argument SHALL remain a compile error. The suite SHALL NOT use aggregate goroutine or
memory counts as proof of joins, and SHALL NOT read readiness from the owner.

#### Scenario: Suite detects each violation

- **WHEN** Run executes against the in-package failpoint double with one failpoint enabled
- **THEN** exactly the corresponding check fails, and all checks pass against the clean double, the failed-start
  check being given the clean double in its must-fail construction mode

#### Scenario: Failed start that holds a resource

- **WHEN** the `mustFail` factory's owner returns an error from Start but keeps a goroutine or handle
- **THEN** the failed-start check fails naming the unresolved item

#### Scenario: MustFail with a nil factory

- **WHEN** a test calls `MustFail(nil)`
- **THEN** the call panics naming the argument before Run is reached

#### Scenario: Zero StartFailure value

- **WHEN** Run is called with the zero value of `StartFailure`
- **THEN** Run fails before any check, naming the `mustFail` argument

#### Scenario: Owner declared without a fallible start

- **WHEN** Run is called with `NoFallibleStart()`
- **THEN** no failed-start subtest runs, no skip is called and no line is logged in its place, and every other check
  runs

#### Scenario: mustFail factory whose Start succeeds

- **WHEN** the `mustFail` factory's owner returns nil from Start
- **THEN** the failed-start check fails stating that the factory did not fail

## ADDED Requirements

### Requirement: Pinned owners without a fallible start

A contract test SHALL parse the module's Go files with `go/parser` and resolve `lifecycletest` through each file's
import names; inside package `lifecycletest` the bare identifiers `Run` and `NoFallibleStart` are recognised without
an import name. It SHALL collect every call of `NoFallibleStart`. Each call SHALL be keyed by package directory,
enclosing top-level function — with its receiver type for a method — and the source text of the factory and promise
expressions passed as `Run`'s second and fourth arguments, so one function may run the same owner under two promises.
The test SHALL fail when that set differs from a list pinned in the test, naming each call missing from the list by
`file:line` and key and each list entry with no call, and SHALL fail on two calls with the same key, and on a call
anywhere nested inside the body of a `for` or `range` statement, including inside a func literal such as one passed
to `t.Run`, so a loop over a table of factories cannot fold several owners into one entry. The test SHALL fail on
any reference to `NoFallibleStart` other than a direct call written as `Run`'s third argument — through a
variable, a function value, a helper or a dot import — naming `file:line`; the function's own declaration is not a
reference. Each entry SHALL name the owner it exempts, and review checks that name against the factory, because the
test cannot. The check SHALL have no allowlist beyond the pinned list itself, and adding an owner SHALL require
changing the list in the same diff.

#### Scenario: New owner declared without a fallible start

- **WHEN** an adapter test gains `Run(t, factory, NoFallibleStart(), promise)` and the pinned list is unchanged
- **THEN** the contract test fails naming the file, line and key

#### Scenario: Stale entry

- **WHEN** the pinned list names a key that has no `NoFallibleStart()` call
- **THEN** the contract test fails naming the entry

#### Scenario: Exemption passed through a variable

- **WHEN** a test assigns `NoFallibleStart()` to a variable and passes the variable to Run
- **THEN** the contract test fails naming the file and line of the call

#### Scenario: Two owners under one key

- **WHEN** one test function calls `Run(t, f, NoFallibleStart(), promise)` in a loop over a table of factories
- **THEN** the contract test fails naming the file and line of the call inside the loop; written out as separate
  calls, each is keyed by its own factory expression text, and two calls with the same key fail naming both

#### Scenario: Method on an adapter type

- **WHEN** the call sits in a method of an adapter type
- **THEN** its key names the receiver type and the method

### Requirement: Observe adapter contract

An owner ported from the pin SHALL be run through the suite via a test-side adapter in the owner's package, not by
adding Observe to production code. The adapter's Observation SHALL list, under Unresolved, every retained kind the
owner holds while started — connections, subscriptions, consumers, key-value watchers, listeners, goroutines,
tickers and timers — by a stable name each, and SHALL count every cleanup or external call the owner makes so that a
no-op Stop is provable. An owner whose start takes no context SHALL be recorded as outside the suite with its reason
on its ledger row.

#### Scenario: Adapter omits a retained kind (review check, not a test)

- **WHEN** an owner holds a subscription after a Stop that returned nil and its adapter does not list subscriptions
- **THEN** the port review rejects the adapter against the owner's retained kinds; the suite cannot detect the
  omission by itself

#### Scenario: Started owner reports its handles

- **WHEN** an adapted owner has started against the fixture
- **THEN** Observe lists each connection, subscription and goroutine the owner holds, and after a nil Stop lists none
