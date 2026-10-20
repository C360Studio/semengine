# nats-fixture

## MODIFIED Requirements

### Requirement: Phased start in one container

Start SHALL proceed image → create and start → internal ready log → host → mapped client port → connect → JetStream
ready, deriving every operation context from the caller's. A failure SHALL return an error that names the phase, the
container ID when a container exists, and the cause, and that matches the caller's context error under `errors.Is`
whenever that context had ended. Before returning it SHALL remove any container Start created, under a fresh bounded
context, and report a removal failure in the error, and it SHALL write that container's log to the test output. The
create and start phase SHALL run the once-per-process Docker preparation before taking its host port, SHALL publish the
client port on a host port the fixture took from the operating system with a port-0 listener and closed, and SHALL NOT
leave the host port to Docker to choose; a failure to take a port SHALL fail that phase. When Docker fails to start the
container with "address already in use" and the caller's context is still live, the create and start phase SHALL remove
that unstarted container under a fresh bounded context, take a new host port, and try to create and start the container
once more (at most two create-and-start attempts total), returning a second failure with both causes joined; after a
failure in any other phase, or any other create-and-start failure, Start SHALL NOT start another container. A failed
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

#### Scenario: Explicit host-port binding alongside held ephemeral ports

- **WHEN** ephemeral host ports are held by listeners on all addresses and two fixtures start
- **THEN** both fixtures' Start calls return nil, Docker's inspect for each container reports its `URL()` port as the
  explicit `HostPort` binding on `4222/tcp`, and JetStream answers on each `URL()`

#### Scenario: Host port taken before Docker binds it

- **WHEN** another listener holds the first host port the create and start phase picked while the caller's context is
  live
- **THEN** Start removes the first unstarted container, takes a new host port, starts a second container, and returns
  nil with only the second container present; when both attempts hit a held port, Start fails in the start phase after
  two attempts and leaves no container behind

#### Scenario: A missing mapped port fails Start

- **WHEN** the mapped-port read fails
- **THEN** Start returns an error naming the mapped-port phase and what the inspect of the container showed, the
  container is gone, and no second container starts
