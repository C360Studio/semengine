# SemEngine

Guidance for coding agents. Codex loads this file as `AGENTS.md`; `CLAUDE.md` imports it, so there is one copy. It
carries the facts an agent cannot derive from the tree and names where each rule lives. Per-platform command names:
`.agents/README.md`.

## What this is for

SemEngine is a live semantic knowledge-graph framework with durable execution. In the owner's words (2026-10-01):
"TrustGraph and Temporal had a tiny baby — a pragmatic, Go-idiomatic, NATS-based, offline-first and edge-capable
baby." It has two halves. The **live semantic knowledge graph**: ingest, index, query, vocabulary, provenance,
fusion, and a tier ladder that degrades gracefully; tier 0 needs no external provider. **Durable execution**:
workflows that survive restarts, replay, settlement, retries with known outcomes, and the rules that drive them.
A capability is engine-owned when it belongs to either half and runs at tier 0 without an external provider; it is
admitted by owner mandate at a named tier with a named qualifying workload, and consumer need decides order, never
membership (epic #8, Q14). Everything outside both halves is a consumer adapter. SemEngine owns primitives and
contracts, never a consumer's domain semantics.

It is built by extracting admitted packages from SemStreams at the frozen pin `8b99efe9` (epic #7). Starter
consumers: semsource, semconnect, semteams, semboids; semembed and seminstruct are support services.
`docs/inventory-scope.md` says which repositories an agent may read and for what question — read it before any
inventory. Read `docs/setup-plan.md` (the approved plan) and `docs/repository-map.md` (what exists today) before
scoping work. The only Go code in the tree is the test harness under `internal/harness`; do not describe planned
code as present.

## Commands

`task --list` shows every command with its rationale.

```bash
task doctor       # tool versions and Docker reachability; starts no workloads
task fmt          # format Go sources (the only command that writes)
task fmt:check    # fail on formatting drift
task spec:check   # strict OpenSpec validation
task spec:queue   # in-flight OpenSpec changes and their holds
task docs:check   # markdownlint
task tidy:check   # go.mod / go.sum tidy
task build        # build
task vet          # go vet
task lint         # pinned revive, plus the fixed-port guard for tests
task vuln         # pinned govulncheck
task test:unit    # unit tests once, under the race detector, at one CPU
task test:repeat  # unit tests five times at one CPU, without the race detector, shuffled
task ledger:diff  # -- <source_path>...: print how ledger entries differ from the SemStreams pin; fetches it
task merge:check  # -- <n>: fail while a known flake is open that PR n does not close, main's rules lack the
                  # up-to-date setting, or PR n is a code PR, not a draft, whose implemented-by:/reviewed-by:
                  # lines do not name the two agents; reads GitHub
task verify       # spec:check docs:check fmt:check tidy:check cleanup-roots:check build vet lint vuln
                  # ledger:check test:unit test:integration cover:check test:repeat, cheapest first; fails if
                  # tracked files changed. Not included: doctor, fmt, spec:queue, merge:check
```

Run `task verify` before every implementation push. CI has three jobs: `verify` runs the same commands;
`merge-check` runs `scripts/merge-check.sh`, the script behind `task merge:check` (known flakes, the up-to-date rule,
and the cross-agent review lines); `required` fails if either of them failed, is missing, or was skipped or cancelled.
Gate selection per diff: `.agents/skills/semengine-preflight/SKILL.md`.

## Rules and what enforces them

The linked file is the rule; this table is only its index. "Review only" means no command fails when the rule is
broken. In SemStreams' record those are the rules that drifted: the defect class with a command behind it closed, and
the classes policed by review prose stayed open. A change that adds a repository-wide rule or a rule of agent conduct
(in a contract, the protocol, `.agents/README.md`, or this file) adds its row here. A capability spec's requirement
gets a row only when it binds every change in the repository, as the test-text and merge rules do; the rest are
indexed by their spec. A change that can turn a "review only" row into a failing command should.

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| Production structs never retain `context.Context` | `.agents/contracts/semengine-developer.md` § Context ownership; `openspec/specs/harness-boundaries/spec.md` | `TestNoRetainedContext` (`task test:unit`) for struct fields; invented roots and nil defaults are review only |
| Test cleanup never stops, closes or terminates under an unbounded context | `harness-boundaries` spec, "Bounded cleanup roots" | `task cleanup-roots:check` for the call and the unbounded root on one line; other unbounded roots are review only |
| Tests bind no fixed address or port | `harness-boundaries` spec, "No fixed addresses in tests" | `TestNoFixedAddressesInTests`; `scripts/lint-test-ports.sh` (`task lint`) |
| No `time.Sleep` in a test file or in any Go file under `internal/harness/` | `harness-boundaries` spec, "No sleeps in tests" | `TestNoSleepsInTests` (`task test:unit`) for the literal text `time.Sleep`; a renamed import or a wait built from a timer is review only |
| No test calls `Skip`, `Skipf` or `SkipNow`; no test file carries a build tag other than `integration` | `harness-boundaries` spec, "No skipped or hidden tests" | `TestNoSkippedTests` and `TestNoHiddenTests` (`task test:unit`); a skip reached through a helper is review only |
| Unit tests also run five times at one CPU, without the race detector, in shuffled order | `merge-gate` spec, "Varied and repeated unit runs" | `task test:repeat`, the last step of `task verify`; `TestUnitInvocationsPinned` fails if that command line or the step list changes |
| Production code imports no test library and nothing under `internal/harness` | `harness-boundaries` spec, "Import graph" | `TestImportGraph` |
| One NATS image pin; Docker cleanup touches only SemEngine-assigned names | `harness-boundaries` spec | `TestOneImagePin`, `TestSemEngineAssignedNames`, `TestNoBroadDockerCleanup` |
| A ported package has an admission-ledger row; a `carry` row matches the pin (SemStreams at its `source_sha`) | `.agents/contracts/semengine-architect.md` § Extraction slices; `docs/provenance.md` rule 5; `docs/admission-ledger.yaml` | `task ledger:check` (in `task verify`) for the schema of the rows present and, fetching the pin, for each `carry` row's `.go` and `testdata` files; `TestCheckSensitivity`, `TestCommandExitStatus` and `TestLedgerCheckWiring` hold it. That a ported package has a row, its `README.md`, a new sub-package under a carried destination, and `adapt` rows (printed by `task ledger:diff`) are review only |
| Critical packages hold their coverage floor | `.agents/skills/semengine-preflight/SKILL.md` | `task cover:check` |
| OpenSpec changes and specs are well formed | "Where state lives" below | `task spec:check` for document shape; truth against code is review only |
| A merge needs CI green | `.agents/protocol.md` § Work lifecycle | CI job `required`; claim before work and close by merged PR are review only |
| No merge while a known flake (an open `class:flake` issue) is open, unless the PR closes every open one | `.agents/protocol.md` § Work lifecycle, "Known flakes"; `merge-gate` spec, "Known-flake check" | `scripts/merge-check.sh`, run by CI job `merge-check` (which `required` needs) and by `task merge:check -- <n>` before merging; `TestMergeCheckKnownFlake` and `TestCIWorkflowPinned` hold the script and the job wiring. Filing and labelling a flake are review only |
| A failure path fails closed; a skip, drop or degrade is declared | developer contract § Guarantee, signal, and revision contracts | review only |
| No new surface without a present consumer; unused surface is left behind when porting | developer contract § Before adding anything new; architect contract § Extraction slices | review only |
| A change that establishes a reusable primitive lists who should adopt it | architect contract § The adoption sweep | review only |
| A porting design cites a pin probe (this repository's checks run on a copy of the SemStreams pin) for every statement about how the pin behaves | architect contract § Extraction slices; reviewer contract § Pre-owner design review | review only |
| A design states what a caller can observe and the test that proves it, not the lock, wait group or join order; after three review rounds, open findings go to the owner | architect contract § Design discipline; `.agents/README.md` § Orchestrating role agents | review only |
| An inventory lists every open pull request, drafts included, that overlaps the change; the design says which merges first | architect contract § The surface inventory, category 3; reviewer contract § Inventory review | review only; if missed once, the overlap listing (`gh pr list` filtered by path) is the first thing to script |
| A boundary change is checked against the stated purpose | architect contract § Intent check | review only |
| A ported package brings its SemStreams guidance (contract sections and skills) with it | architect contract § Extraction slices | review only |
| A new rule names what enforces it and adds its row to this table | reviewer contract § Port-time and pattern review | review only |
| A brief to a role agent carries the owner's intent and the artifact itself, not a paraphrase; resume an agent for continuity, start a fresh one for mechanics | `.agents/README.md` § Orchestrating role agents | review only |
| A code pull request (any changed file that is not Markdown or under `openspec/`, or that is a role adapter under `.claude/agents/`) is reviewed by the agent that wrote none of its commits; the rule covers pull requests already open; the request and the record are PR comments that name the commit, a merge of `main` that touches none of the pull request's files keeps the record, a disagreement goes to the owner, and the PR body's `implemented-by:` and `reviewed-by:` lines name the two agents with the words `claude` and `codex` | `.agents/protocol.md` § Work lifecycle, "Cross-agent review"; reviewer contract § Purpose and authority; `merge-gate` spec, "Cross-agent review check" | `scripts/merge-check.sh`, run by CI job `merge-check` (which `required` needs) and by `task merge:check -- <n>` before merging, for a code pull request that is not a draft: one `implemented-by:` line naming an agent (not read for a bot's pull request) and one `reviewed-by:` line naming the other; `TestMergeCheckReview` and `TestCIWorkflowPinned` hold the script and the `ready_for_review` trigger. Review only: whether the lines are true (one GitHub login for the owner and both agents); that a review record exists, the commit it read, and the re-review after a later content commit; the record's verdict, scope and content; the request comment; that the owner named the reviewer when both agents wrote commits; a disagreement going to the owner; a pull request that changes the check, which runs its own copy; the moment after marking ready before the new run registers; a code pull request written by hand, which the check does not provide for |
| Every OpenSpec task can be ticked in or before the archive commit; a hold is written as `Hold:` on the unticked task it stops | `.agents/protocol.md`, "Target state, task truth, holds"; reviewer contract § Contract and task-truth review | review only; `task spec:queue`, run in the claim's worktree, displays a hold written this way and fails nothing |
| A pushed claim branch is brought up to date by merging `origin/main`, never by a rebase or a force-push | `.agents/protocol.md` § Work lifecycle, "Land" | review only |
| Evidence cited in a pull request or a task can be opened on another machine; a path git ignores is named as local only | `.agents/skills/semengine-handoff/SKILL.md`, "Reconcile the checkpoint"; `.agents/skills/semengine-preflight/SKILL.md`, "Report evidence and remaining gates" | review only |
| Tests use an independent oracle and are shown able to fail | `docs/testing.md`; developer and reviewer contracts, test fidelity | the structural guards carry paired sensitivity tests (`internal/harness/contract`); elsewhere review only |
| A change with interacting input cases, a stated law, or an order-dependent history records whether it uses generated checks or why examples suffice | `docs/testing.md`, "Decide whether generated checks are needed"; developer and reviewer contracts, test fidelity | review only |
| A generated run records its seed, the checks completed and a replayable failure; each assertion is shown to run; a history is checked against a test-owned reference model | `docs/testing.md`, "Fuzz targets and property-based tests"; developer and reviewer contracts, test fidelity | review only |
| A mutation check reports survivors and inconclusive runs as such, never as detections; fuzz seed replay and exploration are reported apart; the pull request records what was not covered | `docs/testing.md`, "Show that the test can fail" and "What the pull request records"; developer contract § Handoff | review only |
| Sister repositories are read-only and inventoried only as scoped | `docs/inventory-scope.md` | review only |
| Docs are written for a working developer: coined terms defined at first use, no jargon, no marketing | technical-writer contract, rule 9; reviewer contract § Contract and task-truth review | review only |

## Where state lives

- **GitHub issues:** what is wanted and decided; rulings as issue comments; labels `type:epic`,
  `status:needs-decision`, `status:blocked`.
- **Milestones:** what gates a release; one for setup, one per provisional tier release.
- **Draft PRs:** claims; `Closes #n` for a leaf issue, `Addresses #n` for an epic.
- **`openspec/changes/<id>/`:** target state (proposal, design, tasks, holds), archived on completion.
- **`openspec/specs/<capability>/`:** current truth, verified against code before it is written.
- **ADRs:** why; irreversible choices and cross-repo contracts.

There is no `/tickets` state and no handoff document. Non-trivial work starts with an OpenSpec change before code.

## Working here (Claude and Codex)

The shared protocol is `.agents/protocol.md`. Read it before filing, taking, landing, or closing work. Three gates
never become a pointer:

- **Claim:** a draft PR opened before the work, in its own worktree on an agent-prefixed branch
  (`git worktree add ../semengine-wt/claude/<topic> -b claude/<topic> origin/main`; Codex uses `codex/`). No draft
  PR, no claim.
- **Merge:** CI green on a head up to date with `main`; no known flake open (an open `class:flake` issue: a test or
  check that passes and fails on the same tree, never a network fetch that did not answer) unless the PR closes every
  open one, and nothing else gets past it; `task merge:check -- <n>` immediately before merging, with
  `GITHUB_ACTIONS` unset; `implemented-by:` in the PR body, naming `claude` or `codex` beside the model or persona;
  the archive/spec sync is the last content commit; squash merge. A code pull request (any changed file that is not a
  Markdown file or under `openspec/`, or that is a role adapter under `.claude/agents/`) also needs the other agent's
  review recorded on it (Codex reviews what Claude implemented, and the reverse) and `reviewed-by:` in the PR body
  naming that agent, without which `merge-check` fails it once it is not a draft; a documents-only pull request does
  not.
- **Close:** the squash merge of a PR that declared `Closes #n` is the authorization. A close with no merged PR behind
  it takes the owner's word on the issue.

Agents mutate only this repository. SemStreams and other sister repositories are read-only inventory.

## Roles

Role agents are the default path for nontrivial work. Contracts: `.agents/contracts/`.

- `semengine-architect` designs, inventory first; read-only.
- `semengine-developer` implements: failing test, implementation, evidence.
- `semengine-reviewer` reviews every nontrivial change independently before integration; read-only. For a code
  pull request the review of record is the other agent's (`.agents/protocol.md`, "Cross-agent review").
- `semengine-technical-writer` updates current docs and task truth with the approved code.

Binding rulings stay with the owner, on the issue. Provenance and license rules for ported code: `docs/provenance.md`.
