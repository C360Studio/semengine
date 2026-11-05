# Proposal: adopt-ateam-v037-probe

Track: quick (one pin bump and one workflow job; two existing requirements modified, no new Requirement).

## Why

SemEngine pins ateam v0.3.4. v0.3.5 added the `Merge check probe` job to ateam's `merge-check.yml` (ateam #262): a ready
code pull request with no review yet shows its required `Merge check` as missing rather than red, so a red `Merge check`
means a real gate failure again. v0.3.6 and v0.3.7 change nothing SemEngine uses (daemon task leases; Gitea over SSH).
`ateam doctor --check` at v0.3.7 reports one violation on `main`: `.agents/contracts/reviewer.md` lacks the new reviewer
paragraph that explains the probe.

Approved-by: Coby Leuschke, 2026-11-05, in the Enjoy conversation "ateam pin bump to v0.3.7" (chose "bring in the probe").

## What Changes

- The ateam pin moves from `v0.3.4` to `v0.3.7` in `.github/workflows/ci.yml`, `.github/workflows/merge-check.yml`, the
  merge-gate tool tests and `.agents/README.md`.
- `.github/workflows/merge-check.yml` gains the `Merge check probe` job, as ateam's own workflow has it, with
  `go install` of the pinned release in place of ateam's source build. The `merge-check` job needs it and is skipped and
  renamed while the probe answers `awaiting=true`; with no answer it runs in full.
- `.agents/contracts/reviewer.md` takes v0.3.7's template paragraph, so `ateam doctor --check` passes.
- `AGENTS.md` Merge gate: `Merge check` is absent until a review exists, then fails without one, in place of "fails once
  non-draft".

The `main` ruleset does not change: it keeps requiring `Required` and `Merge check`. A skipped job is listed under its
raw `name:` expression, never as `Merge check` (ateam v0.3.6 notes), so the required check is missing, and a missing
required check blocks the merge. A review record posted as a comment starts no run: rerun the whole Merge check run.

## Capabilities

- Modified `merge-gate` (the merge-check workflow requirement: probe sentence, two scenarios).
- Modified `harness-boundaries` (the ateam doctor requirement: pin `v0.3.7`).

## Impact

- `.github/workflows/{ci,merge-check}.yml`, `internal/harness/toolcheck/mergegate_test.go`, `.agents/README.md`,
  `.agents/contracts/reviewer.md`, `AGENTS.md`, the two specs.
