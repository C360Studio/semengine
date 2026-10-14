# integration-test-runner Specification

## Purpose
How SemEngine runs its Docker-backed tests: `task test:integration` is plain `go test` with pinned flags, and it
records which tree its coverage profile measured, so `task cover:check` can refuse a profile taken from another tree.

## Requirements

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
