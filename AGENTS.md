# SemEngine

Guidance for coding agents. Codex and Antigravity (Gemini) load this file as `AGENTS.md`; `CLAUDE.md` imports it, so
there is one copy (kept well under 24,000 bytes so rule loaders with a 24 KB per-file cap never truncate it). It
carries the facts an agent cannot derive from the tree and names where each rule lives. Per-platform command names and
model routing: `.agents/README.md`. Full test-by-test enforcement audit matrix: `.agents/rules-index.md`.

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
scoping work. The Go code in the tree is the test harness under `internal/harness` and the first ported packages
(`pkg/`, `internal/resource`, `internal/timestamp`, `vocabulary`; `docs/admission-ledger.yaml` lists each); do not
describe planned code as present.

## Commands

`task --list` shows every command with its rationale.

```bash
task doctor       # tool versions and Docker reachability; starts no workloads
task fmt          # format Go sources (the only command that writes)
task fmt:check    # fail on formatting drift
task spec:check   # strict OpenSpec validation; fails on a Hold: outside every task
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
task mutate:check # -- -pkg -test -file -mutant -expect/-expect-text: run one test against one mutant outside the repo
task merge:check  # -- <n>: fail on open flakes PR n leaves open, missing up-to-date rule, or missing review lines
task verify       # spec:check docs:check fmt:check tidy:check cleanup-roots:check build vet lint vuln
                  # ledger:check test:unit test:integration cover:check test:repeat; fails if tracked files changed
```

Run `task verify` before every implementation push. CI has three jobs: `verify` runs the same commands;
`merge-check` runs `scripts/merge-check.sh` (`task merge:check`: known flakes, up-to-date rule, cross-agent review
lines); `required` fails if either failed, is missing, or was skipped/cancelled. Gate selection per diff:
`.agents/skills/semengine-preflight/SKILL.md`.

## Rules and what enforces them

Domain-grouped working reference. `.agents/rules-index.md` carries the full 3-column enforcement audit matrix (exact
contract sections, spec requirements, test names, and review-only coverage gaps). Short names in "Where": `arch`,
`dev`, `rev`, `writer` -> `.agents/contracts/semengine-<role>.md`; `protocol` -> `.agents/protocol.md`; `<name> spec` ->
`openspec/specs/<name>/spec.md`. In "Gate": `review` means review only (no command fails when broken); `verify` means
enforced by `task verify` (`verify+review` when structural guards cover the core rule and edge cases remain review-only).
A new repo-wide or agent-conduct rule updates both this file and `.agents/rules-index.md`.

