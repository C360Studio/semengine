# Design: agent-setup-parity

## Context

Issue #225 (`harness(agents): pin adapter model/effort, add gemini to merge-check, and test agent parity smoke`) turns
the agent-routing, `AGENTS.md` size-cap, and rule-index parity rules from PR #226 into tracked adapter pins and
automated checks. Before starting code, the owner confirmed three design choices under the Clarify gate:

1. **Stop boundary**: implement the OpenSpec change, adapter updates, `scripts/merge-check.sh` changes, and contract
   tests in a draft pull request with `task verify` green, paused for cross-agent review.
2. **Code-file classification in `scripts/merge-check.sh`**: treat `.agents/contracts/*` as code files alongside
   `.claude/agents/*`.
3. **Default Claude model pin for `semengine-technical-writer`**: pin `model: sonnet` in
   `.claude/agents/semengine-technical-writer.md` and update `.agents/README.md` to match.

## Decisions

### D1 Adapter model and effort pins

The eight role adapters under `.claude/agents/` and `.codex/agents/` pin explicit model and reasoning-effort defaults:

| Role | `.claude/agents/<role>.md` | `.codex/agents/<role>.toml` |
| --- | --- | --- |
| `semengine-architect` | `model: opus` | `model = "gpt-6-astra"`, `model_reasoning_effort = "medium"`, `sandbox_mode = "read-only"` |
| `semengine-developer` | `model: opus` | `model = "gpt-6-sol"`, `model_reasoning_effort = "medium"` |
| `semengine-reviewer` | `model: opus` | `model = "gpt-6-astra"`, `model_reasoning_effort = "high"`, `sandbox_mode = "read-only"` |
| `semengine-technical-writer` | `model: sonnet` | `model = "gpt-6-sol"`, `model_reasoning_effort = "medium"` |

### D2 Three-agent review check and `.agents/contracts/` classification in `scripts/merge-check.sh`

- **Document vs code names**: a changed file name (or `previous_filename` on rename) is a document name when it ends in
  `.md` or starts with `openspec/`, and does not start with `.claude/agents/` or `.agents/contracts/`. Every other name
  is a code name.
- **Agent names**: an agent name in `implemented-by:` or `reviewed-by:` is `claude`, `codex`, or `gemini`, in any
  letter case, with no letter or digit directly before or after it.
- **Pairing rule**:
  - When the author is not a bot, `implemented-by:` must appear once at the start of a line and name at least one of
    `claude`, `codex`, or `gemini`.
  - `reviewed-by:` must appear once at the start of a line and name exactly one of `claude`, `codex`, and `gemini`.
  - When `implemented-by:` names a single agent (`claude`, `codex`, or `gemini`), `reviewed-by:` must name one of the
    other two agents (`ragents != iagents`). When `implemented-by:` names more than one agent, or when the author is a
    bot, `reviewed-by:` may name any single agent of the three.

### D3 Contract test `TestAgentSetupParity` and `TestAgentSetupParitySensitivity`

`internal/harness/contract/agentsetup_test.go` checks the repository against Steps 1–8 of `.agents/README.md`:

1. All four canonical contracts (`.agents/contracts/semengine-*.md`), all eight role adapters
   (`.claude/agents/semengine-*.md`, `.codex/agents/semengine-*.toml`), `.agents/protocol.md`, `.agents/rules-index.md`,
   `.agents/README.md`, `AGENTS.md`, and `CLAUDE.md` exist.
2. Each role adapter's `name` matches its role, references only its matching `.agents/contracts/<role>.md` path, and
   instructs the agent to read that contract fully first.
3. Claude read-only adapters (`semengine-architect`, `semengine-reviewer`) list `Read, Bash, Grep, Glob, Skill` and
   never `Edit`, `Write`, `Task`, or `LSP`; write adapters (`semengine-developer`, `semengine-technical-writer`) list
   `Read, Edit, Write, Bash, Grep, Glob, Skill` and never `Task` or `LSP`.
4. Codex read-only adapters set `sandbox_mode = "read-only"`; write adapters set no `sandbox_mode`.
5. `CLAUDE.md` imports `@AGENTS.md`, stays within 10 lines, and carries no section headings (`##`) or rule tables;
   `AGENTS.md` stays strictly under 24,000 bytes; and `AGENTS.md` and `.agents/rules-index.md` have identical numbered
   rule-domain headings and identical rule-row counts in every domain table.
6. Every role adapter has at most 20 lines and no markdown task-list checkbox (`- [ ]` or `- [x]`).
7. Every canonical skill under `.agents/skills/<skill>/SKILL.md` has its matching Claude skill adapter under
   `.claude/skills/<cmd>/SKILL.md` (`preflight` for `semengine-preflight`, same name otherwise) that links to
   `../../../.agents/skills/<skill>/SKILL.md`, says to read it `fully`, has at most 25 lines, and carries no checkbox.
8. Every role adapter's `model` and `model_reasoning_effort` match D1's table.

`TestAgentSetupParitySensitivity` seeds a violation for each of Steps 1–8 in a temporary directory tree and verifies
that `agentSetupViolations` reports the expected violation.

### D4 Generated checks

Examples and seeded mutations suffice (`docs/testing.md`, "Decide whether generated checks are needed"): both checks
inspect a fixed schema of tracked configuration files and a finite three-agent pairing table.
