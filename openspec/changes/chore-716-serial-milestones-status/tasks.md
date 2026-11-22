# Tasks: chore-716-serial-milestones-status

Each code task ends at a clean-tree local `task verify`. Single PR, closing #716.

## 1. Proposal

- [ ] 1.1 Proposal (`proposal.md`, `tasks.md`, spec delta); `task spec:check` and `task docs:check` pass.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Implementation

- [ ] 2.1 Update `scripts/port-status.sh` Project v2 `Stage` mapping for serial `v0.1.0-alpha.<N>` milestones and add
  regression checks in `internal/harness/toolcheck/portstatus_test.go` (shown able to fail via mutant check).
- [ ] 2.2 Update `docs/repository-map.md`, `.agents/skills/semengine-status/SKILL.md`, and
  `.claude/skills/semengine-status/SKILL.md` for the serial milestone model and current alpha tags.

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive chore-716-serial-milestones-status --yes`; `task spec:check` and `task docs:check`
  pass.
