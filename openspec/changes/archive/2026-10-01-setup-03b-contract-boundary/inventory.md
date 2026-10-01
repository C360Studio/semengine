# Inventory: setup-03b-contract-boundary

- base: `b89d967` (worktree `claude/setup-03b-contract`, one commit over `main` `34c9dc6`)
- semstreams pin: `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`)
- semstreams beta.161: `9d0ff67f377ea3dd82dca2f3bf614871c0100766` (cited only to explain closure deltas)
- semsource main: `48a317e` (pins `v1.0.0-beta.161`, `go.mod:8`); 03A evidence branch: `75a17f7d6297c3fa18102f3e09f8a0a4a10750bf`
- semconnect main: `d0d06e00bf05a545f30ceea798db1c2b1ee47d4f` (pins `v1.0.0-beta.160`, `go.mod:6`); PR #74 head `7f4887a5`

SemStreams paths are relative to the read-only snapshot
`scratchpad/semstreams-8b99efe9/` (its 4,679 files diff-identical to `git ls-tree -r 8b99efe9`; `diff` exit 0).
SemEngine paths are at `base`. SemSource paths are at `48a317e` unless marked `@75a17f7d`. SemConnect paths are at
`d0d06e00`. Every count below has its command; outputs are quoted, not paraphrased. Facts only; no target state.

## A1. Measured closure at the pin

Command (run in the snapshot root):

```text
go list -mod=mod -deps ./component ./config ./gateway/graph-gateway ./graph ./message ./metric ./model \
  ./natsclient ./payloadregistry ./pkg/buffer ./pkg/errs ./pkg/fusion ./pkg/fusion/fusionnats \
  ./pkg/fusion/fusionvocab ./pkg/projection ./pkg/retry ./pkg/types ./processor/graph-embedding \
  ./processor/graph-index ./processor/graph-ingest ./processor/graph-query ./service ./storage \
  ./storage/objectstore ./storage/storeregistry ./types ./vocabulary ./vocabulary/cco \
  | grep 'c360studio/semstreams' | sort -u | wc -l        → 65
```

Non-test lines (`find <pkg> -maxdepth 1 -name '*.go' -not -name '*_test.go' | xargs cat | wc -l`, summed) → **126,926**.
Both numbers equal Codex's `dependency-results.json` `pinned.port_production` (65 / 126,926); the 65-package list is
`diff`-identical (`IDENTICAL`). Codex ran the same 28 roots through `go list -mod=mod -modfile=<closure.mod> -deps
-json` from the SemSource module (`dependency-results.json` → `pinned.commands[4]`); running inside the SemStreams
module gives the same SemStreams-internal closure. The 28 roots are SemSource's 25 direct imports minus
`componentregistry` and `payloadbuiltins`, plus `processor/graph-{embedding,index,ingest,query}` and
`gateway/graph-gateway` (`pinned.port_production.roots`).

### A1.1 Why 63 became 65: five packages entered, three left

Set difference of Codex's `beta161.port_production.packages` vs `pinned.port_production.packages`:

