# Design: setup-04a — the cut into OpenSpec changes

The cut of the tier-0 port set into OpenSpec changes; architect draft.

Status: architect draft, corrected after the pre-owner design review (`design-review.md`, verdict BLOCKING); for
re-check, then owner acceptance. Nothing here is a ruling. Spec deltas and tasks are named, not drafted; they follow
acceptance. Inventory citations are to `openspec/changes/setup-04a-foundation/inventory.md` at `13b5fee` by section
(§n), never by line. The inventory that passed review is `564bc5e9…`; the committed copy `c6dbfe3a…` differs by
markdownlint formatting only (both checksums are recorded on PR #47).

## Purpose and admission

Epic #9 (Slice 04A) ports the ruled tier-0 set — 65 packages, 140,842 lines at the pin `8b99efe9`, confirmed in §2 —
as "small slices", with every repair row's gate evidence green before its package is admitted (03B Q12; `docs/
setup-plan.md:201-209`: admission gates "cannot be waived"). The question this design answers is the one the inventory
was commissioned for (§0): how the set is cut into OpenSpec changes, in what order, and what the first change
touches. Admission for this design: it is in scope for the architect contract (steps 4–5), it binds nothing, and every
number in it reproduces from the scratch evidence named under Sources with one script.

What is decided here, subject to acceptance: the unit of a change (D1), the chain and its order (D2), the first
change's scope (D3), the harness extension's shape (D4–D6), where each repair row, refactor, and ruled ledger item
lands (D7), what is carried dormant and when the seam closes (D8), the ledger row shape and the context-root triage
(D9), the spec deltas each change carries (D10), what each change makes red in `task verify` (D11), and a
ruling-conformance table (D12) whose deviation rows are owner questions.

## Sources

- The committed inventory (§0–§8, Correction pass) and its scratch evidence: `scratchpad/04a/inset-edges.json`
  (in-set import edges), `tier0-lines.txt` (non-test lines per package; `graph/embedding` taken at 3,082, the ruled
  ceiling figure, §1.4), `ctxroots.txt` (production `context.Background()`/`TODO()` sites per package),
  `owners-raw.txt` (every `Start`/`Stop` method pair over the 65 packages' non-test files), `chain4.py` (the
  partition and every per-change figure below; the review's `04a-review/d.py`, `d2.py` reproduced the sizes
  independently).
- SemEngine base `38b184b` (tree of `main` at `9286055`): `internal/harness/natsfixture/{fixture.go,deps.go,errors.go}`,
  `internal/harness/lifecycletest/{lifecycletest.go,refowner_test.go}`, `internal/harness/natsfixture/
  fixture_integration_test.go`, `internal/harness/contract/imports_test.go`, `internal/harness/runner/runner_test.go`,
  `scripts/verify.sh`, `scripts/cover-check.sh`, `docs/admission-ledger.yaml`, `docs/setup-plan.md`,
  `docs/inventory-scope.md`, `openspec/specs/nats-fixture/spec.md`, 03B design (archived at
  `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md`: D1–D16, Q4 `:292-298`, D7 `:389-407`,
  D8 `:408-429`, D11 `:476-505` incl. Q12, Q18, Q13, and the five drafted deltas).
- Pin snapshot `scratchpad/semstreams-pin/` (never `../semstreams`): `processor/graph-ingest/{component.go,
  component_test.go,canonical_mutations.go}`, `natsclient/kv.go`, `pkg/projection/mutation_client.go`,
  `pkg/lifecycle/graph_emit.go`, `processor/graph-index/failed_start_subscription_test.go`,
  `internal/maxdelivery/runtime_integration_test.go`, `natsclient/client_connect_test.go`, `metric/handler.go`,
  `pkg/worker/pool.go`, `pkg/graphview/view.go`, `graph/readiness/{set.go,watcher.go}`, `graph/embedding/worker.go`,
  `graph/clustering/enhancement_worker.go`, `graph/inference/review_worker.go`, `payloadregistry/testing.go`.
- Adopters, read-only (§5): SemSource `e4febc0d` (`cmd/semsource/run.go`, PR #222 design, PR #223 body,
  `test/setup03a/qualification_test.go` at `75a17f7d`); SemConnect PR #74 `dff12657`; semboids cited from 03B scope
  Q3 at `8c03cc53` (§5.3) and its role in `docs/inventory-scope.md:18` (verification consumer).
- Rulings that bind (§3.4 and 03B): #38; #9 items 1–9; Q4, Q12, Q13, Q18, D4/D4a, D10, D11, D13, D16; the
  `nats-fixture` spec's one-replacement rule (`spec.md:25-26`; `fixture.go:27-30`); the plan's ledger triage of
  carried code (`setup-plan.md:220-222`).
- Reviewer nit R1 on the inventory: `mockKVBucket.Update` (`processor/graph-ingest/component_test.go:122-138`) is a
  fixed CAS fake with no hook; the D8 double is new work.

## Context

The tier-0 set is a DAG with 14 leaves, 16 roots and 15 levels (§1.2). Three edges shape every cut:
`component → pkg/lifecycle → pkg/projection → internal/graphmutation → graph → natsclient` puts the graph package
and the durable-execution kernel under the component model, so nothing with a `Start`/`Stop` component can be ported
before `graph` (closure(component) = 26 / 41,397, §1.4); `service` imports 20 packages including `composition`,
`config`, `pkg/rulepack`, `pkg/lifecycle`, `pkg/projection` (closure 34 / 61,558), so the first bootable process
costs that much; and `processor/graph-ingest`'s closure (33 / 56,371) contains `pkg/projection` and `pkg/lifecycle`
but not `service`, `config` or `composition`, so the first component and its repair proofs do not need the boot path.
Eight packages carry nine out-of-set edges to the behind-seam packages (§1.3); the ruling that agentic-domain cuts land
first makes those edges refactors at port time (#29, #30, #31, E1–E4), while `graph/llm` (842 lines; imports `graph`,
`model`, `model/wire` 1,182 and `go-openai`) is carried dormant to the seam (#32, #35; Q4).

The repair rows are where Q12 bites: #19 and #20 span `pkg/projection` and `processor/graph-ingest`, and their ruled
proving tests run through the owner (`canonical_mutations.go:348-362` fences on `ExpectedRevision`; the D8 test is "an
injected KV `Update` timeout in the harness, and the caller observes `CommitUnknown`"). A cut that admits
`pkg/projection` before `processor/graph-ingest` therefore admits it with its gate evidence not yet runnable — the
state the plan does not have (`setup-plan.md:201-209`). That constraint, not boot order, fixes change 2.

No consumer composes the whole set. SemSource as composed at `e4febc0d` is 56 / 113,786, SemConnect 52 / 107,899,
semboids 49 / 112,758; their union is 63 / 140,382, leaving `composition/cli` and `internal/maxdelivery` (§1.4,
§4.4). SemSource composes `graph-embedding` unconditionally (§5.1). semboids is the verification consumer — "the
measuring fixture the owner runs against each slice as soon as it can" (`inventory-scope.md:18`) — and composes
`graph-ingest`, `graph-index`, `graph-clustering`, `processor/rule`, `output/websocket`, `pkg/graphview`,
`pkg/lifecycle`, `pkg/projection`, `service` (§5.3). Tests are 774 files / 218,454 lines, with four same-class
collisions against the harness (§6).

A **lifecycle owner** is a type whose non-test code starts a goroutine or acquires a resource that outlives the call
that created it (a NATS connection, subscription, consumer or KV watcher, a listener, a ticker, a stored cancel
function) and exposes a method that ends it (`Stop`, `Close`, `Shutdown`, `Drain`). Search: `scratchpad/04a/
ownerscan/main.go` (go/ast over the 65 packages' non-test files: per receiver type and per constructor result type,
a `GoStmt` in any body or a resource-acquiring call; an ending method) → `owners-scan.txt`, 39 types; plus three the
scan cannot see — `service.LogForwarder` (owner by embedding `BaseService`, `log_forwarder.go:96-118`),
`natsclient.TemporalResolver` and `storage/objectstore.Store` (owners by delegation: `Close` closes a `pkg/cache`
instance with a ticker goroutine, `kv_temporal.go:226-228`, `store.go:426-431`). Of the 42, four are unexported
(`pkg/cache.hybridCache`, `pkg/cache.ttlCache`, `pkg/buffer.circularBuffer`, `graph-index.revisionCoalescer`) and are
reached only through their packages' exported constructors; they are covered by their packages' tests and ledger
rows, not counted as suite targets. Subscription handles returned by an owner — `natsclient.Subscription`
(`Drain(ctx)` `client.go:796`, `Unsubscribe()` `:782`) and `graphview.Subscription[T]` (`Unsubscribe()`
`subscription.go:49`) — are not owners: the resource is created and ended by the owner that issued the handle, and
that owner's adapter lists it under `Unresolved`. **38 exported owners**, in three shapes:

- `Start(ctx)`/`Stop` by name, 30 (§3.3's 23 root-package types minus `MilestoneService`, plus `metric.Server`
  `metric/handler.go:59,192`, `pkg/worker.Pool[T]` `pool.go:214,242`, `graph/readiness.Set` `set.go:65,85`,
  `graph/readiness.Watcher` `watcher.go:258,276`, `graph/inference.ReviewWorker` `review_worker.go:114,161`,
  `graph/embedding.Worker` `worker.go:326,364`, `graph/clustering.EnhancementWorker` `enhancement_worker.go:215,511`,
  `pkg/graphview.View[T]` `view.go:203,269`).
- A differently named or constructor start with a context, 7: `natsclient.Client` (`Connect(ctx)` `client.go:471`,
  `Close(ctx)` `:578`), `pkg/resource.Watcher` (`StartBackgroundCheck(ctx)` `watcher.go:141`, `Stop()` `:218-227`
  with an unbounded `wg.Wait`; no production caller of either at the pin — `natsclient/client.go:1394` and
  `graph-clustering/component.go:1328` construct it and call only `WaitForStartup`), `pkg/cache.CoalescingSet`
  (`NewCoalescingSet(ctx, …)` `:27`, `Close()` `:116-126` with an unbounded `<-c.done`),
  `pkg/dispatch.BoundedDispatcher`
  (`New(ctx, …)` `dispatcher.go:95`, `Stop(ctx)` `:196`), `pkg/dispatch.KeyedPool` (`NewKeyedPool(ctx, …)`
  `keyed_pool.go:144`, `Stop(ctx)` `:365`), `natsclient.TemporalResolver` (`NewTemporalResolver(ctx, …)`
  `kv_temporal.go:20`, `Close()`), `storage/objectstore.Store` (`NewStoreWithConfig(ctx, …)` `store.go:84`, `Close()`).
- A start with no context, 1: `pkg/fusion/fusionnats.Client` (`New(nats, timeout)` `client.go:77`; its goroutine is a
  `readiness.Watcher` started lazily; `Close()` `:89` stops it at `:103`).

Ten of the 38 do not end with a `Stop(ctx) error`/`Close(ctx) error`: `config.Manager.Stop(timeout)` (SS#1415, ruled),
`Pool[T].Stop(timeout)`, `Set.Stop()`, `readiness.Watcher.Stop()`, `View[T].Stop()`, `embedding.Worker.Stop()`,
`EnhancementWorker.Stop()`, `ReviewWorker.Stop()`, `resource.Watcher.Stop()`, `CoalescingSet.Close()` (and
`TemporalResolver`, `Store`, `fusionnats.Client` close with no context, delegating to a bounded or package-internal
wait). `processor/rule.ConfigManager.Start(ctx, targets []HotReloadTarget)` (`kv_config_integration.go:117`) takes an
extra argument.

## Decisions

### D1. The unit of a change

An import-closed set, ported with its tests, with its ledger rows, with its proofs green.

A change ports a set S of tier-0 packages such that every in-set import of S is in S or already ported; the change
carries S's `_test.go` files, one ledger row per package (D9), every repair row whose packages are all in S or already
admitted, and the spec delta each repair row was drafted for (D10). The change's gate is Q12's: every row touching a
package in S has its gate evidence green in the change, or the package is not in S. There is no "landed but not
admitted" state; the previous draft assumed one and the review found no record defining it.

Why closed sets: the build is the gate; an unclosed set does not compile, so "small" has a floor set by the DAG. Why
tests travel with packages: `cover-check.sh` reads 80% per target package (`scripts/cover-check.sh:73-75`, §3.2) and
D10's critical list is enforced by it. Why ledger rows per package: #9 item 1 and T-B7 (unique `source_path`), §4.1.

### D2. The chain: seven changes, closure by closure

The first component before the first boot.

| # | Change (proposed id) | Packages | Lines | Tests (files / lines) | Owners | Claim it proves |
|---|---|---|---|---|---|---|
| 1 | `setup-04a-01-floor` | 16 | 25,758 | 127 / 34,887 | 5 | transport and message substrate build and test on `natsfixture`; harness extension lands; #38 runs on `metric.Server` |
| 2 | `setup-04a-02-ingest-kernel` | 17 | 30,613 (+2,024 dormant) | 154 / 40,768 | 7 | graph-ingest runs as a component under the lifecycle suite; #15, #16 (its half), #19, #20, settlement, Q13 proven |
| 3 | `setup-04a-03-boot` | 7 | 20,123 | 110 / 30,257 | 10 | first process boots: `service` over `composition`/`config`; #17, SS#1415/#1218/#1220, #16 refusal; operator surface |
| 4 | `setup-04a-04-graph-roots` | 8 | 26,169 | 148 / 45,864 | 7 | index, query, objectstore as components; #31; SS#1145/#1147 |
| 5 | `setup-04a-05-first-consumer` | 8 | 11,123 | 64 / 15,959 | 3 | SemSource's composition compiles against SemEngine alone; integration branch can start |
| 6 | `setup-04a-06-rule-core` | 2 | 17,243 | 110 / 34,224 | 3 | rule core without agentic (E1–E4); I10 drafted |
| 7 | `setup-04a-07-substrate-seam` | 7 | 9,813 | 61 / 16,495 | 3 | remaining substrate; seam closed (#32, #35): tier 0 compiles without behind-seam packages |

Sums: 65 / 140,842; 774 / 218,454; 38 owners (5 / 7 / 10 / 7 / 3 / 3 / 3). Package lists:

1. `message metric natsclient payloadregistry pkg/acme pkg/cache pkg/errs pkg/platform pkg/projection/contract
   pkg/resource pkg/retry pkg/security pkg/timestamp pkg/tlsutil pkg/types vocabulary` = closure(natsclient) ∪
   closure(message) (§1.4). Owners: `metric.Server`, `natsclient.Client`, `natsclient.TemporalResolver`,
   `pkg/cache.CoalescingSet`, `pkg/resource.Watcher`.
2. `component graph graph/inference graph/readiness graph/structural internal/componentadmission
   internal/graphmutation internal/lifecyclecleanup model pkg/dispatch pkg/lifecycle pkg/projection pkg/worker
   processor/graph-ingest storage storage/storeregistry types` = closure(graph-ingest) − change 1; plus `graph/llm`
   and `model/wire` dormant (D8; `graph/inference` imports `graph/llm`, §1.3). Owners: `graph-ingest.Component`,
   `inference.ReviewWorker`, `readiness.Set`, `readiness.Watcher`, `worker.Pool[T]`, `dispatch.BoundedDispatcher`,
   `dispatch.KeyedPool`.
3. `component/flowgraph composition config health internal/logforwarderpolicy pkg/rulepack service` =
   closure(service) − changes 1–2. Owners: `config.Manager` and the nine ported `service` owners.
4. `graph/clustering graph/embedding graph/query pkg/graphview pkg/revlag processor/graph-index
   processor/graph-query storage/objectstore` = closure(index, query, objectstore) − changes 1–3. Owners:
   `graph-index.Component`, `graph-query.Component`, `objectstore.Component`, `clustering.EnhancementWorker`,
   `embedding.Worker`, `graphview.View[T]`, `objectstore.Store`.
5. `output/websocket pkg/buffer pkg/fusion pkg/fusion/fusionnats pkg/fusion/fusionvocab processor/graph-embedding
   vocabulary/bfo vocabulary/cco` = closure(SemSource as composed) − changes 1–4. Owners: `graph-embedding.Component`,
   `websocket.Output`, `fusionnats.Client`.
6. `processor/rule processor/rule/expression` = closure(rule) − changes 1–5. Owners: `rule.Processor`,
   `rule.CronScheduler`, `rule.ConfigManager`.
7. `composition/cli graph/geo/geojson internal/maxdelivery processor/graph-clustering processor/graph-index-spatial
   processor/graph-index-temporal vocabulary/export`; `processor/graph-clustering` is the last importer of
   `graph/llm`, so the seam closes here. Owners: the three processor `Component`s.

Closure carried across each boundary: 16, 33, 40, 48, 56, 58 packages already ported.

Order rationale: 1 before 2 because the harness extension and the port mechanics (ledger rows, cover targets, revive
at 80 lines, the context-root triage) are proved on 16 packages with one simple owner before the first component;
2 before 3 because #19/#20/#15/settlement need `graph-ingest` and `pkg/projection` admitted together (Q12), and
closure(graph-ingest) does not contain `service` (P3) — the component is proved under the lifecycle suite directly,
as the pin's own graph-ingest tests do (`component_test.go`), with no boot path; 3 before 4 because `service`'s
closure is then 7 packages and the first boot is a small review; 4 before 5 because 5's packages import the graph
roots; 5 before 6 and 7 by consumer need — SemSource (first-wave, the integration branch §5.1) composes none of the
rule core, while semboids, the verification consumer, composes `processor/rule`, `output/websocket`,
`processor/graph-clustering` and so cannot measure a slice before change 7 under this order; that trade is owner
question (c), with the measured alternative that gives semboids change 4; 7 last because it ports the last importer
of the dormant packages and can therefore delete them.

Per-change workload the gates and the ledger see (P17–P19): production context roots to triage 14 / 9 / 5 / 3 / 3 /
0 / 0; cleanup-guard hits 6 / 20 / 92 / 52 / 48 / 10 / 46; `lifecycleUsed` copies 0 / 1 / 2 / 2 / 2 / 2 / 3; the 48
entering `cleanup_baseline.json` entries: 22 in change 5 (websocket), 26 in change 7 (clustering 16, spatial 5,
temporal 5); T-B1 collisions (production files importing `testing`/testify/testcontainers, the import-graph test
`imports_test.go:16-60`): `natsclient/test_client.go` and `payloadregistry/testing.go` in change 1,
`component/lifecycle_test_suite.go` in change 2, `composition/assert.go` in change 3.

### D3. The first change: the floor plus the harness extension

Scope of `setup-04a-01-floor`:

- Packages: the 16 above, 25,758 non-test lines; 127 test files / 34,887 lines, of which 31 integration-tagged, 82
  importing testify, 69 `NewTestClient` sites (all in `natsclient`'s own tests), 1 embedded `nats-server`
  (`natsclient/client_connect_test.go:104`, started and shut down, no restart — served by the fixture), 2 files
  importing `internal/semantictest` (`message/triple_helpers_test.go`, `message/payload_test.go`).
- Five lifecycle owners (Context): `metric.Server` (`Start(ctx)` `metric/handler.go:59`, `Stop(ctx)` `:192`; the
  failing factory is a server whose listener port is already bound by the test, so `Start` returns an error and holds
  nothing), `natsclient.Client` (`Connect(ctx)`/`Close(ctx)`; failing factory: a refused URL), `TemporalResolver` and
  `CoalescingSet` (constructor starts with a context; failing factory: a fault KV / a cancelled context), and
  `pkg/resource.Watcher` (`StartBackgroundCheck(ctx)`; its `Stop()` waits unbounded — a join-completeness item,
  D7). The suite and #38's check run on all five in change 1; their `Observe` adapters (D6) are the first five of 38.
- Ceiling confirmation recorded: the change cites §2 (65 / 140,842 reproduced; `go list -deps` over the 16 roots =
  74 = 65 + 9 behind-seam) and does not re-measure it.
- Ledger rows: 16 package rows (D9) plus the ruled non-package rows that belong to the first ledger extension (D7:
  separated-package rows, Tier-1 cross-check). Existing file rows touched: `natsclient/test_client.go` (adapt →
  natsfixture: performed here, so the row gains `evidence`), `natsclient/test_options.go` (defer-exclude: honoured).
  New file row: `payloadregistry/testing.go` (adapt): `:4` imports `"testing"` from a production file (the T-B1
  shape); it exports `NewForTest`, `NewWithSubset`, `RegisterTestType` (`:18,:32,:53`) called by 8 test files in
  `graph-ingest` (4), `rule` (2), `graph-index` (1), `pkg/lifecycle` (1) and by none of `payloadregistry`'s own
  tests; the adaptation moves them into the harness as `internal/harness/payloadfixture` (D4 "Fixture homes"; a
  home in the ported tree would fail T-B1, which forbids `testing` in any non-test file outside `internal/harness/`),
  and the callers follow in changes 2, 4 and 6.
- Spec deltas carried (D10): `harness-boundaries` (MODIFIED, the drafted T-B8 + I8 delta, §4.2, plus the direction
  rule in D4 "Fixture homes") with the I8 test; `nats-fixture` (MODIFIED: restart, fault KV); `lifecycle-suite`
  (MODIFIED: failed-start check with the required failing factory, D5; the `Observe` adapter definition, D6);
  `process-host` (NEW, D4-B); `transport-client` (NEW, current truth of `natsclient` as ported). The four
  graph/config deltas drafted in 03B are not this change's.
- Repair rows: none admitted here. Two rows have a home in change 1 without a proof: SS#1218's `ErrAlreadyStopped`
  sentinel (`pkg/errs`; proven in change 3 with `service`) and the settlement surface
  (`natsclient/delivery_settlement.go`, carried; first non-agentic caller in change 2). Under Q12 both packages are
  admitted on their own evidence (the row's package list names `service`/`pkg/errs` and `graph-ingest`/`natsclient`;
  the proof that needs the later package runs in the later change — the per-package reading, owner question (g)).
- Third-party dependencies entering `go.mod` as direct: `nats.go`/`jetstream` (present at v1.54.0, §1.6),
  `prometheus/client_golang` (+ `collectors`, `promhttp`), `google/uuid` (`message`), `go-acme/lego/v4` (`pkg/acme`).
- Harness extension (D4), the first task, per #9 item 6: broker restart on `Fixture`, the fault-injecting
  `jetstream.KeyValue`, the process host, the failed-start check, the I8 boundary test, and the fixture homes. All
  three primitives have their first ported consumer in change 2 (graph-ingest's #15, #20 and settlement proofs), so
  change 1 is the only change they can land in; the previous draft's question on the process host's timing is
  withdrawn.

### D4. Harness extension: reuse and new, per collision class (§6)

**A. Fault injection.** Reused: `failAfter[T]` (`natsfixture/fixture_integration_test.go:166-173`) as the shape —
"fail after the real call completes" is the D8 unknown-outcome class — and the `deps` hook style (`deps.go:27-38`) as
the injection seam. New: `failAfter` promoted from the integration test file to an exported primitive, and a
`jetstream.KeyValue` double in `natsfixture` that wraps a real bucket from `CreateKeyValue` (`fixture.go:454`) with
per-method hooks for `Update`, `Put`, `Create`, `Delete` (the four `natsclient.KVStore` write paths; `Update` at
`kv.go:232-236`). It is new because `mockKVBucket` has no `Update` hook (R1) and a fake with fixed CAS semantics cannot
produce "the server applied it and the client saw a timeout". Injection into graph-ingest (change 2): `entityBucket
*natsclient.KVStore` is unexported (`component.go:506`) and built via `graph.EnsureCatalogBucket` (`:1193-1198`);
graph-ingest's 56 tests are internal-package with no `export_test.go` (P9), so a `_test.go` setter in package
`graphingest` injects `natsclient.NewKVStore(double)` at the four existing injection sites (`component_test.go:1030`,
`factory_registry_test.go:49`, `keyed_ingest_test.go:315`, `merge_entity_bench_test.go:119`). The double is typed on
`jetstream.KeyValue`, not `KVStore`, because `natsfixture` never imports `natsclient` (P10).

**B. Subprocess host.** Reused: the mechanics of `runner/runner_test.go` — `exec.CommandContext` start (`:175-176`),
SIGTERM (`:378,615,698`), `Process.Kill` (`:371,585,711,793`), `Setpgid` and group SIGINT (`:639-641`), `pidAlive`
(`:269`), grandchild SIGKILL (`:398,614,697,780`). New: an exported package (proposed `internal/harness/prochost`)
using the helper-process re-exec pattern (`exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")` with an
environment marker), because SemEngine has no runtime binary to launch (P20; SemSource's harness launches its own
binary, `qualification_test.go:404`), with `Signal`, `Kill`, `Stop`/`Continue` (SIGSTOP/SIGCONT, the hung-process shape
at `:829`), bounded `Wait`, `Alive`, and an evidence record. First consumer: `graph-ingest-recovery` "Process killed
between apply and acknowledgement" and the Q18 process-replacement settlement proof (change 2). Its own proof in
change 1: spawn, kill, exit observed, no orphan.

