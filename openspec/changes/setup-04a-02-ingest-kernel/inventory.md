<!-- markdownlint-disable MD013 -->
# Inventory: setup-04a-02-ingest-kernel (#91)

- **base:** `1334ce6` (worktree `claude/setup-04a-02-ingest-kernel`; `main` at `89c878e`). Round 2 (2026-10-06)
  answers review round 1; the branch head `3fcf88a` differs from the base only in this folder. Round 3 (2026-10-07)
  adds §8, the probes the owner's rulings on #91 (comments 6035429806 and 6035477895) called for; §0–§7 are the round-2
  record, and where §8 corrects or supersedes a statement in them, §8 says so.
- **pin:** SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, from `gh api repos/C360Studio/semstreams/tarball/<sha>`
  (sha256 of the tarball `47a0c1c5a050a3d92b392f8b98c98a867c3c67d15cc8a5b275d4523f14773f48`). Every `path:line`
  without a repository prefix below is a pin path.
- **consumers (symbol level, `docs/inventory-scope.md`):** semsource `e4febc0d`, semconnect PR #74 head `dff12657`,
  semteams `d5ee6325` (`pkg/lifecycle` and agentic seams only), semboids `8c03cc53` (`pkg/lifecycle` and the workload
  it drives). Tarballs from `gh api`; no git command touched a sister checkout.
- **Local-only evidence (not committed; the commands and results are quoted here so the claims can be re-run):** the
  pin tarball; a copy of the base with the 19 packages placed at their destinations (`se-copy/`); the round-1 AST
  reader; the round-2 type-based reference reader (a `go/packages` program run inside a pin copy, described in §2);
  coverage profiles of the pin (`cov/unit.out`, `cov/integ.out`); probe outputs `probe-race.txt`,
  `probe-shuffle-{1,2,3}.txt`.

Question (rule 1 of `docs/inventory-scope.md`): what does porting closure(graph-ingest) − change 1 touch, and what
already exists or is already claimed on that territory? Repositories read: the pin (code facts), the four consumers at
symbol level for the 17 packages, nothing else; SemStreams issue #1411 and PR #1437 for the two repair rows that cite
them.

Shorthand used below: **D*n*** is a decision of this change's `design.md`; **foundation D*n*** and **foundation (a)–(h)**
are the decisions and the owner rulings of `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md`
(rulings at `:710-731`); **03B** is `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md`, whose
invariants I1–I10 are at `:766-791` and whose owner questions Q4, Q9, Q12, Q13, Q16 and Q18 were ruled on #8;
**SS#n** is a SemStreams issue.

## 0. The set, measured

`go list -deps ./processor/graph-ingest` at the pin: 38 module packages. Minus the 16 of change 1, minus `agentic`,
`internal/looptoken`, `vocabulary/agentic` (reached only through `component/dependencies.go:7` —
`"github.com/c360studio/semstreams/agentic"`, cut by #29), leaves the 17 of foundation D2 row 2 plus `graph/llm` and
`model/wire` (dormant, D8). Non-test lines 30,613 (+2,024 dormant); tests 154 files / 40,768 lines — both reproduce D2.

| Package | Lines / files | Tests (files / lines, integration) | `NewTestClient` | `time.Sleep` (tests) | `Skip` | Live production context roots |
| --- | --- | --- | --- | --- | --- | --- |
| `internal/lifecyclecleanup` | 38 / 1 | 1 / 93 | 0 | 0 | 0 | 0 |
| `types` | 188 / 2 | 2 / 426 | 0 | 0 | 0 | 0 |
| `model` | 1,397 / 5 | 5 / 1,973 | 0 | 0 | 0 | 0 (1 in a doc comment, `model/watch.go:24`) |
| `storage` | 356 / 2 | none | 0 | 0 | 0 | 0 |
| `pkg/worker` | 707 / 3 | 2 / 566 | 0 | 9 | 0 | 0 (2 in `doc.go` comments) |
| `graph` | 3,293 / 27 | 25 / 4,900 (3) | 11 | 0 | 0 | 0 |
| `graph/readiness` | 1,016 / 4 | 5 / 1,302 | 0 | 2 | 0 | 0 |
| `graph/structural` | 883 / 4 | 2 / 736 | 0 | 0 | 0 | 0 |
| `internal/componentadmission` | 8 / 1 | none | 0 | 0 | 0 | 0 |
| `internal/graphmutation` | 264 / 2 | 2 / 233 | 0 | 0 | 0 | 0 |
| `pkg/dispatch` | 1,122 / 5 | 4 / 1,193 (1) | 1 | 5 | 0 | 0 (1 in `doc.go:63`) |
| `storage/storeregistry` | 115 / 1 | 1 / 131 | 0 | 0 | 0 | 0 |
| `graph/inference` | 5,338 / 14 | 12 / 4,801 | 0 | 1 | 0 | 0 |
| `pkg/projection` | 694 / 4 | 2 / 547 | 0 | 0 | 0 | 0 |
| `pkg/lifecycle` | 3,739 / 13 | 13 / 3,288 (2) | 7 | 0 | 0 | 0 |
| `component` | 5,585 / 28 | 22 / 6,651 (3) | 3 | 1 | 0 | 0 (5 in `lifecycle_test_suite.go`, not ported) |
| `processor/graph-ingest` | 5,870 / 9 | 56 / 13,928 (22) | 21 | 10 | 6 | 0 |
| `graph/llm` (dormant) | 842 / 8 | 2 / 223 | 0 | 0 | 0 | 0 |
| `model/wire` (dormant) | 1,182 / 7 | 6 / 1,038 | 0 | 0 | 0 | 0 |

Measured by a local-only script, not committed (per directory, `*.go` split on `_test.go`; integration = `^//go:build.*integration`).
D2's "9 context roots" are the 5 in `component/lifecycle_test_suite.go` (a file that is not ported: its ledger row is
`adapt → internal/harness/lifecycletest`) and 4 inside doc comments; the change has **no live production root to
triage**. D2's "20 cleanup-guard hits" reproduce exactly (19 in `pkg/dispatch` and `processor/graph-ingest` tests, 1 in
`pkg/lifecycle/harness_gate_integration_test.go:113`).

## 1. Pin probes (architect contract, Extraction slices)

**P-1 unit runs at the pin** over the 17 + 2 packages: `go test -race -count=1 -cpu 1` — 17/17 `ok` (`probe-race.txt`);
`go test -count=5 -cpu 1 -shuffle=on` three times — all `ok`; seeds recorded per package in `probe-shuffle-*.txt` (e.g.
graph-ingest `1791315758…`, run 1). No unit flake at the pin. The 22 graph-ingest integration files were not run at
the pin (they start containers through `natsclient.NewTestClient`, which SemEngine does not carry).

**P-2 build against the ported floor.** The 19 packages placed at their destinations in a copy of the base, imports
rewritten (`pkg/cache`, `pkg/resource`, `pkg/timestamp`, `pkg/tlsutil` → `internal/…`; `internal/semantictest` →
`internal/harness/semantictest`), `component`'s agentic field cut as #29 states. `go build ./...` fails only on the
registration methods change 1 removed from `metric.MetricsRegistry` (`RegisterCounter`/`Gauge`/`Histogram`/`*Vec`;
ledger `metric` row): `pkg/worker/pool.go:133-139`, `graph/readiness/gauges.go:149-157`, `graph/inference/metrics.go:45-99`,
`component/metrics.go:75-78`. With a throwaway shim the build is clean: **no other production API the change uses was
changed or dropped by change 1.** `go get` of the pin's `go-openai v1.41.2` and `x/net v0.53.0` downgrades `x/text`,
`x/tools`, `x/vuln` below the base's versions; the port takes the base's versions instead.

**P-3 test compile against the floor** (`go test -c -tags integration -gcflags=-e`): 43 `natsclient.NewTestClient` /
`TestClient` / `WithKV` / `WithStreams` / `WithKVBuckets` / `NewSharedTestClient` / `WithJetStream` / `WithMinimalFeatures`
uses (graph 22 errors, component 8, dispatch 2, lifecycle 14, graph-ingest 71) — the pin's test client, which change 1
replaced with `natsfixture`. Eight test files import packages outside the set:
`processor/graph-ingest/{component_fixture_test.go:13,15, factory_registry_test.go:17, indexing_profile_registry_test.go:12-13,
merge_entity_integration_test.go:25, registered_type_gate_integration_test.go:16,22, resident_stamp_integration_test.go:20}`
(`agentic`, `agentic/research`, `payloadbuiltins`), `pkg/lifecycle/harness_gate_integration_test.go:19` (`payloadbuiltins`),
`storage/storeregistry/storeregistry_test.go:9` (`pkg/fusion`, change 5; used once, `:30` —
`var _ fusion.StoreResolver = (*storeregistry.Registry)(nil)`). **Not in the foundation design**: D2 counted production
edges only. `component_fixture_test.go` holds the package's shared fixtures (`withAuthority`, `testDependencies`,
`newTestPayloadRegistry`; 74 + 18 + 8 uses), so it is adapted, not dropped.

**P-4 the repository's checks on the copy** (`go test ./internal/harness/contract/`, git-initialised copy):

| Check | Result on the copy |
| --- | --- |
| `TestNoRetainedContext`, `TestNoDeploymentAuthorityNames`, `TestNoSemStreamsImport`, `TestNoAggregatorPackage`, `TestNoBareSelect`, `TestNoHiddenTests`, `TestFixtureImportsNoPortedPackage` | pass |
| `TestImportGraph` | `component/lifecycle_test_suite.go:11` imports `testing` (a T-B1 collision: the 03B rule, enforced by `TestImportGraph`, that no non-test file outside `internal/harness` imports `testing`; the file is not ported) |
| `TestNoSecondAuthorityField` | `graph/inference/hierarchy.go:100-101` (`HierarchyConfig.Org`, `.Platform`, both `json:"-"`); `graph/llm/prompt_types.go:14-15` (`EntityParts.Org`, `.Platform`); the spec names both shapes |
| `TestPublicSignatures` | `component.Registry.CreateComponent` (`component/registry.go:194` — `_ componentadmission.Access,`), reached from `component.NewRegistry`, `component.Registry`, `graphingest.Register` |
| `TestOneImagePin` | 6 false positives: `component/port_resolver_test.go:22-23` (`"nats:in"`, `"nats:out"`), `component/port_test.go:79,86,543,544` — port resource IDs `nats:<subject>` (`component/port_nats.go:14` — `return fmt.Sprintf("nats:%s", n.Subject)`), not images |
| `TestNoFixedAddressesInTests` | 11 lines `nats://localhost:4222` in graph-ingest tests (`authority_gate_test.go:27,59`, `component_test.go:623,726,747,987,1015`, `merge_entity_bench_test.go:109`, `metrics_test.go:164`, `query_contract_guard_test.go:368`, `factory_registry_test.go:24`) |
| `TestNoSleepsInTests` | 28 (`pkg/worker` 9, `graph-ingest` 10, `pkg/dispatch` 5, `graph/readiness` 2, `graph/inference` 1, `component` 1) |
| `TestNoSkippedTests` | 6, all `processor/graph-ingest/component_test.go` (`:579,639,663,680,702,945` — `t.Skip("requires real NATS connection - move to integration tests")`) |
| `cleanup-roots-check.sh` | 20 (above) |

