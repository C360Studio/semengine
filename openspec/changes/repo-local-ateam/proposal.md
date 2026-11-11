# Proposal: repo-local-ateam

Track: design (two capabilities, `harness-boundaries` and `spec-queue`; four requirements modified, one added).

## Why

PRs #545 and #547 added `task ateam:bin`, which builds the pinned `ateam` into `.bin/ateam` per worktree, while the
seven hook and MCP configuration files (`.claude/settings.json`, `.codex/hooks.json`, `.codex/config.toml`,
`.gemini/settings.json`, `.mcp.json`, `.agents/hooks.json`, `.agents/plugins/ateam/mcp_config.json`) still invoke bare
`ateam` on `PATH`, and `.agents/ateam.json` was deferred in PR #586. Every new claim worktree rebuilds `.bin/ateam`,
hook and MCP invocations run whatever `ateam` is first on `PATH`, and `ateam doctor --check` does not verify the pinned
version against `.agents/ateam.json`. Adopting the `C360Studio/ateam` README pattern (shared `<main>/.tools/bin/ateam`
via `task tools:ateam`, `scripts/ateam` shim, and `.agents/ateam.json`) fixes all three, matching `semteams` (#414, #430).

The owner asked for this migration on 2026-11-11 in the Enjoy conversation "Migrate to repo-local ateam" ("we need to
migrate to our own repo version of ateam - semteams has the pattern that ateam readme describes. let's file and issue
and claim via pr and run to ready for merge pleasse").

Approved-by: Coby Leuschke (2026-11-11) sha256:9e661a6e1ce9

## What Changes

- Commit `.agents/ateam.json` with `{"version": 1, "ateam_version": "v0.4.0"}` and executable `scripts/ateam` (`0755`).
- Replace `task ateam:bin` and `.bin/` in `Taskfile.yml` and `.gitignore` with `task tools:ateam` (`ATEAM_VERSION`,
  `ATEAM_BIN_DIR` in the main checkout's `.tools/bin`, `.tools/` ignored).
- Point all seven hook and MCP configuration files, `Taskfile.yml` gates (`doctor`, `ateam:doctor`, `spec:check`,
  `spec:queue`, `merge:check`), `scripts/doctor.sh`, and `TestAteamTemplatesUnchanged` at `scripts/ateam` and
  `tools:ateam`.
- Add `TestAteamShimResolution` (`internal/harness/toolcheck/ateamshim_test.go`) and pin parity checks across
  workflows, `Taskfile.yml`, and `.agents/ateam.json`.

## Capabilities

- Modified `harness-boundaries`: "ateam doctor gates verify", "ateam template files are the pin's"; added
  "scripts/ateam resolves the main checkout's .tools/bin/ateam".
- Modified `spec-queue`: "The spec queue is ateam state spec-queue at the pin", "spec:check runs validate then ateam
  state spec-check".

## Impact

- `.agents/ateam.json`, `scripts/ateam`, `.gitignore`, the seven hook/MCP files, `Taskfile.yml`, `scripts/doctor.sh`,
  `internal/harness/toolcheck/`, `docs/testing.md`, `.agents/skills/preflight/SKILL.md`, and the two specs.
- Closes #595.
