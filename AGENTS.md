# SemEngine

Guidance for coding agents. Codex loads this file as `AGENTS.md`; `CLAUDE.md` imports it, so there is one copy. It
carries the facts an agent cannot derive from the tree and names where each rule lives. Per-platform command names:
`.agents/README.md`.

## What this is for

SemEngine is a Go graph framework whose first consumer is SemSource. It owns primitives and contracts, never a
consumer's domain semantics. It is being built by extracting admitted packages from SemStreams at a frozen pin. Read
`docs/setup-plan.md` (the approved plan) and `docs/repository-map.md` (what exists today) before scoping work. Nothing
in this tree is a Go package yet; do not describe planned code as present.

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

- **Claim:** a draft PR opened before the work, in its own worktree
  (`git worktree add ../semengine-wt/<branch> -b <branch> origin/main`). No draft PR, no claim.
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
