# operator-surface delta: setup-04a-03-service-startup-seam

## ADDED Requirements

### Requirement: The service manager registers the startup gauge

Once `StartAll` has sealed the composition, and before it binds any listener, the service manager SHALL register the
gauge `semengine_startup_units{owner,stage}` on its metrics registry: the injected one, else the `metrics` service's.
With neither registry, no gauge is registered and `StartAll` proceeds. A component manager SHALL NOT register the
gauge itself.

#### Scenario: A foreign collector under the name

- **WHEN** the registry already holds a collector named `semengine_startup_units`
- **THEN** `StartAll` fails before any listener binds or service starts, and adopts no foreign collector
- **AND** the test that verifies this is `TestStartupMetricRegistrationConflictFailsBeforeListenersOrChildren`

#### Scenario: A component manager on its own

- **WHEN** a component manager is started with no service manager
- **THEN** its registry gathers no `semengine_startup_units`
- **AND** the test that verifies this is `TestComponentManagerOnItsOwnPublishesNoStartupGauge`

### Requirement: The startup gauge has nine fixed series

The startup gauge SHALL report exactly nine series: `owner="services"` with `stage` `admitted`, `starts_invoked`,
`starts_completed` and `starts_failed`, and `owner="components"` with `stage` `admitted`, `lifecycle_participants`,
`starts_invoked`, `starts_completed` and `starts_failed`.

#### Scenario: Every series is present

- **WHEN** the registry is gathered
- **THEN** it holds exactly nine `semengine_startup_units` series
- **AND** the test that verifies this is `TestStartupCollectorReportsExactlyNineSeries`

#### Scenario: A component manager that is not the framework's

- **WHEN** the `component-manager` service is not the framework's `*ComponentManager`
- **THEN** the component series read zero
- **AND** the test that verifies this is `TestStartupGaugeReadsZeroComponentsForAStandInComponentManager`

### Requirement: The startup gauge reports the counts as they are when collected

Each collection of the startup gauge SHALL report the counts `/services` reports at that moment, so no count falls
while startup runs.

#### Scenario: Counts while a component's Start is held

- **WHEN** a component manager with one lifecycle component and one plain component is starting and the lifecycle
  component's `Start` is held
- **THEN** the gauge reads two admitted, one lifecycle participant, one start invoked and none completed
- **AND** the test that verifies this is `TestComponentStartupSnapshotDistinguishesAdmittedAndLifecycleStarts`

#### Scenario: A later start finishing first

- **WHEN** of two components started together the second fails and returns before the first
- **THEN** the gauge reads one completed and one failed, then two completed and still one failed
- **AND** the test that verifies this is `TestComponentMetricPublicationRemainsMonotonicUnderReverseCompletion`
