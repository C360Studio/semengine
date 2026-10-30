# Tasks: apply-ateam-rulings

Track: `quick` (no spec Requirement added, no capability touched). Owner ruling on #447 (2026-10-28): proposal
approval is adopted. A hold is written as `Hold:` inside the unticked task it stops.

## 1. Proposal

- [x] 1.1 Proposal and tasks committed; draft PR opened with `Closes #451`, `Closes #447` and `Addresses #446`.
  Gate: `task spec:check` and `task docs:check`.
  Done in c875b06; draft PR #473. Owner approved the proposal in session on 2026-10-30, choosing also to drop the two
  closed-PR worktrees (`Approved-by:` filled, recorded on #451).

## 2. Worktrees (ruled on #447 item 6; no approval needed)

- [x] 2.1 `ateam state prune` removes this agent's clean, merged worktrees and their local branches;
  `git worktree remove` the three clean detached checkouts (`claude-fix-456-guard-watcher-drain`,
  `claude-natsclient-fixture-open-195`, `claude-review-pr-472`). Kept: `claude/adr-063-store-substrate` (#435
  closed, 1 unpushed commit), `claude/fix-420-keysbyfilter-drain-join` (#453 closed, 1 unpushed commit),
  `codex/review-pr-414` (another agent's), the first two pending the owner's word. Gate: `ateam state prune --dry-run`
  prints no `would remove`;
  `ateam state pickup` lists no `merged via`; the output is pasted on the PR.
  Done 2026-10-30: prune removed 16 worktrees and their local branches (each `merged via #n; no remote branch`);
  the three detached checkouts removed by `git worktree remove`; the dry-run after prints no `would remove`;
  `ateam state pickup` prints no `merged via`; four worktrees remain (main, this claim, the two kept closed ones
  and codex's). Output on PR #473. On the owner's approval choice of 2026-10-30 the two closed-PR worktrees
  (`claude/adr-063-store-substrate` at 8f92c7a9, `claude/fix-420-keysbyfilter-drain-join` at a10b3a48) and their
  branches were dropped too; `codex/review-pr-414` alone remains beside main and this claim.

## 3. Protocol, rule rows, contract

- [x] 3.1 `.agents/protocol.md`: the track at Clarify; proposal approval and the design length warning in "Target
  state, task truth, holds"; Close hygiene in "Worktree hygiene" and "Close"; the Ponytail footer and `// shortcut:`
  marker; the test-to-spec deferral line in Verification. Gate: `task docs:check`; `task ateam:doctor` (protocol
  markers intact).
  Done in d635254; `task docs:check` exit 0, `task ateam:doctor` `"passed": true`, `"violations": null`. Notes:
  `notes/3.1.md`.
- [x] 3.2 `AGENTS.md` § 5 and `.agents/rules-index.md` § 5: the three longest § 5 rows shortened in both files, then
  three rows added to both (approval and track; Close hygiene; footer and marker), each naming its canonical home and
  what enforces it. Gate: `wc -c AGENTS.md` under 24,000; `task ateam:doctor` (row parity).
  Done in f6fea74, with cuts outside § 5 on the coordinator's decision (`notes/3.2.md`): `wc -c AGENTS.md` 23948;
  `task ateam:doctor` `"passed": true`, `"violations": null`; § 5 has 20 rows in both files.
- [x] 3.3 `.agents/contracts/semengine-architect.md` § Required workflow: the track declaration and the approval gate
  on the proposal and design. Gate: `task docs:check`.
  Done in 70e3901 (steps 1 and 6; no step reordered); `task docs:check` exit 0.

## 4. Review and archive

- [ ] 4.1 In-house review, then the cross-agent record on the PR in ateam's grammar (a `.agents/contracts/` file
  makes this a code pull request); `implemented-by:` and `reviewed-by:` with `(model, effort)` in the PR body.
- [ ] 4.2 Archive as the last content commit. Gate: `task spec:check` and `task docs:check`; `task verify` once on
  the final head.