### 1. Go, state, and component lifecycle

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Production structs never retain `context.Context`; never invent roots or nil defaults | `dev`; `harness-boundaries` spec |
| `review` | State travels as an argument, field, or stored entry, never `context.WithValue` / `ctx.Value` (#91) | `dev`, `rev` (`BLOCKING`) |
| `verify+review` | Deployment authority is spelled once (`config.Manager.Start` -> `deps.Platform` -> `FrameworkIdentityFamily.EntityID`); no `Federation\|GlobalID\|EntityIRI` or second `Org`/`Platform` field (#72) | `harness-boundaries` spec; ADR-102, ADR-104 |
| `verify+review` | Background work outside a service uses `Run(ctx)`, joining `Close()`, or `Shutdown(ctx)`: no fixed timeout, nil ctx refused | `background-work` spec; `dev`, `rev` |
| `verify` | No bare `select {}` | `harness-boundaries` spec |
| `verify+review` | Public packages' exported signatures name no `internal/` type | `harness-boundaries` spec |
| `verify` | Production code imports no test library and nothing under `internal/harness` | `harness-boundaries` spec |
| `verify` | `component` reaches no agentic or graph-family package (`graph`, `internal/graphmutation`, `pkg/lifecycle`, `pkg/projection` outside `contract`) | `component-registration` spec |
| `verify+review` | Components register collectors only on the injected `metric.MetricsRegistry`, never Prometheus' global or a self-built registry | `metric-registry` spec |
| `review` | Callers use the collector `metric.RegisterOrGet` returns, never their own candidate | `metric/registry.go`; ledger `metric` |
| `verify+review` | Ported components refuse unknown config keys at construction, naming the key | `component-registration` spec; `dev` |
| `review` | Ported components keep owner-lifecycle state in `internal/lifecycleguard`, not a local copy | `dev`, `rev`; `internal/lifecycleguard` |
| `review` | Consumers call `CreateComponent`, `SealComposition`, and `Snapshots` only via `service.ComponentManager` (#91) | `component/registry.go`; `dev` |
| `verify+review` | Ported components whose failed `Start` cannot clean up report both errors and retain handles for `Stop` | `lifecycle-suite` spec |
| `review` | Every polymorphic payload is published inside a `message.BaseMessage` | `dev`, `rev` |
| `verify+review` | Payload packages export `RegisterPayloads`, component packages `Register`, imports register nothing, only `main` aggregates, and tests decode via `message.NewDecoder` | `component-registration`, `harness-boundaries` specs; `dev`, `rev` |
| `review` | Failure paths fail closed; any skip, drop, or degrade is declared | `dev` |

### 2. Graph, identity, and NATS storage

| Gate | Rule | Where |
| --- | --- | --- |
| `review` | Classified NATS RPC handlers use `RequestClassified` / `RequestWithRetryClassified`, propagating class, code, detail | `dev`, `rev` |
| `verify+review` | Non-evictable buckets have no TTL or binding `MaxBytes`; stream bounds protect capacity, not entity lifecycle | `dev`, `rev` |
| `verify+review` | `graph.ingest.query.*` and `graph.mutation.` subjects are spelled only in `graph/query_verbs.go` and `internal/graphmutation/protocol.go` | `graph-transport-boundary` spec |
| `verify` | The `graph` root package imports no NATS client, directly or transitively | `graph-transport-boundary` spec |
| `verify+review` | Predicates (3 parts), literal entity IDs (6 parts, <=256B), entity patterns/prefixes, and NATS KV keys/filters pass shared validators at entry; sort/dedup before limit; no partial readiness | `dev`, `rev` |
| `verify+review` | Only `graph-ingest` writes domain entities to `ENTITY_STATES`; other code emits `graph.Graphable` or writes its own `graph/kvcatalog` bucket; single-valued predicates replace | `dev`, `rev`; `graph-entity-writes` spec |

### 3. Testing and verification

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Test cleanup never stops, closes, or terminates under an unbounded context | `harness-boundaries` spec |
| `review` | Tests confirm a child process is still running before inspecting it (`ps`, signal, output) | `docs/testing.md` |
| `review` | A test running a `nats.go` connection inside `testing/synctest` runs in its own test process (#156) | `docs/testing.md` |
| `verify` | Tests bind no fixed address or port | `harness-boundaries` spec |
| `verify+review` | No `time.Sleep` in a test file or under `internal/harness/` | `harness-boundaries` spec |
| `verify+review` | No test calls `Skip`/`Skipf`/`SkipNow`; no test file carries a build tag other than `integration` | `harness-boundaries` spec |
| `verify` | Unit tests also run five times at one CPU, without `-race`, shuffled | `merge-gate` spec |
| `verify+review` | One NATS image pin, a digest reference; no broad Docker cleanup; script/Compose Docker names start `semengine-` | `harness-boundaries` spec |
| `verify+review` | Integration tests run under plain `go test -tags integration`; `task test:integration` only pins flags; nothing takes a lock or refuses a run (#205) | `integration-test-runner` spec |
| `verify+review` | natsfixture turns Ryuk off, labels its containers `org.c360studio.semengine.test`, sweeps labelled leftovers older than 30 min; never by name or liveness | `nats-fixture` spec |
| `verify` | Critical packages hold their coverage floor | `.agents/skills/semengine-preflight/SKILL.md` |
| `verify+review` | Tests use an independent oracle and are shown able to fail (structural guards carry paired sensitivity tests) | `docs/testing.md`; `dev`, `rev` |
| `review` | Changes with interacting inputs, a stated law, or order-dependent history record whether generated checks are used or why examples suffice | `docs/testing.md`; `dev`, `rev` |
| `review` | Generated runs record seed, completed checks, and replayable failure; assertions are shown to run; histories check against a reference model | `docs/testing.md`; `dev`, `rev` |
| `review` | Mutation checks report survivors/inconclusive runs as such, never as detections; fuzz seed replay and exploration report apart; PRs record uncovered cases | `docs/testing.md`; `dev` |

### 4. Porting and architecture

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Ported packages have an admission-ledger row; `carry` rows match the SemStreams pin (`source_sha`) | `arch`; `docs/provenance.md`; `docs/admission-ledger.yaml` |
| `review` | New surface needs a repo caller, named consumer caller (`docs/inventory-scope.md`), or framework-API contract line + product test; admitted packages port whole and only dead surface is dropped (#190, #161) | `dev`, `rev`, `arch`; `protocol` |
| `review` | Production code carries no back door (test-only hook/method/option to swap deps, read private state, or skip lifecycle steps); unexported pin test-only code follows Bounded port scope (#186, #190) | `dev`, `rev`, `arch` |
| `review` | A change establishing a reusable primitive lists who should adopt it | `arch` |
| `review` | A porting design cites a pin probe for every statement on how the SemStreams pin behaves | `arch`, `rev` |
| `review` | Added/edited statements on how a library (`nats.go`, `nats-server`, etc.) times, orders, bounds, or fails cite a probe at the pinned version or state it is not promised (#170) | `arch`, `rev`, `dev`, `writer`; ADR-112 |
| `review` | Porting designs list/cite the pin's governing ADRs per carried package, port binding ADRs (or state why not), and name changed/dropped ADR decisions for the owner (#157) | `arch`, `rev` |
| `review` | Designs state observable behaviour and proving test, not lock/wait-group/join mechanics; open findings after three review rounds go to the owner | `arch`; `.agents/README.md` |
| `review` | Inventories list every overlapping open PR (drafts included), and designs state merge order | `arch`, `rev` |
| `review` | A boundary change is checked against the stated purpose | `arch` |
| `review` | Ported packages bring their SemStreams guidance (contract sections and skills) with them | `arch` |
| `review` | Ported packages building `GenericJSON` from known fields get an `adapt` item (registered payload or stated reason) (#9) | `arch`, `rev` |
| `review` | Sister repositories are read-only and inventoried only as scoped | `docs/inventory-scope.md` |
| `review` | A porting PR carries one package or lifecycle owner (<= ~3,000 non-test pin lines, ~1-page design, own OpenSpec change) unless the owner waives by count (#159) | `arch`, `rev` |
| `review` | Before a porting design is written, the shape sweep reads each carried package on the pin copy and records each finding with its required/optional disposition (#159, #9) | `arch`, `rev`; `protocol` |
| `review` | Before a porting design is written, the inventory lists every open issue naming a carried package and the design states whether the PR retires it (`Closes #n`) or it stays under Bounded port scope (#160) | `arch`, `rev`; `protocol` |
| `review` | Porting findings add required work only when tied to a binding requirement, admission/verify gate, owner ruling, accepted design commitment, or needed build/packaging change | `protocol` |

### 5. Workflow, review, and merge gates

| Gate | Rule | Where |
| --- | --- | --- |
| `spec:check+review` | OpenSpec changes and specs are well formed and truthful against code | "Where state lives" below |
| `spec:check+review` | Every OpenSpec task can be ticked in or before the archive commit; a hold is written as `Hold:` inside the unticked task it stops | `protocol`; `spec-queue` spec; `rev` |
| `verify+review` | Steps end when `task verify` passes locally on a clean tree; pushes batch at review/end; CI is checked once before merge; reviewer `PASS` cites commit + local `task verify`, never CI (#171) | `dev`, `rev`; `protocol` |
| `CI required` | A merge needs CI green | `protocol` |
| `merge:check` | No merge while a known flake (`class:flake` issue) is open, unless the PR closes every open one | `protocol`; `merge-gate` spec |
| `review` | Any defect found in repo code (carried, ported, or written here) not fixed in the change is filed as an issue (`status:blocked`, linked without closing keyword while on a PR branch), never routed as a residual or left in comments (#212, #193) | `protocol`; `rev` |
| `review` | A finding needing an unchosen condition (library timing/ordering, OS/network state, mid-op fault injection, or test-only call sequence) is `BLOCKING`/`HIGH` only with a named real-world trigger; else `MEDIUM` at most (#185) | `rev`, `dev`; `protocol` |
| `review` | A new rule names what enforces it and adds its row to `AGENTS.md` and `.agents/rules-index.md` | `rev` |
| `review` | Before claiming work, mutating state, or spawning role subagents on an underspecified prompt, check any named issue/PR/file first, then confirm Deliverable, Scope/non-goals, Definition of Done/verification, and Stop boundary; when ambiguous, propose a 3–4 bullet Definition of Done and ask 1–3 targeted questions (#224) | `protocol`; `.agents/README.md` |
| `review` | Role-agent briefs carry the owner's intent and the artifact itself, not a paraphrase; resume for continuity, start fresh for mechanics | `.agents/README.md` |
| `verify+review` | Architect, developer, and reviewer run on the frontier model tier (`opus` / `gpt-6-astra` or `gpt-6-sol` / `pro`); architect and developer default to `medium` effort (escalating to `high` for cross-cutting concurrency/boundary work or failed-review rounds), reviewer runs at `high` effort, and technical-writer uses `sonnet` / `flash` and `low`/`medium` effort (#224, #225) | `.agents/README.md`; `harness-boundaries` spec |
| `verify+review` | `AGENTS.md` stays under 24,000 bytes, matches `.agents/rules-index.md` in numbered rule domains and row counts (`verify`) with 1-to-1 rule correspondence (`review`), and all agent/skill adapters match their canonical contracts (#224, #225) | `.agents/README.md`; `harness-boundaries` spec |
| `merge:check+review` | Code PRs (any non-`.md`/non-`openspec/` file, or `.claude/agents/*` or `.agents/contracts/*`) require review by an agent that wrote no commits (`implemented-by:` and `reviewed-by:` naming `claude`, `codex`, or `gemini`); docs-only/archive commits take any agent's reviewer (#64, #66, #225) | `protocol`; `rev`; `merge-gate` spec |
| `review` | Pushed claim branches update by merging `origin/main`, never by rebase or force-push | `protocol` |
| `review` | Evidence cited in a PR or task can be opened on another machine; git-ignored paths are named as local only | `.agents/skills/semengine-handoff`, `semengine-preflight` |
| `review` | Docs are written for a working developer: coined terms defined at first use, no jargon, no marketing | `writer`, `rev` |
| `review` | Developer notes on task choices not spelled out in the design go in `openspec/changes/<id>/notes/<task>.md`, linked from a PR comment (#159) | `dev`, `rev`; `protocol` |

## Where state lives

- **GitHub issues:** what is wanted and decided; rulings as issue comments; labels `type:epic`,
  `status:needs-decision`, `status:blocked`.
- **Milestones:** what gates a release; one for setup, one per provisional tier release.
- **Draft PRs:** claims; `Closes #n` for a leaf issue, `Addresses #n` for an epic.
- **`openspec/changes/<id>/`:** target state (proposal, design, tasks, holds), archived on completion.
- **`openspec/specs/<capability>/`:** current truth, verified against code before it is written.
- **ADRs:** why; irreversible choices and cross-repo contracts.

There is no `/tickets` state and no handoff document. Non-trivial work starts with an OpenSpec change before code.

## Working here (Claude, Codex, and Gemini)

The shared protocol is `.agents/protocol.md`. Read it before filing, taking, landing, or closing work. Four gates
never become a pointer:

- **Clarify:** before claiming work, mutating state, or spawning role subagents, read any named issue, PR, or file so
  you never ask what the repository already answers, then verify four boundaries are clear: **Deliverable** (answer,
  issue, OpenSpec design, docs-only PR, or code PR), **Scope/non-goals**, **Definition of Done/verification**, and
  **Stop boundary**. When a prompt leaves any of the four ambiguous, stop first, propose a concise 3–4 bullet
  recommended Definition of Done, and ask at most 1–3 targeted questions; once confirmed, run autonomously to that
  finish line without pausing on routine steps.
- **Claim:** a draft PR opened before the work, in its own worktree on an agent-prefixed branch
  (`git worktree add ../semengine-wt/claude/<topic> -b claude/<topic> origin/main`; Codex uses `codex/`, Gemini uses
  `gemini/`, and Enjoy desktop sessions use `move_to_worktree` under `.worktrees/<branch>`). No draft PR, no claim.
- **Merge:** CI green on a head up to date with `main`; no known flake open (an open `class:flake` issue) unless the PR
  closes every open one; `task merge:check -- <n>` immediately before merging (`GITHUB_ACTIONS` unset);
  `implemented-by:` in the PR body (`claude`, `codex`, or `gemini` beside the model/persona); archive/spec sync as the
  last content commit; squash merge. A code PR (any changed file not Markdown or under `openspec/`, or a file under
  `.claude/agents/` or `.agents/contracts/`) also needs another agent's review and `reviewed-by:` in the PR body,
  without which `merge-check` fails once non-draft; a documents-only PR does not.
- **Close:** the squash merge of a PR that declared `Closes #n` is the authorization. A close with no merged PR behind
  it takes the owner's word on the issue.

Agents mutate only this repository. SemStreams and other sister repositories are read-only inventory.

## Roles

Role agents are the default path for nontrivial work. Contracts: `.agents/contracts/`.

- `semengine-architect` designs, inventory first; read-only.
- `semengine-developer` implements: failing test, implementation, evidence.
- `semengine-reviewer` reviews every nontrivial change independently before integration; read-only. For a code
  pull request the review of record is another agent's (`.agents/protocol.md`, "Cross-agent review").
- `semengine-technical-writer` updates current docs and task truth with the approved code.

Binding rulings stay with the owner, on the issue. Provenance and license rules for ported code: `docs/provenance.md`.
