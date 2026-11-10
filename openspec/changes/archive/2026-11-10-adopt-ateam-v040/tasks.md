# Tasks: adopt-ateam-v040

Each step ends when `task verify` passes on a clean tree at its commit (#171), with `.bin/ateam` at v0.4.0
(`task ateam:bin`). One PR.

## 0. Approval

- [x] 0.1 The owner approves the proposal (Enjoy conversation "ateam v0.4.0 upgrade issue", 2026-11-10: "I approved
  the proposal").

## 1. Pin and templates

- [x] 1.1 Pin `v0.4.0` in both workflows, the merge-gate tool tests, `Taskfile.yml` and `.agents/README.md`; copy
  v0.4.0's base contracts and `design`, `handoff`, `pickup`, `review-pr` skill text unedited, with `t3-card.html`;
  `ateam doctor --check` and `TestAteamTemplatesUnchanged` pass at v0.4.0.
- [x] 1.2 The `Merge check probe` job grants `checks: read`; `TestCIWorkflowPinnedSensitivity` fails when the probe
  loses it or the `merge-check` job gains it.

## 2. Prose and spec

- [x] 2.1 `.agents/protocol.md` states the digest form of `Approved-by:` and maps ateam's protocol section numbers to
  this file's sections.
- [x] 2.2 The spec deltas applied to `openspec/specs/` and the change archived.
