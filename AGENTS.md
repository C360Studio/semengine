# SemEngine

Guidance for coding agents. Codex and Antigravity (Gemini) load this file as `AGENTS.md`; `CLAUDE.md` imports it, so
there is one copy (under 24,000 bytes). It carries facts an agent cannot derive from the tree and names where each
rule lives. Per-platform command names, model routing: `.agents/README.md`.

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
consumers: semsource, semconnect, semteams, semboids; semembed, seminstruct are support services.
Before any inventory read `docs/inventory-scope.md` (which repositories to read, for what question); before scoping
work read `docs/setup-plan.md` (the approved plan) and `docs/repository-map.md` (what exists today).
The Go code is the test harness under `internal/harness` and the first ported packages (`pkg/`, `internal/resource`,
`internal/timestamp`, `vocabulary`; `docs/admission-ledger.yaml` lists each). Do not describe planned code as present.

## Commands

`task --list` shows every command with its rationale.

```bash
task doctor       # tool versions, Docker reachable
task ateam:doctor # agent setup, AGENTS.md size, rule-row parity
task fmt          # format Go (the only command that writes)
task fmt:check    # fail on format drift
task spec:check   # strict OpenSpec validation
task spec:queue   # in-flight changes, holds
task status       # port scoreboard, PRs, flakes
task docs:check   # markdownlint
task tidy:check   # go.mod / go.sum tidy
task build        # build
task vet          # go vet
task lint         # pinned revive, fixed-port guard
task vuln         # pinned govulncheck
task test:unit    # unit tests once, race detector, one CPU
task test:repeat  # unit tests but the contract guards, five times, shuffled
task test:tools   # tests of scripts, Taskfile, CI
task ledger:diff  # -- <source_path>...: ledger vs pin
task mutate:check # one test against one mutant
task merge:check  # -- <n>: review gate for PR n
task verify       # every gate, cheapest first
```

Run `task verify` before every implementation push. CI: `ci.yml` runs `verify` and `Required` fails unless it
passed; `merge-check.yml` runs `scripts/merge-check.sh` (`task merge:check`) on the base commit as `Merge check`.
Gate selection per diff: `preflight` skill. Session checkpoints: ateam's `pickup` and `handoff`.

## Rules and what enforces them

Working reference; `.agents/rules-index.md` is the full matrix. "Where" short names: `arch`,
`dev`, `rev`, `writer` -> `.agents/contracts/semengine-<role>.md`; `protocol` -> `.agents/protocol.md`; `<name> spec` ->
`openspec/specs/<name>/spec.md`. "Gate": `review` means review only (no command fails when broken); `verify` means
enforced by `task verify` (`verify+review`: structural guards cover the core rule, edge cases remain review-only).
A new repo-wide or agent-conduct rule updates this file and `.agents/rules-index.md`.

