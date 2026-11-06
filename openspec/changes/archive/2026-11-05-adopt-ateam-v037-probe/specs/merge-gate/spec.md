# merge-gate

## MODIFIED Requirements

### Requirement: The merge-check workflow

`.github/workflows/merge-check.yml` SHALL run the merge check as its own workflow on `pull_request` (`opened`,
`synchronize`, `reopened`, `ready_for_review`, `edited`), one queued run per pull request, skipped on a draft. Each job
checks out the base commit without credentials and installs the pinned release. A `Merge check probe` job runs
`ateam review gate --awaiting <n>`; unless it answers `awaiting=true`, the `merge-check` job runs
`scripts/merge-check.sh <n>` read-only in 5 minutes.

#### Scenario: Reviewer edits the description

- **WHEN** the reviewing agent adds the `reviewed-by:` line to the pull request description
- **THEN** a `Merge check` run starts on the `edited` event and reads the record it just posted

#### Scenario: Draft carries no check

- **WHEN** the pull request is a draft
- **THEN** the `merge-check` job is skipped and no run named `Merge check` counts as passing on that head

#### Scenario: Pull request edits the gate

- **WHEN** a pull request changes `scripts/merge-check.sh` or `.agents/review-policy.json`
- **THEN** the run judging it uses the base commit's files, not the pull request's

#### Scenario: Required checks named

- **WHEN** the `merge-check` job runs `scripts/merge-check.sh`
- **THEN** `ATEAM_REQUIRED_CHECKS` names `Required` and `Merge check`, and the `main` ruleset does not require
  `Merge check probe`

#### Scenario: Code pull request awaiting its review

- **WHEN** a non-draft code pull request has no `reviewed-by:` line and no review record comment
- **THEN** the `merge-check` job is skipped, no run named `Merge check` exists on that head, and the required check is
  absent, which blocks the merge as a missing check, not a red one

#### Scenario: Probe fails

- **WHEN** the `Merge check probe` job fails or prints no `awaiting=` line
- **THEN** the `merge-check` job runs the full check

### Requirement: A contract test guards both workflows

A contract test SHALL fail when `ci.yml` differs from "Required needs verify" or "The verify job has a time limit",
grants a write permission, or lists other `pull_request` types than `opened`, `synchronize`, `reopened`,
`ready_for_review`; and when `merge-check.yml` differs from "The merge-check workflow" in types, draft condition, the
`merge-check` job's name, concurrency, checkout ref, pin, environment, permissions or limit. Every install line SHALL
name one release.

#### Scenario: Edited type missing

- **WHEN** `merge-check.yml` lists `opened`, `synchronize`, `reopened` and `ready_for_review` only
- **THEN** the contract test fails naming `edited`

#### Scenario: Head checked out instead of base

- **WHEN** the checkout step of `merge-check.yml` has no `ref` of the base commit
- **THEN** the contract test fails naming the step

#### Scenario: Pins differ

- **WHEN** `ci.yml` installs `ateam@v0.3.4` and `merge-check.yml` installs `ateam@v0.3.3`
- **THEN** the contract test and `scripts/doctor.sh` fail naming both versions
