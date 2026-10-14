# nats-fixture Specification

## Purpose
The NATS fixture gives one test one disposable, real NATS server in a container, with typed start failures, a
label on every container it starts so leftovers can be found and removed, and a cleanup that returns success only
after it has observed every owned resource gone. `natsfixture.Open` lets a package's tests open a value connected to
that server, such as a client, and closes it under a deadline before the server stops.

## Requirements

### Requirement: Phased start with typed failure

Start SHALL proceed image → create and start → internal ready log → host → mapped client port → connect → JetStream
ready, deriving every operation context from the caller's. A failure SHALL return an error that names the phase, the
container ID when a container exists, and the cause, and that matches the caller's context error under `errors.Is`
whenever that context had ended. Before returning it SHALL remove any container the attempt created, under a fresh
bounded context, and report a removal failure in the error, and it SHALL write that container's log to the test
output. The create and start phase of each attempt SHALL publish the client port on a host port the fixture took from
the operating system with a port-0 listener and closed, and SHALL NOT leave the host port to Docker to choose; a
failure to take a port SHALL fail that phase. At most one replacement container is permitted, only after the
mapped-port phase failed with the caller's context live and the first container removed. The fixture SHALL NOT expose
start, connect or bucket-prefix knobs.

#### Scenario: Failure after the container exists

- **WHEN** connect fails after the container is running
- **THEN** the error names the connect phase and the container ID, the container is gone, and its log is in the test
  output

#### Scenario: Cancellation during start

- **WHEN** the test context is cancelled while the container is starting
- **THEN** Start returns an error satisfying `errors.Is(err, context.Canceled)`, no replacement is attempted, and no
  container remains

#### Scenario: Ports after a running fixture's own are held

- **WHEN** a fixture is running and the host ports after its own are held by listeners on all addresses
- **THEN** a second fixture's Start returns nil, with JetStream answering on its `URL()`

#### Scenario: One replacement after a missing mapped port

- **WHEN** the mapped-port read fails once with the caller's context live
- **THEN** the first container is removed, a second one starts and Start succeeds; if the second read fails too, the
  error names the mapped-port phase and no third container starts

### Requirement: Checked cleanup

Stop(ctx) SHALL, under the caller's context and in order, wait for in-flight publishes, delete owned consumers,
streams, and buckets while observing their absence, drain and close the connection, terminate the container, and
observe it absent; a Stop returning nil SHALL have observed every owned resource absent. When ctx ends first, Stop
SHALL return ctx.Err(), retain the unresolved handles, and a later Stop SHALL retry them. Repeated Stop after success
SHALL be a no-op. Start after Start SHALL be refused. The fixture SHALL retain no context.Context. Every consumer's
delivery SHALL be stopped, its running handlers joined, and only then their context cancelled, even when the
connection is already closed; only the server-side deletes are then skipped, and those resources are released with
the container observed absent. Once Stop begins on owned resources the fixture SHALL refuse every new stream, bucket,
and consumer. Start, Stop, and resource creation SHALL wait for one another only under their own context.

#### Scenario: Blocked callback delays finalisation

- **WHEN** a consumer callback is blocked during Stop
- **THEN** the callback's context is live until released, the join completes before Stop returns, and only then are
  the consumer, stream, connection, and container removed

#### Scenario: Closed connection still joins handlers

- **WHEN** the fixture's connection is closed while a consumer callback is blocked, and Stop runs
- **THEN** Stop stops delivery and waits for the callback; the callback's context is live until it is released and
  cancelled before Stop returns nil, and the container is observed absent

#### Scenario: No creation once Stop begins

- **WHEN** CreateStream, CreateKeyValue, or Consume is called after Stop has begun, or is already waiting for the
  fixture when Stop begins
- **THEN** it returns an error at once, creates nothing, and Stop's nil return still means nothing is owned

#### Scenario: Stop waits for a running Start only under its own context