**C. Broker restart.** Two shapes were weighed.

- *Stable host port* (the previous draft): bind `4222/tcp` to a host port chosen at `Start` so `URL()` survives the
  restart. Cost: it modifies the `nats-fixture` requirement "At most one replacement attempt is permitted, only for
  the mapped-port phase" (`spec.md:25-26`; `maxAttempts = 2` after a mapped-port failure only, `fixture.go:27-30`):
  a host-port bind collision fails at create/start, where no replacement is allowed, and on `Restart` the container
  cannot be replaced without losing the store the primitive exists to preserve, so a collision fails the restart
  outright. The one thing it buys — a component's own nats.go reconnect across the restart — is a scenario no
  drafted delta asks for, and "components reconnect to the URL they were given" was an unmeasured premise.
- *Re-read and reconnect* (SemSource's harness, `qualification_test.go:328-334`: "Docker may assign a new ephemeral
  host port after restart. Read the binding from the same owned container"): `Fixture.Restart(ctx) error` takes the
  slot, drains the fixture's own `*nats.Conn` (dialled with `nats.MaxReconnects(0)`, `deps.go:87`, so it is dead
  after a restart), calls `Container.Stop(ctx, &timeout)` then `Container.Start(ctx)` (testcontainers-go v0.44
  `container.go:52-53`) on the same container, re-runs `PhaseMappedPort → PhaseConnect → PhaseJetStream`
  (`fixture.go:285-333`), and updates `url` and `rec`. `URL()` is valid until the next `Restart`. A component under
  test is stopped before and started after with the new URL — SemSource's shape (`h.stop(true)`, `docker restart`,
  `h.connect()`, re-exec with `--nats-url`, `qualification_test.go:879-885,404`). A `Stop`/`Start` failure is a
  `FixtureError` in a new phase, reported, not retried; the container is terminated by the existing `Stop` path. No
  replacement attempt, so the one-replacement rule is untouched.

