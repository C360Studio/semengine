# harness-boundaries

## MODIFIED Requirements

### Requirement: ateam doctor gates verify

`scripts/verify.sh` SHALL run `task ateam:doctor` first, and `task ateam:doctor` SHALL run `scripts/ateam doctor
--check` with the session variables (`ATEAM_SESSION_ID`, `CLAUDE_SESSION_ID`, `CODEX_SESSION_ID`, `GEMINI_SESSION_ID`)
unset so it never takes the writer lease. The ateam release is pinned as `v0.4.0` in `.github/workflows/ci.yml`,
`merge-check.yml`, `Taskfile.yml`, and `.agents/ateam.json`, compared by `scripts/doctor.sh` and `TestAteamPinParity`.

#### Scenario: Doctor violation fails verify

- **WHEN** `ateam doctor --check` reports a violation (a missing skill adapter, a drifted base contract, a protocol
  marker missing, `AGENTS.md` at 24,000 bytes or more, a rule-row count that differs between `AGENTS.md` and
  `.agents/rules-index.md`, a binary or hook binary version mismatch against `.agents/ateam.json`, or session-start hook
  latency above its 100 ms ceiling)
- **THEN** `task verify` fails at the `ateam:doctor` step naming the file and the rule

#### Scenario: Doctor warning does not fail verify

- **WHEN** `ateam doctor --check` reports only a warning (hook latency above the 10 ms target but below 100 ms, or a
  shared forge token)
- **THEN** `task verify` continues

### Requirement: ateam template files are the pin's

Every file `ateam init` writes into an empty directory at the pinned release SHALL be byte-identical in this repository,
except the files SemEngine writes itself and the client and plugin configurations an adopter edits to invoke
`scripts/ateam` (both listed with reasons in `semengineOwned`). `TestAteamTemplatesUnchanged`
(`internal/harness/toolcheck`) SHALL check it via `scripts/ateam`, failing and never skipping when `scripts/ateam`
(from `task tools:ateam`) is missing or is not the pin.

#### Scenario: A line is added to a base contract

- **WHEN** a line is added to `.agents/contracts/reviewer.md`, or `.agents/contracts/architect.md` is deleted
- **THEN** `TestAteamTemplatesUnchanged` fails naming the file and `task verify` fails; `ateam doctor --check` at
  v0.3.3 does not report either

#### Scenario: An ateam paragraph is edited

- **WHEN** a word inside an ateam paragraph of `.agents/contracts/reviewer.md` changes
- **THEN** `ateam doctor --check` (which checks only that `developer.md` and `reviewer.md` still hold each ateam
  paragraph, and that canonical skills carry their section markers) and `TestAteamTemplatesUnchanged` both fail

#### Scenario: A file SemEngine writes itself

- **WHEN** `AGENTS.md`, `CLAUDE.md`, `.gitignore`, `.agents/README.md`, `.agents/protocol.md`,
  `.agents/rules-index.md`, either `preflight` file, or the `openspec/` scaffold differs from ateam's template
- **THEN** `TestAteamTemplatesUnchanged` does not report it

#### Scenario: A client configuration gains a server

- **WHEN** an MCP server is added to `.mcp.json`, or a permission or `scripts/ateam` command to `.claude/settings.json`,
  `.codex/config.toml`, `.codex/hooks.json`, `.gemini/settings.json`, `.agents/hooks.json`, or
  `.agents/plugins/ateam/mcp_config.json`
- **THEN** `TestAteamTemplatesUnchanged` does not report it; `ateam doctor --check` still checks the file's ateam hook
  and MCP registrations

#### Scenario: ateam is not installed

- **WHEN** `scripts/ateam` is missing or does not report the pinned release
- **THEN** `TestAteamTemplatesUnchanged` fails naming `task tools:ateam`

## ADDED Requirements

### Requirement: scripts/ateam resolves the main checkout's .tools/bin/ateam

`scripts/ateam` SHALL be an executable POSIX `/bin/sh` shim that resolves the main checkout from a linked worktree's
`.git` file without invoking `git`, execs `<main>/.tools/bin/ateam` first, then `<worktree>/.tools/bin/ateam`, then
`ateam` on `PATH`, and exits 127 naming `task tools:ateam` when none is found. `TestAteamShimResolution`
(`internal/harness/toolcheck`) SHALL verify this resolution order and its sensitivity to mutations.

#### Scenario: Linked worktree prefers main .tools/bin/ateam without git

- **WHEN** `scripts/ateam` runs in a linked worktree with binaries planted in `<main>/.tools/bin/ateam`,
  `<worktree>/.tools/bin/ateam`, and `PATH`, and `git` is not on `PATH`
- **THEN** it execs `<main>/.tools/bin/ateam` and passes all arguments through

#### Scenario: Missing binary exits 127 naming task tools:ateam

- **WHEN** neither `.tools/bin/ateam` nor `ateam` on `PATH` exists
- **THEN** `scripts/ateam` exits 127 with a message on standard error naming `task tools:ateam`
