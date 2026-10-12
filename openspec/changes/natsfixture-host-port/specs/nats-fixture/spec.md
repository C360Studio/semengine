# nats-fixture

## ADDED Requirements

### Requirement: Published on a loopback port the fixture picks

Start SHALL pick the host port itself, by binding `127.0.0.1` port 0 and closing that listener, SHALL ask Docker to
publish the client port on exactly that address and port, and SHALL name `127.0.0.1` in `URL()`. A connection made to
`URL()` SHALL NOT reach a listener on the same port on all addresses or on `::1`. Start SHALL refuse, at the host
phase, a Docker host that testcontainers reports as anything other than `localhost` or a loopback address, which
includes a daemon on another machine and the gateway address testcontainers reports when the tests run inside a
container. When the picked port is taken before Docker binds it, the fixture SHALL report the failure with a cause
naming the address and port, and SHALL NOT retry it.

#### Scenario: URL names the loopback address

- **WHEN** Start succeeds
- **THEN** the host in `URL()` is `127.0.0.1`, and Docker reports the client port published on `127.0.0.1` at the
  port in `URL()`

#### Scenario: A listener on all addresses holds the port

- **WHEN** a host process listens on all addresses on the port the fixture publishes
- **THEN** no connection made to `URL()` reaches that process

#### Scenario: The picked port is taken before Docker binds it

- **WHEN** another listener binds the picked port on `127.0.0.1` before Docker binds it
- **THEN** Start returns a `FixtureError` at phase start, after one attempt, whose cause names `127.0.0.1` and the
  port, and no container remains

#### Scenario: Ports Docker would assign are held

- **WHEN** host processes listen on all addresses on the ports Docker would assign next
- **THEN** Start succeeds and JetStream answers, and a Restart succeeds

#### Scenario: Docker host is not a loopback address

- **WHEN** testcontainers reports the Docker host as another machine's name, or as the gateway address it reports
  when the tests run inside a container
- **THEN** Start returns a `FixtureError` at phase host that names that host, and no container remains

### Requirement: Restart keeps the container and its published port

`Fixture.Restart(ctx)` SHALL stop and start the same container under the fixture's one-slot semaphore, SHALL first
stop every consumer the fixture created (cancelling it and waiting for its handler to be idle, as Stop does) and
record it ended, SHALL drain the fixture's own connection, SHALL run the stop-container, start-container,
mapped-port, connect and JetStream-ready phases in that order deriving every operation context from the caller's,
and on success SHALL make `JetStream()` return the new connection's context and `URL()` return the binding re-read
from the container, which keeps the address and port Start published. Streams and buckets the fixture owns SHALL stay
in its ownership record, and Stop SHALL treat one that no longer exists after the restart as observed absent. Restart
SHALL return an error and make no Docker call before Start has succeeded or once Stop has begun. A phase failure SHALL
return a `FixtureError` carrying the phase; the fixture SHALL NOT create a replacement container during Restart. The
container's writable layer SHALL survive a Restart, so a file-backed stream keeps its messages and a memory-backed
stream does not.

#### Scenario: File stream survives, memory stream does not

- **WHEN** one message is acknowledged on a file-backed stream and one on a memory-backed stream, and the fixture is
  restarted
- **THEN** the file-backed message is readable at its sequence through the new `JetStream()` and the memory-backed
  stream reports the message or the stream absent

#### Scenario: Binding kept across a restart

- **WHEN** the fixture is restarted
- **THEN** `URL()` returns the same address as before the restart, Docker reports the client port published there,
  and a dial to it reaches the restarted broker

#### Scenario: Port taken while stopped

- **WHEN** another listener binds the fixture's port on `127.0.0.1` while the container is stopped
- **THEN** Restart returns a `FixtureError` at phase start-container whose cause names `127.0.0.1` and the port, no
  second container exists, and Stop still observes the container gone

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

## MODIFIED Requirements

### Requirement: Phased start with typed failure

Start SHALL proceed image → create/start, publishing the client port on the port it picked → internal ready log →
host → published client port → connect → JetStream ready, deriving every operation context from the test context; a
failure SHALL return an error carrying attempt, phase, container id when one exists, parent-context state, cause, and
cleanup outcome, and matching the parent context's error under errors.Is whenever the parent had ended; SHALL
terminate any container returned with the failure under a fresh bounded context; and SHALL write the phase record and
container logs to the evidence directory. The published client port SHALL be read from one inspect of the container,
and when that inspect shows no published client port, the cause SHALL carry the container's state, the requested
binding and the published bindings from that same inspect. At most one replacement attempt is permitted, only for the
mapped-port phase with a live parent and successful cleanup, and it picks a new port. The fixture SHALL NOT expose
start, connect, or bucket-prefix knobs.

#### Scenario: Failure after the container exists

- **WHEN** connect fails after the container is running
- **THEN** the error names phase connect and the container id, the container is absent, its logs are in the evidence
  dir

#### Scenario: Cancellation during start

- **WHEN** the test context is cancelled while the container is starting
- **THEN** Start returns an error satisfying errors.Is(err, context.Canceled), no replacement is attempted, no
  container remains

#### Scenario: Client port not published

- **WHEN** the inspect of a running container shows no published client port
- **THEN** the mapped-port error names the container's state, the requested binding and the published bindings, all
  read from that one inspect

## REMOVED Requirements

### Requirement: Restart keeps the container and re-reads its binding

**Reason**: Restart no longer re-maps the port: the container keeps the address and port Start published, so
the scenario "Binding changes across a restart" no longer describes it.

**Migration**: Replaced by "Restart keeps the container and its published port", which keeps every other scenario.
