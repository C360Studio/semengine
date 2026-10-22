# Tasks: port-status-tracker

Each task names its outcome and the gate that proves it.

## 1. Design

- [x] 1.1 Proposal, design, tasks, and `port-status` spec delta pass `task spec:check` and `task docs:check` (owner
  decisions recorded in Enjoy session "Port Status and Issue Tracking Audit", 2026-10-22).

## 2. Implementation and tests

- [ ] 2.1 Write `internal/harness/contract/portstatus_test.go` covering `--offline`, `--check` pass/fail cases with a
  fake `gh` binary (missing milestone, missing label, whitespace milestone title, `gh` read failure exit 2), and
  `Taskfile.yml` wiring; confirm it fails before `scripts/port-status.sh` exists.
- [ ] 2.2 Implement `scripts/port-status.sh`, wire `status` in `Taskfile.yml` and `AGENTS.md`, add
  `.agents/skills/semengine-status/SKILL.md` and `.claude/skills/semengine-status/SKILL.md`, and register
  `semengine-status` in `.agents/README.md` and `internal/harness/contract/agentsetup_test.go`. Gate:
  `go test -race ./internal/harness/contract/` and `task status -- --check`.
- [ ] 2.3 Update `docs/repository-map.md` for Change 3 progress, the slugged milestone names (`setup`, `slice-04a`,
  `slice-04b`, `slice-04c`), and `scripts/port-status.sh`. Gate: `task verify` passes on a clean tree.

## 3. Review and archive

- [ ] 3.1 Cross-agent implementation review of record on the pull request, with any findings fixed and `reviewed-by:`
  recorded in the PR description beside `implemented-by:`.
- [ ] 3.2 Archive `port-status-tracker`, sync `openspec/specs/port-status/spec.md`, and update `docs/repository-map.md`
  with the archive row as the final content commit. Gate: `task spec:check` and `task docs:check`.
