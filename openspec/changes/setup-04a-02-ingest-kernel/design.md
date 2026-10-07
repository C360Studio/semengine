# Design: setup-04a-02-ingest-kernel

Status: **draft, round 6.** Round 2 answered the 17 findings of the pre-owner design review's round 1 and went to the
owner with questions A–F. Round 3 applied the owner's rulings of 2026-10-07 on B, C, E and F (#91 comments 6035429806
and 6035477895) and the pin probe P-9 that ruling B asked for (`inventory.md` §8). Round 4 applied #77's ruling
(#77 comment 6035317931), which answers question A: D7 is rewritten and `lifecycle-suite` gains a delta. The owner
accepted round 4 on #91 (2026-10-07; task 1.4). Round 5 folds the owner-ordered pre-port graph design audit (PR #93
comment 6036316289): the rulings on #97, #98 and #99, and the port-refactors #100–#104, #106's pattern and #111 item
1 (D1, D1a, D11 and D15–D22; `inventory.md` §9). Round 6 applies the owner's rulings A–F on the round-4 review of
round 5 (#91 comment 6037287957) and that review's findings: the replace is keyed on (subject, predicate, source),
mutation-lane births keep the pin's no-hierarchy behavior, derived statements take the triggering message's time
(D15, D21), `graph/structural`'s move is ruled (D1a, D11), the `LifecycleManager` adopter sites are named (D17), and
the readiness envelope loses its legacy fields and gains `published_at` here (D16; `inventory.md` §9.15). Round 6
needs the review's re-check and the owner's acceptance (tasks 1.11, 1.12); nothing after round 4 is approved yet.

Shorthand, defined once:

- **D*n*** is a decision below. **Foundation D*n*** is a decision of the 04A foundation design
  (`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md`); **foundation (a)–(h)** are the owner's
  rulings on it, made on #9 on 2026-10-01 and listed at its lines 710-731 (for example (a): `pkg/lifecycle` and
  `pkg/projection` are the durable-execution primitive). **Change 1 D*n*** is a decision of
  `openspec/changes/archive/2026-10-05-setup-04a-01-floor/design.md` (PR #48).
- **03B** is `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md`. Its owner questions were
  ruled on #8: **Q4** (agentic and gateway packages are cut from the engine, and each cut is tracked), **Q9** and its
  extension in #8 comment 5932313950 (the critical packages held to 80% coverage, 03B D10), **Q12** (each repair row's
  proof is green before its package is admitted), **Q13** (SemStreams PR #1437's graph-ingest half is taken up with
  the graph-ingest repair rows), **Q16** (`pkg/lifecycle` is kept) and **Q18** (settlement order and recovery per
  storage class). Its invariants **I1–I10** are at its lines 766-791; the ones cited here are I2 (one reserved-subject
  declaration), I3 (generation-aware replay guard), I5 (commit classification), I6 (conditional reconcile), I7
  (accepted is not durable), I8 (no SemEngine file imports SemStreams) and I9 (settlement order).
- **#19 Q6** and **#20 Q7** are the owner's rulings on those issues (2026-10-01).
- **SS#*n*** is a SemStreams issue; **P-*n*** is a pin probe and **§*n*** a section of `inventory.md`.
- A **service** is an owner the engine starts and stops with a `Start` that can fail; it is run through the lifecycle
  suite. **Background work** is a goroutine that outlives the call that started it, in code that is not a service; it
  takes one of the three shapes of the `background-work` spec (`Run(ctx)`, a joining `Close()`, `Shutdown(ctx)`).
