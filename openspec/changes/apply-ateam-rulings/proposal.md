# Proposal: apply-ateam-rulings

## Why

Issue #451, PR D of epic #446. The owner ruled on every item of #447 on 2026-10-28 (comments 6471411856 and
6475177495). PRs A, B and C applied the items that had a home in code (pickup and handoff, mutcheck kept, the merge
gate, the spec queue) and already work to items 1 and 2 (PR B and PR C each carried `Approved-by:` and the `OPEN-Q`
hold on a `design`-track change). The protocol, the rule rows and the architect contract still say nothing about
them, so a new session would not know the rules it is held to. Sixteen worktrees under `.worktrees/` belong to
merged pull requests (`ateam state prune --dry-run`, 2026-10-30), not the eight #447 counted on 2026-10-28.

Owner ruling on this epic: match ateam unless there is a very good reason not to, and record the divergence.

Approved-by: Coby Leuschke (2026-10-30), in session, recorded on #451.

## What Changes

- `.agents/protocol.md`, in the Work lifecycle: Clarify also declares the track, `quick` (proposal and tasks) or
  `design` (plus `design.md` from the design skill), `design` being required when the change adds a spec Requirement
  or touches more than one capability under `openspec/specs/` (item 2); the "Target state, task truth, holds" bullet
  states proposal approval, one page with `Approved-by:` the owner fills and the first task's
  `Hold: OPEN-Q proposal awaits owner approval`, the one hold covering proposal and design, the 120-line design
  warning accepted (items 1 and 3); Close hygiene, the claiming agent removes its worktree and remote branch after the
  squash merge, never another agent's, never a dirty or unpushed one, with `ateam state prune` as the tool (item 6);
  the Ponytail footer on conclusions and handoffs and the `// shortcut: <limit>, <when to upgrade>` marker that
  `ateam state debt` reads (item 8); and in Verification one line deferring test-to-spec attribution until
  ateam #237 ships a brownfield mode (item 5).
- `AGENTS.md` § 5 and `.agents/rules-index.md` § 5: three rows, one per adopted rule (approval and track; Close
  hygiene; footer and marker), added to both so the counts stay equal. `AGENTS.md` is at 23,890 of 24,000 bytes, so
  the three longest existing rows in § 5 are shortened first, in both files, without losing a clause.
- `.agents/contracts/semengine-architect.md` § Required workflow: the track is declared before the inventory, a
  `design`-track change drafts `design.md` through the design skill, and the design is not approved until the owner
  fills `Approved-by:` and removes the hold; inventory-first stays on top.
- Worktrees: `ateam state prune` removes this agent's clean, merged worktrees and their local branches (sixteen), and
  `git worktree remove` the three clean detached checkouts whose heads are on `main` or reviewed a merged pull
  request. Kept, under the ruling's own words: the two closed-without-merge worktrees (#435, #453), whose one
  commit each is on no remote, and `codex/review-pr-414`, another agent's. The remote branches of the merged ones
  are already gone (GitHub deletes them on merge).

Divergence recorded: none. The `.agents/contracts/` edit makes this a code pull request under the gate's classifier,
so it carries a cross-agent record like any other.

## Capabilities

### Modified Capabilities

None. The change is protocol, rule rows and a role contract; no capability under `openspec/specs/` changes.
