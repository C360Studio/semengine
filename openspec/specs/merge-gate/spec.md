# merge-gate Specification

## Purpose
The merge gate is what must hold before a pull request can merge to `main`. This capability covers the parts of it
that defend against flaky tests: how often and under which settings `task verify` runs the unit tests, that a
failing gate names the test, and that every CI job runs on one pinned runner image, `ubuntu-24.04`, so that two
runs of one commit get the same Ubuntu release. It also covers the merge check: `scripts/merge-check.sh` runs
`ateam review gate` at the pinned ateam release, from its own `merge-check.yml` workflow on the base commit, as the
required check `Merge check`. That gate, not this repository, states the rules it applies (the up-to-date rule, no
open known flake the pull request leaves open, and for a code pull request that is not a draft an approving record at
its head by a different, not-weaker agent); this specification covers how the gate is wired and pinned.

## Requirements

### Requirement: Varied and repeated unit runs

`task test:unit` SHALL run `go test -race -count=1 -cpu 1` over the unit packages: every package `go list ./...`
names but the tool packages. `task test:repeat` SHALL run `go test -count=5 -cpu 1 -shuffle=on` over the packages
after `--`, else the repeat packages: the unit packages but `internal/harness/contract`. A pinning test SHALL fail
when a command line or the `scripts/verify.sh` step list differs from this or from "Verify runs both unit steps, the
repeat step last".

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

### Requirement: Verify runs both unit steps, the repeat step last

`scripts/verify.sh` SHALL run `task test:unit`, `task test:tools:changed` and `task test:repeat`, with `test:repeat`
last. Every test without a build tag in a unit package then runs once under the race detector at one CPU, and in a
repeat package also five times without it at one CPU in shuffled order. In a package with an `integration`-tagged
file it also runs once under the race detector on every CPU.

#### Scenario: Step removed from verify

- **WHEN** `scripts/verify.sh` does not list `test:repeat`
- **THEN** the pinning test fails naming the missing step

#### Scenario: Repeat step not last

- **WHEN** `scripts/verify.sh` lists a step after `test:repeat`
- **THEN** the pinning test fails saying `test:repeat` must be the last step

#### Scenario: Tool step removed from verify

- **WHEN** `scripts/verify.sh` does not list `test:tools:changed`
- **THEN** the pinning test fails naming the missing step

### Requirement: Integration tests are not repeated

Tests tagged `integration` SHALL NOT be repeated: they stay at the one execution the `integration-test-runner`
capability fixes.

#### Scenario: Integration test under the repeat step

- **WHEN** a test carries the `integration` tag
- **THEN** it is not run by `task test:repeat` and stays at the one execution the `integration-test-runner` capability
  fixes

### Requirement: Coverage gate names a failing test

`scripts/cover-check.sh` SHALL print the output of the `go test` run that produces its unit profile, so that a test
failing in that run is named in the output of `task cover:check`.

#### Scenario: Unit test fails in the coverage run

- **WHEN** a test fails in the `go test` run that `scripts/cover-check.sh` starts
- **THEN** the script exits non-zero and its output contains that test's `--- FAIL` line

### Requirement: The flake label is for failures a pull request can end

The label `class:flake` is for a failure that a pull request in this repository can end: a test or check that passes and
fails on the same tree. A failed network fetch SHALL NOT be filed under it. No command sorts the two, so this part of
the rule is checked in review only.

#### Scenario: Failed network fetch

- **WHEN** a network fetch fails
- **THEN** it is not filed under `class:flake`; review checks this, as no command sorts the two

### Requirement: The verify job has a time limit

The job `verify` SHALL have a limit of 15 minutes.

#### Scenario: Verify limit changed

- **WHEN** the `verify` job in `.github/workflows/ci.yml` has a limit other than 15 minutes
- **THEN** the contract test fails naming the limit

### Requirement: Pinned runner image

Every job in the CI workflow SHALL run on the GitHub-hosted image `ubuntu-24.04`, written as the one label `runs-on:
ubuntu-24.04`: not `ubuntu-latest`, not another label, not a list of labels, not a runner group and not an expression.
`ubuntu-latest` moves to a new Ubuntu release on GitHub's schedule, with no change in this repository, and a run of the
same commit can then pass on one release and fail on the other.