### 1. Go, state, and component lifecycle

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Production structs never retain `context.Context`; never invent roots (except a `Start`'s one run context, ended in `Stop`, ADR-113) or nil defaults | `dev`; `harness-boundaries` spec |
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
| `review` | A component keeps its lifecycle state in `component.Lifecycle`, a service in `internal/lifecycleguard`, never its own copy | `dev`, `rev`; `component/lifecycle_type.go`, `internal/lifecycleguard` |
| `review` | Consumers call `CreateComponent`, `SealComposition`, `Snapshots` only via `service.ComponentManager` (#91) | `component/registry.go`; `dev` |
| `verify+review` | Ported components whose failed `Start` cannot clean up report both errors and keep handles for `Stop` | `lifecycle-suite` spec |
| `review` | Every polymorphic payload is published inside a `message.BaseMessage` | `dev`, `rev` |
| `verify+review` | Payload packages export `RegisterPayloads`, component packages `Register`; imports register nothing, only `main` aggregates; tests decode via `message.NewDecoder` | `component-registration`, `harness-boundaries` specs; `dev`, `rev` |
| `verify+review` | Every `SchemaRef` type in a service's OpenAPI spec is in its `ResponseTypes` or `RequestBodyTypes`; the composition root registers the spec, never `init` | `dev`, `rev`; `operator-surface` spec |
| `review` | Failure paths fail closed; any skip, drop, or degrade is declared | `dev` |

### 2. Graph, identity, and NATS storage

| Gate | Rule | Where |
| --- | --- | --- |
| `review` | Classified NATS RPC handlers use `RequestClassified` / `RequestWithRetryClassified`, propagating class, code, detail | `dev`, `rev` |
| `verify+review` | Non-evictable buckets have no TTL or binding `MaxBytes`; stream bounds protect capacity, not entity lifecycle | `dev`, `rev` |
| `review` | Large content moves as a `StorageReference`, never a bucket or stream name; `StoreContent` stores only the raw body | `dev`, `rev` |
| `verify+review` | `graph.ingest.query.*`, `graph.index.query.*`, `graph.mutation.` subjects are spelled only in `graph/query_verbs.go` and `internal/graphmutation/protocol.go` | `graph-transport-boundary` spec |
| `verify` | The `graph` root package imports no NATS client, directly or transitively | `graph-transport-boundary` spec |
| `verify+review` | Predicates (3 parts), literal entity IDs (6 parts, <=256B), entity patterns/prefixes, NATS KV keys/filters pass shared validators at entry; sort/dedup before limit; no partial readiness | `dev`, `rev` |
| `verify+review` | Only `graph-ingest` writes domain entities to `ENTITY_STATES`; other code emits `graph.Graphable` or writes its own `graph/kvcatalog` bucket; single-valued predicates replace | `dev`, `rev`; `graph-entity-writes` spec |

### 3. Testing and verification

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Test cleanup never stops, closes, or terminates under an unbounded context | `harness-boundaries` spec |
| `review` | Tests confirm a child process is still running before inspecting it | `docs/testing.md` |
| `review` | A test running a `nats.go` connection inside `testing/synctest` runs in its own test process (#156) | `docs/testing.md` |
| `review` | An assertion on state another goroutine writes waits on the event that orders the read after the write, named in a comment when not the line before; never wall clock alone or an unordered log line (#392) | `docs/testing.md`; `dev`, `rev` |
| `verify` | Tests bind no fixed address or port | `harness-boundaries` spec |
| `verify+review` | A test's port-0 pick is in the family that is bound; an every-address pick says why with `// Every address: <reason>` on the line above; a release check dials the listener's own loopback (#390, #391) | `harness-boundaries` spec; `docs/testing.md`; `rev` |
| `verify+review` | No `time.Sleep` in a test file or under `internal/harness/` | `harness-boundaries` spec |
| `verify+review` | No test calls `Skip`/`Skipf`/`SkipNow`; no test file carries a build tag other than `integration` | `harness-boundaries` spec |
| `verify` | Unit tests (every package but the tool packages and `contract`) also run five times at one CPU, without `-race`, shuffled | `merge-gate` spec |
| `verify+review` | Tool tests run only when a trigger path changes (`task test:tools:changed`), never in `test:repeat`; a test reading any other real-tree file stays in `contract`, which runs once every time | `merge-gate` spec; `internal/harness/toolcheck/doc.go` |
| `verify+review` | One NATS image pin, a digest reference; no broad Docker cleanup; script/Compose Docker names start `semengine-` | `harness-boundaries` spec |
| `verify+review` | Integration tests (only packages that have one) run under plain `go test -tags integration`; `task test:integration` only pins flags; no lock, no refused run (#205) | `integration-test-runner` spec; `docs/test-infrastructure.md` |
| `verify+review` | natsfixture turns Ryuk off, labels containers `org.c360studio.semengine.test`, sweeps labelled leftovers over 30 min old; never by name or liveness | `nats-fixture` spec; `docs/test-infrastructure.md` |
| `review` | An integration test reaches its package's shared NATS server only through the reset | `nats-fixture` spec; `docs/test-infrastructure.md` |
| `verify+review` | A test starts on a shared server with no stream, consumer, stored byte, or other connection, and closes every connection it opens | `nats-fixture` spec |
| `verify+review` | No `t.Parallel` in an integration-tagged Go file | `harness-boundaries` spec |
| `verify+review` | A test that starts its own NATS server says why, with `// Own server: <reason>` on the line above `natsfixture.New` | `harness-boundaries` spec; `docs/test-infrastructure.md` |
| `review` | Harness code exists only for a failure on today's tree that testcontainers, `go test`, `t.Cleanup` or Task does not handle; its doc comment names failure and issue; removed once retired | `docs/test-infrastructure.md` |
| `verify` | Critical packages hold their coverage floor | `.agents/skills/preflight/SKILL.md` |
| `verify+review` | Tests use an independent oracle and are shown able to fail | `docs/testing.md`; `dev`, `rev` |
| `review` | Changes with interacting inputs, a stated law, or order-dependent history record whether generated checks are used or why examples suffice | `docs/testing.md`; `dev`, `rev` |
| `review` | Generated runs record seed, completed checks, replayable failure; assertions are shown to run; histories check against a reference model | `docs/testing.md`; `dev`, `rev` |
| `review` | Mutation checks report survivors/inconclusive runs as such, never detections; fuzz seed replay and exploration apart; PRs record uncovered cases | `docs/testing.md`; `dev` |
| `verify+review` | Production code creates JetStream consumers only via natsclient's policy entry points, called only from allowlisted files (#422) | `harness-boundaries` spec |

### 4. Porting and architecture

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Ported packages have an admission-ledger row; `carry` rows match the SemStreams pin (`source_sha`) | `arch`; `docs/provenance.md`; `docs/admission-ledger.yaml` |
| `review` | New surface needs a repo caller, a named consumer caller, or a framework-API contract line + product test; admitted packages port whole, only dead surface dropped (#190, #161) | `dev`, `rev`, `arch`; `protocol` |
| `review` | Production code carries no back door (test-only hook, method or option to swap deps, read private state, or skip lifecycle steps); unexported test-only pin code follows Bounded port scope (#186, #190) | `dev`, `rev`, `arch` |
| `review` | A change establishing a reusable primitive lists who should adopt it | `arch` |
| `review` | A porting design cites a pin probe for every statement on how the pin behaves | `arch`, `rev` |
| `review` | Added/edited statements on how a library (`nats.go`, `nats-server`, etc.) times, orders, bounds, or fails cite a probe at the pinned version or say it is not promised (#170) | `arch`, `rev`, `dev`, `writer`; ADR-112 |
| `review` | Porting designs cite the pin's governing ADRs per carried package, port binding ADRs (or say why not), and name changed/dropped ADR decisions for the owner (#157) | `arch`, `rev` |
| `review` | Designs state observable behaviour and proving test, not lock/join mechanics; findings open after three review rounds go to the owner | `arch`; `.agents/README.md` |
| `review` | Inventories list every overlapping open PR (drafts included), and designs state merge order | `arch`, `rev` |
| `review` | A boundary change is checked against the stated purpose | `arch` |
| `review` | Ported packages bring their SemStreams guidance (contract sections and skills) with them | `arch` |
| `review` | Ported packages building `GenericJSON` from known fields get an `adapt` item (registered payload or stated reason) (#9) | `arch`, `rev` |
| `review` | Sister repositories are read-only and inventoried only as scoped | `docs/inventory-scope.md` |
| `review` | A porting PR carries one package or lifecycle owner (<= ~3,000 non-test pin lines, ~1-page design, own OpenSpec change) unless the owner waives by count (#159) | `arch`, `rev` |
| `review` | Before a porting design, the shape sweep reads each carried package on the pin copy and records each finding's required/optional disposition (#159, #9) | `arch`, `rev`; `protocol` |
| `review` | Before a porting design, the inventory lists every open issue naming a carried package; the design says whether the PR retires it (`Closes #n`) or it stays under Bounded port scope (#160) | `arch`, `rev`; `protocol` |
| `review` | Porting findings add required work only when tied to a binding requirement, admission/verify gate, owner ruling, accepted design commitment, or build/packaging need | `protocol` |

### 5. Workflow, review, and merge gates

| Gate | Rule | Where |
| --- | --- | --- |
| `spec:check+review` | OpenSpec changes and specs are well formed and truthful against code | "Where state lives" below |
| `spec:check+review` | Every OpenSpec task can be ticked by the archive commit; a hold is `Hold:` inside the unticked task it stops | `protocol`; `spec-queue` spec; `rev` |
| `review` | Clarify declares the track (`quick` or `design`); `proposal.md` stays one page with `Approved-by:` the owner fills; its first task holds `OPEN-Q` until then | `protocol`; `arch` |
| `review` | After the squash merge the claimer removes its worktree and remote branch (`ateam state prune`); never another agent's, dirty or unpushed | `protocol` |
| `review` | Conclusions/handoffs end with the skipped/residual-risk footer; a deliberate limit is marked `// shortcut: <limit>, <when to upgrade>` | `protocol`; ponytail skill |
| `verify+review` | A step ends at a local clean-tree `task verify` pass; pushes batch at review/end; CI checked once before merge; reviewer `PASS` cites commit + local verify, never CI (#171) | `dev`, `rev`; `protocol` |
| `CI required` | A merge needs `Required` and `Merge check` green | `protocol` |
| `merge:check` | No merge while a known flake (`class:flake` issue) is open, unless the PR closes all of them | `protocol`; `merge-gate` spec |
| `review` | A defect in repo code the change does not fix is filed as an issue (`status:blocked`, linked without a closing keyword on a PR branch), never a residual or comment (#212, #193) | `protocol`; `rev` |
| `review` | A finding needing an unchosen condition (library timing/ordering, OS/network state, fault injection, test-only calls) is `BLOCKING`/`HIGH` only with a named real-world trigger, else `MEDIUM` at most (#185) | `rev`, `dev`; `protocol` |
| `review` | A new rule names what enforces it and adds its row to `AGENTS.md` and `.agents/rules-index.md` | `rev` |
| `review` | Before claiming, mutating state, or spawning role subagents on a vague prompt, read named issues/PRs/files first, then confirm Deliverable, Scope/non-goals, Definition of Done/verification, Stop boundary; if unclear, propose a 3–4 bullet Definition of Done and ask 1–3 questions (#224) | `protocol`; `.agents/README.md` |
| `review` | Role-agent briefs carry the owner's intent and the artifact, not a paraphrase; resume for continuity, fresh for mechanics | `.agents/README.md` |
| `verify+review` | Architect/developer/reviewer: frontier tier, effort `medium`/`medium`/`high` (the first two `high` for cross-cutting concurrency/boundary work or failed reviews); writer: `sonnet`/`flash`, `low`/`medium`; adapters pin both (#224, #225) | `.agents/README.md`; `harness-boundaries` spec |
| `verify+review` | `AGENTS.md` < 24,000 bytes, matching `.agents/rules-index.md` in domains and row counts (`verify`), row for row (`review`); adapters match contracts (#224, #225) | `task ateam:doctor`; `.agents/README.md`; `harness-boundaries` spec |
| `merge:check+review` | Code PRs (any file not `.md` or `openspec/` `.yml`/`.yaml`/`.json`, or in `.claude/agents/`, `.agents/contracts/`) need an approving head record by a not-weaker non-committing agent; docs-only commits: any agent's reviewer (#64, #66, #225) | `protocol`; `rev`; `merge-gate` spec |
| `review` | Pushed claim branches update by merging `origin/main`, never rebase or force-push | `protocol` |
| `review` | Evidence in a PR or task opens on another machine; git-ignored paths named local only | `protocol`; `.agents/skills/preflight` |
| `review` | Docs are for a working developer: coined terms defined at first use, no jargon, no marketing | `writer`, `rev` |
| `review` | Developer notes on task choices the design leaves open go in `openspec/changes/<id>/notes/<task>.md`, linked from a PR comment (#159) | `dev`, `rev`; `protocol` |

## Where state lives

- **GitHub issues:** what is wanted and decided; rulings as comments; labels `type:epic`, `status:needs-decision`,
  `status:blocked`.
- **Milestones:** what gates a release; one for setup, one per provisional tier release.
- **Draft PRs:** claims; `Closes #n` for a leaf issue, `Addresses #n` for an epic.
- **`openspec/changes/<id>/`:** target state (proposal, design, tasks, holds), archived on completion.
- **`openspec/specs/<capability>/`:** current truth, checked against code before it is written.
- **ADRs:** why; irreversible choices and cross-repo contracts.

There is no `/tickets` state and no handoff document. Non-trivial work starts with an OpenSpec change.

## Working here (Claude, Codex, and Gemini)

The shared protocol is `.agents/protocol.md`. Read it before filing, taking, landing, or closing work. Four gates
never become a pointer:

- **Clarify:** before claiming work, mutating state, or spawning role subagents, read any named issue, PR, or file so
  you never ask what the repository answers, then verify four boundaries are clear: **Deliverable** (answer,
  issue, OpenSpec design, docs-only PR, or code PR), **Scope/non-goals**, **Definition of Done/verification**, and
  **Stop boundary**. If any is ambiguous, stop, propose a concise 3–4 bullet Definition of Done, and ask at most 1–3
  targeted questions; once confirmed, run autonomously to that finish line without pausing on routine steps.
- **Claim:** a draft PR opened before the work, in its own worktree on an agent-prefixed branch
  (`git worktree add ../semengine-wt/claude/<topic> -b claude/<topic> origin/main`; Codex `codex/`, Gemini `gemini/`,
  Enjoy desktop sessions `move_to_worktree` under `.worktrees/<branch>`). No draft PR, no claim.
- **Merge:** `Required` and `Merge check` green on a head up to date with `main`; no open `class:flake` issue unless
  the PR closes every open one; `task merge:check -- <n>` just before merging (`GITHUB_ACTIONS` unset);
  `implemented-by: <agent> (<model>, <effort>)` in the PR body; archive/spec sync as the last content commit; squash
  merge. A code PR (any file the gate does not call a document) also needs another agent's approving review record at
  its head and `reviewed-by:` in the PR body, or `Merge check` fails once non-draft.
- **Close:** the squash merge of a PR that declared `Closes #n` is the authorization. A close with no merged PR behind
  it takes the owner's word on the issue.

Agents mutate only this repository. SemStreams and other sister repositories are read-only inventory.

## Roles

Role agents are the default path for nontrivial work; contracts: `.agents/contracts/`.

- `semengine-architect` designs, inventory first; read-only.
- `semengine-developer` implements: failing test, implementation, evidence.
- `semengine-reviewer` reviews every nontrivial change independently before integration; read-only. A code PR's
  review of record is another agent's (`.agents/protocol.md`, "Cross-agent review").
- `semengine-technical-writer` updates current docs and task truth with the approved code.

Binding rulings stay with the owner, on the issue. Provenance and license rules: `docs/provenance.md`.
