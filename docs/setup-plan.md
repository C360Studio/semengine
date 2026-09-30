# Draft SemEngine Tooling and Infrastructure Plan

Status: draft for discussion, 2026-09-30. This proposes the setup sequence; it does not authorize implementation.

SemSource is SemEngine's first consumer and the workload that should establish its value. SemEngine remains a
graph framework. Its first boundary should emerge from SemSource's ingestion, updates, deletions, queries,
provenance, content retrieval, and restart behavior. Dogfooding should help humans and agents work on these repos.

Preserve SemStreams' earned correctness while reducing the maintained code, docs, and process. Port relevant
implementations and tests instead of rewriting them to make extraction look tidy.

The MVP should not depend on SemStreams at runtime. Admit and extract necessary dependencies outside `graph/`
explicitly; give any temporary bridge a removal condition. Consumer adaptation belongs with the SemSource owner:
a new module path is a contract migration, not simply a `go.mod` version bump.

Acceptance: SemSource's retained workload runs on SemEngine, and a developer can understand and change that behavior
using its own code, tests, and current docs. Reconsider a boundary that requires most of SemStreams' machinery.

## Evidence baseline

The audit inspected these live main source revisions and existing tests on 2026-09-30; it ran no suites or services:

- SemStreams: [`5457b345`][semstreams], the source reference for tooling and graph extraction.
- SemSource: [`34bda640`][semsource], the consumer reference for the initial contract and qualification workload.
- SemSource pins SemStreams `v1.0.0-beta.161`. That pin and SemStreams' newer main require separate qualification;
  a successful run against one does not establish compatibility with the other.
- SemConnect: [`d0d06e00`][semconnect], an independent domain reference, pins SemStreams `v1.0.0-beta.160`.
  Its consumer behavior is a separate baseline, not evidence for compatibility with the selected extraction revision.

Select source revisions explicitly and preserve existing MIT notices, including Copyright (c) 2025 C360.

## Earned design baseline

Every tier runs the same graph model and contracts. Tiers are capability profiles of that graph; the graph carries
semantics at every tier. Qualify migration in order: 0, then 1, then 2, keeping each lower profile independently useful
and deployable. The labels below provisionally describe qualification slices, not a settled or exhaustive taxonomy.
BM25 and neural name mechanisms that can support several graph capabilities; retrieval does not exhaust their role,
and neural retrieval does not imply generation.

| Provisional SemEngine qualification slice | SemStreams source tier | SemSource profile in `configs/tiers` |
| --- | --- | --- |
| 0: Graph foundation | 0: no embedding | Structural, unnumbered |
| 1: Lexical retrieval | 1: BM25 | 0: BM25 |
| 2: Neural retrieval | 2: neural | 1: neural |
| Separately admitted generation | Independent optional capability | 2: neural plus instruct |

Record the crosswalk against selected configs before migration; do not equate numbers or rename sister-repo files.
SemSource's numbered configs use `bm25`/`http`/`http`; instruct is not a supported default and disables clustering.
Generative answers/community enrichment require separate admission. Locate traversal, clustering, and rules by
consumer need and dependencies, not old tier names. New tiers preserve identity, authority, and graph correctness.

Before settling tier names, SETUP 03 must include a compact graph capability matrix: each added behavior, its
dependencies, and proving evidence. Define what each profile gives the graph beyond naming a search mechanism.

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

Before selecting the extraction revision or port set, map both families to current behavior, relevant defects, and
owner-approved shared contracts. Record keep, change, or defer with test implications. The `beta.161` baseline is
evidence of current behavior, not a mandate for the future API.

The [graph tool consolidation discussion][graph-tools] chiefly concerns the later SemTeams agentic surface. Its full
redesign is not an MVP dependency; only demonstrated shared-contract overlap blocks extraction. Keep agentic planning
with that consumer. Toolchain, coordination, and isolated harness setup can proceed while this boundary is reviewed.

## Package admission heuristic

Review each consumer-visible slice and its retained dependency closure, not every package in SemStreams. Every
retained package gets a short admission review. Go deeper for asynchronous work, persistence, wire formats, shared
mutable state, and helpers with many callers. Reuse a shared helper's review only at the same revision and with
unchanged required guarantees. Missing evidence first requires targeted qualification, not presumed code repair.

