# Tasks: adopt-ateam-v045

Each step ends when `task verify` passes on a clean tree at its commit, with ateam v0.4.5 installed. One PR.

## 0. Approval

- [ ] 0.1 The owner approves the proposal. Hold: OPEN-Q proposal awaits owner approval

## 1. Pin and templates

- [ ] 1.1 Pin `v0.4.5` in both workflows, the merge-gate tool tests, `Taskfile.yml`, `.agents/ateam.json` and
  `.agents/README.md`; copy v0.4.5's base contracts and `design`, `handoff`, `pickup`, `ponytail`, `review-pr` skill
  text unedited. Accept: `ateam doctor --check`, `TestAteamTemplatesUnchanged` and `TestAteamPinParity` pass.

## 2. SemEngine's own text

- [ ] 2.1 Protocol, `AGENTS.md`, rules index, architect contract, `docs/testing.md` and the review policy as the
  proposal lists. Accept: `task ateam:doctor` (size and row parity) and `task docs:check`.
- [ ] 2.2 The spec delta applied to `openspec/specs/` and the change archived. Accept: `task spec:check`.
