# nats-fixture

## Purpose

The NATS fixture gives one test one disposable, admitted, real NATS server in a container, with typed start failures,
run-unique safe names, and a cleanup that returns success only after it has observed every owned resource gone.

## ADDED Requirements

### Requirement: Admission before Docker

`natsfixture.Start` SHALL refuse with ErrNotAdmitted, naming `task test:integration`, unless
SEMENGINE_DOCKER_ADMISSION_TOKEN is present in the live lock owner file at SEMENGINE_DOCKER_ADMISSION_LOCK_DIR; the
owner file is live when its host is this host and its pid is a running process. The refusal SHALL make no Docker
call.

#### Scenario: Direct go test is refused

- **WHEN** a test calls Start without the runner's environment
- **THEN** Start returns ErrNotAdmitted and no container is created

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

- **WHEN** CreateStream, CreateKeyValue, or Consume is called after Stop has begun
- **THEN** it returns an error at once, creates nothing, and Stop's nil return still means nothing is owned

#### Scenario: Stop waits for a running Start only under its own context

- **WHEN** Stop is called with a short context while Start is parked in a Docker call
- **THEN** Stop returns context.DeadlineExceeded when its context ends

#### Scenario: Deadline is not a join

- **WHEN** Stop's context expires while a callback is blocked
- **THEN** Stop returns context.DeadlineExceeded, the container still exists, remaining() lists the handles, and a
  later bounded Stop completes with everything absent