#### Scenario: A job on the moving label

- **WHEN** a job in `.github/workflows/ci.yml` has `runs-on: ubuntu-latest`, whether it is `verify`, `merge-check`,
  `required` or a job added beside them
- **THEN** the contract test fails naming the job, `ubuntu-latest` and `ubuntu-24.04`

### Requirement: The runner image moves only in its own pull request

The label `ubuntu-24.04` still moves between GitHub's builds of Ubuntu 24.04, and that is accepted. A move to another
image SHALL be a change to the requirement "Pinned runner image", made in its own pull request, whose CI run is the
`task verify` run on the new image. No command checks this; it is checked in review only.

#### Scenario: A move to a new Ubuntu release

- **WHEN** the runner image is to move from `ubuntu-24.04` to another image
- **THEN** one pull request changes "Pinned runner image" and the workflow labels together, and its `task verify` run
  on the new image is the evidence for the move

### Requirement: A contract test guards the runner label

A contract test SHALL fail, naming the job and what it runs on, when any job in the workflow has no `runs-on`, or a
`runs-on` other than the one label `ubuntu-24.04`.

#### Scenario: A job with no runner

- **WHEN** a job in `.github/workflows/ci.yml` has no `runs-on`
- **THEN** the contract test fails naming the job and `runs-on`

#### Scenario: Another label

- **WHEN** a job in `.github/workflows/ci.yml` has `runs-on: ubuntu-26.04` or `runs-on: ubuntu-24.04-arm`
- **THEN** the contract test fails naming the job and that label

#### Scenario: Not one label

- **WHEN** a job's `runs-on` is a list of labels that includes `ubuntu-24.04`, a runner group, or an expression
- **THEN** the contract test fails naming the job and what it runs on

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

`test:unit` SHALL set `PKGS` from `scripts/test-pkgs.sh unit`, `test:repeat` from `scripts/test-pkgs.sh repeat` and
`test:tools` from `scripts/test-pkgs.sh tools`, with no other `vars`. `test:tools:changed` SHALL run exactly `scripts/tools-changed.sh`.
`TestUnitInvocationsPinned` SHALL fail, naming the task, when its `vars` or its command differ from this.

#### Scenario: The unit run reads the tool list

- **WHEN** `test:unit` sets `PKGS` from `scripts/test-pkgs.sh tools`
- **THEN** the pinning test fails naming `test:unit` and the required line

#### Scenario: The repeat run reads the unit list

- **WHEN** `test:repeat` sets `PKGS` from `scripts/test-pkgs.sh unit`
- **THEN** the pinning test fails naming `test:repeat` and the required line

#### Scenario: The repeat run loses its list

- **WHEN** `test:repeat` has no `PKGS` and its command defaults to `./...`
- **THEN** the pinning test fails naming `test:repeat` and the required line

#### Scenario: The tool step runs something else

- **WHEN** `test:tools:changed` runs `task test:tools` directly
- **THEN** the pinning test fails naming `test:tools:changed` and the required command

### Requirement: Tool tests run when their files change

`task test:tools:changed` SHALL run `task test:tools` when a changed file is under `scripts/`, `internal/harness/`,
`.github/`, `.agents/`, `.claude/`, `.codex/` or `.gemini/`, or is `Taskfile.yml`, `go.mod`, `go.sum`,
`docs/admission-ledger.yaml`, `service/base.go`, `service/service_manager.go`, `service/component_manager.go` or
`.mcp.json`. Otherwise it SHALL exit zero after printing that the tool tests were skipped and that no changed file
matched.

#### Scenario: A script changed

- **WHEN** the changed files include `scripts/merge-check.sh`
- **THEN** `task test:tools` runs

#### Scenario: An ateam template changed

- **WHEN** the changed files include `.agents/contracts/reviewer.md`
- **THEN** `task test:tools` runs, so `TestAteamTemplatesUnchanged` checks it

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

### Requirement: The merge check is ateam review gate at the pin

