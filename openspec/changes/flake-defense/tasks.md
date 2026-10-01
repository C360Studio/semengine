# Tasks: flake-defense

Each task names the outcome that proves it. An unchecked task that says "hold" is one `task spec:queue` reports as
blocked until the named review or ruling exists. "This pull request" is PR #44. Evidence is recorded there as a
comment unless a task says otherwise. Every outcome below is reached before this pull request merges.

The tasks are done in the order written, from top to bottom. Four notes on that order:

- **Written first.** Where a task says a test is written first, the test is run locally and seen to fail, and that
  output is recorded. The test is pushed together with the change that makes it pass, so no push is red for that
  reason. Task 6.1 is the one case that spans two tasks: it is pushed with task 6.2.
- **GitHub state before the push that needs it.** Task 7.4's push starts the first run of the `merge-check` job. The
  job needs the label (task 7.1) and the strict setting (task 7.3), so both come first, and task 7.5 reads that run.
- **One run is red on purpose:** the first run that has the `merge-check` job. It meets the open flake #49 before
  this pull request's description closes it (task 7.6).
- **Before the first ported code.** By the owner's direction (2026-10-01, #42 comment 5938232045) this change lands
  before PR #48, the first slice that ports code. Tasks 5.1, 5.2 and 6.2 are written against a
  tree with no ported packages. If PR #48 lands first, those tasks do not repair ported files, because each slice
  repairs the files it ports (ruling Q5): work on them stops and the conflict is put to the owner on #42.

## 1. Inventory

- [x] 1.1 `inventory.md` records the architect's inventory at base `4d96860`, with every entry pinned and no options
      or design.
- [x] 1.2 Hold: independent inventory review. The reviewer's verdict on `inventory.md` is `INVENTORY PASS`, recorded
      on PR #44 with the reviewed file's checksum.

## 2. Options and ruling

- [x] 2.1 `design.md` frames the options with their costs, including doing nothing and extending an existing guard,
      and names one recommendation.
- [x] 2.2 Hold: independent design review. The reviewer's verdict on `design.md` and the spec changes is a pass,
      recorded on PR #44 with the reviewed files' checksums. The first review asked for changes (comment
      5936056866); the second revision of `design.md` lists its answers under "Corrections after design review".
- [x] 2.3 Hold: owner ruling on #42. The owner ruled on questions Q1 to Q5 (comment 5937751262: Q1a (b), Q1b (a), Q2
      (a), Q3 (a), Q4 (a), Q5 (a)) and struck the slice 04A measurement (comment 5937807011). Both rulings are
      recorded in `design.md`, "The owner's ruling".
- [x] 2.4 Hold: check of the ruling pass. The reviewer's verdict on the third revision of `design.md`, the spec
      changes and this file, confined to the diff from `bba268a`, is a pass, recorded on PR #44 with the checked
      files' checksums.
- [x] 2.5 Hold: check of the implementation pass. The reviewer's verdict on `design.md`, the `lifecycle-suite` and
      `merge-gate` spec changes and this file, confined to the diff from `8e1e750`, is a pass, recorded on PR #44 with
      the checked files' checksums.

## 3. Spec changes

- [x] 3.1 The spec changes under `specs/` say what was ruled (task 2.3) and have passed the check of task 2.4;
      `.openspec.yaml` does not set `skip_specs`; `task spec:check` passes.

## 4. Harness tests made cheap and complete

- [x] 4.1 The failpoint table's completeness check, written first, rejects a planted table with a hole. Then one
      table in `refowner_test.go` declares every failpoint with its expected check, and both
      `TestEachFailpointTripsExactlyItsCheck` and `TestAbortStopThenFinishJoinsWorker` take their cases from it.
- [x] 4.2 `finalize` asserts "nothing unresolved" for every failpoint, with no exemption. The assertion is written
      first and fails for `stopReturnsNilWithWorkerRunning`; then `finalize` joins that double's worker and the
      assertion passes.