- **WHEN** Stop is called with a short context while Start is parked in a Docker call
- **THEN** Stop returns context.DeadlineExceeded when its context ends

#### Scenario: Deadline is not a join

- **WHEN** Stop's context expires while a callback is blocked
- **THEN** Stop returns context.DeadlineExceeded, the container still exists, remaining() lists the handles, and a
  later bounded Stop completes with everything absent

### Requirement: Memory-backed owned stream

`CreateMemoryStream(ctx, name, subjects...)` SHALL create a stream the fixture owns with memory storage and the same
MaxAge, MaxBytes and DiscardOld bounds as `CreateStream`; Stop SHALL delete it or observe it absent.

#### Scenario: Memory stream is owned and bounded

- **WHEN** a test creates a memory-backed stream through the fixture
- **THEN** the stream's configuration reports memory storage and the fixture's bounds, and Stop observes it gone

### Requirement: Fault-injecting key-value double

The fixture SHALL provide a `jetstream.KeyValue` wrapper over a real bucket whose `Put`, `Create`, `Update` and
`Delete` can be made to fail before the real call (no effect) or after it (the real effect stands, the caller sees
the injected error), and SHALL report how many real calls each operation made. The wrapper SHALL be typed on
`jetstream.KeyValue` only.

#### Scenario: Fail after a real update

- **WHEN** `Update` is set to fail after the call and a caller updates a key at its current revision
- **THEN** the caller receives the injected error, a fresh read of the key returns the new value at the next
  revision, and the call count for Update is one

#### Scenario: Fail before a real create

- **WHEN** `Create` is set to fail before the call and a caller creates a key
- **THEN** the caller receives the injected error, the key does not exist, and the call count for Create is zero

### Requirement: Fixture satisfies the lifecycle floor

The fixture SHALL pass the lifecycle suite, including the failed-start check, with a `mustFail` factory whose start
hook returns an error.

#### Scenario: Start hook fails

- **WHEN** the suite starts a fixture whose start hook returns an error
- **THEN** Start returns a `FixtureError`, the fixture reports nothing unresolved, and a following Stop returns nil
  without a Docker call

### Requirement: Broker max payload is settable

A test SHALL be able to start the fixture with a broker `max_payload` it chooses; with none set, the broker default
SHALL apply.

#### Scenario: Publish above the set limit

- **GIVEN** a fixture started with max payload N
- **WHEN** a client publishes more than N bytes
- **THEN** the publish is refused with `nats.ErrMaxPayload` (nats.go checks the limit the broker announced before it
  sends), and a message of at most N bytes is accepted

### Requirement: Connected value for a package's tests

`natsfixture` SHALL provide one helper that, given a test, a started fixture and an open function supplied by the
caller (called with a context and the fixture's URL, and returning the opened value and a close function that takes a
context), calls the open function under a bounded context and returns the value. When the open function returns an
error, the helper SHALL fail the test with a message naming the fixture's URL and that error. On success it SHALL
register the close function on the test's cleanup so that it runs before the fixture's own Stop, under a context with
a deadline, and SHALL report a close error as a test error. The helper SHALL import no package of this module outside
`internal/harness`, SHALL start no goroutine, and SHALL hold nothing after it returns beyond the cleanup it
registered.

#### Scenario: Opening fails

- **WHEN** the open function returns an error
- **THEN** the test fails with a message naming the fixture's URL and the error, and no close is registered

#### Scenario: Close runs first, bounded

- **WHEN** a test that opened a value through the helper ends
- **THEN** the close function runs before the fixture stops, with a context that has a deadline, and a close error is
  reported as a test error

#### Scenario: Import bound

- **WHEN** the harness import check lists the helper's package imports
- **THEN** no package of this module outside `internal/harness` is among them

### Requirement: Restart keeps the container and its host port