`scripts/merge-check.sh` SHALL only run `ateam review gate` (`ATEAM`, else `ateam` on `PATH`) with its arguments;
`task merge:check -- <n>` calls it. At the pinned release the gate checks: the `main` rules require every
check in `ATEAM_REQUIRED_CHECKS` with up-to-date on; a non-draft code pull request has both attestation lines with
`(model, effort)`, a reviewer not weaker than every implementer, and an approving record no later code commit
follows; no open `class:flake` issue it leaves open.

#### Scenario: Wrapper passes the exit status through

- **WHEN** `scripts/merge-check.sh 7` runs with `ATEAM` naming a program that exits 0, 1 or 2
- **THEN** the script exits with the same status, having passed `review gate 7` to the program

#### Scenario: Record missing at the head

- **WHEN** a non-draft code pull request's latest review record names an earlier commit and a later commit changed
  a code file
- **THEN** the gate fails naming the commit the record covers and the one it does not

#### Scenario: Documents-only pull request

- **WHEN** every changed file ends in `.md`, or in `.yml`, `.yaml` or `.json` under `openspec/`, and none is under
  `.claude/agents/` or `.agents/contracts/`
- **THEN** the review check passes without reading the attestation lines

### Requirement: The merge-check workflow

`.github/workflows/merge-check.yml` SHALL run the merge check as its own workflow on `pull_request` (`opened`,
`synchronize`, `reopened`, `ready_for_review`, `edited`), one queued run per pull request, skipped on a draft,
checking out the base commit without credentials, installing the pinned release, and running
`scripts/merge-check.sh <n>` read-only with `ATEAM_REQUIRED_CHECKS` naming `Required` and `Merge check`, in 5 minutes.

#### Scenario: Reviewer edits the description

- **WHEN** the reviewing agent adds the `reviewed-by:` line to the pull request description
- **THEN** a `Merge check` run starts on the `edited` event and reads the record it just posted

#### Scenario: Draft carries no check

- **WHEN** the pull request is a draft
- **THEN** the `merge-check` job is skipped and no run named `Merge check` counts as passing on that head

#### Scenario: Pull request edits the gate

- **WHEN** a pull request changes `scripts/merge-check.sh` or `.agents/review-policy.json`
- **THEN** the run judging it uses the base commit's files, not the pull request's

### Requirement: Required needs verify

The job `required` in `.github/workflows/ci.yml` SHALL need `verify` and SHALL fail unless it succeeded, running
whatever the result (`if: always()`) and exiting non-zero for a failed, cancelled, skipped or missing result. The
`main` ruleset SHALL require both `Required` and `Merge check` with the up-to-date setting on; the gate's first rule
reads that ruleset on every run.

#### Scenario: Verify fails

- **WHEN** the `verify` job fails on a pull-request run
- **THEN** `Required` fails on that head

#### Scenario: Merge check dropped from the ruleset

- **WHEN** the `main` ruleset no longer requires `Merge check`
- **THEN** the next `Merge check` run fails its first rule naming the missing check

### Requirement: A contract test guards both workflows

A contract test SHALL fail when `ci.yml` differs from "Required needs verify" or "The verify job has a time limit",
grants a write permission, or lists other `pull_request` types than `opened`, `synchronize`, `reopened`,
`ready_for_review`; and when `merge-check.yml` differs from "The merge-check workflow" in types, draft condition,
concurrency, checkout ref, pin, environment, permissions or limit. Every install line SHALL name one release.

#### Scenario: Edited type missing

- **WHEN** `merge-check.yml` lists `opened`, `synchronize`, `reopened` and `ready_for_review` only
- **THEN** the contract test fails naming `edited`

#### Scenario: Head checked out instead of base

- **WHEN** the checkout step of `merge-check.yml` has no `ref` of the base commit
- **THEN** the contract test fails naming the step

#### Scenario: Pins differ

- **WHEN** `ci.yml` installs `ateam@v0.3.4` and `merge-check.yml` installs `ateam@v0.3.3`
- **THEN** the contract test and `scripts/doctor.sh` fail naming both versions
