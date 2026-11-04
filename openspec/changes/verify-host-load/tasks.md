# Tasks: verify-host-load

Each step ends when `task verify` passes on a clean tree at its commit (#171). One PR; closes #519.

## 1. Script

- [x] 1.1 `scripts/verify.sh` prints the `host:` line at the start and in the step-timings block, and the warning line
  when the one-minute load average is above twice the core count; shown on this Mac at load 2.7 (no warning) and under a
  synthetic load (warning), both in the PR.

## 2. Protocol, skill, spec

- [x] 2.1 `.agents/protocol.md` "Known flakes" sentence; `.agents/skills/preflight/SKILL.md` pointer;
  `openspec/specs/merge-gate/spec.md` delta applied.
