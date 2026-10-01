# Tasks: flake-defense

Each task names the outcome that proves it. An unchecked task that says "hold" is one `task spec:queue` reports as
blocked until the named review or ruling exists. "This pull request" is PR #44. Evidence is recorded there as a
comment unless a task says otherwise. Every outcome below is reached before this pull request merges.

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
- [ ] 2.3 Hold: owner ruling on #42. The owner's ruling on the options and on questions Q1 to Q5 is posted on #42
      and recorded in `design.md`.

## 3. Spec changes

- [ ] 3.1 Hold: task 2.3. The spec changes under `specs/` match the ruled design, `.openspec.yaml` does not set
      `skip_specs`, and `task spec:check` passes.

## 4. Harness tests made cheap and complete

- [ ] 4.1 The failpoint table's completeness check, written first, rejects a planted table with a hole. Then one
      table in `refowner_test.go` declares every failpoint with its expected check, and both
      `TestEachFailpointTripsExactlyItsCheck` and `TestAbortStopThenFinishJoinsWorker` take their cases from it.
- [ ] 4.2 `finalize` asserts "nothing unresolved" for every failpoint, with no exemption. The assertion is written
      first and fails for `stopReturnsNilWithWorkerRunning`; then `finalize` joins that double's worker and the
      assertion passes.
- [ ] 4.3 Each failpoint's subtest of the matrix runs inside `synctest.Test`. The output of
      `go test -race -count=20 ./internal/harness/lifecycletest/` is recorded and shows under 5 s (41.5 s before).
- [ ] 4.4 A contract test runs a copy of `scripts/cover-check.sh` in a throwaway root with a fake `go` that prints a
      `--- FAIL` line and exits 1. It is written first and fails on the current script, which prints nothing;
      then the script prints the test output and the test passes.

## 5. Text checks with no accepted exceptions

- [ ] 5.1 The sleep check and its sensitivity test are in `internal/harness/contract`. The sensitivity test is
      written first and fails, then passes: a planted `time.Sleep` in a test file, and one in a harness file that
      is not a test, are each named by file and line, and a tree with no test file is rejected.
- [ ] 5.2 The skip check and the build-tag check, with their sensitivity tests, are in `internal/harness/contract`.
      The skip check is written first and fails on the tree as it is, naming `runner_test.go:264`. Then `deadPID`
      retries with a fresh child a bounded number of times and fails with the reason, no skip call remains, and
      the check passes.
- [ ] 5.3 A contract test plants a fixed port on a line carrying `// gh#220:allow-fixed-port`. It is written first
      and fails on the current `scripts/lint-test-ports.sh`. Then the script honours no marker and its message
      says to bind port 0 and keep the listener; the fixture test's marker case expects a match; `task lint`
      passes; the two ledger rows read `adapt` and say what differs from the pin; `Taskfile.yml`'s comment and
      `docs/provenance.md` no longer say the scripts are unchanged; `task ledger:check` passes.

## 6. Varied and repeated unit runs

- [ ] 6.1 A contract test that reads `Taskfile.yml` and `scripts/verify.sh` is written first and fails on the tree
      as it is. Its sensitivity test names a lowered count, a missing `-cpu 1`, `-race` added to the repeat step,
      a missing step, and a step placed after `test:repeat`.
- [ ] 6.2 `test:unit` and `test:repeat` carry the command lines of the `merge-gate` spec, `scripts/verify.sh` runs
      `test:repeat` last, and the test of 6.1 passes. Any existing test the new step breaks is fixed in this
      change; `task verify` passes on the branch and its step timings are recorded.
- [ ] 6.3 On a scratch copy of the branch with the #40 defect restored (`o.finishLocked()` back in the
      `abortStopDropsCause` branch of `Stop`), `task test:repeat` exits non-zero naming
      `TestEachFailpointTripsExactlyItsCheck/abortStopDropsCause`. The output is recorded.
- [ ] 6.4 The step timings printed by a CI run of this pull request that includes `test:repeat` are recorded, with
      the run's URL, beside the 61 s of run 36883858915.

## 7. Merge check

- [ ] 7.1 The label `class:flake` exists; `gh label list` output is recorded.
- [ ] 7.2 A contract test runs a copy of `scripts/merge-check.sh` in a throwaway root with a fake `gh`. It is
      written first, so it fails until the script exists. Its cases are the scenarios of the `merge-gate`
      requirements "Known-flake check" and "Up-to-date rule", one case each. Then the script and
      `task merge:check` exist and the test passes.
- [ ] 7.3 Hold: task 2.3, which rules on Q3. The ruleset's strict setting is on. The ruleset as read before and
      after the edit is posted on #42, and the two differ in that one value. The same comment lists every open
      pull request with its number of commits behind `main`, and what `gh pr view --json mergeStateStatus`
      reports for PR #14 and PR #39: `BEHIND`, or the differing result recorded against assumption A1.
- [ ] 7.4 The workflow has the job `merge-check` with its three read permissions, and `required` needs `verify`
      and `merge-check`. A contract test of `ci.yml`, written first, rejects four planted workflows: `required`
      needing only `verify`; a job with a write permission; `merge-check` with a fourth permission; `verify` with
      a limit other than 15 minutes.
- [ ] 7.5 The log of this pull request's first CI run with the `merge-check` job shows the label, issue, pull
      request, branch-rules and ruleset reads succeeding and prints the five fields of `design.md` D9 as the
      job's token sees them. The results for assumptions A2 and A3 are recorded on #42.
- [ ] 7.6 A drill issue labelled `class:flake` is opened; its body says it is a drill and quotes the ruling of
      task 2.3 as the word for opening and closing it. Four results are recorded with their run links, each
      against the assumption it measures when it differs from what `design.md` expects. (a) A CI run of this
      pull request shows `merge-check` and `Required` failing and naming the drill issue. (b) `Closes #<drill>`
      is added to this pull request's description, and the run list before and after shows whether the edit
      started a run (A11). (c) The failed run is re-run once: the record shows its `run_attempt`, whether
      `merge-check` passed, the exemption line, and whether the warning shows on the run (A4, A5, A12). (d) The
      line is removed from the description, the drill issue is closed, and the run of the next head passes with
      no exemption printed.

## 8. Documents and landing

- [ ] 8.1 The Land and Close steps in `.agents/protocol.md`, the merge rule and command list in `AGENTS.md`, and
      the preflight skill's gate table and its rule for an unexplained failure say what `design.md` D10 lists, as
      Q1 and Q4 were ruled; `docs/repository-map.md` lists the new job and script; `task docs:check` passes.
- [ ] 8.2 Each assumption A1 to A14 in `design.md` has its result, or is restated as open, in one comment on #42.
- [ ] 8.3 The change is archived and its specs are synced as the last content commit; `task spec:check` passes on
      that commit.
