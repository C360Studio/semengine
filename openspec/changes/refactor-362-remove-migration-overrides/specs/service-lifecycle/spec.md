<!-- markdownlint-configure-file { "MD013": { "line_length": 120, "heading_line_length": 100 } } -->

# service-lifecycle delta: refactor-362-remove-migration-overrides

## MODIFIED Requirements

### Requirement: The component manager's Start bounds Start

`ComponentManager.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second `Start` and a
`Start` after `Stop` (`errs.ErrAlreadyStarted`), and a refused `Start` SHALL start nothing. Ending `Start`'s context
after `Start` returned SHALL stop nothing the manager owns. The manager SHALL pass the `lifecycle-suite` checks through
a test-side adapter.

#### Scenario: The Start context ends after Start

- **WHEN** a component manager composing a component that follows ADR-113 is started and its `Start` context is then
  cancelled
- **THEN** the base service, the health publisher and the component's work are all still held,
  and a following `Stop` returns nil
- **AND** the tests that verify this are `TestComponentManagerPassesTheLifecycleSuite` and
  `TestComponentStartContextIsTheManagersStartContext`

### Requirement: The component manager's Stop is the only stop and waits for a Start in progress

`Stop(ctx)` SHALL be the only stop: it SHALL refuse a nil context, return an ended context's error and stop nothing, and
otherwise stop each lifecycle component with `ctx` in reverse start order, then the health publisher and
the base service, returning nil only when all are released. A `Stop` during `Start` SHALL wait for every
`Start` to return, or return its own context's error first.

#### Scenario: Stop during Start

- **WHEN** one component's `Start` is held open and `Stop` is called
- **THEN** `Stop` does not return before that `Start` returns, then stops each component once in reverse order and
  returns nil
- **AND** the test that verifies this is `TestStopDuringStartWaitsForComponentStarts`

## REMOVED Requirements

### Requirement: A lapsed stream migration override is reported while running

**Reason**: The owner removed `stream_migration_overrides` and its running expiry reporter (#362, comment
6439894887). With no override there is nothing to lapse, so the component manager registers no
`semengine_streams_migration_override_expired` gauge and runs no reporter.

**Migration**: None. Deployments start fresh, and no configuration in this repository or the four starter consumers
declares an override. An alert copied from the example rule reads a series that no longer exists and never fires.
