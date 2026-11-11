# merge-gate

## MODIFIED Requirements

### Requirement: Varied and repeated unit runs

`task test:unit` SHALL run `go test -race -count=1 -cpu 1 -timeout 5m` over the unit packages (every package
`go list ./...` names but the tool packages). `task test:repeat` SHALL run `go test -count=5 -cpu 1 -shuffle=on
-timeout 5m` over the packages after `--`, else the repeat packages (the unit packages but
`internal/harness/contract`). A pinning test SHALL fail when a command line or the `scripts/verify.sh` step list
differs from this or from the next requirement.

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

- **WHEN** the `test:repeat` command in `Taskfile.yml` defaults to `./...` instead of the repeat packages
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: Timeout dropped or widened

- **WHEN** the `test:unit` or `test:repeat` command in `Taskfile.yml` has no `-timeout 5m`
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: Why five minutes

- **WHEN** a unit package hangs
- **THEN** go test ends it at five minutes and prints every goroutine's stack, inside the 15-minute limit of CI's
  `checks` job; the bound is four times the slowest package's measured time (`internal/harness/contract`, 72 s on CI
  run 41442032443), and no test sets a shorter deadline of its own (`harness-boundaries`, "No own deadlines in tests")

### Requirement: Pinned runner image

Every job in the CI workflow SHALL run on the GitHub-hosted image `ubuntu-24.04`, written as the one label `runs-on:
ubuntu-24.04`: not `ubuntu-latest`, not another label, not a list of labels, not a runner group and not an expression.
`ubuntu-latest` moves to a new Ubuntu release on GitHub's schedule, with no change in this repository, and a run of the
same commit can then pass on one release and fail on the other.

#### Scenario: A job on the moving label

- **WHEN** a job in `.github/workflows/ci.yml` has `runs-on: ubuntu-latest`, whether it is `checks`, `integration`,
  `required` or a job added beside them
- **THEN** the contract test fails naming the job, `ubuntu-latest` and `ubuntu-24.04`

### Requirement: How the changed files are found

Outside GitHub Actions, the changed files SHALL be what `git diff --name-only --no-renames` reports against
`git merge-base HEAD origin/main`, plus untracked files that git does not ignore. In a `pull_request` run the base
SHALL be the merge base with `origin/$GITHUB_BASE_REF`, and CI's `checks` job, which runs `test:tools:changed`, SHALL
check out with `fetch-depth: 0`. A `push` run, any other event, or a git command that fails SHALL run
`task test:tools`, saying why.

#### Scenario: A push to main

- **WHEN** `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` is `push`
- **THEN** `task test:tools` runs

#### Scenario: No base to compare against

- **WHEN** `origin/main` does not exist in a local clone
- **THEN** `task test:tools` runs and the output says no merge base was found

#### Scenario: Full history dropped from CI

- **WHEN** the `checks` job's checkout step in `.github/workflows/ci.yml` has no `fetch-depth: 0`
- **THEN** `TestCIWorkflowPinned` fails naming the job and the setting

### Requirement: A contract test guards both workflows

A contract test SHALL fail when `ci.yml` differs from "Required needs both CI jobs", "Each CI job has a time limit" or
"CI runs verify as two parallel jobs", grants a write permission, or lists other `pull_request` types than `opened`,
`synchronize`, `reopened`, `ready_for_review`; and when `merge-check.yml` differs from "The merge-check workflow" in
types, draft condition, the `merge-check` job's name, concurrency, checkout ref, pin, environment, permissions or
limit. Every install line SHALL name one release.

#### Scenario: Edited type missing

- **WHEN** `merge-check.yml` lists `opened`, `synchronize`, `reopened` and `ready_for_review` only
- **THEN** the contract test fails naming `edited`

#### Scenario: Head checked out instead of base

- **WHEN** the checkout step of `merge-check.yml` has no `ref` of the base commit
- **THEN** the contract test fails naming the step

#### Scenario: Pins differ

- **WHEN** `ci.yml` installs `ateam@v0.3.4` and `merge-check.yml` installs `ateam@v0.3.3`
- **THEN** the contract test and `scripts/doctor.sh` fail naming both versions

