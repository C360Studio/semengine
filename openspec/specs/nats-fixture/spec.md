# nats-fixture Specification

## Purpose
The NATS fixture gives one test one disposable, admitted, real NATS server in a container, with typed start failures,
run-unique safe names, and a cleanup that returns success only after it has observed every owned resource gone.

## Requirements

### Requirement: Admission before Docker

`natsfixture.Start` SHALL refuse with ErrNotAdmitted, naming `task test:integration`, unless
SEMENGINE_DOCKER_ADMISSION_TOKEN is present in the lock owner file at SEMENGINE_DOCKER_ADMISSION_LOCK_DIR and that
owner is live as `integration-test-runner` › "Shared host lock" defines it, read in the test process's environment.
Where the runner respects an owner it cannot prove dead, admission SHALL refuse one it cannot prove live: an owner on
another host, a pid that is not a number, an `unknown` identity, a pid for which ps prints no start time, and a start
time that differs from the identity are each refused, and the refusal names the reason. Admission reads the start
time under Start's context: if that context has ended when the read fails, Start SHALL return the context's error.
Neither a refusal nor that return SHALL make a Docker call.

#### Scenario: Direct go test is refused

- **WHEN** a test calls Start without the runner's environment
- **THEN** Start returns ErrNotAdmitted and no container is created

#### Scenario: A reused pid is refused

- **WHEN** the owner file holds the test's token and names this host and a running pid whose start time differs from
  the file's identity
- **THEN** Start returns ErrNotAdmitted naming that identity, and makes no Docker call

#### Scenario: An unknown identity is refused

- **WHEN** the owner file holds the test's token and names this host, a running pid, and the identity `unknown`
- **THEN** Start returns ErrNotAdmitted naming `unknown`, and makes no Docker call

#### Scenario: A live owner admits

- **WHEN** the owner file holds the test's token and names this host, a running pid, and that pid's start time as ps
  prints it
- **THEN** admission passes and Start goes on to start the container

### Requirement: Phased start with typed failure

Start SHALL proceed image → create/start → internal ready log → host → mapped client port → connect → JetStream ready,
deriving every operation context from the test context; a failure SHALL return an error carrying attempt, phase,
container id when one exists, parent-context state, cause, and cleanup outcome, and matching the parent context's
error under errors.Is whenever the parent had ended; SHALL terminate any container returned
with the failure under a fresh bounded context; and SHALL write the phase record and container logs to the evidence
directory. At most one replacement attempt is permitted, only for the mapped-port phase with a live parent and
successful cleanup. The fixture SHALL NOT expose start, connect, or bucket-prefix knobs.

#### Scenario: Failure after the container exists

- **WHEN** connect fails after the container is running
- **THEN** the error names phase connect and the container id, the container is absent, its logs are in the evidence
  dir

#### Scenario: Cancellation during start

- **WHEN** the test context is cancelled while the container is starting
- **THEN** Start returns an error satisfying errors.Is(err, context.Canceled), no replacement is attempted, no
  container remains

### Requirement: Run-unique ownership and safe names

The fixture SHALL provide Name(base) yielding `semengine-<sanitized test segment>-<base>-<hex>` over `[a-z0-9-]`, free
of every SemStreams destroyer substring (a segment containing one is replaced by its hash); SHALL record every stream,
bucket, and consumer it creates; created streams SHALL declare MaxAge, MaxBytes, and DiscardOld. The fixture creates
no volumes or networks.

#### Scenario: Two fixtures do not collide

- **WHEN** two fixtures start in one test
- **THEN** their container ids, host ports, and Name values are disjoint

#### Scenario: Adversarial test name

- **WHEN** the calling test is named TestOpsResearchGraph
- **THEN** Name output contains neither `ops` nor `research-graph`

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