- [x] 4.3 Each failpoint's subtest of the matrix runs inside `synctest.Test`. The output of
      `go test -race -count=20 ./internal/harness/lifecycletest/` is recorded and shows under 5 s (41.5 s before).
- [x] 4.4 A contract test runs a copy of `scripts/cover-check.sh` in a throwaway root with a fake `go` that prints a
      `--- FAIL` line and exits 1. It is written first and fails on the current script, which prints nothing;
      then the script prints the test output and the test passes.
- [x] 4.5 Flake #49: `TestAwaitReportsLastObservation` in `internal/harness/probe` no longer depends on how many
      observations fit in a stretch of wall-clock time. A reproduction that fails on demand, not by repetition, is
      written first and fails on the tree as it is; then the fix, with no retry, sleep, skip or loosened assertion.
      The search for the same shape in the package's tests is recorded with what it found.
- [x] 4.6 A test that always runs holds `finalize`'s order: with a signalled worker held before its exit, `finalize`
      reports the worker and returns only after the worker has exited. It is written first and fails against a copy
      of `finalize` with the assertion moved after the last join, and against one with the last join removed.

## 5. Text checks with no accepted exceptions

- [x] 5.1 The sleep check and its sensitivity test are in `internal/harness/contract`. The sensitivity test is
      written first and fails, then passes: a planted `time.Sleep` in a test file, and one in a harness file that
      is not a test, are each named by file and line, and a tree with no test file is rejected.
- [x] 5.2 The skip check and the build-tag check, with their sensitivity tests, are in `internal/harness/contract`.
      The skip check is written first and fails on the tree as it is, naming `runner_test.go:264`. Then `deadPID`
      retries with a fresh child a bounded number of times and fails with the reason, no skip call remains, and
      the check passes.
- [x] 5.3 A contract test plants a fixed port on a line carrying `// gh#220:allow-fixed-port`. It is written first
      and fails on the current `scripts/lint-test-ports.sh`. Then the script honours no marker and its message
      says to bind port 0 and keep the listener; the fixture test's marker case expects a match; `task lint`
      passes; the two ledger rows read `adapt` and say what differs from the pin; `Taskfile.yml`'s comment and
      `docs/provenance.md` no longer say the scripts are unchanged; `task ledger:check` passes.

## 6. Varied and repeated unit runs

- [x] 6.1 A contract test that reads `Taskfile.yml` and `scripts/verify.sh` is written first and fails on the tree as
      it is. Its sensitivity test names a lowered count, a missing `-cpu 1`, `-race` added to the repeat step, a
      missing step, and a step placed after `test:repeat`. Its output on the tree as it is, run locally, is recorded.
      It is pushed with task 6.2 and not before.
- [x] 6.2 `test:unit` and `test:repeat` carry the command lines of the `merge-gate` spec, `scripts/verify.sh` runs
      `test:repeat` last, and the test of 6.1 passes. Any existing test the new step breaks is fixed in this
      change; `task verify` passes on the branch and its step timings are recorded.
- [x] 6.3 On a scratch copy of the branch with the #40 defect restored (`o.finishLocked()` back in the
      `abortStopDropsCause` branch of `Stop`), `task test:repeat` exits non-zero naming
      `TestEachFailpointTripsExactlyItsCheck/abortStopDropsCause`. The output is recorded.
- [x] 6.4 The step timings printed by a CI run of this pull request that includes `test:repeat` are recorded, with
      the run's URL, beside the 61 s of run 36883858915.

## 7. Merge check

- [x] 7.1 The label `class:flake` exists. Its description says it is for a test or check that passes and fails on the
      same tree, and not for a network fetch that did not answer. `gh label list` output is recorded.
- [x] 7.2 A contract test runs a copy of `scripts/merge-check.sh` in a throwaway root with a fake `gh`. It is
      written first, so it fails until the script exists. Its cases are the scenarios of the `merge-gate`
      requirements "Known-flake check" and "Up-to-date rule", one case each. Then the script and
      `task merge:check` exist and the test passes.
