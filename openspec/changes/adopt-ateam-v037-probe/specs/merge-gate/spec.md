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
