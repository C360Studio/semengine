# integration-test-runner

## MODIFIED Requirements

### Requirement: Shared host lock

The runner SHALL acquire `/tmp/semstreams-integration.lock` with an atomic directory create and an owner record before
any Docker call. The record SHALL hold SemStreams' six keys host, pid, started, identity, token, command, in that
order (command naming this repository and worktree), followed by the key identity_utc and no other, each on a line
that ends with a newline; SemStreams' runner at its pin ignores a key it does not know, so it reads the record as an
ordinary owner. The runner SHALL publish the record whole: after creating the lock directory it SHALL write the record
to a new file beside that directory, in the same parent directory, named `<lock>.owner.<random>`, give it the mode a new
file gets under the runner's umask, and rename it to `<lock>/owner`, so that a reader finds either no owner file or the
complete record. It SHALL write nothing else inside the lock directory, and that file's name never begins
`<lock>.stale.`, the quarantine name. If publication fails, the runner SHALL exit non-zero before any Docker call,
having removed the lock directory and the file it wrote beside it, unless the complete record already stands in the
lock directory, as when an interrupt stops `mv` after it has renamed the file; that record names the exiting runner,
and the next runner quarantines it once that pid is dead. An interrupt that stops `mktemp` after it created its file
leaves that file beside the lock directory, where no reader reads it. A lock directory without an owner file is
respected by every reader, since its host reads as unknown; a runner killed between creating the lock directory and
the rename leaves one, which stays until it is removed by hand.
The identity SHALL be the runner's start time as `ps -o lstart= -p <pid>` prints it in the runner's
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
  other key, its identity_utc is not empty, and its mode is the one a new file gets under the runner's umask

#### Scenario: A contender during publication respects the owner

- **WHEN** a runner has created the lock directory and not yet renamed its record into it, and a second runner on the
  same host, started from a shell with another time zone, finds the lock with no wait budget set
- **THEN** the second runner exits non-zero without quarantining the lock or making a Docker call, and the first runner
  then publishes its complete record, runs, and releases the lock, leaving no file beside it

#### Scenario: A record that cannot be published releases the lock

- **WHEN** the runner has created the lock directory and cannot write its record beside it
- **THEN** the runner removes the lock directory and any file it wrote beside it, and exits non-zero without a Docker
  call
