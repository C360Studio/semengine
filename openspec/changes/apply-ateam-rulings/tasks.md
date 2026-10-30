# Tasks: apply-ateam-rulings

Track: `quick` (no spec Requirement added, no capability touched). Owner ruling on #447 (2026-10-28): proposal
approval is adopted. A hold is written as `Hold:` inside the unticked task it stops.

## 1. Proposal

- [ ] 1.1 Proposal and tasks committed; draft PR opened with `Closes #451`, `Closes #447` and `Addresses #446`.
  Gate: `task spec:check` and `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval.

## 2. Worktrees (ruled on #447 item 6; no approval needed)

- [ ] 2.1 `ateam state prune` removes this agent's clean, merged worktrees and their local branches;
  `git worktree remove` the three clean detached checkouts (`claude-fix-456-guard-watcher-drain`,
  `claude-natsclient-fixture-open-195`, `claude-review-pr-472`). Kept: `claude/adr-063-store-substrate` (#435
  closed, 1 unpushed commit), `claude/fix-420-keysbyfilter-drain-join` (#453 closed, 1 unpushed commit),
  `codex/review-pr-414` (another agent's). Gate: `ateam state prune --dry-run` prints no `would remove`;
  `ateam state pickup` lists no `merged via`; the output is pasted on the PR.

## 3. Protocol, rule rows, contract

- [ ] 3.1 `.agents/protocol.md`: the track at Clarify; proposal approval and the design length warning in "Target
  state, task truth, holds"; Close hygiene in "Worktree hygiene" and "Close"; the Ponytail footer and `// shortcut:`
  marker; the test-to-spec deferral line in Verification. Gate: `task docs:check`; `task ateam:doctor` (protocol
  markers intact).
- [ ] 3.2 `AGENTS.md` § 5 and `.agents/rules-index.md` § 5: the three longest § 5 rows shortened in both files, then
  three rows added to both (approval and track; Close hygiene; footer and marker), each naming its canonical home and
  what enforces it. Gate: `wc -c AGENTS.md` under 24,000; `task ateam:doctor` (row parity).
- [ ] 3.3 `.agents/contracts/semengine-architect.md` § Required workflow: the track declaration and the approval gate
  on the proposal and design. Gate: `task docs:check`.

## 4. Review and archive

- [ ] 4.1 In-house review, then the cross-agent record on the PR in ateam's grammar (a `.agents/contracts/` file
  makes this a code pull request); `implemented-by:` and `reviewed-by:` with `(model, effort)` in the PR body.
- [ ] 4.2 Archive as the last content commit. Gate: `task spec:check` and `task docs:check`; `task verify` once on
  the final head.
