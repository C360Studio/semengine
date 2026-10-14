# nats-fixture Specification

## Purpose
The NATS fixture gives one test one disposable, real NATS server in a container, with start failures that name the
phase, a label on every container it starts so leftovers can be found and removed, and a Stop that removes the
container, which removes every stream, bucket and consumer in it. `natsfixture.Open` lets a package's tests open a
value connected to that server, such as a client, and closes it under a deadline before the server stops.

## Requirements

### Requirement: Own server per test

`natsfixture.New(t, opts...)` SHALL give one test its own NATS server: Start starts it, Restart and Stop act on it,
and `URL()` and `JetStream()` reach it. New SHALL make no Docker call and SHALL register, as the test's cleanup, a Stop
under a fresh 60-second context whose failure is reported as a test error. Stop SHALL close the fixture's connection
and remove the container, and SHALL return nil only when Docker reported the removal; a Stop after a successful Stop
SHALL return nil and make no Docker call. A second Start SHALL return `ErrAlreadyUsed`. The fixture's methods are
called from the test's own goroutine. The fixture SHALL retain no `context.Context`.

#### Scenario: Container gone after the test

- **WHEN** a test that started its own server ends
- **THEN** Docker lists no container with that server's container ID

#### Scenario: Repeated Stop

- **WHEN** Stop is called again after it succeeded
- **THEN** it returns nil and makes no Docker call

### Requirement: Phased start in one container

Start SHALL proceed image → create and start → internal ready log → host → mapped client port → connect → JetStream
ready, deriving every operation context from the caller's. A failure SHALL return an error that names the phase, the
container ID when a container exists, and the cause, and that matches the caller's context error under `errors.Is`
whenever that context had ended. Before returning it SHALL remove any container Start created, under a fresh bounded
context, and report a removal failure in the error, and it SHALL write that container's log to the test output. The
create and start phase SHALL publish the client port on a host port the fixture took from the operating system with a
port-0 listener and closed, and SHALL NOT leave the host port to Docker to choose; a failure to take a port SHALL fail
that phase. Start SHALL start at most one container: after a failure in any phase it SHALL NOT start another. A failed
mapped-port read SHALL say what an inspect of the container showed, or why the inspect failed. The fixture SHALL NOT
expose start, connect or bucket-prefix knobs.

#### Scenario: Failure after the container exists

- **WHEN** connect fails after the container is running
- **THEN** the error names the connect phase and the container ID, the container is gone, and its log is in the test
  output

#### Scenario: Cancellation during start

- **WHEN** the test context is cancelled while the container is starting
- **THEN** Start returns an error satisfying `errors.Is(err, context.Canceled)`, no other container is started, and no
  container remains

#### Scenario: Ports after a running fixture's own are held

- **WHEN** a fixture is running and the host ports after its own are held by listeners on all addresses
- **THEN** a second fixture's Start returns nil, with JetStream answering on its `URL()`

#### Scenario: A missing mapped port fails Start

- **WHEN** the mapped-port read fails
- **THEN** Start returns an error naming the mapped-port phase and what the inspect of the container showed, the
  container is gone, and no second container starts

### Requirement: Restart keeps the container

`Fixture.Restart(ctx)` SHALL stop and start the same container, drain the fixture's own connection, and run the
stop-container, start-container, mapped-port, connect and JetStream-ready phases in that order, deriving every operation
context from the caller's; on success `URL()` and `JetStream()` SHALL return the new binding. The container SHALL keep
the host port Start gave it, so `URL()` after a successful Restart SHALL equal `URL()` before it. Docker does not
promise that a container keeps an explicit host-port binding across a stop and start: one probe on macOS (Docker Desktop
29.8.2, testcontainers v0.44.0, PR #210 comment 6151736554) showed it kept, and the binding test checks it on every run.
Restart SHALL return an error and start nothing before Start has succeeded or once Stop has begun. A phase failure
SHALL return an error naming the phase; Restart SHALL NOT stop, start or read the port a second time, and SHALL NOT
create a replacement container. The container's writable layer SHALL survive a Restart, so a file-backed stream keeps
its messages and a memory-backed stream does not.

#### Scenario: File stream survives, memory stream does not

- **WHEN** one message is acknowledged on a file-backed stream and one on a memory-backed stream, and the fixture is
  restarted
- **THEN** the file-backed message is readable at its sequence through the new `JetStream()` and the memory-backed
  stream reports the message or the stream absent

#### Scenario: Binding kept across a restart

- **WHEN** the fixture is restarted
- **THEN** `URL()` returns the address it returned before the restart, that address is the container's binding as
  Docker reports it, and a dial to it succeeds

#### Scenario: Restart before Start

- **WHEN** Restart is called on a fixture whose Start has not succeeded, or once Stop has begun
- **THEN** Restart returns an error and starts no container

#### Scenario: A missing mapped port fails Restart

- **WHEN** the mapped-port read after the restart fails
- **THEN** Restart returns an error naming the mapped-port phase and what the inspect of the container showed, with
  no second stop, start or port read and no second container

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

### Requirement: Bounded test streams

`natsfixture.CreateStream` SHALL be a function, not a fixture method, that creates a stream on a given JetStream with
the storage type the caller passes (file or memory), declaring `MaxAge`, `MaxBytes` and `DiscardOld`, so a runaway test
fails on a ceiling instead of filling the server.

#### Scenario: Memory stream is bounded

- **WHEN** a test creates a memory-backed stream through the function
- **THEN** the stream's configuration reports memory storage and the bounds
