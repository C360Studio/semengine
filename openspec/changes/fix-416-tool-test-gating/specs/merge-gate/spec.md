# merge-gate

## MODIFIED Requirements

### Requirement: Varied and repeated unit runs

`task test:unit` SHALL run `go test -race -count=1 -cpu 1` over the unit packages. `task test:repeat` SHALL run
`go test -count=5 -cpu 1 -shuffle=on` over the packages given after `--`, or else the unit packages. The unit packages
are every package `go list ./...` names except the tool packages. A pinning test SHALL fail when either command line,
or the step list of `scripts/verify.sh`, differs from this requirement or from "Verify runs both unit steps, the repeat
step last".

#### Scenario: Count lowered

- **WHEN** the `test:repeat` command in `Taskfile.yml` carries `-count=1`
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: CPU setting dropped

- **WHEN** the `test:unit` command in `Taskfile.yml` has no `-cpu 1`
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: Race detector added to the repeat step

- **WHEN** the `test:repeat` command in `Taskfile.yml` carries `-race`
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: Repeat over every package

- **WHEN** the `test:repeat` command in `Taskfile.yml` defaults to `./...` instead of the unit packages
- **THEN** the pinning test fails naming the task and the required command

### Requirement: Verify runs both unit steps, the repeat step last

`scripts/verify.sh` SHALL run `task test:unit`, `task test:tools:changed` and `task test:repeat`, with `test:repeat`
last. Every test without a build tag in a unit package then runs once under the race detector at one CPU and five
times without it at one CPU in shuffled order. In a package with an `integration`-tagged file it also runs once under
the race detector on every CPU.

#### Scenario: Step removed from verify

- **WHEN** `scripts/verify.sh` does not list `test:repeat`
- **THEN** the pinning test fails naming the missing step

#### Scenario: Repeat step not last

- **WHEN** `scripts/verify.sh` lists a step after `test:repeat`
- **THEN** the pinning test fails saying `test:repeat` must be the last step

#### Scenario: Tool step removed from verify

- **WHEN** `scripts/verify.sh` does not list `test:tools:changed`
- **THEN** the pinning test fails naming the missing step

## ADDED Requirements

### Requirement: Tool tests and repository guards are separate packages

The tool packages SHALL be `internal/harness/toolcheck`, `internal/harness/mutcheck` and `internal/harness/pindiff`,
named once, in `scripts/test-pkgs.sh`. A test SHALL live in `toolcheck` when every file of the real tree it reads is
under a trigger path, and in `internal/harness/contract`, a unit package, when it reads any other file.
`task test:tools` SHALL run the tool packages with `go test -race -count=1 -cpu 1`.

#### Scenario: A tool package among the unit packages

- **WHEN** `scripts/test-pkgs.sh unit` lists `internal/harness/mutcheck`
- **THEN** `TestTestPkgsLists` fails naming the package

#### Scenario: The repository guards dropped from the unit packages

- **WHEN** `scripts/test-pkgs.sh unit` does not list `internal/harness/contract`
- **THEN** `TestTestPkgsLists` fails naming the package

#### Scenario: A real-tree test in toolcheck

- **WHEN** a test that runs `scripts/lint-test-ports.sh` over the real tree's test files is placed in `toolcheck`
- **THEN** review refuses it; no command fails, because which files a test reads is review only

### Requirement: Each test task reads its own package list

`test:unit` and `test:repeat` SHALL set `PKGS` from `scripts/test-pkgs.sh unit` and `test:tools` from
`scripts/test-pkgs.sh tools`, with no other `vars`. `test:tools:changed` SHALL run exactly `scripts/tools-changed.sh`.
`TestUnitInvocationsPinned` SHALL fail, naming the task, when its `vars` or its command differ from this.

#### Scenario: The unit run reads the tool list

- **WHEN** `test:unit` sets `PKGS` from `scripts/test-pkgs.sh tools`
- **THEN** the pinning test fails naming `test:unit` and the required line

#### Scenario: The repeat run loses its list

- **WHEN** `test:repeat` has no `PKGS` and its command defaults to `./...`
- **THEN** the pinning test fails naming `test:repeat` and the required line

#### Scenario: The tool step runs something else

- **WHEN** `test:tools:changed` runs `task test:tools` directly
- **THEN** the pinning test fails naming `test:tools:changed` and the required command

### Requirement: Tool tests run when their files change

`task test:tools:changed` SHALL run `task test:tools` when a changed file is under `scripts/`, `internal/harness/` or
`.github/`, or is `Taskfile.yml`, `go.mod`, `go.sum`, `docs/admission-ledger.yaml`, `service/base.go`,
`service/service_manager.go` or `service/component_manager.go`. Otherwise it SHALL exit zero after printing that the
tool tests were skipped and that no changed file matched.

#### Scenario: A script changed

- **WHEN** the changed files include `scripts/merge-check.sh`
- **THEN** `task test:tools` runs

#### Scenario: Only product code and docs changed

- **WHEN** the changed files are `graph/query_verbs.go` and `docs/testing.md`
- **THEN** `task test:tools` does not run and the output says the tool tests were skipped

#### Scenario: An untracked file under a trigger path

- **WHEN** the only change is an untracked file under `internal/harness/`
- **THEN** `task test:tools` runs

### Requirement: How the changed files are found

Outside GitHub Actions, the changed files SHALL be what `git diff --name-only --no-renames` reports against
`git merge-base HEAD origin/main`, plus untracked files that git does not ignore. In a `pull_request` run the base
SHALL be the merge base with `origin/$GITHUB_BASE_REF`, and CI's verify job SHALL check out with `fetch-depth: 0`.
A `push` run, any other event, or a git command that fails SHALL run `task test:tools`, saying why.

#### Scenario: A push to main

- **WHEN** `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` is `push`
- **THEN** `task test:tools` runs

#### Scenario: No base to compare against

- **WHEN** `origin/main` does not exist in a local clone
- **THEN** `task test:tools` runs and the output says no merge base was found

#### Scenario: Full history dropped from CI

- **WHEN** the verify job's checkout step in `.github/workflows/ci.yml` has no `fetch-depth: 0`
- **THEN** `TestCIWorkflowPinned` fails naming the job and the setting
