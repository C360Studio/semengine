# review-gate-check

Status: draft. It rests on `inventory.md`, which has not had its independent review, and on `design.md`, which has
not been reviewed or accepted.

## Why

The protocol says the other agent reviews a code pull request: Codex reviews what Claude implemented, and the
reverse (`.agents/protocol.md`, "Cross-agent review"). Nothing fails when that is skipped; the rule's row in
`AGENTS.md` reads "review only until issue #66". The owner ruled on issue #64 that it is enforced for code and waived
for a documents-only pull request. In SemStreams the rules that drifted were the ones with no command behind them.

## What Changes

- **`scripts/merge-check.sh` gains a review check** on every pull-request run, in CI's `merge-check` job and in
  `task merge:check -- <n>`. A pull request whose changed files are all Markdown or under `openspec/`, with none
  under `.claude/agents/`, passes as documents only. Any other pull request is a code pull request and needs three
  things.
  - Its description has one line `implemented-by:` and one line `reviewed-by:`. Each names the agent with the word
    `claude` or `codex`, and the reviewer is not the implementer.
  - A comment headed `Review record` carries the lines `reviewed-by:` (the same agent) and `commit:` (a full commit
    id).
  - That commit is the head, or no file the pull request changes differs between it and the head.
- **A draft is told and not failed.** The same findings are printed. The workflow starts a run when a draft is
  marked ready for review, and that run applies the check.
- **Every read fails closed.** A read that fails, is cut short or has the wrong shape never counts as documents
  only or as reviewed. The output says what to add.
- **No waiver, no new permission, no new job.** A push to `main` is not checked and the script says so.
- **Documents.** The protocol, the reviewer and developer contracts, `AGENTS.md` (the rule's row and the Merge
  gate), the preflight skill, `docs/repository-map.md` and `docs/testing.md` name the check and the two fixed lines
  of a review record.

Not in this change: proof of who wrote a comment or a commit (the owner and both agents share one GitHub login);
the verdict or the kind of a review; a waiver; any change to the known-flake check or the up-to-date rule; a review
of PR #48.

## Capabilities

### Modified Capabilities

- `merge-gate`: two requirements added, "Cross-agent review check" and "A run when a pull request is marked ready".
  No existing requirement changes. The capability's Purpose names only flaky tests today and is edited in the
  archive commit to name the review check as well.

## Impact

- `scripts/merge-check.sh`; `internal/harness/contract/mergecheck_test.go` (a new test, `TestMergeCheckReview`, and
  four more answers from the fake `gh`); `internal/harness/contract/mergegate_test.go` (the workflow's trigger);
  `.github/workflows/ci.yml` (the trigger's activity types); `Taskfile.yml` (one description).
- Three more GitHub reads per pull-request run, and one more when a review record names an earlier commit.
- One more CI run each time a pull request is marked ready.
- Every code pull request already open is covered once it is marked ready. PR #48 needs its `implemented-by:` line
  reworded and a record by Codex. This pull request is itself a code pull request and is checked by its own script.
- Each Dependabot pull request waits for a recorded review.
- `design.md` lists seven questions for the owner, the costs, and the parts of the rule a script cannot check.
- PR #48 changes four documents this change also edits. This change merges first.
