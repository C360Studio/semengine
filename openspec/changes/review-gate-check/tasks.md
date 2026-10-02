# Tasks: review-gate-check

Each task names the outcome and the gate that proves it. An unchecked task whose first line says `Hold:` is one
`task spec:queue` reports as blocked until what it names exists. "This pull request" is PR #67; evidence is recorded
there as a comment. Every outcome below is reached on the branch, in or before the archive commit. (D) is the
developer, (W) the technical writer.

"Written first" means the test is run and seen to fail for the stated reason before the code that makes it pass, and
that output is recorded on this pull request. The script's tests run offline against the fake `gh` (the stand-in for
the GitHub command line that the test puts first on `PATH`); their expected exits and words are written out in the
test, from the scenarios of the `merge-gate` delta.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 67` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS` in round 2 of at most three, recorded on this
      pull request (round 1: comment 5956546196; round 2: comment 5956907408). The file reviewed and the file committed
      are the same, sha256 `725b11e7…8fbe8f`.
- [ ] 1.2 Hold: 1.1, then the independent pre-owner review of `design.md`, `proposal.md`, this file and the
      `merge-gate` delta. Outcome: `DESIGN REVIEW PASS` within three rounds, recorded on this pull request with the
      reviewed checksums.
- [ ] 1.3 Hold: the owner's ruling on #66. Outcome: his acceptance of the reviewed design and his answers to Q1 to
      Q7 of `design.md`, recorded on #66. An answer that differs from the recommendation is worked into the design,
      the delta and these tasks, and re-checked by the reviewer, before any code.

## 2. The script and its test

- [ ] 2.1 (D) The fake `gh` answers four more reads (the pull request, its files, its comments, a comparison of two
      commits) and still refuses any other call. The shared healthy state is a documents-only pull request. Written
      first: one assertion in `TestMergeCheckKnownFlake` and one in `TestMergeCheckUpToDateRule` that the pull
      request and its files were read. Every existing case passes with its expectation unchanged.
      Gate: `task test:unit`.
- [ ] 2.2 (D) `TestMergeCheckReview`, written first, the cases for which pull requests are covered: the scenarios
      "Documents only" to "No changed file" of the delta, and the 101-file list served both as one list and as two
      lists back to back. Gate: `task test:unit`.
- [ ] 2.3 (D) `TestMergeCheckReview`, written first, the cases for the two lines: the scenarios "No reviewed-by
      line" to "Carriage returns". Gate: `task test:unit`.
- [ ] 2.4 (D) `TestMergeCheckReview`, written first, the cases for the record and for carrying it over: the
      scenarios "Record names the head" to "More files than GitHub lists, record names an earlier commit". The fake
      answers a comparison only for the pair of commits the case expects. Gate: `task test:unit`.
- [ ] 2.5 (D) `TestMergeCheckReview`, written first, the remaining scenarios of the delta, from "Draft" to "Push
      run": the two draft scenarios; each of the four reads, both when it errors and when its answer has the wrong
      shape; a recorded commit GitHub does not have; a sorting or comparing step that errors; an open flake together
      with a missing review; another base; a push run. Gate: `task test:unit`.
- [ ] 2.6 (D) The time of `go test -count=1 -run '^TestMergeCheck' ./internal/harness/contract/` before and after is
      recorded on this pull request, and the figure under "Declared costs" in `design.md` is replaced by the
      measured one. Gate: `task verify`, whose step timings are recorded with it.

## 3. The workflow

- [ ] 3.1 (D) `TestCIWorkflowPinnedSensitivity`, written first, gains one planted workflow for each scenario of "A
      run when a pull request is marked ready", and its clean fixture gains the trigger. How the YAML library decodes
      the key `on` is measured first and recorded on this pull request. Gate: `task test:unit`.
- [ ] 3.2 (D) `.github/workflows/ci.yml` lists `opened`, `synchronize`, `reopened` and `ready_for_review` under
      `pull_request`; the job's comment and its step's name say what the script now checks. `TestCIWorkflowPinned`
      passes, and the job's three permissions are unchanged. Gate: `task test:unit`.

