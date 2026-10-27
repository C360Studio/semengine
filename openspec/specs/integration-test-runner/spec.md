# integration-test-runner Specification

## Purpose
How SemEngine runs its Docker-backed tests: `task test:integration` is plain `go test` with pinned flags, and it
records which tree its coverage profile measured, so `task cover:check` can refuse a profile taken from another tree.

## Requirements

### Requirement: Integration test command

`task test:integration` SHALL run `go test -race -failfast -tags=integration -count=1 -p 2 -shuffle=on -timeout 10m
-coverprofile=coverage/integration.coverprofile <packages>`, with the packages given after `--`, or else the
integration packages. The same task SHALL run locally and in CI. A pinning test SHALL fail when the command, with the
`coverage/integration.tree` lines in the requirement "The task records which tree its profile measured", differs from
this requirement.

#### Scenario: Task and CI converge

- **WHEN** `task test:integration` runs locally or in CI
- **THEN** the same command and flags are used

#### Scenario: Command drift fails

- **WHEN** the `test:integration` command in `Taskfile.yml` lacks `-shuffle=on`
- **THEN** the pinning test fails naming the task and the required command

### Requirement: The task records which tree its profile measured

`task test:integration` SHALL remove `coverage/integration.tree` before `go test` starts and, only when `go test` exits
zero, write the tree fingerprint `scripts/tree-state.sh` prints to that file.

#### Scenario: A failed run's profile is refused

- **WHEN** `go test` fails in `task test:integration` and `task cover:check` runs next
- **THEN** `coverage/integration.tree` is absent and `cover:check` exits non-zero saying no passing integration run
  measured this tree

### Requirement: Cover check refuses a profile from another tree

`task cover:check` SHALL read the integration profile from `coverage/integration.coverprofile` and SHALL refuse it when
`coverage/integration.tree` is missing or differs from the fingerprint of the tree as it is now.

#### Scenario: A profile from a changed tree

- **WHEN** `coverage/integration.tree` differs from the fingerprint of the tree as it is now
- **THEN** `task cover:check` refuses the integration profile

### Requirement: The task takes no lock and sets nothing up

The task SHALL take no lock, set no environment variable, write no evidence directory and pull no image itself.

#### Scenario: Plain go test runs an integration test

- **WHEN** a developer runs `go test -tags integration ./natsclient` without Task, on a machine with Docker
- **THEN** the integration tests start their containers and run; nothing refuses them

### Requirement: Concurrent runs on one daemon

Two worktrees running `task test:integration` at the same time on one machine and one Docker daemon SHALL both run
and pass without interfering with each other's containers or ports.

#### Scenario: Two worktrees at once

- **WHEN** two worktrees start `task test:integration` at the same time against one Docker daemon
- **THEN** neither blocks the other, neither sweeps the other's live containers, and both exit zero

### Requirement: Only packages with integration tests

The integration packages SHALL be the directories of the tracked or untracked, unignored `_test.go` files outside
`testdata` whose line, trimmed, is `//go:build integration`. `test:integration` SHALL set `PKGS` from
`scripts/test-pkgs.sh integration`, a Task variable and never an environment variable, and a pinning test SHALL fail
when it reads another list. An empty list or a failing command SHALL fail the task before `go test` starts.

#### Scenario: A package without integration tests

- **WHEN** a package such as `graph` has no `integration`-tagged file
- **THEN** `task test:integration` does not run it

#### Scenario: A new integration package

- **WHEN** a package gains its first `integration`-tagged test file
- **THEN** `scripts/test-pkgs.sh integration` lists it with no other edit
- **AND** the test that verifies this is `TestTestPkgsLists`, in a planted module

#### Scenario: A directory whose only Go files are integration tests

- **WHEN** a directory holds only `integration`-tagged test files, so `go list ./...` without the tag does not list it
- **THEN** `scripts/test-pkgs.sh integration` lists it

#### Scenario: The integration run reads the unit list

- **WHEN** `test:integration` sets `PKGS` from `scripts/test-pkgs.sh unit`
- **THEN** the pinning test fails naming `test:integration` and the required line
