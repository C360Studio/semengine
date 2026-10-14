# Proposal: agent-setup-parity

## Why

PR #226 (issue #224) established three-agent workflow routing (`claude`, `codex`, `gemini`), separated model tier from
reasoning effort (`medium` default for `semengine-architect` and `semengine-developer`, `high` for `semengine-reviewer`,
and `sonnet`/`flash` with `low`/`medium` effort for `semengine-technical-writer`), and extracted the 3-column rule
enforcement matrix into `.agents/rules-index.md` while capping `AGENTS.md` below 24,000 bytes. Issue #225 tracks the
code and harness changes deferred from that docs-only pull request so the adapter files, the merge gate, and the
contract test suite enforce those rules automatically.

## What Changes

- **Role adapter model and effort pins**:
  - `.claude/agents/semengine-architect.md` pins `model: opus`.
  - `.claude/agents/semengine-technical-writer.md` pins `model: sonnet`.
  - `.codex/agents/semengine-architect.toml`, `semengine-developer.toml`, and `semengine-technical-writer.toml` pin
    `model_reasoning_effort = "medium"`, while `semengine-reviewer.toml` remains `model_reasoning_effort = "high"`.
- **Three-agent cross-agent review check and `.agents/contracts/` code classification**:
  - `scripts/merge-check.sh` classifies `.agents/contracts/*` as code names alongside `.claude/agents/*`.
  - `scripts/merge-check.sh` recognizes `claude`, `codex`, and `gemini` in `implemented-by:` and `reviewed-by:`,
    requiring `reviewed-by:` to name one of the other two agents when a single agent implemented a code pull request.
- **Automated agent setup and rule-index parity check**:
  - `internal/harness/contract/agentsetup_test.go` adds `TestAgentSetupParity` and `TestAgentSetupParitySensitivity`,
    enforcing Steps 1–8 of `.agents/README.md` (canonical contracts and adapters, tool lists and sandbox modes,
    `CLAUDE.md` import-only structure, `len(AGENTS.md) < 24000`, numbered rule-domain heading and per-domain row-count
    parity between `AGENTS.md` and `.agents/rules-index.md` with semantic correspondence of individual rows remaining
    review-only, adapter line limits and no copied checklists, skill adapter parity, and pinned model and effort values).
- **Ignore desktop worktree and process-log directories**:
  - `.gitignore` ignores `.worktrees/` and `.enjoy-logs/`.

## Capabilities

### Modified Capabilities

- `merge-gate`: modifies "Cross-agent review check" to treat `.agents/contracts/` as code names alongside
  `.claude/agents/` and to accept `claude`, `codex`, and `gemini` in `implemented-by:` and `reviewed-by:`.
- `harness-boundaries`: adds "Agent setup and rule-index parity" enforced by `TestAgentSetupParity` and
  `TestAgentSetupParitySensitivity`.

## Impact

- `.claude/agents/*.md`, `.codex/agents/*.toml`, `.gitignore`.
- `scripts/merge-check.sh`, `internal/harness/contract/mergecheck_test.go`,
  `internal/harness/contract/agentsetup_test.go`.
- `.agents/README.md`, `.agents/protocol.md`, `.agents/rules-index.md`, `AGENTS.md`, `Taskfile.yml`.
- Closes #225.
