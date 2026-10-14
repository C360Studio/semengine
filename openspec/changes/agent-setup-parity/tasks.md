# Tasks: agent-setup-parity

Each task names the outcome and the gate that proves it.

## 1. Design

- [x] 1.1 Clarify scope and owner decisions for #225 (stop boundary at draft PR ready for cross-agent review, classify
  `.agents/contracts/*` as code files in `scripts/merge-check.sh`, and pin `model: sonnet` on
  `.claude/agents/semengine-technical-writer.md`), and scaffold `proposal.md`, `design.md`, `tasks.md`, and the
  `merge-gate` and `harness-boundaries` spec deltas. Gate: `task spec:check`.

## 2. Implementation and tests

- [x] 2.1 Pin `model` and `model_reasoning_effort` in `.claude/agents/*.md` and `.codex/agents/*.toml`, ignore
  `.worktrees/` and `.enjoy-logs/` in `.gitignore`, and write `TestAgentSetupParity` and
  `TestAgentSetupParitySensitivity` in `internal/harness/contract/agentsetup_test.go`. Gate: `task test:unit`.
- [x] 2.2 Extend `scripts/merge-check.sh` and `internal/harness/contract/mergecheck_test.go` (`TestMergeCheckReview`)
  to classify `.agents/contracts/*` as code names and accept `claude`, `codex`, and `gemini` in `implemented-by:` and
  `reviewed-by:`. Gate: `task test:unit`.
- [x] 2.3 Update `.agents/README.md`, `.agents/protocol.md`, `.agents/rules-index.md`, `AGENTS.md`, and `Taskfile.yml`
  to reflect the pinned adapter settings, three-agent `merge-check.sh` enforcement, and `TestAgentSetupParity`
  enforcement rows, then run `task verify` on a clean tree. Gate: `task verify`.

## 3. Review and close

- [ ] 3.1 Cross-agent review of record on the implementation commit; record `reviewed-by:` in the PR body once the
  reviewing agent's newest review record approves.
  Hold: cross-agent review of record.
- [ ] 3.2 Archive the change and sync `openspec/specs/merge-gate/spec.md` and
  `openspec/specs/harness-boundaries/spec.md` as the last content commit. Gate: `task spec:check`.
