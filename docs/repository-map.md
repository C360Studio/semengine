# Repository map

What exists in this repository today and where shared state lives. This is a current-state document: anything not
listed under "Present" does not exist yet. The plan that sequences what comes next is `docs/setup-plan.md`; it is
not a task tracker and not a description of the tree.

## Not yet present

- **No consumer.** SemSource does not build against SemEngine. The contract that slice 04A implements is written down
  (`docs/contract.md`) and approved in SETUP 03B (issue #8), but no code implements it yet.
- **No runtime binary, no release, no tag.**
- **No consumer lane** in the gate graph; it joins `task verify` when its workload exists.
- **Six of the seven Slice 04A implementation changes.** SETUP 02, SETUP 03B, the Slice 04A design,
  `flake-defense` and the first implementation change, `setup-04a-01-floor`, are archived; the capability specs under
  `openspec/specs/` are the current truth, and `docs/contract.md` states the tier-0 contract 03B approved. The Slice
  04A design names seven implementation changes (`setup-04a-01-floor` to `setup-04a-07-substrate-seam`);
  `setup-04a-02` to `setup-04a-07` are not started.

## Present

| Path | What it is |
| --- | --- |
| `docs/setup-plan.md` | The approved plan (merged in PR #1, commit 3ae51c5), amended in place for the tier model (#8) |
| `docs/contract.md` | What SemEngine guarantees: the contract slice 04A implements, written for a new developer |
| `docs/testing.md` | How to write and review a test here: what it must tell apart, which level to run it at, how to show it can fail |
| `docs/repository-map.md` | This file |
| `docs/provenance.md` | License and provenance requirements for ported code |
| `docs/inventory-scope.md` | Which repositories agents may read, for what question; starter consumer set (ruled on #22) |
| `docs/adr/` | Architecture decision records. ADR-102 (what each entity-ID position means) and ADR-104 (the minted `platform.id` suffix), ported from SemStreams with `pkg/types`; ADR-110 (the write seam as the pattern for every `ENTITY_STATES` writer), SemEngine's own, numbered after the pin's last record |
| `docs/specs/entity-id-contract.md` | The entity-ID reference contract, ported from SemStreams with `pkg/types`: the requirements for entity identity as the pin states them, not a statement of current code. Each requirement moves into a capability spec under `openspec/specs/` when the code it describes is ported (#72 ruling, 2026-10-03) |
| `docs/concepts/16-federation.md` | How entity-ID positions and the authority check separate sources and deployments; ported from SemStreams with `pkg/types` |
| `AGENTS.md`, `CLAUDE.md` | Agent entry point; `CLAUDE.md` imports `AGENTS.md` |
| `.agents/` | The shared protocol, role contracts, and skills: the one home of the rules |
| `.claude/`, `.codex/` | Thin platform adapters that point into `.agents/` |
| `go.mod`, `go.sum`, `revive.toml` | Module `github.com/c360studio/semengine`; `revive` and `govulncheck` as tools |
| `Taskfile.yml`, `scripts/` | The one Task entrypoint for local verification and CI |
| `internal/harness/natsfixture/` | Owned NATS fixture: admission-gated start, ordered `Stop`, `Name`, evidence |
| `internal/harness/lifecycletest/` | Owner-first lifecycle checks (`Run`) for stateful components |
| `internal/harness/probe/` | Callback, observed-context, and bounded-polling test probes |
| `internal/harness/contract/` | Tests of repository-wide rules: no fixed network addresses, no stored contexts, the deployment-authority spelling, no broad Docker cleanup, Docker names SemStreams' cleanup cannot match, the import graph, one NATS image pin, the admission ledger (`task ledger:check`), no sleeps or skipped or hidden tests; and tests that the cover, cleanup-roots, tree-state and merge-check scripts, the `test:unit` and `test:repeat` commands, and the CI workflow do what the merge gate requires |
| `internal/harness/prochost/` | Runs a helper function from the test binary as a child process in its own process group, so a test can signal, pause, kill and wait for it; used by tests that prove behavior when a process dies |
| `internal/harness/payloadfixture/` | Payload-registry fixtures for tests (an empty registry, one built from chosen registrations, a stub type); the pin's `payloadregistry/testing.go`, moved here because it imports `testing` |
| `internal/harness/semantictest/` | Helpers that build well-formed entity IDs and predicates for test fixtures; the pin's `internal/semantictest` |
| `internal/harness/runner/` | Tests of the integration runner script |
| `internal/harness/pindiff/` | The program behind `task ledger:check` and `task ledger:diff`: compares ledger entries with the pin (SemStreams at each entry's `source_sha`), which it fetches from GitHub when an entry needs it |
| `internal/harness/mutcheck/` | The program behind `task mutate:check`: runs one named test against one wrong change to a Go source file, kept in a copy outside the repository, and reports detection, survivor, invalid or inconclusive (`docs/testing.md`, "Show that the test can fail"). Run by hand, never by `task verify` or CI; writes nothing in the repository |
| `scripts/test-integration.sh` | `task test:integration`: host lock, image preflight, signal forwarding, leak check |
| `scripts/cover-check.sh` | `task cover:check`: 80% statements on each package in its target list: `natsfixture`, `lifecycletest`, `probe`, `message`, `payloadregistry`, `natsclient` |
| `scripts/merge-check.sh` | `task merge:check` and the CI job `merge-check`: fails while an open `class:flake` issue is not closed by the pull request, or while the rules on `main` do not require an up-to-date head, or while a code pull request that is not a draft lacks the `implemented-by:` and `reviewed-by:` lines naming the two agents (`.agents/protocol.md`, "Cross-agent review"). Reads GitHub, so not part of `task verify` |
| `.nats-image` | The pinned NATS image digest every fixture starts |
| `.evidence/` (ignored) | Per-run integration evidence; CI uploads it as an artifact |
| `.github/` | CI workflow (jobs `verify`, `merge-check` and `required`) and Dependabot configuration |
| `package.json`, `.nvmrc`, `.task-version` | Pins for the Node-based OpenSpec and markdownlint tools and Task |
| `.markdownlint.yaml`, `.markdownlint-cli2.yaml` | Markdown lint configuration behind `task docs:check` |
| `openspec/specs/{integration-test-runner,nats-fixture,lifecycle-suite,harness-boundaries}/` | Current truth, synced by the SETUP 02 archive; `lifecycle-suite` and `harness-boundaries` amended by the `flake-defense` archive, and `nats-fixture`, `lifecycle-suite` and `harness-boundaries` by the `setup-04a-01-floor` archive; `integration-test-runner` and `nats-fixture` amended by the `admission-owner-identity` archive, and again by the `lock-identity-locale` archive |
| `openspec/specs/{background-work,process-host,transport-client,metric-registry,message-codec}/` | Current truth for the floor: how background goroutines stop, the helper-process host, `natsclient`, `metric.RegisterOrGet` and the `message` codec, synced by the `setup-04a-01-floor` archive |
| `message/`, `metric/`, `natsclient/`, `payloadregistry/`, `vocabulary/`, `pkg/{errs,platform,projection/contract,retry,security,types}/`, `internal/{cache,resource,timestamp,tlsutil}/` | The 15 SemStreams packages ported by `setup-04a-01-floor`; each has an admission-ledger row naming its pin path and destination |
| `openspec/specs/merge-gate/` | Current truth for the merge gate's flake defenses, synced by the `flake-defense` archive |
| `openspec/specs/mutation-check/` | Current truth for `task mutate:check`: its inputs and refusals, the runs, how reach is judged, the four outcomes and the report, synced by the `mutation-check` archive |
| `openspec/specs/spec-queue/` | Current truth for `task spec:queue` and `task spec:check`'s hold check: which tasks the queue reads, how it labels them and prints holds, what `--check` fails on, and that a `tasks.md` that cannot be read stops both with exit 2; synced by the `spec-queue-holds` archive and amended by the `spec-queue-unreadable-tasks` archive |
| `openspec/changes/archive/2026-09-30-setup-02-isolated-harness/` | The archived SETUP 02 change (PR #13, epic #6) |
| `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/` | The archived SETUP 03B change (PR #21, epic #8), with its three inventory passes (`inventory.md`, `inventory-2-scope.md`, `inventory-3-pass3.md`) |
| `openspec/changes/archive/2026-10-01-setup-04a-foundation/` | The archived Slice 04A design (PR #47, epic #9): the inventory, the seven-change cut (`design.md` D2), the harness extension for change 1 (D3–D5) and the owner's rulings |
| `openspec/changes/archive/2026-10-05-setup-04a-01-floor/` | The archived first implementation change (PR #48, epic #9): the floor's 15 packages, the harness extension, the surface audit (`surface-audit.md`) and their tasks |
| `openspec/changes/archive/2026-10-01-flake-defense/` | The archived `flake-defense` change (PR #44, issue #42): repeated and shuffled unit runs, the known-flake merge check, and the no-sleep and no-skip test rules |
| `openspec/changes/archive/2026-10-06-mutation-check/` | The archived `mutation-check` change (PR #82, issue #79): the inventory, the design of `internal/harness/mutcheck` with what the trial of a prototype showed and the owner's rulings, and its tasks |
| `openspec/changes/archive/2026-10-07-runner-image-pin/` | The archived `runner-image-pin` change (PR #112, issue #61): every CI job pinned to `ubuntu-24.04` and held by `TestCIWorkflowPinned`; the inventory, the design with the owner's rulings and their conformance table, and its tasks |
| `openspec/changes/archive/2026-10-07-spec-queue-holds/` | The archived `spec-queue-holds` change (PR #113, issue #76): the queue reads every line of an open task and `task spec:check` fails on a hold outside every task; the inventory, the design with the owner's rulings and their conformance table, and its tasks |
| `openspec/changes/archive/2026-10-07-spec-queue-unreadable-tasks/` | The archived `spec-queue-unreadable-tasks` change (PR #122, issue #115): the requirement that a `tasks.md` that cannot be read stops the queue and the check with exit 2, and the two tests that hold it; the inventory, the design the owner accepted, and its tasks |
| `openspec/changes/archive/2026-10-07-admission-owner-identity/` | The archived `admission-owner-identity` change (PR #125, issue #124): fixture admission treats the lock owner as live only when `ps` prints the start time the runner recorded, the one rule written in the runner's spec; the inventory, the design with the owner's answers and their conformance table, and its tasks |
| `openspec/changes/archive/2026-10-08-lock-identity-locale/` | The archived `lock-identity-locale` change (PR #127, issue #126). The runner's lock owner record gains `identity_utc`, the start time read under `TZ=UTC LC_ALL=C`, so a runner or a test started in another time zone or locale reads a live owner as live. The record is published whole, renamed into the lock directory, so a contender never reads half of one. SemStreams' runner ignores the new key. Holds the inventory, the design with the owner's answers and its conformance table, and the tasks |
| `docs/admission-ledger.yaml` | The admission ledger: 46 entries at full SemStreams SHAs, checked by `task ledger:check`, which also fails a `carry` entry that differs from the pin; four are `carry` (three ported packages and `internal/semantictest`) |
| `docs/tier1-cross-check.md` | The tier-0 port set (65 packages) compared with SemStreams' Tier 1 sister-import list at the pin: 39 in both, 23 Tier-1-only (each with its disposition), 26 tier-0-only; a record that changes no ledger row |
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
  composition points. The first implementation change is `setup-04a-01-floor` (15 packages, the transport and
  message floor, plus the harness extension), carried on PR #48 and archived there.
- Owner rulings of 2026-09-30 on the plan are recorded at
  [PR #1 comment 5917717356](https://github.com/C360Studio/semengine/pull/1#issuecomment-5917717356).

This list is a snapshot for orientation. GitHub is authoritative; re-read it before acting.
