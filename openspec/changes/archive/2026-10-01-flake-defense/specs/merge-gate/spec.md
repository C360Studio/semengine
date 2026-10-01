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
detector on every CPU, and five times without the race detector at one CPU in shuffled order. Tests tagged
`integration` SHALL NOT be repeated: they stay at the one execution the `integration-test-runner` capability fixes. A
contract test SHALL fail when either command line, or the step list, differs from this requirement.

#### Scenario: Count lowered

- **WHEN** the `test:repeat` command in `Taskfile.yml` carries `-count=1`
- **THEN** the contract test fails naming the task and the required command

#### Scenario: CPU setting dropped

- **WHEN** the `test:unit` command in `Taskfile.yml` has no `-cpu 1`
- **THEN** the contract test fails naming the task and the required command

#### Scenario: Race detector added to the repeat step

- **WHEN** the `test:repeat` command in `Taskfile.yml` carries `-race`
- **THEN** the contract test fails naming the task and the required command

#### Scenario: Step removed from verify

- **WHEN** `scripts/verify.sh` does not list `test:repeat`
- **THEN** the contract test fails naming the missing step

#### Scenario: Repeat step not last

- **WHEN** `scripts/verify.sh` lists a step after `test:repeat`
- **THEN** the contract test fails saying `test:repeat` must be the last step

### Requirement: Coverage gate names a failing test

`scripts/cover-check.sh` SHALL print the output of the `go test` run that produces its unit profile, so that a test
failing in that run is named in the output of `task cover:check`.

#### Scenario: Unit test fails in the coverage run

- **WHEN** a test fails in the `go test` run that `scripts/cover-check.sh` starts
- **THEN** the script exits non-zero and its output contains that test's `--- FAIL` line

### Requirement: Known-flake check

A known flake is an open GitHub issue labelled `class:flake`. While one is open, `scripts/merge-check.sh` SHALL fail
every pull-request run except that of a pull request whose closing references include every open one. There SHALL be
no waiver: no comment and no label on the pull request SHALL exempt it, and no environment variable SHALL change which
checks apply, other than the two that select the kind of run as stated below.

The label is for a failure that a pull request in this repository can end: a test or check that passes and fails on
the same tree. A failed network fetch SHALL NOT be filed under it. No command sorts the two, so this part of the rule
is checked in review only.

The script SHALL take the kind of run from `GITHUB_EVENT_NAME` only when `GITHUB_ACTIONS` is `true`. There,
`pull_request` is a pull-request run and `push` is a push run; any other value, or none, SHALL make the script exit
non-zero naming the event. When `GITHUB_ACTIONS` is not `true` the script SHALL NOT read `GITHUB_EVENT_NAME`, and the
run is a pull-request run. A pull-request run needs a pull request number and SHALL exit non-zero without one. The
script SHALL print the kind of run and, on a pull-request run, the number it checks. The two variables select the kind
of run and do nothing else. A local run with both set by hand takes the push path, and no command can tell that from a
real push run, so the rule that a local run is made with `GITHUB_ACTIONS` unset is checked in review only.

On a pull-request run the script SHALL exit non-zero naming every open `class:flake` issue that is not among the pull
request's closing references. When every open `class:flake` issue is among them it SHALL exit zero, print each
exempting issue, and print a `::warning::` line naming them. Issues SHALL be compared by URL. The script SHALL exit
non-zero when the label `class:flake` does not exist, when any read from GitHub fails or returns something other than
the list asked for, naming the read, and when the issue list reaches the number asked for; none of these is ever
treated as no known flake. The base branch of the pull request SHALL NOT change any of this. On a push run the script
SHALL NOT apply this check and SHALL say so.

#### Scenario: Open flake and an unrelated pull request

- **WHEN** issue 40 is open with the label `class:flake` and the pull request closes no `class:flake` issue
- **THEN** the script exits non-zero and its output names issue 40

#### Scenario: Pull request that closes the only open flake

- **WHEN** issue 40 is the only open issue labelled `class:flake` and the pull request's closing references
  include it
- **THEN** the script exits zero, prints that closing issue 40 exempts the pull request, and prints a
  `::warning::` line naming issue 40

#### Scenario: Two open flakes and a pull request that closes one

- **WHEN** issues 40 and 52 are open with the label `class:flake` and the pull request's closing references
  include 40 only
- **THEN** the script exits non-zero and its output names issue 52

#### Scenario: Two open flakes and a pull request that closes both

- **WHEN** issues 40 and 52 are open with the label `class:flake` and the pull request's closing references
  include both
- **THEN** the script exits zero and names both as exempting it

#### Scenario: Same number in another repository

- **WHEN** issue 40 is open with the label `class:flake` and the pull request's only closing reference is issue 40
  of another repository
- **THEN** the script exits non-zero and its output names issue 40

#### Scenario: Pull request on another base

- **WHEN** issue 40 is open with the label `class:flake` and a pull request whose base is not the default branch
  has no closing references
- **THEN** the script exits non-zero and its output names issue 40

#### Scenario: No open flake

- **WHEN** the label exists and no open issue carries it
- **THEN** the script exits zero

#### Scenario: Label missing

- **WHEN** the repository has no label `class:flake`
- **THEN** the script exits non-zero saying the label is missing

#### Scenario: A read fails

