# nats-fixture

## Purpose

The NATS fixture gives one test one disposable, admitted, real NATS server in a container, with typed start failures,
run-unique safe names, and a cleanup that returns success only after it has observed every owned resource gone.

## ADDED Requirements

### Requirement: Admission before Docker

`natsfixture.Start` SHALL refuse with ErrNotAdmitted, naming `task test:integration`, unless
SEMENGINE_DOCKER_ADMISSION_TOKEN is present in the live lock owner file at SEMENGINE_DOCKER_ADMISSION_LOCK_DIR; the
refusal SHALL make no Docker call.

#### Scenario: Direct go test is refused

- **WHEN** a test calls Start without the runner's environment
- **THEN** Start returns ErrNotAdmitted and no container is created

### Requirement: Phased start with typed failure

Start SHALL proceed image → create/start → internal ready log → host → mapped client port → connect → JetStream ready,
deriving every operation context from the test context; a failure SHALL return an error carrying attempt, phase,
container id when one exists, parent-context state, cause, and cleanup outcome; SHALL terminate any container returned
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
SHALL be a no-op. Start after Start SHALL be refused. The fixture SHALL retain no context.Context.

#### Scenario: Blocked callback delays finalisation

- **WHEN** a consumer callback is blocked during Stop
- **THEN** the callback's context is live until released, the join completes before Stop returns, and only then are
  the consumer, stream, connection, and container removed

#### Scenario: Deadline is not a join

- **WHEN** Stop's context expires while a callback is blocked
- **THEN** Stop returns context.DeadlineExceeded, the container still exists, remaining() lists the handles, and a
  later bounded Stop completes with everything absent
