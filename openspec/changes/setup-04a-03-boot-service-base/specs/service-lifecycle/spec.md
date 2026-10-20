# service-lifecycle

## ADDED Requirements

### Requirement: BaseService's Start bounds Start and Stop is the only stop

`service.BaseService.Start(ctx)` SHALL refuse a nil context, a context that has already ended, a second Start and a
Start after any Stop, and a refused Start SHALL start nothing. A started `BaseService` SHALL report
`StatusRunning`, run its health check once at Start and then every 30 seconds, and SHALL keep doing so when the
context given to Start ends. `Stop(ctx)` SHALL be the only stop: it SHALL refuse a nil context, and SHALL return nil
only after the health goroutine has exited, leaving `StatusStopped` and `IsHealthy()` false. If `ctx` ends first,
Stop SHALL return that context's error, report `StatusStopping`, and a later Stop SHALL wait again. A repeated Stop
after one that returned nil SHALL return nil. `BaseService` SHALL pass the `lifecycle-suite` checks through a
test-side adapter (ADR-113). The package SHALL declare no stop sentinel of its own; `errs.ErrAlreadyStopped` is the
one.

#### Scenario: The Start context ends after Start

- **WHEN** a started `BaseService`'s Start context is cancelled and the clock then advances 60 seconds
- **THEN** its status is still `StatusRunning`, two more health checks have run, and a following Stop returns nil
- **AND** the tests that verify this are `TestStartContextEndingKeepsHealthChecksRunning` and
  `TestBaseServicePassesTheLifecycleSuite`

#### Scenario: Stop's context ends while a health check blocks

- **WHEN** a health check blocks and Stop is called with an ended context
- **THEN** Stop returns `context.Canceled` and the status is `StatusStopping`; a second Stop does not return while
  the check blocks, and returns nil once it returns, with no further check run
- **AND** the test that verifies this is `TestStopAfterAnEndedStopWaitsForTheHealthCheck`

### Requirement: A failed health check makes the service unhealthy

A started `BaseService` SHALL report `IsHealthy()` false and an unhealthy `Health()` when the check set by
`SetHealthCheck` returns an error, or when its NATS client is not healthy. It SHALL report healthy when the check
passes and the client is healthy.

#### Scenario: Failing health check

- **WHEN** a `BaseService` whose health check returns an error is started
- **THEN** after its first check `IsHealthy()` is false and `Health()` reports unhealthy
- **AND** the test that verifies this is `TestFailingHealthCheckMakesTheServiceUnhealthy`

### Requirement: The profiling server stops on Shutdown

`service.MaybeStartPProf(debug, port)` SHALL start nothing and return a nil server and a nil error when `debug` is
false or `port` is not positive, and a nil server's `Shutdown` SHALL return nil. Otherwise it SHALL bind `:port`
before it returns, returning the bind error and starting nothing when the bind fails, and serve
`http.DefaultServeMux`. `Shutdown(ctx)` SHALL refuse a nil context, stop accepting, and return nil only after every
in-flight request has finished and the serving goroutine has exited. If `ctx` ends first, it SHALL return that
error, and a later Shutdown SHALL wait again.

#### Scenario: Disabled

- **WHEN** `MaybeStartPProf(false, 1)` is called
- **THEN** it returns a nil server and a nil error, and that server's Shutdown returns nil
- **AND** the test that verifies this is `TestMaybeStartPProfDisabledStartsNothing`

#### Scenario: Port already bound

- **WHEN** `MaybeStartPProf(true, port)` is called with the port of a listener the test holds
- **THEN** it returns an error and a nil server
- **AND** the test that verifies this is `TestMaybeStartPProfReportsABindFailure`

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
