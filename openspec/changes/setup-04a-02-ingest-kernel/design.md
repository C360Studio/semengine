# Design: setup-04a-02-ingest-kernel

Status: **draft, round 2, not reviewed.** It answers the 17 findings of the pre-owner design review's round 1 on the
revised inventory (`inventory.md`, this folder). Nothing here is approved; the independent review and the owner's
acceptance on #91 come first.

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

Foundation D2 row 2 and issue #91 fix the scope: the 17 packages of closure(graph-ingest) less change 1 (16 if the
owner accepts question E), `graph/llm` and `model/wire` carried dormant (foundation D8), the repair rows foundation D7
places here (#15, #16's `graph` and graph-ingest half, #19, #20, settlement, Q13, #29, #33, SS#1411), and the deltas of
foundation D10.2. The claim: **graph-ingest runs as a component under the lifecycle suite, with no boot path** — built
through its own factory, started against `natsfixture`, and driven by `lifecycletest.Run` with a failing factory whose
broker refuses the connection. The current `lifecycle-suite` requirement "Observe adapter contract" already binds it
("A service ported from the pin SHALL be run through the suite via a test-side adapter in its package"), so this change
carries no `lifecycle-suite` delta.

Admission gates: `task verify` green, including `cover:check` with this change's targets at the one 80% floor (D11);
graph-ingest green under the lifecycle suite; every helper that runs background work green under its
`background-work` tests (D5); each repair row's proving test green (D8); the ledger rows valid under `task
ledger:check`. Owner rulings this change follows: #9 items 1–9; foundation (a)–(h); change 1's rulings 1.6 and 1.7
(services get the suite, helpers get the three background-work shapes); #9 comments 5968830525 (surface audit) and
5972208367 (generic payload); #69 (outward-facing names, 2026-10-06); #19 Q6, #20 Q7; Q12, Q13, Q16, Q18 on #8.

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
| 0 | `internal/lifecyclecleanup` | `internal/lifecyclecleanup` | internal at the pin | carry (D7; #77 may move it) |
| 0 | `types` | `types` | consumers import it (54 + 20 sites) | carry |
| 0 | `model` | `model` | consumers import it (3) | carry: whole until #32 decides the seam (D6) |
| 0 | `model/wire` | `model/wire` | dormant | carry, dormant (foundation D8) |
| 0 | `storage` | `storage` | consumers import it (20) | carry |
| 1 | `pkg/worker` | — (or `internal/worker`) | no reader once `BoundedDispatcher` goes (§2) | exclude, owner question E (or adapt, D5) |
| 1 | `graph` | `graph` | consumers import it (72 + 11) | adapt (D3, D6, D9) |
| 2 | `graph/llm` | `graph/llm` | dormant | carry, dormant; see owner question B |
| 2 | `graph/readiness` | `graph/readiness` | consumers import it (2) | adapt (D4, D5, D6) |
| 2 | `graph/structural` | `graph/structural` | `graph/inference` (public) names `structural.Indices` (`detector.go:36`) | adapt (D6) |
| 2 | `internal/componentadmission` | `internal/componentadmission` | internal at the pin | carry; see owner question C |
| 2 | `internal/graphmutation` | `internal/graphmutation` | internal at the pin | adapt (D6, D9) |
| 2 | `pkg/dispatch` | `internal/dispatch` | no consumer import; no public signature names it | adapt (D3, D4, D5, D6) |
| 2 | `storage/storeregistry` | `storage/storeregistry` | consumers import it (4) | adapt (D2, D6) |
| 3 | `graph/inference` | `graph/inference` | `processor/graph-clustering.Config` names `inference.Config` (`component.go:77`, change 7) | adapt (D4, D5, D6, D9) |
| 3 | `pkg/projection` | `pkg/projection` | consumers import it (11 + 12) | repair-before-port (#19, #20; D8) |
| 4 | `pkg/lifecycle` | `pkg/lifecycle` | semteams and semboids import it (12) | adapt (D3, D5) |
| 5 | `component` | `component` | consumers import it (76 + 23) | adapt (#29, D6, D9; owner question C) |
| 6 | `processor/graph-ingest` | `processor/graph-ingest` | consumers import it (3 + 8) | repair-before-port (#15, #16, #20, settlement, Q13, #33, SS#1411; D8, D13) |

New, not from the pin: `internal/lifecycleguard` (D13), recorded on graph-ingest's row as its SS#1411 repair item.
`component/lifecycle_test_suite.go` is not ported: its ledger row says `adapt → internal/harness/lifecycletest`; its one
in-set caller, `component/lifecycle_test_support_test.go`, leaves with it. Dependencies entering `go.mod`:
`github.com/sashabaranov/go-openai` (dormant `graph/llm`; `task vuln` scans it) and `golang.org/x/net`
(`model/httpclient.go:7`), at versions that do not downgrade the base's `x/text`, `x/tools` or `x/vuln` (P-2).

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
`component/metrics.go` (4), `graph/inference/metrics.go` (7), `pkg/worker/pool.go:133-139` (7).

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
| `dispatch.KeyedPool` | `Stop(ctx)` | `Shutdown(ctx)` | returns nil once every lane has drained and exited, or `ctx.Err()` first; with no deadline it waits for the join; a later `Shutdown` returns nil; `SubmitBlocking` after `Shutdown` began is refused with `ErrStopped` |
| `readiness.Watcher` | `Start(ctx)`, `Stop()` waiting unbounded on a goroutine doing KV watch I/O | `Run(ctx)` | the caller runs `Run` on its own goroutine; `Run` returns `ctx.Err()` when the context ends and leaves nothing running; `Read` keeps its pin behavior |
| `inference.ReviewWorker` | `Start(ctx)`, `Stop()` | `Shutdown(ctx)` | as `KeyedPool` |
| `lifecycle.Manager.Watch`, `.WatchEvents` | return a channel; the goroutine ends when ctx ends and is never joined | a watch whose callback runs on the caller's goroutine; returning is the join | no goroutine left after return |
| `worker.Pool[T]` (only if question E keeps it) | `Start(ctx)`, `Stop(timeout)`; a timed-out `Stop` leaves workers running and a second `Stop` panics (P-6) | `Shutdown(ctx)` | as `KeyedPool`; a second `Shutdown` never panics |

Dropped, so no shape: `dispatch.BoundedDispatcher` (and with it its fixed 30 s default wait, `dispatcher.go:226-231`),
`readiness.Set`, `inference.NATSAnomalyStorage.Watch` (D6). `lifecycle.Manager.Watch` and `WatchEvents` have **no
reader in the admitted set** (§2: `Watch` is read only by `processor/gated-dag/executor.go:119` and
`gateway/lifecycle-gateway/handlers.go:475`; `WatchEvents` by nobody); they are adapted because D6 keeps
`pkg/lifecycle`'s surface whole (K1), and the shape change is their only change.

The developer chooses locks and join order, settled by a failing-first test under `-race`. A nil context is refused at
the call (an error). Callers inside this change adapt in the same commit (graph-ingest's `KeyedPool` and readiness
use). Callers in later changes are the `class:port-refactor` rows foundation (h) placed (changes 5 and 7:
`fusionnats/client.go:139`, `processor/graph-clustering/component.go:1509,1524`).

### D6. Surface audit dispositions

From §2, read by type (the appendix lists all 186 with a disposition). Admission is per package; only surface nothing
reads is removed (#9 comment 5968830525).

- **Dropped (134 plus the transitive drops of §2):** among them `component.NewProcessorMetrics` and
  `ProcessorMetrics`; `inference.NewReviewMetrics` and `ReviewMetrics`; `worker.WithMetricsRegistry` and
  `Pool.SubmitBlocking`; the 20 `graph/errors.go` sentinels; `graph.IncomingEdges` and its methods;
  `component.{GetString,GetInt,GetBool,GetFloat64,ValidateJSONSize,ValidateComponentConfig,
  ValidateAndPersistComponentConfig,IsLifecycleComponent,Registerable}`, `LogLevel*`, `LogEntry`, and
  `component/config_validator.go` whole; `component.Registry.Snapshot`; `component.MergePortConfig` (read by six
  non-admitted pin files only); `dispatch.BoundedDispatcher` with `New`, `Config`, `Deps`, `ErrQueueFull`, and
  `KeyedPool.Submit`, `KeyedPool.Stats`; `readiness.Set` with `NewSet`, `Dump`, `Verdict`; in `graph/inference`,
  `ReviewWorker.Pause`/`Resume`, `NATSAnomalyStorage.Watch`/`Cleanup` and the rest of its appendix rows;
  `internal/graphmutation.IsCommitUnknown`; `storage/storeregistry.Registry.Instances`.
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
  (`TestCreateGraphIngestRefusesUnknownKey`; at the pin the key is ignored). It adopts the pin's existing shape,
  `inference.RejectUnknownKeys` (`graph/inference/config.go:249`, strict decoding; ADR-054), rather than a second one.
  `IngestLanes < 1` keeps the pin's clamp to 1, a declared degrade, now with a test that an explicit 0 builds a
  one-lane component. Each of graph-ingest's four fields has a test that fails when the field is ignored.
  `graph/inference.Config`: its five fields that nothing reads (`RunWithCommunityDetection`, `ReviewConfig.BatchSize`,
  `RequireLLMClassification`, `RetentionDays`, `CleanupInterval`; §2 b) are dropped with their defaults and
  validation, so a configuration that sets one is refused by `RejectUnknownKeys` rather than ignored. Its other 38
  fields are read by the anomaly detectors, the review worker, the applier and the storage, whose only workload is
  graph-clustering (change 7); their per-field tests are owner question F.
- **Described, not implemented (c):** `processor/graph-ingest/TEST_DISPUTE.md` is not ported. The five READMEs are
  read claim by claim in each port task; a claim no code implements is removed or the gap filed.
- **Generic payload:** no `NewGenericJSON` or `GenericJSONPayload{` construction in the 17 (search over §2's file
  list, empty), so no `adapt` item under #9 comment 5972208367.

### D7. Failed-start rollback (`internal/lifecyclecleanup`)

Ported as `carry` under `internal/`: graph-ingest's `Start` calls it (`component.go:984`) and keeps the pin's
behavior. On a failed start, cleanup runs synchronously under a fresh five-second context that keeps the parent's
values; startup and rollback errors are both returned (`errors.Join`); when rollback succeeds, the component holds
nothing; when rollback fails, the component keeps what it could not release and the next `Stop` tries again. The
five-second budget is a terminal finalization budget, which `background-work` "No fixed shutdown timeout" allows.

The last branch does not meet the current `lifecycle-suite` rule that a failed `Start` holds nothing (#38). Whether it
may is #77's separate grant (comment 6024793500). This change does not write that exception into any spec. It carries
the pin's behavior unchanged, records it on graph-ingest's ledger row under `known_risks` ("a failed `Start` whose
rollback also fails returns both errors and keeps what it could not release until the next `Stop`; pending #77 and its
grant on #38"), and pins it with the pin's own test, carried on the in-package adapter:
`TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop` (`processor/graph-ingest/lifecycle_owner_test.go:134`).
The suite's failed-start check runs on graph-ingest with a failing factory whose cleanup succeeds, and passes as
written.

Two homes remain: `natsfixture.rollback` (15 s, `internal/harness/natsfixture/rollback.go:17`) and this package. They
are kept apart: `natsfixture` may import no module package ("Import graph"), and the two budgets bound different work
(Docker teardown, component cleanup). Recorded on both ledger rows. If #77 is ruled for a public helper before this
design is accepted, the helper's destination is the only thing in this change that moves; otherwise #77's ruling lands
in a later change.

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

**The settlement process-kill test.** There is no boot path, so the process is the test binary itself:
`prochost.Helper` (`internal/harness/prochost/prochost.go:43`) runs a helper function from graph-ingest's
`TestMain` that builds graph-ingest through its factory against the fixture URL passed in the environment, starts it
on a file-backed input stream, and parks on a signal. The window between apply and acknowledgement is held inside the
helper, not raced from outside: in the first run, the helper wraps the guard-record bucket with a `_test.go` wrapper
(set through the same in-package setter as foundation D4-A's KV setter) whose write does not return. The test then
publishes one input, waits until the entity's write appears in the entity bucket, confirms from the consumer's info
that the input is delivered and still pending acknowledgement, confirms the helper is still running
(`Process.Alive`), and kills it (`Process.Kill`). A second helper run, without the wrapper, starts graph-ingest on the
same stream and buckets; the test waits for the input's redelivery to be acknowledged and asserts the entity equals
the state after one application and nothing else changed. No production failpoint is added. Rejected: pausing the
helper with `SIGSTOP` when the entity write appears — the guard write and the acknowledgement follow within the same
call, so the pause can land after the acknowledgement and the test would need retries to catch the window.

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
- `graph/llm.EntityParts.{Org,Platform}` (dormant) fail the same test: owner question B.
- `component.Registry`'s access-token parameter fails `TestPublicSignatures`: owner question C.
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
- The 11 fixed broker addresses, 28 sleeps (19 without `pkg/worker`), 6 skips and 20 unbounded cleanups (P-4) are
  repaired in the port as change 1 D8 did: sleeps become waits on a channel, a callback or a `synctest` bubble; the six
  skipped tests are rewritten as integration tests on `natsfixture`; addresses come from the fixture.

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
  `storage/storeregistry unit`, `pkg/lifecycle merged`, `graph/inference unit` (owner question F), `graph/structural
  unit`. Task 5.2 is ticked only on a green `cover:check`. Per package, from P-7 and P-8:
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
  - `graph/inference` 47.9%, 451 short: owner question F.
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
  owner-lifecycle state and nothing else; rollback stays in `internal/lifecyclecleanup` (D7), which the guard calls on
  a failed start. Cost: a new package; graph-ingest's tests that read the fields directly (`lifecycle_owner_test.go`,
  `test_owner_test.go`, `test_owner_child_test.go`, `component_test.go`) read the guard instead.
- (b) Record the copy as the idiom: each component keeps its own state machine, and the lifecycle suite (already run
  on every service) is the behavior check. Cost: 12 copies of about 60 lines kept in step by review; the divergence
  above already happened.
- (c) A public guard in `component`. Cost: exported surface with no present consumer (no consumer carries the copy),
  which category 4 removes.
- (d) Nothing in this change. Cost: a deviation from foundation D7 that the owner would have to accept.

Recommendation **(a)**. It is what foundation D7 already assigned; it is internal because every adopter is in this
module; whether external component authors get public lifecycle helpers is #77's question (for rollback), and a
ruling there can promote the guard later without changing what it does. Separate from `internal/lifecyclecleanup` so a
ruling that makes the rollback helper public does not drag the guard with it.

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

## Owner questions

Each is written for the owner; the recommendation comes first.

**A. (Framing only; #77 decides it.)** Who cleans up when a component fails partway through `Start`, and may a `Start`
whose cleanup fails return still holding what it could not release? Both questions are posted on #77 (comment
6024793500) with their inventory; this change asks nothing further. Until #77 is ruled, graph-ingest keeps the pin's
behavior, recorded as a known risk and pinned by a test (D7), and no spec here changes.

**B. The dormant `graph/llm` package has an exported `EntityParts` type with `Org` and `Platform` fields, which the
deployment-authority check refuses. Should the spec list it as a temporary exception until change 7 deletes the
package?** Recommendation: yes, a named exception that change 7 removes. Reasons: the code is carried untouched until
the capability seam (#32) decides it; nothing in SemEngine runs it; a named exception is visible in the spec and goes
with the package. Cost of the other answers: editing code nobody runs that change 7 deletes, including its prompt
template (`graph/llm/prompts.go:47` reads `{{.Org}}`); or cutting `graph/llm` out of `graph/inference` now, the blind
seam cut Q4 ruled out. Cost to you: one more exception line in a rule you wanted to have none.

**C. Three component-registry methods (`CreateComponent`, `SealComposition`, `Snapshots`;
`component/registry.go:193,475,842`) take an empty internal "access" value, so only engine code can call them. The
rule that a public API never names an internal type refuses that. Allow it as a stated exception?** Recommendation:
yes — an exception for parameters of one field-less type in `internal/componentadmission`, listed by method. Reasons:
it is the pin's way of keeping consumers from creating components behind the component manager's back; the type has
no fields, so no caller is missing anything it could construct; the exception is narrow and tested. A fourth method,
`Snapshot`, has no reader and is dropped. Cost of the other answers: moving those methods behind an internal hook (a
function variable set at startup, harder to read and review); or dropping the guard, after which a consumer can create
components the manager does not know about. Cost to you: the rule you ruled (#9 comment 5953477174) gains its first
exception.

**E. After removing code nothing reads, the worker-pool package (`pkg/worker`) is used by nothing in SemEngine except
one error value. Leave the package out of this port?** Recommendation: yes; `internal/dispatch` declares its own
"stopped" error with the same text. Reasons: its only user, `dispatch.BoundedDispatcher`, is read only by a gated-DAG
processor SemEngine does not admit; no consumer imports either package; carrying it means repairing a pool whose second
`Stop` panics (P-6), nine sleeps, a metrics goroutine nothing starts, and an 827-line README, for no caller. Cost of
the other answer: about 700 lines and their tests ported and repaired with no reader. Cost to you: the package list you
accepted for change 2 (foundation D2) shrinks from 17 to 16, and a later need for a worker pool comes back as new
surface that needs a present consumer.

**F. `graph/inference` is held to the 80% coverage floor from the change that ports it. It is at 47.9% after removing
dead code, and reaching 80% means tests for about 450 more statements — the anomaly detectors, the review worker, the
anomaly storage and the 38 configuration fields they read — whose only caller, graph-clustering, arrives in change 7.
Add `graph/inference` to the coverage gate in change 7 instead of here?** Recommendation: yes. Reasons: graph-ingest
uses only the package's hierarchy half, which this change tests; tests written now would drive the detectors without
the component that runs them, and change 7 rewrites the call paths they would test (the review worker's `Shutdown`,
the readiness watcher's `Run`); this pull request is already the largest port. The ledger row records the 47.9%
figure, the missing per-field tests and the change that owes them, and a tracking issue holds them. Cost of the other
answer: about 450 statements of tests in this change, mostly table tests of configuration validation and integration
tests of the anomaly storage, written before their caller exists. Cost to you: a package on the ruled critical list
(#8 comment 5932313950: "the gate applies to a package when it is admitted") sits below the floor for five changes,
an exception you would be granting.

(Round 1's question D, the wire and storage names, is answered by #69 and applied in D9.)

## Premises (each with its measurement)

- P1. The set is 17 + 2 packages, 30,613 + 2,024 lines, 154 test files. — §0, `go list -deps` and a line count.
- P2. No live production context root is in the set. — §0, grep with comment lines removed.
- P3. Change 1 removed only the registration methods this set uses. — P-2, build of the copy.
- P4. Eight test files import packages outside the set. — P-3.
- P5. graph-ingest passes the suite's eight checks at the pin, observed through return values only. — P-5.
- P6. A timed-out `Pool.Stop` followed by a second `Stop` panics at the pin. — P-6.
- P7. No unit flake at the pin over five shuffled runs ×3 and one race run. — P-1.
- P8. The repository's checks fail on the copy exactly as listed. — P-4.
- P9. 186 exported identifiers have no reader by type in the admitted set or a consumer; `BoundedDispatcher`,
  `readiness.Set` and `lifecycle.Manager.Watch` among them. — §2, the type-based reader and the hand-read consumer
  hits.
- P10. `pkg/worker`'s only non-test importers are `pkg/dispatch/dispatcher.go` and `errors.go`. — §2, `grep -rl`.
- P11. `graph/inference` is public by signature (`processor/graph-clustering/component.go:77`); `graph/structural` by
  `graph/inference/detector.go:36`; `pkg/dispatch` and `pkg/worker` by neither. — §2.
- P12. Coverage after the drop is as in P-8: five packages are at or over 80%; four are short by 10, 31, 81 and 97
  statements, and `graph/inference` by 451. — P-7, P-8 (local-only profiles; commands in §1).
- P13. The owner-lifecycle state has 12 copies in the admitted set and none in the four consumers. — §3 category 2.
- P14. No `Hash` caller (#78), no storage-report observer (#85) and no second `natsclient.Client` (#75) in the set. —
  `grep -n '\.Hash()'` and `StorageReportObserver` over the 17: empty; observers only in `service` (change 3).

## Invariants and their spec homes

- Commit classification (I5): not-committed only for a rejection proven before any storage effect; otherwise
  commit-unknown — `projection-mutation`, "Commit ambiguity is preserved".
- Conditional reconcile (I6) — `projection-mutation`, "Conditional reconcile at a caller-observed revision".
- Generation-aware replay (I3) — `graph-ingest-recovery`, "Replay protection is generation-aware".
- Settlement order (I9) — `graph-ingest-recovery`, "Settlement order" and "Recovery on a file stream is redelivery".
- Accepted is not durable (I7) — `graph-ingest-recovery`, "Acknowledged is not durable".
- One reserved-subject declaration (I2, its first half) — `graph-transport-boundary`.
- Per-package registration (I1, its adopter path) — `component-registration`.
- One-shot owner lifecycle (D13) — `lifecycle-suite`, "Portable floor" (current spec, unchanged).

## Related issues this change does not close

Issues #75 (shared series; D4 records graph-ingest's gauges under it), #78 and #85 (no caller in the set, P14), #81 (new
and repaired tests carry `// Requirement:` citations in #81's form; carried tests wait for #80's scope ruling), #24
(unblocked when this change merges, foundation (d)), #77 (question A), SS#1411 (answered here for SemEngine; the
SemStreams issue is not touched).

## Declared costs

- The largest port so far: 18 or 19 packages, about 150 test files, 43 fixture-client sites, 28 sleeps, 20 unbounded
  cleanups, 8 test files with out-of-set imports.
- A new harness helper (D3) and a new internal package (D13), each with an adoption list.
- Five helpers change shape (D5); their later callers are port-refactor rows in changes 5 and 7.
- `go-openai` becomes a direct dependency for changes 2–7.
- Tests to write to reach the floor: about 220 statements across four packages, plus question F.
- Owner questions B, C, E and F gate tasks; A is framing only.
- graph-ingest metrics change name (`semengine_*`) and stop appearing on the process-global registry; three wire and
  storage names change (D9).
