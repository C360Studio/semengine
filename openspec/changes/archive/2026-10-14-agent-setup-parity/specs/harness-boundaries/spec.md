# harness-boundaries

## ADDED Requirements

### Requirement: Agent setup and rule-index parity

`task test:unit` SHALL run a contract test (`TestAgentSetupParity` and `TestAgentSetupParitySensitivity`) that checks
the tracked agent contracts, platform adapters, skill adapters, and rule index against `.agents/README.md`:

1. All four canonical contracts (`.agents/contracts/semengine-{architect,developer,reviewer,technical-writer}.md`), all
   eight role adapters (`.claude/agents/semengine-*.md`, `.codex/agents/semengine-*.toml`), `.agents/protocol.md`,
   `.agents/rules-index.md`, `.agents/README.md`, `AGENTS.md`, and `CLAUDE.md` SHALL exist.
2. Each role adapter SHALL declare `name` equal to its role, reference only its matching `.agents/contracts/<role>.md`
   path, and instruct the agent to read that contract fully first.
3. Read-only Claude adapters (`semengine-architect`, `semengine-reviewer`) SHALL list `tools: Read, Bash, Grep, Glob,
   Skill` and SHALL NOT list `Edit`, `Write`, `Task`, or `LSP`; write Claude adapters (`semengine-developer`,
   `semengine-technical-writer`) SHALL list `tools: Read, Edit, Write, Bash, Grep, Glob, Skill` and SHALL NOT list
   `Task` or `LSP`.
4. Read-only Codex adapters (`semengine-architect`, `semengine-reviewer`) SHALL set `sandbox_mode = "read-only"`; write
   Codex adapters (`semengine-developer`, `semengine-technical-writer`) SHALL NOT set `sandbox_mode`.
5. `CLAUDE.md` SHALL contain `@AGENTS.md`, have at most 10 lines, and contain no section heading (`##`) or table;
   `AGENTS.md` SHALL be strictly smaller than 24,000 bytes; and `AGENTS.md` and `.agents/rules-index.md` SHALL have the
   same numbered rule-domain headings and the same number of rule rows in each domain table.
6. Every role adapter SHALL have at most 20 lines and SHALL NOT contain a markdown task-list checkbox (`- [ ]` or
   `- [x]`).
7. Every canonical skill under `.agents/skills/<skill>/SKILL.md` SHALL have its matching Claude adapter under
   `.claude/skills/<cmd>/SKILL.md` (`preflight` for `semengine-preflight`, `<skill>` otherwise) that links to
   `../../../.agents/skills/<skill>/SKILL.md`, says to read it `fully`, has at most 25 lines, and contains no task-list
   checkbox.
8. Claude adapters SHALL pin `model: opus` for `semengine-architect`, `semengine-developer`, and `semengine-reviewer`,
   and `model: sonnet` for `semengine-technical-writer`; Codex adapters SHALL pin `model = "gpt-6-astra"` and
   `model_reasoning_effort = "medium"` for `semengine-architect`, `model = "gpt-6-sol"` and
   `model_reasoning_effort = "medium"` for `semengine-developer` and `semengine-technical-writer`, and
   `model = "gpt-6-astra"` and `model_reasoning_effort = "high"` for `semengine-reviewer`.

#### Scenario: Repository agent setup passes

- **WHEN** `TestAgentSetupParity` runs on the repository root
- **THEN** it reports no violation

#### Scenario: Planted violation in any of Steps 1 to 8 fails

- **WHEN** a fixture tree omits a canonical contract or adapter, points an adapter at the wrong contract, grants `Edit`
  or `LSP` to a read-only Claude adapter, omits `sandbox_mode = "read-only"` on a read-only Codex adapter or sets
  `sandbox_mode` on a write Codex adapter, adds a rule table to `CLAUDE.md`, grows `AGENTS.md` to 24,000 bytes or more,
  adds a rule row to `AGENTS.md` without `.agents/rules-index.md`, adds a checkbox or exceeds 20 lines in a role
  adapter, breaks a Claude skill adapter link, or changes a pinned `model` or `model_reasoning_effort`
- **THEN** the check fails naming the file and the violated rule
