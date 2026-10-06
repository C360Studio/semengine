<!-- markdownlint-disable MD013 -->
# Inventory: setup-04a-02-ingest-kernel (#91)

- **base:** `1334ce6` (worktree `claude/setup-04a-02-ingest-kernel`; `main` at `89c878e`)
- **pin:** SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, from `gh api repos/C360Studio/semstreams/tarball/<sha>`
  (sha256 of the tarball `47a0c1c5a050a3d92b392f8b98c98a867c3c67d15cc8a5b275d4523f14773f48`). Every `path:line`
  without a repository prefix below is a pin path.
- **consumers (symbol level, `docs/inventory-scope.md`):** semsource `e4febc0d`, semconnect PR #74 head `dff12657`,
  semteams `d5ee6325` (`pkg/lifecycle` and agentic seams only), semboids `8c03cc53` (`pkg/lifecycle` only). Tarballs
  from `gh api`; no git command touched a sister checkout.
- **scratch evidence (local only, not committed):** the pin tarball; a copy of the base with the 19 packages placed at
  their destinations and imports rewritten (`se-copy/`); the AST reader `rd/main.go`; `audit/*.tsv`;
  `method-verify.txt`; `deadlist.tsv`; probe outputs `probe-race.txt`, `probe-shuffle-{1,2,3}.txt`.

Question (rule 1 of `docs/inventory-scope.md`): what does porting closure(graph-ingest) − change 1 touch, and what
already exists or is already claimed on that territory? Repositories read: the pin (code facts), the four consumers at
symbol level for the 17 packages, nothing else.

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
| `TestImportGraph` | `component/lifecycle_test_suite.go:11` imports `testing` (known T-B1 collision; file not ported) |
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

**P-7 unit coverage at the pin** for this change's critical-list packages (foundation D10): graph-ingest 67.0%, graph
83.9%, graph/readiness 85.2%, pkg/projection 66.5%, internal/graphmutation 70.3%, component 63.5%,
storage/storeregistry 100%, pkg/lifecycle 67.9%, graph/inference 47.8%, graph/structural 89.8%. Merged (unit +
integration) not measured at the pin.

## 2. Surface audit (Extraction slices)

Method: `rd` (go/ast) lists every exported identifier and exported method of the 17 live packages and counts readers:
selector on the package's import name in every pin file, split admitted (the 65 of `docs/tier1-cross-check.md`) /
other / test, plus non-test files of the four consumers. Methods are matched by name (no type information); every
method with no reader in the first pass was re-searched by `\.Name\b` across all non-test files of the 65 and the
consumers (`method-verify.txt`). Dead = no reader in the package's own non-test code, none in an admitted package, none
in a consumer. Dormant packages (`graph/llm`, `model/wire`) are carried untouched and not audited.

