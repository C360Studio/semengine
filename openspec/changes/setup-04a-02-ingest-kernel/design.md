# Design: setup-04a-02-ingest-kernel

Status: **draft, round 4.** Round 2 answered the 17 findings of the pre-owner design review's round 1 and went to the
owner with questions A–F. Round 3 applied the owner's rulings of 2026-10-07 on B, C, E and F (#91 comments 6035429806
and 6035477895) and the pin probe P-9 that ruling B asked for (`inventory.md` §8). Round 4 applies #77's ruling
(#77 comment 6035317931), which answers question A: D7 is rewritten and `lifecycle-suite` gains a delta. Nothing here
is approved; the owner's acceptance on #91 comes first.

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

## Purpose and admission

Foundation D2 row 2 and issue #91 fix the scope, as the owner's rulings amend it: of the 17 packages of
closure(graph-ingest) less change 1, the 15 left once `pkg/worker` (ruling E) and `internal/componentadmission` (ruling
C) are not ported; from `graph/llm`, only the file `graph/inference` reads (ruling B, D1a); `model/wire` not ported
(D1a); the repair rows foundation D7 places here (issues #15, #19, #20, #29 and #33, the `graph` and graph-ingest half
of #16, settlement, Q13 and SS#1411); and the deltas of foundation D10.2. The claim: **graph-ingest runs as a component
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
comments 6035429806 (E, F) and 6035477895 (B, C).

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
| 0 | `model` | `model` | consumers import it (3) | carry: whole until #32 decides the seam (D6) |
| 0 | `model/wire` | — | nothing ported here reads it (D1a, P-9) | not ported in change 2 (D1a, following ruling B's reasoning); `defer-exclude` row naming change 7 |
| 0 | `graph/llm` | `graph/llm` | `graph/inference` (public) names `llm.Client` in `ReviewWorkerConfig.LLMClient` (`review_worker.go:69`) | adapt: `client.go` only (ruling B, D1a) |
| 0 | `storage` | `storage` | consumers import it (20) | carry |
| 1 | `pkg/worker` | — | no reader once `BoundedDispatcher` goes (§2) | not ported (ruling E); `defer-exclude` row |
| 1 | `graph` | `graph` | consumers import it (72 + 11) | adapt (D3, D6, D9) |
| 2 | `graph/readiness` | `graph/readiness` | consumers import it (2) | adapt (D4, D5, D6) |
| 2 | `graph/structural` | `graph/structural` | `graph/inference` (public) names `structural.Indices` (`detector.go:36`) | adapt (D6) |
| 2 | `internal/componentadmission` | — | its only reader is the token parameter ruling C removes (D14) | not ported (ruling C); `defer-exclude` row |
| 2 | `internal/graphmutation` | `internal/graphmutation` | internal at the pin | adapt (D6, D9) |
| 2 | `pkg/dispatch` | `internal/dispatch` | no consumer import; no public signature names it | adapt (D3, D4, D5, D6; its own `ErrStopped`, ruling E) |
| 2 | `storage/storeregistry` | `storage/storeregistry` | consumers import it (4) | adapt (D2, D6) |
| 3 | `graph/inference` | `graph/inference` | `processor/graph-clustering.Config` names `inference.Config` (`component.go:77`, change 7) | adapt (D4, D5, D6, D9; 47.9% coverage recorded, gate in change 7, ruling F) |
| 3 | `pkg/projection` | `pkg/projection` | consumers import it (11 + 12) | repair-before-port (#19, #20; D8) |
| 4 | `pkg/lifecycle` | `pkg/lifecycle` | semteams and semboids import it (12) | adapt (D3, D5) |
| 5 | `component` | `component` | consumers import it (76 + 23) | adapt (#29, D6, D9, D14) |
| 6 | `processor/graph-ingest` | `processor/graph-ingest` | consumers import it (3 + 8) | repair-before-port (#15, #16, #20, settlement, Q13, #33, SS#1411; D8, D13) |

New, not from the pin: `internal/lifecycleguard` (D13), recorded on graph-ingest's row as its SS#1411 repair item.
`component/lifecycle_test_suite.go` is not ported: its ledger row says `adapt → internal/harness/lifecycletest`; its one
in-set caller, `component/lifecycle_test_support_test.go`, leaves with it. One dependency enters `go.mod`:
`golang.org/x/net` (`model/httpclient.go:7`), at a version that does not downgrade the base's `x/text`, `x/tools` or
`x/vuln` (P-2). `github.com/sashabaranov/go-openai` does not enter: its only importer in the closure is the part of
`graph/llm` ruling B leaves behind (P-9).

### D1a. Ruled deviation from foundation D8: `graph/llm` and `model/wire`

Foundation D8 carried `graph/llm` and `model/wire` whole and unused ("dormant") from change 2 to change 7, because
`graph/inference` imports `graph/llm`. **Owner ruling B (#91 comment 6035477895) modifies D8 for `graph/llm`: port only
what `graph/inference` reads from it.** P-9 (`inventory.md` §8) measured that at the pin:

- `graph/inference`'s non-test code reads `llm.Client` (`review_worker.go:40` — `llmClient llm.Client`, and `:69`, the
  exported `ReviewWorkerConfig.LLMClient`), `llm.ChatRequest` (`:430`) and `llm.Config` (`config.go:123`, the field
  `ReviewConfig.LLM`, JSON key `llm`). Its tests read `llm.Client`, `llm.ChatRequest` and
  `llm.ChatResponse` (`review_worker_test.go:30,42`).
- `Client`, `ChatRequest` and `ChatResponse` are the whole of `graph/llm/client.go` (65 lines; its one import is
  `context`). That file is ported; the other seven (`config.go`, `content_fetcher.go`, `doc.go`, `openai_client.go`,
  `prompt_data.go`, `prompt_types.go`, `prompts.go`), the two test files that test the OpenAI client, and the README
  stay behind until change 7.
- `ReviewConfig.LLM` (the `review.llm` key) is dropped, so a configuration that sets it is refused by
  `RejectUnknownKeys` (D6). P-9 found no reader of it anywhere in the pin, graph-clustering included: the only
  `\.LLM` hit in a non-test file is the doc-comment example `graph/inference/doc.go:51`. graph-clustering builds its
  review client from the model registry (`processor/graph-clustering/component.go:2476` — `func (c *Component)
  resolveReviewLLMClient() llm.Client {`) and reads other `Review` fields, not `LLM` (`:1081`, `:2456-2460`).
  The ruling's note that graph-clustering reads the key does not hold at the pin; the drop stands on either reading.
  Round 2's count of 38 read `graph/inference.Config` fields included this one through the word "LLM" in log text
  (§8): the corrected count is 37 read and 6 unread.
- `client.go`'s package comment (`:1-10`) describes summarization, answer generation and the OpenAI SDK, none of which
  the ported file does; it is rewritten to describe the interface (surface audit (c)). The `doc.go:45-56` example in
  `graph/inference` loses its `cfg.Review.LLM` lines.
- No exception to the deployment-authority rule is needed: `EntityParts` (`prompt_types.go:14-15`) is not ported.

**`model/wire` admits the same cut, and it is cut to nothing.** P-9: at the pin, `model/wire`'s non-test importers are
`graph/llm/openai_client.go`, its own sub-package `model/wire/responses`, and `processor/agentic-model` (cut from the
engine, #8 Q4). In the graph-ingest closure the only edge into it is `graph/llm → model/wire` (`go list -deps -f
'{{.ImportPath}} {{.Imports}}' ./processor/graph-ingest`). Once `openai_client.go` stays behind, no package this change
ports reads `model/wire`, so, following ruling B's reasoning (port what the admitted packages read), none of it is
ported. It is ledgered `defer-exclude`, naming change 7 and the seam (#32) as where it returns if it returns. The same
holds for `go-openai`, whose only importer in the closure is `graph/llm`.

What this changes from foundation D8: change 2 carries no dormant package; no row says "dormant until #32"; `go.mod`
does not gain `go-openai`, so `task vuln` does not scan it for changes 2–6; foundation D8's "excluded from coverage
targets" no longer applies (`graph/llm` after the cut declares one interface and two structs and has no statements,
so it has no coverage figure and no target, D11). What it costs: change 7 ports the rest of `graph/llm` and
`model/wire`, if the seam keeps them, instead of deleting copies; that cut was made at the pin, the "blind" cut #8 Q4
was written to avoid, which the owner accepted in ruling B.

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
`component/metrics.go` (4), `graph/inference/metrics.go` (7); `pkg/worker/pool.go:133-139` (7) is not ported
(ruling E).

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
| `inference.ReviewWorker` | `Start(ctx)`, `Stop()` | `Shutdown(ctx)` | as `KeyedPool` |
| `lifecycle.Manager.Watch`, `.WatchEvents` | return a channel; the goroutine ends when ctx ends and is never joined | a watch whose callback runs on the caller's goroutine; returning is the join | no goroutine left after return |

Dropped, so no shape: `dispatch.BoundedDispatcher` (and with it its fixed 30 s default wait, `dispatcher.go:226-231`),
`readiness.Set`, `inference.NATSAnomalyStorage.Watch` (D6); `worker.Pool[T]`, whose second `Stop` after a timed-out
one panics at the pin (P-6), is not ported (ruling E). `lifecycle.Manager.Watch` and `WatchEvents` have **no
reader in the admitted set** (§2: `Watch` is read only by `processor/gated-dag/executor.go:119` and
`gateway/lifecycle-gateway/handlers.go:475`; `WatchEvents` by nobody); they are adapted because D6 keeps
`pkg/lifecycle`'s surface whole (K1), and the shape change is their only change.

The developer chooses locks and join order, settled by a failing-first test under `-race`. A nil context is refused at
the call (an error). Callers inside this change adapt in the same commit (graph-ingest's `KeyedPool` and readiness
use). Callers in later changes are the `class:port-refactor` rows foundation (h) placed (changes 5 and 7:
`ReviewWorker` start and stop at `processor/graph-clustering/component.go:2450` and `:1232`; the readiness
watcher's start at `:1512` and `:1527` and stop at `:1260` and `:1266`; `fusionnats/client.go:140` (start) and `:103`
(stop)).

### D6. Surface audit dispositions

From §2, read by type (the appendix lists all 186 with a disposition). Admission is per package; only surface nothing
reads is removed (#9 comment 5968830525).

- **Not ported by ruling:** `pkg/worker` whole (ruling E), and with it its appendix rows
  (`worker.WithMetricsRegistry`, `Pool.SubmitBlocking`); `internal/componentadmission` whole (ruling C, D14); the
  seven files of `graph/llm` other than `client.go` (ruling B, D1a).
- **Dropped (the rest of the 134 plus the transitive drops of §2):** among them `component.NewProcessorMetrics` and
  `ProcessorMetrics`; `inference.NewReviewMetrics` and `ReviewMetrics`; the 20 `graph/errors.go` sentinels;
  `graph.IncomingEdges` and its methods;
  `component.{GetString,GetInt,GetBool,GetFloat64,ValidateJSONSize,ValidateComponentConfig,
  ValidateAndPersistComponentConfig,IsLifecycleComponent,Registerable}`, `LogLevel*`, `LogEntry`, and
  `component/config_validator.go` whole; `component.Registry.Snapshot`; `component.MergePortConfig` (read by six
  non-admitted pin files only); `dispatch.BoundedDispatcher` with `New`, `Config`, `Deps`, `ErrQueueFull`, and
  `KeyedPool.Submit`, `KeyedPool.Stats`; `readiness.Set` with `NewSet`, `Dump`, `Verdict`; in `graph/inference`,
  `ReviewWorker.Pause`/`Resume`, `NATSAnomalyStorage.Watch`/`Cleanup` and the rest of its appendix rows;
  `internal/graphmutation.IsCommitUnknown`; `storage/storeregistry.Registry.Instances`.
  `readiness.Set`'s one reader, `gateway/graph-gateway`, is deferred as consumer-owned (03B D4), not abandoned; the
  readiness ledger row's `known_risks` names it, so its return is recognised as returning surface.
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
  (`DisallowUnknownFields`) as `inference.RejectUnknownKeys` (`graph/inference/config.go:249`; ADR-054), rather than a
  second one.
  `IngestLanes < 1` keeps the pin's clamp to 1, a declared degrade, now with a test that an explicit 0 builds a
  one-lane component. Each of graph-ingest's four fields has a test that fails when the field is ignored.
  `graph/inference.Config`: its six fields that nothing reads (`RunWithCommunityDetection`, `ReviewConfig.BatchSize`,
  `ReviewConfig.LLM`, `RequireLLMClassification`, `RetentionDays`, `CleanupInterval`; §2 b and §8) are dropped with
  their defaults and validation, so a configuration that sets one is refused by `RejectUnknownKeys` rather than
  ignored. `ReviewConfig.LLM` (the `review.llm` key) is the one ruling B names (D1a). Its other 37 fields are read by
  the anomaly detectors, the review worker, the applier and the storage, whose only workload is graph-clustering
  (change 7); their per-field tests are owed by change 7 (ruling F, D11).
- **Described, not implemented (c):** `processor/graph-ingest/TEST_DISPUTE.md` is not ported. `graph/llm/client.go`'s
  package comment and `graph/inference/doc.go`'s `cfg.Review.LLM` example are rewritten (D1a); `graph/llm/README.md`
  is not ported. The four READMEs of ported packages (`component`, `graph`, `types`, `processor/graph-ingest`) are
  read claim by claim in each port task; a claim no code implements is removed or the gap filed.
- **Generic payload:** no `NewGenericJSON` or `GenericJSONPayload{` construction in the 17 (search over §2's file
  list, empty; the set this change ports is a subset), so no `adapt` item under #9 comment 5972208367.

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
| #16 (graph half) | the reserved request subjects are declared once in `graph`; no other non-test file spells one (`graph-transport-boundary`) | `TestReservedSubjectsDeclaredOnce` in `internal/harness/contract` with its sensitivity test; the composition refusal is change 3's |
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
server**, not configured by the operator. The pin's silent degrades on this path — no guard bucket means every message
is first-seen (`keyed_ingest.go:267`); a short stored value is treated as first-seen (`:277`) — become declared: a log
line and a counter each, with a test.

### D9. Deployment authority, names and literals

- `graph/inference.HierarchyConfig` loses its exported `Org` and `Platform` (`hierarchy.go:100-101`); the hierarchy
  inference takes the carrier `types.PlatformMeta` from graph-ingest's `deps.Platform` as a constructor argument and
  keeps it unexported. graph-ingest's two unexported strings (`component.go:772-773`) become one unexported
  `types.PlatformMeta`. `TestNoSecondAuthorityField` then passes for every live package.
- `graph/llm.EntityParts.{Org,Platform}` (`prompt_types.go:14-15`) are not ported (ruling B, D1a), so the check needs
  no exception and its requirement text is unchanged.
- `component.Registry`'s access-token parameter, which fails `TestPublicSignatures` at the pin (P-4), is removed
  (ruling C, D14), so that check needs no exception either.
- **Wire and storage names, applied under #69** (not a question: #69's ruling, "one rule for every outward-facing
  name: semengine", with no consumer needing its stored data kept, covers them). `adapt` items on the rows named:
  `graph` — `BucketSemStreamsConfig = "semstreams_config"` (`graph/constants.go:74`) becomes
  `BucketSemEngineConfig = "semengine_config"`, the bucket description "SemStreams runtime configuration"
  (`graph/kvcatalog.go:134`) names SemEngine, and `alertDigestDomain = "semstreams.graph.alert.v1"`
  (`graph/events.go:19`) becomes `"semengine.graph.alert.v1"`; `internal/graphmutation` — `InterfaceType =
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
  `graph merged`, `graph/readiness unit`, `pkg/projection unit`, `internal/graphmutation unit`, `component merged`,
  `storage/storeregistry unit`, `pkg/lifecycle merged`, `graph/structural unit`. Task 5.2 is ticked only on a green
  `cover:check`. `graph/inference` joins the gate in change 7 (ruling F, #91 comment 6035429806); `graph/llm` has no
  statements after the cut (D1a) and so no target. Per package, from P-7 and P-8:
  - Already over the floor after the drop: graph-ingest 84.0%, `graph` 85.5%, `graph/readiness` 83.6%,
    `graph/structural` 89.6%, `storage/storeregistry` 100%.
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
  - `graph/inference` 47.9% (674/1406), 451 short: not a target in this change (ruling F). Its ledger row records the
    figure and that change 7 owes the tests; a tracking issue holds them (task 6.5). The `ReviewWorker` `synctest`
    test that `background-work` requires (D5) stays in this change.
- `TestReservedSubjectsDeclaredOnce` (D8), `TestNoProcessGlobalRegistration` (`metric-registry`) and the `component`
  no-agentic test (#29), each with an AGENTS.md row.
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
  a failed start. Cost: a new package; graph-ingest's tests that read the fields directly (`lifecycle_owner_test.go`,
  `test_owner_test.go`, `test_owner_child_test.go`, `component_test.go`) read the guard instead.
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

## Owner questions

None open. Every question this design asked is ruled; the list follows.

### Ruled

- **A** — asked on #77 (comment 6024793500): the component cleans up its own failed start with a public helper, the
  managers stay as the second line; the #38 exception is granted and binds only ported components (#77 comment
  6035317931); PR #93 closes #77 (#77 comment 6035358884). Applied in D7 and the `lifecycle-suite` delta.

- **B** — "port": port only what `graph/inference` reads from `graph/llm` (`client.go`); drop `ReviewConfig.LLM`; the
  rest of `graph/llm` and `go-openai` wait for change 7; no authority-rule exception; foundation D8 modified for
  `graph/llm` (#91 comment 6035477895). Applied in D1, D1a, D6, D9; `model/wire` follows its reasoning (D1a).
- **C** — "drop the token": `internal/componentadmission` is not ported; the three registry methods are plain public
  methods whose doc comments direct callers to the component manager; no public-signature exception; a consumer
  creating a component outside the manager is review only (#91 comment 6035477895). Applied in D1, D9, D14.
- **E** — "Leave it out if it's dead code": `pkg/worker` is not ported; `internal/dispatch` declares its own "stopped"
  error (#91 comment 6035429806). Applied in D1, D4, D5, D6.
- **F** — "Agree": `graph/inference` joins the 80% gate in change 7; the `ReviewWorker` `synctest` test stays here; the
  ledger row records 47.9% and a tracking issue holds the tests (#91 comment 6035429806). Applied in D6, D11.

Round 1's question D, the wire and storage names, is answered by #69 and applied in D9.

## Premises (each with its measurement)

- P1. The set is 15 whole packages and `graph/llm/client.go`: 29,963 non-test lines (§0's 30,613 less `pkg/worker`'s
  707 and `internal/componentadmission`'s 8, plus 65), and 152 test files with 40,202 lines (§0's 154 less
  `pkg/worker`'s 2; `graph/llm`'s two test files test the OpenAI client and stay behind). — §0, §8.
- P2. No live production context root is in the set. — §0, grep with comment lines removed.
- P3. Change 1 removed only the registration methods this set uses. — P-2, build of the copy.
- P4. Eight test files import packages outside the set. — P-3.
- P5. graph-ingest passes the suite's eight checks at the pin, observed through return values only. — P-5.
- P6. A timed-out `Pool.Stop` followed by a second `Stop` panics at the pin (a reason for ruling E). — P-6.
- P7. No unit flake at the pin over five shuffled runs ×3 and one race run. — P-1.
- P8. The repository's checks fail on the copy exactly as listed. — P-4.
- P9. 186 exported identifiers have no reader by type in the admitted set or a consumer; `BoundedDispatcher`,
  `readiness.Set` and `lifecycle.Manager.Watch` among them. — §2, the type-based reader and the hand-read consumer
  hits.
- P10. `pkg/worker`'s only non-test importers are `pkg/dispatch/dispatcher.go` and `errors.go` (the basis of ruling
  E). — §2, `grep -rl`.
- P11. `graph/inference` is public by signature (`processor/graph-clustering/component.go:77`); `graph/structural` by
  `graph/inference/detector.go:36`; `graph/llm` by `graph/inference/review_worker.go:69`; `pkg/dispatch` by neither.
  — §2, §8.
- P12. Coverage after the drop is as in P-8: five target packages are at or over 80%; four are short by 10, 31, 81 and
  97 statements; `graph/inference`, not a target here (ruling F), is short by 451. — P-7, P-8 (local-only profiles;
  commands in §1).
- P13. The owner-lifecycle state has 12 copies in the admitted set and none in the four consumers. — §3 category 2.
- P14. No `Hash` caller (#78), no storage-report observer (#85) and no second `natsclient.Client` (#75) in the set. —
  `grep -n '\.Hash()'` and `StorageReportObserver` over the 17: empty; observers only in `service` (change 3).
- P15. `graph/inference` reads from `graph/llm` only `Client`, `ChatRequest`, `ChatResponse` (all of `client.go`) and
  `Config` (through `ReviewConfig.LLM`). — P-9.
- P16. In the graph-ingest closure, `model/wire` and `go-openai` are imported only by `graph/llm`, and within
  `graph/llm` only by files other than `client.go`. — P-9.
- P17. `ReviewConfig.LLM` has no reader at the pin outside a doc-comment example. — P-9.
- P18. `componentadmission.Access` is read in the set only by `component/registry.go` (four methods) and its tests. —
  §8.

## Invariants and their spec homes

- Commit classification (I5): not-committed only for a rejection proven before any storage effect; otherwise
  commit-unknown — `projection-mutation`, "Commit ambiguity is preserved".
- Conditional reconcile (I6) — `projection-mutation`, "Conditional reconcile at a caller-observed revision".
- Generation-aware replay (I3) — `graph-ingest-recovery`, "Replay protection is generation-aware".
- Settlement order (I9) — `graph-ingest-recovery`, "Settlement order" and "Recovery on a file stream is redelivery".
- Accepted is not durable (I7) — `graph-ingest-recovery`, "Acknowledged is not durable".
- One reserved-subject declaration (I2, its first half) — `graph-transport-boundary`.
- Per-package registration (I1, its adopter path) — `component-registration`.
- One-shot owner lifecycle (D13) — `lifecycle-suite`, "Portable floor" (as this change modifies it).
- Failed-start rollback (D7): a nil parent or nil rollback is refused; the rollback sees the parent's values with a
  deadline and no parent cancellation; its error and the budget's expiry are both returned — `lifecycle-suite`,
  "Failed-start rollback helper".
- A failed `Start` whose own cleanup fails reports both failures and keeps what is left for a later `Stop` (D7) —
  `lifecycle-suite`, "Failed start whose own cleanup fails".

## Related issues this change does not close

Issues #75 (shared series; D4 records graph-ingest's gauges under it), #78 and #85 (no caller in the set, P14), #81 (new
and repaired tests carry `// Requirement:` citations in #81's form; carried tests wait for #80's scope ruling), #24
(unblocked when this change merges, foundation (d)), SS#1411 (answered here for SemEngine; the SemStreams issue is not
touched). #77 is ruled and this change closes it (comment 6035358884); D7 applies the ruling.

## Declared costs

- The largest port so far: 16 package directories, 152 test files, 43 fixture-client sites, 19 sleeps, 20 unbounded
  cleanups, 8 test files with out-of-set imports.
- A new harness helper (D3) and a new internal package (D13), each with an adoption list.
- Four helpers change shape (D5); their later callers are port-refactor rows in changes 5 and 7.
- Tests to write to reach the floor: about 220 statements across four packages. `graph/inference` sits at 47.9%,
  below the floor, until change 7 (ruling F).
- Change 7 ports the rest of `graph/llm`, `model/wire` and `go-openai` if the seam keeps them (ruling B, D1a).
- A consumer can call the three registry methods directly; that it does not is review only (ruling C, D14).
- One new public package, `pkg/lifecyclecleanup` (one function), by #77's ruling; that an external component calls it
  correctly is checked only by that component's own tests (D7.1).
- A nil rollback callback becomes an error, a declared change from the pin (D7.2).
- graph-ingest metrics change name (`semengine_*`) and stop appearing on the process-global registry; three wire and
  storage names change (D9).
