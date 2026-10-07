# merge-gate Specification

## Purpose
The merge gate is what must hold before a pull request can merge to `main`. This capability covers the parts of it
that defend against flaky tests: how often and under which settings `task verify` runs the unit tests, that a
failing gate names the test, that no merge happens while a known flake is open, that a pull request's green was
produced on the current `main`, and that every CI job runs on one pinned runner image, `ubuntu-24.04`, so that two
runs of one commit get the same Ubuntu release. It also covers the review check: a code pull request that is not a
draft fails `merge-check` unless its description names the implementing agent and the other agent as its reviewer,
and marking a draft ready starts the run that applies it.

## Requirements

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

### Requirement: Cross-agent review check

On a pull-request run `scripts/merge-check.sh` SHALL apply a review check. A code pull request that is not a draft
passes only when its description says which agent implemented it and names the other agent as its reviewer. A
documents-only pull request passes without that. The script SHALL exit zero only when the review check, the
known-flake check and the up-to-date rule all pass. There SHALL be no waiver: no label, no comment and no
environment variable SHALL change the result, other than the two variables that select the kind of run. On a push
run the script SHALL NOT apply the check, SHALL say so, and SHALL NOT read a pull request or its files.

**Documents only.** The script SHALL read every page of the pull request's list of changed files, and the number of
changed files GitHub reports for it. Each entry gives one name, and a renamed file gives two: its new name and its
previous name. A name is a document name when it ends in `.md` or starts with `openspec/`, and does not start with
`.claude/agents/`. Every other name is a code name, including the name of a file the pull request deletes. A pull
request with at least one code name is a code pull request. Every other pull request is documents only, one that
changes no file included. When the entries read are not as many as GitHub reports, the script SHALL exit non-zero,
say the list is incomplete and give both numbers, whatever the names it read.

**The two lines.** The description of a code pull request SHALL have exactly one line that starts `reviewed-by:`
and, unless GitHub reports the pull request's author as a bot, exactly one line that starts `implemented-by:`. A
line starts in the first column. An agent name is the word `claude` or the word `codex`, in any letter case, with no
letter or digit directly before or after it. The `implemented-by:` line SHALL name at least one agent. The
`reviewed-by:` line SHALL name exactly one agent, and not an agent the `implemented-by:` line names, unless that
line names both. For a bot's pull request the `implemented-by:` line is not read, and the `reviewed-by:` line may
name either agent. Nothing else in either line is read. A carriage return at the end of a line SHALL NOT change any
of this.

**Drafts.** While the pull request is a draft the script SHALL make the same reads and print the same findings,
SHALL say that they do not fail a draft, and SHALL NOT exit non-zero because of them.

**Reads.** The script SHALL exit non-zero, naming the read, when the read of the pull request or of its files fails
or returns something other than what was asked for, and when the step that sorts the names fails. This holds for a
draft too. None of these is ever treated as documents only or as a pass.

**What is printed.** The script SHALL print whether the pull request is documents only or a code pull request, and
for a code pull request its code names, up to ten of them. For each finding it SHALL print what is missing and the
line to write. When a pull request that is not a draft has a finding, it SHALL also print that editing the
description starts no run, so the job has to be run again. When a code pull request passes it SHALL print the
reviewing agent.

#### Scenario: Documents only

- **WHEN** a pull request that is not a draft changes `docs/a.md` and `openspec/config.yaml`, and its description
  has neither line
- **THEN** the script exits zero and says the pull request is documents only

#### Scenario: One code file among documents

- **WHEN** a pull request that is not a draft changes `docs/a.md` and `scripts/x.sh`, and its description has no
  `reviewed-by:` line
- **THEN** the script exits non-zero, names `scripts/x.sh`, and says a `reviewed-by:` line is missing

#### Scenario: Names near the rule

