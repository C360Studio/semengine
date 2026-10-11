# integration-test-runner

## MODIFIED Requirements

### Requirement: Shared host lock

The runner SHALL acquire `/tmp/semstreams-integration.lock` with an atomic directory create and an owner record with
exactly the keys host, pid, started, identity, token, command (command naming this repository and worktree) before any
Docker call. The identity SHALL be the runner's start time as `ps -o lstart= -p <pid>` prints it in the runner's
environment, with leading blanks removed and trailing blanks kept, or `unknown` when that prints nothing. An owner
record is live when its host is this host and `ps -o lstart= -p <pid>`, read the same way in the reader's
environment, prints its identity; an `unknown` identity is never live. ps reports start times to the second, so a
pid reused within the second its owner started reads as that owner. The runner SHALL fail fast reporting the owner
when the lock is busy unless SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS (1–3600) is set; SHALL quarantine only a
same-host owner whose pid `kill -0` cannot signal (another user's process included) or whose start time, read the
same way, differs from an identity other than `unknown`, and SHALL respect every other owner; SHALL release only while
the owner token still matches; and SHALL read no SEMSTREAMS_* variable.

#### Scenario: Busy lock refuses before Docker

- **WHEN** the lock directory exists with a live owner and no wait budget is set
- **THEN** the runner exits non-zero, prints the owner record, and invokes no Docker command

#### Scenario: Provably stale lock is quarantined

- **WHEN** the owner record names this host and a pid that is not running
- **THEN** the runner moves the directory to `.stale.<pid>.<started>`, removes it, and acquires

#### Scenario: Changed start time is quarantined

- **WHEN** the owner record names this host and a running pid whose start time differs from the record's identity
- **THEN** the runner moves the directory to `.stale.<pid>.<started>`, removes it, and acquires

#### Scenario: Foreign or live owner is respected

- **WHEN** the owner record names another host, or a live pid with unchanged identity
- **THEN** the runner refuses and reports the owner without removing anything
