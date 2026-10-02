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
- [x] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the `merge-gate` delta:
      `DESIGN REVIEW PASS` in round 2 of at most three, recorded on this pull request (round 1: comment 5957401025;
      round 2: comment 5957614491). The three corrections the pass asked for, with the reviewed and the committed
      checksums, are in comment 5957642045.
- [x] 1.3 The owner's ruling on #66 (comment 5959125319, 2026-10-02): the reviewed design is accepted; Q1 is (b), Q2
      is this change first if it is ready first and neither waits, Q3 is not now. Every answer is the design's
      recommendation, so the design, the delta and these tasks are unchanged.

## 2. The script and its test

- [ ] 2.1 (D) The fake `gh` answers two more reads (the pull request and its files) and still refuses any other
      call. The shared healthy state is a documents-only pull request. Written first: in one pull-request-run case
      of `TestMergeCheckKnownFlake` and one of `TestMergeCheckUpToDateRule`, an assertion that the pull request and
      its files were read; in a push-run case, an assertion that neither was. Every existing case passes with its
      expectation unchanged. Gate: `task test:unit`.
- [ ] 2.2 (D) `TestMergeCheckReview`, written first, the cases for which pull requests are covered: the scenarios
      "Documents only" to "No changed file" of the delta. Gate: `task test:unit`.
- [ ] 2.3 (D) `TestMergeCheckReview`, written first, the cases for the two lines: the scenarios "The other agent is
      named" to "Carriage returns". Gate: `task test:unit`.
- [ ] 2.4 (D) `TestMergeCheckReview`, written first, the remaining scenarios, "Draft" to "Push run and the review
      check". Gate: `task test:unit`.
- [ ] 2.5 (D) The time the `contract` package takes in the `test:unit`, `test:integration` and `test:repeat` steps
      of this pull request's CI run is recorded on this pull request beside the same three figures for `main` (21.0
      s, 14.1 s and 31.5 s in run 37028259077). The design proposes a budget of 30 s more for the three together;
      nothing in the tree sets one. Over it, the cases are run side by side and the figures taken again. Outcome,
      either way: the estimate under "Declared costs" in `design.md` is replaced by the measured figure; a figure
      still over the proposed budget is also stated in this pull request's description for the owner, and no case
      is removed to meet it. Gate: `task verify`.

## 3. The workflow

- [ ] 3.1 (D) `TestCIWorkflowPinnedSensitivity`, written first, gains one planted workflow for each case of the
      scenarios "Ready type missing" and "A default type missing", and its clean fixture gains the trigger.
      Gate: `task test:unit`.
- [ ] 3.2 (D) `.github/workflows/ci.yml` lists `opened`, `synchronize`, `reopened` and `ready_for_review` under
      `pull_request`; the job's comment and its step's name say what the script now checks. `TestCIWorkflowPinned`
      passes, and the job's three permissions are unchanged. Gate: `task test:unit`.

## 4. Against real GitHub

- [ ] 4.1 (D) The script is run locally, with `GITHUB_ACTIONS` unset, against #65, #63, #14, #48 and #67. Each
      output is recorded on this pull request beside the result worked out by hand, beforehand, from the pull
      request's page: #65 documents only; #63 a code pull request with no `reviewed-by:` line; #14 a code pull
      request by a bot with no `reviewed-by:` line; #48 a draft code pull request whose findings are printed and
      not failed; #67 as it then stands. A result that differs is a defect fixed before 7.1.
- [ ] 4.2 (D) The first CI run of this pull request with the new script: the `Merge check` log shows the pull
      request and its files read under the job's token, and the findings printed and not failed for a draft. The
      run's link and what it settles (assumption A2 of `design.md`, and A3 for one page) are recorded on this pull
      request.
- [ ] 4.3 Hold: the owner's word, given in the session that marks it ready and recorded on this pull request, to
      mark PR #67 ready once before any review record exists. Outcome (D): the run that starts fails `Merge check`
      and names what is missing; the pull request is returned to draft; both run links are recorded (assumption
      A1). If no run starts, that is recorded against A1 and decision D3 goes back to design review.

## 5. Documents

- [ ] 5.1 (W) `.agents/protocol.md` says what `design.md` D7 lists. "Land" (`:59`): the `implemented-by:` line
      names the implementing agent with the word `claude` or `codex`, beside the model or persona. "Cross-agent
      review": the `reviewed-by:` line names the reviewing agent with the same words and is written once that
      agent's newest review record approves; what the script checks and what stays review only; that marking ready
      starts the run that applies the check and an edit or a comment starts none. "Verification": what
      `merge-check` now fails on. Gate: `task docs:check`.
- [ ] 5.2 (W) `AGENTS.md`: the rule's row names the script, `TestMergeCheckReview` and `TestCIWorkflowPinned` under
      "Enforced by" and lists what stays review only, from `design.md`, "What the check does not see"; the command
      comment, the CI paragraph and the Merge bullet follow. Gate: `task docs:check`.
- [ ] 5.3 (W) One line each in `.agents/skills/semengine-preflight/SKILL.md`, `docs/repository-map.md`,
      `docs/testing.md`, the description of `merge:check` in `Taskfile.yml`, and the header of
      `scripts/merge-check.sh`. Gate: `task docs:check` and `task test:unit`.
- [ ] 5.4 (W) A note on PR #48, marked as coordination and not a review, says what its description needs before it
      is marked ready: `claude` in its `implemented-by:` line. It also repeats the protocol's sentence from 5.1 on
      when the `reviewed-by:` line is written. Its link is recorded on this pull request.

## 6. Shown able to fail

- [ ] 6.1 (D) The experiment of `docs/testing.md`, "Show that the test can fail", is run for each of the seven wrong
      changes listed in `design.md`, D6. For each, the change and the baseline, wrong-change and restored runs are
      recorded on this pull request. A wrong change the tests let through is reported as a survivor and closed with
      a new case, or listed under what is not covered.

## 7. Review

- [ ] 7.1 Hold: Codex's implementation review of PR #67, which the owner starts. Outcome: a review record by Codex's
      `semengine-reviewer` on this pull request that names the commit it read; every finding fixed; and, where
      there were findings, a record by Codex that names the commit holding the last fix.
- [ ] 7.2 Hold: 7.1 on PR #67. Outcome: this pull request's description has one line `implemented-by:` that names
      `claude`, and one line `reviewed-by:` that names `codex`, written once Codex's newest review record approves,
      as the protocol's sentence from 5.1 says.

## 8. In the archive commit

- [ ] 8.1 (W) The Purpose of `openspec/specs/merge-gate/spec.md` names the review check beside the defence against
      flaky tests, in the same commit as `openspec archive review-gate-check` and the spec sync.
      Gate: `task spec:check` on that commit.
