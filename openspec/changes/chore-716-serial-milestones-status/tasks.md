# Tasks: chore-716-serial-milestones-status

Each code task ends at a clean-tree local `task verify`. Single PR, closing #716.

## 1. Proposal

- [x] 1.1 Proposal (`proposal.md`, `tasks.md`, spec delta); `task spec:check` and `task docs:check` pass.

## 2. Implementation

- [x] 2.1 Update `scripts/port-status.sh` Project v2 `Stage` mapping (including moving existing items whose `Stage`
  prefix differs from their computed slot) and add regression checks in `internal/harness/toolcheck/portstatus_test.go`
  (shown able to fail via mutant check, `notes/2.1.md`).
- [x] 2.2 Update `docs/repository-map.md`, `.agents/skills/semengine-status/SKILL.md`, and
  `.claude/skills/semengine-status/SKILL.md` for the serial milestone model, current alpha tags, and Project v2 `Stage`
  slots.
- [ ] 2.3 Rename the six live `SemEngine Port Tracker` (#1) `Stage` options to match slots 1–6 and verify
  `task status -- --sync-project` reconciles open items onto their milestone slots.
  Hold: awaits owner confirmation on renaming org Project #1 Stage options

## 3. Close-out

- [ ] 3.1 Archive: `openspec archive chore-716-serial-milestones-status --yes`; `task spec:check` and `task docs:check`
  pass.
