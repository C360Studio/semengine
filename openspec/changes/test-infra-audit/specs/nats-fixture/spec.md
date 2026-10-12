# nats-fixture

## REMOVED Requirements

### Requirement: Admission before Docker

**Reason**: The host lock it checked is removed (`integration-test-runner`, owner ruling #205 comment 6132580576). It
made `go test -tags integration` refuse to run (#129), and its owner-liveness rule drifted from the runner's (#124).

**Migration**: None. `ErrNotAdmitted` is removed; nothing outside natsfixture read it.

### Requirement: Run-unique ownership and safe names

**Reason**: Its names guarded against SemStreams' name-substring cleanup, which destroys volumes, and the fixture
creates none (setup-02 premise P11). Its ownership record served the teardown removed below. Isolation on a shared
server is the reset's job ("Reset to a verified-empty server").

**Migration**: `Fixture.Name` callers (`natsclient/integration_test.go:112`,
`processor/graph-ingest/query_verbs_integration_test.go:82`) use literal names. Docker names the container.

### Requirement: Checked cleanup

**Reason**: Removing the container removes every stream, bucket and consumer in it, so deleting them first, and
observing them absent, repeats what the removal does. The consumer joins served `Consume`, which no test outside
natsfixture uses.

**Migration**: Replaced by "Own server per test" (Stop removes the container) and, for shared servers, by the reset.

### Requirement: Memory-backed owned stream

**Reason**: Nothing is owned now; the bounds move to "Bounded test streams".

**Migration**: `CreateMemoryStream` callers call the stream function with memory storage.

### Requirement: Fixture satisfies the lifecycle floor

**Reason**: The fixture is not a component; product components run the lifecycle suite themselves
(`internal/harness/lifecycletest`).

**Migration**: None.

## MODIFIED Requirements

### Requirement: Phased start with typed failure

Start SHALL proceed image → create and start → internal ready log → host → mapped client port → connect → JetStream
ready, deriving every operation context from the caller's. A failure SHALL return an error that names the phase, the
container ID when a container exists, and the cause, and that matches the caller's context error under `errors.Is`
whenever that context had ended. Before returning it SHALL remove any container the attempt created, under a fresh
bounded context, and report a removal failure in the error, and it SHALL write that container's log to the test
output. At most one replacement container is permitted, only after the mapped-port phase failed with the caller's
context live and the first container removed. The fixture SHALL NOT expose start, connect or bucket-prefix knobs.

#### Scenario: Failure after the container exists

- **WHEN** connect fails after the container is running
- **THEN** the error names the connect phase and the container ID, the container is gone, and its log is in the test
  output

#### Scenario: Cancellation during start

- **WHEN** the test context is cancelled while the container is starting
- **THEN** Start returns an error satisfying `errors.Is(err, context.Canceled)`, no replacement is attempted, and no
  container remains

#### Scenario: One replacement after a missing mapped port

- **WHEN** the mapped-port read fails once with the caller's context live
- **THEN** the first container is removed, a second one starts and Start succeeds; if the second read fails too, the
  error names the mapped-port phase and no third container starts

### Requirement: Restart keeps the container and re-reads its binding

`Fixture.Restart(ctx)` SHALL stop and start the same container, drain the fixture's own connection, and run the
stop-container, start-container, mapped-port, connect and JetStream-ready phases in that order, deriving every
operation context from the caller's; on success `URL()` and `JetStream()` SHALL return the new binding. Restart SHALL
return an error and start nothing before Start has succeeded or once Stop has begun. A phase failure SHALL return an
error naming the phase, with one exception: when the mapped-port phase fails and the caller's context is still live,
Restart SHALL stop and start the same container once more and read the mapped port again, and SHALL return a second
failure as an error naming the mapped-port phase. The fixture SHALL NOT create a replacement container during Restart.
The container's writable layer SHALL survive a Restart, so a file-backed stream keeps its messages and a
memory-backed stream does not.

#### Scenario: File stream survives, memory stream does not

- **WHEN** one message is acknowledged on a file-backed stream and one on a memory-backed stream, and the fixture is
  restarted
- **THEN** the file-backed message is readable at its sequence through the new `JetStream()` and the memory-backed
  stream reports the message or the stream absent

#### Scenario: Binding changes across a restart

- **WHEN** Docker maps the container's client port to a different host port after the restart
- **THEN** `URL()` returns the new address and a dial to it succeeds

#### Scenario: Restart before Start

- **WHEN** Restart is called on a fixture whose Start has not succeeded, or once Stop has begun
- **THEN** Restart returns an error and starts no container

#### Scenario: Mapped-port failure after a restart

- **WHEN** the mapped-port read after the restart fails and the caller's context is still live
- **THEN** Restart stops and starts the same container once more and reads the port again; if that read succeeds,
  Restart goes on to connect, and if it fails, Restart returns an error naming the mapped-port phase, with no third
  try and no second container

### Requirement: Connected value for a package's tests

`natsfixture` SHALL provide one helper that, given a test, a server URL and an open function supplied by the caller
(called with a context and the URL, and returning the opened value and a close function that takes a context), calls
the open function under a bounded context and returns the value. When the open function returns an error, the helper
SHALL fail the test with a message naming the URL and that error. On success it SHALL register the close function on
the test's cleanup, under a context with a deadline, and SHALL report a close error as a test error; registered after
the reset or after `New`, it runs before the reset's connection check and before an own server's Stop. The helper
SHALL import no package of this module outside `internal/harness`, SHALL start no goroutine, and SHALL hold nothing
after it returns beyond the cleanup it registered.

#### Scenario: Opening fails

- **WHEN** the open function returns an error
- **THEN** the test fails with a message naming the URL and the error, and no close is registered

#### Scenario: Close runs first, bounded

- **WHEN** a test that opened a value through the helper ends
- **THEN** the close function runs before the connection check or the server's Stop, with a context that has a
  deadline, and a close error is reported as a test error

#### Scenario: Import bound

- **WHEN** the harness import check lists the helper's package imports
- **THEN** no package of this module outside `internal/harness` is among them

## ADDED Requirements

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

### Requirement: Shared server per package

`natsfixture` SHALL provide a server that a package's `TestMain` starts once before `m.Run` and stops after it. Its
start SHALL behave as Start does (the phases, the one replacement container, the container log on failure) and SHALL
also publish the server's monitoring port. Its Stop SHALL remove the container and SHALL return an error naming the
count when the server still reports a client connection after a bounded wait. The server SHALL return its URL only
from the reset.

#### Scenario: Package ends with a connection open

- **WHEN** a client connection is still open after the package's last test
- **THEN** Stop returns an error naming one client connection, the container is gone, and the package fails

#### Scenario: The reset is the only way in

- **WHEN** a test needs the shared server's URL
- **THEN** the reset is the only call that returns it

### Requirement: Reset to a verified-empty server

The per-test reset SHALL, under a bounded context derived from the test's: wait until the shared server reports no
client connection, failing the test with the count when the wait ends; delete every stream on the server, which
removes key-value buckets, object stores and their consumers with it; fail the test unless the account then reports
zero streams and zero consumers; register a cleanup that runs after the test's own cleanups and fails the test, naming
the count, while the server still reports a client connection after a bounded wait; and return the server's URL.

#### Scenario: State does not cross tests

- **WHEN** test A creates a stream, a key-value bucket and a consumer, and test B then resets
- **THEN** B's reset passes and the account reports zero streams and zero consumers

#### Scenario: Fixed names in turn

- **WHEN** two tests in turn each reset and create the stream `ENTITY`
- **THEN** the second finds no message the first published

#### Scenario: A leaked connection fails its own test

- **WHEN** test A leaves a client connection open
- **THEN** test A fails naming one client connection

### Requirement: Labelled containers and the sweep

Every container natsfixture starts SHALL carry the label `org.c360studio.semengine.test`, valued with the test
process's package and process ID. Before a test process starts its first container, natsfixture SHALL remove, with
force, every container carrying that label that Docker created more than 30 minutes earlier, running or not. It SHALL
choose containers by that label and Docker's creation time only, never by name or by whether a process is alive. A
container already gone SHALL be ignored; any other error SHALL be written to the test output and SHALL NOT fail the
test.

#### Scenario: Only old labelled containers are swept

- **WHEN** the sweep runs with an age of zero over a label value the test chose, while the test also runs an unlabelled
  container
- **THEN** the labelled container is gone and the unlabelled one is still running

#### Scenario: Two sweeps at once

- **WHEN** two processes sweep the same old container
- **THEN** neither reports an error for it

### Requirement: Bounded test streams

`natsfixture` SHALL provide functions that create a file-backed or a memory-backed stream on a given JetStream, each
declaring `MaxAge`, `MaxBytes` and `DiscardOld`, so a runaway test fails on a ceiling instead of filling the server.

#### Scenario: Memory stream is bounded

- **WHEN** a test creates a memory-backed stream through the function
- **THEN** the stream's configuration reports memory storage and the bounds
