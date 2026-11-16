<!-- markdownlint-configure-file { "MD013": { "line_length": 120, "heading_line_length": 100 } } -->
# service-lifecycle Delta

## MODIFIED Requirements

### Requirement: BaseService's Stop(ctx) is the only stop

`Stop(ctx)` SHALL be the only stop and SHALL refuse a nil context. Before `Start`, or after a `Stop` joined the health
goroutine, any non-nil `ctx` SHALL return nil. On a started unstopped `BaseService`, any non-nil `ctx` (live or ended)
SHALL cancel the run context so no new health check starts, report `StatusStopping` until exit, wait up to `ctx`, and
return `ctx`'s error if `ctx` ended, else nil with `StatusStopped` and `IsHealthy()` false.

#### Scenario: Stop is given a context that has already ended

- **WHEN** a started `BaseService`'s Stop is called with a cancelled context
- **THEN** Stop cancels the health loop so no further health check runs, returns `context.Canceled`, and a following
  Stop with a live context returns nil with status `StatusStopped`
- **AND** the test that verifies this is `TestStopWithAnEndedContextBeginsCleanupAndReportsContextError`

### Requirement: The service manager's StopAll is the only stop

`StopAll(ctx)` SHALL be the only stop and SHALL refuse a nil context. Before `StartAll` it SHALL return nil if no
health listener started, else stop accepting on it and bound its drain wait by `ctx`. After full release, `StopAll`
SHALL return nil and stop nothing. On a started unstopped manager, any non-nil `ctx` (live or ended) SHALL clear
readiness, initiate teardown across all owned resources, and return `ctx`'s error if `ctx` ends first or had ended.

#### Scenario: StopAll is given a context that has already ended

- **WHEN** a started manager's `StopAll` is called with a cancelled context
- **THEN** `StopAll` calls each started service's `Stop` with that context, stops the health publisher and listeners,
  and returns `context.Canceled`; a following `StopAll` with a live context returns nil
- **AND** the test that verifies this is `TestStopAllWithAnEndedContextBeginsTeardownAndReportsContextError`

### Requirement: StopAll releases services, publisher, listeners and metrics server in order

For any non-nil context on a started manager, `StopAll` SHALL stop every service in reverse registration order, then the
health publisher, the listeners and the metrics server, and return nil only when all are released and `ctx` has not
ended; a listener SHALL stop accepting and `StopAll` SHALL wait up to `ctx` for its in-flight requests.

#### Scenario: Services stop in reverse registration order

- **WHEN** a started manager holding several services is stopped with a live context
- **THEN** the services are stopped in reverse registration order, then the health publisher, the listeners and the
  metrics server, and `StopAll` returns nil once all are released

### Requirement: The component manager's Stop is the only stop and waits for a Start in progress

`Stop(ctx)` SHALL be the only stop and SHALL refuse a nil context. Before `Start` or after full release, any non-nil
`ctx` SHALL return nil. During `Start`, `Stop` SHALL wait up to `ctx` for `Start` to finish. Otherwise any non-nil `ctx`
(live or ended) SHALL deregister provided stores, call `Stop(ctx)` on every started unstopped lifecycle component in
reverse start order, and stop the health publisher and base service, returning nil only once all release and `ctx` is
live.

#### Scenario: Stop during Start

- **WHEN** one component's `Start` is held open and `Stop` is called
- **THEN** `Stop` does not return before that `Start` returns, then stops each component once in reverse order and
  returns nil
- **AND** the test that verifies this is `TestStopDuringStartWaitsForComponentStarts`

#### Scenario: Expired Stop context still stops remaining components

- **WHEN** the last-started component blocks until `Stop`'s context expires while earlier-started components (including
  a `StoreProvider`) are still running
- **THEN** `Stop` still deregisters provided stores and calls `Stop` on every remaining started component in reverse
  start order (initiating each component's cleanup while each started component returns `ctx`'s error), and once the
  blocking component is released a second `Stop` with a live context calls `Stop` only on components whose earlier
  `Stop` returned an error (without re-running any `component.Lifecycle` cleanup that already finished) and returns nil
- **AND** the test that verifies this is `TestComponentManagerStopTimeoutStillStopsRemainingComponents`

### Requirement: A component whose Stop failed is stopped again

A component whose `Stop` returned an error SHALL be stopped again by the next `Stop`; one whose `Stop` returned nil
SHALL not be called again.

#### Scenario: A failed Stop is retried

- **WHEN** a started manager's `Stop` returns an error because one component's `Stop` failed
- **THEN** the next `Stop` calls only that component's `Stop` again, and returns nil once it succeeds
- **AND** the tests that verify this are `TestStopRetriesOnlyComponentsThatDidNotStop` and
  `TestPropComponentManagerStopHistory`

