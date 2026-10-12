# nats-fixture

## MODIFIED Requirements

### Requirement: Restart keeps the container and re-reads its binding

`Fixture.Restart(ctx)` SHALL stop and start the same container under the fixture's one-slot semaphore, SHALL first
stop every consumer the fixture created (cancelling it and waiting for its handler to be idle, as Stop does) and
record it ended, SHALL drain the fixture's own connection, SHALL run the stop-container, start-container,
mapped-port, connect and JetStream-ready phases in that order deriving every operation context from the caller's,
and on success SHALL make `URL()` and `JetStream()` return the new binding. Streams and buckets the fixture owns
SHALL stay in its ownership record, and Stop SHALL treat one that no longer exists after the restart as observed
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

#### Scenario: Binding changes across a restart

- **WHEN** Docker maps the container's client port to a different host port after the restart
- **THEN** `URL()` returns the new address and a dial to it succeeds

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
  Restart returns nil, and if it fails, Restart returns a `FixtureError` whose phase is mapped-port, with no third
  try and no second container
