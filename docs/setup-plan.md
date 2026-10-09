# Draft SemEngine Tooling and Infrastructure Plan

Status: draft for discussion, 2026-09-30. This proposes the setup sequence; it does not authorize implementation.

*Amended 2026-10-01 per #8 ([comment
5932313950](https://github.com/C360Studio/semengine/issues/8#issuecomment-5932313950)): starter consumers and admission
by mandate added below; tiers are capability levels and 04A–04C are slices.*

SemSource is SemEngine's first consumer and the workload that should establish its value. The starter consumers are
semsource, semconnect, semboids and semteams; semembed and seminstruct are support services. SemEngine remains a
graph framework. Its first boundary should emerge from SemSource's ingestion, updates, deletions, queries,
provenance, content retrieval, and restart behavior. Dogfooding should help humans and agents work on these repos.

Preserve SemStreams' earned correctness while reducing the maintained code, docs, and process. Port relevant
implementations and tests instead of rewriting them to make extraction look tidy.

The MVP should not depend on SemStreams at runtime. Admit and extract necessary dependencies outside `graph/`
explicitly; give any temporary bridge a removal condition. Consumer adaptation belongs with the SemSource owner:
a new module path is a contract migration, not simply a `go.mod` version bump.

Acceptance: SemSource's retained workload runs on SemEngine, and a developer can understand and change that behavior
using its own code, tests, and current docs. Reconsider a boundary that exceeds the measured closure ceiling below.

## Evidence baseline

The audit inspected these snapshots and existing tests on 2026-09-30; it ran no suites or services.
SETUP 03A records the exact pins. A pin changes only by owner ruling.

- Extraction source: the newest SemStreams main commit when SETUP 03A runs, or the `v1.0.0-beta.163` tag if it has
  landed by then (owner ruling, 2026-09-30, superseding a same-day beta.161 ruling). 03A records the exact SHA.
- SemStreams main: [`5457b345`][semstreams], the audit snapshot. It is 216 commits past `v1.0.0-beta.161`. Of the
  185 non-merge commits, 66 touch the port set, 27 of them breaking, and ADRs 094–107 were all written in that
  window. That is the lifecycle, identity, and authority campaign the admission gates require; a beta.161 pin would
  forfeit it, and cherry-picking it is not viable because it is interleaved with agentic breaking changes.
- SemSource: [`34bda640`][semsource], the consumer reference for the initial contract and qualification workload.
  It pins SemStreams `v1.0.0-beta.161`.
- SemConnect: [`d0d06e00`][semconnect], an independent domain reference, pins SemStreams `v1.0.0-beta.160`.
  Its consumer behavior is a separate baseline, not evidence for compatibility with the extraction pin.
- Provider references: semembed [`7ceb5281`][semembed] and seminstruct [`7f9135a9`][seminstruct]. These audit snapshots
  do not pin a served build or model. SETUP 03A records the served identities its baseline runs use and holds them
  constant; SETUP 03B approves the provider contract and qualifies the capability.

Preserve existing MIT notices, including Copyright (c) 2025 C360.

### Measured dependency closure

`go list -deps ./...` at the SemSource reference, resolved against `v1.0.0-beta.161`, gives SemSource's compile-time
closure. Line counts are raw non-test `.go` lines per package directory.

| Scope | Packages | Non-test lines | Share of module |
| --- | --- | --- | --- |
| SemStreams at beta.161, whole module | 171 | 273K | 100% |
| SemSource's closure today | 97 | 204K | 74% |
| Direct imports without the two registries | 46 | 82K | 30% |
| Port set: the row above plus the graph processors and gateway SemSource composes | 63 | 127K | 46% |
| `graph/` alone | 9 | 23K | 8% |

SemSource imports 25 SemStreams packages directly. Two of them, imported only by `cmd/semsource/run.go`, register
every component and payload. The graph processors SemSource composes (`graph-ingest`, `graph-index`, `graph-query`,
`graph-embedding`) and `graph-gateway` reach it only through that registry. `graph/` is a small part of what
SemSource needs. Most of the port set is component, service, config, message, NATS, storage, fusion, and processor
code.

The registration cut is therefore the first boundary decision: SemSource registers the components and payloads it
uses instead of importing the full registries. The port set row is the closure ceiling. It still reaches `agentic`,
`agentic/agentrun`, `pkg/rulepack`, `vocabulary/agentic`, `flowstore`, and `engine` through `service`, `config`, and
`component`; SETUP 03B decides whether each is separated or admitted. Growth beyond the ceiling needs architect
approval.

## Earned design baseline

*Amended 2026-10-01 per #8: the tier model below replaces the provisional slice labels and the open term question.*

Every tier runs the same graph model and contracts. A tier is a capability level with graceful fallback, and a tier
boundary exists only where a dependency can be lost at runtime. A deployment runs at the highest tier whose providers
are available and degrades to the next lower tier when one is lost.

- **Tier 0: no external provider.** Graph foundation, rules, entity workflows, settlement, clustering on explicit
  edges, hierarchy inference, lexical BM25 (in process), change observation, and the operator surface.
- **Tier 1: an embedding provider is present.** Neural retrieval; losing the provider degrades to tier 0 with search
  intact.
- **Tier 2: an LLM provider is present.** The slot is defined and empty at the MVP: clustering summaries, the
  inference review worker, the query classifier, and the agentic domain when SemTeams brings it.

Extraction proceeds in **slices**, which are steps, not tiers. Qualify the slices in order, keeping each lower tier
independently useful and deployable. BM25 and neural name mechanisms that can support several
graph capabilities; retrieval does not exhaust their role, and neural retrieval does not imply generation.

| SemEngine slice | SemEngine tier | SemStreams source tier | SemSource profile in `configs/tiers` |
| --- | --- | --- | --- |
| 04A: Graph foundation | 0 | 0: no embedding | Structural, unnumbered |
| 04B: Lexical retrieval (BM25 completes tier 0) | 0 | 1: BM25 | 0: BM25 |
| 04C: Neural retrieval | 1 | 2: neural | 1: neural |
| Separately admitted generation | 2 (slot, empty at the MVP) | Independent optional capability | 2: neural plus instruct |

Record the crosswalk against selected configs before migration; do not equate numbers across repositories or rename
sister-repo files. SemEngine tier 0 maps to SemSource's `configs/tiers/tier0-statistical.json` and tier 1 to
`tier1-semantic.json`. SemSource's numbered examples use `bm25`/`http`/`http`; `tier2-semantic-instruct.json` leaves
clustering off. `tier2-compose-dev.json` plus its opt-in dev overlay enables clustering and LLM; the shipped MVP uses
semembed only. Generative answers/community enrichment require separate admission. New tiers preserve identity,
authority, and graph correctness. The word is "tier"
([#4](https://github.com/C360Studio/semengine/issues/4#issuecomment-5929716018)); the capability matrix, with each
behavior's tier and proving evidence, is in the SETUP 03B change
(`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md`).

Keep discovery and ranking signals distinct from materialized or inferred facts and edges. Relevance does not
automatically become an asserted graph fact. Any such write needs an explicit mutation, provenance, and authority
contract; adding a capability alone does not authorize it.

Retain pragmatic subject/predicate/object facts, `Graphable`, explicit entity references, and vocabulary extension.
Keep statement source, time, confidence, and correlation metadata through retained ingest, mutation, storage, query,
and export paths according to the contract. This is RDF*-like statement metadata, not a claim of formal RDF-star
compliance. Preserve internal predicate conventions and external IRI mappings without requiring ontology machinery.

Component and service contracts remain the composition baseline: declared dependencies and ports, typed operations,
readiness, and explicit lifecycle ownership. Preserve their behavior while auditing implementations independently.
Domain vocabularies and adapters stay consumer-owned; generic registration and export mechanisms belong in the engine.

### Capability and service qualification

*Amended 2026-10-01 per #8: rows renumbered to the tier model.*

This service matrix preserves the shared graph and separates embedding from instruction/generation.

| Tier | Graph capability | Embedding dependency | Generation dependency |
| --- | --- | --- | --- |
| 0 | Ingestion, mutation, graph queries; text relevance by BM25 | None (BM25 is in process) | Not admitted |
| 1 | Learned similarity beyond shared terms | Qualified embedding provider | Not admitted |
| 2 | LLM-backed features (slot, empty at the MVP) | As tier 1 | LLM provider; separate admission |

semembed is the reference tier 1 embedding service; seminstruct is the reference optional generation service.
Generation requires its own consumer contract and qualification; this is not a claim that SemSource ships a supported
BM25-plus-generation configuration. Neither generation nor seminstruct is mandatory for tier 1.
Provider deployment stays outside the engine; SemEngine owns typed callers, context, freshness, and failure behavior.
Prompts and generation policy remain product-owned.

Equivalent providers are eligible only after qualifying the exact adapter contract; an "OpenAI-compatible" label
does not establish interchangeability. The audited semembed serves a boot-selected local model, while seminstruct
runs a llama.cpp server with a baked GGUF model. Pin service build/image, served model/artifact/version, vector
dimensions, query prefixes/preprocessing, and any prompt/output contract. Request model labels alone are insufficient.
Record cache/index identity and the rebuild or invalidation rule when those inputs change.
The audited [semembed serving code][embed-serving] ignores request model selection; unknown configured names can
serve a fallback while echoing the configured name. Qualify actual served artifacts/dimensions, not HTTP success alone.

Real-provider tests apply to each admitted provider-backed capability. Check request/response semantics, readiness,
context propagation, deadlines, cancellation, and recovery using the exact selected configuration. A configured but
missing provider is unavailable evidence, never a passing or silently skipped gate. Report capability availability
truthfully while retaining usable lower-tier graph operations; do not turn provider failures into absence findings.

### SemConnect reference fixtures

Use three small consumer-owned cases: SensorML Asset to Graphable triples and Turtle vocabulary mappings; typed
root-only create/reconcile/delete with exact revisions and an absent relationship target; immutable schema artifact
bytes plus a graph `StorageReference`. These exercise a second domain without adding a full SemConnect migration,
OGC package port, or conformance suite to the MVP. Sources at the recorded SemConnect revision are
`parser/sensorml/graphable_test.go`, `gateway/cs-api/beta160_structural_integration_test.go`, and
`gateway/cs-api/schema_artifacts_test.go`, supported by `vocabulary/csapi/register_test.go`.

SemConnect substantiates generic vocabulary, typed mutation, projection, artifact, and component seams. Its reference
binary composes a standalone component; it does not prove full ServiceManager orchestration or statement metadata
preservation. Its beta.160 projection adapter also exposes a useful helper seam: reconcile using the revision already
read by the caller. Assess that need across consumers before adding a generic helper.

## Fusion and graph tool boundary

SemSource uses synchronous fusion for `code_context`, `code_impact`, `code_search`, and `doc_context`; its separate
`graph_search` path calls `graph.query.searchGraph`. Both families need qualification. Recommend four layers:
graph substrate; generic deterministic fusion; SemSource lenses and policy; consumer-owned public adapters.
SemEngine owns graph state, mutation, indexes, storage and readiness, plus fusion resolve, expand, hydrate, rank,
budget, provenance, and partial results. SemSource owns code/docs lenses, domain and symbol policy, supersession,
and MCP/HTTP adapters. Retain its lens-driven `Engine.Fuse` path first; qualify other generic entry paths by need.
Initially exclude the other paths' research and agentic consumers.

Before selecting the port set, map both families to current behavior at the pin, relevant defects, and
owner-approved shared contracts. Record keep, change, or defer with test implications. The `beta.161` baseline is
evidence of current behavior, not a mandate for the future API.

The [graph tool consolidation discussion][graph-tools] chiefly concerns the later SemTeams agentic surface. Its full
redesign is not an MVP dependency; only demonstrated shared-contract overlap blocks extraction. Keep agentic planning
with that consumer. Toolchain, coordination, and isolated harness setup can proceed while this boundary is reviewed.

## Package admission heuristic

*Amended 2026-10-01 per #8 (Q14): admission by owner mandate added beside consumer need.*

A capability is engine-owned when it belongs to either half of SemEngine (the live semantic knowledge graph, or
durable execution) and runs at tier 0 without an external provider; it is admitted by owner mandate at a named tier
with a named qualifying workload, and consumer need decides order, never membership (`AGENTS.md`; epic #8, Q14).

Review each consumer-visible slice and its retained dependency closure, not every package in SemStreams. Every
retained package gets a short admission review. Go deeper for asynchronous work, persistence, wire formats, shared
mutable state, and helpers with many callers. Reuse a shared helper's review only at the same revision and with
unchanged required guarantees. Missing evidence first requires targeted qualification, not presumed code repair.

Use one ledger row per retained package: source path/full SHA, consumer purpose, destination, contract, dependencies
and side effects, known risks, proving tests, owner, and disposition. Seed the ledger from the measured closure and
cross-check it against SemStreams' [sister-import list][sister-imports]. Choose one of four outcomes:

- **Carry:** behavior and implementation satisfy the admitted contract; preserve code and relevant tests.
- **Adapt:** change packaging or a bounded seam with explicit behavior differences and regression evidence.
- **Repair before port:** qualification finds a broken required guarantee; prove the repair in SemEngine, behind a
  failing-first test, before the package is admitted.
- **Defer/exclude:** no retained consumer need and no owner mandate, or narrow the contract explicitly instead of
  importing the obligation.

Integrity, silent-loss prevention, context ownership, completed joins, authority/readiness, acknowledged durability,
and metadata/content preservation are admission gates. An issue, elapsed audit budget, or passing coverage number
cannot waive them. Noncritical duplication, naming, or optimization may have an owner, bounded scope, proving test,
and due milestone. Unproven guarantees hold admission until qualification passes; elapsed time is not a pass.
Avoid opportunistic refactors. Changed behavior needs failing-first tests; unchanged extraction retains earned tests
and adds only missing boundary evidence. Independent reviewer approval is required before each slice integrates.

The pin is frozen (owner ruling, 2026-09-30). Repairs land in SemEngine behind failing-first tests and are not
round-tripped through SemStreams. A SemStreams change made after the pin enters only as its own ledger row, with the
source commit and a proving test. Nothing syncs automatically in either direction.

SemStreams' lifecycle campaign is not finished: at the audit, [epic 1147][ss-1147] and issues 1145, 1411, 1415, 1417,
and 1218–1220 were open. Whatever is still open against a port-set package at pin time becomes that package's
repair-before-port row (owner ruling, 2026-09-30). SemStreams decides separately for its remaining surface.

The context and lifecycle rules below bind new and changed code. Carried code is triaged in the ledger instead of
being rewritten wholesale: the 46-package row has about 66 non-test `context.Background()` and `context.TODO()`
call sites (a grep count, not an audit). Record each as a legitimate root with its reason, or as a defect. A root
that breaks an admission gate is a repair; the others are carried unchanged.

## Proposed foundation

Start with a Go module consumed by SemSource. This recommendation sets the packaging direction, not the public API.
The architect approves the consumer contract before API implementation or extraction. Add a runtime binary only if
the retained workload needs one; do not copy SemStreams' binary release machinery by default.

Keep the familiar Go, Task, NATS JetStream, Docker, and testcontainers tool set where it serves retained behavior.
Choose exact versions from qualified sources. Include Node and OpenSpec in version checks; SemStreams' Node 22 and
`@fission-ai/openspec@1.7.0` are candidates to qualify, not newest-version claims or a frontend toolchain commitment.
Document behavioral pins, especially NATS client regressions. Upgrades need relevant regression evidence; avoid mixing
blanket upgrades with extraction.

Use one Task entrypoint for local verification and CI, with the same underlying commands. Proposed responsibilities:

- `task fmt` changes formatting; verification checks formatting without changing tracked files.
- `task doctor` reports version and Docker diagnostics without starting workloads.
- `task verify` checks strict OpenSpec, formatting and module tidiness, build/vet, pinned lint and security, then race
  tests and integration/consumer lanes as they exist. It must leave tracked files unchanged.
- Focused tasks expose unit, integration, and consumer qualification checks for development and CI reuse.
- Pin `revive` and `govulncheck`; review vulnerability findings against reachable behavior and the selected toolchain.
  Go's [security guidance][go-security] describes the role of `govulncheck`.

Order checks from cheap to expensive. Define critical packages when the extraction boundary is approved, and enforce
an 80% coverage minimum for them alongside lifecycle, concurrency, integration, and regression evidence. SemStreams
states 80% as guidance in its testing docs and the audit found no CI enforcement, so treat this as a new gate.
Measure gate durations before setting enforceable budgets.

GitHub Actions should cancel superseded PR runs, apply explicit timeouts, use minimal token permissions, and pin
third-party actions to full commit SHAs with an update policy. See [GitHub's security guidance][actions-security].
The final required job must fail if required checks fail, are missing, or are unexpectedly skipped or cancelled.
Conditional test selection must name the required evidence so aggregate success cannot conceal an omitted suite.

## Development and test infrastructure

Separate persistent development services from disposable tests. Each test run owns its containers, networks, volumes,
process group, ports, and artifacts. Use dynamic ports and unique [Compose project names][compose-projects]; ownership
also covers NATS streams, buckets, and consumers when sharing a broker.

Prefer testcontainers for small Go integration fixtures and a scoped Compose project for a full consumer stack.
Wait for authoritative readiness and expected state with deadlines. Avoid fixed sleeps and shared default ports.
Start with conservative concurrency limits. Dynamic ports prevent collisions but do not prevent CPU or memory
saturation. During extraction, admission control for a shared Docker daemon must coordinate SemEngine and SemStreams;
independent repository locks must not each claim exclusive use of the same daemon.

Propagate cancellation to child processes. Capture exit status, logs, readiness failures, versions, and owned resource
identities before cleanup on success, failure, cancellation, or partial startup. Never use broad Docker prune commands.
An interrupted run must leave unrelated worktrees and persistent development services intact.

For paid or prolonged runs, poll concrete state every 30–60 seconds and validate log filters against real output.
Compare timestamps with expected duration and stop provably wedged work. Default qualification requires no metered
hosted API; self-hosted compute, memory, storage, and wall time still have real costs. Hosted runs require deliberate
selection and bounded time, request/token, and cost policies. Never silently fall back to a hosted provider.

### Day-one helpers and lifecycle ownership

Build a small helper kit before porting slices, using standard `context`, channels, `sync`, and `errors`. Keep names
and APIs tied to concrete responsibilities; avoid a generic `utils` package or hidden context manager. Production
packages must not depend on test libraries. Add retries only for a real caller with proven cancellation semantics.

The initial kit comprises owned real-NATS fixtures, authoritative bounded waits, checked cleanup, component lifecycle
tests, explicit callback-entered/release/join probes, classified errors with operation context, and valid Graphable,
triple, metadata, and content-reference fixtures. Reuse proven helpers and fix identified gaps before multiplying them.
Synchronous fakes cannot prove asynchronous NATS behavior; aggregate goroutine counts cannot prove an owner's join.

Follow [Go context guidance][go-context]: pass context first to I/O, propagate it, and call derived cancel functions.
Do not retain contexts in production structs or wrappers/providers; do not invent background roots.
Specify which owner admits work, stops admission, drains/finalizes, cancels, and joins each worker or resource.
Controlled Stop uses the caller's exact finite context, keeping Start authority live until graceful finalization;
abort is distinct. The callee must not replace Stop authority with its own timeout. An owner may create fresh finite
authority for test cleanup. Shutdown order belongs to each resource owner; do not universally cancel before draining.
Timeout or cancellation does not establish completed callback or worker joins.

Lifecycle tests cover partial startup cleanup, cancellation during I/O, blocked callbacks, graceful finalization,
deadline expiry, repeated Stop, and restart where promised. Check completed cleanup and retained data, not only return
values. The shared lifecycle suite is a floor, not proof of every owner's drain and join protocol. SemStreams'
`pkg/lifecycle` governs graph business workflows; it is not automatically needed to manage component goroutines.

## Shared state and working process

Retain GitHub as the async communication bus and external project state, OpenSpec for planning and implementation
details, and SemStreams' shared agents and skills for Codex and Claude continuity, following the user's decision.
Issues own decisions and project status; draft PRs own claims and stop points; OpenSpec changes own designs, tasks,
and holds. This supersedes the generic `/tickets` example: no competing ticket state is needed.

Keep a short root `AGENTS.md` mapping commands, contracts, and role guidance. Use one shared platform-neutral protocol
and thin `.codex` and `.claude` adapters. Retain architect, developer, independent reviewer, and technical writer
handoffs with scoped role instructions. No Svelte workflow is needed without an engine UI.

Adapt the proven `semstreams-handoff`, `semstreams-pickup`, and `semstreams-preflight` mechanisms to SemEngine.
Test the shared workflow on both platforms; add architectural helper skills only for admitted engine capabilities.
Preserve these continuity requirements in their single shared home:

- Checkpoints use Goal, Addresses, Constraints, Done when, and Checks to run.
- Pickup reconciles live GitHub, OpenSpec, branch, worktree, and evidence state in the actual claimed worktree.
  An unavailable read is unavailable, never an empty queue or passing gate.
- The previous writer must stop or release ownership before pickup edits begin.
- Evidence identifies the tested SHA or reproducible dirty snapshot. Handoff alone does not require costly reruns.
  Record pending paid or background runs and their owners so the next session does not duplicate them.
- Shared records remain authoritative; neither a separate handoff document nor private memory is required.

Developers write failing tests for changed behavior, implement, and show evidence; extraction retains relevant tests
and adds missing boundary tests. The independent reviewer checks architecture, context handling, concurrency, errors,
and critical paths before integration. The technical writer updates current docs with the approved code.

Retain draft PR claims and review of OpenSpec archive and spec sync before merge. Adapt preflight to the actual gate
graph, including the separate OpenSpec check; stale command or job names must not become authority. Bootstrap needs a
bounded initial exception while the empty repository has no base commit, protocol, or normal claim workflow.

Current docs include the repository map, architecture, OpenSpec contracts, setup, verification, and admission ledger.
Keep raw logs and reports in CI or local artifacts with retention limits instead of committing them as design history.

Update contracts with code. Initialize OpenSpec fresh; leave old proposals and transcripts in SemStreams, retrievable
through source commits. The work packages below describe sequence and gates; this plan is not a task tracker.

## Setup work packages

### SETUP 01 Foundation

Owner: architect for boundaries, Go developer for tooling, independent Go reviewer for sign-off, technical writer
for current instructions. Dependency: agreement on this plan's scope and bootstrap choices.

Create the minimal module, repository map, fresh OpenSpec setup, shared protocol, agents and skills, pinned tools,
and CI entrypoint. Establish GitHub state homes and record module and release shape without inventing consumer APIs.
Document the source license and provenance requirements. Establish formatting, security checks, and artifact handling.

Pass evidence: a fresh checkout can run the documented commands; local and CI checks agree; verification leaves the
tree unchanged; a deliberately missing or failed check prevents aggregate success. Demonstrate Codex-to-Claude pickup
without private context. Record initial timings. Architect and reviewer sign-off unlock the harness work.
Activate code coverage and consumer lanes when their packages and workload exist; do not add placeholder code to
satisfy empty checks. Once introduced, these lanes remain required according to the gate graph.

### SETUP 02 Isolated harness

Owner: Go developer, reviewed by an independent Go reviewer. Dependency: SETUP 01 sign-off.

Build the initial helper kit, owned disposable NATS fixtures, cancellation, bounded readiness, checked cleanup, and
failure capture before porting slices. Separate persistent development commands. Bound Docker concurrency and
coordinate admission across the repos sharing the daemon. Prove lifecycle ownership before expanding the service stack.

Pass evidence: two worktrees run without resource collisions; interrupting one leaves the other intact; a forced
startup failure captures evidence and cleans partially created resources. Prove that persistent development data and
unrelated containers survive cleanup. Reviewer sign-off unlocks SETUP 03B.

### SETUP 03A Pinned consumer baseline

Owner: SemSource owner for the migration, Go developer for fixtures and measurement, independent Go reviewer for
evidence. Dependency: agreement on this plan. It runs on SemStreams, so it may proceed alongside SETUP 01 and 02.

Record the exact pin. Build the known-answer corpus and run it on SemSource as it is today, under each SemSource
config that a SemEngine tier will be compared against. Then migrate SemSource to the pin on SemStreams and run the
corpus again. The port set carries at least 27 breaking changes since beta.161 that SemSource must absorb regardless,
and the later module-path swap is mechanical. Differences between the two runs are upstream behavior changes,
explained by ADRs 094–107 or filed as defects. Re-measure the dependency closure at the pin, including test-only
dependencies. Every later comparison then has two points: SemSource on SemStreams at the pin, and the same workload
on SemEngine.

For provider-backed configs, record the service image or build, served model artifact and version, vector
dimensions, and query preprocessing before the first run, and hold them constant across both runs. semembed selects
its model from `SEMEMBED_MODEL` at startup, so an unchanged SemSource config does not prove an unchanged model. Carry
those identities into every later comparison. Graph-only and BM25 baselines need no provider record.

Pass evidence: the known-answer workload below has recorded expectations and results on both SemStreams revisions
for each compared config; provider identities are recorded and unchanged across the runs; every difference between
them is attributed; the closure measurement is recorded with its commands. The reviewer approves the baseline
evidence.

### SETUP 03B Contract and boundary

Owner: architect for contract approval, independent Go reviewer for evidence, technical writer for the contract.
Dependency: SETUP 02 and SETUP 03A sign-off.

Decide the registration cut and resolve the fusion and graph tool boundary, then choose the port set within the
closure ceiling. Inventory retained dependencies outside `graph/`. Agree on entity identity, update and deletion
semantics, provenance, content access, query behavior, and acknowledged writes.
Required deliverable: a retained-contract matrix mapping observed `beta.161` behavior to intended SemEngine behavior,
keep/change/defer, owner, and proving test. Include tiers, Graphable/vocabulary, statement metadata, content,
component/service seams, provider/model identity, context/stop/join/durability invariants, and the crosswalk.
Include SemConnect reference cases.
Explicitly record fusion and graph-tool keep/change/defer decisions, owners, and proving tests in that matrix.
Evaluate find/anchor/ask as cases, not an already agreed mode API.

Pass evidence: the matrix is complete against the 03A baseline, and the open owner ruling under deferred work is
recorded. The architect approves the contract, port set, and critical package list before extraction begins.

### SETUP 04 Sequential extraction and slice releases

*Amended 2026-10-01 per #8: slices 04A–04C are named by tier; tier 0 is complete after 04B.*

Owner: Go developer for extraction and consumer integration, independent Go reviewer for release evidence, architect
for contract changes, technical writer for current docs and extraction ledger. Dependency: SETUP 03B sign-off.

Port small slices using the admission heuristic, mapping each dependency closure to the contract and ledger.
From Slice 04A change 3 on, a porting pull request carries one package, and each dependency closure is a group of
such pull requests under one milestone (architect contract, Extraction slices, "One package per pull request"; owner
ruling 2026-10-09, epic #9 comment 6085243841).
Shared contract planning may look ahead; do not port the next slice until the current slice passes its architect and
independent reviewer gates. Record the exact qualified engine commit, SemSource SHA, configuration, and admitted tier.
Keep the accepted lower tier deployable and retain every promised lower-tier regression suite at each stage,
including after model/provider changes. Test tiers independently: this does not require identical rankings or
simultaneous BM25/neural operation. Hybrid retrieval needs separate design. Measure budgets with real workloads.

Qualify each slice on a SemSource integration branch built wholly on SemEngine; the SemSource owner owns that branch
and its per-tier compositions. No binary imports both modules: their Go types are distinct, and both would claim
`GRAPH`, `ENTITY_STATES`, `graph.ingest.>`, and `graph.mutation.>`. SemSource's shipped MVP uses semembed, so its
mainline stays on SemStreams until tier 1 (slice 04C) passes. Tier 0 is a usable lower tier, not its migration
point.

#### Slice 04A: tier-0 graph foundation (provisional)

Qualify Graphable ingestion, typed identities/triples, metadata, vocabulary, mutations, indexed exact queries,
references/body retrieval, and minimal deterministic fusion, including SemConnect and context/lifecycle/restart cases.
SemSource currently always configures graph-embedding: prove a true no-embedder composition and explicitly admitted
graph-only interfaces, not unsupported NL verbs. The SemSource owner builds that composition on the integration
branch. SemSource has no no-embedder config, so the baseline is its structural tools' behavior under
`tier0-statistical.json` at the pin. NATS remains required; no model service is needed. Compile-time imports beyond
the admitted port set, including clustering, LLM, agentic, and rule packages, need a reviewed separation; any
dormant bridge needs an owner and exit condition, not implicit higher-tier admission. Architect contract approval
and independent evidence review unlock slice 04B. (Under the ruled scope, rules, entity workflows and clustering on
explicit edges are tier 0; the SETUP 03B change records their rows.)

#### Slice 04B: tier-0 lexical — BM25 completes tier 0 (provisional)

Add BM25 with known-answer retrieval and ranking cases, scope-before-limit behavior, result caps, stale/update
semantics, index rebuild, and restart. Qualify the base SemSource lexical paths without a model service, retaining
all tier 0 guarantees. Architect and reviewer acceptance unlock slice 04C.

#### Slice 04C: tier-1 neural — embedding provider (provisional)

Add real embeddings from semembed or a qualified equivalent with pinned identity, paraphrase cases, mixed code/docs,
warm/cold paths, deadlines, provider unavailability, and recovery. Specify and test fallback or explicit `Deferred`
outcomes without weakening tier 0; losing the provider degrades to tier 0 with search intact. Only this stage qualifies
the selected full SemSource neural/default workload; generator-backed answers or community enrichment still need
separate admission.

Pass evidence: the declared tier's workload and SemEngine dogfooding pass through admitted SemSource human/agent
interfaces; current docs/tests explain the behavior. Release claims name the tier and tag the exact tested commit and
consumer baseline. Tier 0 needs no embedding-service run. Tier 1 releases and embedding changes require
bounded real-provider qualification; separately admitted generation requires its own real-provider evidence at any
admitted tier. Release checks must verify applicable evidence, never bypass it during tagging.

## Consumer qualification evidence

Use a small versioned corpus with known entities, relationships, content, and source locations. It should include
exact expected answers and explicit absence checks, so startup success cannot substitute for graph correctness.
Then ingest the SemEngine repository through SemSource as a second, useful dogfood workload.

The retained workload should establish all of the following through the interfaces humans and agents actually use:

1. Ingestion produces expected entities and relationships, with a bounded completion condition.
2. Queries return expected answers, provenance resolves to the correct source and revision, and content retrieval
   returns the exact expected bytes or the explicitly documented transformation.
3. Editing a source changes the expected answers and content without preserving obsolete relationships as current.
4. Source deletion follows the agreed retention, stale marking, demotion, and query visibility contract; current
   SemSource retains entities. Recreation restores intended identity and content without duplicating current entities.
5. Restarting the application preserves the promised behavior. A separate broker restart with persistent storage
   verifies the durability contract and the recovery path, including accepted work that had not finished indexing.

Acknowledged-write durability needs an explicit decision. The source GRAPH transport defaults to memory storage;
an accepted publish alone must not be treated as proof that data survives broker restart. Specify what is acknowledged,
where it is durable, and whether recovery depends on replay or re-ingestion, then test that promise.

*Amended 2026-10-01 per #8: tiers renumbered.* Verification follows the admitted tier: deterministic graph and lexical
checks at tier 0 (slices 04A and 04B), and real embedding-provider qualification at tier 1 (slice 04C). Models do not
block tier 0; optional generation is verified only where admitted, and becomes required evidence there. All promised
lower-tier suites remain required. No benchmark targets or latency promises are assumed before measurement. Cover
duplicate-name anchor selection and mixed code/docs scoped before limiting where each capability is admitted; tier 1
adds actual warm/cold embedding paths with recorded model/configuration identity and stable acceptance criteria. Missing
bodies, partial hydration, stale readiness, or transport faults must not become confident not-found answers. Distinguish
healthy lag with freshness information from `Deferred`: no usable answer, rather than an absence finding.

## Known risks and what to carry forward

- SemSource's current smoke checks establish startup and route reachability, not complete semantic correctness.
  The known-answer workload must supply the missing evidence.
- SemStreams [RPC stream collision issue 1143][rpc-issue] is open. Reproduce it against the pin and assess
  whether the retained request paths need the fix before treating it as a current SemEngine defect.
- SemSource [issue 178][semsource-178] reports that beta.161 fails the OSH corpus tail against the GRAPH 256MiB
  ceiling that beta.159/160 absorbed. The 03A baseline must record this and check whether the pin resolves it.
- SemSource [physical deletion issue 210][delete-issue] is open. Physical removal differs from retained stale history;
  require hard removal only if admitted by the consumer contract, and reproduce the relevant behavior.
- Fusion risks include [truncation nondeterminism 621][fusion-truncation] and
  [impact counts without names 603][fusion-impact]. Reproduce retained paths before carrying these as current defects.

Carry reproduced defects and scoped unresolved risks with compact evidence and source links.

## Deferred work and decisions

Defer cloud infrastructure, Kubernetes, Terraform, fleet management, agentic runtime features, sister-project release
matrices, and extra domain skills or approval layers until the retained workload demonstrates a concrete need.
Keep SemStreams as reference until the workload qualifies and remaining ownership resolves; defer SemTeams migration.

Open owner ruling: how SemEngine relates to SemStreams [ADR-106][adr-106], which freezes the packages its sisters
import on the way to 1.0. Decide whether that freeze proceeds unchanged alongside SemEngine or whether extracted
packages leave the frozen surface. The ruling is due with SETUP 03B approval.

[ss-1147]: https://github.com/C360Studio/semstreams/issues/1147
[semsource-178]: https://github.com/C360Studio/semsource/issues/178
[sister-imports]:
  https://github.com/C360Studio/semstreams/blob/5457b3458936f668b71d2fea061f67f8d7d01e67/release/tier1-packages.txt
[adr-106]: https://github.com/C360Studio/semstreams/tree/5457b3458936f668b71d2fea061f67f8d7d01e67/docs/adr
[semstreams]: https://github.com/C360Studio/semstreams/tree/5457b3458936f668b71d2fea061f67f8d7d01e67
[semsource]: https://github.com/C360Studio/semsource/tree/34bda6406fb06fd723a040988647b204204a1583
[semconnect]: https://github.com/C360Studio/semconnect/tree/d0d06e00bf05a545f30ceea798db1c2b1ee47d4f
[semembed]: https://github.com/C360Studio/semembed/tree/7ceb5281c96b3664321f3f28c9d7f96acbb41843
[embed-serving]:
  https://github.com/C360Studio/semembed/blob/7ceb5281c96b3664321f3f28c9d7f96acbb41843/src/main.rs#L162-L188
[seminstruct]: https://github.com/C360Studio/seminstruct/tree/7f9135a99cd27a6c63a2a60db5daeee9f5622be4
[actions-security]: https://docs.github.com/en/actions/reference/security/secure-use
[go-security]: https://go.dev/doc/security/
[go-context]: https://pkg.go.dev/context
[compose-projects]: https://docs.docker.com/compose/how-tos/project-name/
[rpc-issue]: https://github.com/C360Studio/semstreams/issues/1143
[delete-issue]: https://github.com/C360Studio/semsource/issues/210
[graph-tools]: https://github.com/C360Studio/semstreams/issues/1422
[fusion-truncation]: https://github.com/C360Studio/semstreams/issues/621
[fusion-impact]: https://github.com/C360Studio/semstreams/issues/603
