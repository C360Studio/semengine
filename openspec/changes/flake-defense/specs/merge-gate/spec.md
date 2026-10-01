# merge-gate

## Purpose

The merge gate is what must hold before a pull request can merge to `main`. This capability covers the parts of it
that defend against flaky tests: how often and under which settings `task verify` runs the unit tests, that a
failing gate names the test, that no merge happens while a known flake is open, and that a pull request's green was
produced on the current `main`.

## ADDED Requirements

### Requirement: Varied and repeated unit runs

`task test:unit` SHALL run `scripts/gopkgs.sh go test -race -count=1 -cpu 1 ./...`. `task test:repeat` SHALL run
`scripts/gopkgs.sh go test -count=5 -cpu 1 -shuffle=on` over `./...`, or over the packages given after `--`.
`scripts/verify.sh` SHALL run both, with `test:repeat` as its last step. Together with the integration runner's
invocation, every test without a build tag then runs once under the race detector at one CPU, once under the race
detector on every CPU, and five times without the race detector at one CPU in shuffled order. A contract test SHALL
fail when either command line, or the step list, differs from this requirement.

#### Scenario: Count lowered

- **WHEN** the `test:repeat` command in `Taskfile.yml` carries `-count=1`
- **THEN** the contract test fails naming the task and the required command

#### Scenario: CPU setting dropped

- **WHEN** the `test:unit` command in `Taskfile.yml` has no `-cpu 1`
- **THEN** the contract test fails naming the task and the required command

#### Scenario: Step removed from verify

- **WHEN** `scripts/verify.sh` does not list `test:repeat`
- **THEN** the contract test fails naming the missing step

### Requirement: Coverage gate names a failing test

`scripts/cover-check.sh` SHALL print the output of the `go test` run that produces its unit profile, so that a test
failing in that run is named in the output of `task cover:check`.

#### Scenario: Unit test fails in the coverage run

- **WHEN** a test fails in the `go test` run that `scripts/cover-check.sh` starts
- **THEN** the script exits non-zero and its output contains that test's `--- FAIL` line

### Requirement: Known-flake check

A known flake is an open GitHub issue labelled `class:flake`. `scripts/merge-check.sh`, given a pull request number,
SHALL exit non-zero naming every open `class:flake` issue, unless the pull request's closing references include at
least one of them, in which case it SHALL exit zero and print which issue exempts it. It SHALL exit non-zero when
the label `class:flake` does not exist, and when any read from GitHub fails, naming the read; a failed read is never
treated as no known flake. Given no pull request number it SHALL NOT apply this check. No comment, no label on the
pull request and no environment variable SHALL exempt a pull request.

#### Scenario: Open flake and an unrelated pull request

- **WHEN** issue 40 is open with the label `class:flake` and the pull request closes no `class:flake` issue
- **THEN** the script exits non-zero and its output names issue 40

#### Scenario: Pull request that closes the flake

- **WHEN** issue 40 is open with the label `class:flake` and the pull request's closing references include 40
- **THEN** the script exits zero and prints that closing issue 40 exempts the pull request

#### Scenario: No open flake

- **WHEN** the label exists and no open issue carries it
- **THEN** the script exits zero

#### Scenario: Label missing

- **WHEN** the repository has no label `class:flake`
- **THEN** the script exits non-zero saying the label is missing

#### Scenario: A read fails

- **WHEN** the read of the issue list, the pull request or the label fails
- **THEN** the script exits non-zero naming the read that failed, and does not report "no known flake"

#### Scenario: Push run

- **WHEN** the script runs with no pull request number
- **THEN** it says the known-flake check does not apply and checks the ruleset only

### Requirement: Ruleset record

`.github/rulesets/main-required-checks.json` SHALL record the `id`, `name`, `enforcement`, `conditions` and `rules`
of the ruleset that protects the default branch. Its `rules` SHALL require the status check `Required` with
`strict_required_status_checks_policy` set to `true`, so that a pull request whose head does not contain the tip of
`main` cannot merge. `scripts/merge-check.sh` SHALL read the live ruleset on every run and exit non-zero, printing
the recorded and the live value, when `enforcement`, `conditions` or `rules` differ, or when the record does not
require `Required` with the strict setting on.

#### Scenario: Setting turned off in GitHub

- **WHEN** the live ruleset reports `strict_required_status_checks_policy` as `false`
- **THEN** the script exits non-zero and prints the recorded and the live rule

#### Scenario: Record edited to turn the setting off

- **WHEN** the record's `rules` do not require `Required` with the strict setting on
- **THEN** the script exits non-zero saying what the record must require

### Requirement: Required needs both jobs

The CI workflow SHALL run `scripts/merge-check.sh` in a job `merge-check`: with the pull request's number on a
pull-request run and with none on a push run, under a token limited to `contents: read`, `issues: read` and
`pull-requests: read`. The job `required` SHALL need `verify` and `merge-check` and SHALL fail unless both
succeeded. A contract test SHALL fail when `required` does not need both.

#### Scenario: Job dropped from Required

- **WHEN** `required` in `.github/workflows/ci.yml` needs only `verify`
- **THEN** the contract test fails naming the missing job

#### Scenario: Merge check fails

- **WHEN** the `merge-check` job fails on a pull-request run
- **THEN** `Required` fails on that head
