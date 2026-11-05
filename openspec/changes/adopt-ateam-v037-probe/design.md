# Design: adopt-ateam-v037-probe

Base: `c509b25`. Track: `design` (two capabilities change: `merge-gate` and `harness-boundaries`). Draft PR #523,
`Closes #471`. Owner choice: 2026-11-05, Enjoy conversation "ateam pin bump to v0.3.7" ("bring in the probe").

Terms: the *probe* is the `Merge check probe` job. It runs `ateam review gate --awaiting <n>` and prints
`awaiting=true` for a code pull request that has no `reviewed-by:` line and no review record comment, else
`awaiting=false`.

## Context

ateam v0.3.5 added the probe to its own `merge-check.yml` (ateam #262), so that a ready code pull request with no review
yet shows its required `Merge check` as missing instead of red. SemEngine pinned v0.3.4. v0.3.6 and v0.3.7 change
nothing SemEngine uses. At v0.3.7, `ateam doctor --check` and `TestAteamTemplatesUnchanged` flag three template files.

## Decisions

- **D1. The workflow follows ateam's.** `.github/workflows/merge-check.yml` takes the probe job and the `merge-check`
  job's `needs`, `if` and `name` exactly as in ateam v0.3.7's workflow (lines 38-39 and 81-83). One difference: both
  jobs `go install` the pinned release where ateam builds from source, because SemEngine is not ateam's repository.
- **D2. It fails closed.** The `merge-check` job runs under `always()` unless the probe answered `awaiting=true`. A
  probe that fails, is cancelled or prints nothing leaves the output empty, so the full check runs as `Merge check`.
  On a draft, or while the probe answers `awaiting=true`, the job is skipped. GitHub lists a skipped job under its raw
  `name:` expression (ateam v0.3.6 notes), so no check named `Merge check` exists to count as passing.
- **D3. The ruleset is unchanged.** `main` keeps requiring `Required` and `Merge check`, with up-to-date on, and does
  not require the probe. A missing required check already blocks a merge. Requiring the probe would add a second check
  without closing any path.
- **D4. Templates are copied unedited** from a fresh `ateam init` at v0.3.7: `.agents/contracts/reviewer.md`,
  `.agents/skills/review-pr/SKILL.md` and `.agents/skills/ponytail/SKILL.md` (notes/1.2.md).
- **D5. The pin is the PR's.** On `pull_request`, GitHub reads the workflow file, and so its install lines, from the
  pull request's merge ref. Only the script and the review policy come from the base checkout. The header comment says
  so (#471).

## Proof

`TestCIWorkflowPinned` and its sensitivity cases in `internal/harness/toolcheck/mergegate_test.go` pin the probe job,
its base-commit checkout, its install line, the `needs`/`if` wiring and the `merge-check` job's name
(`mergeCheckName`). Each case was shown failing against a mutated workflow (notes/1.1.md). The ready run on #523 shows
the hosted behaviour, and its links go on the PR.
