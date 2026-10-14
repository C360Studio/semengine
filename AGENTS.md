# SemEngine

Guidance for coding agents. Codex and Antigravity (Gemini) load this file as `AGENTS.md`; `CLAUDE.md` imports it, so
there is one copy (kept under 24,000 bytes so rule loaders with a 24 KB per-file cap never truncate it). It carries
the facts an agent cannot derive from the tree and names where each rule lives. Per-platform command names and model
routing: `.agents/README.md`.

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

The linked file is the rule; this table is only its index. Short names in "Canonical home": `arch` (`architect`),
`dev` (`developer`), `rev` (`reviewer`), `writer` (`technical-writer`) -> `.agents/contracts/semengine-<role>.md`;
`protocol` -> `.agents/protocol.md`; `<name> spec` -> `openspec/specs/<name>/spec.md`. "Review only" means no command
fails when broken. A change adding a repo-wide or agent-conduct rule adds its row here; a spec requirement gets a row
only when it binds every change. Turn "review only" rows into failing commands where practical. Keep `AGENTS.md` <24,000B.

| Rule | Canonical home | Enforced by |
| --- | --- | --- |
| Production structs never retain `context.Context` | `dev` § Context ownership; `harness-boundaries` spec | `TestNoRetainedContext`; invented roots/nil defaults: review only |
| State travels as an argument, field, or stored entry, never `context.WithValue` / `ctx.Value` (#91) | `dev` § State ownership; `rev` § Graph and state ownership (`BLOCKING`) | review only |
| Deployment authority is spelled once (`config.Manager.Start` -> `deps.Platform` -> `FrameworkIdentityFamily.EntityID`); no exported `Federation\|GlobalID\|EntityIRI` or second `Org`/`Platform` field (#72) | `harness-boundaries` spec; ADR-102, ADR-104 | `TestNoDeploymentAuthorityNames`, `TestNoSecondAuthorityField` (+ sensitivity); unexported copies, re-joins, ordering, sole composer: review only |
| Background work outside a service uses `Run(ctx)`, joining `Close()`, or `Shutdown(ctx)`: no fixed shutdown timeout, nil ctx refused | `background-work` spec; `dev`, `rev` § Background work | each site's `synctest` test; new goroutines/timers: review only |
| Classified NATS RPC handlers use `RequestClassified` / `RequestWithRetryClassified`, propagating class, code, detail | `dev` § NATS RPC; `rev` § NATS RPC error contract | review only |
| Non-evictable buckets have no TTL or binding `MaxBytes`; stream bounds protect capacity, not entity lifecycle | `dev`, `rev` § Storage and retention | `CheckNoLifecycleRetention`, `AssertNoLifecycleRetention`, `CheckStreamBounds` (`natsclient`); unchosen `DiscardOld`/bypass: review only |
| Public packages' exported signatures name no `internal/` type | `harness-boundaries` spec | `TestPublicSignatures` (+ sensitivity); public-package list (D5): review only |
| Test cleanup never stops, closes, or terminates under an unbounded context | `harness-boundaries` spec | `task cleanup-roots:check` (same-line); other unbounded roots: review only |
| No bare `select {}` | `harness-boundaries` spec | `TestNoBareSelect` (+ sensitivity) |
| Tests confirm a child process is still running before inspecting it (`ps`, signal, output) | `docs/testing.md` § Concurrency and cleanup | review only (`prochost` `Signal`/`Pause`/`Resume` error after exit) |
| A test running a `nats.go` connection inside `testing/synctest` runs in its own test process (#156) | `docs/testing.md` § Concurrency and cleanup | review only |
| Tests bind no fixed address or port | `harness-boundaries` spec | `TestNoFixedAddressesInTests`; `scripts/lint-test-ports.sh` (`task lint`) |
| No `time.Sleep` in a test file or under `internal/harness/` | `harness-boundaries` spec | `TestNoSleepsInTests`; renamed imports/timers: review only |
| No test calls `Skip`/`Skipf`/`SkipNow`; no test file carries a build tag other than `integration` | `harness-boundaries` spec | `TestNoSkippedTests`, `TestNoHiddenTests`; helper skips: review only |
| Unit tests also run five times at one CPU, without `-race`, shuffled | `merge-gate` spec | `task test:repeat` (last in `task verify`), `TestUnitInvocationsPinned` |
| Production code imports no test library and nothing under `internal/harness` | `harness-boundaries` spec | `TestImportGraph` |
| One NATS image pin; Docker cleanup touches only SemEngine-assigned names | `harness-boundaries` spec | `TestOneImagePin`, `TestSemEngineAssignedNames`, `TestNoBroadDockerCleanup`; letter tags (`nats:latest`): review only |
| Components register collectors only on the injected `metric.MetricsRegistry`, never Prometheus' global registry | `metric-registry` spec | `TestNoProcessGlobalRegistration` (+ sensitivity); self-built registries: review only |
| `graph.ingest.query.*` and `graph.mutation.` subjects are spelled only in `graph/query_verbs.go` and `internal/graphmutation/protocol.go` | `graph-transport-boundary` spec | `TestReservedSubjectsDeclaredOnce` (+ sensitivity); assembled strings: review only |
| `component` reaches no agentic or graph-family package (`graph`, `internal/graphmutation`, `pkg/lifecycle`, `pkg/projection` outside `contract`) | `component-registration` spec | `TestComponentReachesNoAgenticOrGraphPackage` (+ sensitivity) |
| The `graph` root package imports no NATS client, directly or transitively | `graph-transport-boundary` spec | `TestGraphImportsNoTransport` (+ sensitivity) |
| Ported components refuse unknown config keys at construction, naming the key | `component-registration` spec; `dev` § State ownership | each component's test (`TestCreateGraphIngestRefusesUnknownKey`); new ports: review only |
| Ported components keep owner-lifecycle state in `internal/lifecycleguard`, not a local copy | `dev` § State ownership; `rev` § Component wiring; `internal/lifecycleguard` | review only |
| Consumers call `CreateComponent`, `SealComposition`, and `Snapshots` only via `service.ComponentManager` (#91 ruling C) | `component/registry.go`; `dev` § State ownership | review only |
| Ported components whose failed `Start` cannot clean up report both errors and retain handles for `Stop` | `lifecycle-suite` spec | each component's `proving_tests` (`TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop`); new ports: review only |
| Every polymorphic payload is published inside a `message.BaseMessage` | `dev`, `rev` § Payload registry | review only |
| Payload packages export `RegisterPayloads`, component packages `Register`, imports register nothing, only `main` aggregates registrations, and round-trip tests decode via `message.NewDecoder` | `component-registration`, `harness-boundaries` specs; `dev`, `rev` § Payload registry; `.agents/skills/new-payload/SKILL.md` | `TestNoAggregatorPackage`, `TestBoundarySensitivity`; missing funcs, import side effects, non-decoder tests: review only |
| Predicates (3 parts), literal entity IDs (6 parts, <=256B), entity patterns/prefixes, and NATS KV keys/filters pass shared validators at entry | `dev`, `rev` § Semantic identity and graph | `TestParsePredicate`, `TestValidateEntityIDContract`/`Pattern`/`Prefix`, `TestValidateKVLiteralKey`/`WildcardFilter`, `TestKVKeyContractNormativeNATS`; entry sites, sort/dedup before limit, readiness: review only |
| Only `graph-ingest` writes domain entities to `ENTITY_STATES`; other code emits `graph.Graphable` or writes its own `graph/kvcatalog` bucket; single-valued predicates replace | `dev` § State ownership; `rev` § Graph and state ownership; `graph-entity-writes` spec | `TestEntityWritesHaveOneSeam` (+ sensitivity), `TestCatalogReader_ExportedSurfaceIsReadOnly`; bypassing `OpenCatalogReader` or appending single-valued predicates: review only |
| Ported packages have an admission-ledger row; `carry` rows match the SemStreams pin (`source_sha`) | `arch` § Extraction slices; `docs/provenance.md` rule 5; `docs/admission-ledger.yaml` | `task ledger:check` (`TestCheckSensitivity`, `TestCommandExitStatus`, `TestLedgerCheckWiring`); row presence, `README.md`, `adapt` rows (`task ledger:diff`): review only |
| Critical packages hold their coverage floor | `.agents/skills/semengine-preflight/SKILL.md` | `task cover:check` |
| OpenSpec changes and specs are well formed | "Where state lives" below | `task spec:check` (shape); truth against code: review only |
| Steps end when `task verify` passes locally on a clean tree; pushes batch at review/end; CI is checked once before merge; reviewer `PASS` cites commit + local `task verify`, never CI (#171) | `dev` § Handoff; `rev` § Verdict format; `protocol` § Cross-agent review | CI job `required` at merge; clean-tree runs, batched pushes, verdict citations: review only |
| A merge needs CI green | `protocol` § Work lifecycle | CI job `required`; claim before work and close by merged PR: review only |
| No merge while a known flake (`class:flake` issue) is open, unless the PR closes every open one | `protocol` § Known flakes; `merge-gate` spec | `scripts/merge-check.sh` (`merge-check` job, `task merge:check -- <n>`), `TestMergeCheckKnownFlake`, `TestCIWorkflowPinned`; filing/labelling: review only |
| A failure path fails closed; a skip, drop, or degrade is declared | `dev` § Guarantee, signal, and revision contracts | review only |
| Any defect found in repo code (carried, ported, or written here) not fixed in the change is filed as an issue (`status:blocked`, linked without closing keyword while on a PR branch), never routed as a residual or left in comments (#212, #193) | `protocol` § File; `rev` § Guarantee, signal, and revision review | review only |
| New surface needs a repo caller, a named consumer caller (`docs/inventory-scope.md`), or a framework-API contract line (with a product test); admitted packages port whole, pin consumer-adapter callers count as readers, and only dead surface is dropped (#190) | `dev` § Before adding anything new, Q3; `rev` § Exported-surface review; `arch` § Surface audit; `protocol` § Bounded port scope | review only until #161's check lands |
| Production code carries no back door (hook/method/option used only by tests to swap deps, read private state, or skip lifecycle steps); unexported test-only code carried unchanged from the pin follows Bounded port scope (#186, #190) | `dev` § Before adding anything new, Q3; `rev` § Exported-surface review; `arch` § The shape sweep, item 4 | review only |
| A caller uses the collector `metric.RegisterOrGet` returns, never its own candidate | `metric/registry.go`; ledger `metric` row | review only |
| A change establishing a reusable primitive lists who should adopt it | `arch` § The adoption sweep | review only |
| A porting design cites a pin probe for every statement on how the SemStreams pin behaves | `arch` § Extraction slices; `rev` § Pre-owner design review | review only |
| Added/edited statements on how a library (`nats.go`, `nats-server`, etc.) times, orders, bounds, or fails cite a probe at the pinned version or state the behaviour is not promised (#170) | `arch` § Design discipline; `rev` § Contract review; `dev` § Guarantee contracts; `writer` rule 10; ADR-112 | review only |
| A finding needing an unchosen condition (library timing/ordering, OS/network state, mid-op fault injection, or test-only call sequence) is `BLOCKING`/`HIGH` only with a named real-world trigger; else `MEDIUM` at most (#185) | `rev` § Verdict format; `dev` § Guarantee contracts; `protocol` § Bounded port scope | review only |
| Porting designs list/cite the pin's governing ADRs per carried package, port binding ADRs (or state why not), and name changed/dropped ADR decisions in an owner question or ruling citation (#157) | `arch` § Surface inventory cat. 6, § Extraction slices; `rev` § Inventory, Pre-owner design review | review only |
| Designs state observable behaviour and proving test, not lock/wait-group/join mechanics; open findings after three review rounds go to the owner | `arch` § Design discipline; `.agents/README.md` § Orchestrating role agents | review only |
| Inventories list every overlapping open PR (drafts included), and designs state merge order | `arch` § Surface inventory cat. 3; `rev` § Inventory review | review only |
| A boundary change is checked against the stated purpose | `arch` § Intent check | review only |
| Ported packages bring their SemStreams guidance (contract sections and skills) with them | `arch` § Extraction slices | review only |
| Ported packages building `GenericJSON` from known fields get an `adapt` item (registered payload or stated reason) (#9) | `arch` § Extraction slices; `rev` § Port-time and pattern review | review only until first ported caller |
| A new rule names what enforces it and adds its row to this table | `rev` § Port-time and pattern review | review only |
| Role-agent briefs carry the owner's intent and the artifact itself, not a paraphrase; resume for continuity, start fresh for mechanics | `.agents/README.md` § Orchestrating role agents | review only |
| Neither `semengine-architect` nor `semengine-reviewer` runs on a weaker model/effort than `semengine-developer`; `semengine-technical-writer` may use a lighter tier (#224) | `.agents/README.md` § Model routing | `.claude/agents/`, `.codex/agents/`; review only until #225 pins architect adapters + contract test |
| `AGENTS.md` stays under 24,000 bytes so rule loaders with a 24 KB per-file cap never truncate it (#224) | `.agents/README.md` § Manual read-only parity smoke, item 5; this file | review only until #225 adds contract test |
| Code PRs (any non-`.md`/non-`openspec/` file, or `.claude/agents/*`) require review by an agent that wrote no commits (`implemented-by:` and `reviewed-by:` in PR body); docs-only/archive commits take any agent's reviewer (#64, #66) | `protocol` § Cross-agent review; `rev` § Purpose and authority; `merge-gate` spec | `scripts/merge-check.sh` (`merge-check` job, `task merge:check -- <n>`), `TestMergeCheckReview`, `TestCIWorkflowPinned` (#225 tracks `gemini`, `.agents/contracts/`); review record truth/re-reviews: review only |
| Every OpenSpec task can be ticked in or before the archive commit; a hold is written as `Hold:` inside the unticked task it stops | `protocol`, "Target state, task truth, holds"; `spec-queue` spec; `rev` § Contract and task-truth review | `task spec:check` (`TestSpecQueueCheck`, `TestSpecCheckWiring`), `task spec:queue` (`TestSpecQueueHolds`); archive tickability, alternate spellings: review only |
| Pushed claim branches update by merging `origin/main`, never by rebase or force-push | `protocol` § Land | review only |
| Evidence cited in a PR or task can be opened on another machine; git-ignored paths are named as local only | `.agents/skills/semengine-handoff/SKILL.md`; `.agents/skills/semengine-preflight/SKILL.md` | review only |
| Tests use an independent oracle and are shown able to fail | `docs/testing.md`; `dev`, `rev`, test fidelity | structural guards carry paired sensitivity tests (`internal/harness/contract`); elsewhere: review only |
| Changes with interacting inputs, a stated law, or order-dependent history record whether generated checks are used or why examples suffice | `docs/testing.md` § Decide whether generated checks are needed; `dev`, `rev` | review only |
| Generated runs record seed, completed checks, and replayable failure; assertions are shown to run; histories check against a test-owned reference model | `docs/testing.md` § Fuzz targets and property-based tests; `dev`, `rev` | review only |
| Mutation checks report survivors/inconclusive runs as such, never as detections; fuzz seed replay and exploration report apart; PRs record uncovered cases | `docs/testing.md` § Show that the test can fail; `dev` § Handoff | review only (`task mutate:check` prints the record) |
| Sister repositories are read-only and inventoried only as scoped | `docs/inventory-scope.md` | review only |
| Docs are written for a working developer: coined terms defined at first use, no jargon, no marketing | `writer` rule 9; `rev` § Contract and task-truth review | review only |
| From Slice 04A change 3 on, a porting PR carries one package or lifecycle owner (<= ~3,000 non-test pin lines, ~1-page design, own OpenSpec change) unless the owner waives by count (#159) | `arch` § One package per pull request; `rev` § Pre-owner design, Contract review | review only |
| From Slice 04A change 3 on, before a porting design is written, the shape sweep reads each carried package on the pin copy and records each finding with its required/optional disposition (#159, #9) | `arch` § The shape sweep; `protocol` § Bounded port scope; `rev` § Pre-owner design review | review only |
| From Slice 04A change 3 on, the inventory lists every open issue naming a carried package and the design states whether the PR retires it (`Closes #n`) or it stays under Bounded port scope (#160) | `arch` § Open issues against the package; `rev` § Pre-owner design review; `protocol` § Bounded port scope | review only |
| Porting findings add required work only when tied to a binding requirement, admission/verify gate, owner ruling, accepted design commitment, or needed build/packaging change | `protocol` § Bounded port scope | review only |
| Developer notes on task choices not spelled out in the design go in `openspec/changes/<id>/notes/<task>.md`, linked from a PR comment (#159) | `dev` § Handoff; `protocol` § Land; `rev` § Contract and task-truth review | review only |

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

The shared protocol is `.agents/protocol.md`. Read it before filing, taking, landing, or closing work. Three gates
never become a pointer:

- **Claim:** a draft PR opened before the work, in its own worktree on an agent-prefixed branch
  (`git worktree add ../semengine-wt/claude/<topic> -b claude/<topic> origin/main`; Codex uses `codex/`, Gemini uses
  `gemini/`, and Enjoy desktop sessions use `move_to_worktree` under `.worktrees/<branch>`). No draft PR, no claim.
- **Merge:** CI green on a head up to date with `main`; no known flake open (an open `class:flake` issue) unless the PR
  closes every open one; `task merge:check -- <n>` immediately before merging (`GITHUB_ACTIONS` unset);
  `implemented-by:` in the PR body (`claude`, `codex`, or `gemini` beside the model/persona); archive/spec sync as the
  last content commit; squash merge. A code PR (any changed file not Markdown or under `openspec/`, or a role adapter
  under `.claude/agents/`) also needs another agent's review and `reviewed-by:` in the PR body, without which
  `merge-check` fails once non-draft (today `scripts/merge-check.sh` checks `claude` and `codex`; #225 tracks adding
  `gemini`); a documents-only PR does not.
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
