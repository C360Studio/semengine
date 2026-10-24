# operator-surface delta: setup-04a-03-service-startup-seam

## ADDED Requirements

### Requirement: The startup gauge reports the startup counts as they are when collected

From the moment `StartAll` has sealed the composition, before it binds any listener, the service manager SHALL expose
the gauge `semengine_startup_units{owner,stage}` on the injected metrics registry with exactly nine series:
`owner="services"` with `stage` `admitted`, `starts_invoked`, `starts_completed` and `starts_failed`, and
`owner="components"` with `stage` `admitted`, `lifecycle_participants`, `starts_invoked`, `starts_completed` and
`starts_failed`. Each collection SHALL report the counts `/services` reports at that moment, so no count falls while
startup runs.

#### Scenario: A component manager that is not the framework's

- **WHEN** the `component-manager` service is not the framework's `*ComponentManager`
- **THEN** the component series read zero
- **AND** the test that verifies this is `TestStartupGaugeReadsZeroComponentsForAStandInComponentManager`

#### Scenario: Counts while a component's Start is held

- **WHEN** a component manager with one lifecycle component and one plain component is starting and the lifecycle
  component's `Start` is held
- **THEN** the gauge reads two admitted, one lifecycle participant, one start invoked and none completed
- **AND** the test that verifies this is `TestComponentStartupSnapshotDistinguishesAdmittedAndLifecycleStarts`

#### Scenario: A later start finishing first

- **WHEN** of two components started together the second fails and returns before the first
- **THEN** the gauge reads one completed and one failed, then two completed and still one failed
- **AND** the test that verifies this is `TestComponentMetricPublicationRemainsMonotonicUnderReverseCompletion`

#### Scenario: A foreign collector under the name

- **WHEN** the registry already holds a collector named `semengine_startup_units`
- **THEN** `StartAll` fails before any listener binds or service starts, and adopts no foreign collector
- **AND** the test that verifies this is `TestStartupMetricRegistrationConflictFailsBeforeListenersOrChildren`

#### Scenario: A component manager on its own

- **WHEN** a component manager is started with no service manager
- **THEN** its registry gathers no `semengine_startup_units`
- **AND** the test that verifies this is `TestComponentManagerOnItsOwnPublishesNoStartupGauge`
