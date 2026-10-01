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
task test:unit    # unit tests
task verify       # spec:check docs:check fmt:check tidy:check cleanup-roots:check build vet lint vuln
                  # ledger:check test:unit test:integration cover:check, cheapest first; fails if tracked
                  # files changed. Not included: doctor, fmt, spec:queue
```

Run `task verify` before every implementation push; CI runs the same commands in two jobs, `verify` and `required`.
Gate selection per diff: `.agents/skills/semengine-preflight/SKILL.md`.

## Rules and what enforces them

The linked file is the rule; this table is only its index. "Review only" means no command fails when the rule is
broken. In SemStreams' record those are the rules that drifted: the defect class with a command behind it closed, and
the classes policed by review prose stayed open. A change that adds a repository-wide rule or a rule of agent conduct
(in a contract, the protocol, `.agents/README.md`, or this file) adds its row here; a capability spec's requirements
are indexed by that spec, not here. A change that can turn a "review only" row into a failing command should.

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| Production structs never retain `context.Context` | `.agents/contracts/semengine-developer.md` § Context ownership; `openspec/specs/harness-boundaries/spec.md` | `TestNoRetainedContext` (`task test:unit`) for struct fields; invented roots and nil defaults are review only |
| Test cleanup never stops, closes or terminates under an unbounded context | `harness-boundaries` spec, "Bounded cleanup roots" | `task cleanup-roots:check` for the call and the unbounded root on one line; other unbounded roots are review only |
| Tests bind no fixed address or port | `harness-boundaries` spec, "No fixed addresses in tests" | `TestNoFixedAddressesInTests`; `scripts/lint-test-ports.sh` (`task lint`) |
| Production code imports no test library and nothing under `internal/harness` | `harness-boundaries` spec, "Import graph" | `TestImportGraph` |
| One NATS image pin; Docker cleanup touches only SemEngine-assigned names | `harness-boundaries` spec | `TestOneImagePin`, `TestSemEngineAssignedNames`, `TestNoBroadDockerCleanup` |
| A ported package has an admission-ledger row | `.agents/contracts/semengine-architect.md` § Extraction slices; `docs/admission-ledger.yaml` | `task ledger:check` for the schema of the rows present; that a ported package has a row, and what the row says, are review only |
| Critical packages hold their coverage floor | `.agents/skills/semengine-preflight/SKILL.md` | `task cover:check` |
| OpenSpec changes and specs are well formed | "Where state lives" below | `task spec:check` for document shape; truth against code is review only |
| A merge needs CI green | `.agents/protocol.md` § Work lifecycle | CI job `required`; claim before work and close by merged PR are review only |
| A failure path fails closed; a skip, drop or degrade is declared | developer contract § Guarantee, signal, and revision contracts | review only |
| No new surface without a present consumer; unused surface is left behind when porting | developer contract § Before adding anything new; architect contract § Extraction slices | review only |
| A change that establishes a reusable primitive lists who should adopt it | architect contract § The adoption sweep | review only |
| A boundary change is checked against the stated purpose | architect contract § Intent check | review only |
| A ported package brings its SemStreams guidance (contract sections and skills) with it | architect contract § Extraction slices | review only |
| A new rule names what enforces it and adds its row to this table | reviewer contract § Port-time and pattern review | review only |
| A brief to a role agent carries the owner's intent and the artifact itself, not a paraphrase | `.agents/README.md` § Orchestrating role agents | review only |
| Tests use an independent oracle and are shown able to fail | `docs/testing.md`; developer and reviewer contracts, test fidelity | the structural guards carry paired sensitivity tests (`internal/harness/contract`); elsewhere review only |
| Sister repositories are read-only and inventoried only as scoped | `docs/inventory-scope.md` | review only |

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
- **Merge:** CI green with no known unfixed flake in a required job; `implemented-by: <model or persona>` in the PR
  body; the archive/spec sync is the last content commit; squash merge.
- **Close:** the squash merge of a PR that declared `Closes #n` is the authorization. A close with no merged PR behind
  it takes the owner's word on the issue.

Agents mutate only this repository. SemStreams and other sister repositories are read-only inventory.

## Roles

Role agents are the default path for nontrivial work. Contracts: `.agents/contracts/`.

- `semengine-architect` designs, inventory first; read-only.
- `semengine-developer` implements: failing test, implementation, evidence.
- `semengine-reviewer` reviews every nontrivial change independently before integration; read-only.
- `semengine-technical-writer` updates current docs and task truth with the approved code.

Binding rulings stay with the owner, on the issue. Provenance and license rules for ported code: `docs/provenance.md`.