`Fixture.Restart(ctx)` SHALL stop and start the same container under the fixture's one-slot semaphore, SHALL first
stop every consumer the fixture created (cancelling it and waiting for its handler to be idle, as Stop does) and
record it ended, SHALL drain the fixture's own connection, SHALL run the stop-container, start-container,
mapped-port, connect and JetStream-ready phases in that order deriving every operation context from the caller's,
and on success SHALL make `URL()` and `JetStream()` return the new binding. The container SHALL keep the host port
Start gave it, so `URL()` after a successful Restart SHALL equal `URL()` before it. Streams and buckets the fixture
owns SHALL stay in its ownership record, and Stop SHALL treat one that no longer exists after the restart as observed
absent. Restart SHALL return an error and make no Docker call before Start has succeeded or once Stop has begun. A
phase failure SHALL return a `FixtureError` carrying the phase, with one exception: when the mapped-port phase fails
and the caller's context is still live, Restart SHALL run the stop-container and start-container phases once more on
the same container and read the mapped port again, and SHALL return a second failure as a `FixtureError`. The
fixture SHALL NOT create a replacement container during Restart. The container's writable layer SHALL survive a
Restart, so a file-backed stream keeps its messages and a memory-backed stream does not.

#### Scenario: File stream survives, memory stream does not

- **WHEN** one message is acknowledged on a file-backed stream and one on a memory-backed stream, and the fixture is
  restarted
- **THEN** the file-backed message is readable at its sequence through the new `JetStream()` and the memory-backed
  stream reports the message or the stream absent

#### Scenario: Binding kept across a restart

- **WHEN** the fixture is restarted
- **THEN** `URL()` returns the address it returned before the restart, that address is the container's binding as
  Docker reports it, and a dial to it succeeds

#### Scenario: Consumer across a restart

- **WHEN** a consumer created by Consume has a handler running and the fixture is restarted
- **THEN** Restart returns only after that handler has returned, the consumer is recorded ended, no handler runs
  after the restart until Consume is called again, and Stop still succeeds

#### Scenario: Restart before Start

- **WHEN** Restart is called on a fixture whose Start has not succeeded, or once Stop has begun
- **THEN** Restart returns an error and the Docker call count is unchanged

#### Scenario: Start-container phase fails

- **WHEN** the start-container hook returns an error
- **THEN** Restart returns a `FixtureError` whose phase is start-container, no second container exists, and Stop
  still observes the container gone

#### Scenario: Mapped-port failure after a restart

- **WHEN** the mapped-port read after the restart fails and the caller's context is still live
- **THEN** Restart stops and starts the same container once more and reads the port again; if that read succeeds,
  Restart goes on to connect, and if it fails, Restart returns a `FixtureError` whose phase is mapped-port, with no
  third try and no second container

### Requirement: Ryuk off, labelled containers and the sweep

Once per process, before its first testcontainers call, natsfixture SHALL set `TESTCONTAINERS_RYUK_DISABLED=true` in
the process environment, whatever it held before. Every container natsfixture starts SHALL carry the label
`org.c360studio.semengine.test`, valued with the test process's package and process ID. In the same once-per-process
step, natsfixture SHALL remove, with force, every container carrying that label that Docker created more than 30
minutes earlier, running or not. It SHALL choose containers by that label and Docker's creation time only, never by
name or by whether a process is alive. A removal error SHALL NOT fail the test: a container already gone is ignored,
and any other error is written to the test output. That natsfixture overrides a value the caller set, rather than
only an unset one, is review only: testcontainers reads its configuration once per process, so a test of it would
depend on which test reached that step first.

#### Scenario: Ryuk is off

- **WHEN** natsfixture has started its first container in a process
- **THEN** `testcontainers.GenericLabels()` has no `org.testcontainers.reap` key

#### Scenario: Only old labelled containers are swept

- **WHEN** the sweep runs over a label value the test chose, first with an age of one hour and then with an age of
  zero, while the test also runs an unlabelled container
- **THEN** the labelled container survives the first sweep and is gone after the second, and the unlabelled one is
  still running
