# Repository map

What exists in this repository today and where shared state lives. This is a current-state document: anything not
listed under "Present" does not exist yet. The plan that sequences what comes next is `docs/setup-plan.md`; it is
not a task tracker and not a description of the tree.

## Not yet present

- **No Go packages.** There are no `.go` files; `go.mod` declares the module and its pinned tools only.
- **No test harness.** The helper kit, owned NATS fixtures, and lifecycle tests are SETUP 02 (issue #6).
- **No consumer.** SemSource does not build against SemEngine; no consumer API, registration cut, or contract exists.
  The consumer baseline and contract are SETUP 03A and 03B (issues #7, #8).
- **No admission ledger.** No SemStreams package has been admitted; the ledger is seeded in SETUP 03 and 04.
- **No runtime binary, no release, no tag.**
- **No integration or consumer lanes** in the gate graph; they join `task verify` when their workload exists.
- **No ADRs, no OpenSpec specs, no active OpenSpec changes.** `openspec/` is initialized and empty.

## Present

| Path | What it is |
| --- | --- |
| `docs/setup-plan.md` | The approved plan (merged in PR #1, commit 3ae51c5) |
| `docs/repository-map.md` | This file |
| `docs/provenance.md` | License and provenance requirements for ported code |
| `AGENTS.md`, `CLAUDE.md` | Agent entry point; `CLAUDE.md` imports `AGENTS.md` |
| `.agents/` | The shared protocol, role contracts, and skills: the one home of the rules |
| `.claude/`, `.codex/` | Thin platform adapters that point into `.agents/` |
| `go.mod`, `go.sum`, `revive.toml` | Module `github.com/c360studio/semengine`; `revive` and `govulncheck` as tools |
| `Taskfile.yml`, `scripts/` | The one Task entrypoint for local verification and CI |
| `.github/` | CI workflow (jobs `verify` and `required`) and Dependabot configuration |
| `package.json`, `.nvmrc`, `.task-version` | Pins for the Node-based OpenSpec and markdownlint tools and Task |
| `.markdownlint.yaml`, `.markdownlint-cli2.yaml` | Markdown lint configuration behind `task docs:check` |
| `openspec/` | OpenSpec configuration with empty `specs/` and `changes/` |
| `LICENSE` | MIT, Copyright (c) 2025 C360 |

## Where state lives

There is no `/tickets` state and no handoff document; each question has one home. The rules are in
`.agents/protocol.md`.

| Question | Home |
| --- | --- |
| What is wanted, is it decided | GitHub issues; rulings are issue comments |
| What gates a release | A GitHub milestone: `Setup: foundation through contract`, then one per provisional tier |
| Who has claimed it | A draft PR: `Closes #n` for a leaf issue, `Addresses #n` for an epic |
| Target state, tasks, holds | The OpenSpec change inside that PR; `task spec:queue` reads the holds |
| What is true now | `openspec/specs/` (none yet), verified against code |
| Why | ADRs, or the owner's ruling comment |

## Current GitHub state

- Issues #2, #3, #4 are open owner decisions (ADR-106 relationship, registration cut, capability-level name).
- Issues #5 to #11 are the SETUP epics: #5 Foundation, #6 Isolated harness, #7 Pinned consumer baseline, #8 Contract
  and boundary, #9 Tier 0 graph foundation, #10 Tier 1 lexical retrieval, #11 Tier 2 neural retrieval.
- Milestones: `Setup: foundation through contract` (#2 to #8), `Tier 0: graph foundation (provisional)` (#9),
  `Tier 1: lexical retrieval (provisional)` (#10), `Tier 2: neural retrieval (provisional)` (#11).
- Draft PR #12 claims #5 (`Addresses #5`).
- Owner rulings of 2026-09-30 on the plan are recorded at
  [PR #1 comment 5917717356](https://github.com/C360Studio/semengine/pull/1#issuecomment-5917717356).

This list is a snapshot for orientation. GitHub is authoritative; re-read it before acting.
