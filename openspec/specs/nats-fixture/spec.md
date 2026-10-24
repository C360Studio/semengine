# nats-fixture Specification

## Purpose
The NATS fixture runs a real NATS server in a container for integration tests: either one shared server per package
(`natsfixture.Shared`, started on the first `Reset` and emptied between tests) or one server for a test that needs its
own (`natsfixture.New`), with start failures that name the phase, a label on every container it starts so leftovers can
be found and removed, and a `Stop` that removes the container. `natsfixture.Open` lets a package's tests open a value
connected to a server URL, such as a client, and closes it under a deadline before the connection check or server stop.

## Requirements

### Requirement: Own server per test

`natsfixture.New(t, opts...)` SHALL give one test its own NATS server: Start starts it, Restart and Stop act on it,
and `URL()` and `JetStream()` reach it. New SHALL make no Docker call. A second Start SHALL return `ErrAlreadyUsed`.
The fixture's methods are called from the test's own goroutine. The fixture SHALL retain no `context.Context`.

#### Scenario: A second Start is refused

- **WHEN** Start is called on a fixture whose Start already succeeded
- **THEN** it returns `ErrAlreadyUsed`

### Requirement: Own server cleanup

`natsfixture.New` SHALL register, as the test's cleanup, a Stop under a fresh 60-second context whose failure is
reported as a test error. Stop SHALL close the fixture's connection and remove the container, and SHALL return nil only
when Docker reported the removal; a Stop after a successful Stop SHALL return nil and make no Docker call.

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
whenever that context had ended.

#### Scenario: Failure after the container exists

- **WHEN** connect fails after the container is running
- **THEN** the error names the connect phase and the container ID, the container is gone, and its log is in the test
  output

#### Scenario: Cancellation during start

- **WHEN** the test context is cancelled while the container is starting
- **THEN** Start returns an error satisfying `errors.Is(err, context.Canceled)`, no other container is started, and no
  container remains

### Requirement: Start cleans up after a failure

Before returning a failure, Start SHALL remove any container it created, under a fresh bounded context, and report a
removal failure in the error, and it SHALL write that container's log to the test output.

#### Scenario: Container removed and log kept

- **WHEN** a phase fails after the container exists
- **THEN** the container is gone before Start returns, a removal failure would appear in the error, and the
  container's log is in the test output

### Requirement: Start takes its host port from the operating system

The create and start phase SHALL run the once-per-process Docker preparation before taking its host port, SHALL
publish the client port on a host port the fixture took from the operating system with a port-0 listener and closed,
and SHALL NOT leave the host port to Docker to choose; a failure to take a port SHALL fail that phase.

#### Scenario: Explicit host-port binding alongside held ephemeral ports

- **WHEN** ephemeral host ports are held by listeners on all addresses and two fixtures start
- **THEN** both fixtures' Start calls return nil, Docker's inspect for each container reports its `URL()` port as the
  explicit `HostPort` binding on `4222/tcp`, and JetStream answers on each `URL()`

### Requirement: Start retries a taken host port once

When Docker fails to start the container with "address already in use" and the caller's context is still live, the
create and start phase SHALL remove that unstarted container under a fresh bounded context, take a new host port, and
try to create and start the container once more (at most two create-and-start attempts total), returning a second
failure with both causes joined. After a failure in any other phase, or any other create-and-start failure, Start SHALL
NOT start another container.

#### Scenario: Host port taken before Docker binds it

- **WHEN** another listener holds the first host port the create and start phase picked while the caller's context is
  live
- **THEN** Start removes the first unstarted container, takes a new host port, starts a second container, and returns
  nil with only the second container present; when both attempts hit a held port, Start fails in the start phase after
  two attempts and leaves no container behind

### Requirement: Start explains a failed mapped-port read

A failed mapped-port read SHALL say what an inspect of the container showed, or why the inspect failed.

#### Scenario: A missing mapped port fails Start

- **WHEN** the mapped-port read fails
- **THEN** Start returns an error naming the mapped-port phase and what the inspect of the container showed, the
  container is gone, and no second container starts

### Requirement: No start knobs

The fixture SHALL NOT expose start, connect or bucket-prefix knobs.

#### Scenario: No knob to set

- **WHEN** a test looks for an option to change how the fixture starts, connects or prefixes buckets
- **THEN** the fixture exports none

### Requirement: Restart keeps the container

