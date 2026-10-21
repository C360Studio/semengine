# service-lifecycle

## ADDED Requirements

### Requirement: The component manager's Start bounds Start

`ComponentManager.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second `Start` and a
`Start` after `Stop` (`errs.ErrAlreadyStarted`), and a refused `Start` SHALL start nothing. Otherwise it SHALL start the
admitted lifecycle components in two phases, `component.StoreProvider` components first and then the rest, each phase's
components concurrently with its start order recorded by name, and SHALL begin a phase only after every `Start` of the
one before returned. Each component's `Start` SHALL get the context the manager's `Start` got, which the manager never
ends (ADR-113). A failed `Start` SHALL return every failure joined, each naming its component, after one rollback that
stops every launched component in reverse launch order; what that rollback could not stop SHALL be stopped by the next
`Stop`. Ending `Start`'s context after `Start` returned SHALL stop nothing the manager owns. `Stop(ctx)` SHALL be the
only stop: it SHALL refuse a nil context, return an ended context's error and stop nothing, and otherwise stop each
lifecycle component with `ctx` in reverse start order, then the health publisher, the override reporter and the base
service, returning nil only when all are released. A `Stop` during `Start` SHALL wait for every `Start` to return, or
return its own context's error first. A component whose `Stop` failed, or was not reached because `ctx` ended, SHALL be
stopped again by the next `Stop`; one whose `Stop` returned nil SHALL not be called again. The manager SHALL pass the
`lifecycle-suite` checks through a test-side adapter.

#### Scenario: The Start context ends after Start

- **WHEN** a component manager composing a component that follows ADR-113 is started and its `Start` context is then
  cancelled
- **THEN** the base service, the health publisher, the override reporter and the component's work are all still held,
  and a following `Stop` returns nil
- **AND** the tests that verify this are `TestComponentManagerPassesTheLifecycleSuite` and
  `TestComponentStartContextIsTheManagersStartContext`

#### Scenario: Stop during Start

- **WHEN** one component's `Start` is held open and `Stop` is called
- **THEN** `Stop` does not return before that `Start` returns, then stops each component once in reverse order and
  returns nil
- **AND** the test that verifies this is `TestStopDuringStartWaitsForComponentStarts`

#### Scenario: A failed Start's cleanup fails

- **WHEN** a held `Start` fails while `Stop` waits, and one component's rollback `Stop` is armed to fail once
- **THEN** `Start` returns both errors, and the waiting `Stop` calls that component's `Stop` again and returns nil
- **AND** the tests that verify this are `TestStopDuringAFailingStartStopsWhatTheRollbackLeft` and
  `TestComponentManagerFailedStartCleanupFailureRetainsAuthorityForLaterStop`

#### Scenario: A failed Stop is retried

- **WHEN** a started manager's `Stop` returns an error because one component's `Stop` failed
- **THEN** the next `Stop` calls only that component's `Stop` again, and returns nil once it succeeds
- **AND** the tests that verify this are `TestStopRetriesOnlyComponentsThatDidNotStop` and
  `TestPropComponentManagerStopHistory`

### Requirement: The component manager composes the boot set once

`NewComponentManager(raw, deps)` SHALL decode `raw` strictly and refuse every key, naming it, and SHALL refuse a nil
`deps`, a nil `deps.Manager`, a configuration manager with no configuration and a nil `deps.ComponentRegistry`. It
SHALL read the configuration once and build each enabled component through `Registry.CreateComponent`. When any
configured component cannot be built, construction SHALL be refused, naming every component that failed; enabled
`rule-processor` instances that fail SHALL be refused together, sorted by instance. It SHALL run `composition.Analyze`
over the admitted declarations and the configuration's explicit streams, log every finding, keep the result, and
refuse construction when any finding has error severity, naming each; then it SHALL seal the component registry.
Writes to the configuration after construction SHALL not change the composed set.

#### Scenario: An error finding refuses construction

- **WHEN** the boot composition has a finding of error severity
- **THEN** `NewComponentManager` returns an error naming the finding, and nothing is started
- **AND** the test that verifies this is `TestComponentManagerRefusesBootOnErrorFinding`

#### Scenario: A component cannot be built

- **WHEN** a configured component's factory returns an error
- **THEN** `NewComponentManager` returns an error naming that component and every other one that failed
- **AND** the test that verifies this is `TestComponentManagerRefusesBootWhenAComponentCannotBeBuilt`

#### Scenario: Rule-pack failures

- **WHEN** two enabled `rule-processor` instances fail to build
- **THEN** construction is refused with both named, sorted by instance
- **AND** the test that verifies this is `TestComponentManagerInitializeAggregatesEnabledRulePackFailuresDeterministically`

#### Scenario: A nil component registry

- **WHEN** `NewComponentManager` is given dependencies with no component registry
- **THEN** it returns an error saying the component manager requires a component registry
- **AND** the test that verifies this is `TestNewComponentManagerRefusesANilComponentRegistry`

#### Scenario: A key in the payload

- **WHEN** the component manager's payload is `{"enabled_components": []}`
- **THEN** construction is refused naming `enabled_components`
- **AND** the test that verifies this is `TestComponentManagerConfigRefusesEveryKey`

### Requirement: Providers start before consumers and own their stores

After a `component.StoreProvider`'s `Start` returns nil, the component manager SHALL register its provided stores in
the store registry in instance order, and SHALL refuse an empty instance name, a nil store or an instance another
component holds, removing that provider's registrations from the same call and failing its start. No consumer SHALL be
started before every provider's `Start` returned. `Stop` SHALL remove a provider's stores from the registry before it
calls that provider's `Stop`.

#### Scenario: Providers register before consumers start

- **WHEN** a composition has a provider and a consumer
- **THEN** the consumer's `Start` begins only after the provider's stores are in the registry
- **AND** the test that verifies this is `TestComponentManagerStart_ProvidersRegisterBeforeConcurrentConsumers`

#### Scenario: A duplicate instance

- **WHEN** a second provider claims a store instance the first holds
- **THEN** the claim is refused, the first provider's store is untouched, and the second provider's other
  registrations from that call are removed
- **AND** the tests that verify this are `TestDuplicateOwnership_RefusedNotClobbered` and
  `TestDuplicateOwnership_RollsBackOnlyRivalRegistrations`

#### Scenario: Stores leave before the provider stops

- **WHEN** a started provider is stopped
- **THEN** its stores are gone from the registry when its `Stop` is called
- **AND** the test that verifies this is `TestComponentManagerRunningStopDeregistersStoreBeforeLiveChildStop`

### Requirement: Component health is published and its drops counted

With a NATS client, the component manager SHALL publish `{"timestamp","name","health"}` for each admitted component on
`health.component.<name>` once when `Start` returns and every 5 seconds until `Stop`; with none it SHALL publish
nothing. A publish or encode that fails SHALL increment
`semengine_errors_total{service="component-manager",type="health_publish"}` and log at Debug naming the subject, and
publishing SHALL go on. A failed `Start` SHALL launch no publisher.

#### Scenario: Publish fails

- **WHEN** the manager's NATS client is not connected
- **THEN** each failed publish increments the error count with `type="health_publish"`
- **AND** the test that verifies this is `TestComponentHealthPublishFailureIsCounted`

### Requirement: A lapsed stream migration override is reported while running

The component manager SHALL register `semengine_streams_migration_override_expired{stream,owner}` through
`metric.RegisterOrGet` at construction and use the collector it returns. It SHALL evaluate the live configuration's
stream migration overrides at construction and every minute from `Start` to `Stop`, setting 0 for each declared bridge
and 1, with a Warn line, for each lapsed one. Readiness SHALL not depend on it. `Stop` SHALL return only after the
reporter returned.

#### Scenario: An override lapses while running

- **WHEN** a declared bridge's expiry passes while the manager runs
- **THEN** the gauge for that stream reads 1 at the next evaluation, without a restart
- **AND** the tests that verify this are `TestOverrideExpiry_CrossesTheDeadlineWithoutRestart` and
  `TestComponentManagerStartRunsOverrideExpiryReporter`

#### Scenario: Two managers on one registry

- **WHEN** two component managers are built on one metrics registry
- **THEN** the second's evaluation shows on the gathered gauge family
- **AND** the test that verifies this is `TestOverrideGaugeUsesTheRegisteredCollector`

### Requirement: Rule-pack contracts are checked before any injection

`ConfigureRulePackMutations(m)` SHALL refuse a nil manager and SHALL run every binder's
`PreflightProjectionMutations` before it injects anything, so that one failure injects nothing. It SHALL validate each
pack ID, refuse a duplicate, copy and validate each pack's contracts, refuse contracts without a NATS client, and then
inject one `projection.PredicateReconciler` per pack that has contracts.

#### Scenario: A preflight fails

- **WHEN** one of two binders' preflight fails
- **THEN** no binder receives a reconciler
- **AND** the test that verifies this is `TestConfigureRulePackMutationsRefusesBeforeAnyInjection`

#### Scenario: One client per pack

- **WHEN** two packs have contracts
- **THEN** each pack's binder receives its own mutation client
- **AND** the test that verifies this is `TestConfigureRulePackMutationsInjectsOneClientPerPack`

### Requirement: The component manager's reads wait on no lock and not on Stop

The component manager's set of components SHALL be fixed at construction, and each component's state (its lifecycle
state, last error, startup counts, provided stores and whether its `Stop` returned nil) SHALL be held on its own,
under no manager-wide lock. A read of component state or health (`GetComponentStatus`, `GetComponentHealth`, the
health publisher and the `/components` routes) SHALL not wait for any component's `Start` or `Stop` and SHALL not be
refused during or after `Stop`; a read during or after `Stop` SHALL ask the component itself. A component's `Health`,
`DataFlow` and `DebugStatus` SHALL be callable at any time, during `Start` or `Stop` and after `Stop`, and return
without waiting for either; this is tested for the components in this module and review only for others.

#### Scenario: A read while a component's Start is blocked

- **WHEN** one component's `Start` is held open by the test
- **THEN** `GetComponentStatus`, `GetComponentHealth` and `/components/status/{name}` return, and a sibling whose
  `Start` returned reads `StateStarted`
- **AND** the test that verifies this is `TestComponentReadsReturnWhileAComponentStartIsBlocked`

#### Scenario: A read while a component's Stop is blocked

- **WHEN** one component's `Stop` is held open by the test
- **THEN** the same reads return
- **AND** the tests that verify this are `TestComponentReadsReturnWhileAComponentStopIsBlocked` and
  `TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop`

#### Scenario: A read after Stop asks the component

- **WHEN** a started manager has been stopped
- **THEN** `GetComponentHealth` and `/components/health` return each component's own answer after its `Stop`
- **AND** the test that verifies this is `TestComponentHealthAfterStopAsksEachComponent`

#### Scenario: The component set is fixed

- **WHEN** the component list is read before `Start`, after it, after `Stop` and after a failed `Start`, and a
  `withComponents` callback writes a component's `State`
- **THEN** every read gives the same names and start positions, and the write changes nothing the manager reports
- **AND** the tests that verify this are `TestComponentListIsFixedAtConstruction` and
  `TestComponentReadsRaceStartAndStop`

### Requirement: The component manager's health reads every component

The component manager's own health check SHALL read every component's state each time it runs and SHALL report
unhealthy when any component is in `StateFailed`, naming each such component in name order with its last error, and
healthy otherwise. It SHALL never answer healthy without reading every component: no answer stands in for a state it
could not read, whether or not `Start` has completed.

#### Scenario: A component left failed by a rollback

- **WHEN** a component's `Start` fails and its rollback `Stop` fails too
- **THEN** the health check reports unhealthy, naming that component and carrying its error
- **AND** the test that verifies this is `TestComponentManagerHealthNamesAComponentLeftFailedByARollback`

#### Scenario: Two failed components

- **WHEN** a `Stop` leaves two components failed
- **THEN** the health check names both, each carrying its error, and a retried `Stop` that succeeds clears it
- **AND** the test that verifies this is `TestComponentManagerHealthNamesEveryFailedComponent`
