---
name: semengine-status
description: >-
  Report SemEngine package port progress, active PR claims, known flakes, and GitHub
  milestone/issue queue hygiene, and sync the GitHub Project v2 tracker. Use when asked for port
  status, auditing the issue queue, or updating the project board.
---

# Check and sync SemEngine port status

Read [the shared protocol](../../protocol.md) first. `docs/admission-ledger.yaml` is the source of truth for package
admission against the SemStreams pin (`8b99efe9`), and GitHub milestones (`v0.1.0-alpha.7`, `v0.1.0-alpha.8`,
`v0.1.0-alpha.9`, `v0.1.0-alpha.10`, `tier-2`, `backlog`), issues, and draft PRs hold live project state.

## Run the status scoreboard and hygiene check

From the repository or worktree root, run:

```bash
task status                 # full local ledger scoreboard + live GitHub milestones, PRs, flakes, and issue hygiene
task status -- --offline    # local 65-package Tier-0 + service 7-PR scoreboard only (no network)
task status -- --check      # fail if any milestone title has whitespace or any open issue lacks a milestone/label
```

## Milestone and label rules for every issue and PR

Every open issue MUST carry:

1. **A slug milestone** (no spaces, colons, or characters that break unquoted GitHub search):
   - `v0.1.0-alpha.7` — in-flight pre-release pin (`pkg/fusion` merged NL search).
   - `v0.1.0-alpha.8` — hardening, consumer boot contract, and Tier-1 qualification.
   - `v0.1.0-alpha.9` — rule core (`processor/rule`, `processor/rule/expression`) and deadline timers.
   - `v0.1.0-alpha.10` — remaining Tier-0 substrate tail (`spatial`, `temporal`, `geojson`, `maxdelivery`,
     `composition/cli`).
   - `tier-2` — separately admitted LLM-backed features (empty slot at MVP).
   - `backlog` — unscheduled `class:port-refactor` cleanups, adoption sweeps, and blocked decision epics.
2. **At least one classification label:**
   - `bug` for defects in carried, ported, or harness code;
   - `class:port-refactor` for structural adaptations away from the pin shape;
   - `class:flake` for a test or check that passes and fails on the same tree (blocks `task merge:check`);
   - `enhancement` or `documentation` for tooling, ADR carry-ins, or doc/comment updates;
   - `type:epic`, `status:blocked`, or `status:needs-decision` where applicable per `.agents/protocol.md`.

When `task status -- --check` reports a violation, inspect the issue body and assign the milestone and label with:

```bash
gh issue edit <n> --milestone "<v0.1.0-alpha.7|v0.1.0-alpha.8|v0.1.0-alpha.9|v0.1.0-alpha.10|tier-2|backlog>" --add-label "<label>"
```

## Sync the GitHub Project v2 board

To add any newly opened issues or pull requests to the organization's `SemEngine Port Tracker` GitHub Project v2 board:

```bash
task status -- --sync-project
```

If `gh` reports a missing `project` scope, ask the user to run `gh auth refresh -s project` in their terminal once, then
re-run `task status -- --sync-project`.