Decision: re-read and reconnect. New surface: `Restart`, two `Phase` constants, two `deps` hooks (`stop`,
`startContainer`) so the fault matrix covers them, `count("restart")`. Durability across restart: the container
runs `--js` with no `-sd` and no volume (`fixture.go:251-254`), so the JetStream store is in the writable layer, which
`Stop`/`Start` preserves and `Terminate` discards (P11, measured by change 1's proof: a file stream survives, a
memory stream does not — the pair the `graph-ingest-recovery` "Memory transport lost at broker restart" scenario
needs in change 2). If a reconnect-across-restart scenario is ever ruled, the stable port returns as its own delta
with the bind-failure path designed; it is not in 04A.

**Fixture homes.** `natsclient.NewTestClient` → `natsfixture`: the 69 sites in `natsclient`'s own tests move in
change 1 (internal-package tests may import `natsfixture`; the reverse import is the cycle P10 forbids); the 209 sites
in the other 20 packages move with their packages. Two cross-package test helpers typed on `testing.TB` need a home:
`internal/semantictest` (67 lines; `fixtures.go:11` imports `"testing"`, `:13-14` `pkg/types` and `vocabulary`;
`testing.TB` at `:21,:41`; imported by 58 tier-0 test files — 14 packages by this grep,
13 in §1.5) and `payloadregistry/testing.go`
(`NewForTest(tb testing.TB)` `:18`; 8 callers in 4 packages, P24). Three options were measured against T-B1 as it
exists on the base:

- (i) *Rehome both under `internal/harness/`* as `internal/harness/semantictest` and `internal/harness/payloadfixture`.
  T-B1 skips every file under `internal/harness/` and every `_test.go` (`imports_test.go:66-72`) and forbids `testing`,
  testcontainers, yaml and the harness itself in any other non-test file (`:48-60`); it says nothing about what the
  harness may import. Cycle check per helper, over the helper's full dependency closure (`go list -deps`), not its
  direct imports only: no internal-package test of any package in that closure may import the helper — for
  `semantictest` the closure is `pkg/types`, `vocabulary` and their in-set dependencies, and no test in any of them
  imports `semantictest`; for `payloadfixture` it is `payloadregistry`, `pkg/types` and theirs, and none of their
  tests calls the helpers (pin grep; P24, P26). The plan's helper kit already lists "valid Graphable,
  triple, metadata, and content-reference fixtures" as harness items (`setup-plan.md:284-286`).
- (ii) *Keep them in the ported tree* as `_test.go`-only or build-tagged helper packages. A `_test.go` file cannot be
  imported by another package; a build-tagged non-test file is still parsed by T-B1, which "parses imports without
  evaluating build constraints, so a file behind a tag … is checked too" (`imports_test.go:64-65`). Change 1 would be
  red. Not possible without (iii).
- (iii) *Amend T-B1* through the harness-boundaries delta to allow a named helper package outside the harness. A
  ruling change for a problem (i) solves inside the rule as written; it would be an owner question.

