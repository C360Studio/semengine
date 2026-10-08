# integration-test-runner

## MODIFIED Requirements

### Requirement: Shared host lock

The runner SHALL acquire `/tmp/semstreams-integration.lock` with an atomic directory create and an owner record before
any Docker call. The record SHALL hold SemStreams' six keys host, pid, started, identity, token, command, in that
order (command naming this repository and worktree), followed by the key identity_utc and no other, each on a line
that ends with a newline; SemStreams' runner at its pin ignores a key it does not know, so it reads the record as an
ordinary owner. The identity SHALL be the runner's start time as `ps -o lstart= -p <pid>` prints it in the runner's
environment, and identity_utc the same read made with TZ=UTC and LC_ALL=C set for ps; each with leading blanks removed
and trailing blanks kept, or `unknown` when that read prints nothing. An owner record is live when its host is this
host and, if its identity_utc is present and not empty, `ps -o lstart= -p <pid>` read with TZ=UTC and LC_ALL=C prints
that identity_utc; if its identity_utc is absent or empty, as in a record SemStreams' runner or a SemEngine runner
older than identity_utc wrote, the same read made in the reader's environment prints its identity. An `unknown` value
is never live. ps reports start times to the second, so a pid reused within the second its owner started reads as that
owner. A record judged by its identity is misread when the writer's and the reader's time zone or locale differ: a live
owner then reads as changed. The runner SHALL fail fast reporting the owner when the lock is busy unless
SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS (1–3600) is set; SHALL quarantine only a same-host owner whose pid `kill -0`
cannot signal (another user's process included) or whose start time, read as the liveness rule above reads it,
differs from the value that rule compares (identity_utc, or identity for a record without one) when that value is not
`unknown`, and SHALL respect every other owner; SHALL release only while the owner token still matches; and SHALL read
no SEMSTREAMS_* variable.

#### Scenario: Busy lock refuses before Docker

- **WHEN** the lock directory exists with a live owner and no wait budget is set
- **THEN** the runner exits non-zero, prints the owner record, and invokes no Docker command

#### Scenario: Provably stale lock is quarantined

- **WHEN** the owner record names this host and a pid that is not running
- **THEN** the runner moves the directory to `.stale.<pid>.<started>`, removes it, and acquires

#### Scenario: Changed start time is quarantined

- **WHEN** the owner record names this host and a running pid whose start time, read with TZ=UTC and LC_ALL=C, differs
  from the record's identity_utc, or, for a record without identity_utc, whose start time read in the runner's
  environment differs from its identity
- **THEN** the runner moves the directory to `.stale.<pid>.<started>`, removes it, and acquires

#### Scenario: Foreign or live owner is respected

- **WHEN** the owner record names another host, or a live pid with unchanged identity_utc, or, for a record without
  identity_utc, unchanged identity
- **THEN** the runner refuses and reports the owner without removing anything

#### Scenario: Owner in another time zone or locale is respected

- **WHEN** a runner holds the lock, and a second runner on the same host, started from a shell with another time zone
  or locale, finds the lock with no wait budget set
- **THEN** the second runner exits non-zero reporting the first as owner, and the first keeps the lock

#### Scenario: Owner record format

- **WHEN** the runner holds the lock
- **THEN** its owner record holds host, pid, started, identity, token, command and identity_utc, in that order and no
  other key, and its identity_utc is not empty