**P-5 lifecycle suite against the service the change ports**, `graph-ingest.Component`, on the copy:
`lifecycletest.Run` through a throwaway external-package adapter (`internal/zzprobe`, run with `scripts/test-integration.sh`),
`natsfixture` broker, `ENTITY` stream pre-created, failing factory = a client on a stopped fixture's URL. All eight
checks pass (2.47 s). **Caveat:** an external package cannot read the component's handles, so `Observe` was vacuous;
the join and "holds nothing" checks saw only return values. Without the pre-created stream, `Start` spends 49 s in
`waitForStream` (`processor/graph-ingest/component.go:1621` — `maxRetries := 30`) and fails naming the stream.

**P-6 `pkg/worker.Pool` second Stop after a timed-out Stop** (throwaway test in the pin copy): first `Stop(1ms)` returns
`timeout waiting for workers to stop`; the second `Stop` **panics `close of closed channel`** (`pkg/worker/pool.go:242`
— `func (p *Pool[T]) Stop(timeout time.Duration) error {`; `stopped` is set only on the clean path).

**P-7 coverage at the pin** for this change's critical-list packages (foundation D10; 03B D10 as extended by #8
comment 5932313950), measured the way `scripts/cover-check.sh` measures (statements per block, a block counted once,
covered if any profile covered it). Unit: `go test -count=1 -coverprofile` over the ten packages, all `ok`. Merged:
the same plus `go test -count=1 -tags integration -coverprofile ./processor/graph-ingest/ ./graph/ ./component/
./pkg/lifecycle/` on the pin copy with Docker, all `ok`, exit 0 (the pin's own test client, which SemEngine does not
carry, so this is the pin's figure, not the port's). The other six packages have no integration test.

| Package | Target profile (D11) | Unit | Merged |
| --- | --- | --- | --- |
| `processor/graph-ingest` | merged | 67.0% | 84.0% (1756/2091) |
| `graph` | merged | 83.9% | 87.9% (452/514) |
| `graph/readiness` | unit | 85.2% | — |
| `pkg/projection` | unit | 66.5% (153/230) | — |
| `internal/graphmutation` | unit | 70.3% (64/91) | — |
| `component` | merged | 63.5% | 64.8% (1122/1731), of which 463 lines are `lifecycle_test_suite.go`, not ported |
| `storage/storeregistry` | unit | 100% | — |
| `pkg/lifecycle` | merged | 67.7% | 70.7% (736/1041) |
| `graph/inference` | unit | 47.4% (766/1616) | — |
| `graph/structural` | unit | 89.8% | — |

**P-8 what removing dead surface buys.** The same profiles with every statement inside a function the surface audit
drops (§2, disposition `drop`, plus the transitive drops listed there) removed from both sides, and
`component/lifecycle_test_suite.go` removed. An upper bound for the alive code: a test that read only dropped surface
leaves with it and may also have covered alive code.

| Package | After the drop | Statements still to cover for 80% | Largest uncovered alive functions |
| --- | --- | --- | --- |
| `processor/graph-ingest` | 84.0% | 0 | — |
| `graph` | 85.5% (355/415) | 0 | — |
| `graph/readiness` | 83.6% (138/165) | 0 | — |
| `storage/storeregistry` | 100% | 0 | — |
| `graph/structural` | 89.6% | 0 | — |
| `internal/graphmutation` | 69.7% (62/89) | 10 | `client.go` `validateAppendResponse` 8, `Reconcile` 6, `Delete` 3, `request` 3 |
| `pkg/projection` | 66.5% (153/230; all its dead surface is kept, K1) | 31 | `mutation_client.go` `newMutationError` 10, `cloneDetail` 8, `Create` 7, `canonicalizeGroupMutation` 7, `NewMutationClient` 6, `Delete` 6, `Append` 5 |
| `component` | 73.9% (975/1320) | 81 | `schema_tags.go` 100 (`generateNestedSchema` 37, `GenerateCacheFieldSchema` 20, `inferPropertyFromType` 18), `registry.go` `Declare` 19, `validation.go` 34 |
| `pkg/lifecycle` | 70.7% (all its dead surface is kept, K1) | 97 | `manager_query.go` 125 (`Children` 36, `AssertRuleWritable` 13, `List` 12, `ListWorkflows` 10), `workflow.go` `validate` 19, `manager.go` `Complete` 15, `Fail` 9 |
| `graph/inference` | 47.9% (674/1406) | 451 | `config.go` 175, `storage.go` 154, `review_worker.go` 126, `semantic_gap.go` 100, `applier.go` 41, `http_handlers.go` 39 |

## 2. Surface audit (Extraction slices)

**Method (round 2, by type).** Round 1 matched methods by name, which hid dead surface as well as inventing it: a
method stayed "alive" when any other type had a method of the same name (`lifecycle.Manager.Watch` stayed alive
through `config/manager.go:425`'s `kvHandle.Watch`). Round 2 reads references by type. A `go/packages` program loads
every package of the pin with its tests and the `integration` tag and resolves each identifier through
`types.Info.Uses` (generic instantiations mapped to their origin), so each read is attributed to the exact object it
names. A reader is classified as the package's own non-test code, its tests, an admitted package (the 65 of
`docs/tier1-cross-check.md`) or another pin package. The other direction, a method reached only through an
interface value: a concrete method also counts as read when its type implements a module interface whose method of
that name has a non-test reader in an admitted package. Methods of standard-library interfaces (`Error`, `String`,
`Unwrap`, `MarshalJSON`, `UnmarshalJSON`; 15 of them) are read by the standard library and are not candidates.
Consumers are read by name, only in files that import the package, and every hit was then read by hand: of the
non-standard method hits, only `lifecycle.Manager.Create` (semboids `internal/sim/lifecycle.go:52` — `Create(ctx
context.Context, p lifecycle.Participant) error`, a consumer-side interface) and `lifecycle.NewManager` (semteams
`cmd/semteams/main.go:791`, semboids `cmd/semboids/main.go:177`) are real. Four were another type with the same name
and are dead: `graph.Event.Payload` (semconnect `gateway/cs-api/systemevents.go:121` sets its own `ev.Payload`),
`graph.IncomingEdges.Count` (semsource `processor/mcp-gateway/graph_matches.go:160` reads `body.Count`), `readiness.Set.Stop`
(semconnect `conformance/cmd/index-readiness/main.go:93` stops its own type; that file reads only
`readiness.BucketGraphStatus` and `readiness.KeyGraphIndex`), and `component.SimpleMockComponent.ConfigSchema`
(semsource components define their own `ConfigSchema`). Dead = no non-test reader in its own package, in an admitted
package, through an admitted interface read, or in a consumer. Dormant packages (`graph/llm`, `model/wire`) are not
audited.

- **(a) exported, nothing reads it: 186** (appendix, each with its disposition). Against round 1's 157: 35 more found
  dead by type, and the dormant packages' entries are gone. The clusters the by-name pass hid:
  - `pkg/dispatch.BoundedDispatcher` with `New`, `Config`, `Deps`, `Submit`, `Stop`, `Stats` and `ErrQueueFull`: its
    only readers are `processor/gated-dag/executor.go` and its test (not admitted). graph-ingest uses only `KeyedPool`
    (`processor/graph-ingest/keyed_ingest.go:93` — `pool, err := dispatch.NewKeyedPool(poolCtx,
    dispatch.KeyedConfig[ingestWork]{`) and only `SubmitBlocking` and `Stop` on it (`component.go:1585`, `:1143`), so
    `KeyedPool.Submit` and `KeyedPool.Stats` are dead too.
  - **Transitively, `pkg/worker`:** its only importers are `pkg/dispatch/dispatcher.go` and `pkg/dispatch/errors.go`
    (`grep -rl` over the pin, non-test). Once `BoundedDispatcher` goes, the one read left is the sentinel
    `pkg/dispatch/errors.go:24` — `ErrStopped = worker.ErrPoolStopped`, which `KeyedPool` returns
    (`keyed_pool.go:227,267,279`). No consumer imports `pkg/worker` or `pkg/dispatch` (search of the four tarballs for
    the two import paths: empty).
  - `graph/readiness.Set` with `NewSet`, `Start`, `Stop`, `FullyCovered`, `Dumps` (and its `Dump` and `Verdict`
    types): readers `gateway/graph-gateway/readiness_surface.go:46` and `test/e2e/scenarios/stages/entities.go:69`, not
    admitted. `readiness.Watcher` is alive (graph-clustering `component.go:1509`, `pkg/fusion/fusionnats/client.go:139`).
  - `graph/inference`: `ReviewWorker.Pause`/`Resume`, `NATSAnomalyStorage.Watch`/`Cleanup`, `Result.Duration`,
    `HierarchyInference.ClearCache`, `HTTPHandler.RegisterHTTPHandlers`.
  - `pkg/lifecycle.Manager`: `Watch` (read only by `processor/gated-dag/executor.go:119` and, through the
    `LifecycleManager` interface, `gateway/lifecycle-gateway/handlers.go:475`; neither admitted), `History`, `List`,
    `References`, `GetWithRevision`, besides round 1's nine.
  - `pkg/projection.MutationClient.Create`, `Append`, `Delete` and `ErrInvalidContract`.
  - `component.Registry.Snapshot` (one of the four access-token methods; the other three have admitted readers:
    `CreateComponent` 1, `SealComposition` 2, `Snapshots` 2).
  - `component.Discoverable.ConfigSchema` has no non-test reader in an admitted package (its readers are
    `componentregistry` and the schema tooling, not admitted), so every component's `ConfigSchema` method looks
    dead. It is an interface method of the component contract that consumer components implement (semsource:
    16 non-test hits of `ConfigSchema` across its components); removing it is a component-model change, not a port
    drop (disposition K4).
- **Transitive drops** (readers only among dropped symbols; the port task's `gopls references` check settles the
  closure): `component/config_validator.go` whole (`ValidateWithSchema`, `ConfigPersister` and
  `ConfigComponentRegistry` are read only by the two dropped validators at `:91` and `:123`); `ProcessorMetrics`,
  `ReviewMetrics`, `KeyedStats`; `readiness.Dump` and `Verdict`; `dispatch.Config` and `dispatch.Deps`; and, under
  owner question E, `worker.Pool` and its whole API.