- **WHEN** the read of the issue list, the pull request or the label fails, or returns something other than a list
- **THEN** the script exits non-zero naming the read that failed, and does not report "no known flake"

#### Scenario: List may be cut short

- **WHEN** the read of open `class:flake` issues returns as many issues as the script asked for
- **THEN** the script exits non-zero saying the list may be incomplete

#### Scenario: Push run

- **WHEN** `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` is `push`
- **THEN** the script says the known-flake check does not apply and checks the up-to-date rule only

#### Scenario: Push event named outside Actions

- **WHEN** `GITHUB_ACTIONS` is not `true`, `GITHUB_EVENT_NAME` is `push`, a pull request number is given, and issue 40
  is open with the label `class:flake` and is not among that pull request's closing references
- **THEN** the script says it is a pull-request run, exits non-zero and names issue 40

#### Scenario: Pull-request run without a number

- **WHEN** `GITHUB_ACTIONS` is `true`, `GITHUB_EVENT_NAME` is `pull_request` and no pull request number is given
- **THEN** the script exits non-zero saying a pull request number is required

#### Scenario: Local run without a number

- **WHEN** `GITHUB_ACTIONS` is not `true` and no pull request number is given
- **THEN** the script exits non-zero saying a pull request number is required

#### Scenario: Event with no rule

- **WHEN** `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` is `merge_group`
- **THEN** the script exits non-zero naming the event

#### Scenario: No event inside Actions

- **WHEN** `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` is not set
- **THEN** the script exits non-zero saying the event is missing

### Requirement: Up-to-date rule

The rules in force on the default branch SHALL require the status check `Required` with
`strict_required_status_checks_policy` set to `true`, in a ruleset whose enforcement is `active`, so that a pull
request whose head does not contain the tip of `main` cannot merge. `scripts/merge-check.sh` SHALL read the rules
in force on the default branch from GitHub on every run and exit non-zero, printing what it found, unless a rule of
type `required_status_checks` has the context `Required` and the strict setting on and the ruleset it comes from
reports `enforcement` as `active`. No other field of those answers SHALL change the script's result.

#### Scenario: Setting turned off in GitHub

- **WHEN** the rule that requires `Required` reports `strict_required_status_checks_policy` as `false`
- **THEN** the script exits non-zero and prints the rule it found

#### Scenario: No rule requires the check

- **WHEN** no rule in force on the default branch has the context `Required`
- **THEN** the script exits non-zero saying no rule in force requires `Required`

#### Scenario: Ruleset not active

- **WHEN** the ruleset the rule comes from reports `enforcement` as `evaluate` or `disabled`
- **THEN** the script exits non-zero and prints the enforcement it found

#### Scenario: Field the script does not read

- **WHEN** GitHub's answers carry a field the script does not name, and the rule, its context, the strict setting
  and the enforcement are as required
- **THEN** the script exits zero

### Requirement: Required needs both jobs

The CI workflow SHALL run `scripts/merge-check.sh` in a job `merge-check`, passing the pull request's number on a
pull-request run, under a token limited to `contents: read`, `issues: read` and `pull-requests: read`. No job in
the workflow SHALL be granted a write permission. The job `verify` SHALL have a limit of 15 minutes. The job
`required` SHALL need `verify` and `merge-check` and SHALL fail unless both succeeded. For that it SHALL run whatever
the results of the jobs it needs (`if: always()`), and its step SHALL read the result of every job it needs and exit
non-zero unless each is `success`: a needed job that failed, was cancelled or was skipped fails `Required`, and so
does an empty list of results. A contract test SHALL fail when the workflow differs from this requirement in any of
these ways: the permissions of `merge-check`, a write permission on any job, the limit of `verify`, the jobs
`required` needs, the condition `required` runs under, or what its step does with the results.

#### Scenario: Job dropped from Required

- **WHEN** `required` in `.github/workflows/ci.yml` needs only `verify`
- **THEN** the contract test fails naming the missing job

#### Scenario: Merge check fails

- **WHEN** the `merge-check` job fails on a pull-request run
- **THEN** `Required` fails on that head

#### Scenario: Needed job cancelled or skipped

- **WHEN** the step of `required`, as the workflow writes it, is given `success` for one job it needs and `failure`,
  `cancelled` or `skipped` for the other, or no result at all
- **THEN** the step exits non-zero

#### Scenario: Required can be skipped

- **WHEN** `required` in `.github/workflows/ci.yml` has no `if: always()`
- **THEN** the contract test fails naming the job and the missing condition

#### Scenario: Step passes a result other than success

- **WHEN** the step of `required` in `.github/workflows/ci.yml` exits zero for a needed job whose result is
  `skipped`, or takes its results from `verify` alone
- **THEN** the contract test fails naming the job and what its step let through

#### Scenario: Write permission added

- **WHEN** a job in `.github/workflows/ci.yml` is granted `issues: write`
- **THEN** the contract test fails naming the job and the permission

#### Scenario: Permission added to the merge check

- **WHEN** the `merge-check` job is granted a permission other than its three reads
- **THEN** the contract test fails naming the permission

#### Scenario: Verify limit changed

- **WHEN** the `verify` job in `.github/workflows/ci.yml` has a limit other than 15 minutes
- **THEN** the contract test fails naming the limit