| Direction | Package | Non-test lines at pin | Reached from (pin) | Commit |
| --- | --- | --- | --- | --- |
| in | `composition` | 914 | `service/component_manager.go:17`, `service/component_manager_http.go:15` | `78fe095c` feat(composition)!: validation substrate (#1101) |
| in | `internal/lifecyclecleanup` | 38 | every graph processor, `service`, `storage/objectstore` (replaces `internal/lifecyclejoin`) | `509cf8b2` refactor(research): restore one-shot lifecycle ownership |
| in | `internal/deliverylane` | 248 | `agentic/agentrun/agentrun.go:37`, `agentic/agentrun/milestone_settlement.go:12` | `b7ce8727` refactor(agentic): one home for the delivery-lane admission latch (#1357) |
| in | `internal/looptoken` | 42 | `agentic/user_types.go:11` | `0a40ddf3` feat(agentic)!: loop instance tokens are framework-minted UUIDs (#1210) |
| in | `pkg/projection/contract` | 163 | `payloadregistry/registry.go:29`, `pkg/projection/contract.go:3`, `agentic/{agent_lesson_entity,loop_execution_entity,payload_registry}.go` | `62f56d7e` feat(payloadregistry)!: single type authority (ADR-103) (#1109) |
| out | `engine` | — | directory absent at the pin (`ls engine` → No such file) | `b0a3f0b9` refactor(flow)!: retire the flow-authoring surface (ADR-100 D5) (#1116) |
| out | `flowstore` | — | directory absent at the pin | `b0a3f0b9` (same) |
| out | `internal/lifecyclejoin` | — | directory absent at the pin | `8da1b83a` refactor(lifecycle): remove unused lifecyclejoin |

At beta.161 `engine` was reached only via `service/flow_service.go:15` and `flowstore` via `engine/*.go` and
`service/flow_runtime_messages.go:11`, `service/flow_service.go:16` (`git grep` at `9d0ff67f`); both files are absent
at the pin. Net: 63 − 3 + 5 = 65; lines 126,596 → 126,926 (+330).

### A1.2 Import graph among the 65 (importers of the questioned packages)

`go list -mod=mod -f '{{.ImportPath}} {{join .Imports " "}}'` over the 65, reversed and filtered to the set:

| Package | Imported by (within the 65) | Exact import sites |
| --- | --- | --- |
| `agentic` (6,005) | `agentic/agentrun`, `component`, `gateway/graph-gateway`, `internal/agentterminal` | `component/dependencies.go:7` (used `:56-57` `agentic.ToolCall/ToolResult/ToolDefinition` in `ToolRegistryReader`); `gateway/graph-gateway/component.go:40` (used `:2093` `agentic.TrajectoryPage`, `:2105` `agentic.TrajectorySchemaV1`) |
| `agentic/agentrun` (1,505) | `service` | `service/milestone_service.go:10` (`agentrun.StartConfig`, `:23,:50,:64`) |
| `pkg/rulepack` (143) | `service` | `service/rule_pack_bind.go:8` (`rulepackcontract.ValidateID` `:96`) |
| `vocabulary/agentic` (2,133) | `agentic`, `agentic/agentrun`, `processor/graph-query` | `processor/graph-query/graphrag.go:22`; used only at `:1718-1720` in `labelPredicates` (`agvocab.IdentityDisplayName`, `CapabilityName`, `ModelName`) |
| `pkg/lifecycle` (3,739) | `agentic/agentrun`, `component`, `service` | `component/dependencies.go:12` (`Dependencies.LifecycleManager *lifecycle.Manager` `:99`); `service/component_manager.go:25`, `service/dependencies.go:13` |
| `composition` (914) | `service` | `service/component_manager.go:17` (`composition.Analyze` at `:402`, boot path `analyzeBootComposition` `:389`) |
| `graph/clustering` (5,516) | `processor/graph-query` | `processor/graph-query/{community_cache.go:14,component.go:17,graphrag.go:16,summary_view.go:11}` |
| `graph/llm` (842) | `graph/clustering`, `graph/inference`, `graph/query`, `processor/graph-query` | `processor/graph-query/answer.go:11`, `component.go:18`; `graph/query/classifier_llm_adapter.go:9`; `graph/inference/{config.go:12,review_worker.go:17}`; `graph/clustering/summarizer.go:12` |
| `graph/inference` (5,338) | `gateway/graph-gateway`, `processor/graph-ingest` | `processor/graph-ingest/component.go:19` (hierarchy, `:736`, `:1428-1442`; `EnableHierarchy` default false `:343,:408`); `gateway/graph-gateway/{openapi.go:6,component.go:44}` |
| `pkg/graphview` (1,193) | `processor/graph-query` | `component.go:24`, `summary_view.go:12` |
| `gateway` (319) | `gateway/graph-gateway` | base gateway interface |
| `internal/*` (7 pkgs) | see table in A1.1 and: `internal/graphmutation` ← `component/flowgraph`, `graph/inference`, `pkg/lifecycle`, `pkg/projection`, `processor/graph-ingest`; `internal/componentadmission` ← `component`, `service`; `internal/logforwarderpolicy` ← `service/log_forwarder.go:9`; `internal/agentterminal` ← `agentic/agentrun/agentrun.go:36` | — |
| `pkg/tlsutil` (533) → `pkg/acme` (543) | `metric` | `metric/handler.go:20` |

Callers of the two `service` files that pull `agentrun` and `rulepack`: `service.NewMilestoneService` is constructed
only at `internal/boot/run.go:573`; `service.ConfigureRulePackMutations` is called only at `internal/boot/run.go:330`
and `service.ComponentsImplementing` at `internal/boot/rule_config_service.go:92` (`grep -rn` over non-test files; no
other callers). Neither is in SemSource's import or call set (A3.2).

Subtotals from the per-package table (`my-port65-lines.txt`): agentic-reached set {`agentic`, `agentic/agentrun`,
`vocabulary/agentic`, `internal/agentterminal`, `internal/deliverylane`, `internal/looptoken`, `pkg/rulepack`} =
**10,264** lines; higher-tier graph libraries {`graph/clustering`, `graph/llm`, `graph/inference`,
`graph/structural`, `model/wire`, `pkg/graphview`} = **14,954**; `gateway/graph-gateway` + `gateway` = **2,995**;
`pkg/lifecycle` = **3,739**. Largest packages: `natsclient` 12,377; `service` 11,819; `processor/graph-query` 6,745.

## A2. Registration cut (#3): what the two registries do and what SemSource composes

### A2.1 The registries at the pin

- `componentregistry/register.go:81` `func Register(registry *component.Registry) error` → `registerProtocolLayer`
  (`:101`: udp, websocket input, file input, json_generic/filter/map, objectstore, file/httppost/websocket outputs,
  http gateway, lifecycle-gateway), `registerSemanticLayer` (`:163`: graph-ingest, graph-index, graph-gateway,
  graph-query, graph-embedding, graph-clustering, graph-index-spatial, graph-index-temporal, rule, gated-dag),
  `registerAgenticLayer` (`:215`: agentic-dispatch/governance/model/tools/loop).
- `payloadbuiltins/register.go:36` `func Register(reg *payloadregistry.Registry) error` tracks seven owners
  (`:44-52`): `message`, `agentic`, `gated-dag`, `objectstore`, `governance`, `pkg/lifecycle`, `graph/inference`.
  Owners inside the 65: `message/generic_json.go:28`, `agentic/payload_registry.go:26`,
  `storage/objectstore/stored_message.go:92`, `pkg/lifecycle/harness_entity.go:65`,
  `graph/inference/container_entity.go:62`. Outside: `processor/gated-dag/payload.go:108`, `governance/verdict.go:132`.
- ADR-103 (`docs/adr/103-payload-registry-is-the-single-type-authority.md:31-55`): the registry is the single type
  authority; graph-ingest rejects an unregistered stamp (`message_type_unregistered`); a product registers its types
  in the binary that hosts graph-ingest. graph-ingest's construction check: `processor/graph-ingest/component.go:735-741`
  requires `inference.HierarchyContainerMessageType()` registered only when `config.EnableHierarchy` (default false,
  `:343`, `:408`).

### A2.2 SemSource's registration and composition (`cmd/semsource/run.go` at `48a317e`)

- Imports: `componentregistry` `:47`, `payloadbuiltins` `:51`. Calls: `componentregistry.Register(registry)` `:273`
  inside `registerComponentFactories`; `payloadbuiltins.Register(reg)` `:289` inside `buildPayloadRegistry`, followed by
  `graph.RegisterPayloads(reg)` (SemSource's own `graph/event_payload.go:21`) and `sourcemanifest.RegisterPayloads`.
  The 03A branch still imports both (`run.go@75a17f7d:48,52,294,310`); `compatibility.md:109-110`: "The broad built-in
  registries remain imported; this is not yet SemEngine's selective extraction boundary."
- SemStreams component types SemSource composes (`graphSubsystemComponents`, `run.go:739-958`; `websocketComponentConfig`
  `:1013-1061`): `graph-ingest` `:744`, `graph-index` `:785`, `graph-embedding` `:797`, `graph-query` `:827`,
  `graph-gateway` `:852` (type Gateway), `objectstore` `:902` (type Storage), `websocket` output `:1060-1061`, and
  `graph-clustering` `:932-940` only when `graph.enable_clustering` is set.
- Of those, **`output/websocket` and `processor/graph-clustering` are not in the 65** (`grep -c processor/graph-clustering
  my-port65.txt` → 0; `output/websocket` absent) — they reach SemSource's binary only through `componentregistry.Register`.
- SemSource's own factories (`registerSemsourceFactories`, `:301-320`): ast-source, git-source, doc-source,
  cfgfile-source, url-source, objectstore-source, image/video/audio-source, filestore, source-manifest, code-context,
  mcp-gateway, supersession.
- graph-gateway is composed with three **required** requester ports (`run.go:862-890`): `graph_queries`
  (`graph.query.*`), `graph_index_queries` (`graph.index.query.*`), `agentic_queries` (`agentic.query.*`, interface
  `agentic.query v1`). graph-gateway declares the same at `gateway/graph-gateway/component.go:143-145,158-160` and
  enforces the interface at `:221-222`. SemSource composes no responder for `agentic.query.*` (no agentic component
  in its composition; `grep '"agentic-'` over its non-test code → none).
- GRAPH stream: subjects `graph.ingest.{entity,batch,manifest,status,predicates}`, `Storage: "memory"`,
  `MaxBytes 256 MiB`, `MaxAge 1h`, `Discard: new` (`run.go:961-987`), overridable per field (`:989-1005`).
  The websocket output's input port filters `graph.ingest.>` on GRAPH as a **consumer** filter (`run.go:1043`), not a
  stream subject.
- Symbols SemSource consumes (`grep -rhoE` over non-test files): `service.{Manager, Dependencies, RegisterAll,
  NewServiceRegistry, NewServiceManager, ComponentManager, MaybeStartPProf}`; `semconfig.{Config, Manager, SafeConfig,
  StreamConfigs, StreamConfig, StreamDiscardNew, PlatformConfig, ComponentConfigs, NewSafeConfig, NewStreamsManager,
  NewConfigManager, NATSConfig, JetStreamConfig}`; `component.{Port, Dependencies, Discoverable, PortDefinition,
  RegistrationConfig, Metadata, HealthStatus, FlowMetrics, PortConfig, JetStreamPort, GenerateConfigSchema, ConfigSchema,
  PlatformMeta, Direction*, NATSRequestPort, InterfaceContract, Registry, KVWritePort, NewRegistry, NetworkPort,
  LifecycleComponent}`. `service.RegisterAll` registers metrics, message-logger, log-forwarder, metrics-forwarder,
  component-manager, heartbeat, storage-observability (`service/register.go:8-22`); not milestone, not rule.

### A2.3 SemConnect's registration

Production imports (A11) include neither registry. Tests use `payloadbuiltins.NewTestRegistry(t)`
(`gateway/cs-api/nats_integration_test.go:207`, `gateway/cs-api/observations_test.go:110`,
`message/oms/roundtrip_test.go:67`) and construct graph-ingest directly
(`gateway/cs-api/beta160_structural_integration_test.go:18,122,144`). PR #74's proposal states SemConnect runs against a
"stock backend binary" and must add "a SemConnect-owned backend composition root" to register its eleven payload types
under ADR-103 (`openspec/changes/migrate-semstreams-setup03a/proposal.md` "What changes", bullet 3, at `7f4887a5`).

## A3. Fusion and graph-tool seam

### A3.1 Two `Fuse` entry points in `pkg/fusion`

- Package-level `fusion.Fuse(ctx, gq GraphQueryClient, queries []SubQuery, opts FuseOptions, logger)`
  (`pkg/fusion/engine.go:96`) — the sub-query fan-out engine; `doc.go:1-10` names `processor/research-graph-execute` as
  its composing caller. SemSource calls it nowhere (`grep -rn 'fusion\.Fuse('` → none).
- Lens-driven `(*Engine).Fuse(ctx, req Request, lens Lens) (Response, error)` (`pkg/fusion/engine_lens.go:123`);
  `NewEngine(graph RetrievalClient, body *BodyResolver)` `:77`, `WithSignals` `:85`, `WithMetrics` `:102`. SemSource
  constructs two engines at `processor/code-context/component.go:209,215` and calls `Fuse` at `:294`; its lenses
  implement `fusion.Lens` at `source/fusion/lens/code/code.go:21` and `source/fusion/lens/docs/docs.go:40`; its NATS
  retrieval client is `fusionnats.New(deps.NATSClient, 0)` (`component.go:135`).
- `Lens` SPI methods (`pkg/fusion/lens.go:144-199`): `Name`, `ResolveMode(query)`, `Edges`, `Label`, `Kind`, `Location`,
  `Hydrate(ctx, e) (*message.StorageReference, error)`. `ResolveMode` values (`lens.go:78-95`): `nl`, `symbol`, `prefix`.
- `RetrievalClient` (`pkg/fusion/retrieval.go:17-47`): `Status`, `Resolve(ResolveQuery) ([]Seed, error)`, `Entity`,
  `Entities(ids) (Hydration, error)` (order-preserving contract `:41-45`), `Neighbors`, `Names`.
  `fusionnats` request subjects (`pkg/fusion/fusionnats/client.go:26-31`): `graph.query.{byName,prefix,semantic,entity,
  batch,relationships}`; readiness via `graph/readiness.Watcher` (`:115`).
- Facets computed by the engine: `computeImpact` `engine_facets.go:50`, `computePaths` `:110`, `computeGraph`
  `engine_graph.go:158`; budget `contract.go:313-331`; `notReadyEnvelope` `engine_lens.go:305`; `miss` `:340`.

### A3.2 The `graph.query.searchGraph` path

- SemSource `graph_search` MCP tool (`processor/mcp-gateway/component.go:124`) → `graphSearch` → request
  `graph.query.searchGraph` with `summarize_threshold: 1` (`processor/mcp-gateway/query_tools.go:99-107`).
  `code_context/code_impact/code_search/doc_context` go to SemSource's own fusion over HTTP/NATS
  (`query_tools.go:60-73`, verbs `code.v1.impact`, `code.v1.search`, `docs.v1.context`).
- Handler at the pin: `processor/graph-query/query.go:63` operation `searchGraph` → `handleSearchGraph`
  (`processor/graph-query/searchgraph.go:56-66`) = `handleGlobalSearchWithLease` (GraphRAG: classifier → strategy →
  semantic → community) then, on empty, `querySemantic` fallback (`:68-110`; `Strategy = "semantic_fallback"`, `:26`).
  Declared consumers: `graph-gateway`, `research-graph-classify`, `research-graph-execute` (`query.go:63`).
- graph-query's 16 operations: `entity, entityByAlias, batch, relationships, pathSearch, hierarchyStats, prefix,
  spatial, temporal, semantic, similar, globalSearch, summary, searchGraph, byName, localSearch` (`query.go:50-65`);
  subject family `graph.query.*` (`query.go:21`). Its `StaticRouter` (`router.go:16-42`) routes to
  `graph.ingest.query.*`, `graph.index.query.*`, `graph.spatial/temporal.query.*`, `graph.embedding.query.*`,
  `graph.clustering.query.community`, `graph.anomalies.query.detect`.
- graph-query's LLM client: `llm.Client` field `component.go:137`, built `:432-434,:465-467`; `LLMAnswerSynthesizer`
  `answer.go:123-193`.
- `find/anchor/ask`: not a code symbol at the pin or in SemSource (`grep -rn '"find"\|"anchor"\|"ask"'` → only a
  stop-word table `graph/embedding/bm25_embedder.go:37`). The phrase is SemStreams issue #1422's title ("find by
  spelling, look up by anchor, ask the graph"), labelled `area:agentic`, `status:needs-decision`, `status:blocked`.

### A3.3 What the 03A workload exercised (`test/setup03a/*_test.go` @75a17f7d)

HTTP fusion through SemSource's code-context server `POST {httpURL}/{lens}-context/{verb}`
(`qualification_test.go:597`); NATS `graph.query.{status,entity,byName,prefix,searchGraph}`
(`qualification_test.go:132,555,630,184`; `removal_test.go:80`), `graph.ingest.query.entity` (`:949`),
`graph.ingest.entity` publish (`:819`), SemSource's own `graph.lifecycle.run` (`:796`; `graph/lifecycle.go:17`).
`gateway_bind` is set to a free port (`:384`) and **no assertion addresses it** (`grep gateway` → that one line).
graph-gateway is composed but not exercised by the retained workload.

## A4. RPC request/reply planes in the port set (input to #16)

| Subject family | Served by (file:line) | Transport |
| --- | --- | --- |
| `graph.ingest.query.{entity,batch,prefix,suffix}` | `processor/graph-ingest/query.go:27,34,41,48` (`SubscribeForRequests`) | core NATS |
| `graph.mutation.>` | `processor/graph-ingest/mutation_runtime.go:27`; family constant `internal/graphmutation/protocol.go:16` | core NATS |
| `graph.index.query.{outgoing,incoming,alias,predicate,predicateList,…}` | `processor/graph-index/query.go:31-59` | core NATS |
| `graph.embedding.query.{similar,search,status}` | `processor/graph-embedding/query.go:24,31,39` | core NATS |
| `graph.query.*` (16 ops) | `processor/graph-query/query.go:21,50-65` | core NATS |
| `agentic.query.*` | requester only, `gateway/graph-gateway/component.go:145,160`; no responder in the 65 | core NATS |
| `storage.objectstore.{write,events,stored}` | `storage/objectstore/config.go:78-89` (`NATSPort`, publish/subscribe, not request) | core NATS |

Hard-coded `graph.ingest.query.*` literals at the pin (`grep -rn '"graph\.ingest\.query\.' --include='*.go' .`,
non-test): `processor/graph-ingest/query.go:27,34,41,48,55`; `processor/graph-query/router.go:19-21`;
`processor/graph-query/entity_resolver.go:102`; `graph/exact_entity.go:15`; outside the 65:
`processor/agentic-loop/lessons.go:20`, `processor/gated-dag/reader.go:64`. No symbol spells "reserved"
(`grep -rn 'reserved\|Reserved' processor/graph-ingest/query.go composition/*.go` → none). Composition already receives
stream configs: `composition.Analyze(declarations, streams config.StreamConfigs)` `composition/analyze.go:23`;
`explicitStreamCovers(streams, streamName, subjects)` `:114`.

## A5. Lifecycle debt against port-set packages at the pin (ruling 4 inputs)

| Issue | State (2026-10-01) | Measured at the pin |
| --- | --- | --- |
| SS#1147 epic restart behavior | open | children #759 and #1146 **closed** (beta.163, settlement foundation and first process-replacement vertical); #1145 and #821 open; no port-set-specific code claim |
| SS#1145 declare/prove restart | open | `component/lifecycle.go:63-68` `LifecycleComponent{Discoverable; Initialize; Start(ctx); Stop(ctx)}`; no restart declaration (`grep -ci restart component/registry.go component/lifecycle.go` → 2, 2, all prose) |
| SS#1411 `lifecycleUsed` copy | open | repo-wide 29 non-test files; **7 in the 65**: `gateway/graph-gateway/component.go`, `processor/graph-embedding/component.go`, `processor/graph-ingest/component.go`, `processor/graph-query/component.go`, `service/component_manager.go`, `service/message_logger.go`, `storage/objectstore/component.go` (graph-index: 0) |
| SS#1415 root services outside `service.Service` | open | `config/manager.go:317` `Start(ctx context.Context) error`; `:478` `Stop(timeout time.Duration) error`; adapter `internal/boot/run.go:664` `stopWithinShutdownBudget`, called `internal/boot/resources.go:37` |
| SS#1417 unbounded cleanup roots | open, pre-v1 backlog | `test/testinfra/cleanup_baseline.json`: 234 entries / 97 resolutions at the pin (334 at filing); **105 in the 65**: graph-gateway 32, graph-index 27, service 20, graph-embedding 13, agentrun 4, pkg/dispatch 4, objectstore 4, pkg/lifecycle 1 |
| SS#1218 two `ErrAlreadyStopped` | open | `service/base.go:28` `"service already stopped"`; `pkg/errs/errs.go:47` `"component already stopped"`; `pkg/errs/doc.go:81` advertises the latter under component lifecycle; `service/service_manager.go:888` filters only `service.ErrAlreadyStopped` |
| SS#1219 StopAll property reach | open, rc.1 | test-only gap in `service/service_manager_prop_test.go` |
| SS#1220 registry clear mode-independent | open | `service/service_manager.go:929` `m.services = make(map[string]Service)` inside `stopAll`; entered from `StopAll` and `cleanupFailedStart` `:494` with mode `managerCleanupFailedStart` `:490` |
| SS PR #1437 (fixes #1112) | open, mergeable, **after the pin** | touches two port-set packages: `processor/graph-ingest/component.go` (+30/−5, `decodeEntity` runs decode → `Validate()` → extraction under one `recover`) and `agentic/loop_execution_entity.go` (+40/−8); breaking contract tightening (a decoded payload failing `Validate()` is poison). Not in the pin; a ledger-row candidate under ruling 3 (`gh pr view 1437 -R C360Studio/semstreams --json files`) |
| SS PR #1361 (`c4a79fd5`, restart-safety L4a) | merged 2026-09-23, **in the pin** (`git merge-base --is-ancestor c4a79fd5 8b99efe9` → true) | precedent for recovering redelivered inputs from durable applied facts on the owning entity (agentic-loop); the SS#1147 hierarchy's item 2 shape, not a new bucket |
| SemSource PR #213 | merged `3604a9ce` (+30,824/−2,460 incl. evidence) | the consumer design #18–#20 serve: consumer-private source-lifecycle machinery (durable fences, `applied_tail_unproven`, `conditional_reconcile_unavailable`); the inventory review reports about 2,600 lines of it (not re-measured here) |

## A6. Repair and decision issues: the code at the pin

- **#15 / SS#1442.** `processor/graph-ingest/keyed_ingest.go:265` and `:282` `return work.seq <= last, nil` inside
  `ingestGuardStale` (`:261`), keyed by `guardKey(work.entityID, work.stream)` `:262`; durable tier
  `c.ingestGuardBucket` created at `component.go:1243` `c.natsClient.NewKVStore(guardBucket)`. No stream-generation
  identity in the key or the comparison. Observed at both pins: `broker_restart_reingested_exact_relationship` and
  `broker_restart_reingested_exact_content` fail (`pinned-results.json` profiles bm25 61/63, neural 65/67;
  `restart_guard` applied 33 vs restarted last 20).
- **#16 / SS#1143.** A4 above. Observed: `rpc_wildcard_collision_reproduced` true in both profiles; SemSource's
  explicit subject list (`run.go:961-968`) works.
- **#17 / SS#1443.** `config/manager.go:1315` `syncFromKV`: resets only `current.Services` (`:1326-1329`), then
  overlays each surviving KV key through `updateConfig` (`:1352`); Components are neither reset nor tombstoned.
  `DeleteComponentFromKV` `:782` deletes key `components.<name>` (`:787-788`) and applies the in-memory delete.
  03A removal supplement: 9/10, same-handle re-add works, markers missing (`compatibility.md:244-251`).
- **#18 / SS#1444.** `graph/index_status.go:263-286` `BacklogStatusInputs.Outstanding` doc: sum of NumPending +
  NumAckPending; "A MaxDeliver-parked message leaves BOTH counters"; "Caught-up … cannot license an absence claim".
  `component/port_jetstream.go:130` `MaxDeliver: 3` default, `:148` replaced only when `> 0`; `component/port_codec.go:382`
  rejects negative `max_deliver`. No census primitive exists (`GRAPH_INGEST_APPLIED_SEQ` is the per-(entity,stream)
  latest sequence, A6 #15).
- **#19 / SS#1445.** `pkg/projection/mutation_types.go:43-49` `ReconcileMutation{Contract, Group, EntityID, Desired,
  Metadata}` — no expected revision. `pkg/projection/mutation_client.go:191` `exact, err := c.ReadAuthoritative(ctx,
  request.EntityID)`; `:199` `ExpectedRevision: exact.KVRevision`. The wire type carries it:
  `graph/mutation_requests.go:27-34` `ReconcilePredicatesRequest.ExpectedRevision uint64`. `internal/graphmutation/
  protocol.go:1-2`: "Application callers use narrow typed clients rather than subjects directly."
  **SemConnect at `d0d06e00` sends the wire request itself with the revision it read**:
  `gateway/cs-api/graph_mutations.go:214-215` `graph.ReconcilePredicatesRequest{EntityID: exact.Entity.ID,
  ExpectedRevision: exact.KVRevision, …}` and `:275-276` for delete, over its declared `graph.mutation.>` port
  (`component.go:222-224`); it uses `projection.Contract` for validation (`projection_contracts.go:42-162`) but not
  `projection.MutationClient`.
- **#20 / SS#1446.** Client: `CommitUnknown` exists (`pkg/projection/mutation_types.go:74`); `mutationFailure`
  (`mutation_client.go:404-409`) returns `unknownMutation` unless `isDefiniteFailure(err)` (`:419-422`), which is true
  for **any** `*errs.ClassifiedError`, `graphmutation.IsDefinitelyNotCommitted`, or no-responders. Server:
  `processor/graph-ingest/canonical_mutations.go:349` `entityBucket.Update(...)`; a failure that is neither revision
  mismatch (`:351`) nor not-found (`:356`) goes to `rejectFromError(err)` (`:360`) → `rejectInternal`
  (`mutation_runtime.go:218` → `:214-216`, `errs.ClassifiedCode(errs.ErrorTransient, ErrorCodeInternal, err)`). A backend
  write timeout therefore arrives at the client as a classified error and is reported `CommitNotCommitted`.
  **SemConnect misreports the same case the same way**: `gateway/cs-api/graph_mutations.go:113-117` passes a classified
  reply (`natsclient.ClassifyReply`) to `mutationFailure` and audits it `"not-committed"`; `systems_post.go:465-469`
  maps `graph.ErrorCodeInternal` (what `rejectInternal` sends) to "graph backend mutation failed" (HTTP 500).
  `commitUnknownError` (`:108`, `:178`, `:229`, `:288`) covers only a transport error on the request (`:108-111`) and a
  malformed or unexpected response (`:177-183`, `:228-233`, `:287-293`); `systems.go:1034-1042` returns HTTP 503 with
  `X-CS-Commit-Uncertain: true` and "verify resource state before retrying" — no re-read, no fence. No consumer
  resolves an unknown outcome today; SemConnect bypasses `pkg/projection` and carries its own copy of the defective
  definite-failure assumption.

## A7. ADR-106 and the sister-import freeze (#2 inputs)

- ADR-106 `docs/adr/106-…md:1-148` read in full. Tier 1 = `release/tier1-packages.txt` (62 entries, `:52-57`), the 33
  schemas, the payload envelope, entity-ID grammar, subject namespace. Ruling 5 (`:76-88`): an incompatible Tier 1 change
  forced by a sister migration resets to a new RC; compatible additions pass RC-6. Ruling 6 (`:90-98`): canaries
  semteams, semsource, semmachina. Consequences (`:122-125`) already name `engine`/`flowstore`/`flowtemplate` as
  migration debt retired by #1116.
- Cross-check (`comm` of normalized `tier1-packages.txt` vs the 65): **39 in both**; **23 Tier 1 not in the 65**
  (`componentregistry`, `payloadbuiltins`, `gateway/lifecycle-gateway`, `graph/geo/geojson`, `input/websocket`,
  `output/websocket`, `persona`, `pkg/context`, `pkg/logging`, `processor/agentic-*` ×7, `processor/gated-dag`,
  `processor/graph-clustering`, `processor/rule`, `processor/rule/expression`, `test/e2e/mock`, `vocabulary/builtins`,
  `vocabulary/export`); **26 in the 65 not in Tier 1** (`composition`, `graph/{embedding,inference,llm,query,structural}`,
  `health`, seven `internal/*`, `pkg/{acme,cache,dispatch,platform,projection/contract,resource,revlag,rulepack,
  security,timestamp,tlsutil,worker}`).
- SS#1177 (beta.163 breaking wave) is open, milestone `v1.0.0-beta.163`; no `v1.0.0-beta.163` tag exists
  (`git tag -l 'v1.0.0-beta.16*'` → `v1.0.0-beta.162` only; `git describe 8b99efe9` → `v1.0.0-beta.162-96-g8b99efe9`).
  The pin carries `taskfiles/apicompat.yml` (RC-4 instrument).

## A8. SemEngine constraints that bind ported code

- T-B1 (`internal/harness/contract/imports_test.go:12-16,52-60`): production files may not import `testing`,
  `testcontainers-go`, `gopkg.in/yaml.v3`, or `internal/harness`. Port-set production files that do at the pin:
  `component/lifecycle_test_suite.go:11` (`testing`), `composition/assert.go:4` (`testing`),
  `natsclient/test_client.go:10,16,17` (`testing`, testcontainers ×2), `payloadregistry/testing.go:4` (`testing`).
- Context roots in port-set production files (`grep -c 'context\.Background()\|context\.TODO()'`): **34** total —
  natsclient 9, component 5, service 3, pkg/worker 2, pkg/errs 2, pkg/buffer 2, metric 2, graph/query 2, config 2,
  objectstore 1, pkg/dispatch 1, pkg/cache 1, model 1, graph-gateway 1 (plan's "about 66" was the 46-package row at
  beta.161). Struct fields typed `context.Context` in port-set production files: **0**
  (`grep -nE '^\s+\w+\s+context\.Context\s*(//.*)?$'` → exit 1).
- `docs/admission-ledger.yaml:1-18` schema (ten fields; dispositions `carry | adapt | repair-before-port |
  defer-exclude`); 12 rows at `5457b345`; `component/lifecycle_test_suite.go` (adapt), `internal/lifecyclecleanup/
  lifecyclecleanup.go` (adapt), `natsclient/test_client.go` (adapt) already have rows.
- `openspec/specs/` holds `harness-boundaries`, `integration-test-runner`, `lifecycle-suite`, `nats-fixture`; no graph
  capability spec exists. `scripts/openspec-queue.sh:80-85`: an unchecked task line containing the word `hold`,
  `blocked`, or `blocking` is reported `BLOCKED`; `halt*` → `HALT`.

## A9. 03A observed baseline (the matrix's "observed" column)

Both profiles at the pin (`pinned-results.json`, binary `7847df90…`, corpus `9724050f…`): bm25 61/63 (slices [0,1]),
neural 65/67 (slices [0,2]). Observation names (identical set except neural's `provider_metrics_*` and
`neural_paraphrase`): `ingestion_ready`, `governed_identity_authority`, `initial_{persisted_authority, entity_run,
entity_greet_alpha, entity_greet_beta, entity_farewell, exact_relationship, provenance, exact_content}`,
`{cold,warm}_{duplicate_name_anchors, exact_absence, code_scope_before_limit, doc_scope_before_limit,
doc_exact_passage_content}`, `graph_search_public_query`, `edited_*` (8), `deleted_{lifecycle_pass,
retains_stale_history, query_visibility}`, `recreated_{lifecycle_pass, same_identity_not_stale, …}` (10),
`application_restart_*` (8), `accepted_unindexed_before_broker_restart`, `memory_transport_not_durable`,
`authority_survives_broker_restart_before_reingest`, `content_survives_broker_restart_before_reingest`,
`broker_restart_reingested_*` (10; **two fail**), `rpc_wildcard_collision_reproduced`. Provider fault supplement 6/6.
Provider identity (`pins.json`): semembed image `sha256:7972174f…`, build `7ceb5281`, model
`Snowflake/snowflake-arctic-embed-s`, artifact `e596f507`, ONNX `579c1f17…`, 384 dims, query prefix
"Represent this sentence for searching relevant passages: ", document prefix empty; same container both runs.
Broker `nats:2.14.4-alpine@sha256:f2123f53…`. Capacity: 32,720 offers, GRAPH 239,366,950 → 244,999,958 bytes under
268,435,456 (`compatibility.md:255-265`); historical #178 (77,802) unresolved. Unavailable: MinIO-backed governance
integration tests (`review.md` "Check inventory"). Removal supplement 9/10; `source_removed` markers absent at both pins.

## A10. SemConnect import surface and port-set delta

- Production imports at `d0d06e00` (`git grep -h -oE '"github.com/c360studio/semstreams/[^"]*"' -- '*.go'
  ':!*_test.go' | sort -u` → 13): `component`, `gateway`, `graph`, `graph/geo/geojson`, `graph/readiness`, `message`,
  `natsclient`, `payloadregistry`, `pkg/errs`, `pkg/projection`, `pkg/types`, `vocabulary`, `vocabulary/export`.
  Test-only additions: `config`, `payloadbuiltins`, `processor/graph-ingest`.
- Outside the 65: **`graph/geo/geojson`** (1,030 lines; imports no SemStreams package) and **`vocabulary/export`**
  (1,107 lines; imports `message`, `vocabulary`, both already in the set). Closure with both added: **67 packages /
  129,063 lines** (`go list -mod=mod -deps <28 roots> ./graph/geo/geojson ./vocabulary/export` → 67; added lines
  1,030 + 1,107). Test-only `payloadbuiltins` would add `governance` and `processor/gated-dag` (A2.1) — not measured
  as production.
- Seams exercised (call sites): typed exact-revision mutations over the raw `graph.mutation.>` port
  (`gateway/cs-api/graph_mutations.go:165,214-215,275-276`; port `component.go:222-224`); projection contracts
  (`projection_contracts.go:42-162`, `projection.ValidateContracts` `:162`); `message.StorageReference` for immutable
  schema artifacts (`schema_artifacts.go:51`, `graph_mutations.go:136`); JSON-LD export (`systems.go:746`
  `export.Serialize(&buf, state.Triples, export.JSONLD)`) and vocabulary registration (`vocabulary/csapi/register.go:16`
  `export.Register(Prefix, Namespace)`); GeoJSON (`spatial.go:150-163`); `gateway.Gateway` (`component.go:195`);
  readiness bucket/key constants (`conformance/cmd/index-readiness/main.go:31-38`). Commit classification is
  SemConnect's own (`graph_mutations.go:84-120`): transport error and malformed response → `commitUnknownError`;
  any classified reply, including `ErrorCodeInternal` from an uncertain backend write → "not-committed" (A6 #20).
- Reference fixture files exist at `d0d06e00`: `parser/sensorml/graphable_test.go` (10,594 B),
  `gateway/cs-api/beta160_structural_integration_test.go` (6,986 B), `gateway/cs-api/schema_artifacts_test.go`
  (15,548 B), `vocabulary/csapi/register_test.go` (3,868 B).
- PR semconnect#74 (draft, head `7f4887a5`, branch `codex/migrate-semstreams-setup03a`): migrates to the pin; local
  checkout `/Users/coby/Code/c360/semconnect` is on that branch with 7 dirty files (not touched). Its `go.mod` still
  reads beta.160 at that head. No before/after results are recorded yet (`openspec/changes/migrate-semstreams-setup03a/`
  holds proposal, tasks, two pins files only).

## A11. Same-class collision table

| Semantic job | Existing owners at the pin | Catalogs / status | Lifecycle & recovery | Readers / writers |
| --- | --- | --- | --- | --- |
| Redelivery/replay guard per entity (#15) | `GRAPH_INGEST_APPLIED_SEQ` KV, `keyed_ingest.go:261-282`, bucket `component.go:1243`; mem tier `ingestGuardMem` | none operator-visible; `MAX_DELIVERY_EVENTS` ledger (SS#742) is occurrence-only | survives broker restart; no generation identity; stream recreation resets sequences | writer: graph-ingest lanes; reader: same |
| Reserved subject declaration (#16) | none (A4: literals in 4 port-set files) | `composition.Analyze` findings `composition/analyze.go:23`; `explicitStreamCovers` `:114` | boot-time only | writer: none; readers: composition at boot |
| Desired-config deletion durability (#17) | `config/manager.go:782` delete; `syncFromKV :1315` overlay | KV bucket `config.BucketName(org, stem)` (ADR-104) | next-boot merge; no tombstone | writer: Manager; reader: Manager at Start |
| Applied-input completeness (#18) | none; `BacklogStatusInputs.Outstanding` `graph/index_status.go:263` is a backlog claim | `GRAPH_STATUS` readiness bucket (`graph/readiness`) | n/a | readers: fusionnats status watcher `fusionnats/client.go:115`; SemConnect `index-readiness` |
| Conditional reconcile at observed revision (#19) | wire: `graph/mutation_requests.go:29`; raw caller: SemConnect `graph_mutations.go:214`; typed client re-reads: `mutation_client.go:191,199` | — | — | two consumer spellings (A6 #19) |
| Commit-outcome classification (#20) | `CommitState{not-committed, unknown, verified}` `mutation_types.go:68-76`; `isDefiniteFailure` `mutation_client.go:419`; server `rejectFromError` `mutation_runtime.go:218` → `rejectInternal` `:214-216` | — | no terminal-status lookup by request ID (`RequestID` carried `graph/mutation_requests.go:33`; graph-ingest echoes it into responses at `canonical_mutations.go:293,336,365,398,507` (`grep -n RequestID processor/graph-ingest/*.go`, non-test → 5 echo sites, no store or lookup)) | SemConnect's own classifier (`graph_mutations.go:113-117`) repeats the definite-failure assumption; no consumer resolves unknown |

## A12. Adopter seam inventory (surfaces reached from outside this repo)

1. **Registration** (`componentregistry.Register`, `payloadbuiltins.Register`). What the adopter must know today:
   that the two aggregator imports pull 94 and 41 SemStreams packages at the pin (`go list -mod=mod -deps
   ./componentregistry | grep -c c360studio/semstreams` → 94; `./payloadbuiltins` → 41; issue #3's 91/50 were
   beta.161 figures); that graph-ingest refuses to construct without the hierarchy
   type only when hierarchy is enabled (`component.go:735-741`); which payload owners its composition needs (A2.1).
   If they do nothing: they import both aggregators (SemSource does, `run.go:47,51`) and carry the agentic/rule/research
   closure. Found out at: compile time only for missing types (ADR-103 rejects at ingest with
   `message_type_unregistered`, a typed runtime error).
2. **Fusion Lens SPI** (`fusion.Lens`, 7 methods). Must know: `Entities` order contract (`retrieval.go:41-45`), budget
   defaults (`engine.go:17-44`), that `Hydrate` returns a `StorageReference` resolved via `StoreRegistry`
   (`component/dependencies.go:101-112`, "content-unresolved and excludes the body" when the instance is unknown).
   If they do nothing: a missing `StoreRegistry` yields bodies silently excluded with a reason, not an error
   (`engine_lens.go:443` `reportBodyFailure`). Found out at: log/metric.
3. **Stream subjects for the ingest plane** (#16). Must know: the four reserved subjects. If they do nothing and bind
   `graph.ingest.>`: reads return zero results, no error (SS#1143). Found out at: nowhere (the SemSource test pins the
   list by hand). This is a predict-not-observe seam.
4. **Projection mutation client** (#19, #20). Must know: `Reconcile` re-reads and fences on its own read; a classified
   error means not-committed even when the backend effect is uncertain. If they do nothing: an invalid freshness
   grant or a re-add admitted over an uncertain delete. Found out at: nowhere (post-check only).
5. **Desired-config removal** (#17). Must know: deleting a file-declared component's KV key lets it return; the
   workaround is a retained `Enabled:false` envelope (`compatibility.md:87-92`). Found out at: next boot, by observing
   the component start.
6. **GRAPH durability** (plan "Consumer qualification evidence"). Must know: memory storage; PubAck is not durability
   (`memory_transport_not_durable` passes as a *recorded* fact). Found out at: broker restart.

## A13. Not established

- Per-package test coverage at the pin for the critical-package list: the tracked `coverage.out` is a stale
  2025-12-26 profile of one retired package (`head -2 coverage.out` → module path `github.com/c360/semstreams`,
  `processor/graph/gateway/mcp`); establishing it needs `go test -cover` over the 65 (not run here).
- SemConnect before/after results at the pin: PR #74 has recorded none yet.
- Whether `graph-gateway`'s GraphQL/MCP HTTP surface is used by any SemSource operator path outside `test/setup03a`
  (only the composition and `gateway_bind` plumbing were found; a runtime trace was not taken).
- Whether SemSource's `output/websocket` component is part of the retained workload (composed at `run.go:1060`; no 03A
  assertion addresses it).
- The exact `MaxDeliver` SemSource's graph-ingest consumer runs with (declared ports at `run.go:756-764` do not set it;
  default 3 per `port_jetstream.go:130`) — a `nats consumer info` read would establish it.

## Changelog (post inventory-review, 2026-10-01)

- **B1 (blocking) corrected.** A6 #20, A10, A11 row #20: SemConnect does not classify the backend-write-uncertain case
  as unknown and does not resolve unknown; `graph_mutations.go:113-117` audits any classified reply "not-committed",
  `systems_post.go:465-469` maps `ErrorCodeInternal` to a 500, `commitUnknownError` covers only transport errors and
  malformed responses, `systems.go:1034-1042` returns 503 "verify resource state before retrying". No consumer resolves
  unknown today. (`design.md` D8 and owner question Q7, #8 comment 5929442167, re-derived on these premises.)
- A5: SS#1147 children #759 and #1146 are closed (beta.163); #1145 and #821 open.
- A5 (new rows): SS PR #1437 (open, after the pin, two port-set packages); SS PR #1361 `c4a79fd5` (L4a precedent, in
  the pin); SemSource PR #213 (merged `3604a9ce`, the consumer design #18–#20 serve).
- Counts: seven `internal/*` packages (A1.2, A7), `processor/agentic-*` ×7 in the Tier-1-only list (A7).
- Line drift: `mutation_runtime.go` `rejectInternal` `:214-216`, `rejectFromError` `:218`; `keyed_ingest.go`
  `ingestGuardStale` `:261`, `guardKey` `:262`; `config/manager.go` `updateConfig` `:1352`.
- Design/questions (outside this file): D6 gains option (iii) durable applied facts on the owning entity (L4a); D7 is
  placed on SS#1147's hierarchy and names SemSource PR #213; D4/Q4 restate the dormant-bridge alternative as "owner and
  exit condition" per `setup-plan.md:412-414`; Q4 names the fifth agentic edge (`gateway/graph-gateway/component.go:40`);
  Q12 gives 68 entries / 6 copies for the tier-0 set alongside 105 / 7 for the 65; Q13 added for SS PR #1437;
  `setup-plan.md:103` for the export-mechanism citation; `openspec-draft/.openspec.yaml` carries `skip_specs: true`;
  tasks 2.1–2.4, 4.1–4.3, 4.5 carry first-line holds on the #8 question they depend on.
