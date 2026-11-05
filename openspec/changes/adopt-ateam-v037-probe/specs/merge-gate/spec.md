# merge-gate

## MODIFIED Requirements

### Requirement: The merge-check workflow

`.github/workflows/merge-check.yml` SHALL run the merge check as its own workflow on `pull_request` (`opened`,
`synchronize`, `reopened`, `ready_for_review`, `edited`), one queued run per pull request, skipped on a draft,
checking out the base commit without credentials, installing the pinned release, and running
`scripts/merge-check.sh <n>` read-only with `ATEAM_REQUIRED_CHECKS` naming `Required` and `Merge check`, in 5 minutes.
A first job, `Merge check probe`, checks out the same base commit, installs the same release, and runs
`ateam review gate --awaiting <n>`; while it answers `awaiting=true` (a code pull request with no `reviewed-by:` line and
no review record comment) the `merge-check` job is skipped, and when the probe gives no answer the full check runs. The
`main` ruleset does not require `Merge check probe`.

#### Scenario: Reviewer edits the description

- **WHEN** the reviewing agent adds the `reviewed-by:` line to the pull request description
- **THEN** a `Merge check` run starts on the `edited` event and reads the record it just posted

#### Scenario: Draft carries no check

- **WHEN** the pull request is a draft
- **THEN** the `merge-check` job is skipped and no run named `Merge check` counts as passing on that head

#### Scenario: Pull request edits the gate

- **WHEN** a pull request changes `scripts/merge-check.sh` or `.agents/review-policy.json`
- **THEN** the run judging it uses the base commit's files, not the pull request's

#### Scenario: Code pull request awaiting its review

- **WHEN** a non-draft code pull request has no `reviewed-by:` line and no review record comment
- **THEN** the `merge-check` job is skipped, no run named `Merge check` exists on that head, and the required check is
  absent, which blocks the merge as a missing check, not a red one

#### Scenario: Probe fails

- **WHEN** the `Merge check probe` job fails or prints no `awaiting=` line
- **THEN** the `merge-check` job runs the full check
