# runner-image-pin

Status: revision 2, implemented. It rests on `inventory.md` (`INVENTORY PASS`) and `design.md` (`DESIGN REVIEW
PASS`), both reviewed on PR #112, comment 6036743505. Codex's implementation review approved `7c6dae2` (comment
6037693225). The owner's rulings it implements are on #61: comment 6035540608 (pin `ubuntu-24.04`) and comment
6036642422 (the check holds the exact label; GitHub's builds within 24.04 are accepted).

## Why

Every job in `.github/workflows/ci.yml` runs on `ubuntu-latest`. GitHub starts moving that label to Ubuntu 26 on
2026-10-19 (actions/runner-images#14748). After that, two runs of the same commit can land on different releases,
with a different Docker, git and system libraries under `task verify`, and a red that no diff explains. By the
protocol, a check that passes and fails on the same tree is a known flake, and a known flake stops every merge
(`.agents/protocol.md`, "Known flakes"). Go, Node, Task, the tools and every action are pinned; the runner image is
not. The owner ruled to pin `ubuntu-24.04` and make the move to Ubuntu 26 a deliberate later change, with a
`task verify` run on the new image first, and to land this before 2026-10-19.

## What Changes

- **The workflow.** The `runs-on:` of `verify`, `merge-check` and `required` changes from `ubuntu-latest` to
  `ubuntu-24.04`. Today that changes nothing: main's runs already report `Image: ubuntu-24.04`. A comment above
  `jobs:` says the image is pinned by the `merge-gate` spec and how to move it.
- **The check.** `TestCIWorkflowPinned` does not read `runs-on` today, so on its own the edit would be review only.
  It gains one check: every job's `runs-on` is the one label `ubuntu-24.04`, compared with the test's constant
  `runnerLabel`. `TestCIWorkflowPinnedSensitivity` gains seven planted workflows: `ubuntu-latest` on a fourth job,
  no `runs-on`, `ubuntu-26.04`, `ubuntu-24.04-arm`, a list of labels, a runner group, and an expression.
- **Moving later.** Because the spec names the image, moving to Ubuntu 26 is a change to this requirement, the
  test's constant and the three lines; that pull request's own CI run is the `task verify` run on the new image.

Not in this change: the move to Ubuntu 26; any pin of GitHub's build within 24.04, which the owner accepted (the two
builds seen in one run of main were dated a week apart; that is one observation, not a measured schedule); a check of
workflow files other than `ci.yml`; a row in `AGENTS.md` (`design.md`, D5).

## Capabilities

### Modified Capabilities

- `merge-gate`: one requirement added, "Pinned runner image". No existing requirement changes. The Purpose is edited
  in the archive commit to name the runner image beside the defences against flaky tests.

## Impact

- `.github/workflows/ci.yml` (three lines and one comment); `internal/harness/contract/mergegate_test.go` (one field,
  one constant, one check, seven planted workflows, the test's comment); `docs/repository-map.md` (the archive row).
- A code pull request: Codex's review of record is needed before merge.
- No new job, permission, tool, exported name or run time worth measuring.
- Moving to another image now takes an OpenSpec change, a small one, instead of a three-line edit.
- PR #93 shares the Go package `internal/harness/contract` and no file; neither waits for the other.
