# integration-test-runner

## Purpose

The integration runner is the one admitted entry to Docker-backed tests in SemEngine: it takes the host lock shared
with SemStreams, owns the `go test` process group, and proves after every run that nothing it started survives.

## ADDED Requirements

### Requirement: Shared host lock

The runner SHALL acquire `/tmp/semstreams-integration.lock` with an atomic directory create and an owner record with
exactly the keys host, pid, started, identity, token, command (command naming this repository and worktree) before any
Docker call; SHALL fail fast reporting the owner when the lock is busy unless SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS
(1–3600) is set; SHALL quarantine only a same-host owner whose pid is dead or whose start identity changed; SHALL
release only while the owner token still matches; and SHALL read no SEMSTREAMS_* variable.

#### Scenario: Busy lock refuses before Docker

- **WHEN** the lock directory exists with a live owner and no wait budget is set
- **THEN** the runner exits non-zero, prints the owner record, and invokes no Docker command

#### Scenario: Provably stale lock is quarantined

- **WHEN** the owner record names this host and a pid that is not running
- **THEN** the runner moves the directory to `.stale.<pid>.<started>`, removes it, and acquires

#### Scenario: Foreign or live owner is respected

- **WHEN** the owner record names another host, or a live pid with unchanged identity
- **THEN** the runner refuses and reports the owner without removing anything

### Requirement: Process group ownership

The runner SHALL start `go test` in its own process group, forward INT and TERM to that group, escalate to KILL after
a bounded grace, reap the group before the leak check and the lock release, and record the signal and timings. A
signal received during the bounded image pull SHALL kill and reap the pull before the lock is released. The log's
writer SHALL survive a terminal interrupt, so output `go test` writes while shutting down reaches the evidence; the
runner SHALL wait for the log writer only boundedly after the group is reaped, recording `log_incomplete=yes` when a
process outside the group still holds the output open, so no escaped writer holds the lock. A
runner started with SIGINT ignored SHALL warn and record `int_ignored_on_entry=yes`; SIGTERM is the scripted
interrupt. SEMENGINE_TEST_SIGNAL_GRACE_SECONDS (1–20) shortens the grace for the runner's own contract tests only.

#### Scenario: TERM reaches the test binaries

- **WHEN** the runner receives SIGTERM while `go test` and a test binary run
- **THEN** both receive SIGTERM and are reaped, the leak check runs, the lock is released, the exit status is 143

#### Scenario: A group that ignores TERM is killed

- **WHEN** `go test` and a test binary ignore SIGTERM past the grace
- **THEN** the runner sends KILL to the group, records it, reaps the group, and releases the lock

#### Scenario: Signal during the image pull

- **WHEN** the runner receives a signal while the image pull runs
- **THEN** the pull is killed and reaped before the lock is released, and no `go test` runs

#### Scenario: Terminal interrupt keeps the log

- **WHEN** a terminal Ctrl-C reaches the runner's foreground process group
- **THEN** output `go test` writes after the interrupt is in `go-test.log` and the per-package summary

### Requirement: Leak check by session

The runner SHALL read the testcontainers session id recorded by the fixture and, after reaping the child group, SHALL
wait bounded for containers labelled with that session to be absent; survivors SHALL be recorded, removed by container
id only, and fail the run. The runner SHALL NOT filter Docker resources by name, SHALL NOT invoke any Docker prune,
and SHALL NOT run Compose `down` for a project it did not create.

#### Scenario: Interrupted run leaves nothing labelled with its session

- **WHEN** the run is interrupted mid-test
- **THEN** within the bound no container carries the run's session label and the evidence dir records how each was
  removed

### Requirement: Canonical invocation, pins, and environment

The runner SHALL run `go test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m
-coverprofile=<evidence dir>/integration.coverprofile <packages>` (the profile is what the 80% coverage gate on
`natsfixture` reads); SHALL export TESTCONTAINERS_RYUK_DISABLED=false, SEMENGINE_DOCKER_ADMISSION_TOKEN,
SEMENGINE_DOCKER_ADMISSION_LOCK_DIR, SEMENGINE_EVIDENCE_DIR, and SEMENGINE_NATS_IMAGE read from `.nats-image`. The
runner SHALL accept SEMENGINE_NATS_IMAGE from its caller as a replacement digest reference only together with a
non-empty SEMENGINE_NATS_IMAGE_OVERRIDE_REASON, refusing before the lock otherwise, and SHALL print a WARN line and
record `image_override=<ref>` and the reason in `runner.env`. It SHALL check the image cache by digest and
pull under the lock with a bounded budget when absent; and SHALL record per-package wall time, Docker preflight
latency, effective Docker host and context, and Ryuk settings.

#### Scenario: Task and CI converge

- **WHEN** `task test:integration` runs locally or in CI
- **THEN** the same script, flags, and environment are used

#### Scenario: Image override needs a reason

- **WHEN** SEMENGINE_NATS_IMAGE is set without SEMENGINE_NATS_IMAGE_OVERRIDE_REASON
- **THEN** the runner exits non-zero before taking the lock or calling Docker

#### Scenario: Digest pin ignores tag movement

- **WHEN** another repository re-pulls the mutable `nats:2.14-alpine` tag
- **THEN** the runner's digest-addressed cache check and pull are unaffected
