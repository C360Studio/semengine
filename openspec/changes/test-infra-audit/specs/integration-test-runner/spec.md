# integration-test-runner

## REMOVED Requirements

### Requirement: Shared host lock

**Reason**: The lock's stated jobs were bounding CPU and memory use and coordinating Docker use with SemStreams
(`docs/setup-plan.md:266-268,362-363`). SemEngine no longer coordinates with SemStreams (owner ruling, #205 comment
6132580576), and two runs at once on one daemon pass (the short check recorded on the pull request). Judging whether
the lock's owner was alive caused #50, #124, #126 and #201.

**Migration**: None. `SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS` and `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR` are no longer
read, and nothing reads or writes `/tmp/semstreams-integration.lock`.

### Requirement: Process group ownership

**Reason**: A terminal Ctrl-C reaches every process in the terminal's foreground group, and a test binary stopped by a
signal runs no cleanup whatever the runner does (setup-02 premise P3). The containers such a run leaves are removed by
the sweep (`nats-fixture`, "Labelled containers and the sweep"). The runner's signal handling caused
issues #62, #128 and #202.

**Migration**: A SIGTERM sent to `task` alone leaves the test binaries running until their `-timeout`; their containers
fall to the sweep.

### Requirement: Leak check by session

**Reason**: Replaced by labels and the sweep (owner ruling, #205 comment 6132995614). With Ryuk off, testcontainers
adds no session label to read.

**Migration**: None.

## MODIFIED Requirements

### Requirement: Canonical invocation, pins, and environment

`task test:integration` SHALL run `go test -race -failfast -tags=integration -count=1 -p 2 -shuffle=on -timeout 10m
-coverprofile=coverage/integration.coverprofile <packages>`, with `./...` when no package is given, and SHALL set
`TESTCONTAINERS_RYUK_DISABLED=true` in that command's environment. It SHALL remove `coverage/integration.tree` before
`go test` starts and, only when `go test` exits zero, write the tree fingerprint `scripts/tree-state.sh` prints to that
file. `task cover:check` SHALL read the integration profile from `coverage/integration.coverprofile` and SHALL refuse
it when `coverage/integration.tree` is missing or differs from the fingerprint of the tree as it is now. The task SHALL
take no lock, export no admission token, write no evidence directory and pull no image itself. The same task runs
locally and in CI. A contract test SHALL fail when the command or its environment differs from this requirement.

#### Scenario: Task and CI converge

- **WHEN** `task test:integration` runs locally or in CI
- **THEN** the same command, flags and environment are used

#### Scenario: Plain go test runs an integration test

- **WHEN** a developer runs `go test -tags integration ./natsclient` without Task, on a machine with Docker
- **THEN** the integration tests start their containers and run; nothing refuses them

#### Scenario: A failed run's profile is refused

- **WHEN** `go test` fails in `task test:integration` and `task cover:check` runs next
- **THEN** `coverage/integration.tree` is absent and `cover:check` exits non-zero saying no passing integration run
  measured this tree

#### Scenario: Command drift fails

- **WHEN** the `test:integration` command in `Taskfile.yml` lacks `-shuffle=on` or the Ryuk setting
- **THEN** the contract test fails naming the task and the required command

## ADDED Requirements

### Requirement: Concurrent runs on one daemon

Two `task test:integration` runs started at the same time from two worktrees, on one Docker daemon, SHALL both pass,
and neither SHALL remove a container the other started while that run is in progress.

#### Scenario: Two worktrees at once

- **WHEN** two worktrees each run `task test:integration` over natsclient, graph-ingest and natsfixture at the same
  time, three times
- **THEN** all six runs pass, as recorded by the short check on the pull request that removed the host lock
