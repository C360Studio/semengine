# merge-gate

## ADDED Requirements

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

## REMOVED Requirements

### Requirement: Known-flake check

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Known-flake check reports on a pull-request run

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: No waiver for the known-flake check

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Kind of run comes from the Actions event

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: A pull-request run needs a number, and the run is printed

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The two run-kind variables do nothing else

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Known-flake check fails closed

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Known-flake check does not apply to a push run

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Up-to-date rule

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The up-to-date rule is read on every run

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Required needs both jobs

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The merge-check job runs read-only

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: A contract test guards the CI workflow

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Cross-agent review check

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: No waiver for the review check

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The review check does not apply to a push run

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The changed-file list is read whole

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Names are sorted into document names and code names

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The reviewed-by and implemented-by lines are written once

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The two lines name the agents

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Drafts are not failed by the review check

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The review check fails closed on its reads

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: The review check prints its findings

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: Each finding says how to fix it

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: A run when a pull request is marked ready

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.

### Requirement: A contract test guards the activity types

**Reason**: the merge check is `ateam review gate` at the pinned release. The rule this requirement stated is
restated in "The merge check is ateam review gate at the pin", "The merge-check workflow", "Required needs verify"
or "A contract test guards both workflows", or it was a mechanic of the retired script, whose oracle is now the pin's
own tests.

**Migration**: none; `task merge:check -- <n>` keeps its name and exit statuses.