Decision: (i). The previous draft's rule "`internal/harness/*` imports no ported package" is withdrawn (it cannot
coexist with T-B1 and reversed the helper-kit item). The bound that replaces it, for the harness-boundaries delta:
`natsfixture` imports no ported package (P10, the only cycle that exists); any other harness package may import a
ported package that is a pure library under the owner definition in Context (no goroutine, no held resource, no
NATS), and the cycle check above is recorded in the helper's ledger row. The directions that still hold and that the
delta states: production code outside the harness never imports the harness (`imports_test.go:24`, enforced); no
file in the module, harness or ported, imports `github.com/c360studio/semstreams` (the I8 test; `go list -deps`
over the harness is part of its scope, so a harness helper cannot smuggle a SemStreams import either). D12 carries
the T-B1 and helper-kit rows. The `payloadregistry/testing.go` ledger row becomes `adapt → internal/harness/
payloadfixture`, the shape of the existing `natsclient/test_client.go` row.

**I8 boundary test.** A unit test in `internal/harness/contract` (or a `scripts/verify.sh` step) asserting that `go
list -deps ./...` contains no `github.com/c360studio/semstreams` path and that `go.mod` has no such requirement; T-B8
(no aggregator) alongside, per the drafted delta. Before change 1 the test is vacuous (`git grep` is empty, §3.3);
from change 2 it has teeth (`component → agentic` is cut by #29 there, and `graph/llm` arrives as a copy, not an
import).

### D5. Failed-start honesty (#38)

A required failing factory, not an optional one.

`lifecycletest.Run(t, factory, promise)` (`lifecycletest.go:68`) gains a second factory parameter whose owner's
`Start` must return an error; omission is a compile error at every call site, the compile-time shape the `Owner` doc
comment already chose over an optional interface (`lifecycletest.go:30-34`). The check: `Start` returns non-nil;
`Observe().Unresolved` is empty; `Stop` afterwards returns nil and changes no `Calls` entry. The factory, not the
suite, knows how to make the dependency fail — a bound listener port, a refused `nats://` URL, or a fault KV from
D4-A — which answers #38's open question. Call sites on the base are two: `refowner_test.go:303-304` and
`natsfixture/fixture_integration_test.go:519` (`TestS1_7Restart`, the fixture running the suite on itself). Change 1
therefore supplies three failing factories: the `refowner` double extended; a `natsfixture.Fixture` whose
`deps.start` hook fails (the S1 fault-matrix shape already in that file), so `Start` returns a `FixtureError` and
holds nothing; and `metric.Server` on a bound port (D3). Change 2 brings the first processor, where the pin already
has three failed-start tests to port as the basis (`processor/graph-index/failed_start_subscription_test.go:48,125,201`
is change 4's; graph-ingest's own failing factory is a fault KV at `EnsureCatalogBucket`). No readiness accessor on
`Owner` (ruled, §3.4).

The failed-start check needs a context-taking start. Thirty-seven of the 38 owners have one — `Start(ctx)`,
`Connect(ctx)`, `StartBackgroundCheck(ctx)`, or a constructor with a context (Context); the adapter's `Start` is that
call (`rule.ConfigManager.Start(ctx, targets)` is bound by its adapter), and the check runs on all 37.
`fusionnats.Client` (`New(nats, timeout)`, no context) is excluded from the suite with the reason on its ledger row
(change 5): the two context checks would test the adapter, not the owner; its one goroutine is a `readiness.Watcher`,
whose own row covers the wait. What the suite's other checks do with the ten non-context enders is D6 and D7.

Alternatives considered: a `Promise.FailingStart` field skipped when nil (the silent-skip shape; loses on the
owner's own wording); a per-package obligation with no suite check (loses on #38's ruling).

### D6. The Observe() adapter is test-side, one per owner type

Non-context Stops are ledger rows.

`lifecycletest.Owner` requires `Observe() Observation` (`lifecycletest.go:36-40`); the pin's
`component.LifecycleComponent` has no `Observe` (`component/lifecycle.go:63-68`). Options: (i) add `Observe` to each
ported owner in production — 38 types gain exported surface for a test's sake, against D16; (ii) a `_test.go` adapter
per owner type wrapping it and reading retained state through unexported fields, which internal-package tests can do
(P9); (iii) a harness-side generic adapter over the pin's `lifecycleUsed` counters — 12 copies of a pattern D11 wants
removed, and a counter is not an inventory of what is held. Decision: (ii), 37 adapters: 5 / 7 / 10 / 7 / 2 / 3 / 3
across changes 1–7 (the owner list in Context; `fusionnats.Client` excluded,
D5). The `lifecycle-suite` delta names what `Unresolved` must list for a
NATS owner (subscriptions, consumers, KV watchers, goroutines, timers, listeners) so the adapters are written to one
definition; the reviewer's checklist for every owner port includes "adapter lists every retained kind the owner holds".

The ten non-context enders cannot be driven by the suite's bounded `Stop(ctx)` without an adapter that either
(a) calls the no-argument `Stop` on a goroutine and returns `ctx.Err()` when it has not returned — which leaks the
goroutine and makes the completed-joins check report `Unresolved` honestly, or (b) is given a context-bounded `Stop`
by the package. (b) is the ruled shape: SS#1415 made `config.Manager.Stop(timeout)` → `Stop(ctx)` a repair row. The
design extends that row's shape to the nine others as `adapt` rows (D7) — `Pool[T]` (timeout → ctx); `Set`,
`readiness.Watcher`, `View[T]`, `Worker`, `EnhancementWorker`, `ReviewWorker`, `resource.Watcher` (no argument → ctx);
`CoalescingSet.Close()` → `Close(ctx)` — proven by the suite over the adapted type in the change that ports it. Two of
them wait unbounded today (`resource.Watcher.Stop` on `wg.Wait`, `watcher.go:218-227`; `CoalescingSet.Close` on
`<-c.done`, `coalescing_set.go:116-126`): the bounded `ctx` is what makes the completed-joins check decidable, and
the suite's `CheckControlledStopUnderLiveStartAuthority`/`CheckRepeatedStopIsNoOp` over them in change 1 is the
proof. Cross-package callers of the changed signatures are in later changes (N3): `embedding.Worker.Stop()` at
`processor/graph-embedding/component.go:882` and `readiness.Watcher.Stop()` at `fusionnats/client.go:103` (change 5);
`ReviewWorker.Stop()`, `EnhancementWorker.Stop()`, two `readiness.Watcher.Stop()`s at
`processor/graph-clustering/component.go:1232,1239,1260,1266` (change 7); `Pool[T].Stop(timeout)` at
`pkg/dispatch/dispatcher.go:231` and `View[T].Stop()` at `processor/graph-query/summary_view.go:160,173,184` are
in the same change as their types (2 and 4); `CoalescingSet.Close()` is called at
`processor/graph-embedding/component.go:875` (change 5) and `processor/rule/entity_watcher.go:980` (change 6); `Set`
and `resource.Watcher` have no cross-package production caller of the changed method. Whether these are repairs
(context ownership and completed joins are
admission gates, `setup-plan.md:206-209`) or carried roots with a reason (`:220-222`) is owner question (h); the
design assumes repairs because the ruled precedent is one.

### D7. Repair rows, port refactors, and ruled ledger items by change