- **WHEN** one pull request changes `AGENTS.md`, `.claude/skills/preflight/SKILL.md`, `x/.claude/agents/a.md`,
  `openspec/changes/x/.openspec.yaml` and `openspec/x.sh`, and a second changes those five and also `README.MD`,
  `docs/a.md.txt`, `openspecs/a.yaml`, `docs/openspec/a.yaml`, `Taskfile.yml`, `docs/admission-ledger.yaml`,
  `.claude/agents/semengine-reviewer.md`, `.codex/agents/semengine-reviewer.toml`, and deletes `scripts/x.sh`
- **THEN** the script says the first is documents only, and treats the second as a code pull request and names each
  of the nine files that the first does not change

#### Scenario: Renamed file

- **WHEN** the only entry in the list renames `scripts/x.sh` to `docs/x.md`
- **THEN** the script treats the pull request as a code pull request and names `scripts/x.sh`

#### Scenario: Code file on the second page

- **WHEN** a pull request changes 101 files, the first 100 listed are Markdown files and the last is `go.mod`, and
  GitHub's two pages reach the script either joined into one list or as two lists one after the other
- **THEN** the script treats it as a code pull request and names `go.mod`, in both cases

#### Scenario: File list incomplete

- **WHEN** GitHub reports 101 changed files, the list read has 100 entries, and every one is a Markdown file
- **THEN** the script exits non-zero, says the list has 100 of 101 files, and does not say documents only

#### Scenario: No changed file

- **WHEN** GitHub reports no changed file and the list read is empty
- **THEN** the script exits zero and says the pull request is documents only

#### Scenario: The other agent is named

- **WHEN** a code pull request that is not a draft has the line `implemented-by: claude (opus)` and the line
  `reviewed-by: codex semengine-reviewer — targeted (3.1–3.3 fixes) APPROVE at 0a86a9a; full-diff review pending.`
- **THEN** the script exits zero and prints `codex` as the reviewing agent; what follows the agent's name is not
  read

#### Scenario: No reviewed-by line

- **WHEN** a code pull request that is not a draft has the line `implemented-by: claude (opus)` and no
  `reviewed-by:` line
- **THEN** the script exits non-zero, says to add a line `reviewed-by: codex`, and says that editing the description
  starts no run

#### Scenario: Implementer line missing or naming no agent

- **WHEN** a code pull request whose author is a user has no `implemented-by:` line, or has the line
  `implemented-by: opus (semengine-developer); fable (orchestrating session)`
- **THEN** the script exits non-zero and says the `implemented-by:` line must name `claude` or `codex`

#### Scenario: Same agent implements and reviews

- **WHEN** a code pull request has the lines `implemented-by: claude-fable-5-1` and `reviewed-by: Claude (opus)`
- **THEN** the script exits non-zero and says the other agent, `codex`, reviews

#### Scenario: Reviewer line names no agent, both, or a longer word

- **WHEN** a code pull request has the line `implemented-by: claude`, and its `reviewed-by:` line reads
  `gpt-6-astra`, or `codex, then claude`, or `codex2`
- **THEN** the script exits non-zero each time and says the line must name exactly one of `claude` and `codex`

#### Scenario: Both agents implemented

- **WHEN** a code pull request has the lines `implemented-by: codex (gpt-6-sol); claude (review fixes)` and
  `reviewed-by: claude`
- **THEN** the script exits zero

#### Scenario: Pull request by a bot

- **WHEN** a code pull request whose author GitHub reports as a bot has no `implemented-by:` line and has the line
  `reviewed-by: codex`, or has the lines `implemented-by: codex` and `reviewed-by: codex`
- **THEN** the script exits zero in both cases

#### Scenario: Line not written once at the start of a line

- **WHEN** the description of a code pull request has two lines that start `reviewed-by:`, or has
  `- reviewed-by: codex` as a list item and no line that starts with it, or has one `reviewed-by:` line and two
  lines that start `implemented-by:`