`Fixture.Restart(ctx)` SHALL stop and start the same container, drain the fixture's own connection, and run the
stop-container, start-container, mapped-port, connect and JetStream-ready phases in that order, deriving every operation
context from the caller's; on success `URL()` and `JetStream()` SHALL return the new binding.

#### Scenario: Restart after Start

- **WHEN** Restart is called on a fixture whose Start succeeded
- **THEN** Restart returns nil and `URL()` and `JetStream()` reach the same container through its binding after the
  restart

### Requirement: Restart keeps the host port

The container SHALL keep the host port Start gave it, so `URL()` after a successful Restart SHALL equal `URL()` before
it. Docker does not promise that a container keeps an explicit host-port binding across a stop and start: one probe on
macOS (Docker Desktop 29.8.2, testcontainers v0.44.0, PR #210 comment 6151736554) showed it kept, and the binding test
checks it on every run.

#### Scenario: Binding kept across a restart

- **WHEN** the fixture is restarted
- **THEN** `URL()` returns the address it returned before the restart, that address is the container's binding as
  Docker reports it, and a dial to it succeeds

### Requirement: Restart refuses outside a running fixture

Restart SHALL return an error and start nothing before Start has succeeded or once Stop has begun.

#### Scenario: Restart before Start

- **WHEN** Restart is called on a fixture whose Start has not succeeded, or once Stop has begun
- **THEN** Restart returns an error and starts no container

### Requirement: Restart fails without retrying

A phase failure SHALL return an error naming the phase; Restart SHALL NOT stop, start or read the port a second time,
and SHALL NOT create a replacement container.

#### Scenario: A missing mapped port fails Restart

- **WHEN** the mapped-port read after the restart fails
- **THEN** Restart returns an error naming the mapped-port phase and what the inspect of the container showed, with
  no second stop, start or port read and no second container

### Requirement: Restart keeps the writable layer

The container's writable layer SHALL survive a Restart, so a file-backed stream keeps its messages and a memory-backed
stream does not.

#### Scenario: File stream survives, memory stream does not

- **WHEN** one message is acknowledged on a file-backed stream and one on a memory-backed stream, and the fixture is
  restarted
- **THEN** the file-backed message is readable at its sequence through the new `JetStream()` and the memory-backed
  stream reports the message or the stream absent

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

`natsfixture` SHALL provide one helper that, given a test, a server URL and an open function supplied by the caller
(called with a context and the URL, and returning the opened value and a close function that takes a context), calls
the open function under a bounded context and returns the value. When the open function returns an error, the helper
SHALL fail the test with a message naming the URL and that error.

#### Scenario: Opening fails

- **WHEN** the open function returns an error
- **THEN** the test fails with a message naming the URL and the error, and no close is registered

### Requirement: Connected value is closed under a deadline

On success the helper SHALL register the close function on the test's cleanup, under a context with a deadline, and
SHALL report a close error as a test error; registered after the reset or after `New`, it runs before the reset's
connection check and before an own server's Stop.

#### Scenario: Close runs first, bounded

- **WHEN** a test that opened a value through the helper ends
- **THEN** the close function runs before the connection check or the server's Stop, with a context that has a
  deadline, and a close error is reported as a test error

### Requirement: Connected value helper holds nothing

The helper SHALL import no package of this module outside `internal/harness`, SHALL start no goroutine, and SHALL hold
nothing after it returns beyond the cleanup it registered.

#### Scenario: Import bound

- **WHEN** the harness import check lists the helper's package imports
- **THEN** no package of this module outside `internal/harness` is among them

### Requirement: Shared server per package

`natsfixture` SHALL provide a server shared by a package's tests. It SHALL start at the first reset in the process,
as Start does (the phases, one container, the container log on failure), under a context bounded at
five minutes so that a cold image pull fits (the removed runner's pull budget was 300 s), and SHALL close the
connection its start made before the reset counts connections. A process that never resets starts no container.

#### Scenario: A process that never resets starts nothing

- **WHEN** a prochost child, or a run whose `-run` selects no test that resets, runs the package's `TestMain`
- **THEN** no container is started and Stop returns nil

### Requirement: A failed shared start fails every reset

When the shared server's start fails, the reset SHALL fail its test with the start error, and every later reset in the
process SHALL fail at once with the same error.

#### Scenario: A failed start fails later resets at once

- **WHEN** the server's start fails at the first reset
- **THEN** that test fails with the start error, and the next reset fails with the same error without a Docker call

### Requirement: Shared server stops after the package

A package's `TestMain` SHALL stop the server after `m.Run`; Stop SHALL remove the container when the server started,
and SHALL return nil without a Docker call when it did not.

#### Scenario: Container gone after the package

- **WHEN** the package's tests have run and `TestMain` has stopped the server
- **THEN** Docker lists no container with the server's container ID

### Requirement: Shared server URL comes from the reset

The shared server SHALL return its URL only from the reset, and SHALL retain no `context.Context`.

#### Scenario: URL only through the reset

- **WHEN** a test needs the shared server's URL
- **THEN** it gets it from the reset's return value, and the server holds no `context.Context`

### Requirement: Reset to a verified-empty server

The per-test reset SHALL, under a bounded context derived from the test's, open its own connection and wait until the
server's account statistics (`$SYS.REQ.ACCOUNT.PING.STATZ`) report one client connection, the reset's own, failing the
test, when the wait ends, with a message that a connection from an earlier test is still open.

#### Scenario: A reset while a connection is open fails

- **WHEN** a connection from an earlier test is still open when a test resets
- **THEN** the reset fails the test, saying a connection from an earlier test is still open

### Requirement: Reset empties the server

After the connection wait, the reset SHALL delete every stream, which removes key-value buckets, object stores and
their consumers; fail the test unless `AccountInfo` then reports zero streams, zero consumers, zero memory bytes and
zero storage bytes; close its connection; and return the server's URL.

#### Scenario: State does not cross tests

- **WHEN** test A creates a stream, a key-value bucket, an object store and a durable consumer, and test B then resets
- **THEN** B's reset passes, and the account reports zero streams, consumers, memory bytes and storage bytes

#### Scenario: Fixed names in turn

- **WHEN** two tests in turn each reset and create the stream `ENTITY`
- **THEN** the second finds no message the first published

### Requirement: Reset registers a leak check

The reset SHALL register a cleanup that runs after the test's own cleanups and fails the test, naming the count, while
the account reports a client connection besides the one asking after a bounded wait.

#### Scenario: A leaked connection fails its own test

- **WHEN** test A leaves a client connection open
- **THEN** test A fails naming one client connection

### Requirement: Ryuk off, labelled containers and the sweep

Once per process, before its first testcontainers call, natsfixture SHALL set `TESTCONTAINERS_RYUK_DISABLED=true` in
the process environment, whatever it held before. That natsfixture overrides a value the caller set, rather than
only an unset one, is review only: testcontainers reads its configuration once per process, so a test of it would
depend on which test reached that step first.

#### Scenario: Ryuk is off

- **WHEN** natsfixture has started its first container in a process
- **THEN** `testcontainers.GenericLabels()` has no `org.testcontainers.reap` key

### Requirement: Containers carry a label

Every container natsfixture starts SHALL carry the label `org.c360studio.semengine.test`, valued with the test
process's package and process ID.

#### Scenario: Label on a started container

- **WHEN** natsfixture starts a container
- **THEN** the container carries the label `org.c360studio.semengine.test`, valued with the test process's package and
  process ID

### Requirement: Sweep of old labelled containers

In the once-per-process step that sets Ryuk off, natsfixture SHALL remove, with force, every container carrying the
label `org.c360studio.semengine.test` that Docker created more than 30 minutes earlier, running or not. It SHALL
choose containers by that label and Docker's creation time only, never by name or by whether a process is alive.

#### Scenario: Only old labelled containers are swept

- **WHEN** the sweep runs over a label value the test chose, first with an age of one hour and then with an age of
  zero, while the test also runs an unlabelled container
- **THEN** the labelled container survives the first sweep and is gone after the second, and the unlabelled one is
  still running

### Requirement: Sweep errors do not fail the test

A removal error SHALL NOT fail the test: a container already gone is ignored, and any other error is written to the
test output.

#### Scenario: Removal error

- **WHEN** the sweep cannot remove a container
- **THEN** the test does not fail; a container already gone is ignored and any other error is in the test output

### Requirement: Bounded test streams

`natsfixture.CreateStream` SHALL be a function, not a fixture method, that creates a stream on a given JetStream with
the storage type the caller passes (file or memory), declaring `MaxAge`, `MaxBytes` and `DiscardOld`, so a runaway test
fails on a ceiling instead of filling the server.

#### Scenario: Memory stream is bounded

- **WHEN** a test creates a memory-backed stream through the function
- **THEN** the stream's configuration reports memory storage and the bounds
