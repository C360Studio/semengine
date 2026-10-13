# nats-fixture

## MODIFIED Requirements

### Requirement: Phased start with typed failure

Start SHALL proceed image → create/start → internal ready log → host → mapped client port → connect → JetStream ready,
deriving every operation context from the test context; a failure SHALL return an error carrying attempt, phase,
container id when one exists, parent-context state, cause, and cleanup outcome, and matching the parent context's
error under errors.Is whenever the parent had ended; SHALL terminate any container returned
with the failure under a fresh bounded context; and SHALL write the phase record and container logs to the evidence
directory. The create/start phase of each attempt SHALL publish the client port on a host port the fixture took from
the operating system with a port-0 listener and closed, and SHALL NOT leave the host port to Docker to choose; a
failure to take a port SHALL fail that phase. At most one replacement attempt is permitted, only for the mapped-port
phase with a live parent and successful cleanup. The fixture SHALL NOT expose start, connect, or bucket-prefix knobs.

#### Scenario: Failure after the container exists

- **WHEN** connect fails after the container is running
- **THEN** the error names phase connect and the container id, the container is absent, its logs are in the evidence
  dir

#### Scenario: Cancellation during start

- **WHEN** the test context is cancelled while the container is starting
- **THEN** Start returns an error satisfying errors.Is(err, context.Canceled), no replacement is attempted, no
  container remains

#### Scenario: Ports after a running fixture's own are held

- **WHEN** a fixture is running and the host ports after its own are held by listeners on all addresses
- **THEN** a second fixture's Start returns nil, with JetStream answering on its `URL()`

## ADDED Requirements

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

## REMOVED Requirements

### Requirement: Restart keeps the container and re-reads its binding

**Reason**: Its scenario "Binding changes across a restart" is no longer true: the container keeps the host port Start
gave it (#209). OpenSpec's archive refuses a modified requirement that drops a scenario, so the requirement is
re-added above as "Restart keeps the container and its host port", with the same text, one more sentence on the host
port, and "Binding kept across a restart" in place of that scenario.

**Migration**: None. A test that dialled the new `URL()` after a Restart still dials the same address.