- **THEN** the script exits non-zero each time, and says there are two `reviewed-by:` lines in the first case, none
  in the second, and two `implemented-by:` lines in the third

#### Scenario: Carriage returns

- **WHEN** every line of the description ends in a carriage return and a line feed, and the two lines are otherwise
  as required
- **THEN** the script exits zero

#### Scenario: Draft

- **WHEN** a draft code pull request has no `reviewed-by:` line, no known flake is open and the up-to-date rule
  holds
- **THEN** the script exits zero, prints that a `reviewed-by:` line is missing, and says this does not fail a draft

#### Scenario: A read for the review check fails

- **WHEN** the read of the pull request or of its files fails, or returns something other than what was asked for,
  on a pull request that is a draft or is not
- **THEN** the script exits non-zero naming the read, and does not say documents only or ok

#### Scenario: Sorting names fails

- **WHEN** the step that sorts the changed files into code names fails
- **THEN** the script exits non-zero, and does not say documents only

#### Scenario: Push run and the review check

- **WHEN** `GITHUB_ACTIONS` is `true` and `GITHUB_EVENT_NAME` is `push`
- **THEN** the script says the review check does not apply and reads no pull request and no file list

### Requirement: A run when a pull request is marked ready

The `pull_request` trigger of the CI workflow SHALL list the activity types `opened`, `synchronize`, `reopened` and
`ready_for_review`, so that a run starts when a pull request is opened, pushed to or reopened, and when a draft is
marked ready for review. The review check does not fail a draft, so the run that marking it ready starts is the one
that applies the check to the head that can now merge. A contract test SHALL fail when the workflow has no
`pull_request` trigger, when the trigger lists no activity types, and when it lacks any one of the four.

#### Scenario: Ready type missing

- **WHEN** the `pull_request` trigger in `.github/workflows/ci.yml` lists `opened`, `synchronize` and `reopened`, or
  lists no activity types, or the workflow has no `pull_request` trigger
- **THEN** the contract test fails each time naming `ready_for_review`

#### Scenario: A default type missing

- **WHEN** the `pull_request` trigger lists `opened`, `reopened` and `ready_for_review`
- **THEN** the contract test fails naming `synchronize`

### Requirement: Pinned runner image

Every job in the CI workflow SHALL run on the GitHub-hosted image `ubuntu-24.04`, written as the one label
`runs-on: ubuntu-24.04`: not `ubuntu-latest`, not another label, not a list of labels, not a runner group and not an
expression. `ubuntu-latest` moves to a new Ubuntu release on GitHub's schedule, with no change in this repository, and
a run of the same commit can then pass on one release and fail on the other. The label `ubuntu-24.04` still moves
between GitHub's builds of Ubuntu 24.04, and that is accepted. A move to another image is a change to this
requirement, made in its own pull request, whose CI run is the `task verify` run on the new image. A contract test
SHALL fail, naming the job and what it runs on, when any job in the workflow has no `runs-on`, or a `runs-on` other
than the one label `ubuntu-24.04`.

#### Scenario: A job on the moving label

- **WHEN** a job in `.github/workflows/ci.yml` has `runs-on: ubuntu-latest`, whether it is `verify`, `merge-check`,
  `required` or a job added beside them
- **THEN** the contract test fails naming the job, `ubuntu-latest` and `ubuntu-24.04`

#### Scenario: A job with no runner

- **WHEN** a job in `.github/workflows/ci.yml` has no `runs-on`
- **THEN** the contract test fails naming the job and `runs-on`

#### Scenario: Another label

- **WHEN** a job in `.github/workflows/ci.yml` has `runs-on: ubuntu-26.04` or `runs-on: ubuntu-24.04-arm`
- **THEN** the contract test fails naming the job and that label

#### Scenario: Not one label

- **WHEN** a job's `runs-on` is a list of labels that includes `ubuntu-24.04`, a runner group, or an expression
- **THEN** the contract test fails naming the job and what it runs on
