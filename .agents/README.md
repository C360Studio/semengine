# SemEngine Agent Profiles

The files in `contracts/` are the tracked, platform-neutral behavioral authority. Platform adapters are intentionally
thin and must point to exactly one canonical contract. Adapted from SemStreams at commit
`5457b3458936f668b71d2fea061f67f8d7d01e67`; where a rule differs, this tree is authoritative.

## Role and platform mapping

- SemEngine architect
  - Canonical: `.agents/contracts/semengine-architect.md`
  - Claude: `.claude/agents/semengine-architect.md`
  - Codex: `.codex/agents/semengine-architect.toml`
- SemEngine developer
  - Canonical: `.agents/contracts/semengine-developer.md`
  - Claude: `.claude/agents/semengine-developer.md`
  - Codex: `.codex/agents/semengine-developer.toml`
- SemEngine reviewer (independent; read-only)
  - Canonical: `.agents/contracts/semengine-reviewer.md`
  - Claude: `.claude/agents/semengine-reviewer.md`
  - Codex: `.codex/agents/semengine-reviewer.toml`
- SemEngine technical writer
  - Canonical: `.agents/contracts/semengine-technical-writer.md`
  - Claude: `.claude/agents/semengine-technical-writer.md`
  - Codex: `.codex/agents/semengine-technical-writer.toml`

SemStreams also has explorer and judge contracts. They are deliberately left out for now: the plan names four roles,
and their cost and independence rules (judge model routing, inventory files written by an explorer) have no SemEngine
work to justify them yet. Add one only with a recorded owner decision.

## Model routing

Adapters carry the model choices SemStreams uses for the equivalent role. The reviewer is never weaker than the
developer.

| Role | Claude | Codex |
| --- | --- | --- |
| Architect | inherits (read-only tools) | `gpt-6-astra`, resolved effort inherited |
| Developer | `opus` | `gpt-6-sol`, `high` |
| Reviewer | `opus` | `gpt-6-astra`, `high` |
| Technical writer | `opus` | `gpt-6-sol`, `high` |

SemStreams has no technical-writer adapter; the writer follows the developer's routing because it edits files under
the parent workspace permissions. This is a SemEngine choice, open to owner override.

The table does not rank Claude's models against Codex's; across agents the pairing is the owner's decision. For a
code pull request the reviewer of record is the other agent's reviewer from the table above; a documents-only pull
request keeps this repository's reviewer (owner rulings of 2026-10-02 on issue #64, and this repository's reading of
them: `.agents/protocol.md`, "Cross-agent review").

## Orchestrating role agents

These rules bind the session that spawns and briefs role agents. They come from the SETUP 03B orchestrating
session's own account of where its cost went (2026-10-01), relayed by the owner and recorded on issue #37; its
figures were not re-measured. By that account every review round found something real, so the review loop is not
where to cut, and the avoidable cost was in the briefs.

- **A brief carries the owner's intent, not only the approved document.** State what the product is for
  (`AGENTS.md`, "What this is for") and any owner statement that bears on the question. An architect briefed with the
  plan's admission heuristic alone drew a boundary that left out the rule engine (issue #8), and correcting it took
  another inventory pass and its review.
- **An orchestrator's paraphrase is an unreviewed claim.** Hand an agent the artifact itself (a path, an issue
  comment URL, a `file:line`), not a summary of it. Where a summary is unavoidable, mark it as the orchestrator's so
  the reviewer checks it like any other claim.
- **Resume for continuity, start fresh for mechanics.** A resumed agent re-reads its own transcript on every resume.
  For a mechanical follow-up, a fresh agent with a precise brief costs less.
- **A design gets three review rounds.** Findings still open after the third go to the owner on the issue, with the
  choice stated, instead of a fourth round. This does not cut review short; it moves the choice to the person who
  can make it. PR #48's design took at least eleven rounds before any code (issue #53). The architect contract's
  rules on pin probes, open pull requests and behaviour-not-mechanism remove the causes of most of them.