Use one ledger row per retained package: source path/full SHA, consumer purpose, destination, contract, dependencies
and side effects, known risks, proving tests, owner, and disposition. Choose one of four outcomes:

- **Carry:** behavior and implementation satisfy the admitted contract; preserve code and relevant tests.
- **Adapt:** change packaging or a bounded seam with explicit behavior differences and regression evidence.
- **Repair before port:** qualification finds a broken required guarantee; prove the repair before admission.
- **Defer/exclude:** no retained consumer need, or narrow the contract explicitly instead of importing the obligation.

Integrity, silent-loss prevention, context ownership, completed joins, authority/readiness, acknowledged durability,
and metadata/content preservation are admission gates. An issue, elapsed audit budget, or passing coverage number
cannot waive them. Noncritical duplication, naming, or optimization may have an owner, bounded scope, proving test,
and due milestone. Unproven guarantees hold admission until qualification passes; elapsed time is not a pass.
Avoid opportunistic refactors. Changed behavior needs failing-first tests; unchanged extraction retains earned tests
and adds only missing boundary evidence. Independent reviewer approval is required before each slice integrates.

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

Order checks from cheap to expensive. Define critical packages when the extraction boundary is approved, and retain
their 80% coverage minimum alongside lifecycle, concurrency, integration, and regression evidence. Measure gate
durations before setting enforceable budgets.

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
Compare timestamps with expected duration and stop provably wedged work. Qualification requires no paid LLM calls.

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
unrelated containers survive cleanup. Reviewer sign-off unlocks consumer baseline work.

### SETUP 03 Pinned consumer baseline and contract

Owner: architect for contract approval, Go developer for fixtures, independent Go reviewer for evidence, technical
writer for the contract. Dependency: SETUP 02 sign-off and a recorded SemSource revision.

Qualify SemSource against `beta.161`, then resolve the fusion and graph tool boundary before choosing the port set and
source revision. Inventory retained dependencies outside `graph/`; separately qualify the chosen revision. Agree on
entity identity, update and deletion semantics, provenance, content access, query behavior, and acknowledged writes.
Required deliverable: a retained-contract matrix mapping observed `beta.161` behavior to intended SemEngine behavior,
keep/change/defer, owner, and proving test. Include tiers, Graphable/vocabulary, statement metadata, content,
component/service seams, context/stop/join/durability invariants, and the crosswalk, with SemConnect reference cases.
Evaluate find/anchor/ask as cases, not an already agreed mode API.

Pass evidence: the known-answer workload below has recorded expectations and results on the pinned baseline; selected
revision differences and reproducible defects are explicit. The architect approves the contract and critical package
list; the reviewer approves the baseline evidence before extraction begins.

### SETUP 04 Sequential extraction and tier releases

Owner: Go developer for extraction and consumer integration, independent Go reviewer for release evidence, architect
for contract changes, technical writer for current docs and extraction ledger. Dependency: SETUP 03 sign-off.

Port small slices using the admission heuristic, mapping each dependency closure to the contract and ledger.
Shared contract planning may look ahead; do not port the next tier until the current tier passes its architect and
independent reviewer gates. Record the exact qualified engine commit, SemSource SHA, configuration, and admitted tier.
Keep the accepted lower profile deployable and retain every promised lower-profile regression suite at each stage,
including after model/provider changes. Test profiles independently: this does not require identical rankings or
simultaneous BM25/neural operation. Hybrid retrieval needs separate design. Measure budgets with real workloads.

#### SETUP 04A Tier 0: Graph foundation (provisional slice)

Qualify Graphable ingestion, typed identities/triples, metadata, vocabulary, mutations, indexed exact queries,
references/body retrieval, and minimal deterministic fusion, including SemConnect and context/lifecycle/restart cases.
SemSource currently always configures graph-embedding: prove a true no-embedder composition and explicitly admitted
graph-only interfaces, not unsupported NL verbs. NATS remains required; no model service is needed. Compile-time
clustering/LLM imports may need a minimal reviewed separation; any dormant bridge needs an owner and exit condition,
not implicit higher-tier admission. Architect contract approval and independent evidence review unlock tier 1.

