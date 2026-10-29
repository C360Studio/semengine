---
name: review-pr
description: >-
  Review one pull request as an independent, read-only reviewer and post the record comment
  merge-check reads. The manual trigger for a cross-agent review.
---

# Review a Pull Request

Read [the shared protocol](../../protocol.md) and [the reviewer contract](../../contracts/reviewer.md) first. Review in a separate run that wrote none of the change. The deliverable stays read-only; the author makes corrections.

## 1. Pick the PR

Use the PR the user named. Otherwise list the open, non-draft PRs whose `implemented-by:` line does not name you and whose comments hold no record (`reviewed-by:` with `Reviewed at <sha>`) for the current head: `gh pr list --state open --json number,title,isDraft,headRefOid,body`, then `gh pr view <n> --comments` (`tea` on Gitea). One candidate: review it and say which. Zero or several: ask which PR to review, then stop.

## 2. State the Problem and Your Own Approach

Read the PR description and the issue it closes or addresses. State the problem in one sentence. Before opening the diff, decide how you would solve it: the files you would touch, the shape of the change, and what you would not build. The diff must not anchor you.

## 3. Review

Check out the PR head detached at `.worktrees/<agent>-review-pr-<n>` (`git worktree add --detach .worktrees/<agent>-review-pr-<n> <head-sha>`), never in the author's worktree. Read the complete base-to-head diff and the callers, tests, fixtures, and configuration it touches. Compare it with your approach: a difference is a finding only when the PR's choice is worse. Pragmatic, idiomatic, simple yet thorough code beats complex or clever code (the Ponytail ladder). For each finding, try to refute it, then report the failure mechanism, location, minimal correction, and how to verify. On a code PR, run `task verify` in the checkout and record the result verbatim.

## 4. Post the Record

Post one PR comment in the contract grammar (`gh pr comment <n> --body-file <file>`):

```text
reviewed-by: <agent> (<model>, <effort>)
Reviewed at <head-sha>
Verdict: APPROVE | REQUEST CHANGES
<findings, checks run, unavailable evidence, limitations>
```

Name your provider, model as `.agents/review-policy.json` lists it (for example `opus`), and reasoning effort. Remove the review checkout (`git worktree remove .worktrees/<agent>-review-pr-<n>`) once the comment is posted. End your conclusion with 1–2 lines naming what you skipped or did not check and any residual risk.

## 5. Clear the Merge Check

The PR's `Merge check` ran when the PR was marked ready, before your record existed, so it has failed. After `Verdict: REQUEST CHANGES`, stop here and leave it red: the gate passes only a latest record with exactly one `Verdict: APPROVE` line. After `Verdict: APPROVE`, the gate also needs your `reviewed-by: <agent> (<model>, <effort>)` line in the PR description: add it if it is missing (`gh pr view <n> --json body -q .body`, then `gh pr edit <n> --body-file <file>`). Where `Merge check` runs on the `edited` event (ateam's `merge-check.yml`), that edit, made after the record comment, reruns it at the head. If the description already carried your line, nothing was edited, so rerun the latest run once it has finished: `run=$(gh run list --branch <head-branch> --workflow merge-check.yml --limit 1 --json databaseId -q '.[0].databaseId')`, `gh run watch "$run" --exit-status || true`, then `gh run rerun "$run"` (in a repository that runs the gate inside `ci.yml`, use that workflow and `--failed`). On Gitea, rerun it from the Actions page, and say so in your reply. Do not update the branch to clear it.