## 4. Against real GitHub

- [ ] 4.1 (D) The script is run locally, with `GITHUB_ACTIONS` unset, against #65, #63, #14, #48 and #67. Each
      output is recorded on this pull request beside the result worked out by hand from the pull request's page:
      #65 documents only; #63 a code pull request with no `reviewed-by:` line; #14 a code pull request by a bot; #48
      a draft code pull request whose findings are printed and not failed; #67 as it then stands. A result that
      differs is a defect fixed before 7.1.
- [ ] 4.2 (D) The first CI run of this pull request with the new script: the `Merge check` log shows the pull
      request, its files and its comments read under the job's token, and the findings printed and not failed for a
      draft. The run's link and the assumptions it settles (A2, A3) are recorded on this pull request.
- [ ] 4.3 (D) Before any review record exists, this pull request is marked ready once. The run that starts fails
      `Merge check` and names what is missing. The pull request is returned to draft, and both run links are
      recorded (A1). If no run starts, that is recorded against A1 and decision D5 goes back to design review.
- [ ] 4.4 (D) Hold: 7.1. Outcome: in the first CI run after the record of 7.1 exists and a later commit has been
      pushed, the `Merge check` log shows the comparison read under the job's token and what the script made of it.
      The run's link is recorded (A2).

## 5. Documents

- [ ] 5.1 (W) `.agents/protocol.md`, "Cross-agent review" and "Verification": the words `claude` and `codex` in the
      two lines; the heading and two lines of a review record; what the script checks and what stays review only;
      that a comment or an edited description starts no run. Gate: `task docs:check`.
- [ ] 5.2 (W) `.agents/contracts/semengine-reviewer.md` says a review record on a code pull request carries the
      heading and the two lines; `.agents/contracts/semengine-developer.md`, step 6, names the two description
      lines and the full commit id in the request. Gate: `task docs:check`.
- [ ] 5.3 (W) `AGENTS.md`: the rule's row names the script, `TestMergeCheckReview` and `TestCIWorkflowPinned` under
      "Enforced by" and lists what stays review only, from `design.md`, "What the check does not see"; the command
      comment and the Merge gate follow. Gate: `task docs:check`.
- [ ] 5.4 (W) One line each in `.agents/skills/semengine-preflight/SKILL.md`, `docs/repository-map.md`,
      `docs/testing.md`, the description of `merge:check` in `Taskfile.yml`, and the header of
      `scripts/merge-check.sh`. Gate: `task docs:check` and `task test:unit`.
- [ ] 5.5 (W) A note on PR #48, marked as coordination and not a review, gives the two words, the two lines of a
      record, and what #48's description and its open `Review request` need before it is marked ready. Its link is
      recorded on this pull request.

## 6. Shown able to fail

- [ ] 6.1 (D) The experiment of `docs/testing.md`, "Show that the test can fail", is run for each wrong change listed
      in `design.md`, D9. For each, the change and the baseline, wrong-change and restored runs are recorded on this
      pull request. A wrong change the tests let through is reported as a survivor and closed with a new case, or
      listed under what is not covered.

## 7. Review

- [ ] 7.1 Hold: Codex's implementation review, which the owner starts. Outcome: a comment on this pull request
      headed `Review record`, by Codex's `semengine-reviewer`, with the lines `reviewed-by: codex` and `commit:`
      naming the commit read. The `Review request` gives that commit by its 40 characters and the two lines.
- [ ] 7.2 Hold: 7.1. Outcome: every finding of 7.1 is fixed, and a review record by Codex names the commit that
      holds the last fix. With no finding this is ticked with 7.1.
- [ ] 7.3 Hold: 7.1. Outcome: this pull request's description has one line `implemented-by:` that names `claude`
      and one line `reviewed-by:` that names `codex`.

## 8. In the archive commit

- [ ] 8.1 (W) The Purpose of `openspec/specs/merge-gate/spec.md` names the review check beside the defence against
      flaky tests, in the same commit as `openspec archive review-gate-check` and the spec sync.
      Gate: `task spec:check` on that commit.