#### SETUP 04B Tier 1: Lexical retrieval (provisional slice)

Add BM25 with known-answer retrieval and ranking cases, scope-before-limit behavior, result caps, stale/update
semantics, index rebuild, and restart. Qualify the retained SemSource lexical paths with no model service, retaining
all tier 0 guarantees. Architect and reviewer acceptance unlock tier 2 extraction.

#### SETUP 04C Tier 2: Neural retrieval (provisional slice)

Add qualified real embeddings with recorded model/configuration identity, paraphrase cases, mixed code/docs scope,
warm/cold paths, deadlines, provider unavailability, and recovery. Specify and test fallback or explicit `Deferred`
outcomes without weakening tiers 0 and 1. Only this stage qualifies the selected full SemSource neural/default
workload; generator-backed answers or community enrichment still need separate admission.

Pass evidence: the declared tier's workload and SemEngine dogfooding pass through admitted SemSource human/agent
interfaces; current docs/tests explain the behavior. Release claims name the tier and tag the exact tested commit and
consumer baseline. Tier 0/1 releases need no semembed run; tier 2 releases and embedding integration changes require
bounded real-semembed qualification. Release checks must verify applicable evidence, never bypass it during tagging.

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

Verification follows the admitted tier: deterministic graph checks start at tier 0, lexical checks at tier 1, and
real-semembed qualification at tier 2. Model services must not block tier 0 or 1 admission or release. All previously
promised lower-tier suites remain required. No benchmark targets or latency promises are assumed before measurement.
Cover duplicate-name anchor selection and mixed code/docs scoped before limiting where each capability is admitted;
tier 2 adds actual warm/cold embedding paths with recorded model/configuration identity and stable acceptance criteria.
Missing bodies, partial hydration, stale readiness, or transport faults must not become confident not-found answers.
Distinguish healthy lag with freshness information from `Deferred`: no usable answer, rather than an absence finding.

## Known risks and what to carry forward

- SemSource's current smoke checks establish startup and route reachability, not complete semantic correctness.
  The known-answer workload must supply the missing evidence.
- SemStreams [RPC stream collision issue 1143][rpc-issue] is open. Reproduce it against the chosen revision and assess
  whether the retained request paths need the fix before treating it as a current SemEngine defect.
- SemSource [physical deletion issue 210][delete-issue] is open. Physical removal differs from retained stale history;
  require hard removal only if admitted by the consumer contract, and reproduce the relevant behavior.
- Fusion risks include [truncation nondeterminism 621][fusion-truncation] and
  [impact counts without names 603][fusion-impact]. Reproduce retained paths before carrying these as current defects.

Carry reproduced defects and scoped unresolved risks with compact evidence and source links.

## Deferred work and decisions

Defer cloud infrastructure, Kubernetes, Terraform, fleet management, agentic runtime features, sister-project release
matrices, and extra domain skills or approval layers until the retained workload demonstrates a concrete need.
Keep SemStreams as reference until the workload qualifies and remaining ownership resolves; defer SemTeams migration.

[semstreams]: https://github.com/C360Studio/semstreams/tree/5457b3458936f668b71d2fea061f67f8d7d01e67
[semsource]: https://github.com/C360Studio/semsource/tree/34bda6406fb06fd723a040988647b204204a1583
[semconnect]: https://github.com/C360Studio/semconnect/tree/d0d06e00bf05a545f30ceea798db1c2b1ee47d4f
[actions-security]: https://docs.github.com/en/actions/reference/security/secure-use
[go-security]: https://go.dev/doc/security/
[go-context]: https://pkg.go.dev/context
[compose-projects]: https://docs.docker.com/compose/how-tos/project-name/
[rpc-issue]: https://github.com/C360Studio/semstreams/issues/1143
[delete-issue]: https://github.com/C360Studio/semsource/issues/210
[graph-tools]: https://github.com/C360Studio/semstreams/issues/1422
[fusion-truncation]: https://github.com/C360Studio/semstreams/issues/621
[fusion-impact]: https://github.com/C360Studio/semstreams/issues/603
