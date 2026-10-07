# nats-fixture

## MODIFIED Requirements

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
