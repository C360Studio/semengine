# Repository map

What exists in this repository today and where shared state lives. This is a current-state document: anything not
listed under "Present" does not exist yet. The plan that sequences what comes next is `docs/setup-plan.md`; it is
not a task tracker and not a description of the tree.

## Not yet present

- **No product Go packages.** The only Go code is the test harness under `internal/harness/`.
- **No consumer.** SemSource does not build against SemEngine. The contract that slice 04A implements is written down
  (`docs/contract.md`) and approved in SETUP 03B (issue #8), but no code implements it yet.
- **No ported SemStreams package.** SETUP 02 adapted patterns and carried two scripts unchanged (the port
  guard `scripts/lint-test-ports.sh` and its fixture test); change `flake-defense` has since adapted those two
  scripts. Package rows are seeded in the admission ledger during SETUP 03 and 04.
- **No runtime binary, no release, no tag.**
- **No consumer lane** in the gate graph; it joins `task verify` when its workload exists.
- **No ADRs and no active OpenSpec change.** SETUP 02, SETUP 03B and the Slice 04A design are archived; SETUP 02's
  four capability specs are the current truth under `openspec/specs/`, and `docs/contract.md` states the tier-0
  contract 03B approved. The Slice 04A design names seven implementation changes (`setup-04a-01-floor` to
  `setup-04a-07-substrate-seam`); none is open yet.

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
| `scripts/merge-check.sh` | `task merge:check` and the CI job `merge-check`: fails while an open `class:flake` issue is not closed by the pull request, or while the rules on `main` do not require an up-to-date head. Reads GitHub, so not part of `task verify` |
| `.nats-image` | The pinned NATS image digest every fixture starts |
| `.evidence/` (ignored) | Per-run integration evidence; CI uploads it as an artifact |
| `.github/` | CI workflow (jobs `verify`, `merge-check` and `required`) and Dependabot configuration |
| `package.json`, `.nvmrc`, `.task-version` | Pins for the Node-based OpenSpec and markdownlint tools and Task |
| `.markdownlint.yaml`, `.markdownlint-cli2.yaml` | Markdown lint configuration behind `task docs:check` |
| `openspec/specs/{integration-test-runner,nats-fixture,lifecycle-suite,harness-boundaries}/` | Current truth, synced by the SETUP 02 archive |
| `openspec/changes/archive/2026-09-30-setup-02-isolated-harness/` | The archived SETUP 02 change (PR #13, epic #6) |
| `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/` | The archived SETUP 03B change (PR #21, epic #8), with its three inventory passes (`inventory.md`, `inventory-2-scope.md`, `inventory-3-pass3.md`) |
| `openspec/changes/archive/2026-10-01-setup-04a-foundation/` | The archived Slice 04A design (PR #47, epic #9): the inventory, the seven-change cut (`design.md` D2), the harness extension for change 1 (D3–D5) and the owner's rulings |
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
- PR #21 (SETUP 03B) merged as `9286055` and #8 is closed; its rulings are on #8. The approved boundary it records:
  tier 0 is 65 packages / 140,842 non-test lines at the pin (`design.md` D4); the
  critical coverage list is `design.md` D10 (task `cover:check` scope as each package is ported); the twelve port
  refactors are issues #25–#36 (label `class:port-refactor`, milestone Slice 04A).
- The Slice 04A design (PR #47) is accepted: owner rulings of 2026-10-01 on #9 fix the seven-change chain, the
  rule that each package lands with its tests, ledger rows and repair proofs green, and #24's re-scope to the pin's
  composition points. The first implementation change is `setup-04a-01-floor` (16 packages, the transport and
  message floor plus the harness extension), claimed on its own draft PR when it starts.
- Owner rulings of 2026-09-30 on the plan are recorded at
  [PR #1 comment 5917717356](https://github.com/C360Studio/semengine/pull/1#issuecomment-5917717356).

This list is a snapshot for orientation. GitHub is authoritative; re-read it before acting.
