# Repository map

What exists in this repository today and where shared state lives. This is a current-state document: anything not
listed under "Present" does not exist yet. The plan that sequences what comes next is `docs/setup-plan.md`; it is
not a task tracker and not a description of the tree.

## Not yet present

- **No product Go packages.** The only Go code is the test harness under `internal/harness/`.
- **No consumer.** SemSource does not build against SemEngine. The contract that slice 04A implements is written down
  (`docs/contract.md`) and approved in SETUP 03B (issue #8), but no code implements it yet.
- **No ported SemStreams package.** SETUP 02 carried two scripts and adapted patterns; package rows are seeded in the
  admission ledger during SETUP 03 and 04.
- **No runtime binary, no release, no tag.**
- **No consumer lane** in the gate graph; it joins `task verify` when its workload exists.
- **No ADRs.** The SETUP 02 change is archived; its four capability specs are the current truth under
  `openspec/specs/`. The one active change is listed under "Present".

## Present

| Path | What it is |
| --- | --- |
| `docs/setup-plan.md` | The approved plan (merged in PR #1, commit 3ae51c5), amended in place for the tier model (#8) |
| `docs/contract.md` | What SemEngine guarantees: the contract slice 04A implements, written for a new developer |
| `docs/repository-map.md` | This file |
| `docs/provenance.md` | License and provenance requirements for ported code |
| `docs/inventory-scope.md` | Which repositories agents may read, for what question; starter consumer set (ruled on #22) |
| `AGENTS.md`, `CLAUDE.md` | Agent entry point; `CLAUDE.md` imports `AGENTS.md` |
| `.agents/` | The shared protocol, role contracts, and skills: the one home of the rules |
| `.claude/`, `.codex/` | Thin platform adapters that point into `.agents/` |
| `go.mod`, `go.sum`, `revive.toml` | Module `github.com/c360studio/semengine`; `revive` and `govulncheck` as tools |
| `Taskfile.yml`, `scripts/` | The one Task entrypoint for local verification and CI |
| `internal/harness/natsfixture/` | Owned NATS fixture: admission-gated start, ordered `Stop`, `Name`, evidence |
| `internal/harness/lifecycletest/` | Owner-first lifecycle checks (`Run`) for stateful components |
| `internal/harness/probe/` | Callback, observed-context, and bounded-polling test probes |
| `internal/harness/contract/` | Repository contract tests T-B1 to T-B7, including `task ledger:check` |
| `internal/harness/runner/` | Tests of the integration runner script |
| `scripts/test-integration.sh` | `task test:integration`: host lock, image preflight, signal forwarding, leak check |
| `scripts/cover-check.sh` | `task cover:check`: 80% statements on `natsfixture`, `lifecycletest`, `probe` |
| `.nats-image` | The pinned NATS image digest every fixture starts |
| `.evidence/` (ignored) | Per-run integration evidence; CI uploads it as an artifact |
| `.github/` | CI workflow (jobs `verify` and `required`) and Dependabot configuration |
| `package.json`, `.nvmrc`, `.task-version` | Pins for the Node-based OpenSpec and markdownlint tools and Task |
| `.markdownlint.yaml`, `.markdownlint-cli2.yaml` | Markdown lint configuration behind `task docs:check` |
| `openspec/specs/{integration-test-runner,nats-fixture,lifecycle-suite,harness-boundaries}/` | Current truth, synced by the SETUP 02 archive |
| `openspec/changes/archive/2026-09-30-setup-02-isolated-harness/` | The archived SETUP 02 change (PR #13, epic #6) |
| `openspec/changes/setup-03b-contract-boundary/` | The one active change: SETUP 03B (draft PR #21, epic #8), with its three inventory passes (`inventory.md`, `inventory-2-scope.md`, `inventory-3-pass3.md`) |
| `docs/admission-ledger.yaml` | The admission ledger: twelve entries at full SemStreams SHAs, checked by `task ledger:check` |
| `LICENSE` | MIT, Copyright (c) 2025 C360 |
| `package-lock.json`, `.gitignore` | npm lockfile for the pinned tools; ignore rules for Go, Node, editors, coverage |

## Where state lives

There is no `/tickets` state and no handoff document; each question has one home. The rules are in
`.agents/protocol.md`.

| Question | Home |
| --- | --- |
| What is wanted, is it decided | GitHub issues; rulings are issue comments |
| What gates a release | A GitHub milestone: `Setup: foundation through contract`, then one per provisional slice |
| Who has claimed it | A draft PR: `Closes #n` for a leaf issue, `Addresses #n` for an epic |
| Target state, tasks, holds | The OpenSpec change inside that PR; `task spec:queue` reads the holds |
| What is true now | `openspec/specs/`, verified against code |
| Why | ADRs, or the owner's ruling comment |

## Current GitHub state

- Issues #2 and #3 (ADR-106 relationship, registration cut) are ruled, as Q2 and Q1 on #8; #4 is closed, ruled "tier".
- Issues #5 to #11 are the SETUP epics: #5 Foundation, #6 Isolated harness, #7 Pinned consumer baseline, #8 Contract
  and boundary, #9 Slice 04A tier-0 graph foundation, #10 Slice 04B tier-0 lexical (BM25 completes tier 0), #11 Slice
  04C tier-1 neural (embedding provider).
- Milestones: `Setup: foundation through contract` (#2 to #8), `Slice 04A: tier-0 graph foundation (provisional)` (#9),
  `Slice 04B: tier-0 lexical — BM25 completes tier 0 (provisional)` (#10), `Slice 04C: tier-1 neural — embedding
  provider (provisional)` (#11).
- PR #12 (SETUP 01) merged as `819c461`; #5 stays open for the Codex-to-Claude pickup demonstration.
- PR #13 (SETUP 02) merged as `34c9dc6` and #6 is closed; its SETUP 02 rulings are on
  [issue #6](https://github.com/C360Studio/semengine/issues/6#issuecomment-5921046663).
- Draft PR #21 claims #8 (`Addresses #8`) on `claude/setup-03b-contract`; its change is committed from `a120165`.
  The approved boundary it records: tier 0 is 65 packages / 140,842 non-test lines at the pin (`design.md` D4); the
  critical coverage list is `design.md` D10 (task `cover:check` scope as each package is ported); the twelve port
  refactors are issues #25–#36 (label `class:port-refactor`, milestone Slice 04A).
- Owner rulings of 2026-09-30 on the plan are recorded at
  [PR #1 comment 5917717356](https://github.com/C360Studio/semengine/pull/1#issuecomment-5917717356).

This list is a snapshot for orientation. GitHub is authoritative; re-read it before acting.