## REMOVED Requirements

### Requirement: The verify job has a time limit

**Reason**: CI's one `verify` job is split into `checks` and `integration` (#599); each has its own limit.

**Migration**: "Each CI job has a time limit".

### Requirement: Required needs verify

**Reason**: there is no `verify` job after the split (#599); `required` needs both new jobs.

**Migration**: "Required needs both CI jobs".

## ADDED Requirements

### Requirement: Verify runs in groups

`scripts/verify.sh` SHALL take an optional group name and walk its one step list in order: with no name it runs every
step; `checks` runs every step but `test:integration` and `cover:check`; `integration` runs only `ateam:doctor`,
`test:integration` and `cover:check`. Each run SHALL print its host lines and step timings and fail when tracked files
changed. Any other name SHALL fail before any step. `task verify` SHALL pass the arguments after `--` to it.

#### Scenario: Full run

- **WHEN** `scripts/verify.sh` runs with no argument
- **THEN** it runs every step of its list in order, `test:integration` and `cover:check` included

#### Scenario: The checks group

- **WHEN** `scripts/verify.sh checks` runs
- **THEN** it runs every step of its list in order but `test:integration` and `cover:check`

#### Scenario: The integration group

- **WHEN** `scripts/verify.sh integration` runs
- **THEN** it runs `ateam:doctor`, `test:integration` and `cover:check`, in that order, and no other step

#### Scenario: An unknown group

- **WHEN** `scripts/verify.sh bogus` runs
- **THEN** it runs no step, exits non-zero and names `checks` and `integration`

### Requirement: CI runs verify as two parallel jobs

In `.github/workflows/ci.yml` the job `checks` SHALL run `task verify -- checks` and the job `integration` SHALL run
`task verify -- integration`, neither needing the other, so they run in parallel. Local `task verify` with no argument
stays the full gate (owner ruling of 2026-11-11, #599).

#### Scenario: A job loses its group

- **WHEN** the `integration` job runs `task verify` with no group, or both jobs run the same group
- **THEN** the contract test fails naming the job and its required run line

### Requirement: Each CI job has a time limit

In `.github/workflows/ci.yml` the job `checks` SHALL have a limit of 15 minutes and the job `integration` a limit of
20 minutes, each about twice its estimated normal run time: on `ubuntu-24.04` with 4 cores, the single job's step
timings put `checks` at 4 to 7 minutes and `integration` at 8 to 10 minutes, setup included; these are starting budgets,
not measurements of the split jobs (owner ruling of 2026-11-11, #599).

#### Scenario: A limit changed

- **WHEN** the `checks` job has a limit other than 15 minutes, or the `integration` job one other than 20 minutes
- **THEN** the contract test fails naming the job and the limit

#### Scenario: A limit removed

- **WHEN** the `checks` or `integration` job has no `timeout-minutes`
- **THEN** the contract test fails naming the job and the required limit

### Requirement: Required needs both CI jobs

The job `required` in `.github/workflows/ci.yml` SHALL need `checks` and `integration` and SHALL fail unless both
succeeded, running whatever the results (`if: always()`) and exiting non-zero for a failed, cancelled, skipped or
missing result. The `main` ruleset SHALL require both `Required` and `Merge check` with the up-to-date setting on; the
gate's first rule reads that ruleset on every run.

#### Scenario: One job fails

- **WHEN** the `checks` job passes and the `integration` job fails on a pull-request run
- **THEN** `Required` fails on that head

#### Scenario: One job runs out of time

- **WHEN** the `integration` job is cancelled at its limit after its steps passed
- **THEN** `Required` fails naming the result `cancelled`

#### Scenario: A job dropped from the needs

- **WHEN** `required` needs `checks` alone
- **THEN** the contract test fails naming `integration`

#### Scenario: Merge check dropped from the ruleset

- **WHEN** the `main` ruleset no longer requires `Merge check`
- **THEN** the next `Merge check` run fails its first rule naming the missing check
