# harness-boundaries

## ADDED Requirements

### Requirement: ateam doctor gates verify

`scripts/verify.sh` SHALL run `task ateam:doctor` as its first step, and `task ateam:doctor` SHALL run
`ateam doctor --check` with the session identity variables (`ATEAM_SESSION_ID`, `CLAUDE_SESSION_ID`,
`CODEX_SESSION_ID`, `GEMINI_SESSION_ID`) unset, so that the check never takes the writer lease. The ateam release is
pinned once as `v0.3.3` in `.github/workflows/ci.yml` and compared by `scripts/doctor.sh`.

#### Scenario: Doctor violation fails verify

- **WHEN** `ateam doctor --check` reports a violation (a missing skill adapter, a drifted base contract, a protocol
  marker missing, `AGENTS.md` at 24,000 bytes or more, a rule-row count that differs between `AGENTS.md` and
  `.agents/rules-index.md`)
- **THEN** `task verify` fails at the `ateam:doctor` step naming the file and the rule

#### Scenario: Doctor warning does not fail verify

- **WHEN** `ateam doctor --check` reports only a warning (hook latency above the 10 ms target but below 25 ms, or a
  shared forge token)
- **THEN** `task verify` continues

### Requirement: ateam template files are the pin's

Every file `ateam init` writes into an empty directory at the pinned release SHALL be byte-identical in this
repository, except the files SemEngine writes itself (listed with reasons in `semengineOwned`). The tool test
`TestAteamTemplatesUnchanged` (`internal/harness/toolcheck`) SHALL check it, failing and never skipping when `ateam`
is missing or is not the pinned release.

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

#### Scenario: ateam is not installed

- **WHEN** `ateam` is not on `PATH`
- **THEN** `TestAteamTemplatesUnchanged` fails naming the pinned install line

### Requirement: SemEngine contracts sit on the base contracts

A SemEngine rule on top of a base contract SHALL live in the matching `semengine-*` contract, whose first paragraph
names the base contract to read first: `developer.md` for the developer and the technical writer, `reviewer.md` for
the reviewer, `architect.md` for the architect. `TestAgentSetupParity` SHALL check the first paragraph.

#### Scenario: A SemEngine rule is added

- **WHEN** a rule applies only to SemEngine's reviewer
- **THEN** it is written in `.agents/contracts/semengine-reviewer.md`, not in `reviewer.md`

#### Scenario: The base-contract pointer is removed

- **WHEN** the first paragraph of `.agents/contracts/semengine-reviewer.md` no longer names
  `.agents/contracts/reviewer.md`
- **THEN** the contract test fails naming the file

## MODIFIED Requirements

### Requirement: Agent setup and rule-index parity

Agent-setup parity SHALL be checked by two oracles that do not overlap: `ateam doctor --check` (run by
`task ateam:doctor` inside `task verify`) for the files ateam owns, the hook and MCP registrations, the canonical
skills and their adapters, the adopter skills' links, `CLAUDE.md`, the `AGENTS.md` size and its rule-row parity with
`.agents/rules-index.md`; and the contract test `TestAgentSetupParity` with `TestAgentSetupParitySensitivity`
(`task test:unit`) for the SemEngine role adapters and the adopter skills' adapter size, in the steps stated in the
requirements that follow.

#### Scenario: Repository agent setup passes

- **WHEN** `ateam doctor --check` and `TestAgentSetupParity` run on the repository root
- **THEN** neither reports a violation

#### Scenario: Planted violation in any of Steps 1 to 8 fails

- **WHEN** a fixture tree omits a `semengine-*` contract or role adapter, points an adapter at the wrong contract,
  grants `Edit` or `LSP` to a read-only Claude adapter, omits `sandbox_mode = "read-only"` on a read-only Codex
  adapter or sets `sandbox_mode` on a write Codex adapter, adds a checkbox or exceeds 20 lines in a role adapter,
  exceeds 25 lines or adds a checkbox in an adopter skill's Claude adapter, or changes a pinned `model`, `effort` or
  `model_reasoning_effort`
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 1: required files exist

Step 1. All four SemEngine contracts (`.agents/contracts/semengine-{architect,developer,reviewer,technical-writer}.md`)
and all eight role adapters (`.claude/agents/semengine-*.md`, `.codex/agents/semengine-*.toml`) SHALL exist. The
files ateam owns (`AGENTS.md`, `CLAUDE.md`, `.agents/README.md`, `.agents/protocol.md`, `.agents/rules-index.md`,
`openspec/config.yaml`) are checked by `ateam doctor --check`; the base contracts by `TestAteamTemplatesUnchanged`.

#### Scenario: Missing file

- **WHEN** a fixture tree omits a SemEngine contract or a role adapter
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 5: CLAUDE.md, AGENTS.md and the rule index

Step 5. `CLAUDE.md` SHALL contain `@AGENTS.md`, have at most 10 lines, and contain no section heading (`##`) or table;
`AGENTS.md` SHALL be strictly smaller than 24,000 bytes; and `AGENTS.md` and `.agents/rules-index.md` SHALL have the
same numbered rule-domain headings and the same number of rule rows in each domain table. This step is checked by
`ateam doctor --check`, not by the contract test.

#### Scenario: AGENTS.md and rule index drift

- **WHEN** `CLAUDE.md` gains a rule table, `AGENTS.md` reaches 24,000 bytes, or a rule row is added to `AGENTS.md`
  without `.agents/rules-index.md`
- **THEN** `ateam doctor --check` reports the violation and `task verify` fails

### Requirement: Agent setup step 7: skill adapters

Step 7. Every canonical skill under `.agents/skills/<skill>/SKILL.md` SHALL have its matching Claude adapter under
`.claude/skills/<skill>/SKILL.md` that links to `../../../.agents/skills/<skill>/SKILL.md` and says to read it `fully`;
`ateam doctor --check` checks existence and links for every skill and the required section markers for the
canonical six. The contract test checks that each adopter skill's Claude adapter (`entity-or-bucket`, `kv-or-stream`,
`new-payload`, `query-pattern`, `semengine-status`) has at most 25 lines and contains no task-list checkbox.

#### Scenario: Broken skill adapter link

- **WHEN** a Claude skill adapter's link does not resolve
- **THEN** `ateam doctor --check` reports it and `task verify` fails

#### Scenario: Adopter skill adapter with a copied checklist

- **WHEN** a fixture adds `- [ ]` to `.claude/skills/query-pattern/SKILL.md`
- **THEN** the contract test fails naming the file and the violated rule

### Requirement: Agent setup step 8: Claude model pins

Step 8, Claude. Claude adapters SHALL pin `model: opus` and `effort: medium` for `semengine-architect` and
`semengine-developer`, `model: opus` and `effort: high` for `semengine-reviewer`, and `model: sonnet` and
`effort: medium` for `semengine-technical-writer`.

#### Scenario: Claude model changed

- **WHEN** a fixture changes a pinned Claude `model` or `effort`
- **THEN** the check fails naming the file and the violated rule
