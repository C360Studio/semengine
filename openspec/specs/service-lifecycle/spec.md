# service-lifecycle Specification

## Purpose

`service` is the layer a SemEngine process will boot on: services that embed `BaseService`, the managers that start
and stop them, and the helpers they share. This capability states what `BaseService` promises an embedding service
(Start bounds Start, `Stop(ctx)` is the only stop, a health check every 30 seconds, ADR-113), what the profiling
server promises its caller (`MaybeStartPProf` then `Shutdown`), and what `WireGraphRuntime` does before it builds
the graph mutation client (the catalog-retention backstop, pin ADR-068 D1). The service and component managers and
the services arrive in later pull requests of the port and extend this capability or add their own.

## Requirements

### Requirement: BaseService's Start bounds Start and Stop is the only stop

`service.BaseService.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second Start and a
Start after any Stop, and a refused Start SHALL start nothing. A started `BaseService` SHALL report `StatusRunning`,
run its health check once at Start and then every 30 seconds, and SHALL keep doing so when the context given to Start
ends. `Stop(ctx)` SHALL be the only stop. It SHALL refuse a nil context. Given a context that has already ended, it
SHALL return that context's error and stop nothing. Given a live context, it SHALL report `StatusStopping`, start no
further health check, and return nil only after the health goroutine has exited, leaving `StatusStopped` and
`IsHealthy()` false; if that context ends while Stop waits, Stop SHALL return its error and a later Stop SHALL wait
again. A repeated Stop after one that returned nil SHALL return nil. `BaseService` SHALL pass the `lifecycle-suite`
checks through a test-side adapter (ADR-113). The package SHALL declare no stop sentinel of its own;
`errs.ErrAlreadyStopped` is the one.

#### Scenario: The Start context ends after Start

- **WHEN** a started `BaseService`'s Start context is cancelled and the clock then advances 60 seconds
- **THEN** its status is still `StatusRunning`, two more health checks have run, and a following Stop returns nil
- **AND** the tests that verify this are `TestStartContextEndingKeepsHealthChecksRunning` and
  `TestBaseServicePassesTheLifecycleSuite`

#### Scenario: Stop is given a context that has already ended

- **WHEN** a started `BaseService`'s Stop is called with a cancelled context
- **THEN** Stop returns `context.Canceled`, the status is still `StatusRunning`, and a following Stop with a live
  context returns nil
- **AND** the test that verifies this is `TestStopWithAnEndedContextStopsNothing`

#### Scenario: Stop's context ends while a health check blocks

- **WHEN** a health check blocks and Stop, called with a live context, has that context cancelled while it waits
- **THEN** Stop returns `context.Canceled` and the status is `StatusStopping`; a second Stop does not return while
  the check blocks, and returns nil once it returns, with no further check run
- **AND** the test that verifies this is `TestRepeatedStopWaitsForABlockedHealthCheck`

### Requirement: A failed health check makes the service unhealthy

A started `BaseService` SHALL report `IsHealthy()` false and an unhealthy `Health()` when the check set by
`SetHealthCheck` returns an error, or when its NATS client is not healthy. It SHALL report healthy when the check
passes and the client is healthy.

#### Scenario: Failing health check

- **WHEN** a `BaseService` whose health check returns an error is started
- **THEN** after its first check `IsHealthy()` is false and `Health()` reports unhealthy
- **AND** the test that verifies this is `TestFailingHealthCheckMakesTheServiceUnhealthy`

### Requirement: The profiling server stops on Shutdown

`service.MaybeStartPProf(debug, port, logger)` SHALL start nothing and return nil when `debug` is false or `port` is
not positive. Otherwise it SHALL bind `:port` before it returns and serve `http.DefaultServeMux`; when the bind fails
it SHALL log a Warn naming the port and the error on `logger` (`slog.Default()` when nil), start nothing and return
nil. `Shutdown(ctx)` SHALL refuse a nil context before anything else, on a nil server too; a nil server's Shutdown
with a context SHALL return nil. Otherwise Shutdown SHALL stop accepting and return nil only after every in-flight
request has finished and the serving goroutine has exited. If `ctx` ends first, it SHALL return that error, and a
later Shutdown SHALL wait again.

#### Scenario: Disabled

- **WHEN** `MaybeStartPProf(false, 1, logger)` is called
- **THEN** it returns nil, and that nil server's Shutdown with a live context returns nil
- **AND** the test that verifies this is `TestMaybeStartPProfDisabledStartsNothing`

#### Scenario: Port already bound

- **WHEN** `MaybeStartPProf(true, port, logger)` is called with the port of a listener the test holds
- **THEN** it returns nil, and `logger` has recorded one Warn naming that port
- **AND** the test that verifies this is `TestMaybeStartPProfLogsABindFailure`

#### Scenario: Nil context

- **WHEN** Shutdown is called with a nil context on a started server and on a nil server
- **THEN** both calls return an invalid-class error, and the started server still serves until a later Shutdown
- **AND** the test that verifies this is `TestPProfShutdownRefusesANilContext`

#### Scenario: Shutdown under an in-flight request

- **WHEN** a request is blocked in a handler and Shutdown is called with an ended context
- **THEN** Shutdown returns `context.Canceled`; after the handler returns, a later Shutdown returns nil
- **AND** the test that verifies this is `TestPProfShutdownWaitsForAnInFlightRequest`

#### Scenario: Nothing left behind

- **WHEN** the server is started on a loopback listener inside `synctest.Test`, serves `GET /debug/pprof/` with
  status 200, and is shut down
- **THEN** Shutdown returns nil and the bubble returns
- **AND** the test that verifies this is `TestPProfServesTheIndexAndLeavesNothingRunning`

### Requirement: WireGraphRuntime cleans catalog retention first

`service.WireGraphRuntime(ctx, client, logger, contracts...)` SHALL run the catalog-retention backstop
(`kvcatalog.EnsureCatalogRetentionClean`, pin ADR-068 D1) before it builds the graph mutation client, SHALL return
the backstop's error and no client when it fails, including for a nil context, and SHALL start no goroutine
(pin ADR-091).

#### Scenario: A catalog bucket left with a foreign TTL

- **WHEN** `EMBEDDING_INDEX` exists with a 7-day TTL and a stored key, and `WireGraphRuntime` is called
- **THEN** it returns a mutation client, the bucket's TTL is 0, and the key is still stored
- **AND** the test that verifies this is `TestWireGraphRuntimeStripsAForeignTTL`

#### Scenario: Nil context

- **WHEN** `WireGraphRuntime` is called with a nil context
- **THEN** it returns an invalid-class error and no client
- **AND** the test that verifies this is `TestWireGraphRuntimeRefusesANilContext`

### Requirement: The service manager's StartAll bounds StartAll

`service.Manager.StartAll(ctx)` SHALL refuse a nil context, a context that has already ended, a second `StartAll` and a
`StartAll` after `StopAll` (`errs.ErrAlreadyStarted`), and a refused `StartAll` SHALL start nothing. Otherwise it SHALL
seal the composition, bind its main listener and the metrics server it claimed before it starts any service, start every
service in registration order, and commit the full route set and readiness only after every service started. A failure
at any step SHALL be returned joined with the rollback's error, and the rollback SHALL stop what had started; a rollback
that fails SHALL be run again by `StopAll`. Ending the context given to `StartAll` after it returned SHALL stop nothing:
no service, no health publishing, no listener, and no request's context. `StopAll(ctx)` SHALL be the only stop. It SHALL
refuse a nil context; given a context that has already ended it SHALL return that context's error and stop nothing;
before any `StartAll` it SHALL return nil, stopping only a health listener that was started. Otherwise it SHALL stop
every service in reverse registration order, then the health publisher, the listeners and the metrics server, and return
nil only when all are released; a listener SHALL stop accepting and `StopAll` SHALL wait for its in-flight requests. If
`ctx` ends first, `StopAll` SHALL close every connection still open on its listeners, ending each open request's
context, and return that error; a later `StopAll` SHALL return nil only once every handler has returned. A `StopAll`
after one that returned nil SHALL return nil and stop nothing. The manager and the metrics service SHALL pass the
`lifecycle-suite` checks through test-side adapters (ADR-113).

#### Scenario: The StartAll context ends after StartAll

- **WHEN** a manager composing a child `BaseService` and the metrics service is started and its `StartAll` context
  is then cancelled
- **THEN** its main listener, health publisher, metrics server and child service are all still held, a request
  served afterwards sees a request context that has not ended, and a following `StopAll` returns nil
- **AND** the tests that verify this are `TestManagerPassesTheLifecycleSuite` and
  `TestRequestContextOutlivesStartContext`

#### Scenario: StopAll is given a context that has already ended

- **WHEN** a started manager's `StopAll` is called with a cancelled context
- **THEN** `StopAll` returns `context.Canceled`, every service is still running, and a following `StopAll` with a
  live context returns nil
- **AND** the test that verifies this is `TestStopAllWithAnEndedContextStopsNothing`

#### Scenario: StopAll's context ends while a request is open

- **WHEN** a handler on the main listener waits for its request context to end and then for a release, and `StopAll`
  is called with a 100 ms context
- **THEN** `StopAll` returns `context.DeadlineExceeded`, the handler has seen its request context end, and a new
  connection to the listener is refused; a second `StopAll` does not return before the release, and returns nil
  after it
- **AND** the test that verifies this is `TestStopAllCutsOpenConnectionsAtItsDeadline`

#### Scenario: A service fails to start

- **WHEN** the second of two services returns an error from `Start` during `StartAll`
- **THEN** `StartAll` returns that error, the first service has been stopped, and the main listener no longer
  accepts connections
- **AND** the test that verifies this is `TestStartAllRollsBackFailedChildStartWithDetachedBoundedContext`

#### Scenario: The main port is already bound

- **WHEN** `StartAll` is called with the port of a listener the test holds
- **THEN** it returns the bind error and no service's `Start` has been called
- **AND** the test that verifies this is `TestStartAllMainBindFailureStartsNoService`

### Requirement: StopAll counts a service already stopped as stopped

`StopAll` SHALL treat a service whose `Stop` returns nil or `errs.ErrAlreadyStopped` as stopped. Any other error
SHALL be returned naming the service, the services after it SHALL still be stopped, and the next `StopAll` SHALL call
every service's `Stop` again. `service` SHALL declare no stop sentinel of its own.

#### Scenario: One service was already stopped

- **WHEN** a started manager holds one service whose `Stop` returns `errs.ErrAlreadyStopped` and one whose `Stop`
  returns nil, and `StopAll` is called
- **THEN** `StopAll` returns nil and each service's `Stop` was called once
- **AND** the test that verifies this is `TestStopAllCountsAlreadyStoppedAsStopped`

#### Scenario: One service fails to stop

- **WHEN** one service's `Stop` returns an error during `StopAll`
- **THEN** `StopAll` returns an error naming that service, the other service was stopped, and a second `StopAll`
  calls the failing service's `Stop` again
- **AND** the test that verifies this is `TestStopAllRetainsServiceAuthorityAfterFailure`

### Requirement: Every composition has a component manager

`ConfigureFromServices` and `StartAll` SHALL refuse a composition with no `component-manager` service, naming the
missing constructor when none is registered. Once `StartAll` has sealed the composition, `RegisterInstance`,
`CreateService` and `ConfigureFromServices` SHALL return a `*service.CompositionSealedError`.

#### Scenario: Only the metrics constructor is registered

- **WHEN** a registry holding only the `metrics` constructor configures a manager with no `services` section
- **THEN** `ConfigureFromServices` returns an error reading `create configured service component-manager: no
  constructor registered for service component-manager`
- **AND** the test that verifies this is `TestConfigureRefusesACompositionWithoutAComponentManager`

#### Scenario: A service is added after StartAll

- **WHEN** `CreateService` is called on a manager whose `StartAll` returned nil
- **THEN** it returns a `*service.CompositionSealedError` and the service is not added
- **AND** the test that verifies this is `TestCreateServiceRefusedAfterStartAll`

### Requirement: The service manager publishes health and counts its drops

With a NATS client, the manager SHALL publish one JSON object with the keys `timestamp`, `name`, `status` and
`health` for every composed service on `health.service.<name>` at once when `StartAll`, after every service has
started, starts the publisher, and then every 5 seconds until `StopAll`; with none it SHALL publish nothing. A
publish or encode that fails SHALL increment
`semengine_errors_total{service="service-manager",type="health_publish"}` on the injected metrics registry, SHALL be
logged at Debug naming the subject, and SHALL not stop the loop.

#### Scenario: Publish at StartAll

- **WHEN** a manager with a connected client and a stream on `health.>` starts a service named `probe`
- **THEN** a message on `health.service.probe` arrives with `name` equal to `probe` and `status` equal to `running`
- **AND** the test that verifies this is `TestHealthPublisherPublishesEachServiceAtStart`

#### Scenario: Publish fails

- **WHEN** a manager whose NATS client is not connected completes `StartAll`
- **THEN** `StartAll` returns nil and `semengine_errors_total{service="service-manager",type="health_publish"}` is at
  least 1
- **AND** the test that verifies this is `TestHealthPublishFailureIsCounted`

### Requirement: The metrics service serves on its own or through the manager

`service.NewMetrics` SHALL default `port` to 9090 and `path` to `/metrics`. Standalone, `Metrics.Start(ctx)` SHALL
bind its port before it returns, returning a bind failure and holding nothing, and `Stop` SHALL stop its server and
its health check. When the manager has claimed its server, the manager SHALL start that server before any service
starts and stop it after every service stopped, and `Metrics` SHALL report healthy only while that server serves. A
claim after `Metrics.Start` or `Stop` was called, or a second claim, SHALL be refused.

#### Scenario: Standalone bind failure

- **WHEN** `Metrics.Start` is called with the port of a listener the test holds
- **THEN** it returns an error and the service holds no server
- **AND** the test that verifies this is `TestMetricsStartReportsServerBindFailureBeforeReturning`

#### Scenario: Claim after Start

- **WHEN** a started `Metrics` is asked for its server by the manager
- **THEN** the claim returns an error wrapping `errs.ErrAlreadyStarted`
- **AND** the test that verifies this is `TestMetricsRefusesAClaimAfterStart`

#### Scenario: The lifecycle floor

- **WHEN** the lifecycle suite runs against a standalone `Metrics` on a test listener
- **THEN** every check passes, `StartContextEndStopsNothing` included
- **AND** the test that verifies this is `TestMetricsPassesTheLifecycleSuite`

### Requirement: A service configuration key in another letter case is refused

A service payload decoded strictly (the service manager's and the metrics service's) SHALL be refused when a key
differs only in letter case from the name of the field it decodes into, in any struct field, pointer or map value the
payload type reaches, with an error naming the key as written and as declared. This SHALL be the same check `config`
applies to its own document. A guard test SHALL fail if `config.Config`, `service.ManagerConfig` or
`service.MetricsConfig` reaches a shape the check does not walk: an embedded struct, a slice or array of structs or
maps, or a type that decodes itself.

#### Scenario: A metrics key in another letter case

- **WHEN** `service.NewMetrics` is given `{"Port":9091}`
- **THEN** it returns an error containing `config key Port must be written port` and no service
- **AND** the test that verifies this is `TestServiceConfigRefusesKeyInAnotherCase`

#### Scenario: A payload type grows a shape the check does not walk

- **WHEN** the guard runs over the three payload types, and over a type with each unwalked shape
- **THEN** it reports nothing for the three, and names each unwalked shape
- **AND** the tests that verify this are `TestPayloadTypesReachOnlyShapesTheLetterCaseCheckWalks` and
  `TestLetterCaseWalkerGapsDetectsEachShape`

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
