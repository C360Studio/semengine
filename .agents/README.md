# SemEngine Agent Profiles

The files in `contracts/` are the tracked, platform-neutral behavioral authority. Platform adapters are intentionally
thin and must point to exactly one canonical contract. Adapted from SemStreams at commit
`5457b3458936f668b71d2fea061f67f8d7d01e67`; where a rule differs, this tree is authoritative.

## Role and platform mapping

- SemEngine architect (read-only)
  - Canonical: `.agents/contracts/semengine-architect.md`
  - Claude: `.claude/agents/semengine-architect.md`
  - Codex: `.codex/agents/semengine-architect.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "semengine-architect"`, `enable_write_tools = false`)
- SemEngine developer
  - Canonical: `.agents/contracts/semengine-developer.md`
  - Claude: `.claude/agents/semengine-developer.md`
  - Codex: `.codex/agents/semengine-developer.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "semengine-developer"`, `enable_write_tools = true`)
- SemEngine reviewer (independent; read-only)
  - Canonical: `.agents/contracts/semengine-reviewer.md`
  - Claude: `.claude/agents/semengine-reviewer.md`
  - Codex: `.codex/agents/semengine-reviewer.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "semengine-reviewer"`, `enable_write_tools = false`)
- SemEngine technical writer
  - Canonical: `.agents/contracts/semengine-technical-writer.md`
  - Claude: `.claude/agents/semengine-technical-writer.md`
  - Codex: `.codex/agents/semengine-technical-writer.toml`
  - Gemini (Antigravity): `define_subagent` (`name = "semengine-technical-writer"`, `enable_write_tools = true`)

SemStreams also has explorer and judge contracts. They are deliberately left out for now: the plan names four roles,
and their cost and independence rules (judge model routing, inventory files written by an explorer) have no SemEngine
work to justify them yet. Add one only with a recorded owner decision.

## Model routing

Adapters separate **model tier** from **reasoning effort** (`low -> medium -> high`). Neither the architect nor the
reviewer runs on a weaker model tier than the developer.

| Role | Claude | Codex | Gemini (Antigravity) | Target effort |
| --- | --- | --- | --- | --- |
| Architect | `opus`, `medium` | `gpt-6-astra`, `medium` | `inherit` or `pro` (`enable_write_tools = false`) | `medium` default; escalate to `high` for cross-cutting concurrency/boundary designs |
| Developer | `opus`, `medium` | `gpt-6-sol`, `medium` | `inherit` or `pro` (`enable_write_tools = true`) | `medium` default for bounded work; escalate to `high` for complex/failed-review rounds |
| Reviewer | `opus`, `high` | `gpt-6-astra`, `high` | `inherit` or `pro` (`enable_write_tools = false`) | `high` (single-pass audit of record against review-only rules) |
| Technical writer | `sonnet`, `medium` | `gpt-6-sol`, `medium` | `flash` or `inherit` (`enable_write_tools = true`) | `low` or `medium` (mechanical doc and task-truth sync) |

- **Model tier:** `semengine-architect`, `semengine-developer`, and `semengine-reviewer` run on the frontier model
  (`opus` on Claude, `gpt-6-astra` / `gpt-6-sol` on Codex, `inherit` / `pro` on Gemini). `semengine-technical-writer`
  is non-decision-making (doc sync, `task docs:check`, and conservative task ticking) and runs on a lighter tier where
  supported (`sonnet` on Claude, `flash` on Gemini).
- **Reasoning effort (`low -> medium -> high`):** Start `semengine-architect` and `semengine-developer` at `medium`
  effort by default and escalate to `high` only when a change crosses module boundaries, designs a concurrency or
  durable-execution primitive, or fails a review round. Keep `semengine-reviewer` at `high` effort because it runs once
  per ready diff as the independent audit against the repository's review-only rules. Run `semengine-technical-writer`
  at `low` or `medium` effort. The tracked `.claude/agents/` and `.codex/agents/` adapter files pin these defaults and
  are checked by `TestAgentSetupParity` (`internal/harness/contract/agentsetup_test.go`).

On Gemini (Antigravity), role subagents are registered with `define_subagent` pointing at the matching
`.agents/contracts/semengine-*.md` contract and invoked with `invoke_subagent`; `Model` defaults to `"inherit"` (for
example, Gemini 4 Argon High) unless `"pro"` or `"flash"` is explicitly requested.

The table does not rank one platform's models against another's; across agents the pairing is the owner's decision.
For a code pull request the reviewer of record is another agent's reviewer from the table above; a documents-only
pull request keeps this repository's reviewer (owner rulings of 2026-10-02 on issue #64, and this repository's reading
of them: `.agents/protocol.md`, "Cross-agent review"). `scripts/merge-check.sh` recognizes `claude`, `codex`, and
`gemini` on code pull requests.