### Requirement: The message logger's Stop drains and releases what it holds

`Stop(ctx)` SHALL refuse a nil context. For any non-nil `ctx` (live or ended), it SHALL fence new KV watch streams and,
when started and unstopped, end the subscribe retry and wait up to `ctx` for it, initiate drain on each subscription so
a message being handled when a live `Stop` begins is stored, cancel the run context, end open KV watch streams and wait
for their watchers, and stop the base service, returning nil only once all release and `ctx` has not ended.

#### Scenario: Stop stores the message being handled

- **WHEN** `Stop` begins while one message's handler is still running
- **THEN** `Stop` returns nil and that message and the one published after it are stored
- **AND** the test that verifies this is `TestMessageLoggerStopDrainsInFlightMessage`

#### Scenario: Stop ends open watch streams

- **WHEN** `Stop` runs while a KV watch stream is open
- **THEN** `Stop` returns nil, the stream's body reaches its end and no stream is left open
- **AND** the test that verifies this is `TestMessageLoggerStopEndsOpenWatchStreams`

#### Scenario: Ended Stop context fences watches and cancels loops

- **WHEN** `Stop` is called with an already-cancelled context while a subscription drain does not complete immediately
- **THEN** `Stop` fences new watch streams, cancels the run context, ends open watch streams, initiates base-service
  stop, and returns `context.Canceled`; a subsequent `Stop` with a live context returns nil
- **AND** the test that verifies this is `TestMessageLoggerStopWithEndedContextInitiatesTeardown`

Not covered by a test, and reviewed only:

- a `Stop` that must wait for a subscribe retry still running (`TestMessageLoggerRetriesSubscribeUntilConnected` stops
  the logger after the retry has ended, so it cannot tell a joined retry from an unjoined one).

### Requirement: Storage observability's Stop is the only stop and can be retried

`Stop(ctx)` SHALL refuse a nil context, refuse a concurrent `Stop` as transient, and return nil before `Start` or after
a `Stop` joined everything. On a started unstopped service, any non-nil `ctx` (live or ended) SHALL end both loops,
wait up to `ctx` for them, and initiate base-service stop, returning nil only once all three finish and `ctx` has not
ended; if `ctx` ended, `Stop` SHALL return `ctx`'s error and a later `Stop` SHALL wait again for anything unjoined.

#### Scenario: A failed Stop is retried

- **WHEN** the first `Stop` returns its context's error while the collection loop is held inside a log call
- **THEN** a second `Stop` has not returned while the loop is held, and after release the second and third `Stop`
  return nil
- **AND** the test that verifies this is `TestStorageObservabilityFailedStopIsRetried`

#### Scenario: A collection already inside the account listing

- **WHEN** `Stop` begins while a collection is already inside the account listing
- **THEN** how soon that collection sees `Stop` begin is not promised: it depends on nats.go

Not covered by a test, and reviewed only: a collection already inside the JetStream account listing when `Stop` begins
(no test holds a loop inside the listing), and the report bucket's lock, which `lazyReportBucket.resolve` holds while
the bucket is created or reconciled, so a slow server holds a second caller behind the first.

### Requirement: The heartbeat's Stop is the only stop

`Stop(ctx)` SHALL refuse a nil context, refuse a concurrent `Stop` as transient, and return nil before `Start` or after
a `Stop` joined everything. On a started unstopped service, any non-nil `ctx` (live or ended) SHALL end the loop, wait
up to `ctx` for it, and initiate base-service stop, returning nil only once both finish and `ctx` has not ended; if
`ctx` ended, `Stop` SHALL return `ctx`'s error and a later `Stop` SHALL wait again for anything unjoined.

#### Scenario: A failed Stop is retried

- **WHEN** the first `Stop` returns its context's error while the loop is held inside a log call
- **THEN** a second `Stop` has not returned while the loop is held, and after release a later `Stop` returns nil
- **AND** the test that verifies this is `TestHeartbeatFailedStopIsRetried`

### Requirement: The log forwarder's Stop is the base service's

`LogForwarder.Stop(ctx)` SHALL be the base service's: it SHALL refuse a nil context, initiate base-service teardown for
any non-nil context on a started forwarder (returning `ctx`'s error when `ctx` ends first or had already ended, and nil
once the base service has stopped under a live context), and return nil on any subsequent `Stop` once stopped.

#### Scenario: Stop under a nil, an ended and a live context

- **WHEN** a started log forwarder's `Stop` is called with a nil context, then an ended one, then a live one twice
- **THEN** the nil context is refused, the ended one begins teardown and returns its error, and both live calls return
  nil