- **Scope an inventory before it starts.** `docs/inventory-scope.md` says which repositories may be read and for what
  question. An inventory names its question and its repositories first.

## Shared work protocol

`.agents/protocol.md` is the canonical shared work protocol (state homes, rituals, worktree hygiene). `CLAUDE.md` and
`AGENTS.md` point to it and carry the three gates (claim, merge, close) inline; edit the protocol only in
`.agents/protocol.md`.

## Shared skills

Canonical skills live in `skills/`. Read the relevant `SKILL.md` fully; the platform adapters add discovery
metadata and argument handling, not another rule set. Repository protocol and role contracts remain authoritative.

- [semengine-preflight](skills/semengine-preflight/SKILL.md): scope existing verification gates and record their
  evidence. Claude command `/preflight`.
- [semengine-handoff](skills/semengine-handoff/SKILL.md): publish shared state and preserve unfinished work. Claude
  command `/semengine-handoff`.
- [semengine-pickup](skills/semengine-pickup/SKILL.md): reconcile current state and verify worktree ownership. Claude
  command `/semengine-pickup`.

Codex can invoke the canonical names with `$`, or read their paths directly. Claude uses the command names above;
its adapters live in `.claude/skills/<command-name>/SKILL.md`. The `preflight` adapter deliberately maps to the
canonical `semengine-preflight` name. There are no engine-capability helper skills yet: add one only when its
capability is admitted. A private memory reference cannot be required to interpret a shared rule.

## Manual read-only parity smoke

Run this procedure after changing a contract, adapter, or repository routing rule. It only reads tracked files.

1. Confirm all four canonical contracts and all eight agent adapters exist, and that `.agents/protocol.md` exists.
2. Confirm each adapter names exactly its matching `.agents/contracts/...` path and says to read it fully first.
3. Confirm the Claude reviewer and architect tool lists contain `Read`, `Bash`, `Grep`, `Glob`, `Skill`, but not
   `Edit`, `Write`, `Task`, or another delegation tool. `gopls` is reached through `Bash`; do not add an `LSP` tool
   name, because an unresolved tool name makes Claude Code refuse to launch the agent.
4. Confirm the Codex reviewer and architect set `sandbox_mode = "read-only"`; the developer and the technical writer
   have no sandbox override and inherit the parent workspace permissions.
5. Confirm `CLAUDE.md` only imports `AGENTS.md` and carries no rules of its own.
6. Inspect adapter size with `wc -l .claude/agents/semengine-*.md .codex/agents/semengine-*.toml`; adapters should
   remain short and contain no copied checklist.
7. For every skill in the shared-skills list, confirm the canonical file and Claude adapter exist, the adapter points
   to that canonical file and says to read it fully, and it contains no copied checklist.
8. Confirm the Codex model and effort settings match the table above.
9. Validate changed skill frontmatter and relative links. A structurally valid adapter does not prove its
   instructions make the right decisions; walk the applicable scenarios below without private memory.

Use these semantic fixtures when reading the routing text:

- "Implement a nontrivial change" routes first to the SemEngine developer and then the SemEngine reviewer.
- "Review a nontrivial SemEngine change" routes to the SemEngine reviewer in read-only mode.
- "Design an API contract or OpenSpec target" routes to the SemEngine architect (surface inventory first, drafts as
  text); binding rulings and approval stay with the owner session.
- "Update durable docs or reconcile task truth" routes to the SemEngine technical writer.
- "Check an isolated Go idiom" may use a generic Go agent only as a second pass.
- "Prepare a docs-only PR" retains protocol review/CI gates and selects checks for the changed artifacts.
- "The previous revision was green" preserves that evidence as historical, not current-head proof.
- "Continue without private memory" resolves shared state and verifies ownership through pickup/protocol.

The smoke passes only when Claude and Codex resolve the same logical role and canonical contract for every fixture.
