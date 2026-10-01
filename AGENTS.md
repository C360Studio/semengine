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
task lint         # pinned revive
task vuln         # pinned govulncheck
task test:unit    # unit tests
task verify       # spec:check docs:check fmt:check tidy:check build vet lint vuln test:unit, cheapest first;
                  # fails if tracked files changed. Not included: doctor, fmt, spec:queue
```

Run `task verify` before every implementation push; CI runs the same commands in two jobs, `verify` and `required`.
Gate selection per diff: `.agents/skills/semengine-preflight/SKILL.md`.

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
