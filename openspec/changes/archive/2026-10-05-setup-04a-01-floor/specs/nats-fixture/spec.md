# nats-fixture

## ADDED Requirements

### Requirement: Restart keeps the container and re-reads its binding

`Fixture.Restart(ctx)` SHALL stop and start the same container under the fixture's one-slot semaphore, SHALL first
stop every consumer the fixture created (cancelling it and waiting for its handler to be idle, as Stop does) and
record it ended, SHALL drain the fixture's own connection, SHALL run the stop-container, start-container,
mapped-port, connect and JetStream-ready phases in that order deriving every operation context from the caller's,
and on success SHALL make `URL()` and `JetStream()` return the new binding. Streams and buckets the fixture owns
SHALL stay in its ownership record, and Stop SHALL treat one that no longer exists after the restart as observed
absent. Restart SHALL return an error and make no Docker call before Start has succeeded or once Stop has begun. A
phase failure SHALL return a `FixtureError` carrying the phase; the fixture SHALL NOT create a replacement
container during Restart. The container's writable layer SHALL survive a Restart, so a file-backed stream keeps its
messages and a memory-backed stream does not.

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
- **THEN** the broker refuses it, and a message of at most N bytes is accepted