- A **guard record** is graph-ingest's applied-sequence record (`keyed_ingest.go:75` — `func guardKey(entityID, stream
  string) string {`): the last stream sequence applied per entity and stream, kept in memory and in a key-value
  bucket so a redelivery is dropped. A **stream generation** is one lifetime of a JetStream stream: a stream deleted
  and created again starts a new generation whose sequences restart at 1. **Settlement** is what a consumer does with
  a delivered message: acknowledge, ask for redelivery, mark in progress, or terminate
  (`natsclient/delivery_settlement.go`). The **owner-lifecycle state** is a component's one-shot `Start`/`Stop`
  bookkeeping (SS#1411; §4.2), not the guard record.
- **#97–#111** are the issues of the 2026-10-07 pre-port graph design audit (PR #93 comment 6036316289); its
  "port untouched" list is in that comment and is kept as it is here.
- A **write lane** is a path by which a change reaches graph-ingest's entity bucket: the stream lane (a `Graphable`
  message on an input stream), the mutation lane (a `graph.mutation.*` request), or the in-process lane (a call from
  inside graph-ingest, which hierarchy inference uses). A **write mode** is what the change does to the stored
  entity: create, replace, conditional replace, append or delete (D15). A **statement** is one stored triple with its
  metadata (`Source`, `Timestamp`, `Confidence`, `Context`).

## Purpose and admission

Foundation D2 row 2 and issue #91 fix the scope, as the owner's rulings amend it: of the 17 packages of
closure(graph-ingest) less change 1, the 14 left once `pkg/worker` (ruling E) and `internal/componentadmission` (ruling
C) are not ported and `graph/structural` moves to change 7 with its readers (D1a), one of them, `graph/inference`, as
its hierarchy slice (#97); nothing of `graph/llm` or `model/wire` (D1a); the repair rows foundation D7 places here
(issues #15, #19, #20, #29 and #33, the `graph` and graph-ingest half of #16, settlement, Q13 and SS#1411); the
port-refactors of the pre-port audit (#100–#104, #106's pattern and #111 item 1; D15–D21); and the deltas of foundation
D10.2. The claim: **graph-ingest runs as a component
under the lifecycle suite, with no boot path** — built through its own factory, started against `natsfixture`, and
driven by `lifecycletest.Run` with a failing factory whose broker refuses the connection. The current `lifecycle-suite`
requirement "Observe adapter contract" already binds it ("A service ported from the pin SHALL be run through the suite
via a test-side adapter in its package"), so graph-ingest's suite run needs no new requirement. The `lifecycle-suite`
delta of this change writes the #38 exception #77's ruling granted, and the public rollback helper's contract (D7).

Admission gates: `task verify` green, including `cover:check` with this change's targets at the one 80% floor (D11);
graph-ingest green under the lifecycle suite; every helper that runs background work green under its
`background-work` tests (D5); each repair row's proving test green (D8); the ledger rows valid under `task
ledger:check`. Owner rulings this change follows: #9 items 1–9; foundation (a)–(h); change 1's rulings 1.6 and 1.7
(services get the suite, helpers get the three background-work shapes); #9 comments 5968830525 (surface audit) and
5972208367 (generic payload); #69 (outward-facing names, 2026-10-06); #19 Q6, #20 Q7; Q12, Q13, Q16, Q18 on #8; #91
comments 6035429806 (E, F) and 6035477895 (B, C); #97, #98 and #99 (comments 6036308092, 6036308354 and
6036308600); the audit's port-refactors, owner-ordered in PR #93 comment 6036316289; rulings A–F on the audit's
review (#91 comment 6037287957).

**Order with #92.** PR #92 (claim for #69, head `aceb2be`) renames the floor's outward-facing names to `semengine`.
It changes 25 files (`gh pr view 92 --json files`), one of which this change also edits:
`docs/admission-ledger.yaml`. **#92 merges first.** #92 edits the `vocabulary`, `metric`, `pkg/cache` and
`natsclient` rows (hunks at base lines 518, 614, 923, 1299, 1329, 1426, 1701); this change edits the
`internal/lifecyclecleanup/lifecyclecleanup.go` file row (`:148-162`) and adds package rows before the `agentic`
`defer-exclude` row (`:2173`), touching none of #92's rows. Task 1.4 merges `origin/main` into this branch after #92
merges (never a rebase), re-reads #92's final file list, and every ledger task (5.1) runs on top of it; `task
ledger:check` is the gate. Every metric this change ports registers under `semengine` from its first commit (D4).

## Decisions

### D1. Packages, destinations and ledger verdicts

Port order is import order within the set, leaves first, as change 1 D1. Destinations follow the standing rule
(change 1 D5, #9 comment 5953295358): public if a starter consumer imports it or a public signature names its type;
otherwise `pkg/<name>` moves to `internal/<name>`.

| Level | Package | Destination | Why public / internal | Verdict |
| --- | --- | --- | --- | --- |
| 0 | `internal/lifecyclecleanup` | `pkg/lifecyclecleanup` | public by #77's ruling (comment 6035317931); SemTeams' components call it | adapt (D7: public home, a nil rollback refused) |
| 0 | `types` | `types` | consumers import it (54 + 20 sites) | carry |
| 0 | `model` | `model` | consumers import it (3); `component.Dependencies.ModelRegistry` names `model.RegistryReader` | carry: whole until #32 decides the seam (D6) |
| 0 | `model/wire` | — | nothing ported here reads it (D1a, P-9) | not ported; `defer-exclude` row naming change 7 |
| 0 | `graph/llm` | — | nothing ported here reads it once `graph/inference` is its hierarchy slice (D1a, P-11) | not ported; `defer-exclude` row naming changes 4 and 7 |
| 0 | `storage` | `storage` | consumers import it (20) | carry |
| 1 | `pkg/worker` | — | no reader once `BoundedDispatcher` goes (§2) | not ported (ruling E); `defer-exclude` row |
| 1 | `graph` | `graph`, with its catalog moved to `graph/kvcatalog` and its readiness computation to `graph/readiness` (D16) | consumers import it (72 + 11) | adapt (D3, D6, D9, D15, D16, D18, D20) |
| 2 | `graph/readiness` | `graph/readiness` | consumers import it (2) | adapt (D4, D5, D6, D16) |
| 2 | `graph/structural` | — | its readers are the detectors and graph-clustering, both in change 7 (D1a, P-11) | not ported in change 2; `defer-exclude` row naming change 7 |
| 2 | `internal/componentadmission` | — | its only reader is the token parameter ruling C removes (D14) | not ported (ruling C); `defer-exclude` row |
| 2 | `internal/graphmutation` | `internal/graphmutation` | internal at the pin | adapt (D6, D9) |
| 2 | `pkg/dispatch` | `internal/dispatch` | no consumer import; no public signature names it | adapt (D3, D4, D5, D6; its own `ErrStopped`, ruling E) |
| 2 | `storage/storeregistry` | `storage/storeregistry` | consumers import it (4) | adapt (D2, D6) |
| 3 | `graph/inference` | `graph/inference` | `processor/graph-clustering.Config` names `inference.Config` (`component.go:77`, change 7) | adapt: the hierarchy slice only (#97, D1a; D6, D9, D15, D21) |
| 3 | `pkg/projection` | `pkg/projection` | consumers import it (11 + 12) | repair-before-port (#19, #20; D8; D15) |
| 4 | `pkg/lifecycle` | `pkg/lifecycle` | semteams and semboids import it (12) | adapt (D3, D5, D15, D17) |
| 5 | `component` | `component` | consumers import it (76 + 23) | adapt (#29, D6, D9, D14, D17) |
| 6 | `processor/graph-ingest` | `processor/graph-ingest` | consumers import it (3 + 8) | repair-before-port (#15, #16, #20, settlement, Q13, #33, SS#1411; D8, D13, D15, D19–D22) |

New, not from the pin: `internal/lifecycleguard` (D13), recorded on graph-ingest's row as its SS#1411 repair item, and
`graph/kvcatalog` (D16), which holds two files and one function of the pin's `graph` and is recorded on the `graph`
row. `component/lifecycle_test_suite.go` is not ported: its ledger row says `adapt → internal/harness/lifecycletest`;
its one in-set caller, `component/lifecycle_test_support_test.go`, leaves with it. One dependency enters `go.mod`:
`golang.org/x/net` (`model/httpclient.go:7`), at a version that does not downgrade the base's `x/text`, `x/tools` or
`x/vuln` (P-2). `github.com/sashabaranov/go-openai` does not enter: its only importer in the closure is `graph/llm`
(P-9), which is not ported.

`pkg/lifecycle` and `pkg/projection` leave closure(graph-ingest) once `component` drops its lifecycle slot (D17, P-11);
they stay in this change because the owner admitted them as the durable-execution primitive (#8 Q16; foundation (a)),
issue #24 waits for them (foundation (d)), and #19 and #20 are proven through `pkg/projection`'s client.

### D1a. The `graph/inference` slice (#97), and what it leaves out

**Ruling #97** (comment 6036308092, "accepted as recommended"): `graph/inference` ports as its hierarchy slice;
`ReviewWorker`, `http_handlers.go`, `NATSRelationshipApplier` and `ReviewConfig.LLM` do not port in change 2; the
ledger row records the slice; this design re-measures foundation D8's dormant carriage.

**The slice** (P-10, inventory §9.1): `hierarchy.go`, `container_entity.go`, and from `applier.go` only the three-line
`TripleAdder` interface that `hierarchy.go` names (`applier.go:137-141`). The pin's `doc.go` describes anomaly
detection, review and storage, which stay behind, so the slice gets a package comment that describes what it holds.
Its tests are the three `hierarchy_*_test.go` files. At the pin the slice builds alone and its tests pass under the
race detector and five shuffled runs. Its dead surface goes (D6): `DefaultHierarchyConfig`, `OnEntityCreated`,
`ClearCache`, `GetMetrics`, `GetCacheStats`. The tests that call `OnEntityCreated` (27 reads) call
`GetHierarchyTriples` and the adder they pass instead, as the `MergeEntity` tests do. The rest of the package
(`detector.go`, the three detectors, `storage.go`, `review_worker.go`, `http_handlers.go`, the appliers in
`applier.go`, `config.go`, `metrics.go`, `types.go`, `doc.go`) is ported with graph-clustering in change 7; the
ledger row names those files and #97.

**What follows, each measured** (P-11, inventory §9.2):

- **`graph/llm` is not ported at all.** Ruling B (#91 comment 6035477895) ported `client.go` because `graph/inference`
  read it (`review_worker.go:40,69,430`, `config.go:123`); the slice reads nothing of `graph/llm`. Following ruling B's
  own reasoning (port what the admitted packages read), nothing of it is ported now. Its next readers are
  `graph/clustering` and `processor/graph-query` (change 4) and graph-clustering (change 7); it is ledgered
  `defer-exclude` naming those changes and #32. Foundation D8's dormant carriage happens in no form: no `graph/llm`
  file, no `model/wire`, no `go-openai` in `go.mod` in changes 2 onward until a change that reads them. Round 4's
  `client.go` port, its package-comment rewrite, and the `doc.go` example edit are moot.
- **`model/wire`** stays as round 4 decided: not ported, `defer-exclude`, change 7.
- **`graph/structural` moves to change 7 with its readers.** Its importers at the pin are the detectors
  (`graph/inference`, four files) and `processor/graph-clustering` (three files). Options: (a) port it here as
  foundation D2 placed it: 883 lines with no importer for five changes, its six dead-surface rows judged with no
  caller in the tree; (b) **move it with its readers** (recommended): the reasoning of ruling B and #97 ("the detectors
  … ride with graph-clustering at change 7"). (b) changes order, not admission: the package stays in the admitted set
  and change 7 ports it whole. Ruled (E, #91 comment 6037287957): "`graph/structural` moves to change 7 with its only
  readers. This changes foundation D2's package list and the D10 critical list." Foundation D2 row 2 loses
  `graph/structural` and row 7 gains it; 03B D10's critical list keeps `graph/structural` at the 80% floor, gated
  from change 7, when it is ported, not from change 2 (D11).
- **Carried to change 7 with the code:** the review worker's `Shutdown(ctx)` shape and its `synctest` test (round 4
  D5), and the drop of `graph/inference.Config`'s six unread fields with `review.llm` among them (round 4 D6).
  Ruling F is re-read in D11.
- `HierarchyConfig.Org` and `.Platform` are in the slice, so D9's authority change applies unchanged.

### D2. Test files that import packages outside the set

P-3 found eight test files the foundation did not count. Each is adapted inside its package row:

- `payloadbuiltins.NewTestRegistry(t)` / `payloadbuiltins.Register(reg)` (graph-ingest: `component_fixture_test.go:66`,
  `factory_registry_test.go:122`, `merge_entity_integration_test.go:369`, `resident_stamp_integration_test.go:51`,
  `registered_type_gate_integration_test.go:78,106,122,166`; `pkg/lifecycle/harness_gate_integration_test.go`) become
  `payloadfixture.NewWithSubset(t, <the per-package RegisterPayloads the test needs>)`, D1's adopter path (#33).
- `agentic` and `agentic/research` types used as sample payloads (`component_fixture_test.go:54`,
  `registered_type_gate_integration_test.go:110-147`) become a test-registered payload type with the same indexing
  profile (`payloadfixture.RegisterTestType`). `indexing_profile_registry_test.go` asserts the indexing profile of 20
  agentic and research payloads (`:96-116`): that is the agentic domain's contract, not graph-ingest's; the file is
  `defer-exclude` with that reason, and the test that graph-ingest reads a payload's declared profile is kept through a
  test-registered type.
- `storage/storeregistry/storeregistry_test.go:30` — `var _ fusion.StoreResolver = (*storeregistry.Registry)(nil)` —
  is removed here and returns with `pkg/fusion` in change 5 (a `port-refactor` note on both rows).

### D3. The fixture client: one helper in `natsfixture`

43 sites in five packages use the pin's `natsclient.NewTestClient` family. Change 1 replaced it inside `natsclient` with
a package-internal helper (`natsclient/test_helpers_integration_test.go:47`): start a fixture, build a client, connect
under a bound, register a bounded `Close` on test cleanup. Options:

- (a) A copy of that helper in each package's tests: five more copies, six homes. Cost: six copies of the cleanup and
  failure-reporting rules to keep in step; a reviewer compares them.
- (b) A shared `internal/harness/clientfixture` that imports `natsclient`. The `harness-boundaries` "Import graph"
  requirement allows a harness package to import a module package only when that package "starts no goroutine and
  holds no resource beyond a call"; `natsclient.Client` holds a connection, so (b) needs that requirement changed.
- (c) Do nothing: not possible; the tests do not compile (P-3).
- (d) **One helper in `natsfixture` that takes an open function.** The caller passes a function that, given a context
  and the fixture's URL, returns the opened value and its close function; the helper opens it under a bound, fails the
  test naming the URL when opening fails, and registers the close on test cleanup under a bounded context so it runs
  before the fixture stops. `natsfixture` imports no `natsclient` and holds nothing after the call, so "Import graph"
  is unchanged. Cost: each package still writes a short open function naming the client options (no reconnects, no
  health monitor), so those three option lines repeat; the rules that matter (bounded cleanup, cleanup order, failure
  naming the URL) have one home.

Recommendation: **(d)**. It is the only option with one home for the cleanup rule the `harness-boundaries` "Bounded
cleanup roots" requirement polices, without weakening the import rule (b) would relax. Spec home: the `nats-fixture`
delta, "Connected value for a package's tests". Adoption sweep (D3 establishes a reusable primitive): natsclient's own
`newFixtureClient` (`natsclient/test_helpers_integration_test.go:47`) and the 20 later packages foundation D4 lists;
one tracking issue (task 6.3). The establishing change converts only its own five packages.

### D4. Metrics: one collector per key, `semengine`, no global registry

Every live registration moves to `metric.RegisterOrGet` and uses the collector it returns (the AGENTS.md rule; ledger
`metric` row). Live sites: `graph/readiness/gauges.go:149-157` (7), `KeyedPool`'s four (`keyed_pool.go:441-460`),
graph-ingest (`component.go` 12, `poison_inventory.go` 1). The dead ones go with their surface (D6):
`component/metrics.go` (4); `graph/inference/metrics.go` (7) and `pkg/worker/pool.go:133-139` (7) are not ported
(#97, ruling E).

What a caller observes:

- Every collector this change registers is named `semengine_*` where the pin named it `semstreams_*` (#69 ruling,
  2026-10-06). Metrics with no namespace at the pin (`dispatch_queue_wait_seconds`) keep their names: #69 renames the
  `semstreams` word, it does not add a namespace.
- graph-ingest's collectors are registered on the `component.Dependencies.MetricsRegistry` it is built with. Two
  graph-ingest instances on one registry share each collector (`RegisterOrGet` returns the held one). With a nil
  registry, graph-ingest builds its collectors and registers them nowhere; nothing is registered on
  `prometheus.DefaultRegisterer` (the pin's fallback, `component.go` 12 sites, is removed). This is a declared change:
  at the pin, a nil registry put the series on the process-global registry for the life of the process.
- Tests: `TestGraphIngestMetricsRegisterOnItsRegistry` (two registries, two components: each registry gathers its own
  series; the default registry gathers none of them); `TestGraphIngestNilRegistryRegistersNothing`; a readiness and a
  dispatch test that the returned collector, not the candidate, is the one written (a mutant that writes the candidate
  fails it). graph-ingest's gauges are set from its own state, so two instances on one registry overwrite each other:
  #75's class, recorded on the graph-ingest row and linked from #75, not fixed here.

### D5. Background work in this change takes the three shapes

Ruling 1.7 replaces foundation D6's "non-context `Stop` → `Stop(ctx)`" rows for the helpers. A helper whose surface the
audit drops (D6) needs no shape. Per remaining helper, what the caller observes, and the test (`synctest` bubble,
`background-work` "Nothing left behind"):

| Helper | Pin | After the port | What the caller observes |
| --- | --- | --- | --- |
| `dispatch.KeyedPool` | `Stop(ctx)` | `Shutdown(ctx)` | returns nil once every lane has drained and exited, or `ctx.Err()` first; with no deadline it waits for the join; a later `Shutdown` returns nil; `SubmitBlocking` after `Shutdown` began is refused with `ErrStopped`, which `internal/dispatch` now declares itself (ruling E; at the pin it re-exported `worker.ErrPoolStopped`, `pkg/dispatch/errors.go:24`) |
| `readiness.Watcher` | `Start(ctx)`, `Stop()` waiting unbounded on a goroutine doing KV watch I/O | `Run(ctx)` | the caller runs `Run` on its own goroutine; `Run` returns `ctx.Err()` when the context ends and leaves nothing running; `Read` keeps its pin behavior |
| `lifecycle.Manager.Watch`, `.WatchEvents` | return a channel; the goroutine ends when ctx ends and is never joined | a watch whose callback runs on the caller's goroutine; returning is the join | no goroutine left after return |

Dropped, so no shape: `dispatch.BoundedDispatcher` (and with it its fixed 30 s default wait, `dispatcher.go:226-231`)
and `readiness.Set` (D6); `worker.Pool[T]`, whose second `Stop` after a timed-out one panics at the pin (P-6), is not
ported (ruling E). Not ported in change 2, so their shape is change 7's: `inference.ReviewWorker` and
`inference.NATSAnomalyStorage.Watch` (#97, D1a). `lifecycle.Manager.Watch` and `WatchEvents` have **no
reader in the admitted set** (§2: `Watch` is read only by `processor/gated-dag/executor.go:119` and
`gateway/lifecycle-gateway/handlers.go:475`; `WatchEvents` by nobody); they are adapted because D6 keeps
`pkg/lifecycle`'s surface whole (K1), and the shape change is their only change.

The developer chooses locks and join order, settled by a failing-first test under `-race`. A nil context is refused at
the call (an error). Callers inside this change adapt in the same commit (graph-ingest's `KeyedPool` and readiness
use). Callers in later changes are the `class:port-refactor` rows foundation (h) placed (changes 5 and 7: the
readiness watcher's start at `processor/graph-clustering/component.go:1512` and `:1527` and stop at `:1260` and
`:1266`; `fusionnats/client.go:140` (start) and `:103` (stop)). The review worker's start and stop
(`processor/graph-clustering/component.go:2450`, `:1232`) adapt with the review worker itself in change 7.

### D6. Surface audit dispositions

From §2, read by type (the appendix lists all 186 with a disposition). Admission is per package; only surface nothing
reads is removed (#9 comment 5968830525).

- **Not ported by ruling:** `pkg/worker` whole (ruling E), and with it its appendix rows
  (`worker.WithMetricsRegistry`, `Pool.SubmitBlocking`); `internal/componentadmission` whole (ruling C, D14);
  `graph/inference` outside its hierarchy slice (#97, D1a), whose 21 appendix rows are audited again in change 7;
  `graph/llm`, `model/wire` and `graph/structural` (D1a), the last with its six appendix rows.
- **Dropped (the rest of the 134 plus the transitive drops of §2):** among them `component.NewProcessorMetrics` and
  `ProcessorMetrics`; the 20 `graph/errors.go` sentinels;
  `graph.IncomingEdges` and its methods;
  `component.{GetString,GetInt,GetBool,GetFloat64,ValidateJSONSize,ValidateComponentConfig,
  ValidateAndPersistComponentConfig,IsLifecycleComponent,Registerable}`, `LogLevel*`, `LogEntry`, and
  `component/config_validator.go` whole; `component.Registry.Snapshot`; `component.MergePortConfig` (read by six
  non-admitted pin files only); `dispatch.BoundedDispatcher` with `New`, `Config`, `Deps`, `ErrQueueFull`, and
  `KeyedPool.Submit`, `KeyedPool.Stats`; `readiness.Set` with `NewSet`, `Dump`, `Verdict`; in the `graph/inference`
  slice, `DefaultHierarchyConfig`, `HierarchyInference.OnEntityCreated`, `ClearCache`, `GetMetrics` and
  `GetCacheStats`; `internal/graphmutation.IsCommitUnknown`; `storage/storeregistry.Registry.Instances`.
  `readiness.Set`'s one reader, `gateway/graph-gateway`, is deferred as consumer-owned (03B D4), not abandoned, and
  #110's fix shape needs it back ("`Health()` reports from the same `readiness.Set` the bucket is written from",
  change 7). The readiness ledger row's `known_risks` names both, so `Set` returns as returning surface with #110 and
  the drop forecloses nothing.
- **Removed by a port-refactor, not as dead surface** (each read by something, each with its decision): `graph`'s
  `events.go` (D16), the `TOOL_CALL_OUTCOMES` catalog row and constant (D16), `EntityState.Version` (D15),
  `UnwrapQueryResponse` (D18), `component.Dependencies.LifecycleManager` (D17), and graph-ingest's suffix index with
  the `graph.ingest.query.suffix` verb (D19).
- **`Component.MergeEntity`** (`processor/graph-ingest/component.go:2021`) is a one-line wrapper over
  `mergeEntityOnLane(ctx, entity, false)` with no non-test reader. Its 27 test call sites in 10 files exercise the live
  merge path (ADR-072 merge, the write gate, the poison proofs): `batch_integration_test.go` 5,
  `merge_entity_integration_test.go` 8, `indexing_profile_test.go` 5, `merge_entity_write_gate_test.go` 3, and one
  each in `authority_gate_integration_test.go`, `canonical_mutations_test.go`, `entity_state_preio_contract_test.go`,
  `merge_entity_bench_test.go`, `poison_scoping_integration_test.go`, `poison_scoping_test.go`. They are in-package
  tests, so they call `mergeEntityOnLane(ctx, entity, false)` directly; none is deleted.
- **Kept with a reason:** K1 — `pkg/lifecycle`'s and `pkg/projection`'s unread surface (23 rows, including
  `Manager.Watch`, `WatchEvents`, `History`, `List`, `Children`, `MutationClient.Create`/`Append`/`Delete`): the two
  packages are the durable-execution primitive by owner ruling (#8 Q16; foundation (a)), and #24, unblocked by this
  change (foundation (d)), decides their surface. K2 — `model`'s agentic-only surface: #32 (change 7) decides the seam;
  cutting it now is the blind cut Q4 forbids. K3 — `graph/inference.RegisterPayloads`, `pkg/lifecycle.RegisterPayloads`:
  the per-package registration D1 makes the consumer's call (#33). K4 — `ConfigSchema` on graph-ingest and on
  `SimpleMockComponent`: required by `component.Discoverable`, which consumer components implement; whether the
  component contract keeps `ConfigSchema` (no admitted package calls it) is filed as a follow-up issue (task 6.3), not
  decided in a port.
- **Config (b).** graph-ingest refuses an unknown configuration key at construction, naming the key
  (`TestCreateGraphIngestRefusesUnknownKey`; at the pin the key is ignored). It uses the same strict-decoding shape
  (`DisallowUnknownFields`) as the pin's `inference.RejectUnknownKeys` (`graph/inference/config.go:249`; ADR-054),
  rather than a second one; that function is in `config.go`, which ports in change 7 (#97).
  `IngestLanes < 1` keeps the pin's clamp to 1, a declared degrade, now with a test that an explicit 0 builds a
  one-lane component. Each of graph-ingest's four fields has a test that fails when the field is ignored.
  `graph/inference.HierarchyConfig` is built in code by graph-ingest (`component.go:1428-1440`), never decoded from
  operator configuration in change 2, so its JSON tags carry no configuration surface here. `graph/inference.Config`
  and its six unread fields, `review.llm` among them, are change 7's (D1a).
- **Described, not implemented (c):** `processor/graph-ingest/TEST_DISPUTE.md` is not ported. `graph/README.md` says
  the package is "types and interfaces only" while it holds the catalog and readiness computation; it is rewritten
  for what the root holds after D16. The four READMEs of ported packages (`component`, `graph`, `types`,
  `processor/graph-ingest`) are read claim by claim in each port task; a claim no code implements is removed or the
  gap filed.
- **Generic payload:** no `NewGenericJSON` or `GenericJSONPayload{` construction in the 17 (search over §2's file
  list, empty; the set this change ports is a subset), so no `adapt` item under #9 comment 5972208367. `events.go`,
  whose payloads are `map[string]any`, is not ported (D16).

### D7. Failed-start rollback: the public helper and the #38 exception (#77)

**The ruling.** #77 comment 6035317931: a component cleans up its own failed start, using a helper SemEngine makes
public; the component manager and the service manager stay as the second line; the #38 exception is granted, and #38
binds only components SemEngine ports. The helper's public home and name, and whether it keeps accepting a nil
callback, are this design's to settle. PR #93 closes #77 (comment 6035358884). This section settles the three, writes
the exception as a `lifecycle-suite` delta, and reconciles the `natsfixture` copy. #77's passed inventory (comments
6024786639 and 6024787123) is taken as given; its sections cited below as "#77 §n".

**The helper at the pin** (`internal/lifecyclecleanup/lifecyclecleanup.go`, 38 lines, standard library only):
`func RollbackFailedStart(parent context.Context, rollback func(context.Context) error) error` (`:17`) runs `rollback`
synchronously under `context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)` (`:12`, `:33`), so the
rollback keeps the parent's values but not its cancellation or deadline, and returns `errors.Join(rollbackErr,
ctx.Err())` (`:36-37`). A nil parent is refused with an error (`:26-27`); a nil `rollback` returns nil (`:29-30`).
34 production call sites at the pin, all in components and the two managers; none passes nil (#77 §3, §2d).

**Its consumer.** SemTeams' proving case (#77 §8): it compiles "the exported helper plus SemEngine's component
interface" and tests a component directly, with no manager, from change 2 on. The agentic components SemTeams is taking
over call it at the pin as `lifecyclecleanup.RollbackFailedStart(…)` (#77 §3, rows 1-2 and 9-13).

#### D7.1 Home and exported name

Options, each keeping the function name `RollbackFailedStart` (the ruling's premise: "nothing in SemEngine's ported
managers needs adapting if the helper keeps its name when it moves"):

- (a) **`pkg/lifecyclecleanup.RollbackFailedStart`.** The pin's package moved from `internal/` to `pkg/`, unchanged
  in name. SemTeams changes one import path per calling file and no call text. It follows the standing destination
  rule (change 1 D5, #9 comment 5953295358): a package a starter consumer imports is public, and SemEngine's public
  utility packages already live under `pkg/` (`pkg/errs`, `pkg/retry`, `pkg/projection`). It imports only the standard
  library, so a caller pulls in nothing else, and `internal/lifecycleguard` (D13) can call it without importing
  `component`. Cost: one more public package, holding one function.
- (b) **`component.RollbackFailedStart`.** No new package; `component` already holds the component interface the
  helper serves, and every pin caller already imports `component` (no import cycle: `go list -deps ./component` at the
  pin reaches none of the callers' packages). Cost: SemTeams changes the call text as well as the import, at every
  call; `internal/lifecycleguard` would import `component`, a 5,585-line package, for one function; and the helper,
  which is about any lifecycle owner, sits in the package about components.
- (c) **A new top-level package** (for example `lifecycle` or `failedstart`). Cost: a new name to learn; `lifecycle`
  would be confused with `pkg/lifecycle`, the workflow-entity layer (`pkg/lifecycle/doc.go:1-3`; #77 §13 Q3); the
  module's top-level packages are the pin's domain packages, not helpers.
- (d) **Do nothing (keep it internal).** Not open: the ruling makes it public.

Recommendation **(a)**: `pkg/lifecyclecleanup.RollbackFailedStart`. It is the only option under which SemTeams' change
is the import path alone, as the ruling describes, and the only one that keeps the guard free of `component`. The name
`RollbackFailedStart` is unused in SemEngine (`git grep -n RollbackFailedStart -- '*.go'` on the branch: empty; the
only `lifecyclecleanup` mention is the provenance comment `internal/harness/natsfixture/rollback.go:16`). Later
changes that port a caller rewrite its import through the ledger's destination (`harness-boundaries`, "Comparison with
the pin", scenario "Import of a package the ledger moved").

**With D13 and ruling C.** The guard (D13) stays `internal/lifecycleguard`, owns the one-shot state, and calls
`lifecyclecleanup.RollbackFailedStart` on a failed `Start`; SemEngine's own components get the five facts #77 §10 lists
(record cleanup pending first, call the helper synchronously with `Start`'s context, join its result, clear the record
only on nil, release in `Stop`) from the guard. An external component gets them from the helper's doc comment, which
states all five: the cost #77 §10 names, which the owner accepted ("Accept that SemEngine can check the helper and its
managers in its own repository, while only SemTeams' tests can check that SemTeams' components call the helper", asked
in #77 comment 6024793500, ruled in 6035317931). Making the guard public as well would remove most of that cost, but it
has no consumer who asked for it (D13 option (c)); it is not proposed. Ruling C is unaffected: the helper names no
internal type, so `TestPublicSignatures` passes with no exception.

#### D7.2 A nil rollback callback

At the pin a nil `rollback` returns nil (`:29-30`), pinned by `lifecyclecleanup_test.go:32`
(`TestRollbackFailedStartNilRollback`). What a caller observes: a component whose `Start` failed and that passed nil
gets "rollback succeeded", although no rollback ran; it then clears its cleanup-pending record (#77 §10, fact (d)), and
whatever it acquired is held with no record, so no later `Stop` releases it. That is a silent success on a failure
path, which the developer contract forbids ("A failure path fails closed").

Options: (a) keep the pin's nil-returns-nil; (b) **refuse it: return an error naming the nil callback**, as the nil
parent is refused (`:26-27`); (c) panic. Recommendation **(b)**. A caller observes `RollbackFailedStart(ctx, nil)`
returning a non-nil error that names the nil rollback, which it joins into `Start`'s error and which keeps its record
set, so the failure is reported and a later `Stop` still runs. No pin caller passes nil (#77 §3), so no ported caller
changes. Cost: a declared change from the pin (an `adapt` item on the row); the carried test
`TestRollbackFailedStartNilRollback` is inverted to expect the error, written first and failing on the pin's code; an
external caller that passed nil on purpose now gets an error; none is known (all 34 pin callers, the agentic ones
SemTeams is taking over included, pass a closure or a method value, #77 §13 Q4). (c) is rejected: a panic in a failing
`Start` is the shape the lifecycle suite does not count as a refusal ("Portable floor").

The rest of the contract is the pin's: the five-second budget per call, fresh, synchronous and cooperative (#77 §11,
behaviour 2; the ruling keeps "a fresh five-second budget per component"); a terminal finalization budget, which
`background-work` "No fixed shutdown timeout" allows. Its spec home is the `lifecycle-suite` delta, "Failed-start
rollback helper".

#### D7.3 The #38 exception, as a spec delta

The current "Portable floor" requires a failed `Start` to hold nothing, and its failed-start check fails the honest
"rollback failed, still held" branch at either boundary (#77 §2c rows B and F). The owner granted the exception for
ported components (comment 6035317931, item 2). The `lifecycle-suite` delta:

- modifies "Portable floor" to say that its failed-start check judges a failed `Start` whose own cleanup succeeded,
  and that the branch where that cleanup fails is governed by the new requirement, not by this check;
- adds "Failed start whose own cleanup fails": a component SemEngine ports MAY return from a failed `Start` still
  holding what its cleanup could not release, provided its error reports both the start failure and the cleanup
  failure, and it keeps what is left on record so that a later `Stop` tries again; each such component proves it with
  its own test, not the shared check; components outside this module are proven by their own tests;
- adds "Failed-start rollback helper": the D7.1 and D7.2 contract.

This replaces round 3's `known_risks` treatment: graph-ingest's retained branch is now specified behavior, proven by
the pin's own test carried on the in-package adapter,
`TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` (`processor/graph-ingest/lifecycle_owner_test.go:134`),
tagged with the new requirement. The suite's failed-start check runs on graph-ingest with a failing factory whose
cleanup succeeds, and passes as written.

#### D7.4 The `natsfixture` copy (foundation D9)

Foundation D9: "`internal/lifecyclecleanup` keeps its harness-helper file row and gains a package row; the two homes
are reconciled in change 2". The copy is `natsfixture.rollback` (`internal/harness/natsfixture/rollback.go:18`, 15 s
budget at `:12`), adapted from the pin file (ledger file row `internal/lifecyclecleanup/lifecyclecleanup.go`, base
`docs/admission-ledger.yaml:148-162`, `known_risks`: "production home deferred until a production consumer exists").
Options:

- (a) `natsfixture` calls the public helper. Cost: the `harness-boundaries` "Import graph" requirement says
  `natsfixture` "SHALL import no package of this module outside `internal/harness/`", so that requirement changes; and
  the helper's fixed five seconds is shorter than Docker teardown under concurrent package cleanup, which the copy's
  15 s exists for (`rollback.go:9-11`), so the helper would gain a budget parameter whose only caller is the harness
  (surface with no production consumer).
- (b) **Two homes, reconciled on the record.** The production home is `pkg/lifecyclecleanup`; the harness keeps its
  unexported copy for the two stated reasons. The file row's `known_risks` changes from "production home deferred" to
  name the production home and the two reasons; the package row names the harness copy. Both refuse a nil parent and
  keep the parent's values; each has its own test (`internal/harness/natsfixture/fixture_test.go:294-309` for the
  copy). The copy has no nil-callback case: its callers are its own package's.
- (c) Move the copy's budget into the public helper and delete the copy: (a)'s costs, plus a fixture timeout chosen by
  production code.

Recommendation **(b)**. It changes no requirement and adds no surface; the reconciliation foundation D9 asked for is
that each home names the other and why they stay apart.

### D8. Repair rows and what proves each

| Row | Behavior after the change (spec home) | Proving test (failing first on the pin's code) |
| --- | --- | --- |
| #20 (Q7) | a KV `Update` failure that is not a revision conflict or not-found reaches the caller as commit-unknown, through graph-ingest's reply and `projection.MutationClient` (`projection-mutation`) | `natsfixture.FaultKV.FailAfter(Update, timeout)` injected into graph-ingest's entity bucket by a `_test.go` setter (foundation D4-A); the typed client returns `CommitUnknown`; conflict and not-found still return not-committed |
| #19 (Q6) | `ReconcileMutation` carries an expected revision; a stale one returns revision-conflict naming both revisions and changes nothing (`projection-mutation`) | read at R, another writer commits R+1, reconcile at R → conflict, entity unchanged at R+1; reconcile at the current revision → applied |
| #15 | the guard record keys on the stream generation; within one generation a sequence not newer than the last applied is stale; a new generation never suppresses current source state (`graph-ingest-recovery`) | memory stream + `Fixture.Restart` + file-backed guard bucket: re-ingestion at lower sequences after the restart is applied and queried back; a redelivery within one generation is acknowledged without change |
| Settlement (Q18) | graph-ingest acknowledges only after its effect and durable guard stamp are committed; a long apply signals progress so it is not redelivered (`graph-ingest-recovery`) | the process-kill test below; a long-apply test; a durable-record failure test (`FaultKV.FailBefore` on the guard bucket's write) |
| Q13 (SS PR #1437 head `0ea823a6`) | a payload that fails validation or panics in its own code on the Graphable lane is poison — counted, logged, terminated — never a redelivery loop (`graph-ingest-recovery`) | the PR's `fact_lane_fence_test.go` and `_integration_test.go`, adapted to a test-registered payload type (they import `agentic`) |
| #16 (graph half) | the `graph.ingest.query.*` verbs are declared once, in `graph`'s verb table, by their responder (D20); `graph.mutation.>` keeps its one home in `internal/graphmutation/protocol.go`; no other non-test file spells one (`graph-transport-boundary`) | `TestReservedSubjectsDeclaredOnce` in `internal/harness/contract` with its sensitivity test; `TestGraphIngestServesExactlyTheDeclaredVerbs` (D20); the composition refusal is change 3's |
| #33 | graph-ingest's refusal names the per-package call, not `payloadbuiltins.Register` (`component-registration`) | factory test asserting the refusal names `inference.RegisterPayloads` |
| #29 | `component` reaches no agentic package (`component-registration`) | `go list -deps ./component` has no `agentic` path, in a contract test; I8's test already forbids a SemStreams import |
| SS#1411 | graph-ingest composes the one owner-lifecycle guard (D13); the 11 later admitted copies adopt it when ported | the guard's own `lifecycletest.Run` over a test owner built only from the guard (with a failing factory); graph-ingest's suite run; `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` on the guard-composed component |

**The settlement process-kill test.** There is no boot path, so the process is the test binary itself: `prochost.Helper`
(`internal/harness/prochost/prochost.go:43`) runs a helper function from graph-ingest's `TestHelperProcess`, as its doc
comment directs, that builds graph-ingest through its factory against the fixture URL passed in the environment, starts
it on a file-backed input stream, and parks on a signal. The window between apply and acknowledgement is held inside the
helper, not raced from outside: in the first run, the helper wraps the guard-record bucket with a `_test.go` wrapper
(set through the same in-package setter as foundation D4-A's KV setter) whose write does not return. The test then
publishes one input, waits until the entity's write appears in the entity bucket, confirms from the consumer's info that
the input is delivered and still pending acknowledgement, confirms the helper is still running (`Process.Alive`), and
kills it (`Process.Kill`). A second helper run, without the wrapper, starts graph-ingest on the same stream and buckets;
the test waits for the input's redelivery to be acknowledged and asserts the entity equals the state after one
application and nothing else changed. No production failpoint is added. Rejected: pausing the helper with `SIGSTOP` when
the entity write appears — the guard write and the acknowledgement follow within the same call, so the pause can land
after the acknowledgement and the test would need retries to catch the window.

The "delivery limit exhausted → parked and visible" scenario needs `internal/maxdelivery` (change 7). Under ruling (g)
it is not in this change's delta; the row names change 7.

How the guard record learns the generation is the developer's choice under one constraint: it is **observed from the
server**, not configured by the operator. The pin's silent degrades on this path change. No guard bucket, which only a
hand-built component can have (the bucket's acquisition failure fails `Start`, `component.go:1239-1242`), means every
message is first seen (`keyed_ingest.go:267`), now with a log line, a counter and a test. A stored value graph-ingest
cannot decode, which the pin read as first seen (`:277-278`), is refused (D21, #111 item 1).

The audit's port-refactors are D15–D21; each names its own failing-first test.

### D9. Deployment authority, names and literals

- `graph/inference.HierarchyConfig` loses its exported `Org` and `Platform` (`hierarchy.go:100-101`); the hierarchy
  inference takes the carrier `types.PlatformMeta` from graph-ingest's `deps.Platform` as a constructor argument and
  keeps it unexported. graph-ingest's two unexported strings (`component.go:772-773`) become one unexported
  `types.PlatformMeta`. `TestNoSecondAuthorityField` then passes for every live package.
- `graph/llm.EntityParts.{Org,Platform}` (`prompt_types.go:14-15`) are not ported (nothing of `graph/llm` is, D1a),
  so the check needs no exception and its requirement text is unchanged.
- `component.Registry`'s access-token parameter, which fails `TestPublicSignatures` at the pin (P-4), is removed
  (ruling C, D14), so that check needs no exception either.
- **Wire and storage names, applied under #69** (not a question: #69's ruling, "one rule for every outward-facing
  name: semengine", with no consumer needing its stored data kept, covers them). `adapt` items on the rows named:
  `graph` — `BucketSemStreamsConfig = "semstreams_config"` (`graph/constants.go:74`) becomes
  `BucketSemEngineConfig = "semengine_config"`, and the bucket description "SemStreams runtime configuration"
  (`graph/kvcatalog.go:134`, moving to `graph/kvcatalog`, D16) names SemEngine; `alertDigestDomain`
  (`graph/events.go:19`) needs no rename, because `events.go` is not ported (D16); `internal/graphmutation` —
  `InterfaceType =
  "semstreams.graph.mutation"` (`protocol.go:12`) becomes `"semengine.graph.mutation"`. The consumers who spell them
  edit them when they adopt SemEngine (§5 item e): semsource (`cmd/semsource/run.go:788`,
  `internal/cutover/buckets.go:58`, three test files), semconnect (`gateway/cs-api/component.go:227`, two deploy
  configs), semboids (`configs/flock.json`, seven integration test files), and semteams' flow configs. A running
  deployment clears its NATS volume (#69).
- `TestOneImagePin` flags six `nats:<subject>` port identifiers in `component` tests: D10.
- The 11 fixed broker addresses, 19 sleeps (the pin's 28 less `pkg/worker`'s 9), 6 skips and 20 unbounded cleanups (P-4)
  are repaired in the port as change 1 D8 did: sleeps become waits on a channel, a callback or a `synctest` bubble; the
  six skipped tests are rewritten as integration tests on `natsfixture`; addresses come from the fixture.

### D10. The image-pin check and component port identifiers

`TestOneImagePin` treats every `.go` file as one that configures Docker, and matches `nats:` followed by any letter,
digit or variable (`internal/harness/contract/imagepin_test.go:78`). `component`'s port identifiers are
`nats:<subject>` (`component/port_nats.go:14` — `return fmt.Sprintf("nats:%s", n.Subject)`); the six flagged
literals are `"nats:in"`, `"nats:out"`, `"nats:mavlink.position"`, `"nats:sensor.data"` and `"nats:test.a"` (twice).
Options:

- (a) Narrow the Go scope to files that import a container library or `os/exec`. Cost: the sensitivity case
  "digest literal in Go" (`imagepin_test.go:37`, a digest in a constant in a Go file with no imports) stops failing.
- (b) Rewrite the six expectations so no literal starts `nats:`. Cost: the expected value comes from the code under
  test, which weakens the oracle.
- (c) An inline exemption marker: SemEngine removed those (`scripts/lint-test-ports.sh:45`).
- (d) **In Go files only, match `nats:` when what follows is a digit or a variable (`${`, `$`), or `nats@sha256:`.**
  Every file type keeps its scope; `:37` still fails; `nats:in` and every `nats:<subject>` pass. Cost: in a Go file, an
  image tag that starts with a letter (`nats:latest`, `nats:alpine`) is no longer caught; that becomes review only.

Recommendation **(d)**, as a `harness-boundaries` modification with a sensitivity case for each side. The AGENTS.md row
"One NATS image pin" gains the review-only part.

### D11. Gates this change adds

- `scripts/cover-check.sh` targets, one floor (80%, `scripts/cover-check.sh:16`): `processor/graph-ingest merged`,
  `graph merged`, `graph/kvcatalog merged`, `graph/readiness unit`, `graph/inference unit`, `pkg/projection unit`,
  `internal/graphmutation unit`, `component merged`, `storage/storeregistry unit`, `pkg/lifecycle merged`. Task 5.2 is
  ticked only on a green `cover:check`. `graph/structural` is not ported here (D1a, ruling E), so it is not a target
  here; it stays on 03B D10's critical list and becomes a target in change 7. Per
  package, from P-7, P-8 and P-19 (inventory §9.10):
  - Already over the floor after the drop: graph-ingest 84.7% (its suffix code removed, D19), `graph` 80.9% (the root
    after D16; a two-statement margin), `graph/kvcatalog` 80.3% merged, `graph/readiness` 86.4% (with the readiness
    computation D16 moves into it), `graph/inference` 82.8% (the slice), `storage/storeregistry` 100%.
  - `internal/graphmutation` 69.7%, 10 statements short: tests that the client refuses a malformed append response
    (`validateAppendResponse`) and that `Reconcile` and `Delete` send their requests and read their replies.
  - `pkg/projection` 66.5%, 31 short (its unread surface is kept, K1, so the drop buys nothing): the typed client's
    `Create`, `Append` and `Delete` each sending its canonical request and reading its reply; the mutation error keeping
    its class and detail (`newMutationError`, `cloneDetail`); group and triple canonicalization; with the #19 and #20
    tests in the same package.
  - `component` 73.9%, 81 short: schema generation for nested structs and cache fields (`schema_tags.go`:
    `generateNestedSchema`, `GenerateCacheFieldSchema`, `inferPropertyFromType`), `Registry.Declare`'s refusals, and
    `validation.go`'s value checks (`validateValue`, `SafeUnmarshal`).
  - `pkg/lifecycle` 70.7% merged, 97 short (unread surface kept, K1): `Manager.Children`, `List`, `ListWorkflows`,
    `AssertRuleWritable`, a workflow definition's `validate` refusals, and `Complete`/`Fail` transitions, on
    `natsfixture`.
  - The figures are upper bounds measured at the pin; D15's rewrite of graph-ingest's write path changes its figure,
    and the port re-measures every target (task 5.2).
- **Ruling F, re-read under #97.** Ruling F (#91 comment 6035429806) put `graph/inference` outside the gate until
  change 7 because the package then entering measured 47.9% (674/1406), 451 statements short, almost all of them in
  the review worker, the detectors and their storage. That code no longer enters here (#97); the slice that does
  measures 82.8% (101/122) after its drop (P-10). So `graph/inference` joins the gate in this change: the stricter
  reading, and one the ruling's purpose allows. The rest of ruling F moves to change 7 with the code it was about: the
  tests the rest of the package owes, a test per read configuration field, and the review worker's `synctest` test.
  Change 7 then ports code into a package already under the floor, so it brings those tests in the same change (task
  6.5's tracking issue). Not an owner question: the ruling's text assumed the whole package was entering, and nothing
  measured contradicts it.
- `TestReservedSubjectsDeclaredOnce` (D8, D20), `TestNoProcessGlobalRegistration` (`metric-registry`), the `component`
  boundary test (#29 and #102, D17) and the `graph` transport test (#101, D16), each with an AGENTS.md row.
- The `revive` package-comment lint covers the public packages ported here.

### D12. Guidance that returns with these packages

Carried, adapted to SemEngine's names and rules, into `.agents/contracts/semengine-developer.md` and
`semengine-reviewer.md`: the pin developer contract's "Semantic identity and graph contracts" (`:109-126`), "Payload
registry" (`:234-242`), "State ownership and component wiring" (`:243-252`); the reviewer's "Semantic identity and
graph review" (`:155-169`), "Payload registry" (`:227-235`), "Graph and state ownership" (`:236-245`), "Component and
schema wiring" (`:246-253`). Skills carried into `.agents/skills/`: `entity-or-bucket`, `kv-or-stream`, `new-payload`
(with SS PR #1437's two added lines, carried with Q13), `query-pattern`. `orchestration-check` waits for the rule core
(change 6). Each carried rule that adds a repository-wide or agent-conduct rule adds its AGENTS.md row with what
enforces it.

### D13. One owner-lifecycle guard (SS#1411)

Foundation D7 places SS#1411 here: "change 2 lands one owner-lifecycle guard; each later owner adopts it when
ported". The inventory's census (§3 category 2, §4.2): 12 copies in the admitted set, already diverging (the
cleanup-pending flag in 2 of 12; the terminal flag spelled two ways), no doc or spec naming the idiom, and no consumer
carrying a copy. SS#1411 asks for exactly this choice. Options:

- (a) **One guard under `internal/`**, a new package `internal/lifecycleguard`, that graph-ingest composes. It owns the
  owner-lifecycle state and nothing else; rollback stays in `pkg/lifecyclecleanup` (D7), which the guard calls on
  a failed start. Cost: a new package; graph-ingest's tests that read or set the fields directly
  (`lifecycle_owner_test.go`, `test_owner_test.go`, `test_owner_child_test.go`, `component_test.go`) drive the guard
  through its methods instead (D22).
- (b) Record the copy as the idiom: each component keeps its own state machine, and the lifecycle suite (already run
  on every service) is the behavior check. Cost: 12 copies of about 60 lines kept in step by review; the divergence
  above already happened.
- (c) A public guard in `component`. Cost: exported surface with no present consumer (no consumer carries the copy),
  which category 4 removes.
- (d) Nothing in this change. Cost: a deviation from foundation D7 that the owner would have to accept.

Recommendation **(a)**. It is what foundation D7 already assigned; it is internal because every adopter is in this
module. #77's ruling made only the rollback helper public (D7); a later ruling can promote the guard without changing
what it does. Separate from `pkg/lifecyclecleanup`, so the public helper does not drag the guard's state machine into
public surface.

What a component composing the guard observes (developer chooses the methods): a second `Start` returns
`ErrAlreadyStarted` before acquiring anything; a nil or already-cancelled context is refused; `Stop` before `Start`
returns nil and a later `Start` is refused; `Stop` during an in-flight `Start` waits for that `Start` to finish, or
returns its own context's error first; a repeated `Stop` after a completed one returns nil and calls nothing; a failed
`Start` whose rollback succeeds holds nothing, and one whose rollback fails keeps the cleanup pending for the next
`Stop` (D7). Tests: `lifecycletest.Run` over a test owner built only from the guard, with a failing factory, in the
guard's package; graph-ingest's suite run; the carried pin test of D7. Spec home: the existing `lifecycle-suite`
"Portable floor" requirement, whose checks are exactly these behaviors; no delta.

Adoption sweep (D13 establishes a reusable primitive; one tracking issue, task 6.3): `service/component_manager.go:102`,
`service/message_logger.go:234` (change 3); `processor/graph-query/component.go:177`,
`storage/objectstore/component.go:49` (4); `processor/graph-embedding/component.go:298`,
`output/websocket/websocket.go:157` (5); `processor/rule/processor.go:102`, `processor/rule/cron_scheduler.go:54`
(6); `processor/graph-clustering/component.go:646`, `processor/graph-index-spatial/component.go:187`,
`processor/graph-index-temporal/component.go:196` (7). Evaluated and not adopting: `metric.Server`
(`metric/handler.go:37-38`) and `natsclient.Client` (`natsclient/client.go:154`), which are not components, track
different state (admitted HTTP requests; a connection with its own close path), and already pass the suite (change 1).

### D14. Registry methods without the access token (ruling C)

Owner ruling C (#91 comment 6035477895): "drop the token". At the pin, `component.Registry.CreateComponent`
(`component/registry.go:193-201`), `SealComposition` (`:475`) and `Snapshots` (`:842-844`) take a parameter of type
`componentadmission.Access`, an empty struct in `internal/componentadmission` (`access.go`, 8 lines) that only code in
the module can build. After the port:

- The three methods take no token; `internal/componentadmission` is not ported. Their behavior is otherwise the pin's.
  `Snapshot` (`:824`) has no reader and is dropped (D6).
- Each method's doc comment says that it exists for the component manager (`service.ComponentManager`, ported in
  change 3), that a consumer creates and composes components through that manager, and that a direct call bypasses
  the manager's bookkeeping. The comments name the manager in plain text, not as a doc link, until change 3 ports it.
  `Snapshots` keeps its `revive:disable:unexported-return` directive (`:841`) with its reason restated without the
  token.
- `TestPublicSignatures` passes with no exception; the "Public signatures name no internal type" requirement is
  unchanged.
- What a caller observes: nothing refuses a consumer's direct call. That a consumer creates components only through
  the manager is review only, with an `AGENTS.md` row (task 6.1). semboids' integration tests call
  `registry.CreateComponent` directly (`internal/zone/ingest_integration_test.go:52` and eight more sites in six other
  files; semsource, semconnect and semteams call none of the three methods). They are on SemStreams
  `v1.0.0-beta.160`, whose call takes three arguments, so on SemEngine they add the pin's `prepare` argument (nil is
  accepted, `component/registry.go:222`) and need no token.
- Callers that change: in this change, the token's 19 uses in `component`'s tests (`registry_boot_admission_test.go`
  12, `registry_integration_test.go` 4, `registry_test.go` 3) drop the argument. In later changes,
  `class:port-refactor` items: `service/component_manager.go:290,384,397,1108` and `service/message_logger.go:347`
  (change 3), and the test files `componentregistry/register_integration_test.go` and
  `internal/portgrammarcontrol/target_test.go` when their packages are ported.

### D15. One write seam (#100), statement metadata (#98), one revision (#99)

**The problem** (P-12, inventory §9.3). graph-ingest writes its entity bucket at seven sites on three lanes, with three
identity rules for one predicate: the stream lane replaces the whole set of a predicate (`graph.MergeTriples`,
`graph/helpers.go:98-108`), the append lanes deduplicate by six fields (`message.AppendIdentityKey`), and the
conditional replace judges "unchanged" by those six plus `Confidence`, `Timestamp` and `ExpiresAt`
(`canonical_mutations.go:630-640`). `EntityState.Version` is bumped at three sites, documented as optimistic
concurrency, and read by nothing. Births differ by lane: the mutation lane's create runs no hierarchy inference.
`Source` and `Timestamp` are optional, and a replace ignores `Timestamp`, so an older arrival overwrites a newer
statement. No test drives one predicate through two lanes.

**Options.**

- (a) Port the lanes as they are. Cost: #100's lock-in: three write semantics for one predicate become SemEngine's
  contract, and a dead `version` field sits in every stored value.
- (b) Keep the code, and write each lane's rule into the spec. Cost: the same three rules, now promised.
- (c) One write seam with the replace keyed on (subject, predicate), round 5's design. Cost: a stream arrival from one
  source removes every other source's statements of its predicate, appended ones included (the cross-lane wipe the
  round-4 review raised).
- (d) **One write seam, with the replace keyed on (subject, predicate, source)**: ruled (A, #91 comment 6037287957):
  "each source replaces only its own values … A predicate may hold values from several sources; a reader that wants
  one value picks." Every change to a stored entity goes through one write path inside graph-ingest, and the rule for
  a statement's identity belongs to the write mode, declared once and the same on every lane. Ruling A replaces the
  "(subject, predicate)" wording of #98's ruling for the replace identity; the rest of #98's ruling stands.

**The write modes** (spec home: `graph-entity-writes`). The rules are pure functions in `graph`, beside
`EntityState`, with no I/O, so they are tested without a broker; the seam calls them. `MergeTriples` becomes the
replace rule (the developer chooses the name).

| Mode | Lanes | Identity of a stored statement | Fence and order |
| --- | --- | --- | --- |
| create | mutation create; in-process create (hierarchy containers); the stream lane when the entity is absent | none: the entity is new | the bucket's create; an existing key is refused (`entity_already_exists` on the mutation lane; the stream lane retries as a replace) |
| replace | the stream lane, for each (predicate, source) set the arrival carries once its statements are stamped (below) | (subject, predicate, source): the stored statements of the predicate from that source are replaced whole; the predicate's statements from other sources stay | the KV revision (compare-and-set); `Timestamp` orders it, per set (below) |
| conditional replace | mutation reconcile, for the predicates the request names, from the request's one source (an empty set clears that source's statements of them) | (subject, predicate, source), as replace | the caller's expected revision; `Timestamp` does not order it, because the caller saw the state at that revision (ruling C) |
| append | mutation append; in-process append (hierarchy's inverse edges) | `message.AppendIdentityKey`: subject, predicate, datatype, source, context, object | the KV revision; a statement whose identity is stored is not added again |
| delete | mutation delete | none | the caller's expected revision |

"Unchanged", the conditional replace's no-op answer, is equality of every field of the stored statements of its
predicates from its source, `Confidence`, `Timestamp` and `ExpiresAt` included; it is a test of equal values, used by
that mode only, not an identity rule. Within one write, statements equal in every field count once.

**The source a set is keyed on.**

- **Stream lane.** Statements are stamped first (statement metadata, below) and then grouped. A statement without a
  `Source` takes the envelope's, so a producer that never names a source per statement replaces exactly what its
  earlier messages stored. A statement that names its own `Source` is grouped under it, so one message may carry sets
  for several sources, and a relay that re-publishes another producer's statements under that producer's name
  replaces that producer's set. The stream lane does not require a statement's source to match the envelope's, as the
  pin requires nothing of it; that one source's message can replace a set stored under another source's name is
  declared, not checked.
- **Conditional replace.** `graph.ReconcilePredicatesRequest` gains `source`, required: every desired statement's
  `Source` must equal it, or the request is refused as `invalid_request` naming the statement's index. The typed
  client fills it from `Metadata.Source`, which `Reconcile` now requires as `Create` and `Append` do
  (`pkg/projection/mutation_client.go:344-350` at the pin requires it for those two only); `pkg/lifecycle` sends its
  reconciles on the raw wire, not through the typed client, and its statements carry no source at the pin
  (`graph_emit.go:116-121`); after the port they carry one constant naming the lifecycle manager, which its three
  reconcile builders (`manager.go:419`, `:619`, `:739`) also send as the request's source. One source for every
  lifecycle write keeps a transition's reconcile replacing the phase statement it wrote before, as the pin's comment
  at `manager.go:611-618` requires (a phase that accumulates values is the bug it names). The other
  answer, a reconcile that replaces every source's statements of its predicates because its caller read them at that
  revision, keeps the cross-source wipe ruling A removes, for the lane a projection writes on; ruling A names no
  exception, so this design takes none. The reviewer may weigh it.
- **Append and delete** are unchanged: the append identity already contains the source.

**What a reader sees.** A predicate may hold statements from several sources; `EntityState.Triples` returns all of
them. The single-value reads in `graph`, `EntityState.GetTriple` and `EntityState.GetPropertyValue`
(`graph/types.go:49-77`), which at the pin return the first stored match, pick one the same way every time: the latest
`Timestamp`; on equal timestamps, the `Source` that sorts first; on equal sources, the first stored. Stored order is no
longer a meaning a reader can rely on, since a source's replace moves its set. No reader in this change calls them
(`gopls references`); their readers are graph-query, graph-embedding and graph-clustering (changes 4 and 7) and the
rule processor (change 6), which inherit this pick, including for the indexing profile: a later producer's profile
statement now sits beside the one graph-ingest stamped at birth, where at the pin it replaced it, and the read returns
the later one. A reader that wants another choice reads `Triples` and picks.

**Births.** A birth stamps the indexing profile on every lane (ADR-054, as at the pin). Hierarchy statements are added
at birth, with `enable_hierarchy`, on the lanes that infer hierarchy at the pin: the stream lane
(`processor/graph-ingest/component.go:2088`) and the in-process create (`:2240`) (D21). An entity created on the
mutation lane gets no hierarchy statements, as at the pin (`canonical_mutations.go:226-296`): ruled (B, #91 comment
6037287957).

**Statement metadata (#98, ruled).**

- Every statement the seam stores has a non-empty `Source` and a non-zero `Timestamp`. A write that carries a
  statement without them is refused as `invalid_request`, naming the statement's index and the missing field, on every
  lane; nothing is defaulted from the clock.
- On the stream lane, a statement from a `Graphable` payload that lacks `Source` or `Timestamp` gets it from the
  message envelope: the envelope's source (`Meta().Source()`) and creation time (`Meta().CreatedAt()`). A statement
  that carries its own keeps it. When the envelope has no source or no creation time either, the message is refused
  as poison: terminated, counted and logged as a structurally invalid candidate is.
- **Statements graph-ingest derives** (the indexing profile, hierarchy edges, a hierarchy container's type statement)
  name graph-ingest's producer in `Source` (one constant per producer; the pin's `graph-ingest-indexing-profile`,
  `component.go:1890`, is one) and carry the triggering message's time, never the clock (the round-4 review, applied
  on #91 comment 6037287957). On the stream lane that is the envelope's creation time, the value that stamps the
  message's own statements. On the mutation and in-process lanes it is the latest `Timestamp` among the write's own
  statements, each of which is required. A create on those two lanes that carries no statement has no time to give,
  and is refused as `invalid_request`; at the pin it was accepted and its profile stamped with `time.Now()`
  (`component.go:1891`). `GetHierarchyTriples` takes the triggering time as an argument, and the containers it creates
  for a birth carry it too: an `adapt` item on the `graph/inference` row, since the pin's hierarchy statements carry
  neither source nor time (`graph/inference/hierarchy.go:358-374`, `:409-430`, `:485-493`).
- `EntityState.UpdatedAt` is the store's write time, not a statement; it stays the clock, as at the pin
  (`component.go:2165`, `:2494`, `:2685`).
- The typed mutation client stops reading the clock: a `Create`, `Append` or `Reconcile` whose `Metadata.Timestamp` is
  zero and whose statement has none is refused before any request is sent, not-committed
  (`pkg/projection/mutation_client.go:378-383` at the pin).
- **`Timestamp` orders a replace** (stream lane only; ruling C). For each (predicate, source) set an arrival carries,
  the set is applied only if its latest `Timestamp` is not older than the latest `Timestamp` of the stored statements
  of that predicate from that source; equal timestamps apply, in arrival order. Another source's statements of the
  predicate take no part in the comparison. A set not applied keeps the stored statements; the arrival's other sets
  are still applied. The message is acknowledged: it has been applied as far as it may be.
- **The result says so.** The stream lane has no reply, so each set not applied as older is counted, on
  `semengine_graph_ingest_stale_sets_total` (the count #98 requires; its present consumers are the ruling and the test
  below), and logged at debug level with the entity, predicate and source.
- The entity's own fields (`MessageType`, `StorageRef`) take the arrival's values only when no set of that arrival was
  skipped as older.
- `Confidence` and `Context` are carried and never order a write.

**`EntityState.Version` goes (#100).** The KV revision is the only fence. `EntityState` has no `Version` field, and a
stored value has no `version` key. Decoding stays lenient (no `DisallowUnknownFields` in `graph`, §9.3), so a value
written by the pin still decodes. `pkg/lifecycle`'s `Version: 1` (`manager.go:395`) and graph-ingest's bumps go; the
one reader outside the set, `processor/rule/message_handler.go:380`, is a `class:port-refactor` note on the rule row
(change 6). No consumer reads it (§9.3).

**One stored revision (#99, ruled).** `ENTITY_STATES` keeps one revision per key (`History` 1), as at the pin
(`graph/kvcatalog.go:59-68`). SemEngine offers no point-in-time read of an entity's state; an entity's history is
its input stream, not the bucket. This is written into `graph-entity-writes` and the graph-ingest ledger row, and is
re-decided only on a named consumer need with its own design. The boot sweep's arithmetic depends on it
(`processor/graph-ingest/component.go:1256-1260`).

**What a caller observes, and the tests** (each written first and failing on the pin's code, except where a test is
marked as holding the pin's behavior; spec home `graph-entity-writes`, plus `projection-mutation` for the client):

- `TestWriteModesAgreeAcrossLanes`: one predicate written through each lane that uses a mode leaves the same stored
  statements (append through the mutation and in-process lanes; birth through all three; replace and conditional
  replace leave the same set for the same incoming statements from one source).
- `TestReplaceKeepsOtherSourcesStatements`: append P from source A, then a stream arrival with P from source B: P holds
  both; a second arrival with P from source B replaces only B's statement. Fails on the pin, where the arrival removes
  A's.
- `TestStreamLaneGroupsByStampedSource`: an arrival whose statements carry no source replaces the set stored under the
  envelope's source; one whose statements name two sources replaces both sets and no other.
- `TestReconcileReplacesOnlyItsSource` (graph-ingest) and `TestReconcileRefusesForeignSourceStatement`: an empty
  reconcile from A leaves B's statements of P; a desired statement from B in A's reconcile is refused, nothing stored.
- `TestMutationClientReconcileRequiresSource` (`pkg/projection`): not-committed, no request sent.
- `TestTransitionReplacesItsPhaseStatement` (`pkg/lifecycle`, through graph-ingest): after a create and two
  transitions the entity holds one phase statement, carrying the lifecycle manager's source. Fails on the pin's
  statements, which carry no source and are refused once D15 lands.
- `TestSingleValueReadPicksLatestAcrossSources` (`graph`): the latest timestamp wins whatever the stored order; ties
  go to the source that sorts first.
- `TestReplaceOrderedByTimestamp` (rule level in `graph`, and through the stream lane): an older set from the same
  source leaves P's statements from that source unchanged and raises the stale counter by one while the arrival's
  newer sets apply; an older set from another source is stored and counts nothing; an equal timestamp applies.
- `TestConfidenceAndContextNeverOrder`: a newer arrival with lower confidence replaces; an older one with higher
  confidence does not.
- `TestWriteRefusesStatementWithoutSourceOrTimestamp`: on the mutation lane (create, append, reconcile) and the
  in-process lane, `invalid_request` naming the index and the field; nothing stored. An empty create on those lanes is
  refused the same way.
- `TestGraphableLaneStampsFromEnvelope` and `TestGraphableLaneRefusesWithoutEnvelopeMetadata` (poison, counted).
- `TestDerivedStatementsCarryTriggeringTime`: a stream birth with hierarchy enabled, the envelope created at T, under a
  test clock set elsewhere: the profile, hierarchy and container statements carry T and graph-ingest's producer; a
  mutation create's profile carries the latest timestamp of its statements. Fails on the pin, which stamps the profile
  with the clock and hierarchy statements with nothing.
- `TestMutationClientRefusesMissingTimestamp` (`pkg/projection`): not-committed, and no request reaches the broker.
- `TestStoredEntityHasNoVersion`: the stored JSON has no `version` key; a value that has one decodes.
- `TestEntityStatesKeepsOneRevision`: the catalog descriptor of `ENTITY_STATES` has `History` 1.
- `TestMutationCreateBirthGetsNoHierarchy`: with `enable_hierarchy`, an entity created on the mutation lane carries its
  statements and its profile and no hierarchy statement. Holds the pin's behavior (ruling B), so it passes on the pin's
  code; its sensitivity is shown by `task mutate:check` with a mutant that runs the inference on that lane.
- `TestEntityWritesHaveOneSeam`: a package test that parses graph-ingest's non-test files and fails, naming file and
  line, when a write method of the entity bucket (`Create`, `Update`, `UpdateWithRetry`, `UpdateWithRetryRev`, `Put`,
  `Delete`, `DeleteAtRevision`) is called outside the seam's file, with a sensitivity case that plants one. It holds
  the shape #100 asks for; the developer chooses the seam's API.

**Not changed** (the audit's "port untouched" list): acknowledgement order (KV write, durable guard stamp, ack,
`keyed_ingest.go:208-224`); `authorizeSubject` at every lane before any I/O; `MarshalEntityState` as the one validating
write gate and `UnmarshalEntityState`'s poison classification; `projection.MutationClient`'s commit-state model over
`graphmutation`; append deduplication inside the compare-and-set closure; `internal/dispatch.KeyedPool`;
`graph.mutation.>` with one subject home, typed requests and one client.

### D16. The `graph` root holds the data model and the wire types (#101)

At the pin the root carries six jobs (P-14, §9.5) and four of its files import NATS. After the port:

- **The root (`graph`)** holds the data model (`EntityState`, `Graphable`, the write-mode rules of D15, entity-ID
  prefixes, the state-contract encode and decode, the predicate codec, the bucket-name constants, `StateContractError`
  and its classifiers) and the wire types (the query request and reply types, the mutation requests and responses,
  `ExactEntity`, the GRAPH_STATUS envelope `IndexStatusResponse` with its `IndexState*` constants, the reply envelope
  of D18, and the verb table of D20). It imports no NATS package: `ExactEntityReader` keeps its narrow requester
  interface and passes a zero timeout through, so the request's own default applies (SemEngine's `natsclient`
  documents "If timeout is 0, DefaultRequestTimeout is used", `natsclient/request.go:177`); the pin's only use of
  `natsclient` in that file was that default.
- **`graph/kvcatalog`** (new) holds the bucket catalog and its acquisition (`kvcatalog.go`), the retention check
  (`owned_bucket_retention.go`) and `IsKVTombstone`, the one `jetstream` use in `state_contract.go`. Options: (a)
  **a sub-package** (recommended): the catalog keeps its names in `graph` and its NATS edge out of the root; (b) move
  it into `natsclient`, the mechanism's package: the floor ported in change 1 would gain graph's bucket names;
  (c) leave it in the root: #101's lock-in, every importer of a type pulls the NATS client.
- **Catalog rows.** All of the pin's rows except `TOOL_CALL_OUTCOMES`, whose owner, agentic-tools, is cut (#8 Q4;
  semteams acquires it with its own descriptor), and `ENTITY_SUFFIX_INDEX` (D19). #101 also names
  `COMMUNITY_SUMMARIES`, `STORAGE_REPORT` and the configuration bucket family as rows for unadmitted owners; the pin
  admits all three owners: graph-clustering (change 7), the storage collector in `service` (change 3) and
  `config.Manager` (change 3) (§9.5). They stay: the surface audit keeps what an admitted package reads (#9 comment
  5968830525), and the catalog is the one home their owners would otherwise re-add them to. #101 is a port-refactor
  issue, not a ruling, so this is not an owner question; the reviewer may weigh it.
- **`graph/readiness`** receives the readiness computation: `readiness_gate.go` whole (`EvaluateReadinessGate`,
  `StatusReading`, `DeferReason`) and `index_status.go`'s computation (`ComputeIndexStatus`, `IndexStatusInputs`,
  `ComputeBacklogStatus`, `BacklogStatusInputs`), with their tests. The wire type stays in the root, so a reader of
  GRAPH_STATUS needs no NATS import.
- **The readiness envelope (#110's change-2 part; ruled F, #91 comment 6037287957).** `IndexStatusResponse` ports
  without `Phase`, `Revision` and `LastSynced` (`graph/index_status.go:134-138`), and gains `published_at`. P-20
  (§9.15): `Revision` is `IndexedRevision` as a string (`index_status.go:249-251`) and `LastSynced` the last apply time
  (`processor/graph-ingest/readiness.go:309-321`); `Phase` is never set. With them go `IndexStatusInputs.LastSynced` and
  `BacklogStatusInputs.LastSynced`, graph-ingest's `lastSyncedRFC3339` and its test (`readiness_test.go:172-183`);
  `lastAppliedAt` stays, since the staleness computation reads it (`oldestOutstandingAt`, `readiness.go:295-307`).
  `published_at` is set by `graph/readiness.Publisher.Publish` (`graph/readiness/publisher.go:90-107`) on every write,
  whatever the caller passed, so no producer can leave it out: UTC, RFC 3339 with nanoseconds, from the wall clock. The
  clock is right here: the field states when the producer wrote, not when anything in the graph was asserted, so #98's
  rule does not apply. What a reader does with it (stale to unknown in the gate, #110's fix shape) is change 7's; this
  change ships the field. The doc comment's "the two structs change together" with `pkg/fusion.IndexStatus` becomes a
  `class:port-refactor` note on the `pkg/fusion` row (change 5, #110). Consumer: semsource decodes `GRAPH_STATUS` into
  its own struct and reads `revision` and `last_synced` (`processor/source-manifest/workbench_capabilities.go:110-118`,
  `readiness.go:42-43` at `e4febc0d`); after the port both arrive absent and its browser contract carries them empty. No
  compile error tells it: a silent change, named on the `graph` row's adapt item and in §9.13 (l), and semsource changes
  once, on adoption (ruling F). Tests: `TestIndexStatusResponseHasNoLegacyFields` (`graph`: the encoded envelope has no
  `phase`, `revision` or `last_synced` key) and `TestPublishStampsPublishedAt` (`graph/readiness`: the stored value's
  `published_at` lies between the clock read before and after `Publish`, and a caller's value is replaced), each written
  first and failing on the pin's code. Spec home: `graph-transport-boundary`, "The readiness envelope carries its
  publish time and no legacy fields".
- **`events.go` is not ported.** It is a second mutation grammar (`map[string]any` payloads on `graph.events.*`) with
  no subscriber at the pin; its one admitted producer is `processor/rule` (`expression_factory.go:353`,
  `publisher.go:151-152`), whose port moves that emission to the mutation-request protocol or drops it (a
  `class:port-refactor` note on the rule row, change 6, #101). D9's `alertDigestDomain` rename goes with it.
- **Dead exports** (the appendix's `graph` rows) are dropped; an export read only by `graph`'s own tests is unexported
  or moved into a `_test.go` file. The port task settles the set with `gopls references`.
- **README** rewritten for what the root holds and where the catalog and readiness computation went.
- **Test:** `TestGraphImportsNoTransport` in `internal/harness/contract`: `go list -deps ./graph` lists no `natsclient`
  and no `github.com/nats-io` path, with a sensitivity test over a temporary module. Written first and failing on the
  pin-shaped root. Spec home: `graph-transport-boundary`.
- **Consumers** edit imports when they adopt SemEngine (§9.13 item h): a compile error each.

### D17. `component` reaches no graph package (#102)

`component` reaches the graph family only through `Dependencies.LifecycleManager *lifecycle.Manager`
(`component/dependencies.go:99`), because `pkg/lifecycle` imports `graph`, `internal/graphmutation` and
`pkg/projection` (P-13, §9.4). Every reader narrows the slot to an interface whose methods name `lifecycle.Participant`,
`TransitionSource` or `WorkflowDef` (`processor/rule/actions.go:509-532`; semboids `internal/sim/lifecycle.go:51-53`).

**Options.**

- (a) A narrow interface declared in `component`, as #102 suggests. Every method the readers use names a
  `pkg/lifecycle` type, so the interface imports `pkg/lifecycle` again, and the edge stays, unless those types first
  move to a graph-free package (they are in `participant.go`, which imports nothing) and `component` declares an
  interface as wide as the rule's eight methods. Cost: a split of `pkg/lifecycle`, a new import path for
  `lifecycle.Participant` in semteams and semboids, and an interface that is not narrow.
- (b) An untyped slot (`any`) that readers assert to their own interface. Cost: a wrong value is found at
  construction by a reader that checks, or never by one that does not; an untyped field in a public struct.
- (c) **Drop the slot** (recommended), as #29 drops `ToolRegistry`: #29's issue text is "drop
  `Dependencies.ToolRegistry` and `ToolRegistryReader`", and `ToolRegistryReader` itself names `agentic` types
  (`dependencies.go:56`), so the narrow interface was the edge, not the cure. The host passes the manager to the
  packages that use it, at their registration or construction. Cost: `processor/rule` (change 6) takes the manager at
  registration and `service` (change 3) stops copying it into every component's dependencies, both
  `class:port-refactor` notes on their rows; semboids' sim component takes it through its own constructor. Each is a
  compile error, the best way to find out.
- (d) Keep the concrete field: #102's lock-in.

Ruled (D, #91 comment 6037287957): "`Dependencies.LifecycleManager` is dropped, not narrowed. semboids' sim component
and semteams' wiring change on adoption." The `component` ledger row's adapt item for the drop names the adopter
sites (§9.4, at the inventoried commits): semboids `internal/sim/component.go:255-256`, which reads the field, and
`cmd/semboids/main.go:190`, and semteams `cmd/semteams/main.go:209`, which set it on the service's dependencies
(change 3's struct, which loses its copy into components under the same item).

**`ModelRegistry` and `StoreRegistry`, asked the same question** (#102): neither reaches a graph package.
`ModelRegistry` is `model.RegistryReader`, an interface `model` declares, and `model` imports only
`golang.org/x/net/http2`; `StoreRegistry` is `*storeregistry.Registry`, whose package imports only `storage`, which
imports only the standard library (§9.4). Both stay as they are.

**`pkg/lifecycle`'s discarded error** (`graph_emit.go:31`, named by #102): the manager no longer discards
`graphmutation.NewClient`'s error. With no client it holds no emitter, and every emit returns `ErrEmitFailed` naming
the missing client, which is the pin's observable outcome without the discarded error.

**What a caller observes, and the tests.** `go list -deps ./component` lists no `agentic`, `graph`, `graph/…`,
`internal/graphmutation`, `pkg/lifecycle` or `pkg/projection` path (`pkg/projection/contract`, which
`payloadregistry` imports, is allowed): the #29 contract test, extended, with its sensitivity test, written first and
failing on the pin's `dependencies.go` (spec home `component-registration`). `TestManagerWithoutClientRefusesEmit` in
`pkg/lifecycle`. Consequence: `pkg/lifecycle` and `pkg/projection` leave closure(graph-ingest) and stay in this
change (D1).

### D18. One reply envelope for the graph query family (#103)

Issue #103 asks for the envelope to be decided now, with its producers in change 4. At the pin, `graph.QueryResponse[T]`
is `{data, timestamp}`, 14 of graph-query's 16 verbs reply bare, and `UnwrapQueryResponse` decides by sniffing a closed
key set whether a reply is enveloped (P-15, §9.6).

**Decision.**

- Every reply on the `graph.query.*` family is one envelope, `graph.QueryResponse[T]`: `data`, `indexed_revision`
  (the `ENTITY_STATES` revision the answer reflects), `producer` (the responding component instance) and
  `timestamp`. Building one requires the producer and the revision.
- A request on that family may carry `min_revision`, one field declared once in `graph` that request types embed. A
  producer whose indexed revision is below it answers with the classified `index_not_ready`
  (`graph/mutation_responses.go:79`) or after a bounded wait; that producer behavior is change 4's to specify and
  prove.
- `UnwrapQueryResponse` is not ported: a reply's shape is its verb's declared reply type (D20), never sniffed. Its one
  admitted reader, `pkg/fusion/fusionnats/client.go` (change 5), decodes by the declared type (a
  `class:port-refactor` note on its row).
- **`graph.ingest.query.*` is not on the envelope.** It is the authority-read family: graph-ingest reads the
  authoritative bucket, so a reader that wrote at revision R and reads the entity sees R or later without asking, and
  the entity verb's reply already carries the revision it read (`graph.ExactEntity.kvRevision`,
  `processor/graph-ingest/query.go:119-122`). graph-query wraps graph-ingest's replies once, when it serves them on
  `graph.query.*` (change 4).

**Options considered.** (a) The envelope on both families now: graph-ingest has no indexed revision to report, and
an authority read needs none; (b) the decision above (recommended); (c) defer the whole decision to change 4: #103
asks for it now.

**What a caller observes, and the test.** `TestQueryResponseCarriesIndexedRevisionAndProducer`: a response built for a
producer at a revision encodes `data`, `indexed_revision`, `producer` and `timestamp`, and decodes back equal; written
first, failing on the pin's type. The absence of `UnwrapQueryResponse` is a compile fact. Consumers: semconnect decodes
`graph.QueryResponse[graph.PredicateData]` (`gateway/cs-api/systems.go:993`); the added fields do not break that.
Spec home: `graph-transport-boundary`.

### D19. `ENTITY_SUFFIX_INDEX` and the suffix verb are not ported (#104)

At the pin the suffix index maps an instance suffix to one ID, so two entities with one suffix resolve to whichever
wrote last; writes fail at debug level and deletes are discarded; a miss falls to a scan of every key that returns
the first match, or `{"id":""}` instead of not-found (P-16, §9.7). Its only caller is graph-query's partial-ID
resolution (`processor/graph-query/entity_resolver.go:102`), the step after the alias index, on the natural-language
path (`graphrag.go:422,1016`, change 4). No consumer calls it.

**Options.** (a) Drop the index and keep the verb on the scan: every call reads every key of the authoritative bucket
and the first-match collision stays; making it correct is a new verb design with no consumer outside graph-query;
(b) **drop the index and the verb** (recommended): the bucket, its catalog row and constant, the suffix cache and its
metrics, `updateSuffixIndex`, `removeSuffixIndex` and `graph.ingest.query.suffix`; graph-query's partial-ID
resolution is a `class:port-refactor` note for change 4, which resolves through the alias index or designs a
multi-ID verb that refuses ambiguity, as #104's fix shape says; (c) keep both: #104's lock-in.

**Test:** `TestGraphIngestProvisionsNoSuffixIndex`: after `Start`, no `ENTITY_SUFFIX_INDEX` bucket exists, and a
request on `graph.ingest.query.suffix` gets no responder. Written first, failing on the pin's code. Coverage of
graph-ingest after the drop: 84.7% (P-19).

### D20. `graph.ingest.query.*` declared once, by its responder (#106, #16)

- **A verb table in `graph`**: one entry per verb, with its name, subject, responder (`graph-ingest`), request type and
  reply type: `entity`, `batch`, `prefix` (the suffix verb goes, D19). Its shape is graph-query's internal
  operation table (`processor/graph-query/query.go:33-43`) without the handler, so change 4 extends it to
  `graph.query.*` and the other responders instead of inventing a second one (#105, #106).
- **Callers take the subject from the table**: `graph`'s `ExactEntityReader` here, graph-query's router and resolver
  in change 4. **graph-ingest subscribes by walking its entries in the table**, and to nothing else.
- **`graph.mutation.>` keeps its one home**, `internal/graphmutation/protocol.go:16` (the audit's "port untouched"
  list). Round 4's delta put it in `graph`; this round keeps it where it already has one home.
- **Tests.** `TestReservedSubjectsDeclaredOnce` (D8, task 2.4) fails, naming file and line, on a literal of a
  table subject or of the `graph.mutation.` prefix in a non-test file other than the two declaring files.
  `TestGraphIngestServesExactlyTheDeclaredVerbs`: the subjects graph-ingest's adapter lists as request
  subscriptions equal the table's entries for responder `graph-ingest`; written first, failing on the pin's code
  (four literal subscriptions, one undeclared). Spec home: `graph-transport-boundary`.
- **Not in this change:** refusing a second subscriber on a declared verb at `Start` (#106's boot error) needs a view
  across components, which change 4 builds when it generalises the table; refusing a stream filter that overlaps a
  reserved subject stays change 3's (#16).
- **Adoption sweep** (D20 establishes a reusable shape; one tracking issue, task 6.3):
  `processor/graph-index/query.go:31-106`, `processor/graph-query/query.go:49-66` and `router.go:16-43` (change 4);
  `processor/graph-embedding/query.go:24-46`, `pkg/fusion/fusionnats/client.go:26-31` (change 5);
  `processor/graph-index-spatial/query.go:27-43`, `processor/graph-index-temporal/query.go:22-29`,
  `processor/graph-clustering/query.go:24-56` (change 7).

### D21. Hierarchy birth and the guard record fail closed (#111 item 1)

**Hierarchy at birth.** At the pin a failed hierarchy inference at an entity's birth is a warning, the entity is
born without its container edges, and nothing adds them later, because the inference runs on the first write only
(`processor/graph-ingest/component.go:2088-2095`, `:2112`, `:2240-2246`); inside the inference a failed inverse edge
or sibling pass is a warning too (`graph/inference/hierarchy.go:245-250`, `:376-385`, `:432-448`) (P-17, §9.8).
Options: (a) **fail the birth** (recommended); (b) store a repairable "hierarchy pending" statement and add a repair
pass: new stored state and a new background pass, for a consumer no one has named; (c) the pin's warning: the silent
degrade #111 names.

What a caller observes under (a): with `enable_hierarchy`, an entity born on a lane that infers hierarchy (the stream
lane and the in-process create, as at the pin; D15) is born with its hierarchy statements or not at all. A mutation-lane
create infers none, so the rule does not reach it (ruled B, #91 comment 6037287957). `GetHierarchyTriples` returns an
error when any part fails: a container birth, a forward edge, an inverse edge or the sibling pass. graph-ingest then
writes nothing for the entity and returns the error, classified transient. On the stream lane the input is not
acknowledged and is delivered again; on the in-process lane the caller gets the error. Containers and inverse edges
committed before the failure are what the next attempt commits too; the append identity suppresses the repeats. Test:
`TestHierarchyFailureFailsTheBirth`: the inference's entity manager fails once, the entity is absent and the error is
transient; on the redelivery the entity is born with its container edges. Written first, failing on the pin's code.

**The guard record.** At the pin a stored applied-sequence value shorter than eight bytes is read as "first seen"
(`keyed_ingest.go:277-278`), which reopens the overwrite the record exists to prevent. After the port, a stored record
graph-ingest cannot decode is refused: the input is not applied and not acknowledged, the refusal is counted and
logged once per key with the key, and the input is delivered again until the record is repaired (deleted) or the
consumer's delivery limit ends it. This is the shape graph-ingest already uses for poisoned resident state
(`keyed_ingest.go:168-186`). An absent record is first seen, as at the pin; the no-bucket case is D8's. With #15's
generation-aware record, "cannot decode" covers whatever value format that work chooses. Test:
`TestCorruptGuardRecordIsRefused`: a three-byte value at the input's guard key; the input is neither applied nor
acknowledged and the counter rises by one; once the key is deleted, the redelivery applies. Written first, failing on
the pin's code. Spec home: `graph-ingest-recovery` (this change's delta).

### D22. The guard's state in graph-ingest's tests (tasks 3.12, 3.13)

The developer's open point: `internal/lifecycleguard.Guard` exposes no state, and the pin's graph-ingest tests read
and set the lifecycle fields directly (P-18, §9.9). Options: (a) a read-out on the guard for tests: new exported
surface whose only consumer is tests (category 4 removes it); (b) **tests drive the guard into each state through its
methods and assert what a caller observes** (recommended).

Under (b): every read has an observable stand-in in the same test. "A refused `Start` left the guard unused" becomes
"a following `Start` is not refused with `errs.ErrAlreadyStarted`"; "terminal" becomes "a repeated `Stop` calls
nothing and a `Start` is refused", which those tests already count. Every struct literal that built a component
mid-lifecycle becomes a setup step that calls the component's guard's `Start` with a start function that acquires
nothing, or one that fails with a rollback that fails, which leaves the cleanup pending (`guard.go:66-71`). The lock in
`lifecycle_integration_test.go:192-197` guards the component's own handles, not the guard's state, and stays.
`internal/lifecycleguard` does not change; task 2.7 stands.

### Left open for #105–#111

These issues belong to changes 4, 5 and 7. This change forecloses none of them:

- **#103 and #106** are decided here in the shape change 4 extends (D18, D20).
- **#105** (split graph-query): the NL coordinator registers its verbs in the same table (D20).
- **#107** (`KeyedPool` adopters): `internal/dispatch.KeyedPool` is the target. `Submit` and `Stats` go as dead
  surface (no reader at the pin); if graph-index needs a non-blocking submit in change 4, it is new surface with a
  present consumer then.
- **#108** (rebuild on restart): nothing here.
- **#109** (follower and processor shell): D17's test forbids `component` from importing a graph package, so an
  `ENTITY_STATES` follower cannot live in `component`; change 4 homes it in a graph package. The processor shell
  imports no graph package and can live in `component`, holding an `internal/lifecycleguard.Guard` unexported (D13).
- **#110** (readiness envelope): its envelope fields land here (ruling F, D16); the fusion copy is change 5's and
  the gate's use of `published_at`, the single consumer entry and `Health()` from the readiness set are change 7's.
  `readiness.Set` is dropped here as dead surface and returns with #110 as returning surface, named on the readiness
  row's `known_risks` (D6), so the drop forecloses nothing.
- **#111 items 2–4**: changes 4 and 7.

**D7 and D13, re-checked against D15 and D17.** `pkg/lifecyclecleanup` imports only the standard library and
`internal/lifecycleguard` only `pkg/errs` and `pkg/lifecyclecleanup`; neither touches a graph package or `component`,
and `component` imports neither. graph-ingest composes the guard; the write seam does not touch it. D13's adoption list
is unchanged; #109's processor shell would be one more adopter, in change 4.

## Owner questions

None open. No ruling's text contradicts the measured pin. One reading of ruling A is named for the reviewer: it is
applied to the conditional replace as well as the stream lane's replace, so a reconcile names one source (D15, "The
source a set is keyed on"); the ruling names no exception, and the other answer's cost is stated there. #101's list of
catalog rows "for owners SemEngine does not admit" is a design decision, not a ruling (three of its four owners are
admitted; D16).

### Ruled

- **A** — asked on #77 (comment 6024793500): the component cleans up its own failed start with a public helper, the
  managers stay as the second line; the #38 exception is granted and binds only ported components (#77 comment
  6035317931); PR #93 closes #77 (#77 comment 6035358884). Applied in D7 and the `lifecycle-suite` delta.
- **B** — "port": port only what `graph/inference` reads from `graph/llm` (`client.go`); drop `ReviewConfig.LLM`; the
  rest of `graph/llm` and `go-openai` wait for change 7; no authority-rule exception; foundation D8 modified for
  `graph/llm` (#91 comment 6035477895). After #97 the slice reads nothing of `graph/llm`, so by the same reasoning
  nothing of it is ported (D1a); `ReviewConfig.LLM` goes to change 7 with `config.go`.
- **C** — "drop the token": `internal/componentadmission` is not ported; the three registry methods are plain public
  methods whose doc comments direct callers to the component manager; no public-signature exception; a consumer
  creating a component outside the manager is review only (#91 comment 6035477895). Applied in D1, D9, D14.
- **E** — "Leave it out if it's dead code": `pkg/worker` is not ported; `internal/dispatch` declares its own "stopped"
  error (#91 comment 6035429806). Applied in D1, D4, D5, D6.
- **F** — "Agree": `graph/inference` joins the 80% gate in change 7; the `ReviewWorker` `synctest` test stays here; the
  ledger row records 47.9% and a tracking issue holds the tests (#91 comment 6035429806). Re-read under #97 in D11:
  the slice joins the gate here at 82.8%; the review worker and its test go to change 7 with the code.
- **#97** — `graph/inference` ports as its hierarchy slice; `ReviewWorker`, the HTTP handlers,
  `NATSRelationshipApplier` and `ReviewConfig.LLM` do not port in change 2 (comment 6036308092). Applied in D1, D1a,
  D5, D6, D11.
- **#98** — `Source` and `Timestamp` required on every statement at the write seam, stamped from the envelope on the
  stream lane, never from the clock; `Timestamp` orders a replace and the result says when it did not apply;
  `Confidence` and `Context` never order (comment 6036308354). Applied in D15.
- **#99** — `ENTITY_STATES` keeps `History` 1, declared as a constraint, re-decided only on a named consumer need
  (comment 6036308600). Applied in D15.

- **Rulings A–F on the round-4 review of round 5** (#91 comment 6037287957, 2026-10-07, "accept all
  recommendations"):
  - **A** (#100's cross-lane wipe): a replace is keyed on (subject, predicate, source); a predicate may hold values
    from several sources, and a reader that wants one value picks. Applied in D15 (the write modes, the stream lane's
    stamped source, the single-value read) and `graph-entity-writes`.
  - **B** (hierarchy on mutation-lane births): the pin's behavior; a mutation-lane create gets no hierarchy
    statements, and #111's fail-closed birth does not reach it. Applied in D15 and D21.
  - **C** (#98's ordering and reconcile): a conditional replace is fenced by the caller's KV revision, not by
    `Timestamp`; `Timestamp` orders the plain replace. Applied in D15 as round 5 had it.
  - **D** (#102): `Dependencies.LifecycleManager` is dropped, not narrowed; semboids' sim component and semteams'
    wiring change on adoption. Applied in D17, the adopter sites named on the `component` ledger row (task 5.1).
  - **E**: `graph/structural` moves to change 7 with its only readers, changing foundation D2's package list and 03B
    D10's critical list. Applied in D1a and D11.
  - **F** (#110): `IndexStatusResponse` ports without `Phase`, `Revision` and `LastSynced` and gains `published_at`
    in this change; semsource changes once, on adoption. Applied in D16.
  - From the same comment: statements graph-ingest derives carry the triggering message's timestamp, never the
    clock (#98). Applied in D15.

The port-refactors #100–#104, #106's pattern and #111 item 1 are owner-ordered for this change (PR #93 comment
6036316289) and applied in D15–D21. Round 1's question D, the wire and storage names, is answered by #69 and applied in
D9.

## Premises (each with its measurement)

- P1. The set is 14 pin packages, `graph/inference` as its hierarchy slice: 24,293 non-test lines before dead-surface
  removal (round 3's 29,963 less `graph/llm/client.go` 65, `graph/structural` 883 and `graph/inference` outside the
  slice 4,722), and 141 test files with 36,301 lines. — §0, §8, §9.2.
- P2. No live production context root is in the set. — §0, grep with comment lines removed.
- P3. Change 1 removed only the registration methods this set uses. — P-2, build of the copy.
- P4. Eight test files import packages outside the set. — P-3.
- P5. graph-ingest passes the suite's eight checks at the pin, observed through return values only. — P-5.
- P6. A timed-out `Pool.Stop` followed by a second `Stop` panics at the pin (a reason for ruling E). — P-6.
- P7. No unit flake at the pin over five shuffled runs ×3 and one race run; the slice alone passes one race run and
  five shuffled runs. — P-1, P-10.
- P8. The repository's checks fail on the copy exactly as listed. — P-4.
- P9. 186 exported identifiers have no reader by type in the admitted set or a consumer; `BoundedDispatcher`,
  `readiness.Set` and `lifecycle.Manager.Watch` among them; 27 of them are in files not ported in change 2. — §2,
  §9.1, the appendix.
- P10. `pkg/worker`'s only non-test importers are `pkg/dispatch/dispatcher.go` and `errors.go` (the basis of ruling
  E). — §2, `grep -rl`.
- P11. `graph/inference` is public by signature (`processor/graph-clustering/component.go:77`); `pkg/dispatch` is not.
  — §2, §8.
- P12. Coverage after the drops, regrouped by D16: graph-ingest 84.7%, `graph` 80.9%, `graph/kvcatalog` 80.3%,
  `graph/readiness` 86.4%, `graph/inference` (slice) 82.8%, `storage/storeregistry` 100%; four packages short by 10,
  31, 81 and 97 statements. — P-7, P-8, P-10, P-19 (local-only profiles; commands in §1 and §9).
- P13. The owner-lifecycle state has 12 copies in the admitted set and none in the four consumers. — §3 category 2.
- P14. No `Hash` caller (#78), no storage-report observer (#85) and no second `natsclient.Client` (#75) in the set. —
  `grep -n '\.Hash()'` and `StorageReportObserver` over the set: empty; observers only in `service` (change 3).
- P15. The hierarchy slice compiles alone with `TripleAdder` added and imports neither `graph/llm` nor
  `graph/structural`. — P-10.
- P16. After #97, no package ported in change 2 imports `graph/llm`, `model/wire` or `graph/structural`; after #102,
  `pkg/lifecycle` and `pkg/projection` leave closure(graph-ingest). — P-11.
- P17. `graph/structural`'s importers are the detectors and graph-clustering only. — P-11.
- P18. `componentadmission.Access` is read in the set only by `component/registry.go` (four methods) and its tests. —
  §8.
- P19. graph-ingest writes `ENTITY_STATES` at seven sites with three identity rules for one predicate, and the
  mutation lane's create runs no hierarchy inference. — P-12.
- P20. `EntityState.Version` has no reader outside a write in the set, and none in the consumers; the graph decoders
  are lenient. — P-12, `gopls references graph/types.go:43:2`.
- P21. The stream lane stamps neither `Source` nor `Timestamp`; the envelope has both; the typed client defaults
  `Timestamp` to the clock; hierarchy statements carry neither. — P-12.
- P22. Every reader of `Dependencies.LifecycleManager` narrows it to an interface naming `pkg/lifecycle` types; with
  the field gone, `component` reaches no graph package; `model` and `storage/storeregistry` reach none. — P-13.
- P23. Three of the four catalog rows #101 names have admitted owners; `graph.events.*` has no subscriber. — P-14.
- P24. `UnwrapQueryResponse`'s one admitted reader is `pkg/fusion/fusionnats`; graph-ingest's replies are bare and the
  entity reply carries its KV revision. — P-15.
- P25. `graph.ingest.query.suffix` has one caller, graph-query's partial-ID resolution, and no consumer. — P-16.
- P26. A hierarchy failure at birth leaves the entity without its edges for good; a guard value shorter than eight
  bytes is read as first seen. — P-17.
- P27. Every guard-state read in graph-ingest's tests has an observable stand-in, and every literal state is reachable
  through the guard's methods. — P-18.
- P28. `IndexStatusResponse.Phase` is never set; `Revision` repeats `IndexedRevision` and `LastSynced` the last apply
  time, written in the ported set only by the readiness computation from graph-ingest's input; semsource reads
  `revision` and `last_synced` from its own struct. — P-20.
- P29. `pkg/lifecycle`'s statements carry no `Source` and stamp `Timestamp` from the clock, and it reconciles on the
  raw wire; the hierarchy slice's statements carry neither; the pin's indexing profile is stamped with the clock;
  the mutation lane's create may carry no statement. — P-21.

## Invariants and their spec homes

- Commit classification (I5): not-committed only for a rejection proven before any storage effect; otherwise
  commit-unknown — `projection-mutation`, "Commit ambiguity is preserved".
- Conditional reconcile (I6) — `projection-mutation`, "Conditional reconcile at a caller-observed revision".
- Generation-aware replay (I3) — `graph-ingest-recovery`, "Replay protection is generation-aware".
- Settlement order (I9) — `graph-ingest-recovery`, "Settlement order" and "Recovery on a file stream is redelivery".
- Accepted is not durable (I7) — `graph-ingest-recovery`, "Acknowledged is not durable".
- One reserved-subject declaration (I2, its first half), by the verbs' responder (D20) — `graph-transport-boundary`.
- Per-package registration (I1, its adopter path) — `component-registration`.
- One-shot owner lifecycle (D13) — `lifecycle-suite`, "Portable floor" (as this change modifies it).
- Failed-start rollback (D7): a nil parent or nil rollback is refused; the rollback sees the parent's values with a
  deadline and no parent cancellation; its error and the budget's expiry are both returned — `lifecycle-suite`,
  "Failed-start rollback helper".
- A failed `Start` whose own cleanup fails reports both failures and keeps what is left for a later `Stop` (D7) —
  `lifecycle-suite`, "Failed start whose own cleanup fails".
- One identity rule per write mode, the same on every lane (D15) — `graph-entity-writes`, "One rule per write mode".
- Every stored statement has a `Source` and a `Timestamp`; none comes from the clock by default (#98, D15) —
  `graph-entity-writes`, "Statement metadata is required".
- Each source replaces only its own statements of a predicate; a single-value read picks the same statement every time
  (ruling A, D15) — `graph-entity-writes`, "One rule per write mode" and "A single-value read picks one statement the
  same way every time".
- A replace never applies statements older than the stored ones for their (subject, predicate, source) (#98, D15) —
  `graph-entity-writes`, "Timestamp orders a replace".
- The KV revision is the only fence; one stored revision per key (#99, #100, D15) — `graph-entity-writes`, "The
  revision is the only fence" and "One stored revision per entity".
- The readiness envelope carries `published_at`, set by the publisher, and none of the legacy fields (ruling F, D16) —
  `graph-transport-boundary`, "The readiness envelope carries its publish time and no legacy fields".
- An entity born on a lane that infers hierarchy is born with its hierarchy statements or not at all (#111, D21) —
  `graph-entity-writes`, "Birth with hierarchy fails closed".
- A guard record that cannot be decoded is refused, never read as first seen (#111, D21) — `graph-ingest-recovery`,
  "Replay protection is generation-aware".
- The `graph` root imports no transport; `component` reaches no graph package (#101, #102; D16, D17) —
  `graph-transport-boundary`, "The graph root imports no transport"; `component-registration`, "The component model
  reaches no agentic or graph package".

## Related issues this change does not close

Issues #75 (shared series; D4 records graph-ingest's gauges under it), #78 and #85 (no caller in the set, P14), #81 (new
and repaired tests carry `// Requirement:` citations in #81's form; carried tests wait for #80's scope ruling), #24
(unblocked when this change merges, foundation (d)), SS#1411 (answered here for SemEngine; the SemStreams issue is not
touched), #105, #107, #108 and #109 (changes 4, 5 and 7; "Left open for #105–#111"), #106, #110 and #111 (this change
lands #106's pattern, #110's envelope fields (ruling F) and #111's item 1; the rest is changes 4, 5 and 7). #77 is ruled
and this change closes it (comment 6035358884); D7 applies the ruling. The rulings on #97, #98 and #99 say each closes
with the change that implements it, and #100–#104 land whole here, so PR #93 closes #97–#104 (task 6.6).

## Declared costs

- The largest port so far: 14 pin packages, one of them as a slice, 141 test files, 43 fixture-client sites, 19
  sleeps, 20 unbounded cleanups, 8 test files with out-of-set imports.
- A new harness helper (D3), a new internal package (D13) and a new public package split from `graph`
  (`graph/kvcatalog`, D16), each with its reason; D3, D13 and D20 each with an adoption list.
- Three helpers change shape (D5); their later callers are port-refactor rows in changes 5 and 7.
- Tests to write to reach the floor: about 220 statements across four packages. The `graph` root clears it by two
  statements at the pin (D11).
- `graph/llm`, `model/wire`, `go-openai` and `graph/structural` arrive later (changes 4 and 7) than foundation D2 and
  D8 planned; the review worker's shape and the rest of `graph/inference` are change 7's (#97, D1a).
- A consumer can call the three registry methods directly; that it does not is review only (ruling C, D14).
- One new public package, `pkg/lifecyclecleanup` (one function), by #77's ruling; that an external component calls it
  correctly is checked only by that component's own tests (D7.1).
- A nil rollback callback becomes an error, a declared change from the pin (D7.2).
- graph-ingest metrics change name (`semengine_*`) and stop appearing on the process-global registry; two wire and
  storage names change (D9).
- The write path is rewritten at port (D15): a replace touches only its source's statements of a predicate, so a
  predicate can hold several sources' values and the single-value reads pick by time, not stored order; a statement
  without `Source` or `Timestamp` is refused on every lane, the typed client no longer reads the clock, and a reconcile
  names its source; derived statements take the triggering message's time, and an empty create on the mutation or
  in-process lane is refused; an older stream arrival from the same source no longer overwrites its statements;
  stored values lose `version`. Mutation-lane births keep the pin's no-hierarchy behavior (ruling B).
- The readiness envelope loses `phase`, `revision` and `last_synced` and gains `published_at` (D16); semsource's
  reads of the two it uses go empty without a compile error, and it changes on adoption (ruling F).
- Later changes inherit port-refactor items: rule's event emission, its `version` read and its lifecycle-manager wiring
  (change 6); `service`'s dependency copy (change 3); graph-query's suffix resolution, the envelope's producers and
  the verb table's generalisation (change 4); `fusionnats`' unwrap (change 5).
- Consumers edit imports for what moved out of `graph` (D16), semboids' sim takes the lifecycle manager through its
  constructor (D17), semteams acquires `TOOL_CALL_OUTCOMES` itself (D16), and raw-wire mutation callers stamp `source`
  and `timestamp` and name a reconcile's source (D15); each finds out from a compile error or a typed refusal
  (§9.13), except semsource's readiness reads, which go empty silently (D16, §9.13 (l)).
