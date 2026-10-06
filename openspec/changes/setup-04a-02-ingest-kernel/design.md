# Design: setup-04a-02-ingest-kernel

Status: **draft, not reviewed.** Written in one pass with the inventory (`inventory.md`, this folder), at the
caller's request; the architect contract orders an `INVENTORY PASS` before any design, so this text is conditional on
that pass and on the independent pre-owner design review. Nothing here is approved. The slicing, the owner
definition and the ledger conventions are the foundation design's
(`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md`, cited "foundation D*n*"); change 1
(`openspec/changes/archive/2026-10-05-setup-04a-01-floor/`, PR #48) is cited "change 1 D*n*". Inventory sections are
cited "§n"; probes "P-n" (§1).

## Purpose and admission

Foundation D2 row 2 and issue #91 fix the scope: the 17 packages of closure(graph-ingest) less change 1, `graph/llm`
and `model/wire` carried dormant (foundation D8), the repair rows foundation D7 places here (#15, #16's `graph` and
graph-ingest half, #19, #20, settlement, Q13, #29, #33, SS#1411), and the deltas of foundation D10.2. The claim the
change proves: **graph-ingest runs as a component under the lifecycle suite, with no boot path** — built through its
own factory, started against `natsfixture`, and driven by `lifecycletest.Run` with a failing factory, as the pin's own
tests drive it.

Admission gates: `task verify` green (incl. the new `cover:check` targets, D11); graph-ingest green under the
lifecycle suite including the failed-start check; every helper that runs background work green under its
`background-work` tests (D5); each repair row's proving test green (D8); the ledger rows valid under `task
ledger:check`. Owner rulings this change follows: #9 items 1–9; foundation (a)–(h); change 1's 1.6/1.7 (services get
the suite, helpers get the three background-work shapes); #9 comments 5968830525 (surface audit), 5972208367
(generic payload); #69 (names, 2026-10-06); #19 Q6, #20 Q7, Q12, Q13, Q18 on #8.

**Order with #92.** PR #92 (claim for #69) renames the floor's outward-facing names to `semengine`. **#92 merges
first.** This change's branch merges `origin/main` after it, and every metric this change ports registers under the
`semengine` namespace from its first commit (D4). If #92 is not merged when this change's port tasks start, task 1.4
holds them (it is the only cross-PR dependency; no file is shared, measured by `gh pr view 92 --json files`, 0 files
at read time).

## Context

Terms used below. A **service** is an owner the engine starts and stops with a `Start` that can fail; it is run
through the lifecycle suite. **Background work** is a goroutine that outlives the call that started it, in code that
is not a service; it takes one of the three shapes of the `background-work` spec (`Run(ctx)`, a joining `Close()`,
`Shutdown(ctx)`). A **guard** is graph-ingest's applied-sequence record (`keyed_ingest.go:75` — `func
guardKey(entityID, stream string) string {`): the last stream sequence applied per entity and stream, kept in memory
and in a key-value bucket so a redelivery is dropped. A **stream generation** is one lifetime of a JetStream stream: a
stream deleted and created again (for example a memory stream lost at a broker restart) starts a new generation whose
sequences restart at 1. **Settlement** is what a consumer does with a delivered message: acknowledge, ask for
redelivery, mark in progress, or terminate (`natsclient/delivery_settlement.go`).

Services in this change: one, `processor/graph-ingest.Component`. The other six owners foundation D2 lists
(`inference.ReviewWorker`, `readiness.Set`, `readiness.Watcher`, `worker.Pool[T]`, `dispatch.BoundedDispatcher`,
`dispatch.KeyedPool`) are helpers that run background work (ruling 1.6), as are three channel-returning watches the
foundation's owner scan did not list (§0, `graph/inference/storage.go:432`, `pkg/lifecycle/manager_query.go:148,191`).

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
| 1 | `pkg/worker` | `internal/worker` | no consumer import; no public signature names it (§2) | adapt (D5, D6) |
| 1 | `graph` | `graph` | consumers import it (72 + 11) | adapt (D3, D6, D9) |
| 2 | `graph/llm` | `graph/llm` | dormant | carry, dormant; see owner question B |
| 2 | `graph/readiness` | `graph/readiness` | consumers import it (2) | adapt (D4, D5) |
| 2 | `graph/structural` | `graph/structural` | `graph/inference` (public) names `structural.Indices` (`detector.go:36`) | adapt (D6) |
| 2 | `internal/componentadmission` | `internal/componentadmission` | internal at the pin | carry; see owner question C |
| 2 | `internal/graphmutation` | `internal/graphmutation` | internal at the pin | adapt (D6; owner question D) |
| 2 | `pkg/dispatch` | `internal/dispatch` | no consumer import; no public signature names it | adapt (D3, D4, D5) |
| 2 | `storage/storeregistry` | `storage/storeregistry` | consumers import it (4) | adapt (D2, D6) |
| 3 | `graph/inference` | `graph/inference` | `processor/graph-clustering.Config` names `inference.Config` (`component.go:77`, change 7) | adapt (D4, D5, D6, D10) |
| 3 | `pkg/projection` | `pkg/projection` | consumers import it (11 + 12) | repair-before-port (#19, #20; D8) |
| 4 | `pkg/lifecycle` | `pkg/lifecycle` | semteams and semboids import it (12) | adapt (D2, D3, D5) |
| 5 | `component` | `component` | consumers import it (76 + 23) | adapt (#29, SS#1411, D6, D9; owner question C) |
| 6 | `processor/graph-ingest` | `processor/graph-ingest` | consumers import it (3 + 8) | repair-before-port (#15, #16, #20, settlement, Q13, #33; D8) |

`component/lifecycle_test_suite.go` is not ported: its ledger row already says `adapt → internal/harness/lifecycletest`;
its one in-set caller, `component/lifecycle_test_support_test.go`, leaves with it. The `internal/lifecyclecleanup`
file row stays as the record of the `natsfixture` adaptation, and the new package row says so (foundation D9).
Dependencies entering `go.mod`: `github.com/sashabaranov/go-openai` (dormant `graph/llm`; `task vuln` scans it) and
`golang.org/x/net` (`model/httpclient.go:7`), at versions that do not downgrade the base's `x/text`, `x/tools` or
`x/vuln` (P-2 found the pin's do).

### D2. Test files that import packages outside the set

P-3 found eight test files the foundation did not count. Each is adapted inside its package row:

- `payloadbuiltins.NewTestRegistry(t)` / `payloadbuiltins.Register(reg)` (graph-ingest: `component_fixture_test.go:66`,
  `factory_registry_test.go:122`, `merge_entity_integration_test.go:369`, `resident_stamp_integration_test.go:51`,
  `registered_type_gate_integration_test.go:78,106,122,166`; `pkg/lifecycle/harness_gate_integration_test.go`) become
  `payloadfixture.NewWithSubset(t, <the per-package RegisterPayloads the test needs>)`. That is D1's adopter path (#33)
  exercised in the tests that boot the component.
- `agentic` and `agentic/research` types used as sample payloads (`component_fixture_test.go:54`,
  `registered_type_gate_integration_test.go:110-147`) become a test-registered payload type with the same indexing
  profile (`payloadfixture.RegisterTestType`). `indexing_profile_registry_test.go` asserts the indexing profile of 20
  agentic and research payloads (`:96-116`): that is the agentic domain's contract, not graph-ingest's; the file is
  `defer-exclude` with that reason, and the test that graph-ingest reads a payload's declared profile is kept through a
  test-registered type.
- `storage/storeregistry/storeregistry_test.go:30` — `var _ fusion.StoreResolver = (*storeregistry.Registry)(nil)` —
  is removed here and returns with `pkg/fusion` in change 5 (a `port-refactor` note on both rows).

### D3. The fixture client: one small helper per package

43 sites in five packages use the pin's `natsclient.NewTestClient` family. Change 1 replaced it inside `natsclient`
with an unexported helper (`natsclient/test_helpers_integration_test.go:27-70`). Options:

- (a) **A copy of that helper in each package's tests** (five copies of about 40 lines). No rule changes. Cost: five
  copies to keep in step; a reviewer compares them.
- (b) A shared `internal/harness/clientfixture` that imports `natsclient`. The `harness-boundaries` "Import graph"
  requirement allows a harness package to import a module package only when that package "starts no goroutine and holds
  no resource beyond a call"; `natsclient.Client` holds a connection, so (b) needs that requirement changed.
- (c) Do nothing: not possible; the tests do not compile (P-3).

Recommendation: (a). The rule (b) would relax exists to keep the harness from owning ported resources; five copies of a
connect-and-cleanup helper are cheaper than weakening it. Each copy registers the client's `Close` on `t.Cleanup` under
a bounded context, so the cleanup-roots guard passes.

### D4. Metrics: one collector per key, `semengine`, no global registry

Every live registration moves to `metric.RegisterOrGet` and uses the collector it returns (the AGENTS.md rule; ledger
`metric` row). Live sites: `graph/readiness/gauges.go:149-157` (7), `pkg/dispatch` (`keyed_pool.go:444-460` and the
dispatcher's, 7), graph-ingest (`component.go` 12, `poison_inventory.go` 1). The dead ones are dropped with their
surface (D6): `component/metrics.go` (4), `graph/inference/metrics.go` (7), `pkg/worker/pool.go:133-139` (7).

What a caller observes:

- Every collector this change registers is named `semengine_*` where the pin named it `semstreams_*` (#69 ruling,
  2026-10-06). Metrics with no namespace at the pin (`dispatch_queue_wait_seconds`, the pool's prefix-named ones) keep
  their names: #69 renames the `semstreams` word, it does not add a namespace.
- graph-ingest's collectors are registered on the `component.Dependencies.MetricsRegistry` it is built with. Two
  graph-ingest instances on one registry share each collector (`RegisterOrGet` returns the held one). With a nil
  registry, graph-ingest builds its collectors and registers them nowhere; nothing is registered on
  `prometheus.DefaultRegisterer` (the pin's fallback, `component.go` 12 sites, is removed). This is a declared change:
  at the pin, a nil registry put the series on the process-global registry, and the first registry to ask got them
  for the life of the process (package-level `sync.Once`).
- Tests: `TestGraphIngestMetricsRegisterOnItsRegistry` (two registries, two components: each registry gathers its own
  series; the default registry gathers none of them); `TestGraphIngestNilRegistryRegistersNothing`; a readiness and a
  dispatch test that the returned collector, not the candidate, is the one written (a mutant that writes the candidate
  fails it). The two-instance gauge question is #75's class (shared series with no owner); graph-ingest's gauges are
  set from its own state, so two instances on one registry overwrite each other. That is recorded on the graph-ingest
  row and linked from #75, not fixed here.

### D5. Background work in this change takes the three shapes

Ruling 1.7 replaces foundation D6's "non-context `Stop` → `Stop(ctx)`" rows for the helpers. Per helper, what the
caller observes after the change, and the test (`synctest` bubble, `background-work` "Nothing left behind"):

| Helper | Pin | Shape after the port | What the caller observes |
| --- | --- | --- | --- |
| `worker.Pool[T]` (`internal/worker`) | `Start(ctx)`, `Stop(timeout)`; a timed-out `Stop` leaves workers running, and a second `Stop` panics (P-6) | `Shutdown(ctx)` (workers run the caller's function) | `Shutdown` returns nil once every worker has exited, or `ctx.Err()` first; a later `Shutdown` returns nil once they exit and never panics; `Submit` after `Shutdown` began is refused |
| `dispatch.BoundedDispatcher` | `Stop(ctx)`; with no deadline it waits a fixed 30 s (`dispatcher.go:226-231`) | `Shutdown(ctx)` | the bound is the caller's context only; with no deadline it waits for the join |
| `dispatch.KeyedPool` | `Stop(ctx)` | `Shutdown(ctx)` | as above; lanes drained or `ctx.Err()` |
| `readiness.Watcher` | `Start(ctx)`, `Stop()` waiting unbounded on a goroutine doing KV watch I/O | `Run(ctx)` | the caller runs `Run` on its own goroutine; `Run` returns `ctx.Err()` when the context ends; `Read` keeps its pin behavior |
| `readiness.Set` | `Start(ctx)`, `Stop()` | `Run(ctx)` over its watchers | `Run` returns when every watcher's `Run` has returned |
| `inference.ReviewWorker` | `Start(ctx)`, `Stop()` | `Shutdown(ctx)` (calls an LLM client and the caller's applier) | as `Pool` |
| `inference.NATSAnomalyStorage.Watch`, `lifecycle.Manager.Watch`, `.WatchEvents` | return a channel; the goroutine ends when ctx ends and is never joined | `Run(ctx, func(T))`-shaped watch: the callback runs on the caller's goroutine; returning is the join | no goroutine left after return |

The developer chooses locks and join order, settled by a failing-first test under `-race`; the table states only what
is returned and what is still running. A nil context is refused at the call (an error). Callers inside this change
adapt in the same commit (`dispatcher.go:231`; graph-ingest's readiness use). Callers in later changes are the
`class:port-refactor` rows foundation (h) placed (changes 5 and 7: `fusionnats/client.go:103`,
`processor/graph-clustering/component.go:1232,1239,1260,1266`; `processor/graph-index`, `processor/graph-embedding`,
`processor/rule` for `readiness.NewGauges` only, which does not change). `Manager.WatchEvents` has no reader at the pin
(§2); it is kept under D6's `pkg/lifecycle` reason and adapted the same way.

### D6. Surface audit dispositions

From §2. Admission is per package; only surface nothing reads is removed (#9 comment 5968830525).

- **Dropped (dead, no reader in an admitted package or a consumer):** `component.NewProcessorMetrics` and
  `ProcessorMetrics`; `inference.NewReviewMetrics` and `ReviewMetrics`; `worker.WithMetricsRegistry` with the pool's
  metrics struct and `metricsUpdater` goroutine; the 20 `graph/errors.go` sentinels; `graph.IncomingEdges` and its six
  methods; `component.{GetString,GetInt,GetBool,GetFloat64,ValidateJSONSize,ValidateComponentConfig,
  ValidateAndPersistComponentConfig,IsLifecycleComponent,Registerable}`, `LogLevel*`, `LogEntry`; the remaining
  `graph`, `graph/structural`, `graph/inference`, `internal/graphmutation` (`IsCommitUnknown`), `pkg/projection`
  (`EntityCreator`, `TripleAppender`), `storage/storeregistry` (`Registry.Instances`) and graph-ingest
  (`Component.MergeEntity`) entries of the appendix. Each drop is confirmed by `gopls references` first (open evidence
  question 2); a test that read only the dropped symbol leaves with it and is named on the row.
- **Kept with a reason:** `pkg/lifecycle`'s unread `Manager` methods — the package is the durable-execution primitive by
  owner ruling (#8 Q16; foundation (a): "`pkg/lifecycle` + `pkg/projection` are the primitive"), and #24, unblocked by
  this change (foundation (d)), decides its surface. `model`'s agentic-only surface — #32 (change 7) decides the seam;
  cutting it now is the blind cut Q4 forbids. `graph/inference.RegisterPayloads`, `pkg/lifecycle.RegisterPayloads` —
  the per-package registration D1 makes the consumer's call (#33). `component.MergePortConfig` — read by six
  non-admitted pin files only, but it is the port-merge half of the component configuration contract; **dropped
  unless the inventory reviewer finds an admitted reader** (it has none at the pin).
- **Config (b):** graph-ingest refuses an unknown configuration key at construction, naming the key
  (`TestCreateGraphIngestRefusesUnknownKey`; at the pin the key is ignored). `IngestLanes < 1` keeps the pin's clamp
  to 1, a declared degrade (the schema and `Validate`'s comment say so), now with a test that an explicit 0 builds a
  one-lane component. Every kept field has a test that fails when it is ignored.
- **Described, not implemented (c):** `processor/graph-ingest/TEST_DISPUTE.md` is not ported. The five READMEs are
  read claim by claim in each port task; a claim no code implements is removed or the gap filed.
- **Generic payload:** no `NewGenericJSON` or `GenericJSONPayload{` construction in the 17 (search in §2's file list,
  empty), so no `adapt` item under #9 comment 5972208367.

### D7. Failed-start rollback (`internal/lifecyclecleanup`)

Ported as `carry` under `internal/`: graph-ingest's `Start` calls it (`component.go:984`) and keeps the pin's
behavior — on a failed start, cleanup runs synchronously under a fresh five-second context that keeps the parent's
values; startup and rollback errors are both returned (`errors.Join`); when rollback fails, the component keeps its
obligations and the next `Stop` retries them. The five-second budget is a terminal finalization budget, which
`background-work` "No fixed shutdown timeout" allows ("Terminal finalization work with no caller context … MAY run
under a budgeted context").

Two homes remain: `natsfixture.rollback` (15 s, `internal/harness/natsfixture/rollback.go:17`) and this package. They
are kept apart: `natsfixture` may import no module package ("Import graph"), and the two budgets bound different work
(Docker teardown, component cleanup). Recorded on both ledger rows.

Issue #77 asks who owns rollback for **external** component owners. This change needs no ruling to proceed: both #77 options
leave graph-ingest's internal call as it is (an exported helper would move this package, in this change or later; a
manager-owned rollback lands with `service` in change 3). The question is framed for the owner below (A).

Tests: the pin's `lifecyclecleanup_test.go` (93 lines) as is; graph-ingest's `TestStartRollbackFailureLeavesStopToFinish`
(new, failing first on the adapter): a `Start` whose dependency fails and whose cleanup is made to fail returns both
errors; the adapter's `Observe` lists what is still held; a following `Stop` with a working dependency returns nil and
`Observe` lists nothing.

### D8. Repair rows and what proves each

| Row | Behavior after the change (spec home) | Proving test (failing first on the pin's code) |
| --- | --- | --- |
| #20 (Q7) | a KV `Update` failure that is not a revision conflict or not-found reaches the caller as commit-unknown, through graph-ingest's reply and `projection.MutationClient` (`projection-mutation`) | `natsfixture.FaultKV.FailAfter(Update, timeout)` injected into graph-ingest's entity bucket by a `_test.go` setter (foundation D4-A); the typed client returns `CommitUnknown`; conflict and not-found still return not-committed |
| #19 (Q6) | `ReconcileMutation` carries an expected revision; a stale one returns revision-conflict naming both revisions and changes nothing (`projection-mutation`) | read at R, another writer commits R+1, reconcile at R → conflict, entity unchanged at R+1; reconcile at the current revision → applied |
| #15 | the guard keys on the stream generation; within one generation a sequence not newer than the last applied is stale; a new generation never suppresses current source state (`graph-ingest-recovery`) | memory stream + `Fixture.Restart` + file-backed guard bucket: re-ingestion at lower sequences after the restart is applied and queried back; a redelivery within one generation is acknowledged without change |
| Settlement (Q18) | graph-ingest acknowledges only after its effect and durable guard stamp are committed; a long apply signals progress so it is not redelivered (`graph-ingest-recovery`) | `prochost` kills graph-ingest between apply and acknowledgement on a file stream; after restart the input is redelivered, entity state equals one application; a long-apply test that the input is not redelivered while applying |
| Q13 (SS PR #1437 head `0ea823a6`) | a payload that fails validation or panics in its own code on the Graphable lane is poison — counted, logged, terminated — never a redelivery loop (`graph-ingest-recovery`) | the PR's `fact_lane_fence_test.go` and `_integration_test.go`, adapted to a test-registered payload type (they import `agentic`) |
| #16 (graph half) | the reserved request subjects are declared once in `graph`; no other non-test file spells one (`graph-transport-boundary`) | `TestReservedSubjectsDeclaredOnce` in `internal/harness/contract` with its sensitivity test; the composition refusal and the PubAck-collision demonstration are change 3's (`composition` lands there) |
| #33 | graph-ingest's refusal names the per-package call, not `payloadbuiltins.Register` (`component-registration`) | factory test asserting the refusal names `inference.RegisterPayloads` |
| #29 | `component` reaches no agentic package (`component-registration`) | `go list -deps ./component` has no `agentic` path, in a contract test; I8 already forbids a SemStreams import |
| SS#1411 | one owner-lifecycle guard; graph-ingest's `lifecycleUsed` copy is the only one in this change (foundation P19: 1) | the lifecycle suite over graph-ingest |

The "delivery limit exhausted → parked and visible" scenario of the drafted `graph-ingest-recovery` delta needs
`internal/maxdelivery` (change 7). Under ruling (g) it is not in this change's delta; the row names change 7.

How the guard learns the generation is the developer's choice under one constraint: it is **observed from the
server**, not configured by the operator (inventory "Prefer observation to prediction"). The pin's silent degrades on
this path — no guard bucket means every message is first-seen (`keyed_ingest.go:267`); a short stored value is treated
as first-seen (`:277`) — become declared: a log line and a counter each, with a test.

### D9. Deployment authority, names and literals

- `graph/inference.HierarchyConfig` loses its exported `Org` and `Platform` (`hierarchy.go:100-101`); the hierarchy
  inference takes the carrier `types.PlatformMeta` from graph-ingest's `deps.Platform` as a constructor argument and
  keeps it unexported. graph-ingest's two unexported strings (`component.go:772-773`) become one unexported
  `types.PlatformMeta`. `TestNoSecondAuthorityField` then passes for every live package.
- `graph/llm.EntityParts.{Org,Platform}` (dormant) fail the same test: owner question B.
- `component.Registry`'s access-token parameter fails `TestPublicSignatures`: owner question C.
- Wire and storage names holding `semstreams` (`graph/constants.go:74`, `internal/graphmutation/protocol.go:12`,
  `graph/events.go:19`, `graph/kvcatalog.go:134`): owner question D.
- `TestOneImagePin` flags six `nats:<subject>` port identifiers in `component` tests: D10.
- The 11 fixed broker addresses, 28 sleeps, 6 skips and 20 unbounded cleanups (P-4) are repaired in the port as change
  1 D8 did: sleeps become waits on a channel, a callback or a `synctest` bubble; the six skipped tests are rewritten
  as integration tests on `natsfixture`; addresses come from the fixture.

### D10. The image-pin check reads Go files that can run a container

`TestOneImagePin` treats every `.go` file as one that configures Docker. `component`'s port identifiers
(`component/port_nats.go:14` — `return fmt.Sprintf("nats:%s", n.Subject)`) are `nats:<subject>`, which has the shape
of an image reference: `nats:in` is a valid image tag, so no pattern can tell them apart. Options:

- (a) **Narrow the check's Go scope** to files that can start a container: files importing `testcontainers-go`, a
  Docker client, or `os/exec`. T-B1 already confines container libraries to `internal/harness`. Cost: an image string
  defined in a constant in a file that imports none of them and used elsewhere is missed; the sensitivity test plants
  `nats:2.10` in an `os/exec` file and requires a failure.
- (b) Rewrite the six test expectations so no literal starts `nats:` (build them from the port type). Cost: the
  expected value comes from the code under test, which weakens the oracle.
- (c) An inline exemption marker: SemEngine removed those (`scripts/lint-test-ports.sh:45`, "there is no inline marker").

Recommendation (a), as a `harness-boundaries` modification.

### D11. Gates this change adds

- `scripts/cover-check.sh` targets (foundation D10's critical list members in this set): `processor/graph-ingest
  merged`, `graph merged`, `graph/readiness unit`, `pkg/projection unit`, `internal/graphmutation unit`, `component
  merged`, `storage/storeregistry unit`, `pkg/lifecycle merged`, `graph/inference unit`, `graph/structural unit`.
  Unit coverage at the pin (P-7) is below 80% for six of them (graph/inference 47.8%). Dead-surface removal and the
  repair tests raise it; where a package stays below 80% after them, that is a finding on this pull request, never a
  reason to drop it from the list (03B D10).
- `TestReservedSubjectsDeclaredOnce` (D8) and the `component` no-agentic test (#29).
- The `revive` package-comment lint covers the 14 public packages (two dormant).

### D12. Guidance that returns with these packages

Carried, adapted to SemEngine's names and rules, into `.agents/contracts/semengine-developer.md` and
`semengine-reviewer.md`: the pin developer contract's "Semantic identity and graph contracts" (`:109-126`), "Payload
registry" (`:234-242`), "State ownership and component wiring" (`:243-252`); the reviewer's "Semantic identity and
graph review" (`:155-169`), "Payload registry" (`:227-235`), "Graph and state ownership" (`:236-245`), "Component and
schema wiring" (`:246-253`). Skills carried into `.agents/skills/`: `entity-or-bucket`, `kv-or-stream`, `new-payload`,
`query-pattern`. `orchestration-check` waits for the rule core (change 6). Each carried rule that adds a
repository-wide or agent-conduct rule adds its AGENTS.md row with what enforces it.

## Owner questions

Each is written for the owner; the recommendation comes first.

**A. (#77, framed only; this change does not wait on it.) When a component fails to start, who cleans up what it had
already opened — the component, using a helper the engine exports, or the engine's component manager, for every
component?** Recommendation: the component manager. Reasons: an outside component author then has nothing to import
and nothing to remember; the four behaviors #77 lists (values kept, a bounded budget, both errors reported, leftovers
kept for the next Stop) are written and tested once, at one boundary, instead of in every component. Cost of the
other answer (an exported helper): every component author must call it in the right place and keep those four
behaviors themselves, and a component that forgets leaks with no error. Cost of the recommended answer to you: the
lifecycle rule from #38 ("a failed Start holds nothing") gains one stated exception — when cleanup itself fails, what
is left is held until the next Stop — and the manager work lands in change 3 with `service`. #77's own inventory is
running; its reviewed inventory should settle this, not this change.

**B. The dormant `graph/llm` package has an exported `EntityParts` type with `Org` and `Platform` fields, which the
deployment-authority check (`TestNoSecondAuthorityField`) refuses. Should the spec list it as a temporary exception
until change 7 deletes the package?** Recommendation: yes, a named exception that change 7 removes. Reasons: the code
is carried untouched until the capability seam (#32) decides it; nothing in SemEngine runs it; a named exception is
visible in the spec and is removed with the package. Cost of the other answers: editing code nobody runs that change 7
deletes, and changing its prompt template (`graph/llm/prompts.go:47` reads `{{.Org}}`); or cutting `graph/llm` out of
`graph/inference` now, which is the blind seam cut Q4 ruled out. Cost to you: one more exception line in a rule you
wanted to have none.

**C. The component registry has four methods (`CreateComponent`, `SealComposition`, `Snapshot`, `Snapshots`;
`component/registry.go:193,475,824,842`) that take an empty internal "access" value, so only engine code can call
them. The rule that a public API never names an internal type refuses that. Allow it as a stated exception?**
Recommendation: yes — an exception for parameters of one empty struct type in `internal/componentadmission`, listed
by method. Reasons: it is the pin's deliberate way of keeping consumers from creating components behind the component
manager's back; the type has no fields, so no caller is missing anything it could construct; the exception is narrow
and tested. Cost of the other answers: moving those methods behind an internal hook (a function variable set at
startup, typed `any`, harder to read and to review); or dropping the guard, after which a consumer can create
components the manager does not know about. Cost to you: the rule you ruled (#9 comment 5953477174) gains its first
exception.

**D. Four names on the wire or in storage still say `semstreams`: the configuration bucket `semstreams_config`, the
mutation interface type `semstreams.graph.mutation`, the alert digest domain `semstreams.graph.alert.v1`, and the
bucket description "SemStreams runtime configuration". Rename them to `semengine` in this change, under the #69 rule?**
Recommendation: yes. Reasons: it is the same rule you gave for metrics and the IRI base — one name everywhere — and
the same reason holds: no consumer needs its stored data kept. Cost of not renaming: a second rename later, after a
consumer pins SemEngine. Cost to you and consumers: semsource spells two of them itself (`cmd/semsource/run.go:788`,
`internal/cutover/buckets.go:58`) and edits them at its SemEngine pin bump; a running deployment clears its NATS volume.

## Premises (each with its measurement)

- P1. The set is 17 + 2 packages, 30,613 + 2,024 lines, 154 test files. — §0, `go list -deps` and `measure.sh`.
- P2. No live production context root is in the set. — §0, grep with comment lines removed.
- P3. Change 1 removed only the registration methods this set uses. — P-2, build of the copy.
- P4. Eight test files import packages outside the set. — P-3.
- P5. graph-ingest passes the suite's eight checks at the pin, observed through return values only. — P-5.
- P6. A timed-out `Pool.Stop` followed by a second `Stop` panics at the pin. — P-6.
- P7. No unit flake at the pin over five shuffled runs ×3 and one race run. — P-1.
- P8. The repository's checks fail on the copy exactly as listed. — P-4.
- P9. `component.NewProcessorMetrics`, `inference.NewReviewMetrics` and `worker.WithMetricsRegistry` have no caller
  anywhere at the pin. — §2, `rd` and `grep -rn` (declaration lines only).
- P10. `graph/inference` is public by signature (`processor/graph-clustering/component.go:77`); `graph/structural` by
  `graph/inference/detector.go:36`; `pkg/dispatch` and `pkg/worker` by neither (no consumer import, no exported
  signature in a public package names them). — §2 and the grep in the inventory scratch.
- P11. No `Hash` caller (#78), no storage-report observer (#85) and no second `natsclient.Client` (#75) in the set. —
  `grep -n '\.Hash()'` and `StorageReportObserver` over the 17: empty; observers only in `service` (change 3).

## Invariants and their spec homes

- Commit classification (I5): not-committed only for a rejection proven before any storage effect; otherwise
  commit-unknown — `projection-mutation`, "Commit ambiguity is preserved".
- Conditional reconcile (I6) — `projection-mutation`, "Conditional reconcile at a caller-observed revision".
- Generation-aware replay (I3) — `graph-ingest-recovery`, "Replay protection is generation-aware".
- Settlement order (I9) — `graph-ingest-recovery`, "Settlement order" and "Recovery on a file stream is redelivery".
- Accepted is not durable (I7) — `graph-ingest-recovery`, "Acknowledged is not durable".
- One reserved-subject declaration (I2, first half) — `graph-transport-boundary`.
- Per-package registration (I1 adopter path) — `component-registration`.

## Related issues this change does not close

Issues #75 (shared series; D4 records graph-ingest's gauges under it), #78 and #85 (no caller in the set, P11), #81
(new and repaired tests in this change carry `// Requirement:` citations from the start, in #81's form; carried tests
wait for #80's scope ruling), #24 (unblocked when this change merges, foundation (d)), #77 (question A).

## Declared costs

- The largest port so far: 19 packages, 154 test files, 43 fixture-client sites, 28 sleeps, 20 unbounded cleanups, 8
  test files with out-of-set imports.
- Five copies of the fixture-client helper (D3).
- Seven helpers change shape (D5); their later callers are port-refactor rows in changes 5 and 7.
- `go-openai` becomes a direct dependency for changes 2–7.
- Coverage below 80% at the pin for six critical packages; the gate may be red until tests are added.
- Four owner questions; B, C and D change specs or wire names and gate tasks 1.5–1.7.
- graph-ingest metrics change name (`semengine_*`) and stop appearing on the process-global registry.