- [x] 7.3 The session turns the ruleset's strict setting on with `gh api`, as ruled on Q3, before task 7.4 is pushed.
      The ruleset as read before and after the edit is posted on #42, and the two differ in that one value; any other
      difference is put back and reported. The same comment lists every open pull request with its number of commits
      behind `main`, and what `gh pr view --json mergeStateStatus` reports for PR #14 and PR #39: `BEHIND`, or the
      differing result recorded against assumption A1.
- [x] 7.4 The workflow has the job `merge-check` with its three read permissions, and `required` needs `verify` and
      `merge-check`. A contract test of `ci.yml`, written first, rejects four planted workflows: `required` needing
      only `verify`; a job with a write permission; `merge-check` with a fourth permission; `verify` with a limit
      other than 15 minutes. This task is pushed only after tasks 7.1 and 7.3 are done: without the label or the
      strict setting, the job's first run stops on those and shows nothing else. Open again after implementation
      (`design.md`, "After implementation"; the first half is done at `8e1e750`): the same test also requires
      `if: always()` on `required`, requires that its step takes its results from `needs.*.result`, and runs that
      step's script as the workflow writes it, which exits 0 only when every result is `success`. It rejects three
      more planted workflows: `required` without `if: always()`; a step that exits 0 for a `skipped` result; a step
      that reads the result of `verify` alone. The three are written first and fail against the test as it stands
      at `8e1e750`, which names none of them.
- [x] 7.5 After task 7.3: the first CI run of this pull request that has the `merge-check` job is the run that task
      7.4's push starts. Its log names the run as a pull-request run, shows the label, issue, pull request,
      branch-rules and ruleset reads succeeding, and prints the five fields of `design.md` D9 as the job's token sees
      them. The up-to-date half passes. A different result is recorded against the assumption it contradicts and is
      put right before task 7.6. The results for assumptions A2, A3 and A17 are recorded on #42.
- [x] 7.6 The red path and the exemption are exercised against the real flake #49, which takes the place of the drill
      issue first planned here; no issue is filed for the purpose. Three results are recorded with their run links,
      each against the assumption it measures when it differs from what `design.md` expects. (a) The run of task 7.5
      starts while #49 is open and this pull request's description does not yet close it: `merge-check` and `Required`
      fail and name #49. That run is red on purpose. (b) `Closes #49` is added to this pull request's description, before
      the implementation review, and the run list before and after shows whether the edit started a run (A11). (c)
      Only if `Verify` passed in run (a): the failed `merge-check` and `required` jobs of that run are re-run once,
      and no other job. The record shows the `run_attempt`, whether `merge-check` passed, the exemption line, and
      whether the warning shows on the run (A4, A5, A12). If `Verify` failed in run (a), nothing is re-run: a re-run
      would re-roll `Verify` past an open flake, so the cause is fixed and a new head is pushed instead. A run that
      passes with no exemption printed is not seen in this change, because #49 stays open until this pull request
      merges; task 8.2 restates that as open.

## 8. Documents and landing

- [x] 8.1 The Land and Close steps in `.agents/protocol.md`, the merge rule and command list in `AGENTS.md`, and the
      preflight skill's gate table and its rule for an unexplained failure say what `design.md` D10 lists, as ruled:
      the waiver sentence is gone from the Land step, a network fetch that did not answer is not a `class:flake`, and
      the exemption needs every open flake. `git grep -n -i waiver -- .agents AGENTS.md` then finds no sentence that
      offers a way past a flake; the two lines D10 names remain. The three sentences that say CI has two jobs
      (`.agents/protocol.md`, `AGENTS.md`, the preflight skill) name `verify`, `merge-check` and `required`, and say
      `required` fails if either of the other two failed, is missing, or was skipped or cancelled;
      `git grep -n 'two jobs' -- .agents AGENTS.md` then finds nothing. `docs/repository-map.md` lists the new job
      and script; `task docs:check` passes.
- [x] 8.2 Each assumption A1 to A18 in `design.md` has its result, or is restated as open, in one comment on #42.
- [ ] 8.3 The change is archived and its specs are synced as the last content commit; `task spec:check` passes on
      that commit.
