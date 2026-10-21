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

