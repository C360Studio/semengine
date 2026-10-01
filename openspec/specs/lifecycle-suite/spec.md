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
factory promises restart, in which case a full second cycle SHALL succeed. `Run(t, factory, Promise)` SHALL drive them
as subtests. The suite SHALL NOT use aggregate goroutine or memory counts as proof of joins.

#### Scenario: Suite detects each violation

- **WHEN** Run executes against the in-package failpoint double with one failpoint enabled
- **THEN** exactly the corresponding check fails, and all checks pass against the clean double

### Requirement: Bound is not a join

Checks SHALL treat a Stop that returns after its context ended as incomplete, SHALL report which observation was
missing, and SHALL NOT invent a second Stop or a replacement context.

#### Scenario: Owner ignores the Stop deadline

- **WHEN** Stop does not return before the check's bound
- **THEN** the check reports the bound and the retained state without hanging the test

### Requirement: Probes retain observable state

`probe.Callback` SHALL expose entered, release, and joined channels; `probe.ObservedContext` SHALL signal the first
Done() observation and optionally hold it; `probe.Await` SHALL poll under the caller's context and return the last
observation and error on failure.

#### Scenario: Await reports the last observation

- **WHEN** the observed condition never holds
- **THEN** Await returns after the context ends with the last observed value and last error in the message

