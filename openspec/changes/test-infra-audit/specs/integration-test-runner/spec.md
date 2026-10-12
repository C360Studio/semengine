# integration-test-runner

## REMOVED Requirements

### Requirement: Shared host lock

**Reason**: The lock's stated jobs were bounding CPU and memory use and coordinating Docker use with SemStreams
(`docs/setup-plan.md:266-268,361-363`). SemEngine no longer coordinates with SemStreams (owner ruling, #205 comment
6132580576), and two runs at once on one daemon pass (the short check recorded on the pull request). Judging whether
the lock's owner was alive caused #50, #124, #126 and #201.

**Migration**: None. `SEMENGINE_DOCKER_ADMISSION_WAIT_SECONDS` and `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR` are no longer
read, and nothing reads or writes `/tmp/semstreams-integration.lock`.

### Requirement: Process group ownership

**Reason**: A terminal Ctrl-C reaches every process in the terminal's foreground group, and a test binary stopped by a
signal runs no cleanup whatever the runner does (setup-02 premise P3). The containers such a run leaves are removed by
the sweep (`nats-fixture`, "Ryuk off, labelled containers and the sweep"). The runner's signal handling caused
issues #62, #128 and #202.

**Migration**: A SIGTERM sent to `task` alone leaves the test binaries running until their `-timeout`; their containers
fall to the sweep.

### Requirement: Leak check by session

**Reason**: The owner ruled Ryuk off and the runner's leak check out, replaced by labels and the sweep (#205 comment
6132995614).

**Migration**: None.

### Requirement: Canonical invocation, pins, and environment

**Reason**: The runner it describes is removed. The command it fixes moves to "Integration test command"; the image
override and its reason go with the forced-failure protocol of setup-02; the digest property moves to
`harness-boundaries`, "One image pin".

**Migration**: `SEMENGINE_NATS_IMAGE` and `SEMENGINE_NATS_IMAGE_OVERRIDE_REASON` are no longer read. To run another
image, edit the pin file in a branch.

## ADDED Requirements

### Requirement: Integration test command

`task test:integration` SHALL run `go test -race -failfast -tags=integration -count=1 -p 2 -shuffle=on -timeout 10m
-coverprofile=coverage/integration.coverprofile <packages>`, with `./...` when no package is given. It SHALL remove
`coverage/integration.tree` before `go test` starts and, only when `go test` exits zero, write the tree fingerprint
`scripts/tree-state.sh` prints to that file. `task cover:check` SHALL read the integration profile from
`coverage/integration.coverprofile` and SHALL refuse it when `coverage/integration.tree` is missing or differs from
the fingerprint of the tree as it is now. The task SHALL take no lock, set no environment variable, write no evidence
directory and pull no image itself. The same task runs locally and in CI. A contract test SHALL fail when the command
differs from this requirement.

#### Scenario: Task and CI converge

- **WHEN** `task test:integration` runs locally or in CI
- **THEN** the same command and flags are used

#### Scenario: Plain go test runs an integration test

- **WHEN** a developer runs `go test -tags integration ./natsclient` without Task, on a machine with Docker
- **THEN** the integration tests start their containers and run; nothing refuses them

#### Scenario: A failed run's profile is refused

- **WHEN** `go test` fails in `task test:integration` and `task cover:check` runs next
- **THEN** `coverage/integration.tree` is absent and `cover:check` exits non-zero saying no passing integration run
  measured this tree

#### Scenario: Command drift fails

- **WHEN** the `test:integration` command in `Taskfile.yml` lacks `-shuffle=on`
- **THEN** the contract test fails naming the task and the required command

### Requirement: Concurrent runs on one daemon

Two `task test:integration` runs started at the same time from two worktrees, on one Docker daemon, SHALL both pass,
and neither SHALL remove a container the other started while that run is in progress.

#### Scenario: Two worktrees at once

- **WHEN** two worktrees each run `task test:integration` over natsclient, graph-ingest and natsfixture at the same
  time, three times
- **THEN** all six runs pass, as recorded by the short check on the pull request that removed the host lock and again
  on the one that made servers shared
