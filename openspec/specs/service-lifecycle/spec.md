# service-lifecycle Specification

## Purpose

`service` is the layer every SemEngine process boots: services that embed `BaseService`, the managers that start and
stop them, and the helpers they share. This capability states what `BaseService` promises an embedding service
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
