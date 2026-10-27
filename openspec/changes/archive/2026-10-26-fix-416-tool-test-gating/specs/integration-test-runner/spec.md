# integration-test-runner

## MODIFIED Requirements

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

## ADDED Requirements

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