- **(a) exported, nothing reads it: 157 candidates** (96 top-level, 61 methods; full pinned list in the appendix).
  Not dead after classification: `graph/inference.RegisterPayloads` and `pkg/lifecycle.RegisterPayloads` (read at the
  pin only by `payloadbuiltins`; under D1 they are the consumer's per-package call, #33); the three
  `lifecycle_test_suite.go` functions (file not ported). Clusters that matter:
  - **metrics nothing writes:** `component.NewProcessorMetrics` (`component/metrics.go:31`) and
    `inference.NewReviewMetrics` (`graph/inference/metrics.go:26`) have no caller anywhere at the pin, so their 11
    registrations never run; `worker.WithMetricsRegistry` (`pkg/worker/pool.go:57`) has no caller, so `Pool`'s 7
    metrics and its `metricsUpdater` goroutine never run (class `silent-noop-surface`).
  - `graph/errors.go:13-91`: 20 sentinel errors nothing returns or tests.
  - `graph.IncomingEdges` and its 6 methods (`graph/incoming.go`): test-only.
  - `component` config helpers `GetString/GetInt/GetBool/GetFloat64`, `ValidateJSONSize`, `ValidateComponentConfig`,
    `ValidateAndPersistComponentConfig`, `Registerable`, `IsLifecycleComponent`, `LogLevel*`/`LogEntry`.
  - `pkg/lifecycle.Manager`: `GetRaw`, `UpdateFromOperator`, `Despawn`, `DespawnWith`, `CreateFromOperator`,
    `WatchEvents`, `Children`, `FieldType`, `ListWorkflows` — read at the pin only by `gateway/lifecycle-gateway`
    (not admitted) or nobody; none read by semteams or semboids.
  - `model`: 9 methods and 12 identifiers read only by `processor/agentic-*` (not admitted).
- **(b) config:** `processor/graph-ingest` decodes its config with plain `json.Unmarshal`
  (`processor/graph-ingest/component.go:672` — `if err := json.Unmarshal(rawConfig, &config); err != nil {`): an
  unknown key is accepted silently (`phantom-config`). All four fields (`Ports`, `EnableHierarchy`,
  `EnableTypeSiblings`, `IngestLanes`) are read by behavior. `IngestLanes < 1` is clamped to 1 in `Validate` (a
  silent degrade, commented as deliberate). `component/schema.go:29` and `component/validation.go:186` state that
  unknown fields are allowed. Not yet done: the same read for `graph/inference.Config` and `pkg/dispatch` configs
  (**open evidence question**).
- **(c) described, not implemented:** `processor/graph-ingest/TEST_DISPUTE.md` (74 lines) describes test/API disputes
  against a `natsclient` API that no longer exists; `component/README.md` (445), `graph/README.md` (144),
  `pkg/worker/README.md` (827), `types/README.md` (105), `processor/graph-ingest/README.md` (127) were not read
  claim by claim (**open evidence question**; change 1 did this per package in its port tasks).

## 3. Inventory categories

**1. The claimed gap.** The change claims "graph-ingest runs as a component under the lifecycle suite, with no boot
path". Searches on the base: `git grep -n 'graph-ingest\|graphingest\|LifecycleComponent\|componentadmission' -- '*.go'`
→ no hits; `ls processor component graph pkg/lifecycle pkg/projection` → absent. The gap is real. The harness pieces
the proofs need exist (`natsfixture.Restart` `internal/harness/natsfixture/restart.go:40`, `NewFaultKV`
`faultkv.go:47`, `prochost.Start` `internal/harness/prochost/prochost.go:72`, `lifecycletest.Run`
`lifecycletest.go:77`, `payloadfixture.NewWithSubset` `internal/harness/payloadfixture/testing.go:33`).

**2. Every current spelling of the facts this change models.**

- *Failed-start rollback:* `internal/harness/natsfixture/rollback.go:17` (base; adapted copy, 15 s budget,
  `rollbackBudget`) and pin `internal/lifecyclecleanup/lifecyclecleanup.go:12` — `const failedStartRollbackTimeout =
  5 * time.Second` (production policy, called by 33 pin files, one in this change:
  `processor/graph-ingest/component.go:984`). Two homes after the port; `natsfixture` may not import a module package
  (`harness-boundaries` "Import graph").
- *Deployment authority:* `graph/inference.HierarchyConfig.{Org,Platform}` (`hierarchy.go:100-101`), graph-ingest's
  unexported `org`/`platform` copies (`component.go:772-773`), `graph/llm.EntityParts.{Org,Platform}` (dormant). The
  carrier is `component.Dependencies.Platform` (allowed by name in `internal/harness/contract/authority_test.go`).
- *Metric registration:* the pin's six `Register*` methods (removed) and graph-ingest's process-global `sync.Once`
  collectors with a `prometheus.DefaultRegisterer` fallback (`component.go`: 12 sites, `poison_inventory.go`: 1;
  `graph/readiness/gauges.go`: 1); the base has one home, `metric.RegisterOrGet` (`metric/registry.go:29`), and no
  `DefaultRegisterer` use (`git grep -n DefaultRegisterer` → none).
- *Outward-facing `semstreams` names in the 17:* metric namespace `"semstreams"` ×17 (`component/metrics.go` 4,
  `graph/readiness/gauges.go:79`, `graph/inference/metrics.go:32`, graph-ingest 13 incl. `component.go:2851`,
  `poison_inventory.go:66`); bucket `BucketSemStreamsConfig = "semstreams_config"` (`graph/constants.go:74`; readers
  at the pin: `graph/kvcatalog.go`, `config/bucket_name.go`, `config/manager.go`); `InterfaceType =
  "semstreams.graph.mutation"` (`internal/graphmutation/protocol.go:12`); `alertDigestDomain =
  "semstreams.graph.alert.v1"` (`graph/events.go:19`); bucket description "SemStreams runtime configuration"
  (`graph/kvcatalog.go:134`); 79 comment lines. No `SEMSTREAMS_` env var and no `SemStreamsBase` reader in the 17
  (env reads: `model/registry.go:83`, `graph/llm/config.go:87` `LLM_API_KEY`).
- *Reserved request subjects (#16):* `graph/exact_entity.go:15`, `processor/graph-ingest/query.go:27,34,41,48,55`,
  `internal/graphmutation/protocol.go:16` (`graph.mutation.>`), prefix test `canonical_mutations.go:174`; later
  changes: `component/flowgraph/flowgraph.go:264` (3), graph-query (4).
- *Test fixture client:* `natsclient/test_helpers_integration_test.go:27-70` (base, package-internal, not importable);
  this change needs the same helper in five packages (43 sites).

**3. Adjacent claims.** `gh pr list --state open` → **#92** (draft, `claude/semengine-names`, 0 files at read time;
claim for #69: renames every outward-facing name to `semengine`, `SEMENGINE_`, `vocabulary.SemEngineBase`) and **#93**
(this claim, 0 files). No capability delta overlaps. Issues on the territory: #15, #16, #19, #20 (repair rows; #19 Q6
and #20 Q7 ruled 2026-10-01), #29, #33 (`class:port-refactor`), #75, #78, #81, #85 (#48 follow-ups), #77
(`status:needs-decision`, its own inventory running since 2026-10-06 per its comment), #24 (durable execution, blocked
on #9), SemStreams PR #1437 (open; head `0ea823a6`, graph-ingest half: `component.go` +30/−5, two test files +401; Q13).
Ledger rows on the territory: `internal/lifecyclecleanup/lifecyclecleanup.go` (file row, `adapt → natsfixture`),
`component/lifecycle_test_suite.go` (`adapt → lifecycletest`), `processor/graph-query/lifecycle_owner_test.go`;
`agentic`, `internal/looptoken`, `vocabulary/agentic` (`defer-exclude`). Specs: `lifecycle-suite`, `background-work`,
`harness-boundaries`, `transport-client` (settlement), `metric-registry`.

**4. Consumer at birth (new surface this change could add).** Candidates and their present consumers: a shared
fixture-client helper (5 packages, 43 sites in this change); an `ExpectedRevision` field on
`projection.ReconcileMutation` (#19 Q6; semsource #215, semconnect `graph_mutations.go:214-215`); a reserved-subject
declaration in `graph` (#16; graph-ingest, graph/exact_entity.go now, graph-query in change 4). No other new exported
symbol is needed by the issue's scope.

**5. The problem shape.** (i) *Owner ports under a portable lifecycle floor* — nearest instance: change 1's
`metric.Server` / `natsclient.Client` adapters (`metric/lifecycle_test.go`, `natsclient/client_lifecycle_test.go`); adopt.
(ii) *Background work in three shapes* — nearest: `internal/resource.Watcher.Run`, `internal/cache.CoalescingSet.Shutdown`;
adopt. (iii) *Classified refusal plus observed signal* for commit outcomes — nearest: `natsclient.SettleDelivery`
(`natsclient/delivery_settlement.go:277`) and the pin's `CommitUnknown` (`pkg/projection/mutation_types.go:74`); adopt.
(iv) *Create-vs-exists / one canonical collector* — `metric.RegisterOrGet`; adopt. No new pattern is established, so
no adoption sweep is owed, except the fixture-client helper if it is chosen (its adopters are the 5 packages here and
the 20 later packages D4 lists).

## 4. Same-class collision table (runtime coordination: failed-start rollback)

| Dimension | Evidence |
| --- | --- |
| Semantic class | release what a failed `Start` acquired, bounded, keeping parent values |
| Owners | pin `internal/lifecyclecleanup` (5 s); base `natsfixture.rollback` (15 s); pin `service/component_manager.go` (change 3) per #77's placement comment |
| Catalogs | none (`git grep -n lifecyclecleanup` on base: ledger and the `natsfixture` comment only) |
| Status | component `Health()`; graph-ingest `cleanupPending`/`terminal` fields (`component.go:984-994`) |
| Lifecycle | rollback runs synchronously inside `Start`'s deferred block; on failure obligations stay for `Stop` |
| Ownership | the component (pin shape); #77 asks whether external owners get an exported helper or the manager owns it |
| Readers | graph-ingest only in this change; 32 more pin files in later changes |
| Writers | n/a |
| Recovery | a failed rollback leaves `cleanupPending=true`; `Stop` retries cleanup |

## 5. Adopter seam inventory

Surfaces reached from outside: `component.Dependencies` and `Registry` (consumer composition roots: semsource, semconnect,
semboids, semteams import `component` 76/23 times), `graphingest.Register` / `CreateGraphIngest`, graph-ingest's
config JSON, its metrics (scraped), `projection.MutationClient`, `pkg/lifecycle.Manager`.

1. **What must they know?** (a) graph-ingest needs its input stream to exist before `Start` (P-5: 49 s then an error);
   (b) `deps.Platform` must carry the authority (refused at construction, `component.go:721`); (c) the payload
   registry must hold the types graph-ingest decodes (the refusal text names `payloadbuiltins.Register`, which
   SemEngine does not ship, `component.go:739`); (d) metric names change from `semstreams_*` to `semengine_*`.
2. **If they do nothing:** (a) a 49 s start then a boot error naming the stream — loud; (b) a construction error —
   loud; (c) a boot error naming a symbol that does not exist (#33) — loud but wrong; (d) dashboards go silent.
   Config typos: silently ignored today (§2 b).
3. **Where they find out:** (a)(b) boot error; (c) boot error with a wrong pointer; (d) nowhere; config typo nowhere.
4. **What they should have to know:** nothing for (c) and the config typo — the refusal names the per-package call,
   and an unknown key is refused at construction; (d) is a one-time rename with no consumer pinned to SemEngine yet
   (#69 ruling).

## 6. Intent check (boundary moved by this change: none)

This change moves no tier or package boundary: it ports exactly D2 row 2. Capabilities named in `AGENTS.md` "What
this is for": ingest — admitted (this change; #8 Q4 tier-0 set); index, query — admitted (change 4); vocabulary —
admitted (change 1); provenance — admitted (`pkg/projection`, `graph`; this change); fusion — admitted (change 5);
tier ladder — admitted (#8 tier model, 5932313950); workflows that survive restarts, replay, settlement, retries —
admitted (`pkg/lifecycle` Q16, settlement Q18 here; the general primitive #24 deferred to after #9's first green
extraction, ruled on #8); rules — admitted (change 6, Q15). No deferral without a ruling.

## 7. Guidance at the pin that returns with these packages

SemStreams developer contract `.agents/contracts/semstreams-developer.md`: "Semantic identity and graph contracts"
(`:109-126`), "Payload registry" (`:234-242`), "State ownership and component wiring" (`:243-252`). Reviewer contract:
"Semantic identity and graph review" (`:155-169`), "Payload registry" (`:227-235`), "Graph and state ownership"
(`:236-245`), "Component and schema wiring" (`:246-253`). Skills: `entity-or-bucket` (136 lines), `kv-or-stream` (94),
`new-payload` (264; its "Payload registry" sections were not carried with `payloadregistry` in change 1), `query-pattern`
(93; graph-ingest serves the four `graph.ingest.query.*` handlers). `orchestration-check` belongs to the rule core
(change 6). The base contracts have none of these sections (`grep -n '^## \|^### ' .agents/contracts/semengine-*.md`).

## Open evidence questions (for the inventory reviewer)

1. §2 (b) for `graph/inference.Config` and the `pkg/dispatch` configs; §2 (c) claim-by-claim README reads.
2. Method-level dead claims rest on a by-name search: a method reached only through an interface value whose name
   differs, or by reflection (`text/template`), is a false "dead". Each drop needs a `gopls references` before it
   lands.
3. P-5 ran with a vacuous `Observe`; the in-package adapter is the real check.

## Appendix: dead-surface candidates (157), pinned at the pin

| Identifier | Pin | Kind | Test / non-admitted readers |
| --- | --- | --- | --- |
| `ValidateAndPersistComponentConfig` | `component/config_validator.go:123` — `func ValidateAndPersistComponentConfig(` | func | ownTest=0 admTest=0 other=0 |
| `ValidateComponentConfig` | `component/config_validator.go:91` — `func ValidateComponentConfig(` | func | ownTest=0 admTest=0 other=0 |
| `BenchmarkLifecycleMethods` | `component/lifecycle_test_suite.go:330` — `func BenchmarkLifecycleMethods(b *testing.B, factory LifecycleFactory) {` | func | ownTest=1 admTest=1 other=0 |
| `TestErrorInjection` | `component/lifecycle_test_suite.go:452` — `func TestErrorInjection(t *testing.T, factory LifecycleFactory) {` | func | ownTest=1 admTest=1 other=0 |
| `StandardLifecycleTests` | `component/lifecycle_test_suite.go:98` — `func StandardLifecycleTests(t *testing.T, factory LifecycleFactory) {` | func | ownTest=1 admTest=2 other=0 |
| `IsLifecycleComponent` | `component/lifecycle.go:94` — `func IsLifecycleComponent(comp Discoverable) bool {` | func | ownTest=0 admTest=0 other=0 |
| `LogLevelInfo` | `component/logging.go:10` — `LogLevelInfo LogLevel = "INFO"` | const | ownTest=1 admTest=0 other=0 |
| `LogLevelWarn` | `component/logging.go:12` — `LogLevelWarn LogLevel = "WARN"` | const | ownTest=1 admTest=0 other=0 |
| `LogLevelError` | `component/logging.go:14` — `LogLevelError LogLevel = "ERROR"` | const | ownTest=1 admTest=0 other=0 |
| `LogEntry` | `component/logging.go:19` — `type LogEntry struct {` | type | ownTest=1 admTest=0 other=0 |
| `LogLevelDebug` | `component/logging.go:8` — `LogLevelDebug LogLevel = "DEBUG"` | const | ownTest=1 admTest=0 other=0 |
| `ProcessorMetrics.ObserveDuration` | `component/metrics.go:100` — `func (m *ProcessorMetrics) ObserveDuration(seconds float64) {` | method | by-name check |
| `NewProcessorMetrics` | `component/metrics.go:31` — `func NewProcessorMetrics(registry *metric.MetricsRegistry, subsystem string) *Pr…` | func | ownTest=0 admTest=0 other=0 |
| `ProcessorMetrics.RecordEvent` | `component/metrics.go:85` — `func (m *ProcessorMetrics) RecordEvent(operation string) {` | method | by-name check |
| `ProcessorMetrics.RecordKVOperation` | `component/metrics.go:95` — `func (m *ProcessorMetrics) RecordKVOperation(operation string) {` | method | by-name check |
| `PortFacts.KVReadBucket` | `component/port_facts.go:105` — `func (f PortFacts) KVReadBucket() (string, bool) {` | method | by-name check |
| `MergePortConfig` | `component/ports.go:184` — `func MergePortConfig(defaults, overrides PortConfig) (PortConfig, error) {` | func | ownTest=1 admTest=0 other=6 |
| `Registerable` | `component/registerable.go:4` — `type Registerable interface {` | type | ownTest=0 admTest=0 other=0 |
| `GetFloat64` | `component/registry.go:1003` — `func GetFloat64(config map[string]any, key string, defaultValue float64) float64…` | func | ownTest=0 admTest=0 other=0 |
| `Registry.ListComponentTypes` | `component/registry.go:502` — `func (r *Registry) ListComponentTypes() []string {` | method | by-name check |
| `Registry.GetFactory` | `component/registry.go:592` — `func (r *Registry) GetFactory(name string) (Factory, bool) {` | method | by-name check |
| `Registry.ListAvailable` | `component/registry.go:637` — `func (r *Registry) ListAvailable() map[string]Info {` | method | by-name check |
| `ValidateJSONSize` | `component/registry.go:685` — `func ValidateJSONSize(data json.RawMessage) error {` | func | ownTest=0 admTest=0 other=0 |
| `GetString` | `component/registry.go:918` — `func GetString(config map[string]any, key string, defaultValue string) string {` | func | ownTest=0 admTest=0 other=0 |
| `GetInt` | `component/registry.go:946` — `func GetInt(config map[string]any, key string, defaultValue int) int {` | func | ownTest=0 admTest=0 other=0 |
| `GetBool` | `component/registry.go:988` — `func GetBool(config map[string]any, key string, defaultValue bool) bool {` | func | ownTest=0 admTest=0 other=0 |
| `PortFieldInfo.ZeroIsOmitted` | `component/schema_tags.go:106` — `func (p PortFieldInfo) ZeroIsOmitted() bool { return p.zeroIsOmitted }` | method | by-name check |
| `GetPropertyValue` | `component/schema.go:274` — `func GetPropertyValue(config map[string]any, key string) (any, bool) {` | func | ownTest=1 admTest=0 other=0 |
| `GetProperties` | `component/schema.go:311` — `func GetProperties(schema ConfigSchema, category string) map[string]PropertySche…` | func | ownTest=1 admTest=0 other=0 |
| `IsComplexType` | `component/schema.go:353` — `func IsComplexType(propType string) bool {` | func | ownTest=1 admTest=0 other=0 |
| `SortedPropertyNames` | `component/schema.go:380` — `func SortedPropertyNames(schema ConfigSchema) []string {` | func | ownTest=1 admTest=0 other=0 |
| `ValidateNetworkConfig` | `component/validation.go:207` — `func ValidateNetworkConfig(port int, bindAddr string) error {` | func | ownTest=0 admTest=0 other=1 |
| `ErrEntityNotFound` | `graph/errors.go:13` — `ErrEntityNotFound = errors.New("entity not found")` | var | ownTest=0 admTest=0 other=0 |
| `ErrEntityExists` | `graph/errors.go:16` — `ErrEntityExists = errors.New("entity already exists")` | var | ownTest=0 admTest=0 other=0 |
| `ErrInvalidEntityID` | `graph/errors.go:19` — `ErrInvalidEntityID = errors.New("invalid entity ID")` | var | ownTest=0 admTest=0 other=0 |
| `ErrInvalidEntityData` | `graph/errors.go:22` — `ErrInvalidEntityData = errors.New("invalid entity data")` | var | ownTest=0 admTest=0 other=0 |
| `ErrVersionConflict` | `graph/errors.go:25` — `ErrVersionConflict = errors.New("entity version conflict")` | var | ownTest=0 admTest=0 other=0 |
| `ErrIndexNotFound` | `graph/errors.go:31` — `ErrIndexNotFound = errors.New("index not found")` | var | ownTest=0 admTest=0 other=0 |
| `ErrIndexCorrupted` | `graph/errors.go:34` — `ErrIndexCorrupted = errors.New("index corrupted")` | var | ownTest=0 admTest=0 other=0 |
| `ErrIndexUpdateFailed` | `graph/errors.go:37` — `ErrIndexUpdateFailed = errors.New("index update failed")` | var | ownTest=0 admTest=0 other=0 |
| `ErrInvalidIndexKey` | `graph/errors.go:40` — `ErrInvalidIndexKey = errors.New("invalid index key")` | var | ownTest=0 admTest=0 other=0 |
| `ErrQueryTimeout` | `graph/errors.go:46` — `ErrQueryTimeout = errors.New("query timeout")` | var | ownTest=0 admTest=0 other=0 |
| `ErrQueryTooComplex` | `graph/errors.go:49` — `ErrQueryTooComplex = errors.New("query too complex")` | var | ownTest=0 admTest=0 other=0 |
| `ErrQueryDepthExceeded` | `graph/errors.go:52` — `ErrQueryDepthExceeded = errors.New("query depth exceeded")` | var | ownTest=0 admTest=0 other=0 |
| `ErrInvalidQueryParams` | `graph/errors.go:55` — `ErrInvalidQueryParams = errors.New("invalid query parameters")` | var | ownTest=0 admTest=0 other=0 |
| `ErrAliasNotFound` | `graph/errors.go:61` — `ErrAliasNotFound = errors.New("alias not found")` | var | ownTest=0 admTest=0 other=0 |
| `ErrAliasExists` | `graph/errors.go:64` — `ErrAliasExists = errors.New("alias already exists")` | var | ownTest=0 admTest=0 other=0 |
| `ErrInvalidAlias` | `graph/errors.go:67` — `ErrInvalidAlias = errors.New("invalid alias")` | var | ownTest=0 admTest=0 other=0 |
| `ErrBufferFull` | `graph/errors.go:73` — `ErrBufferFull = errors.New("buffer full")` | var | ownTest=0 admTest=0 other=0 |
| `ErrBatchTooBig` | `graph/errors.go:76` — `ErrBatchTooBig = errors.New("batch too big")` | var | ownTest=0 admTest=0 other=0 |
| `ErrFlushFailed` | `graph/errors.go:79` — `ErrFlushFailed = errors.New("flush failed")` | var | ownTest=0 admTest=0 other=0 |
| `ErrNotStarted` | `graph/errors.go:85` — `ErrNotStarted = errors.New("service not started")` | var | ownTest=0 admTest=0 other=0 |
| `ErrAlreadyStarted` | `graph/errors.go:88` — `ErrAlreadyStarted = errors.New("service already started")` | var | ownTest=0 admTest=0 other=0 |
| `ErrShuttingDown` | `graph/errors.go:91` — `ErrShuttingDown = errors.New("service shutting down")` | var | ownTest=0 admTest=0 other=0 |
| `NewRelationshipCreateEvent` | `graph/events.go:158` — `func NewRelationshipCreateEvent(` | func | ownTest=1 admTest=0 other=0 |
| `NewAlertEvent` | `graph/events.go:175` — `func NewAlertEvent(` | func | ownTest=1 admTest=0 other=0 |
| `NewEntityCreateEvent` | `graph/events.go:204` — `func NewEntityCreateEvent(` | func | ownTest=1 admTest=0 other=0 |
| `NewEntityDeleteEvent` | `graph/events.go:217` — `func NewEntityDeleteEvent(entityID, reason string, metadata EventMetadata) (*Eve…` | func | ownTest=1 admTest=0 other=0 |
| `NewRelationshipDeleteEvent` | `graph/events.go:223` — `func NewRelationshipDeleteEvent(` | func | ownTest=1 admTest=0 other=0 |
| `GetPropertyValueTyped` | `graph/helpers.go:28` — `func GetPropertyValueTyped[T any](entity *EntityState, predicate string) (T, boo…` | func | ownTest=1 admTest=0 other=0 |
| `GetProperties` | `graph/helpers.go:46` — `func GetProperties(entity *EntityState) map[string]any {` | func | ownTest=1 admTest=0 other=0 |
| `GetRelationshipTriples` | `graph/helpers.go:62` — `func GetRelationshipTriples(entity *EntityState) []message.Triple {` | func | ownTest=1 admTest=0 other=0 |
| `GetPropertyTriples` | `graph/helpers.go:78` — `func GetPropertyTriples(entity *EntityState) []message.Triple {` | func | ownTest=1 admTest=0 other=0 |
| `HasProperty` | `graph/helpers.go:93` — `func HasProperty(entity *EntityState, predicate string) bool {` | func | ownTest=1 admTest=0 other=0 |
| `IncomingEdges.AddIncomingEdge` | `graph/incoming.go:22` — `func (ie *IncomingEdges) AddIncomingEdge(edge IncomingEdge) {` | method | by-name check |
| `IncomingEdges.RemoveIncomingEdge` | `graph/incoming.go:37` — `func (ie *IncomingEdges) RemoveIncomingEdge(fromEntityID, edgeType string) {` | method | by-name check |
| `IncomingEdges.GetIncomingEdgesByType` | `graph/incoming.go:49` — `func (ie *IncomingEdges) GetIncomingEdgesByType(edgeType string) []IncomingEdge…` | method | by-name check |
| `IncomingEdges.GetIncomingEntityIDs` | `graph/incoming.go:60` — `func (ie *IncomingEdges) GetIncomingEntityIDs() []string {` | method | by-name check |
| `IncomingEdges` | `graph/incoming.go:7` — `type IncomingEdges struct {` | type | ownTest=1 admTest=0 other=0 |
| `IncomingEdges.HasIncomingFrom` | `graph/incoming.go:80` — `func (ie *IncomingEdges) HasIncomingFrom(fromEntityID string) bool {` | method | by-name check |
| `IncomingEdges.HasIncomingOfType` | `graph/incoming.go:90` — `func (ie *IncomingEdges) HasIncomingOfType(edgeType string) bool {` | method | by-name check |
| `NewNoOpApplier` | `graph/inference/applier.go:117` — `func NewNoOpApplier(logger *slog.Logger) *NoOpApplier {` | func | ownTest=0 admTest=0 other=0 |
| `NewDirectRelationshipApplier` | `graph/inference/applier.go:152` — `func NewDirectRelationshipApplier(adder TripleAdder, logger *slog.Logger) *Direc…` | func | ownTest=1 admTest=0 other=0 |
| `NewNATSRelationshipApplier` | `graph/inference/applier.go:35` — `func NewNATSRelationshipApplier(` | func | ownTest=0 admTest=0 other=1 |
| `RegisterPayloads` | `graph/inference/container_entity.go:62` — `func RegisterPayloads(reg *payloadregistry.Registry) error {` | func | ownTest=0 admTest=0 other=1 |
| `Orchestrator.UpdateConfig` | `graph/inference/detector.go:329` — `func (o *Orchestrator) UpdateConfig(config Config) error {` | method | by-name check |
| `Orchestrator.GetRegisteredDetectors` | `graph/inference/detector.go:361` — `func (o *Orchestrator) GetRegisteredDetectors() []string {` | method | by-name check |
| `Result.AnomalyCount` | `graph/inference/detector.go:534` — `func (r *Result) AnomalyCount() int {` | method | by-name check |
| `Result.CountByType` | `graph/inference/detector.go:539` — `func (r *Result) CountByType() map[AnomalyType]int {` | method | by-name check |
| `DefaultHierarchyConfig` | `graph/inference/hierarchy.go:122` — `func DefaultHierarchyConfig() HierarchyConfig {` | func | ownTest=1 admTest=0 other=0 |
| `HierarchyInference.OnEntityCreated` | `graph/inference/hierarchy.go:290` — `func (h *HierarchyInference) OnEntityCreated(ctx context.Context, entityID strin…` | method | by-name check |
| `HierarchyInference.GetMetrics` | `graph/inference/hierarchy.go:530` — `func (h *HierarchyInference) GetMetrics() (containersCreated, edgesCreated, edge…` | method | by-name check |
| `HierarchyInference.GetCacheStats` | `graph/inference/hierarchy.go:535` — `func (h *HierarchyInference) GetCacheStats() int {` | method | by-name check |
| `NewHTTPHandler` | `graph/inference/http_handlers.go:21` — `func NewHTTPHandler(storage Storage, applier RelationshipApplier, logger *slog.L…` | func | ownTest=1 admTest=0 other=1 |
| `ReviewMetrics.SetPendingCount` | `graph/inference/metrics.go:149` — `func (m *ReviewMetrics) SetPendingCount(count int) {` | method | by-name check |
| `ReviewMetrics.IncWorkersActive` | `graph/inference/metrics.go:157` — `func (m *ReviewMetrics) IncWorkersActive() {` | method | by-name check |
| `ReviewMetrics.DecWorkersActive` | `graph/inference/metrics.go:165` — `func (m *ReviewMetrics) DecWorkersActive() {` | method | by-name check |
| `NewReviewMetrics` | `graph/inference/metrics.go:26` — `func NewReviewMetrics(component string, registry *metric.MetricsRegistry) *Revie…` | func | ownTest=0 admTest=0 other=0 |
| `NATSAnomalyStorage.GetByType` | `graph/inference/storage.go:288` — `func (s *NATSAnomalyStorage) GetByType(ctx context.Context, anomalyType AnomalyT…` | method | by-name check |
| `NATSAnomalyStorage.UpdateStatus` | `graph/inference/storage.go:342` — `func (s *NATSAnomalyStorage) UpdateStatus(` | method | by-name check |
| `StructuralAnomaly.NeedsHumanReview` | `graph/inference/types.go:174` — `func (a *StructuralAnomaly) NeedsHumanReview() bool {` | method | by-name check |
| `FrameworkOwnedBuckets` | `graph/kvcatalog.go:219` — `func FrameworkOwnedBuckets() []string {` | func | ownTest=3 admTest=0 other=0 |
| `Config.IsEnabled` | `graph/llm/config.go:66` — `func (c Config) IsEnabled() bool {` | method | by-name check |
| `Config.ToOpenAIConfig` | `graph/llm/config.go:71` — `func (c Config) ToOpenAIConfig() OpenAIConfig {` | method | by-name check |
| `MissingUnknown` | `graph/query_batch_types.go:39` — `MissingUnknown MissingReason = "unknown"` | const | ownTest=0 admTest=1 other=0 |
| `ContextEntry` | `graph/query_index_types.go:19` — `type ContextEntry struct {` | type | ownTest=0 admTest=0 other=0 |
| `NameIndexEntry` | `graph/query_name_types.go:34` — `type NameIndexEntry struct {` | type | ownTest=0 admTest=0 other=0 |
| `PredicateQueryResponse` | `graph/query_response_types.go:16` — `type PredicateQueryResponse = QueryResponse[PredicateData]` | type | ownTest=0 admTest=5 other=0 |
| `PredicateListQueryResponse` | `graph/query_response_types.go:19` — `type PredicateListQueryResponse = QueryResponse[PredicateListData]` | type | ownTest=0 admTest=5 other=1 |
| `PredicateStatsQueryResponse` | `graph/query_response_types.go:22` — `type PredicateStatsQueryResponse = QueryResponse[PredicateStatsData]` | type | ownTest=0 admTest=2 other=0 |
| `CompoundPredicateQueryResponse` | `graph/query_response_types.go:25` — `type CompoundPredicateQueryResponse = QueryResponse[CompoundPredicateData]` | type | ownTest=0 admTest=2 other=0 |
| `EntityCriteria` | `graph/query_types.go:31` — `type EntityCriteria struct {` | type | ownTest=0 admTest=0 other=0 |
| `RelationshipCriteria` | `graph/query_types.go:38` — `type RelationshipCriteria struct {` | type | ownTest=0 admTest=0 other=0 |
| `QueryResult` | `graph/query_types.go:47` — `type QueryResult struct {` | type | ownTest=0 admTest=0 other=0 |
| `Gauges.MetricNames` | `graph/readiness/gauges.go:131` — `func (g *Gauges) MetricNames() []string {` | method | by-name check |
| `Set.FullyCovered` | `graph/readiness/set.go:153` — `func (s *Set) FullyCovered() Verdict {` | method | by-name check |
| `Set.Dumps` | `graph/readiness/set.go:203` — `func (s *Set) Dumps() []Dump {` | method | by-name check |
| `NewSet` | `graph/readiness/set.go:41` — `func NewSet(src BucketSource, keys []string, opts ...Option) *Set {` | func | ownTest=1 admTest=0 other=2 |
| `KCoreComputer.ComputeIncremental` | `graph/structural/kcore.go:161` — `func (c *KCoreComputer) ComputeIncremental(ctx context.Context, _ []string) (*KC…` | method | by-name check |
| `PivotComputer.ComputeIncremental` | `graph/structural/pivot.go:319` — `func (c *PivotComputer) ComputeIncremental(ctx context.Context, _ []string) (*Pi…` | method | by-name check |
| `PivotIndex.IsWithinHops` | `graph/structural/types.go:204` — `func (idx *PivotIndex) IsWithinHops(entityA, entityB string, maxHops int) bool {` | method | by-name check |
| `PivotIndex.GetReachableCandidates` | `graph/structural/types.go:213` — `func (idx *PivotIndex) GetReachableCandidates(source string, maxHops int) []stri…` | method | by-name check |
| `KCoreIndex.FilterByMinCore` | `graph/structural/types.go:73` — `func (idx *KCoreIndex) FilterByMinCore(entityIDs []string, minCore int) []string…` | method | by-name check |
| `KCoreIndex.GetEntitiesInCore` | `graph/structural/types.go:91` — `func (idx *KCoreIndex) GetEntitiesInCore(core int) []string {` | method | by-name check |
| `IsCommitUnknown` | `internal/graphmutation/client.go:57` — `func IsCommitUnknown(err error) bool {` | func | ownTest=1 admTest=0 other=0 |
| `NewRollingWindowBreaker` | `model/breaker.go:108` — `func NewRollingWindowBreaker(cfg BreakerConfig) *RollingWindowBreaker {` | func | ownTest=1 admTest=0 other=1 |
| `RollingWindowBreaker.EndpointStats` | `model/breaker.go:153` — `func (b *RollingWindowBreaker) EndpointStats(endpoint string) HealthStats {` | method | by-name check |
| `RollingWindowBreaker.RecordResult` | `model/breaker.go:178` — `func (b *RollingWindowBreaker) RecordResult(endpoint string, result Result) {` | method | by-name check |
| `ComposeHealth` | `model/health.go:188` — `func ComposeHealth(r RegistryReader, p HealthPolicy) HealthAwareRegistry {` | func | ownTest=1 admTest=0 other=0 |
| `ErrorKindNone` | `model/health.go:50` — `ErrorKindNone ErrorKind = ""` | const | ownTest=0 admTest=0 other=1 |
| `ErrorKindTimeout` | `model/health.go:52` — `ErrorKindTimeout ErrorKind = "timeout"` | const | ownTest=0 admTest=0 other=1 |
| `ErrorKindRateLimit` | `model/health.go:55` — `ErrorKindRateLimit ErrorKind = "rate_limit"` | const | ownTest=0 admTest=0 other=1 |
| `ErrorKindServerError` | `model/health.go:58` — `ErrorKindServerError ErrorKind = "server_error"` | const | ownTest=1 admTest=0 other=1 |
| `ErrorKindNetwork` | `model/health.go:62` — `ErrorKindNetwork ErrorKind = "network"` | const | ownTest=0 admTest=0 other=1 |
| `ErrorKindUnknown` | `model/health.go:65` — `ErrorKindUnknown ErrorKind = "unknown"` | const | ownTest=0 admTest=0 other=1 |
| `HTTPClientOptionsFromEndpoint` | `model/httpclient.go:157` — `func HTTPClientOptionsFromEndpoint(ep *EndpointConfig) HTTPClientOptions {` | func | ownTest=1 admTest=0 other=1 |
| `CapabilityResearchRouting` | `model/registry.go:45` — `CapabilityResearchRouting = "research_routing"` | const | ownTest=1 admTest=0 other=2 |
| `CapabilityResearchAssessment` | `model/registry.go:51` — `CapabilityResearchAssessment = "research_assessment"` | const | ownTest=1 admTest=0 other=2 |
| `CapabilityResearchSynthesis` | `model/registry.go:58` — `CapabilityResearchSynthesis = "research_synthesis"` | const | ownTest=1 admTest=0 other=2 |
| `Registry.GetFallbackChain` | `model/registry.go:625` — `func (r *Registry) GetFallbackChain(capability string) []string {` | method | by-name check |
| `Registry.GetMaxTokens` | `model/registry.go:656` — `func (r *Registry) GetMaxTokens(name string) int {` | method | by-name check |
| `Registry.GetDefault` | `model/registry.go:665` — `func (r *Registry) GetDefault() string {` | method | by-name check |
| `Registry.ListCapabilities` | `model/registry.go:670` — `func (r *Registry) ListCapabilities() []string {` | method | by-name check |
| `Registry.ListEndpoints` | `model/registry.go:680` — `func (r *Registry) ListEndpoints() []string {` | method | by-name check |
| `Registry.ResolveSummarization` | `model/registry.go:690` — `func (r *Registry) ResolveSummarization() string {` | method | by-name check |
| `Watch` | `model/watch.go:35` — `func Watch(ctx context.Context, watcher Watcher, apply func(*Registry)) {` | func | ownTest=1 admTest=0 other=0 |
| `Client.ChatCompletionStream` | `model/wire/client.go:115` — `func (c *Client) ChatCompletionStream(ctx context.Context, req *ChatCompletionRe…` | method | by-name check |
| `Client.Embeddings` | `model/wire/client.go:143` — `func (c *Client) Embeddings(ctx context.Context, req *EmbeddingsRequest) (*Embed…` | method | by-name check |
| `Accumulator.Final` | `model/wire/stream.go:314` — `func (a *Accumulator) Final() (msg Message, finishReason string, usage *Usage) {` | method | by-name check |
| `Message.ContentParts` | `model/wire/types_message.go:51` — `func (m Message) ContentParts() (parts []ContentPart, ok bool, err error) {` | method | by-name check |
| `RegisterPayloads` | `pkg/lifecycle/harness_entity.go:65` — `func RegisterPayloads(reg *payloadregistry.Registry) error {` | func | ownTest=1 admTest=0 other=1 |
| `Manager.WatchEvents` | `pkg/lifecycle/manager_query.go:177` — `func (m *Manager) WatchEvents(ctx context.Context, workflow string) (<-chan Even…` | method | by-name check |
| `Manager.Children` | `pkg/lifecycle/manager_query.go:389` — `func (m *Manager) Children(ctx context.Context, parentEntityID string, opts Chil…` | method | by-name check |
| `Manager.FieldType` | `pkg/lifecycle/manager_query.go:580` — `func (m *Manager) FieldType(workflow, fieldJSONName string) (reflect.Type, error…` | method | by-name check |
| `Manager.ListWorkflows` | `pkg/lifecycle/manager_query.go:604` — `func (m *Manager) ListWorkflows() []WorkflowDef {` | method | by-name check |
| `Manager.GetRaw` | `pkg/lifecycle/manager.go:279` — `func (m *Manager) GetRaw(ctx context.Context, entityID string) (*graph.EntitySta…` | method | by-name check |
| `Manager.UpdateFromOperator` | `pkg/lifecycle/manager.go:711` — `func (m *Manager) UpdateFromOperator(ctx context.Context, workflow, entityID str…` | method | by-name check |
| `Manager.Despawn` | `pkg/lifecycle/manager.go:858` — `func (m *Manager) Despawn(ctx context.Context, workflow, entityID string) error…` | method | by-name check |
| `Manager.DespawnWith` | `pkg/lifecycle/manager.go:899` — `func (m *Manager) DespawnWith(ctx context.Context, workflow, entityID string, so…` | method | by-name check |
| `Manager.CreateFromOperator` | `pkg/lifecycle/manager.go:997` — `func (m *Manager) CreateFromOperator(ctx context.Context, workflow string, initi…` | method | by-name check |
| `EntityCreator` | `pkg/projection/mutation_types.go:148` — `type EntityCreator interface {` | type | ownTest=0 admTest=0 other=0 |
| `TripleAppender` | `pkg/projection/mutation_types.go:158` — `type TripleAppender interface {` | type | ownTest=0 admTest=0 other=0 |
| `EntityDeleter` | `pkg/projection/mutation_types.go:163` — `type EntityDeleter interface {` | type | ownTest=0 admTest=0 other=1 |
| `AuthoritativeReader` | `pkg/projection/mutation_types.go:168` — `type AuthoritativeReader interface {` | type | ownTest=0 admTest=0 other=2 |
| `WithMetricsRegistry` | `pkg/worker/pool.go:57` — `func WithMetricsRegistry[T any](registry *metric.MetricsRegistry, prefix string)…` | func | ownTest=0 admTest=0 other=0 |
| `Component.MergeEntity` | `processor/graph-ingest/component.go:2021` — `func (c *Component) MergeEntity(ctx context.Context, entity *graph.EntityState)…` | method | by-name check |
| `Registry.Instances` | `storage/storeregistry/storeregistry.go:106` — `func (r *Registry) Instances() []string {` | method | by-name check |