## Orchestrating role agents

These rules bind the session that intakes owner requests and spawns/briefs role agents. They come from the SETUP 03B
orchestrating session's own account of where its cost went (2026-10-01), relayed by the owner and recorded on issue #37;
its figures were not re-measured. By that account every review round found something real, so the review loop is not
where to cut, and the avoidable cost was in the briefs and unclarified scope.

- **Clarify Definition of Done before starting or delegating work.** Before mutating state or spawning a role agent on
  an underspecified prompt, do a quick read-only check of any named issue, PR, or file (never asking what the tree
  already answers), then verify four boundaries are clear: (1) **Deliverable** (answer/audit, issue, OpenSpec design,
  docs-only PR, or code PR), (2) **Scope and non-goals** (in-bounds packages/files and what must not change),
  (3) **Definition of Done and verification** (observable completion criteria and `task` gates), and (4) **Stop
  boundary** (pause after inventory/design for an owner decision vs run through to a ready PR). If any of the four is
  missing or ambiguous, stop, propose a concise 3–4 bullet recommended Definition of Done, and ask at most 1–3 targeted
  questions; once confirmed, run autonomously to that finish line without stopping for routine steps.
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

## Shared work protocol and rule index

`.agents/protocol.md` is the canonical shared work protocol (state homes, rituals, worktree hygiene). `CLAUDE.md` and
`AGENTS.md` point to it and carry the four gates (clarify, claim, merge, close) inline; edit the protocol only in
`.agents/protocol.md`. `.agents/rules-index.md` is the full test-by-test enforcement audit matrix backing the
domain-grouped rule summary in `AGENTS.md`.

## Shared skills

Canonical skills live in `skills/`. Read the relevant `SKILL.md` fully; the platform adapters add discovery
metadata and argument handling, not another rule set. Repository protocol and role contracts remain authoritative.

- ateam's six canonical skills, written by `ateam init` at the pin (v0.3.4): [pickup](skills/pickup/SKILL.md),
  [handoff](skills/handoff/SKILL.md), [preflight](skills/preflight/SKILL.md), [ponytail](skills/ponytail/SKILL.md),
  [review-pr](skills/review-pr/SKILL.md) and [design](skills/design/SKILL.md). Claude commands `/<name>`. `preflight`
  keeps SemEngine's gate table and evidence rules; the other five are ateam's text, unedited.
- [semengine-status](skills/semengine-status/SKILL.md): report Slice 04A package port status, audit milestone/issue
  hygiene, and sync the GitHub Project tracker. Claude command `/semengine-status`.

Engine-capability skills, carried from SemStreams with the packages they govern (change
`setup-04a-02-ingest-kernel`, design D12):

- [entity-or-bucket](skills/entity-or-bucket/SKILL.md): decide whether a fact lives in `ENTITY_STATES` or a bucket of
  its own. Claude command `/entity-or-bucket`.
- [kv-or-stream](skills/kv-or-stream/SKILL.md): choose a KV watch or a JetStream stream. Claude command
  `/kv-or-stream`.
- [new-payload](skills/new-payload/SKILL.md): add a payload type to the registry. Claude command `/new-payload`.
- [query-pattern](skills/query-pattern/SKILL.md): choose how code reads the graph. Claude command `/query-pattern`.

Codex can invoke the canonical names with `$`, or read their paths directly. Antigravity (Gemini) automatically
discovers `.agents/skills/<name>/SKILL.md` from the workspace root. Claude uses the command names above; its adapters
live in `.claude/skills/<command-name>/SKILL.md`. Add an engine-capability skill only when its capability is
admitted. A private memory reference cannot be required to interpret a shared rule.

## Read-only parity smoke

`task ateam:doctor` (`ateam doctor --check`, the first step of `task verify`) checks the required files, the
canonical skills' section markers and their adapters' links, hooks, MCP, `CLAUDE.md`, the `AGENTS.md` size, and
rule-row parity with `.agents/rules-index.md`. `TestAteamTemplatesUnchanged` (`internal/harness/toolcheck`,
`task test:tools:changed`) checks that every file `ateam init` writes, but the ones SemEngine writes itself, is
byte-identical to the pinned release. `TestAgentSetupParity` and `TestAgentSetupParitySensitivity`
(`internal/harness/contract/agentsetup_test.go`, `task test:unit`) check what neither does: each role adapter's
contract pointer, tools, sandbox mode, size and model and effort pins against the table above, and, for every skill
under `skills/` that is not one of ateam's six, that its Claude adapter exists, links to it, says to read it fully,
and stays within 25 lines with no checkbox. After changing a contract, adapter, or routing rule, also walk the
fixtures below; a structurally valid adapter does not prove its instructions make the right decisions.

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

The smoke passes only when Claude, Codex, and Gemini resolve the same logical role and canonical contract for every
fixture.
