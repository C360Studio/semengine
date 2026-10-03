# review-gate-check

Status: revision 3, accepted. It rests on `inventory.md`, which has `INVENTORY PASS` (PR #67, comment 5956907408),
and on `design.md`, which has `DESIGN REVIEW PASS` in round 2 of three (PR #67, comment 5957614491) and was accepted
by the owner, with its three questions answered as recommended (#66, comment 5959125319).

## Why

The protocol says the other agent reviews a code pull request: Codex reviews what Claude implemented, and the
reverse (`.agents/protocol.md`, "Cross-agent review"). Nothing fails when that is skipped; the rule's row in
`AGENTS.md` reads "review only until issue #66". The owner ruled on issue #64 that it is enforced for code and waived
for a documents-only pull request. In SemStreams the rules that drifted were the ones with no command behind them.

## What Changes

- **`scripts/merge-check.sh` gains a review check** on every pull-request run, in CI's `merge-check` job and in
  `task merge:check -- <n>`. A pull request whose changed files are all Markdown or under `openspec/`, with none
  under `.claude/agents/`, passes as documents only. Any other pull request is a code pull request, and its
  description must say two things.
  - Who implemented it: one line `implemented-by:` that names the agent with the word `claude` or `codex`.
  - Who reviewed it: one line `reviewed-by:` that names the other agent.
- **A draft is told and not failed.** The same findings are printed. The workflow starts a run when a draft is
  marked ready for review, and that run applies the check.
- **Every read fails closed.** A read that fails, is cut short or has the wrong shape never counts as documents
  only or as reviewed. The output says what to write.
- **No waiver, no new permission, no new job.** A push to `main` is not checked and the script says so.
- **Documents.** The protocol, `AGENTS.md` (the rule's row and the Merge bullet), the preflight skill,
  `docs/repository-map.md` and `docs/testing.md` name the check, what the two lines carry, and what stays checked
  in review only.

Not in this change: proof of who wrote anything (the owner and both agents share one GitHub login); any check of
which commit the review read, of its verdict, or of a review record; a waiver; any change to GitHub's own
`pull_request` rule on `main`, to the known-flake check or to the up-to-date rule; a review of PR #48. `design.md`
gives the commit and the verdict as options O4 and O5, with what each stops and costs, for the owner to choose.

## Capabilities

### Modified Capabilities

- `merge-gate`: two requirements added, "Cross-agent review check" and "A run when a pull request is marked ready".
  No existing requirement changes. The capability's Purpose names only flaky tests today and is edited in the
  archive commit to name the review check as well.

## Impact

- `scripts/merge-check.sh`; `internal/harness/contract/mergecheck_test.go` (a new test, `TestMergeCheckReview`, and
  two more answers from the fake `gh`); `internal/harness/contract/mergegate_test.go` (the workflow's trigger);
  `.github/workflows/ci.yml` (the trigger's activity types); `Taskfile.yml` (one description).
- Two more GitHub reads per pull-request run.
- One more CI run each time a pull request is marked ready.
- Test time: an estimated 14 to 41 s more for each `task verify`, which takes 160 s on CI today; measured in task
  2.5. The design proposes a budget of 30 s; nothing in the tree sets one.
- Every code pull request already open is covered once it is marked ready. PR #48 needs `claude` in its
  `implemented-by:` line. This pull request is itself a code pull request and is checked by its own script.
- Each Dependabot pull request, weekly for three ecosystems, is red on `Merge check` until an agent has reviewed
  it, a session has added the `reviewed-by:` line and the job has been run again.
- The owner's own instructions for Claude sessions ask for `implemented-by: sonnet|opus|fable`, which names no
  agent; a session that follows only them writes a line the check fails.
- A code pull request written by hand, by neither agent and not by a bot, cannot pass as designed (`design.md`, Q3).
- PR #48 changes three documents this change also edits. This change merges first if it is ready first; neither
  waits for the other.
- `design.md` gives the options, three questions for the owner, the costs, and the parts of the rule a script
  cannot check.