| Row / refactor / item | Change | Why there |
|---|---|---|
| #29 `component` agentic cut | 2 | out-of-set edge `component/dependencies.go:7,76`; `component` lands in 2 |
| SS#1411 (owner-lifecycle guard composed by the `lifecycleUsed` copies) | 2 lands; applies in 2–7 | `component` in 2; each later owner adopts it when ported |
| #19, #20 (`projection-mutation`) | 2 | `pkg/projection` and `processor/graph-ingest` admitted together; the D7/D8 proofs run through the owner (`canonical_mutations.go:348-362`) with the change-1 fault KV; `pkg/lifecycle` writes through `graphmutation` (`pkg/lifecycle/graph_emit.go:24,31`) and `pkg/projection` through `mutation_client.go:184-198` |
| #15, settlement (Q18), `graph-ingest-recovery` | 2 | graph-ingest; needs restart (D4-C) and the process host (D4-B), both change 1 |
| Q13: SemStreams PR #1437 graph-ingest half | 2 | ruled "taken up with the graph-ingest repair rows (#15, #20, settlement)"; the row cites the PR head SHA |
| #16 (`graph-transport-boundary`): declaration in `graph`, literal sites in graph-ingest | 2 | both packages in 2; `composition.Analyze` refusal in 3; graph-query's site in 4 — a four-package row admitted per package (owner question g) |
| #33 D1 adopter path (per-package `Register`, refusal text `component.go:739`) | 2 | first processor and `component.Registry` both in 2 |
| non-context ender adapt rows (D6): `pkg/resource` (`Watcher.Stop` unbounded wait), `pkg/cache` (`CoalescingSet.Close` unbounded wait) | 1 | ported in 1; proven by the suite's join checks over the adapted types in change 1 |
| non-context `Stop` adapt rows (D6): `pkg/worker`, `graph/readiness` (2 types), `graph/inference` | 2 | ported in 2; `Pool[T]`'s caller `dispatcher.go:231` is in 2 |
| #38 on real owners | 1, then every owner | `metric.Server` in 1; graph-ingest and the four workers in 2; `service` owners and `config.Manager` in 3; the pin's graph-index failed-start tests in 4 |
| #17 (`config-desired-state`), SS#1415 (`config.Manager.Stop(ctx)`), SS#1220 (registry after failed start) | 3 | `config`, `service` land in 3 |
| SS#1218 (one `ErrAlreadyStopped`) | 1 home, 3 proof | sentinel in `pkg/errs` (1); the unit test needs `service` (3) |
| #30 `service` agentrun cut | 3 | `service/milestone_service.go:10` |
| #16 `composition.Analyze` refusal | 3 | `composition` lands in 3 |
| #31 graph-query cut (`vocabulary/agentic`, `graph/llm` call sites) | 4 | `processor/graph-query` lands in 4 |
| non-context `Stop` adapt rows: `graph/embedding`, `graph/clustering`, `pkg/graphview` | 4 | ported in 4; `View[T]`'s callers `summary_view.go:160,173,184` are in 4 |
| port-refactor rows for callers of changed ender signatures (`class:port-refactor`, D4a) | 5: `processor/graph-embedding/component.go:875` (`CoalescingSet.Close`), `:882` (`Worker.Stop`), `pkg/fusion/fusionnats/client.go:103`; 6: `processor/rule/entity_watcher.go:980` (`CoalescingSet.Close`); 7: `processor/graph-clustering/component.go:1232,1239,1260,1266` | the signature changes land in 1, 2 and 4; the callers are ported later and edited at port time, each as its own row naming the call sites |
| `fusionnats.Client` suite exclusion with reason (D5) | 5 | constructor takes no context |
| SS#1145/#1147 (restart behaviour as `Promise{Restart}`) | 1 onward, per owner | the suite's floor plus each ported owner's promise |
| SS#1417 (ported tests pass `cleanup-roots-check.sh`) | every change | the guard is a `task verify` step; per-change hits in D2 |
| #34 `output/websocket` | 5 | lands in 5 |
| E1–E4 (#25–#28), I10 | 6 | the rule core's four agentic edges (`actions.go`, `config_validation.go`, `verdict_auditor.go`) |
| #36 `internal/maxdelivery` | 7 | no consumer composes it (§4.4); its three-node-cluster test is owner question (b) |
| #32 capability seam, #35 `graph/embedding` split | 7 | last importer of `graph/llm` is `processor/graph-clustering` |
| #9 item 1: separated-package rows (D4's ten-out and the 9 behind-seam packages) | 1 (ten-out, behind-seam not yet carried); 2 (`graph/llm`, `model/wire` as carried-dormant rows); 7 (closed at the seam) | the first ledger extension writes the rows that need no port; dormant rows are written when the copies enter |
| #9 item 1: Tier-1 cross-check re-measured on the ruled set | 1 | an inventory task on the first ledger extension; architect writes, technical writer records |
| #9 item 1: port-refactor rows per the ledger header convention | the change that performs each refactor | #25–#36 as placed above |
| #9 item 3: `proving_tests` per package row, naming the role and change that writes a missing test | every change, for its own rows | the convention is set in change 1; a row whose proof is in a later change names that change (the per-package reading, g) |

### D8. Dormant carriage and the seam (Q4)

`graph/llm` (842) and `model/wire` (1,182) enter in change 2 as copies — `graph/inference` imports `graph/llm`
(`config.go:12`, `review_worker.go:17`) and graph-ingest imports `graph/inference` — and leave in change 7, the seam.
That is six OpenSpec changes of dormancy. Q4 says the behind-seam code "is carried dormant through the first green
tier-0 extraction (Slice 04A)", the seam is "the last task of 04A", and "The dormant bridge is bounded to that one
change, not deferred to Slice 04B" (03B `design.md:292-298`). Read as "bounded to Slice 04A", the design conforms;
read as "bounded to one OpenSpec change", no multi-change cut conforms: every importer of `graph/llm` would need to
land in the one change that also closes the seam (changes 2, 4 and 7 merged: 32 / 66,595), or the cut would have
to be made per importer at port time, which is the refactor "made blind at the pin" Q4 was written to avoid. Owner
question (f). Cost while dormant: `go-openai` is a direct requirement from change 2 to change 7 (`task vuln` scans
it); the two packages are ledgered `carry`, `known_risks: dormant until #32`, and excluded from coverage targets.
`vocabulary/agentic` (2,133) is never carried: #31 cuts it from graph-query (change 4) and E1–E4 from the rule core
(change 6). Exit condition of change 7: tier 0 compiles with `graph/llm`, `model/wire`, `go-openai` removed; the I8
test and a seam test (`go list -deps ./... | grep -E 'graph/llm|model/wire'` empty) both green.

### D9. Ledger rows are package rows keyed by directory

Context roots are triaged in them.

One row per package, `source_path` = package directory (unique against the file rows, T-B7, §4.1), `source_sha` =
the pin `8b99efe9`, disposition `carry` for untouched packages, `adapt` where a file changes at port time (a
`NewTestClient` site, a `lifecycleUsed` copy, an out-of-set edge, a non-context `Stop`, a T-B1 collision),
`repair-before-port` for D7's rows, with `proving_tests` per #9 item 3. Production `context.Background()`/`TODO()`
sites (34 in the set, `ctxroots.txt`) are not measured by any gate on the tree: T-B1 is the import-graph test
(`imports_test.go:16-60`), and `git grep -n 'context.Background\|context.TODO' -- internal/harness/contract scripts`
finds only `cleanup-roots-check.sh`, which reads test files. The plan says carried code is "triaged in the ledger
instead of being rewritten wholesale: … Record each as a legitimate root with its reason, or as a defect"
(`setup-plan.md:220-222`). So the per-change counts in D2 are triage workload, recorded in each package row (a
`context_roots` note under `known_risks`, or a new field if the header gains one — the technical writer's call); a
root that breaks an admission gate becomes a repair row. No new guard is proposed. `internal/lifecyclecleanup` keeps
its harness-helper file row and gains a package row; the two homes are reconciled in change 2 (its row's note:
"production home deferred until a production consumer exists (04A)", §4.1).

### D10. Spec deltas by change (named only)

1. `harness-boundaries` (MODIFIED, drafted + the harness import bound from D4 "Fixture homes"), `nats-fixture`
   (MODIFIED), `lifecycle-suite` (MODIFIED), `process-host` (NEW), `transport-client` (NEW).
2. `graph-ingest-recovery` (NEW, drafted), `projection-mutation` (NEW, drafted), `graph-transport-boundary` (NEW,
   drafted; declaration and the graph-ingest literal sites), `component-registration` (NEW: D1's per-package
   `Register` and the refusal), `lifecycle-suite` (MODIFIED: first processor's failed start).
3. `config-desired-state` (NEW, drafted), `graph-transport-boundary` (MODIFIED: `composition.Analyze` refusal
   scenarios), `operator-surface` (NEW: D15's `service` endpoints as ported).
4. `graph-transport-boundary` (MODIFIED: graph-query's site; "SemSource's explicit list starts and answers"),
   `graph-query` (NEW: current truth as ported, after #31).
5. `change-observation` (NEW: D14's websocket output as ported), `fusion-boundary` (NEW: D5's fusion/graph-tool line).
6. `rule-core` (NEW, I10 import rules; "drafted by the 04A change that ports the rule core", §4.2).
7. `capability-seam` (NEW: #32/#35 exit condition), `nats-fixture` (MODIFIED only if a cluster mode is chosen under
   owner question b).

### D11. What each change makes red, and where it is measured

`task verify` (`scripts/verify.sh:10-11`) runs the same steps for every change; what a change must turn green is
measured by: the import-graph test T-B1 (the four collisions, D2); the cleanup-roots guard (6 / 20 / 92 / 52 / 48 /
10 / 46 hits, §3.3 partitioned by D2's lists); `cover-check.sh` with its hard-coded targets extended per change by
D10's critical list (`:73-75`); revive 1.15.0 function-length 80 with no test exclusion (70 findings at the pin, all
in tests, §3.3; the two function-length findings are the only ones that need a rewrite); `task spec:check` on the
deltas named in D10; the I8 test (D4); the lifecycle suite over every owner in the change (D6). Context roots are not
a gate (D9). The `AGENTS.md:43-44` verify list is stale against `scripts/verify.sh` (§3.3); change 1's
technical-writer task fixes it.

### D12. Ruling-conformance table

| Ruling | Where it binds this design | Conforms / deviation |
|---|---|---|
| Q12 (gate evidence green before the package is admitted; plan `:201-209`) | D1, D2 change 2, D7 | Conforms for single-package rows and for #19/#20/#15/settlement (all packages in change 2). For the four multi-package rows whose packages span changes — #16 (`graph`, graph-ingest in 2; `composition` in 3; graph-query in 4), SS#1218 (`pkg/errs` in 1; `service` in 3), settlement (`natsclient` in 1; graph-ingest in 2) — the design admits each package on the evidence that its own code can produce and names the later change for the rest. **Owner question (g).** |
| D11 / Q12 recompute (69 entries / 6 copies; the ten entering packages unmeasured) | D2 workload, D7 SS#1417 | Conforms; the ten entering packages are now measured (§3.3: 48 entries, 6 copies) and placed (changes 5 and 7). |
| Q4 (dormant bridge bounded to "that one change"; seam the last task of 04A) | D8 | Conforms if "that one change" is Slice 04A; **deviation if it is one OpenSpec change** (six changes of dormancy). **Owner question (f).** |
| Q13 (PR #1437 graph-ingest half with #15/#20/settlement) | D7 | Conforms (change 2). |
| Q18 (settlement row with the process-replacement proving test in the harness) | D4-B, D7 | Conforms (host in change 1, proof in change 2). |
| D4 / D4a (65 packages; port refactors tracked as rows) | D2, D7, D9 | Conforms (65 partitioned; #25–#36 placed with rows). |
| D10 (80% coverage on the critical list) | D1, D11 | Conforms; `cover-check.sh` targets extended per change (declared cost). |
| D13 (durable execution at tier 0; #24 "after #9's first green extraction") | D2, D7 | Conforms; which change is "first green" is **owner question (d)**. |
| D16 (exported surface) | D6 | Conforms: `Observe` is test-side. |
| #38 (failed-start honesty in the standard run, against a real component in 04A; no readiness accessor) | D3, D5 | Conforms; first real owner is `metric.Server` in change 1. |
| #9 item 6 (harness extension first task of the first change) | D3, D4 | Conforms. |
| `nats-fixture` one-replacement rule (`spec.md:25-26`) | D4-C | Conforms (re-read and reconnect; no replacement on `Restart`). |
| Plan `:220-222` (carried code triaged in the ledger) | D9 | Conforms; no context-root guard proposed. |
| SS#1415 (a timeout-shaped `Stop` is a repair) | D6, D7 | Conforms by extension to nine more types; **owner question (h)** because the extension is the design's, not a ruling's. |
| T-B1 (import-graph test, `imports_test.go:48-72`: `testing`/testcontainers/yaml/harness forbidden in non-test files outside `internal/harness/`; harness and `_test.go` files unchecked; build tags not evaluated) | D3, D4 "Fixture homes" | Conforms: both `testing.TB` helpers are rehomed under `internal/harness/`; the previous draft's helper homes in the ported tree would have failed it. |
| Plan helper kit (`setup-plan.md:284-286`: Graphable, triple, metadata, content-reference fixtures are harness items) | D4 "Fixture homes" | Conforms: `internal/harness/semantictest` lands in change 1; the previous draft's "harness imports no ported package" rule is withdrawn. |
| D9 in 03B (SemConnect cutover not an MVP gate) and `inventory-scope.md:18` (semboids is the verification consumer) | D2 order of changes 4–7 | The order serves SemSource first; semboids cannot measure before change 7. **Owner question (c).** |

## Premises (each with its measurement)

- P1. The tier-0 set is a DAG; closure-closed sets are exactly the compilable units. — §1.2 (Tarjan over
  `inset-edges.json`: no SCC of size > 1).
- P2. closure(natsclient) ∪ closure(message) = 16 / 25,758; it contains five lifecycle owners under the Context
  definition (`metric.Server`, `natsclient.Client`, `natsclient.TemporalResolver`, `pkg/cache.CoalescingSet`,
  `pkg/resource.Watcher`) and no out-of-set edge. — §1.4; `chain4.py` C1; `owners-scan.txt` (go/ast scan,
  `ownerscan/main.go`) plus the by-name list `owners-raw.txt`; §1.3 lists eight importing packages, none in C1.
- P3. closure(graph-ingest) − C1 = 17 / 30,613 and contains `pkg/projection`, `pkg/lifecycle`, `component`, `graph`
  but not `service`, `config`, `composition`. — §1.4 (closure(graph-ingest) 33 / 56,371); `chain4.py` C2.
- P4. closure(service) − (C1 ∪ C2) = 7 / 20,123. — §1.4 (closure(service) 34 / 61,558); `chain4.py` C3.
- P5. closure(index, query, objectstore) − (C1 ∪ C2 ∪ C3) = 8 / 26,169 at the ruled ceiling (`graph/embedding`
  3,082). — §1.4; `chain4.py` C4.
- P6. SemSource as composed at `e4febc0d` composes `graph-embedding` unconditionally and its closure minus C1–C4 is
  8 / 11,123. — §5.1 (`run.go:812-817`, `embedder_type` default `bm25` `:724`; PR #222 seam table); `chain4.py` C5.
- P7. closure(rule) − closure(service) = {`graph/readiness`, `processor/rule`, `processor/rule/expression`}; after
  change 2 (`graph/readiness` in it), the rule core is 2 / 17,243 and could follow change 3. — §1.4; `chain4.py`.
- P8. semboids composes `graph-ingest`, `graph-index`, `graph-clustering`, `processor/rule`, `output/websocket`,
  `pkg/graphview`, `pkg/lifecycle`, `pkg/projection`, `service`; its closure minus C1–C3 is 9 / 36,264. — §5.3
  (03B scope Q3 at `8c03cc53`); `inventory-scope.md:18`; `chain4.py` (`semboids closure 49 / 112,758`).
- P9. graph-ingest's tests are 56 internal-package files, 0 external, no `export_test.go`; natsclient's are 78
  internal, 0 external. — pin: `grep -L '^package graphingest$' processor/graph-ingest/*_test.go` empty; no
  `export_test.go`; same for `natsclient`.
- P10. `natsfixture` imports no ported package and must not import `natsclient`: natsclient's internal tests will
  import `natsfixture`, and the reverse import is a cycle. — `go list -f '{{join .Imports "\n"}}'
  ./internal/harness/natsfixture` on base: std, `internal/harness/probe`, nats.go, jetstream, testcontainers, `wait`.
- P11. The fixture's JetStream store is in the container's writable layer: `Cmd` is `--port 4222 --js` with no `-sd`
  and no volume (`fixture.go:251-254`). The durability claim (file stream survives `Stop`/`Start`, memory stream does
  not) is what change 1's restart proof measures, not an assumption.
- P12. The mapped host port can change across a container restart; the fixture reads it only in `Start`. — SemSource
  `qualification_test.go:328-334`; `fixture.go:285-290`, `deps.go:48`.
- P13. `mockKVBucket.Update` is a fixed CAS fake with no hook; the D8 server path is
  `canonical_mutations.go:348-362`; the four `KVStore` write paths are `kv.bucket.{Create,Delete,Put,Update}`. — pin,
  R1.
- P14. testify is indirect-only in SemEngine (`go.mod:67`) and imported by 438 pin test files, partitioned 82 / 74 /
  71 / 92 / 36 / 53 / 30 by change. — §1.5; `chain4.py`.
- P15. `natsclient.NewTestClient(` has 278 sites in 21 packages: 69 / 43 / 59 / 31 / 31 / 20 / 25 by change (change
  1's are all natsclient's own). — §1.5; `chain4.py`.
- P16. Nine test files embed `nats-server/v2`: 1 / 0 / 1 / 2 / 1 / 0 / 4 by change; `internal/maxdelivery/
  runtime_integration_test.go` (change 7) runs a three-node cluster via `natstest.RunServerWithConfig` (`:320,365`);
  `natsclient/client_connect_test.go:104` starts and shuts down one server. — §1.5; pin grep.
- P17. Production context roots: 34 in the set, 14 / 9 / 5 / 3 / 3 / 0 / 0 by change; no gate on the tree counts
  them (D9's search). — `ctxroots.txt`; `imports_test.go:16-60` is an import test.
- P18. The cleanup baseline has 117 entries in the 65, 48 in the entering packages (22 websocket → change 5; 26
  clustering/spatial/temporal → change 7); the guard finds 274 hits in 15 packages, 6 / 20 / 92 / 52 / 48 / 10 / 46 by
  change. — §3.3; D2 lists.
- P19. `lifecycleUsed` has 12 copies: 0 / 1 / 2 / 2 / 2 / 2 / 3 by change. — pin grep (`service` 2; graph-ingest;
  graph-query, objectstore; graph-embedding, websocket; `rule` 2; graph-clustering, spatial, temporal).
- P20. SemEngine has no runtime binary and the pin's `cmd/` is not in the set; SemSource's harness launches its own
  binary (`qualification_test.go:404`). — §0; `tier0-set.txt`.
- P21. `go-openai` enters the module only through `graph/llm` once `http_embedder.go` is out (#35). — §1.6;
  `pin-edges.txt`.
- P22. Thirty-eight exported owner types under the Context definition (30 `Start`/`Stop` by name, 7 with a
  differently named or constructor start taking a context, 1 with no context), plus 4 unexported; ten end without a
  context; one `Start` takes an extra argument. — Context (file:line per type); `owners-scan.txt` (39 types from
  `ownerscan/main.go`) + `LogForwarder` by embedding + `TemporalResolver`/`Store` by delegation; `owners-raw.txt`
  (by-name list, 30). The scan's limits: it sees goroutines and resource calls in a type's own methods and
  constructors, not in embedded types or delegated `Close`s — the three it missed were found by reading the
  by-name list against it.
- P23. `lifecycletest.Run` has exactly two call sites on the base: `refowner_test.go:303-304` (unqualified `Run(`,
  same package) and `natsfixture/fixture_integration_test.go:519`. — `git grep -n 'lifecycletest.Run(' -- internal`
  (one hit) plus `grep -n '^\s*Run(' internal/harness/lifecycletest/*_test.go` (two) on `38b184b`.
- P24. `payloadregistry/testing.go:4` imports `"testing"`; its three exported helpers are called by 8 tier-0 test files
  in 4 packages and by none of `payloadregistry`'s own tests. — pin grep (`payloadregistry\.(NewForTest|NewWithSubset|
  RegisterTestType)\(` over `*_test.go`).
- P25. T-B1 skips `_test.go` files and everything under `internal/harness/`, parses without evaluating build
  constraints, and forbids `testing`, testcontainers, yaml and the harness in every other non-test file; it has no
  rule on what the harness imports. — `internal/harness/contract/imports_test.go:48-72` on `38b184b`.
- P26. No test in `pkg/types` or `vocabulary` imports `internal/semantictest` (so a harness helper importing them
  creates no cycle); `StartBackgroundCheck` has no production caller at the pin. — pin grep over `*_test.go` for the
  import path; `grep -rn StartBackgroundCheck\(` (only `pkg/resource` tests and docs).

## Options (step 4): the cut, with costs

Each option is measured with the same partition tooling; "review" is files/lines a reviewer reads per change.

**A. One change.** 65 / 140,842 (+2,024 dormant), 774 test files / 218,454 lines, 12 repair rows, 12 refactors, 5
drafted deltas plus the harness deltas, 38 owners, in one review. Closure carried across boundaries: none. Dormant:
one change (Q4 trivially satisfied). I8: teeth only at the end. Ledger: 65 rows in one PR. "No binary imports both
modules": SemSource's branch starts only after the single merge. Loses on "port small slices" and on blast radius:
one red gate blocks 140K lines.

**B. Strata by import level.** Fifteen levels (§1.2): level 0 = 14 / 6,195, 1 = 4 / 4,207, 2 = 4 / 4,564, 3 = 3 /
2,247 (`graph/query` → `graph/llm` dormant from here), 4 = 2 / 1,726, 5 = 4 / 6,577, 6 = 2 / 13,484 (`natsclient`),
7 = 3 / 4,724 (`graph`), 8 = 6 / 11,885, 9 = 4 / 6,741 (`pkg/projection`), 10 = `pkg/lifecycle`, 11 = `component`,
12 = 12 / 40,315 (every processor, `config`, `websocket`), 13 = 2 / 1,057, 14 = 3 / 27,796 (`service`, `rule`, `cli`).
Fifteen changes, or five strata (39,000 / 23,350 / 9,324 / 40,315 / 28,853). Q12: `pkg/projection` (level 9) is
admitted three levels (two strata) before graph-ingest (level 12) — the same violation the previous draft had,
built into the
option. I8: teeth from level 3. Nothing boots before the last stratum, and the 12 processors arrive together anyway.
Loses on Q12 and on review shape.

**C. Consumer-need order.** SemConnect first (52 / 107,899) or SemSource first (56 / 113,786): 77–81% of the lines,
52–56 of the 65 packages, all 12 repair rows and 5–7 of the 12 refactors, 31–32 of the 38 owners,
in change 1. Q12: conforms (a
composition is closed and carries its proofs). Dormant: as D8. "No binary imports both": the first-wave consumer can
cut over after one change — its only win, and not a gate (SemConnect's cutover is not an MVP gate, 03B D9; SemSource's
branch is a qualification branch, §5.1). Loses on size: option A with nine packages deferred.

**D. Root-set closures (recommended: the chain in D2, graph-ingest second).** Seven changes of 25,758 / 30,613 /
20,123 / 26,169 / 11,123 / 17,243 / 9,813 lines; the largest review is change 4 at 148 test files / 45,864 lines.
Q12: conforms for every single-package row; the multi-package rows are owner question (g). I8: teeth from change 2.
Ledger: 16 / 17 / 7 / 8 / 8 / 2 / 7 rows. "No binary imports both": SemSource's branch can start after change 5.
Sub-variants measured:

- (D-v) *service second, graph-ingest third* — the previous draft: 18 / 35,800 then 14 / 41,105. Rejected: it admits
  `pkg/projection` (#19/#20) with its proofs not runnable until the next change (finding 1; Q12).
- (D-ii) *merge service and graph-ingest into change 2*: 24 / 50,736, 264 test files / 71,025 lines, 15 owners, 112
  guard hits, in one review; six changes. Conforms to Q12 with no owner question; loses on review size (the largest
  change in any option but A/C) and on mixing the first boot with the first component's recovery proofs.
- (D-i, chosen) *graph-ingest second, service third*: 17 / 30,613 then 7 / 20,123. The first component's proofs run
  under the lifecycle suite with no boot path — the pin's own shape; the first boot is then a 7-package review.
- (D-iii) *rule core fourth* (2 / 17,243 right after `service`): legal by imports (P7); semboids still waits for
  `graph-clustering` and `websocket`; SemSource's composition completes one change later. Owner question (c).
- (D-vi) *verification-consumer order*: after change 3, change 4 = semboids' remainder 9 / 36,264 (`graph-index`,
  `graph/clustering`, `processor/graph-clustering`, `processor/rule` + `expression`, `output/websocket`, `pkg/buffer`,
  `pkg/graphview`, `pkg/revlag`), change 5 = SemSource's remainder 10 / 22,388, change 6 = the rest 6 / 5,696 plus
  the seam. Six changes; semboids measures from change 4, SemSource from change 5; change 4 mixes E1–E4, #34, the
  seam's largest importer and `graph-index` in one review and carries 2 of the 7 adapt-row owners. Owner question (c).
- (D-iv) *merge changes 4 and 5*: 16 / 37,292, 212 test files; rejected for the same review-size reason as D-ii.

Why D-i wins: it is the only shape in which every change has one provable claim, every single-package repair row is
admitted with its proof in the same change, the largest change is 30,613 production lines, the first component's
recovery proofs come second rather than third, and the first-wave consumer's branch can start two changes before
the end. Its cost against D-ii is one more change and one more review.

## Open questions for the owner (questions with options; none are decided here)

(a) **D13 / epic #24 after SemSource PR #223.** #24's body (lines 11, 16) still names `internal/sourcelifecycle` and
`internal/sourceintent`, which PR #223 deleted at `e4febc0d` ("removes 3,426 net production Go lines"; "SemEngine
\#18–20 must be reconsidered as separate framework contract questions"; PR #222: "remain separately justified"), §5.1,
§7.4. Options: (1) re-scope #24 to the pin's own composition points (`internal/boot/run.go:184,220,299,330,339`, §4.4)
with `pkg/lifecycle` + `pkg/projection` as the primitive and no consumer-derived input; (2) hold #24 until a consumer
names a workload (Q14's "named qualifying workload"), leaving `pkg/lifecycle` admitted at tier 0 as carried code with
its own tests; (3) close #24 and reopen when a consumer asks. This design assumes (1) or (2): `pkg/lifecycle` lands in
change 2 either way; only its proving workload differs.

(b) **Tests travel with packages, or a test-adaptation change.** Facts: testify is indirect-only (P14) and 438 files
use it; 278 `NewTestClient` sites (P15); 9 embedded-server files (P16), one of which needs a three-node cluster.
Options: (1) admit testify as a direct test dependency and port tests as they are, moving only the `NewTestClient`
sites and the T-B1 collisions (the design's assumption; cost: one `go.mod` line, zero rewrites); (2) rewrite the 438
files to std `testing` — not measured beyond the count, and no gate asks for it; (3) a separate test-adaptation
change per ported change (doubles the change count). For the cluster test (change 7): (1) carry embedded
`nats-server` for that one file under an `adapt` row with the reason; (2) add a cluster mode to `natsfixture` (three
containers; not in any current spec); (3) defer-exclude the cluster tests with the lost proof named ("replicas one
retains and handles occurrence once").

(c) **Order of changes 4–7: first-wave consumer or verification consumer first.** semboids is "the measuring fixture
the owner runs against each slice as soon as it can" (`inventory-scope.md:18`) and composes the rule core,
`graph-clustering` and `websocket` (P8); SemSource is the integration branch (§5.1) and composes none of those.
Options: (1) D2's order — SemSource composable after change 5, semboids after change 7; (2) D-vi — semboids after
change 4 (9 / 36,264, the largest and most mixed review), SemSource after change 5, six changes; (3) D-iii — rule
core fourth alone, then graph roots, SemSource, the rest: semboids still waits for change 7; (4) fold SemConnect's
`spatial`/`temporal`/`geojson`/`export` (4 / 5,236) into change 5 so both first-wave consumers can start after it.
Changes 1–3 are the same under every option.

(d) **Which change is "#9's first green extraction" for #24's "after" clause?** Options: change 1 (first merged
port; one owner, nothing boots), change 2 (first component, `pkg/lifecycle` admitted, recovery proofs green),
change 3 (first boot). The design assumes change 2.

(f) **Q4's "that one change".** Dormant `graph/llm` + `model/wire` span changes 2–7 (D8). Options: (1) "that one
change" means Slice 04A — the design conforms, with the seam as change 7's last task; (2) it means one OpenSpec
change — then either changes 2, 4 and 7 merge (32 / 66,595) or the seam is cut per importer at port time, which Q4's
own reason rejects; the owner says which. The design assumes (1).

(g) **Q12 for multi-package rows.** #16 spans `graph`, graph-ingest (change 2), `composition` (3), graph-query (4);
SS#1218 spans `pkg/errs` (1) and `service` (3); settlement spans `natsclient` (1) and graph-ingest (2). Options:
(1) per-package reading — each package is admitted when the evidence its own code can produce is green, and the row
names the later change for the rest (the design's assumption); (2) whole-row reading — a row's packages are admitted
only together, which forces `pkg/errs` to wait for `service` (change 1 loses `pkg/errs`, which `natsclient` imports:
not closable) or `graph` to wait for graph-query (changes 2–4 merge); (3) split the rows per package in the ledger so
each has single-package evidence (a ledger edit, not a code change).

(h) **Non-context enders on nine ported types.** `Pool[T].Stop(timeout)`; no-argument `Stop` on `Set`,
`readiness.Watcher`, `View[T]`, `embedding.Worker`, `EnhancementWorker`, `ReviewWorker`, `resource.Watcher`; and
`CoalescingSet.Close()` (Context). Two wait unbounded today (`resource.Watcher`, `CoalescingSet`). Options: (1)
SS#1415-class `adapt` rows — each gains a context-bounded ender, proven by the suite in the change that ports the
type (the design's assumption): 9 signature changes in changes 1, 2 and 4, plus the cross-package callers ported
later as `class:port-refactor` rows — 8 call sites: 3 in change 5 (`graph-embedding/component.go:875,882`,
`fusionnats/client.go:103`), 1 in change 6 (`rule/entity_watcher.go:980`), 4 in change 7
(`graph-clustering/component.go:1232,1239,1260,1266`); the remaining callers (`dispatcher.go:231`,
`summary_view.go:160,173,184`) are in the same change as their types, and `Set` and `resource.Watcher` have no
cross-package production caller of the changed method; (2) carry with
reason, driven by a goroutine adapter whose leak the completed-joins check reports — a gate that reads red by
design, and the two unbounded waits stay; (3) exclude the nine from the suite as internal workers, with the
context-ownership gate applied only to the processor `Component`s and `service` owners — the two unbounded waits
then need their own rows or a carried-root reason; (4) add `Stop(ctx)`/`Close(ctx)` beside the old method (a
compatibility shim) so later callers port unchanged and the shim is removed with the last caller in change 7 — two
enders per type for up to five changes.

## Owner rulings (2026-10-01, on #9)

Each question above was ruled on #9 (comment of 2026-10-01) with the design's assumed answer, so the chain and the
first change's scope stand as written:

- **Chain accepted**: seven changes by root-set closure, D2's order, each admitting its packages with their tests,
  ledger rows and repair proofs green.
- **(a)** #24 re-scoped to the pin's composition points: `pkg/lifecycle` + `pkg/projection` are the primitive; the
  input is `internal/boot/run.go`'s composition points, not a consumer's code. Recorded on #24.
- **(b)** Tests travel with their packages; testify becomes a direct test dependency; only `NewTestClient` sites and
  T-B1 collisions move. The cluster test carries embedded `nats-server` for that one file under an `adapt` row.
- **(c)** Changes 4–7 in D2's order: SemSource composable after change 5, semboids after change 7.
- **(d)** #24's "first green extraction" is change 2.
- **(f)** Q4's "that one change" is Slice 04A as a whole; the seam is change 7's last task.
- **(g)** Q12 is read per package for multi-package repair rows; the row names the later change for the rest.
- **(h)** Non-context enders get SS#1415-class `adapt` rows, proven in the change that ports the type; the eight
  later cross-package callers are `class:port-refactor` rows in changes 5, 6 and 7.

Consequence for this change: it archives as design-only. Each of the seven changes carries its own spec deltas (D10),
tasks and holds in its own OpenSpec change and claim PR, `setup-04a-01-floor` first.

## Declared costs

- Seven changes, seven reviews, seven ledger extensions; the largest review is change 4 (8 packages, 148 test files,
  six owners of which three need a new `Stop(ctx)`).
- Change 1 boots nothing: its value is the harness, the port mechanics, and one owner (`metric.Server`) under the
  suite. If the owner answers (d) with "change 1", green means less than #24 may have intended.
- Change 2 proves the first component without a boot path; the first operator-visible process is change 3.
- `go-openai` is a direct dependency for six changes (2–7) because of dormant `graph/llm`; `task vuln` scans it.
- After `Restart`, `URL()` changes; every restart-shaped test stops its owner before and starts it after. A
  component's own reconnect across a broker restart is not proven in 04A.
- The failing-factory parameter changes both `lifecycletest.Run` call sites and adds three failing factories in
  change 1; every later owner adds one.
- Thirty-seven test-side `Observe` adapters are written against unexported fields; each is a per-package review
  item. Ten owners need a signature change or a ruling (h) before the suite can bound their ender; six cross-package
  call sites of those signatures are ported in changes 5 and 7 as port-refactor rows. `fusionnats.Client` is outside
  the suite with a recorded reason.
- The owner scan is go/ast over a type's own methods and constructors; owners by embedding or delegation are found
  by reading, not by the tool, so a reviewer of each change re-checks its packages for them.
- `cover-check.sh`'s hard-coded targets (`:73-75`) are edited in every change; the context-root triage is ledger
  work (34 rows' notes) with no gate behind it.
- Not measured: the 438-file testify rewrite under (b)(2); a cluster-mode fixture under (b); the effect of (g)(3) on
  the ledger's row count; the review size of D-vi's change 4 in test files (its package list is measured).

## Tasks named per change (not drafted; follow acceptance)

1. Harness extension (D4-A, B, C; D5 with three failing factories; I8; `internal/harness/semantictest` and
   `internal/harness/payloadfixture` with the import bound) with its own proofs; port the 16 packages and tests;
   `test_client.go` and `payloadregistry/testing.go` adaptations; five adapters and suite runs (`metric.Server`,
   `natsclient.Client`, `TemporalResolver`, `CoalescingSet`, `resource.Watcher`); two adapt rows for the unbounded
   waits; 16 package rows + separated-package rows + Tier-1 cross-check; five spec deltas; context-root triage for 14
   roots; `AGENTS.md:43-44` fix; `go.mod` direct deps.
2. Port 17 + 2 dormant; #29; SS#1411 guard; `_test.go` KV setter and the D8/D7 proofs (#19, #20); #15 and settlement
   with restart and process host; Q13 row; #16 declaration and graph-ingest sites; #33 first processor; four
   `Stop(ctx)` adapt rows; seven adapters; `graph-ingest-recovery`, `projection-mutation`, `graph-transport-boundary`,
   `component-registration` deltas; dormant rows for `graph/llm`, `model/wire`.
3. Port 7; #30; #17 + `config-desired-state`; SS#1415, SS#1218 proof, SS#1220; `composition.Analyze` refusal;
   ten adapters; `operator-surface` delta; DX gates from #9 item 5 (example consumer composes `service`; metric-name
   drift test over the 213 `Name:` literals).
4. Port 8; #31; the pin's graph-index failed-start tests; three `Stop(ctx)` adapt rows; seven adapters; graph-query's
   #16 site and the "explicit list answers" scenario; `graph-query` delta.
5. Port 8; #34; two adapters and the `fusionnats.Client` exclusion row; port-refactor rows for
   `graph-embedding/component.go:882` and `fusionnats/client.go:103`; `change-observation` and `fusion-boundary`
   deltas; SemSource integration-branch start
   note (a pointer on #9, not a SemEngine task).
6. Port 2; E1–E4; I10 `rule-core` delta; three adapters (`ConfigManager`'s binds `targets`).
7. Port 7; #36 (cluster test per (b)); #32, #35; delete `graph/llm`, `model/wire`, `go-openai`; seam test; three
   adapters; port-refactor row for `graph-clustering/component.go:1232,1239,1260,1266`; SemConnect's observed
   column (#9 item 4) closes.

## Correction pass (after `design-review.md`, verdict BLOCKING)

1. BLOCKING, D7 / costs / change 2 — the "landed, not admitted" rule is gone (D1). The chain is restructured so
   `pkg/projection` and graph-ingest are admitted together: graph-ingest's closure is change 2 (17 / 30,613),
   `service`'s remainder change 3 (7 / 20,123); seven changes. The consequence is measured (D-v rejected, D-ii and
   D-i costed in Options); no harness piece moves — all three primitives were already change 1 and their first
   consumer is now change 2. Ruling-conformance table added (D12) with three deviation/confirmation rows as owner
   questions (f), (g), (h).
2. HIGH, P2 / D3 / D5 / D6 — owners recounted over all 65 packages: 30 ported, listed with file:line in Context and
   placed per change (1 / 5 / 10 / 6 / 2 / 3 / 3). `metric.Server` is change 1's owner and #38's first real target
   (failing factory: bound port). Eight non-context `Stop`s (the review's five plus `EnhancementWorker.Stop()`,
   `ReviewWorker.Stop()`, and `config.Manager`) are placed as SS#1415-class adapt rows in changes 2 and 4 (D6, D7)
   with owner question (h); `rule.ConfigManager.Start(ctx, targets)` noted.
3. HIGH, D4-C — stable host port withdrawn; re-read-and-reconnect chosen with the cost of each weighed; the
   one-replacement rule is untouched; the unmeasured reconnect premise is dropped; `Restart` failure is reported,
   not retried.
4. HIGH, D8 — the span is now stated as six changes (2–7, since `graph/inference` is in change 2) and reconciled
   with Q4 in D12; owner question (f) with the merged-change cost (32 / 66,595).
5. MEDIUM, D5 — call sites corrected to `refowner_test.go:303-304` and `fixture_integration_test.go:519`; natsfixture's
   failing factory (a failing `deps.start`) named; P23 added.
6. MEDIUM, D2 / D11 — "retires context roots" replaced by ledger triage per `setup-plan.md:220-222` (D9); T-B1
   restated as the import-graph test with its four collisions placed by change; no new guard proposed; P17 reworded.
7. MEDIUM, D7 / D9 — Q13 PR #1437 row (change 2), separated-package rows (changes 1, 2, 7), Tier-1 cross-check
   (change 1), port-refactor rows (with each refactor), and `proving_tests` per row (every change) placed in D7.
8. MEDIUM, D2 / (c) — semboids' role as verification consumer stated in Context, the order rationale, D12, and
   question (c), with the measured verification-consumer order (D-vi) as an option.
9. NIT, D3 — `payloadregistry/testing.go:4` corrected to the `"testing"` import; its three helpers, their 8 callers
   in 4 packages named (P24; the home was corrected again under N1). NIT, D7 — `graph_emit.go:24,31` re-cited to
   `pkg/lifecycle`; `pkg/projection`'s path cited as `mutation_client.go:184-198`.
10. Withdrawn: owner question (e) (process host timing) — with graph-ingest in change 2 the host's first consumer is
    change 2, so change 1 is its only home. Also corrected while here: Option B's Q12 violation noted; both
    inventory checksums recorded in the header. (The `internal/semantictest` home and the direction rule written
    here were wrong; see N1.)

Re-check 1 (verdict CHANGES REQUESTED; three new findings):

- N1 (HIGH, D4 "Fixture homes", D3, D10.1, D12) — the "harness imports no ported package" rule is withdrawn: it could
  not coexist with T-B1 (`imports_test.go:48-72`, which leaves no legal home for a `testing.TB` helper outside
  `internal/harness/`) and reversed the plan's helper-kit item (`setup-plan.md:284-286`). Three options measured;
  (i) chosen: `internal/harness/semantictest` and `internal/harness/payloadfixture`, with the bound "`natsfixture`
  imports no ported package; other harness packages may import pure-library ported packages, cycle-checked per
  helper" and the two directions that still hold for I8/T-B8. D12 rows added for T-B1 and the helper kit; P25, P26
  added.
- N2 (MEDIUM, Context, P2, P22, D2, D3, D5, D6, D7) — owner definition stated (goroutine or held resource outliving
  the call, plus an ender) with its go/ast search (`ownerscan/main.go`, `owners-scan.txt`); 38 exported owners
  (30 by name + `natsclient.Client`, `TemporalResolver`, `CoalescingSet`, `resource.Watcher`, `BoundedDispatcher`,
  `KeyedPool`, `objectstore.Store`, `fusionnats.Client`) + 4 unexported, placed 5 / 7 / 10 / 7 / 3 / 3 / 3;
  `resource.Watcher`'s and `CoalescingSet`'s unbounded waits are change-1 adapt rows proven by the suite's join
  checks; `fusionnats.Client` excluded from the suite with reason; 37 adapters.
- N3 (MEDIUM, (h), D6, D7) — "callers inside the same packages" corrected: `graph-embedding/component.go:882` and
  `fusionnats/client.go:103` (change 5), `graph-clustering/component.go:1232,1239,1260,1266` (change 7) are
  cross-package callers ported after the signature changes; placed as `class:port-refactor` rows in D7, costed in
  (h) option (1), with a shim option (4) added.

Re-check 2 (N1, N2 resolved; N3 resolved for the listed callers):

- N3a (MEDIUM, D6, D7, (h)) — "`CoalescingSet` has no cross-package caller" was false: `CoalescingSet.Close()` is
  called at `processor/graph-embedding/component.go:875` (change 5) and `processor/rule/entity_watcher.go:980`
  (change 6). Both added to the D7 port-refactor row (now changes 5, 6, 7) and to (h) option (1), whose cost is 8
  later call sites, not 6.
- NIT — the helper cycle check is stated over each helper's full dependency closure, not its direct imports.
- NIT — subscription handles (`natsclient.Subscription.Drain(ctx)`/`Unsubscribe()`, `graphview.Subscription[T].
  Unsubscribe()`) are noted as covered by the issuing owner's adapter, not counted as owners.
