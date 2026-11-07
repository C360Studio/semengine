# harness-boundaries

## MODIFIED Requirements

### Requirement: ateam doctor gates verify

`scripts/verify.sh` SHALL run `task ateam:doctor` as its first step, and `task ateam:doctor` SHALL run `.bin/ateam
doctor --check` with the session identity variables (`ATEAM_SESSION_ID`, `CLAUDE_SESSION_ID`, `CODEX_SESSION_ID`,
`GEMINI_SESSION_ID`) unset, so that the check never takes the writer lease. The ateam release is pinned as `v0.3.9` in
`.github/workflows/ci.yml` and `.github/workflows/merge-check.yml`, every install line naming the same version, compared
by `scripts/doctor.sh`.

#### Scenario: Doctor violation fails verify

- **WHEN** `ateam doctor --check` reports a violation (a missing skill adapter, a drifted base contract, a protocol
  marker missing, `AGENTS.md` at 24,000 bytes or more, a rule-row count that differs between `AGENTS.md` and
  `.agents/rules-index.md`, or session-start hook latency above its 100 ms ceiling)
- **THEN** `task verify` fails at the `ateam:doctor` step naming the file and the rule

#### Scenario: Doctor warning does not fail verify

- **WHEN** `ateam doctor --check` reports only a warning (hook latency above the 10 ms target but below 100 ms, or a
  shared forge token)
- **THEN** `task verify` continues

### Requirement: ateam template files are the pin's

Every file `ateam init` writes into an empty directory at the pinned release SHALL be byte-identical in this repository,
except the files SemEngine writes itself and the five client configurations an adopter may edit (both listed with
reasons in `semengineOwned`). The tool test `TestAteamTemplatesUnchanged` (`internal/harness/toolcheck`) SHALL check it,
failing and never skipping when `.bin/ateam` (built by `task ateam:bin`) is missing or is not the pinned release.

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

- **WHEN** an MCP server is added to `.mcp.json`, or a permission to `.claude/settings.json`, `.codex/config.toml`,
  `.codex/hooks.json` or `.gemini/settings.json`
- **THEN** `TestAteamTemplatesUnchanged` does not report it; `ateam doctor --check` still checks the file's ateam hook
  and MCP registrations

#### Scenario: ateam is not installed

- **WHEN** `.bin/ateam` does not exist, whatever `ateam` is on `PATH`
- **THEN** `TestAteamTemplatesUnchanged` fails naming `task ateam:bin`
