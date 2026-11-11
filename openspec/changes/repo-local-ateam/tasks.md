# Tasks: repo-local-ateam

Each step ends when `task verify` passes on a clean tree at its commit (#171). One PR (#595).

## 1. Repo-local shim, pin, and gates

- [ ] 1.1 Add `.agents/ateam.json` (`v0.4.0`) and executable `scripts/ateam`; replace `.bin/` and `ateam:bin` with
  `.tools/` and `tools:ateam` in `.gitignore`, `Taskfile.yml`, and `scripts/doctor.sh`; update all seven hook and MCP
  configuration files to invoke `scripts/ateam`.
- [ ] 1.2 Update `internal/harness/toolcheck/{ateamtemplates,mergegate,specwiring}_test.go` and `doc.go`, add
  `internal/harness/toolcheck/ateamshim_test.go` (`TestAteamShimResolution`), and update `docs/testing.md` and
  `.agents/skills/preflight/SKILL.md`.

## 2. Spec

- [ ] 2.1 Apply the spec deltas to `openspec/specs/{harness-boundaries,spec-queue}/spec.md` and archive the change.
