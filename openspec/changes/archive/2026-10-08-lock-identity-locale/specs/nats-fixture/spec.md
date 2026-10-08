# nats-fixture

## MODIFIED Requirements

### Requirement: Admission before Docker

`natsfixture.Start` SHALL refuse with ErrNotAdmitted, naming `task test:integration`, unless
SEMENGINE_DOCKER_ADMISSION_TOKEN is present in the lock owner file at SEMENGINE_DOCKER_ADMISSION_LOCK_DIR and that
owner is live as `integration-test-runner` › "Shared host lock" defines it: by its identity_utc, read with TZ=UTC and
LC_ALL=C, when the record has a non-empty identity_utc, and otherwise by its identity, read in the test process's
environment. Only SemEngine's runner exports the admission token, so a record without identity_utc reaches admission
only from a SemEngine runner older than identity_utc or from a test. Where the runner respects an owner it cannot
prove dead, admission SHALL refuse one it cannot prove live: an owner on another host, a pid that is not a number, a
compared value that is `unknown`, a pid for which ps prints no start time, and a start time that differs from the
compared value are each refused, and the refusal names the reason. Admission reads the start time under Start's
context: if that context has ended when the read fails, Start SHALL return the context's error. Neither a refusal nor
that return SHALL make a Docker call.

#### Scenario: Direct go test is refused

- **WHEN** a test calls Start without the runner's environment
- **THEN** Start returns ErrNotAdmitted and no container is created

#### Scenario: A reused pid is refused

- **WHEN** the owner file holds the test's token and names this host and a running pid whose start time, read with
  TZ=UTC and LC_ALL=C, differs from the file's identity_utc
- **THEN** Start returns ErrNotAdmitted naming that identity_utc, and makes no Docker call

#### Scenario: An unknown identity is refused

- **WHEN** the owner file holds the test's token and names this host, a running pid, and the identity_utc `unknown`
- **THEN** Start returns ErrNotAdmitted naming `unknown`, and makes no Docker call

#### Scenario: A live owner admits

- **WHEN** the owner file holds the test's token and names this host, a running pid, and as identity_utc that pid's
  start time as ps prints it with TZ=UTC and LC_ALL=C
- **THEN** admission passes and Start goes on to start the container

#### Scenario: A test process in another time zone is admitted

- **WHEN** the owner file holds the test's token, this host, a running pid and that pid's identity_utc, and the test
  process's time zone and locale differ from those its identity was read in
- **THEN** admission passes

#### Scenario: A record without identity_utc is judged by identity

- **WHEN** the owner file holds the test's token and names this host and a running pid, and has no identity_utc
- **THEN** admission passes when its identity is that pid's start time as ps prints it in the test process's
  environment, and Start returns ErrNotAdmitted naming the identity otherwise