- **(b) config.** `processor/graph-ingest` decodes its config with plain `json.Unmarshal`
  (`processor/graph-ingest/component.go:672` — `if err := json.Unmarshal(rawConfig, &config); err != nil {`): an
  unknown key is accepted silently. All four fields (`Ports`, `EnableHierarchy`, `EnableTypeSiblings`, `IngestLanes`)
  are read by behavior; `IngestLanes < 1` is clamped to 1 in `Validate`. `component/schema.go:29` and
  `component/validation.go:186` state that unknown fields are allowed. **`graph/inference.Config`** (`config.go:17`)
  has 43 JSON fields in eight structs (`config.go:17-180`). The pin already refuses its unknown keys:
  `graph/inference/config.go:249` — `func RejectUnknownKeys(raw json.RawMessage) error {`, called by
  `processor/graph-clustering/component.go:701` (ADR-054, "no silent drop"). Its only way in is
  `processor/graph-clustering/component.go:77` — `AnomalyConfig inference.Config` (change 7); graph-ingest uses only
  the hierarchy half (`inference.HierarchyConfig`, `NewHierarchyInference`, `HierarchyInference`, `EntityManager`,
  `TripleAdder`, `HierarchyContainerMessageType`; `grep -o 'inference\.[A-Za-z]*'` over graph-ingest's non-test
  files). Readers inside the package (a by-name search of each Go field name over the package's non-test files other
  than `config.go`, plus `config.go`'s own `BuildPredicate`, `ShouldAutoApply`, `ShouldQueue`): 38 fields are read by
  the orchestrator (`detector.go`), the detectors (`semantic_gap.go`, `core_anomaly.go`, the transitivity detector),
  `review_worker.go`, `applier.go` and `storage.go`. Five are parsed, defaulted and validated but read by no
  behavior: `Config.RunWithCommunityDetection` (`config.go:22`), `ReviewConfig.BatchSize` (`:117`),
  `ReviewQueueConfig.RequireLLMClassification` (`:165`), `StorageConfig.RetentionDays` (`:175`) and
  `StorageConfig.CleanupInterval` (`:178`; the last two would be read by `NATSAnomalyStorage.Cleanup`, which nothing
  calls). The only workload that sets these fields and runs the detectors is graph-clustering (change 7).
  **`pkg/dispatch`**: `KeyedConfig` is built in code by graph-ingest (`keyed_ingest.go:93-106`), not decoded from
  operator JSON; `BoundedDispatcher`'s `Config` drops with it.
- **(c) described, not implemented:** `processor/graph-ingest/TEST_DISPUTE.md` (74 lines) describes test/API disputes
  against a `natsclient` API that no longer exists; `component/README.md` (445), `graph/README.md` (144),
  `pkg/worker/README.md` (827), `types/README.md` (105), `processor/graph-ingest/README.md` (127) are read claim by
  claim in their port tasks, as change 1 did.

## 3. Inventory categories

**1. The claimed gap.** The change claims "graph-ingest runs as a component under the lifecycle suite, with no boot
path". Searches on the base: `git grep -n 'graph-ingest\|graphingest\|LifecycleComponent\|componentadmission' -- '*.go'`
→ no hits; `ls processor component graph pkg/lifecycle pkg/projection` → absent. The gap is real. The harness pieces
the proofs need exist (`natsfixture.Restart` `internal/harness/natsfixture/restart.go:40`, `NewFaultKV`
`faultkv.go:47`, `prochost.Start` `internal/harness/prochost/prochost.go:72`, `prochost.Helper` `:43`,
`lifecycletest.Run` `lifecycletest.go:77`, `payloadfixture.NewWithSubset`
`internal/harness/payloadfixture/testing.go:33`). `FaultKV` fails a write before or after the real call
(`faultkv.go:52,56`); it cannot hold one open.

**2. Every current spelling of the facts this change models.**

- *Failed-start rollback:* `internal/harness/natsfixture/rollback.go:17` (base; adapted copy, 15 s budget) and pin
  `internal/lifecyclecleanup/lifecyclecleanup.go:12` — `const failedStartRollbackTimeout = 5 * time.Second`
  (production policy, 34 call sites in 33 pin files, one in this change: `processor/graph-ingest/component.go:984`).
- *Owner-lifecycle state (SS#1411):* the one-shot `Start`/`Stop` state machine — `lifecycleMu`, `lifecycleUsed`,
  `terminal`, `startDone`, `stopping`, `cleanupPending`, a `Stop` loop that waits for an in-flight `Start`. At the pin
  28 non-test files carry it (`grep -rl lifecycleUsed --include='*.go'`, non-test); 12 copies in 10 admitted
  packages (P19 of the foundation design: 0 / 1 / 2 / 2 / 2 / 2 / 3 by change). This change's copy:
  `processor/graph-ingest/component.go:520` — `lifecycleUsed bool`, with the guard at `:954`
  (`if c.lifecycleUsed {`), the commit at `:975` (`c.lifecycleUsed, c.cleanupPending = true, true`) and `Stop`'s
  loop at `:1074` (`if !c.lifecycleUsed {`). The base's owners spell the same job differently: `metric/handler.go:37-38`
  (`used`, `stopping`; guard `:101-106`) and `natsclient/client.go:154` (`closing`; `Connect`'s guard `:707-714`). No
  consumer carries the copy (search of the four tarballs, non-test: 0 files). Table in §4.
- *Strict configuration decoding:* `graph/inference/config.go:249` (`RejectUnknownKeys`, strict decode of a raw
  object); graph-ingest decodes leniently (`component.go:672`).
- *Deployment authority:* `graph/inference.HierarchyConfig.{Org,Platform}` (`hierarchy.go:100-101`), graph-ingest's
  unexported `org`/`platform` copies (`component.go:772-773`), `graph/llm.EntityParts.{Org,Platform}` (dormant). The
  carrier is `component.Dependencies.Platform` (allowed by name in `internal/harness/contract/authority_test.go`).
- *Metric registration:* the pin's six `Register*` methods (removed by change 1) and graph-ingest's process-global
  `sync.Once` collectors with a `prometheus.DefaultRegisterer` fallback (`component.go`: 12 sites,
  `poison_inventory.go`: 1; `graph/readiness/gauges.go`: 1); the base has one home, `metric.RegisterOrGet`
  (`metric/registry.go:29`), and no `DefaultRegisterer` use (`git grep -n DefaultRegisterer` → none).
- *Outward-facing `semstreams` names in the 17:* metric namespace `"semstreams"` ×17 (`component/metrics.go` 4,
  `graph/readiness/gauges.go:79`, `graph/inference/metrics.go:32`, graph-ingest 13 incl. `component.go:2851`,
  `poison_inventory.go:66`); bucket `BucketSemStreamsConfig = "semstreams_config"` (`graph/constants.go:74`; readers
  at the pin: `graph/kvcatalog.go`, `config/bucket_name.go`, `config/manager.go`); `InterfaceType =
  "semstreams.graph.mutation"` (`internal/graphmutation/protocol.go:12`); `alertDigestDomain =
  "semstreams.graph.alert.v1"` (`graph/events.go:19`); bucket description "SemStreams runtime configuration"
  (`graph/kvcatalog.go:134`); 79 comment lines. No `SEMSTREAMS_` env var and no `SemStreamsBase` reader in the 17.
  Consumers that spell them: §5 item (e).
- *Reserved request subjects (#16):* `graph/exact_entity.go:15`, `processor/graph-ingest/query.go:27,34,41,48,55`,
  `internal/graphmutation/protocol.go:16` (`graph.mutation.>`), prefix test `canonical_mutations.go:174`; later
  changes: `component/flowgraph/flowgraph.go:264` (3), graph-query (4).
- *Test fixture client:* `natsclient/test_helpers_integration_test.go:47` — `func newFixtureClient(t *testing.T, opts
  ...natsfixture.Option) *fixtureClient {` (base, package-internal); this change needs the same helper in five
  packages (43 sites); foundation D4 lists 20 later packages with `NewTestClient` sites.
- *Process kill between apply and acknowledgement:* no test at the pin or in the four consumers (03B `:485`, scope
  Q4.4); graph-ingest's order at the pin is apply → durable guard stamp → in-memory stamp → `Ack`
  (`keyed_ingest.go:116` comment, `:213-222`).

**3. Adjacent claims.** `gh pr list --state open --json number,title,isDraft,headRefName` (2026-10-06): **#92** and
**#93** only.

- **#92** (draft, `claude/semengine-names`, claim for #69, head `aceb2be`): 25 files (`gh pr view 92 --json files`):
  `docs/admission-ledger.yaml` (+28 −5), `internal/cache/` (6 files), `metric/` (9), `natsclient/` (6), `vocabulary/`
  (3). The ledger is the one file this change also edits. #92's hunks
  (`gh api repos/C360Studio/semengine/pulls/92/files`) start at base lines 518, 614 (`vocabulary` row, `:506-624`),
  923 (`metric`, `:842-1001`), 1299, 1329 (`pkg/cache`, `:1232-1404`), 1426 and 1701 (`natsclient`, `:1405-2172`).
  This change edits the `internal/lifecyclecleanup/lifecyclecleanup.go` file row (`:148-162`) and adds package rows;
  neither touches #92's rows. No capability delta overlaps (#92 has no `openspec/` file).
- **#93** (this claim).
- Issues on the territory: #15, #16, #19, #20 (repair rows; #19 Q6 and #20 Q7 ruled 2026-10-01), #29, #33
  (`class:port-refactor`), #75, #78, #81, #85 (#48 follow-ups), #24 (durable execution, blocked on #9), **#77**
  (`status:needs-decision`): its inventory passed (comments 6024786639, 6024787123; review record 6024790647) and its
  ruling question is posted (comment 6024793500). It recommends component-owned rollback through a helper SemEngine
  makes public, and asks separately for an exception to #38 for a `Start` whose cleanup fails; it says #93's question A
  links there and #93's case-4 scenario waits on that grant. **#69** (ruled 2026-10-06: one rule for every
  outward-facing name, `semengine`; no consumer needs stored data kept).
- SemStreams: **SS#1411** (open): "decide one owner-lifecycle guard or record the idiom"; its ask is "an architect pass
  with the census above as its inventory; the design chooses between the shared guard and the recorded idiom, with the
  migration cost per component". **SS PR #1437** (open, head `0ea823a6`): graph-ingest half `component.go` +30/−5 and
  two test files (+146, +255); also `.agents/skills/new-payload/SKILL.md` +2, `docs/concepts/15-payload-registry.md`
  +2, the `agentic` fix and its OpenSpec change (`gh api repos/C360Studio/semstreams/pulls/1437/files`).
- Ledger rows on the territory: `internal/lifecyclecleanup/lifecyclecleanup.go` (file row, `adapt → natsfixture`),
  `component/lifecycle_test_suite.go` (`adapt → lifecycletest`), `processor/graph-query/lifecycle_owner_test.go`;
  `agentic`, `internal/looptoken`, `vocabulary/agentic` (`defer-exclude`). Specs: `lifecycle-suite`, `background-work`,
  `harness-boundaries`, `nats-fixture`, `transport-client` (settlement), `metric-registry`.

**4. Consumer at birth (new surface this change could add).** A shared fixture-client helper (5 packages, 43 sites
here; natsclient's own copy; 20 later packages); an owner-lifecycle guard (graph-ingest here; 11 later admitted
copies, §4); an `ExpectedRevision` field on `projection.ReconcileMutation` (#19 Q6; semsource #215, semconnect
`graph_mutations.go:214-215`); a reserved-subject declaration in `graph` (#16; graph-ingest, `graph/exact_entity.go`
now, graph-query in change 4). No other new exported symbol is needed by the issue's scope.

**5. The problem shape.** (i) *Owner ports under a portable lifecycle floor* — nearest instance: change 1's
`metric.Server` / `natsclient.Client` adapters (`metric/lifecycle_test.go`, `natsclient/client_lifecycle_test.go`).
(ii) *Background work in three shapes* — `internal/resource.Watcher.Run`, `internal/cache.CoalescingSet.Shutdown`.
(iii) *Classified refusal plus observed signal* for commit outcomes — `natsclient.SettleDelivery`
(`natsclient/delivery_settlement.go:277`) and the pin's `CommitUnknown` (`pkg/projection/mutation_types.go:74`).
(iv) *Create-vs-exists / one canonical collector* — `metric.RegisterOrGet`. (v) *One-shot lifecycle state machine* —
no shared instance anywhere: 12 admitted copies, two differently spelled base owners (category 2). (vi) *Strict
decoding of operator configuration* — `inference.RejectUnknownKeys` (`config.go:249`). (vii) *Connect a client to a
test broker with bounded cleanup* — one package-internal instance (`natsclient/test_helpers_integration_test.go:47`).

## 4. Same-class collision tables

### 4.1 Runtime coordination: failed-start rollback

| Dimension | Evidence |
| --- | --- |
| Semantic class | release what a failed `Start` acquired, bounded, keeping parent values |
| Owners | pin `internal/lifecyclecleanup` (5 s); base `natsfixture.rollback` (15 s); pin `service/component_manager.go` and `service_manager.go` (change 3), per #77's passed inventory |
| Catalogs | none (`git grep -n lifecyclecleanup` on base: ledger and the `natsfixture` comment only) |
| Status | component `Health()`; graph-ingest `cleanupPending`/`terminal` fields (`component.go:984-994`) |
| Lifecycle | rollback runs synchronously inside `Start`'s deferred block; on failure the obligations stay for `Stop` |
| Ownership | the component (pin shape); #77 asks whether external owners get a public helper or the managers own it |
| Readers | graph-ingest only in this change; 32 more pin files in later changes |
| Writers | n/a |
| Recovery | a failed rollback leaves `cleanupPending=true`; `Stop` retries cleanup; pinned by `processor/graph-ingest/lifecycle_owner_test.go:134` — `func TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop(t *testing.T) {` |

### 4.2 Runtime coordination: one-shot owner-lifecycle state (SS#1411)

| Dimension | Evidence |
| --- | --- |
| Semantic class | admit one `Start` per owner; refuse a second; make `Stop` safe before, during and after `Start`; record what a failed rollback left for `Stop` |
| Owners | 12 admitted pin copies: `processor/graph-ingest/component.go:520` (this change); `service/component_manager.go:102`, `service/message_logger.go:234` (change 3); `processor/graph-query/component.go:177`, `storage/objectstore/component.go:49` (4); `processor/graph-embedding/component.go:298`, `output/websocket/websocket.go:157` (5); `processor/rule/processor.go:102`, `processor/rule/cron_scheduler.go:54` (6); `processor/graph-clustering/component.go:646`, `processor/graph-index-spatial/component.go:187`, `processor/graph-index-temporal/component.go:196` (7). Base: `metric/handler.go:37-38` (`used`, `stopping`), `natsclient/client.go:154` (`closing`, with `conn != nil` as "started") |
| Catalogs | none; no doc, spec or contract names the idiom at the pin (SS#1411 body) or here (`git grep -n lifecycleUsed` on base: none) |
| Status | `Health()` per component; the fields themselves, read by package-internal tests (`processor/graph-ingest/lifecycle_owner_test.go`, `test_owner_test.go`, `test_owner_child_test.go`, `component_test.go`) |
| Lifecycle | variants: `cleanupPending` in 2 of 12 (graph-ingest `:975`, objectstore `:452`); the terminal flag is `terminal` in graph-ingest (`:521`) and `lifecycleTerminal` in graph-query (`:605`); the agentic copies set both in one statement (`c.lifecycleUsed, c.terminal = true, true`) |
| Ownership | each component instance; single use (a second `Start` returns `ErrAlreadyStarted`, `component.go:954-956`) |
| Readers | the component's own `Start`, `Stop`, `Health`; its package tests |
| Writers | the same |
| Recovery | a failed `Start` whose rollback fails stays `cleanupPending` and `Stop` retries (4.1) |

## 5. Adopter seam inventory

Surfaces reached from outside: `component.Dependencies` and `Registry` (consumer composition roots: semsource,
semconnect, semboids, semteams import `component` 76/23 times), `graphingest.Register` / `CreateGraphIngest`,
graph-ingest's config JSON, its metrics (scraped), `projection.MutationClient`, `pkg/lifecycle.Manager`, and three wire
or storage names.

1. **What must they know?** (a) graph-ingest needs its input stream to exist before `Start` (P-5: 49 s then an error);
   (b) `deps.Platform` must carry the authority (refused at construction, `component.go:721`); (c) the payload
   registry must hold the types graph-ingest decodes (the refusal text names `payloadbuiltins.Register`, which
   SemEngine does not ship, `component.go:739`); (d) metric names change from `semstreams_*` to `semengine_*`;
   (e) the mutation interface type, the configuration bucket and the alert digest domain change from `semstreams` to
   `semengine`.
2. **If they do nothing:** (a) a 49 s start then a boot error naming the stream — loud; (b) a construction error —
   loud; (c) a boot error naming a symbol that does not exist (#33) — loud but wrong; (d) dashboards go silent;
   (e) a flow config whose port declares `{"type": "semstreams.graph.mutation"}` no longer matches graph-ingest's
   port. Config typos: silently ignored today (§2 b).
3. **Where they find out:** (a)(b) boot error; (c) boot error with a wrong pointer; (d) nowhere; (e) at composition,
   when the port interfaces are matched (change 3 brings `composition`); config typo nowhere.
4. **What they should have to know:** nothing for (c) and the config typo — the refusal names the per-package call,
   and an unknown key is refused at construction; (d) and (e) are one-time renames under #69's ruling, made when a
   consumer adopts SemEngine (no consumer pins SemEngine yet).

Who spells the names of (e), `grep -rn` over the four tarballs (Go and JSON/YAML, evidence folders excluded):

- `semstreams.graph.mutation`: semsource `cmd/semsource/run.go:788`, `internal/governance/live_graph_integration_test.go:71,451`;
  semconnect `gateway/cs-api/component.go:227` (`graphMutationInterfaceType = "semstreams.graph.mutation"`),
  `deploy/semstreams.json:41`, `conformance/compose.semstreams.config.json:41`; semboids `configs/flock.json:114,151,341`
  and nine sites in seven integration test files under `internal/boidgraph`, `internal/zone` and `internal/sim`; semteams comments only in
  scope (`cmd/semteams/main.go:765`), and its flow configs spell it too (`configs/flow-bootstrap.json:211,298`,
  `configs/e2e-flow-bootstrap.json:180,267`; noted for the rename's cost, outside semteams' symbol-level scope).
- `semstreams_config`: semsource `internal/cutover/buckets.go:58`, `test/setup03a/qualification_test.go:997`,
  `test/setup03a/desired_lifecycle_correction_test.go:145`, `test/e2e/upgrade_path_test.go:34,170`.
- `semstreams.graph.alert.v1`: none.

The removals of §2 reach no consumer: no consumer imports `pkg/dispatch` or `pkg/worker`, and none reads
`readiness.Set` (§2 a).

## 6. Intent check (boundary moved by this change)

This change ports D2 row 2. If the owner accepts question E, `pkg/worker` leaves the port set; it is a helper, not a
capability. Capabilities named in `AGENTS.md` "What this is for": ingest — admitted (this change; #8 Q4 tier-0 set);
index, query — admitted (change 4); vocabulary — admitted (change 1); provenance — admitted (`pkg/projection`,
`graph`; this change); fusion — admitted (change 5); tier ladder — admitted (#8 tier model, 5932313950); workflows that
survive restarts, replay, settlement, retries — admitted (`pkg/lifecycle` Q16, settlement Q18 here; the general
primitive #24 deferred to after #9's first green extraction, ruled on #8); rules — admitted (change 6, Q15). No
deferral without a ruling.

## 7. Guidance at the pin that returns with these packages

SemStreams developer contract `.agents/contracts/semstreams-developer.md`: "Semantic identity and graph contracts"
(`:109-126`), "Payload registry" (`:234-242`), "State ownership and component wiring" (`:243-252`). Reviewer contract:
"Semantic identity and graph review" (`:155-169`), "Payload registry" (`:227-235`), "Graph and state ownership"
(`:236-245`), "Component and schema wiring" (`:246-253`). Skills: `entity-or-bucket` (136 lines), `kv-or-stream` (94),
`new-payload` (264; its "Payload registry" sections were not carried with `payloadregistry` in change 1; SS PR #1437
adds 2 lines to it), `query-pattern` (93; graph-ingest serves the four `graph.ingest.query.*` handlers).
`orchestration-check` belongs to the rule core (change 6). The base contracts have none of these sections
(`grep -n '^## \|^### ' .agents/contracts/semengine-*.md`).

## Open evidence questions (for the reviewer)

1. Round 1's three questions: (1) answered in §2 (b) for `graph/inference.Config` and `pkg/dispatch`; the README
   reads stay with the port tasks. (2) answered by the type-based method of §2. (3) stands: P-5 ran with a vacuous
   `Observe`; the in-package adapter is the real check.
2. P-8 is an upper bound computed at the pin; the port re-measures (task 5.2).

## 8. Round 3: the owner's rulings and the probes they required

Rulings (2026-10-07): **E** and **F** in #91 comment 6035429806, **B** and **C** in #91 comment 6035477895. Ruling B
asks whether `model/wire` admits the same cut as `graph/llm`. All pin reads below are on the pin tarball named in
the header (local only; the commands are quoted so they can be re-run on any copy of the pin).

**P-9 what the closure reads from `graph/llm` and `model/wire`.**

- Edges into the two packages inside closure(graph-ingest): `go list -deps -f '{{.ImportPath}} {{join .Imports " "}}'
  ./processor/graph-ingest`, filtered to imports ending `model/wire` or `graph/llm`: two lines,
  `graph/inference -> graph/llm` and `graph/llm -> model/wire`. `go-openai`'s importers in the same listing:
  `graph/llm` only.
- What `graph/inference` reads from `graph/llm` (`grep -on 'llm\.[A-Za-z]*'` over its files): non-test
  `graph/inference/review_worker.go:40` — `llmClient llm.Client          // optional - nil if LLM disabled`,
  `:69` — `LLMClient     llm.Client // optional`, `:430` — `response, err := w.llmClient.ChatCompletion(ctx,
  llm.ChatRequest{`, and `graph/inference/config.go:123` — ``LLM llm.Config `json:"llm"` ``; doc comment
  `graph/inference/doc.go:51`, the example line `cfg.Review.LLM = llm.Config{`; tests
  `graph/inference/review_worker_test.go:30,42` (`llm.Client`, `llm.ChatRequest`, `llm.ChatResponse`).
- `graph/llm/client.go` (65 lines) declares exactly `Client` (`:19` — `type Client interface {`), `ChatRequest`
  (`:31`) and `ChatResponse` (`:47`), and imports only `context` (`:14`). Its package comment (`:1-10`) speaks of
  community summarization, search answer generation and the OpenAI SDK.
- `model/wire`'s importers in the whole pin (`grep -rln '"github.com/c360studio/semstreams/model/wire'
  --include='*.go'`), non-test: `graph/llm/openai_client.go`, `model/wire/responses/errors.go`, and 12 files of
  `processor/agentic-model` (cut, #8 Q4). With `openai_client.go` not ported, no package this change ports imports
  `model/wire`: it admits the cut, to nothing.
- `graph/llm`'s importers in the pin outside `graph/inference` (same search for `graph/llm`): `graph/clustering`,
  `graph/query`, `processor/graph-clustering`, `processor/graph-query`, `processor/agentic-loop` and the four
  `processor/research-graph-*` packages, which read `EntityParts`, `NewOpenAIClient`, `OpenAIConfigFromEndpoint`,
  prompts and summarizers as well as `Client`, `ChatRequest` and `ChatResponse`. None is ported in change 2.

**`ReviewConfig.LLM` has no reader at the pin.** `grep -rnE '\.LLM\b' --include='*.go' .` over the pin, non-test
files: one hit, `graph/inference/doc.go:51` (a doc-comment example). graph-clustering's review worker gets its client
from `processor/graph-clustering/component.go:2476` — `func (c *Component) resolveReviewLLMClient() llm.Client {`
(the model registry's `anomaly_review` capability, falling back to the community-summary client) and passes it as
`LLMClient: reviewClient` (`:2439`); it reads `c.config.AnomalyConfig.Review.Enabled` (`:1081`), `.Workers`, `.AutoApproveThreshold` and
`.AutoRejectThreshold` (`:2456-2460`), never `.Review.LLM`. **Correction to §2 (b):** the by-name search counted
`LLM` as read because `review_worker.go` spells the word in log text (`:389` — `w.logger.Warn("LLM review failed,
falling back",`). `graph/inference.Config` has 43 JSON fields (`sed -n 17,180p graph/inference/config.go | grep -c
'json:"'` → 43): 37 read, 6 unread.

**`internal/componentadmission`'s readers.** `grep -rn componentadmission --include='*.go'` over the pin, non-test:
`component/registry.go:194,475,825,843` (the parameters of `CreateComponent`, `SealComposition`, `Snapshot`,
`Snapshots`), `service/component_manager.go:290,384,397,1108` and `service/message_logger.go:347` (change 3). Tests
that build the token: `component/registry_boot_admission_test.go` 12 lines, `component/registry_integration_test.go`
4, `component/registry_test.go` 3; outside the set, `componentregistry/register_integration_test.go` 2,
`internal/portgrammarcontrol/target_test.go` 1, and five `service` test files. Consumers: semboids calls
`registry.CreateComponent` at 9 sites in 7 integration-test files (`internal/zone/ingest_integration_test.go:52` —
`inst, err := registry.CreateComponent("graph-ingest-test", types.ComponentConfig{`), on SemStreams
`v1.0.0-beta.160` (semboids `go.mod:6`), with three arguments; semsource, semconnect and semteams call none of
`CreateComponent`, `SealComposition` or `Snapshots` (search of the three tarballs: 0 lines). The pin's
`CreateComponent` accepts a nil `prepare` (`component/registry.go:222` — `if prepare != nil {`).

**`pkg/worker`'s one remaining read.** `pkg/dispatch/errors.go:24` — `ErrStopped = worker.ErrPoolStopped`, whose
text is `pkg/worker/errors.go:11` — `ErrPoolStopped = errors.New("worker pool stopped")`. `KeyedPool` returns it
(`pkg/dispatch/keyed_pool.go:227`); its tests assert it with `errors.Is` (`keyed_pool_test.go:353-354`), so a
declaration in `internal/dispatch` keeps them meaningful.

**Counts after the rulings.** 15 whole packages (§0's 17 less `pkg/worker` and `internal/componentadmission`) and
`graph/llm/client.go`: non-test lines 30,613 − 707 − 8 + 65 = 29,963; test files 154 − 2 = 152, lines 40,768 − 566
= 40,202 (`graph/llm`'s two test files test the OpenAI client and are not ported; `internal/componentadmission` has
none). Sleeps in tests: P-4's 28 less `pkg/worker`'s 9 = 19. Coverage: unchanged for the nine targets; `graph/llm`
after the cut has no statements.

**Intent check (§6), re-run for the moved boundary.** The rulings move four package boundaries (`pkg/worker`,
`internal/componentadmission`, most of `graph/llm`, `model/wire`). None is a capability `AGENTS.md` "What this is for"
names: the first two are helpers, and the LLM client and model wire format are providers, which tier 0 does without.
Each move has its ruling (B, C, E above; `model/wire` follows B's reasoning and is recorded as such in the design).

**Adopter seam (§5), one item added by ruling C.** (f) A consumer must know to create components through the component
manager, not by calling `Registry.CreateComponent`, `SealComposition` or `Snapshots`. If it does not, a component
exists that the manager does not track. It finds out from the methods' doc comments only. The owner accepted that
cost in ruling C ("a consumer creating a component outside the manager is review only").

**#77 was ruled after round 2** (#77 comment 6035317931; relayed on PR #93 in comments 6035318863 and 6035358556):
the component cleans up its own failed start with a public helper whose home and name this change settles; the #38
exception is granted for ported components; PR #93 closes #77 (#77 comment 6035358884). Round 3 records it and does
not apply it (design task 1.9).

## Appendix: dead-surface candidates (186), by type, pinned at the pin

Dispositions: **drop**; **keep K1** — `pkg/lifecycle` and `pkg/projection` are the durable-execution primitive (#8
Q16; foundation (a)) and #24 decides their surface; **keep K2** — `model` is carried whole until #32 decides the seam
(change 7; Q4); **keep K3** — the per-package `RegisterPayloads` (#33); **keep K4** — required by
`component.Discoverable`, an interface of the component contract that consumer components implement. Readers are
non-test/test counts.

| Identifier | Pin | Kind | Readers | Disposition |
| --- | --- | --- | --- | --- |
| `ValidateComponentConfig` | `component/config_validator.go:91` — `func ValidateComponentConfig(` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `ValidateAndPersistComponentConfig` | `component/config_validator.go:123` — `func ValidateAndPersistComponentConfig(` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `IsLifecycleComponent` | `component/lifecycle.go:94` — `func IsLifecycleComponent(comp Discoverable) bool {` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `LogLevelDebug` | `component/logging.go:8` — `LogLevelDebug LogLevel = "DEBUG"` | const | own 0/1, admitted 0/0, other 0/0 | drop |
| `LogLevelInfo` | `component/logging.go:10` — `LogLevelInfo LogLevel = "INFO"` | const | own 0/3, admitted 0/0, other 0/0 | drop |
| `LogLevelWarn` | `component/logging.go:12` — `LogLevelWarn LogLevel = "WARN"` | const | own 0/1, admitted 0/0, other 0/0 | drop |
| `LogLevelError` | `component/logging.go:14` — `LogLevelError LogLevel = "ERROR"` | const | own 0/1, admitted 0/0, other 0/0 | drop |
| `LogEntry` | `component/logging.go:19` — `type LogEntry struct {` | type | own 0/3, admitted 0/0, other 0/0 | drop |
| `NewProcessorMetrics` | `component/metrics.go:31` — `func NewProcessorMetrics(registry *metric.MetricsRegistry, subsystem st…` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `ProcessorMetrics.RecordEvent` | `component/metrics.go:85` — `func (m *ProcessorMetrics) RecordEvent(operation string) {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `ProcessorMetrics.RecordError` | `component/metrics.go:90` — `func (m *ProcessorMetrics) RecordError(errorType string) {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `ProcessorMetrics.RecordKVOperation` | `component/metrics.go:95` — `func (m *ProcessorMetrics) RecordKVOperation(operation string) {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `ProcessorMetrics.ObserveDuration` | `component/metrics.go:100` — `func (m *ProcessorMetrics) ObserveDuration(seconds float64) {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `PortFacts.KVReadBucket` | `component/port_facts.go:105` — `func (f PortFacts) KVReadBucket() (string, bool) {` | method | own 0/0, admitted 0/0, other 1/1 | drop |
| `StreamFacts.ConsumerName` | `component/port_facts.go:134` — `func (f StreamFacts) ConsumerName() string { return f.consumerName }` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `MergePortConfig` | `component/ports.go:184` — `func MergePortConfig(defaults, overrides PortConfig) (PortConfig, error…` | func | own 0/3, admitted 0/0, other 6/1 | drop |
| `Registerable` | `component/registerable.go:4` — `type Registerable interface {` | type | own 0/0, admitted 0/0, other 0/0 | drop |
| `declarationSnapshot.Factory` | `component/registry.go:115` — `func (s declarationSnapshot) Factory() string { return s.record.Factory…` | method | own 0/2, admitted 0/7, other 0/0 | drop |
| `Registry.ListComponentTypes` | `component/registry.go:502` — `func (r *Registry) ListComponentTypes() []string {` | method | own 0/0, admitted 0/0, other 0/3 | drop |
| `Registry.GetFactory` | `component/registry.go:592` — `func (r *Registry) GetFactory(name string) (Factory, bool) {` | method | own 0/0, admitted 0/0, other 0/5 | drop |
| `Registry.ListAvailable` | `component/registry.go:637` — `func (r *Registry) ListAvailable() map[string]Info {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `ValidateJSONSize` | `component/registry.go:685` — `func ValidateJSONSize(data json.RawMessage) error {` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `Registry.Snapshot` | `component/registry.go:824` — `func (r *Registry) Snapshot(` | method | own 0/3, admitted 0/0, other 0/1 | drop |
| `GetString` | `component/registry.go:918` — `func GetString(config map[string]any, key string, defaultValue string) …` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `GetInt` | `component/registry.go:946` — `func GetInt(config map[string]any, key string, defaultValue int) int {` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `GetBool` | `component/registry.go:988` — `func GetBool(config map[string]any, key string, defaultValue bool) bool…` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `GetFloat64` | `component/registry.go:1003` — `func GetFloat64(config map[string]any, key string, defaultValue float64…` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `GetPropertyValue` | `component/schema.go:274` — `func GetPropertyValue(config map[string]any, key string) (any, bool) {` | func | own 0/2, admitted 0/0, other 0/0 | drop |
| `GetProperties` | `component/schema.go:311` — `func GetProperties(schema ConfigSchema, category string) map[string]Pro…` | func | own 0/4, admitted 0/0, other 0/0 | drop |
| `IsComplexType` | `component/schema.go:353` — `func IsComplexType(propType string) bool {` | func | own 0/1, admitted 0/0, other 0/0 | drop |
| `SortedPropertyNames` | `component/schema.go:380` — `func SortedPropertyNames(schema ConfigSchema) []string {` | func | own 0/2, admitted 0/0, other 0/0 | drop |
| `PortFieldInfo.ZeroIsOmitted` | `component/schema_tags.go:106` — `func (p PortFieldInfo) ZeroIsOmitted() bool { return p.zeroIsOmitted }` | method | own 0/1, admitted 0/0, other 1/1 | drop |
| `SimpleMockComponent.ConfigSchema` | `component/test_helpers.go:57` — `func (m *SimpleMockComponent) ConfigSchema() ConfigSchema {` | method | own 0/0, admitted 0/0, other 0/0; via component.Discoverable(adm=0, all=5); consumer name match was another type | keep K4 |
| `ValidateNetworkConfig` | `component/validation.go:207` — `func ValidateNetworkConfig(port int, bindAddr string) error {` | func | own 0/0, admitted 0/0, other 1/0 | drop |
| `ErrEntityNotFound` | `graph/errors.go:13` — `ErrEntityNotFound = errors.New("entity not found")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrEntityExists` | `graph/errors.go:16` — `ErrEntityExists = errors.New("entity already exists")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrInvalidEntityID` | `graph/errors.go:19` — `ErrInvalidEntityID = errors.New("invalid entity ID")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrInvalidEntityData` | `graph/errors.go:22` — `ErrInvalidEntityData = errors.New("invalid entity data")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrVersionConflict` | `graph/errors.go:25` — `ErrVersionConflict = errors.New("entity version conflict")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrIndexNotFound` | `graph/errors.go:31` — `ErrIndexNotFound = errors.New("index not found")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrIndexCorrupted` | `graph/errors.go:34` — `ErrIndexCorrupted = errors.New("index corrupted")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrIndexUpdateFailed` | `graph/errors.go:37` — `ErrIndexUpdateFailed = errors.New("index update failed")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrInvalidIndexKey` | `graph/errors.go:40` — `ErrInvalidIndexKey = errors.New("invalid index key")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrQueryTimeout` | `graph/errors.go:46` — `ErrQueryTimeout = errors.New("query timeout")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrQueryTooComplex` | `graph/errors.go:49` — `ErrQueryTooComplex = errors.New("query too complex")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrQueryDepthExceeded` | `graph/errors.go:52` — `ErrQueryDepthExceeded = errors.New("query depth exceeded")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrInvalidQueryParams` | `graph/errors.go:55` — `ErrInvalidQueryParams = errors.New("invalid query parameters")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrAliasNotFound` | `graph/errors.go:61` — `ErrAliasNotFound = errors.New("alias not found")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrAliasExists` | `graph/errors.go:64` — `ErrAliasExists = errors.New("alias already exists")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrInvalidAlias` | `graph/errors.go:67` — `ErrInvalidAlias = errors.New("invalid alias")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrBufferFull` | `graph/errors.go:73` — `ErrBufferFull = errors.New("buffer full")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrBatchTooBig` | `graph/errors.go:76` — `ErrBatchTooBig = errors.New("batch too big")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrFlushFailed` | `graph/errors.go:79` — `ErrFlushFailed = errors.New("flush failed")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrNotStarted` | `graph/errors.go:85` — `ErrNotStarted = errors.New("service not started")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrAlreadyStarted` | `graph/errors.go:88` — `ErrAlreadyStarted = errors.New("service already started")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `ErrShuttingDown` | `graph/errors.go:91` — `ErrShuttingDown = errors.New("service shutting down")` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `Event.Payload` | `graph/events.go:137` — `func (e *Event) Payload() map[string]any {` | method | own 0/1, admitted 0/0, other 0/0; consumer name match was another type | drop |
| `NewRelationshipCreateEvent` | `graph/events.go:158` — `func NewRelationshipCreateEvent(` | func | own 0/4, admitted 0/0, other 0/0 | drop |
| `NewAlertEvent` | `graph/events.go:175` — `func NewAlertEvent(` | func | own 0/10, admitted 0/0, other 0/5 | drop |
| `NewEntityCreateEvent` | `graph/events.go:204` — `func NewEntityCreateEvent(` | func | own 0/2, admitted 0/0, other 0/0 | drop |
| `NewEntityDeleteEvent` | `graph/events.go:217` — `func NewEntityDeleteEvent(entityID, reason string, metadata EventMetada…` | func | own 0/1, admitted 0/0, other 0/0 | drop |
| `NewRelationshipDeleteEvent` | `graph/events.go:223` — `func NewRelationshipDeleteEvent(` | func | own 0/2, admitted 0/0, other 0/0 | drop |
| `GetPropertyValueTyped` | `graph/helpers.go:28` — `func GetPropertyValueTyped[T any](entity *EntityState, predicate string…` | func | own 0/5, admitted 0/0, other 0/0 | drop |
| `GetProperties` | `graph/helpers.go:46` — `func GetProperties(entity *EntityState) map[string]any {` | func | own 0/3, admitted 0/0, other 0/0 | drop |
| `GetRelationshipTriples` | `graph/helpers.go:62` — `func GetRelationshipTriples(entity *EntityState) []message.Triple {` | func | own 0/2, admitted 0/0, other 0/0 | drop |
| `GetPropertyTriples` | `graph/helpers.go:78` — `func GetPropertyTriples(entity *EntityState) []message.Triple {` | func | own 0/2, admitted 0/0, other 0/0 | drop |
| `HasProperty` | `graph/helpers.go:93` — `func HasProperty(entity *EntityState, predicate string) bool {` | func | own 0/3, admitted 0/0, other 0/0 | drop |
| `IncomingEdges.AddIncomingEdge` | `graph/incoming.go:22` — `func (ie *IncomingEdges) AddIncomingEdge(edge IncomingEdge) {` | method | own 0/4, admitted 0/0, other 0/0 | drop |
| `IncomingEdges.RemoveIncomingEdge` | `graph/incoming.go:37` — `func (ie *IncomingEdges) RemoveIncomingEdge(fromEntityID, edgeType stri…` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `IncomingEdges.GetIncomingEdgesByType` | `graph/incoming.go:49` — `func (ie *IncomingEdges) GetIncomingEdgesByType(edgeType string) []Inco…` | method | own 0/3, admitted 0/0, other 0/0 | drop |
| `IncomingEdges.GetIncomingEntityIDs` | `graph/incoming.go:60` — `func (ie *IncomingEdges) GetIncomingEntityIDs() []string {` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `IncomingEdges.Count` | `graph/incoming.go:75` — `func (ie *IncomingEdges) Count() int {` | method | own 0/4, admitted 0/0, other 0/0; consumer name match was another type | drop |
| `IncomingEdges.HasIncomingFrom` | `graph/incoming.go:80` — `func (ie *IncomingEdges) HasIncomingFrom(fromEntityID string) bool {` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `IncomingEdges.HasIncomingOfType` | `graph/incoming.go:90` — `func (ie *IncomingEdges) HasIncomingOfType(edgeType string) bool {` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `FrameworkOwnedBuckets` | `graph/kvcatalog.go:219` — `func FrameworkOwnedBuckets() []string {` | func | own 0/5, admitted 0/0, other 0/0 | drop |
| `MissingUnknown` | `graph/query_batch_types.go:39` — `MissingUnknown MissingReason = "unknown"` | const | own 0/0, admitted 0/1, other 0/0 | drop |
| `ContextEntry` | `graph/query_index_types.go:19` — `type ContextEntry struct {` | type | own 0/0, admitted 0/0, other 0/0 | drop |
| `NameIndexEntry` | `graph/query_name_types.go:34` — `type NameIndexEntry struct {` | type | own 0/0, admitted 0/0, other 0/0 | drop |
| `PredicateQueryResponse` | `graph/query_response_types.go:16` — `type PredicateQueryResponse = QueryResponse[PredicateData]` | type | own 0/0, admitted 0/7, other 0/0 | drop |
| `PredicateListQueryResponse` | `graph/query_response_types.go:19` — `type PredicateListQueryResponse = QueryResponse[PredicateListData]` | type | own 0/0, admitted 0/6, other 1/0 | drop |
| `PredicateStatsQueryResponse` | `graph/query_response_types.go:22` — `type PredicateStatsQueryResponse = QueryResponse[PredicateStatsData]` | type | own 0/0, admitted 0/2, other 0/0 | drop |
| `CompoundPredicateQueryResponse` | `graph/query_response_types.go:25` — `type CompoundPredicateQueryResponse = QueryResponse[CompoundPredicateDa…` | type | own 0/0, admitted 0/3, other 0/0 | drop |
| `EntityCriteria` | `graph/query_types.go:31` — `type EntityCriteria struct {` | type | own 0/0, admitted 0/0, other 0/0 | drop |
| `RelationshipCriteria` | `graph/query_types.go:38` — `type RelationshipCriteria struct {` | type | own 0/0, admitted 0/0, other 0/0 | drop |
| `QueryResult` | `graph/query_types.go:47` — `type QueryResult struct {` | type | own 0/0, admitted 0/0, other 0/0 | drop |
| `NewNATSRelationshipApplier` | `graph/inference/applier.go:35` — `func NewNATSRelationshipApplier(` | func | own 0/0, admitted 0/0, other 1/0 | drop |
| `NewNoOpApplier` | `graph/inference/applier.go:117` — `func NewNoOpApplier(logger *slog.Logger) *NoOpApplier {` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `NewDirectRelationshipApplier` | `graph/inference/applier.go:152` — `func NewDirectRelationshipApplier(adder TripleAdder, logger *slog.Logge…` | func | own 0/7, admitted 0/0, other 0/0 | drop |
| `RegisterPayloads` | `graph/inference/container_entity.go:62` — `func RegisterPayloads(reg *payloadregistry.Registry) error {` | func | own 0/0, admitted 0/0, other 1/1 | keep K3 |
| `Orchestrator.UpdateConfig` | `graph/inference/detector.go:329` — `func (o *Orchestrator) UpdateConfig(config Config) error {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `Orchestrator.GetRegisteredDetectors` | `graph/inference/detector.go:361` — `func (o *Orchestrator) GetRegisteredDetectors() []string {` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `Result.Duration` | `graph/inference/detector.go:529` — `func (r *Result) Duration() time.Duration {` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `Result.AnomalyCount` | `graph/inference/detector.go:534` — `func (r *Result) AnomalyCount() int {` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `Result.CountByType` | `graph/inference/detector.go:539` — `func (r *Result) CountByType() map[AnomalyType]int {` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `DefaultHierarchyConfig` | `graph/inference/hierarchy.go:122` — `func DefaultHierarchyConfig() HierarchyConfig {` | func | own 0/1, admitted 0/0, other 0/0 | drop |
| `HierarchyInference.OnEntityCreated` | `graph/inference/hierarchy.go:290` — `func (h *HierarchyInference) OnEntityCreated(ctx context.Context, entit…` | method | own 0/27, admitted 0/0, other 0/0 | drop |
| `HierarchyInference.ClearCache` | `graph/inference/hierarchy.go:523` — `func (h *HierarchyInference) ClearCache() {` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `HierarchyInference.GetMetrics` | `graph/inference/hierarchy.go:530` — `func (h *HierarchyInference) GetMetrics() (containersCreated, edgesCrea…` | method | own 0/3, admitted 0/0, other 0/0 | drop |
| `HierarchyInference.GetCacheStats` | `graph/inference/hierarchy.go:535` — `func (h *HierarchyInference) GetCacheStats() int {` | method | own 0/5, admitted 0/0, other 0/0 | drop |
| `NewHTTPHandler` | `graph/inference/http_handlers.go:21` — `func NewHTTPHandler(storage Storage, applier RelationshipApplier, logge…` | func | own 0/12, admitted 0/0, other 1/2 | drop |
| `HTTPHandler.RegisterHTTPHandlers` | `graph/inference/http_handlers.go:33` — `func (h *HTTPHandler) RegisterHTTPHandlers(prefix string, mux *http.Ser…` | method | own 0/1, admitted 0/0, other 1/0 | drop |
| `NewReviewMetrics` | `graph/inference/metrics.go:26` — `func NewReviewMetrics(component string, registry *metric.MetricsRegistr…` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `ReviewMetrics.SetPendingCount` | `graph/inference/metrics.go:149` — `func (m *ReviewMetrics) SetPendingCount(count int) {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `ReviewMetrics.IncWorkersActive` | `graph/inference/metrics.go:157` — `func (m *ReviewMetrics) IncWorkersActive() {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `ReviewMetrics.DecWorkersActive` | `graph/inference/metrics.go:165` — `func (m *ReviewMetrics) DecWorkersActive() {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `ReviewWorker.Pause` | `graph/inference/review_worker.go:197` — `func (w *ReviewWorker) Pause() {` | method | own 0/3, admitted 0/0, other 0/0 | drop |
| `ReviewWorker.Resume` | `graph/inference/review_worker.go:215` — `func (w *ReviewWorker) Resume() {` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `NATSAnomalyStorage.GetByType` | `graph/inference/storage.go:288` — `func (s *NATSAnomalyStorage) GetByType(ctx context.Context, anomalyType…` | method | own 0/2, admitted 0/1, other 0/0 | drop |
| `NATSAnomalyStorage.UpdateStatus` | `graph/inference/storage.go:342` — `func (s *NATSAnomalyStorage) UpdateStatus(` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `NATSAnomalyStorage.Watch` | `graph/inference/storage.go:417` — `func (s *NATSAnomalyStorage) Watch(ctx context.Context) (<-chan *Struct…` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `NATSAnomalyStorage.Cleanup` | `graph/inference/storage.go:481` — `func (s *NATSAnomalyStorage) Cleanup(ctx context.Context, retention tim…` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `StructuralAnomaly.NeedsHumanReview` | `graph/inference/types.go:174` — `func (a *StructuralAnomaly) NeedsHumanReview() bool {` | method | own 0/1, admitted 0/0, other 0/0 | drop |
| `Gauges.MetricNames` | `graph/readiness/gauges.go:131` — `func (g *Gauges) MetricNames() []string {` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `NewSet` | `graph/readiness/set.go:41` — `func NewSet(src BucketSource, keys []string, opts ...Option) *Set {` | func | own 0/3, admitted 0/0, other 2/0 | drop |
| `Set.Start` | `graph/readiness/set.go:65` — `func (s *Set) Start(ctx context.Context) error {` | method | own 0/1, admitted 0/0, other 2/0 | drop |
| `Set.Stop` | `graph/readiness/set.go:85` — `func (s *Set) Stop() {` | method | own 0/1, admitted 0/0, other 3/0; consumer name match was another type | drop |
| `Set.FullyCovered` | `graph/readiness/set.go:153` — `func (s *Set) FullyCovered() Verdict {` | method | own 0/7, admitted 0/0, other 1/0 | drop |
| `Set.Dumps` | `graph/readiness/set.go:203` — `func (s *Set) Dumps() []Dump {` | method | own 0/4, admitted 0/0, other 2/0 | drop |
| `KCoreComputer.ComputeIncremental` | `graph/structural/kcore.go:161` — `func (c *KCoreComputer) ComputeIncremental(ctx context.Context, _ []str…` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `PivotComputer.ComputeIncremental` | `graph/structural/pivot.go:319` — `func (c *PivotComputer) ComputeIncremental(ctx context.Context, _ []str…` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `KCoreIndex.FilterByMinCore` | `graph/structural/types.go:73` — `func (idx *KCoreIndex) FilterByMinCore(entityIDs []string, minCore int)…` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `KCoreIndex.GetEntitiesInCore` | `graph/structural/types.go:91` — `func (idx *KCoreIndex) GetEntitiesInCore(core int) []string {` | method | own 0/5, admitted 0/0, other 0/0 | drop |
| `PivotIndex.IsWithinHops` | `graph/structural/types.go:204` — `func (idx *PivotIndex) IsWithinHops(entityA, entityB string, maxHops in…` | method | own 0/4, admitted 0/0, other 0/0 | drop |
| `PivotIndex.GetReachableCandidates` | `graph/structural/types.go:213` — `func (idx *PivotIndex) GetReachableCandidates(source string, maxHops in…` | method | own 0/2, admitted 0/0, other 0/0 | drop |
| `IsCommitUnknown` | `internal/graphmutation/client.go:57` — `func IsCommitUnknown(err error) bool {` | func | own 0/3, admitted 0/0, other 0/0 | drop |
| `NewRollingWindowBreaker` | `model/breaker.go:108` — `func NewRollingWindowBreaker(cfg BreakerConfig) *RollingWindowBreaker {` | func | own 0/7, admitted 0/0, other 1/5 | keep K2 |
| `RollingWindowBreaker.IsHealthy` | `model/breaker.go:117` — `func (b *RollingWindowBreaker) IsHealthy(endpoint string) bool {` | method | own 0/6, admitted 0/0, other 0/1; via model.HealthPolicy(adm=0, all=6) | keep K2 |
| `RollingWindowBreaker.EndpointStats` | `model/breaker.go:153` — `func (b *RollingWindowBreaker) EndpointStats(endpoint string) HealthSta…` | method | own 0/6, admitted 0/0, other 0/4 | keep K2 |
| `RollingWindowBreaker.RecordResult` | `model/breaker.go:178` — `func (b *RollingWindowBreaker) RecordResult(endpoint string, result Res…` | method | own 0/12, admitted 0/0, other 0/0; via model.HealthPolicy(adm=0, all=3) | keep K2 |
| `ErrorKindNone` | `model/health.go:50` — `ErrorKindNone ErrorKind = ""` | const | own 0/0, admitted 0/0, other 1/0 | keep K2 |
| `ErrorKindTimeout` | `model/health.go:52` — `ErrorKindTimeout ErrorKind = "timeout"` | const | own 0/0, admitted 0/0, other 1/1 | keep K2 |
| `ErrorKindRateLimit` | `model/health.go:55` — `ErrorKindRateLimit ErrorKind = "rate_limit"` | const | own 0/0, admitted 0/0, other 1/2 | keep K2 |
| `ErrorKindServerError` | `model/health.go:58` — `ErrorKindServerError ErrorKind = "server_error"` | const | own 0/1, admitted 0/0, other 1/3 | keep K2 |
| `ErrorKindNetwork` | `model/health.go:62` — `ErrorKindNetwork ErrorKind = "network"` | const | own 0/0, admitted 0/0, other 1/2 | keep K2 |
| `ErrorKindUnknown` | `model/health.go:65` — `ErrorKindUnknown ErrorKind = "unknown"` | const | own 0/0, admitted 0/0, other 1/1 | keep K2 |
| `alwaysHealthyPolicy.IsHealthy` | `model/health.go:154` — `func (alwaysHealthyPolicy) IsHealthy(string) bool                { retu…` | method | own 0/0, admitted 0/0, other 0/0; via model.HealthPolicy(adm=0, all=6) | keep K2 |
| `alwaysHealthyPolicy.EndpointStatus` | `model/health.go:155` — `func (alwaysHealthyPolicy) EndpointStatus(string) EndpointStatus { retu…` | method | own 0/0, admitted 0/0, other 0/0; via model.HealthPolicy(adm=0, all=4) | keep K2 |
| `alwaysHealthyPolicy.EndpointStats` | `model/health.go:156` — `func (alwaysHealthyPolicy) EndpointStats(string) HealthStats {` | method | own 0/0, admitted 0/0, other 0/0 | keep K2 |
| `alwaysHealthyPolicy.RecordResult` | `model/health.go:159` — `func (alwaysHealthyPolicy) RecordResult(string, Result) {}` | method | own 0/0, admitted 0/0, other 0/0; via model.HealthPolicy(adm=0, all=3) | keep K2 |
| `ComposeHealth` | `model/health.go:188` — `func ComposeHealth(r RegistryReader, p HealthPolicy) HealthAwareRegistr…` | func | own 0/3, admitted 0/0, other 0/0 | keep K2 |
| `HTTPClientOptionsFromEndpoint` | `model/httpclient.go:157` — `func HTTPClientOptionsFromEndpoint(ep *EndpointConfig) HTTPClientOption…` | func | own 0/3, admitted 0/0, other 1/0 | keep K2 |
| `CapabilityResearchRouting` | `model/registry.go:45` — `CapabilityResearchRouting = "research_routing"` | const | own 0/1, admitted 0/0, other 5/2 | keep K2 |
| `CapabilityResearchAssessment` | `model/registry.go:51` — `CapabilityResearchAssessment = "research_assessment"` | const | own 0/1, admitted 0/0, other 5/1 | keep K2 |
| `CapabilityResearchSynthesis` | `model/registry.go:58` — `CapabilityResearchSynthesis = "research_synthesis"` | const | own 0/1, admitted 0/0, other 5/1 | keep K2 |
| `Registry.GetFallbackChain` | `model/registry.go:625` — `func (r *Registry) GetFallbackChain(capability string) []string {` | method | own 0/1, admitted 0/0, other 0/3; via model.RegistryReader(adm=0, all=2) | keep K2 |
| `Registry.GetMaxTokens` | `model/registry.go:656` — `func (r *Registry) GetMaxTokens(name string) int {` | method | own 0/2, admitted 0/0, other 0/0; via model.RegistryReader(adm=0, all=1) | keep K2 |
| `Registry.GetDefault` | `model/registry.go:665` — `func (r *Registry) GetDefault() string {` | method | own 0/2, admitted 0/1, other 1/1; via model.RegistryReader(adm=0, all=4) | keep K2 |
| `Registry.ListCapabilities` | `model/registry.go:670` — `func (r *Registry) ListCapabilities() []string {` | method | own 0/1, admitted 0/0, other 0/0 | keep K2 |
| `Registry.ListEndpoints` | `model/registry.go:680` — `func (r *Registry) ListEndpoints() []string {` | method | own 0/1, admitted 0/0, other 0/0; via model.RegistryReader(adm=0, all=1) | keep K2 |
| `Registry.ResolveSummarization` | `model/registry.go:690` — `func (r *Registry) ResolveSummarization() string {` | method | own 0/1, admitted 0/0, other 0/0; via model.RegistryReader(adm=0, all=1) | keep K2 |
| `Watch` | `model/watch.go:35` — `func Watch(ctx context.Context, watcher Watcher, apply func(*Registry))…` | func | own 0/7, admitted 0/0, other 0/0 | keep K2 |
| `New` | `pkg/dispatch/dispatcher.go:95` — `func New[W any](ctx context.Context, cfg Config[W], deps Deps) (*Bounde…` | func | own 0/19, admitted 0/0, other 1/1 | drop |
| `BoundedDispatcher.Submit` | `pkg/dispatch/dispatcher.go:165` — `func (d *BoundedDispatcher[W]) Submit(work W) error {` | method | own 0/11, admitted 0/0, other 1/2 | drop |
| `BoundedDispatcher.Stop` | `pkg/dispatch/dispatcher.go:196` — `func (d *BoundedDispatcher[W]) Stop(ctx context.Context) error {` | method | own 0/16, admitted 0/0, other 2/0 | drop |
| `BoundedDispatcher.Stats` | `pkg/dispatch/dispatcher.go:244` — `func (d *BoundedDispatcher[W]) Stats() worker.PoolStats {` | method | own 0/2, admitted 0/0, other 0/2 | drop |
| `ErrQueueFull` | `pkg/dispatch/errors.go:20` — `ErrQueueFull = worker.ErrQueueFull` | var | own 0/0, admitted 0/0, other 0/0 | drop |
| `KeyedPool.Submit` | `pkg/dispatch/keyed_pool.go:223` — `func (p *KeyedPool[W]) Submit(work W) error {` | method | own 0/14, admitted 0/1, other 0/0 | drop |
| `KeyedPool.Stats` | `pkg/dispatch/keyed_pool.go:419` — `func (p *KeyedPool[W]) Stats() KeyedStats {` | method | own 0/3, admitted 0/0, other 0/0 | drop |
| `RegisterPayloads` | `pkg/lifecycle/harness_entity.go:65` — `func RegisterPayloads(reg *payloadregistry.Registry) error {` | func | own 0/1, admitted 0/0, other 1/1 | keep K1 |
| `Manager.GetWithRevision` | `pkg/lifecycle/manager.go:230` — `func (m *Manager) GetWithRevision(ctx context.Context, workflow, entity…` | method | own 0/0, admitted 0/0, other 0/0 | keep K1 |
| `Manager.GetRaw` | `pkg/lifecycle/manager.go:279` — `func (m *Manager) GetRaw(ctx context.Context, entityID string) (*graph.…` | method | own 0/1, admitted 0/0, other 0/0 | keep K1 |
| `Manager.UpdateFromOperator` | `pkg/lifecycle/manager.go:711` — `func (m *Manager) UpdateFromOperator(ctx context.Context, workflow, ent…` | method | own 0/3, admitted 0/0, other 0/0; via lifecycle-gateway.LifecycleManager(adm=0, all=1) | keep K1 |
| `Manager.Despawn` | `pkg/lifecycle/manager.go:858` — `func (m *Manager) Despawn(ctx context.Context, workflow, entityID strin…` | method | own 0/7, admitted 0/0, other 0/0 | keep K1 |
| `Manager.DespawnWith` | `pkg/lifecycle/manager.go:899` — `func (m *Manager) DespawnWith(ctx context.Context, workflow, entityID s…` | method | own 0/5, admitted 0/0, other 0/0 | keep K1 |
| `Manager.CreateFromOperator` | `pkg/lifecycle/manager.go:997` — `func (m *Manager) CreateFromOperator(ctx context.Context, workflow stri…` | method | own 0/7, admitted 0/0, other 0/0; via lifecycle-gateway.LifecycleManager(adm=0, all=1) | keep K1 |
| `Manager.List` | `pkg/lifecycle/manager_query.go:36` — `func (m *Manager) List(ctx context.Context, workflow string, opts ListO…` | method | own 0/2, admitted 0/0, other 0/0; via lifecycle-gateway.LifecycleManager(adm=0, all=2) | keep K1 |
| `Manager.Watch` | `pkg/lifecycle/manager_query.go:142` — `func (m *Manager) Watch(ctx context.Context, workflow string) (<-chan P…` | method | own 0/9, admitted 0/0, other 1/0; via lifecycle-gateway.LifecycleManager(adm=0, all=1) | keep K1 |
| `Manager.WatchEvents` | `pkg/lifecycle/manager_query.go:177` — `func (m *Manager) WatchEvents(ctx context.Context, workflow string) (<-…` | method | own 0/2, admitted 0/0, other 0/0 | keep K1 |
| `Manager.History` | `pkg/lifecycle/manager_query.go:351` — `func (m *Manager) History(ctx context.Context, workflow, entityID strin…` | method | own 0/6, admitted 0/0, other 0/0; via lifecycle-gateway.LifecycleManager(adm=0, all=1) | keep K1 |
| `Manager.Children` | `pkg/lifecycle/manager_query.go:389` — `func (m *Manager) Children(ctx context.Context, parentEntityID string, …` | method | own 0/0, admitted 0/0, other 0/0; via lifecycle-gateway.LifecycleManager(adm=0, all=1) | keep K1 |
| `Manager.References` | `pkg/lifecycle/manager_query.go:455` — `func (m *Manager) References(ctx context.Context, entityID string) ([]R…` | method | own 0/1, admitted 0/0, other 0/0 | keep K1 |
| `Manager.FieldType` | `pkg/lifecycle/manager_query.go:580` — `func (m *Manager) FieldType(workflow, fieldJSONName string) (reflect.Ty…` | method | own 0/0, admitted 0/0, other 0/0 | keep K1 |
| `Manager.ListWorkflows` | `pkg/lifecycle/manager_query.go:604` — `func (m *Manager) ListWorkflows() []WorkflowDef {` | method | own 0/0, admitted 0/0, other 0/0; via lifecycle-gateway.LifecycleManager(adm=0, all=1) | keep K1 |
| `ErrInvalidContract` | `pkg/projection/contract.go:29` — `var ErrInvalidContract = contract.ErrInvalidContract` | var | own 0/3, admitted 0/0, other 0/0 | keep K1 |
| `MutationClient.Create` | `pkg/projection/mutation_client.go:133` — `func (c *MutationClient) Create(ctx context.Context, request CreateMuta…` | method | own 0/6, admitted 0/0, other 2/0 | keep K1 |
| `MutationClient.Append` | `pkg/projection/mutation_client.go:210` — `func (c *MutationClient) Append(ctx context.Context, request AppendMuta…` | method | own 0/5, admitted 0/0, other 0/0 | keep K1 |
| `MutationClient.Delete` | `pkg/projection/mutation_client.go:242` — `func (c *MutationClient) Delete(ctx context.Context, request DeleteMuta…` | method | own 0/0, admitted 0/0, other 0/0; via projection.EntityDeleter(adm=0, all=1), lessons.authoritativeCleaner(adm=0, all=1) | keep K1 |
| `EntityCreator` | `pkg/projection/mutation_types.go:148` — `type EntityCreator interface {` | type | own 0/0, admitted 0/0, other 0/0 | keep K1 |
| `TripleAppender` | `pkg/projection/mutation_types.go:158` — `type TripleAppender interface {` | type | own 0/0, admitted 0/0, other 0/0 | keep K1 |
| `EntityDeleter` | `pkg/projection/mutation_types.go:163` — `type EntityDeleter interface {` | type | own 0/0, admitted 0/0, other 1/0 | keep K1 |
| `AuthoritativeReader` | `pkg/projection/mutation_types.go:168` — `type AuthoritativeReader interface {` | type | own 0/0, admitted 0/0, other 3/0 | keep K1 |
| `WithMetricsRegistry` | `pkg/worker/pool.go:57` — `func WithMetricsRegistry[T any](registry *metric.MetricsRegistry, prefi…` | func | own 0/0, admitted 0/0, other 0/0 | drop |
| `Pool.SubmitBlocking` | `pkg/worker/pool.go:186` — `func (p *Pool[T]) SubmitBlocking(ctx context.Context, work T) error {` | method | own 0/0, admitted 0/0, other 0/0 | drop |
| `Component.ConfigSchema` | `processor/graph-ingest/component.go:835` — `func (c *Component) ConfigSchema() component.ConfigSchema {` | method | own 0/1, admitted 0/0, other 0/0; via component.Discoverable(adm=0, all=5), component.LifecycleComponent(adm=0, all=5) | keep K4 |
| `Component.MergeEntity` | `processor/graph-ingest/component.go:2021` — `func (c *Component) MergeEntity(ctx context.Context, entity *graph.Enti…` | method | own 0/27, admitted 0/0, other 0/0 | drop |
| `Registry.Instances` | `storage/storeregistry/storeregistry.go:106` — `func (r *Registry) Instances() []string {` | method | own 0/4, admitted 0/1, other 0/0 | drop |
