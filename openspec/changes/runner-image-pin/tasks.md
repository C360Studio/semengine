# Tasks: runner-image-pin

Each task names the outcome and the gate that proves it. An unchecked task whose first line says `Hold:` is one
`task spec:queue` reports as blocked until what it names exists. "This pull request" is PR #112; evidence is recorded
there as a comment. Every outcome below is reached on the branch, in or before the archive commit. (D) is the
developer, (W) the technical writer.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 112` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS`, recorded on this pull request with the
      reviewed file's checksum (comment 6036743505).
- [x] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the `merge-gate` delta:
      `DESIGN REVIEW PASS`, recorded on this pull request (comment 6036743505).
- [x] 1.3 The owner's rulings on #61 (comment 6036642422, 2026-10-07): the check holds the exact label `ubuntu-24.04`
      on every job, and GitHub's builds within 24.04 are an accepted cost. They are recorded in `design.md`, "Owner's
      rulings", and leave no open owner question.

## 2. The check and the workflow

- [ ] 2.1 (D) Written first: `ciJob` gains `RunsOn`, and `ciWorkflowViolations` requires every job's `runs-on` to
      be the one label `runnerLabel` (`ubuntu-24.04`), a constant in the test (`design.md`, D2). Run against the
      unchanged `.github/workflows/ci.yml`, `TestCIWorkflowPinned` fails naming `verify`, `merge-check` and
      `required` with `ubuntu-latest`; that output is recorded on this pull request. Gate: the recorded failing run.
- [x] 2.2 (D) `TestCIWorkflowPinnedSensitivity`: the clean fixture gains `runs-on: ubuntu-24.04` on each job, and
      the seven planted workflows of `design.md`, D3, are added, the `ubuntu-latest` one on a fourth job.
      Gate: `task test:unit`.
- [x] 2.3 (D) `.github/workflows/ci.yml`: the three `runs-on:` lines read `ubuntu-24.04`, with the comment of
      `design.md`, D4. Committed with 2.1 and 2.2 so that no commit is red. `TestCIWorkflowPinned` passes.
      Gate: `task verify`.
- [ ] 2.4 (D) Shown able to fail, by hand (`docs/testing.md`, "Run it by hand"): the four wrong changes of
      `design.md`, "Tests", each caught by its plant (`-latest` only, absent skipped, three named jobs only, prefix
      match). Baseline, wrong-change and restored runs, and what is not covered, are recorded on this pull request.
- [ ] 2.5 The first CI run of this pull request with the pin: each job's "Set up job" log reports
      `Image: ubuntu-24.04`. The run's link and the image version of each job are recorded on this pull request.
      Gate: CI `Required` on that head.

## 3. Review

- [ ] 3.1 Codex's implementation review of this pull request, recorded on it, naming the commit it read. Each
      finding is fixed or answered before 4.1.
- [ ] 3.2 The description has one line `implemented-by:` that names `claude` and one line `reviewed-by:` that names
      `codex`, written once Codex's newest review record approves.

## 4. In the archive commit

- [ ] 4.1 (W) In the same commit as `openspec archive runner-image-pin` and the spec sync: the Purpose of
      `openspec/specs/merge-gate/spec.md` names the pinned runner image beside the defences against flaky tests, and
      `docs/repository-map.md` gains a row for the archived change, as the `mutation-check` row does. Gate:
      `task spec:check` and `task docs:check` on that commit.
