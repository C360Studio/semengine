# SETUP 03B scope inventory

Rules, workflows, settlement, change observation, storage classes.

Copied into this change on 2026-10-01 from the architect's reviewed scratch file `scope-inventory.md` (with its
changelog). Edits are mechanical only: prose rewrapped and long headings shortened to the line limits; no
fact changed. Paths under `scratchpad/` name the architect's read-only working trees.

Inventory only. No target state, no options except where a fact forces one, no recommendations.
Feeds the owner's scope ruling on #8 Q14–Q18 (comments 5929815222, 5929902986, 5930151428). Scope per #22
(ruled 5930150451): semsource, semconnect, semteams, semboids, and the SemStreams pin. Prior pass
(`openspec/changes/setup-03b-contract-boundary/{design,inventory}.md` at `48b4616`) is not redone.

Bases read (every path below is relative to the named tree):

| Tree | Base | Path root |
| --- | --- | --- |
| SemStreams pin | `8b99efe9` (snapshot; `module github.com/c360studio/semstreams`) | `scratchpad/semstreams-8b99efe9/` |
| semsource | `3604a9ce8d5aec717253d7a905389f7910efd25d` (post-#213) | `scratchpad/semsource-main/` |
| semconnect | `d0d06e00bf05a545f30ceea798db1c2b1ee47d4f`; PR #74 head `dff1265708910ef411ab6f85b16fd9e94663e997` (branch `codex/migrate-semstreams-setup03a`, OPEN) read via `git show <sha>:<path>` | `scratchpad/semconnect/` |
| semteams | `ce22c961d30014c463a09f8f8a2a90044ee1a1cf` (main, shallow); `go.mod:6` semstreams `v1.0.0-beta.160` | `scratchpad/semteams/` |
| semboids | `8c03cc53836ced93a5df7064473c63ff144e64f1` (main, shallow); `go.mod:6` semstreams `v1.0.0-beta.160` | `scratchpad/semboids/` |

"The 65" = `scratchpad/my-port65.txt` (inventory A1). Line counts are non-test `.go` at depth 1 unless stated.

## Summary

| Q | One-line answer | Confidence |
| --- | --- | --- |
| 1 | `processor/rule` = 17,243 lines (15,826 + `expression` 1,417); 23 pin imports, 21 in the 65, but 5 of those are D4 separations (`agentic`, `agentic/agentrun`, `pkg/lifecycle`, `pkg/rulepack`, `vocabulary/agentic`) plus `governance` (never in the 65). The out-of-set edges live in 7 files (4 if `pkg/lifecycle` is kept: `actions.go`, `config_validation.go`, `config.go`, `verdict_auditor.go`) and belong to 3 action families (`publish_agent`, `deny`/`approve`, `lifecycle_*`) plus rule-pack ID validation; the shared `Action` struct (`actions.go:89`) itself carries two fields typed by `agentic` (`ResponseFormat *agentic.ResponseFormat` `:191`, `ToolChoice *agentic.ToolChoice` `:206`), so every rule-pack JSON decodes through `agentic`, and the `VerdictAuditor` interface (`:501`) and run stamping (`:750-752`) are `governance`/`agvocab`-typed. Two D4 edits break `processor/rule` at compile time (`Dependencies.ToolRegistry`/`ToolRegistryReader` drop → `factory.go:148`; `Dependencies.LifecycleManager` removal → `factory.go:160-161`); not porting `service/rule_pack_bind.go` strands `Processor.ProjectionBindings()` (`processor.go:443`). `pkg/lifecycle` (3,739) imports 9 packages, all in the 65 and none separated by D4. Keeping `pkg/lifecycle` costs six pass-through lines in the 65 (`component/dependencies.go:99`; `service/dependencies.go:36`; `service/component_manager.go:49,199,205,219,1218`) plus three import lines (`component/dependencies.go:12`, `service/component_manager.go:25`, `service/dependencies.go:13`); A1.2/D4's citation `:996-1111` is a local `component.LifecycleComponent`, not `pkg/lifecycle`. | high |
| 2 | **Overlapping in part**: both are durable, named-instance phase state with restart resume; disjoint on everything the owner's "durable execution" half names (journal, fences, receipts, effect retry, replay scan, generation). SemSource states its reason in `openspec/changes/replay-source-removal/design.md:61-63` (classified as an execution artifact, "not in `ENTITY_STATES`… no rule chain or second lifecycle authority"), not as "pkg/lifecycle missing"; no code comment names `pkg/lifecycle` (0 hits). | high |
| 3 | semteams: rules enter only via `componentregistry.Register` (`cmd/semteams/main.go:777`) + `rulepkg.NewConfigManager` (`:550`); lifecycle via `lifecycle.NewManager` (`:920`) with the framework's `agent-run` workflow (`agentrun.Register` `:929`); 78 rule JSON files, actions add_triple 50 / publish_agent 40 / lifecycle_transition 8 (all `"workflow": "agent-run"`) / reconcile_predicates 6 / remove_triple 4 / publish 2; `agentic` 29 identifiers/330 refs (tool SDK); `vocabulary/agentic` 25 identifiers; `pkg/rulepack` 0; settlement 0; also imports `engine`, `flowstore`, `flowtemplate`, absent at the pin. semboids: `rule.Register` only; own `flock.boid` workflow (`Workflow`, `Transitions`, `Participant`, `Manager.Create`); 7 rule actions (publish 6, lifecycle_transition 1); `agentic*`/`rulepack`/settlement 0. Workload: 200 boids @ 30 Hz, graph snapshots at `graph_hz` (1 in `flock.json:121`) → one message per boid to `entity.boid.upsert` on stream `ENTITY` (file, 24h, 2 GiB) as one async batch ≈ 200 entity msgs/s; composes sim, graph-ingest, rule-processor, websocket, graph-index, graph-clustering; `cmd/sweep` measures ingest lag, e2e latency, index write amplification. | high |
| 4 | graph-ingest's own order is apply → durable guard stamp → in-memory stamp → `Ack` (`keyed_ingest.go:144-229`); the only `Ack`-before-write paths are deliberate ack-and-drop of poison/metadata-less/stale messages (`component.go:1548,1566`; `keyed_ingest.go:158`), and the pinned spec says exactly that (`openspec/specs/graph-ingest/spec.md:162-172`). `natsclient/delivery_settlement.go` (#759) is called at the pin only by `internal/deliverylane/deliverylane.go:118,152`, used by `agentic/agentrun` and five `processor/agentic-*`; graph-ingest uses none of it and has 0 `InProgress` calls. MaxDeliver: default 3 (`component/port_jetstream.go:130`); graph-ingest, SemSource, semboids declare none → 3; semconnect's graph-ingest has no JetStream input at all. After MaxDeliver the advisories ARE retained durably at the pin: `config` (in the 65) provisions the fixed `MAX_DELIVERY_EVENTS` stream (file, 168h, 64 MiB; `config/streams.go:188-196,292`, `stream_bounds.go:231`) from `EnsureStreams`, which all four consumers call (semsource `run.go:256`, semboids `main.go:138`, semteams `main.go:705`, semconnect PR #74 `cs-graph-backend/main.go:224`); what is missing is the observer, `internal/maxdelivery`, started only by `internal/boot/run.go:184` and outside the 65. No test at the pin restarts a graph-ingest *process*; the "restart" test wipes the in-memory guard in-process (`keyed_ingest_integration_test.go:125-145`); `test/e2e/harness/processbarrier` is agentic-only. SemSource's `test/setup03a/qualification_test.go` restarts the real binary (`:246-248`) and the broker with the app SIGSTOPped (`:829-926`), proving memory-transport loss and authority survival, not in-flight settlement. | high |
| 5 | None of the four observes graph changes through an engine contract. semsource: no graph watch (its `Watch` calls are source watchers), reads via `graph.query.*` RPC, composes `output/websocket` unconditionally relaying `graph.ingest.>` (input, not applied state). semconnect: `graph.query.entity\|batch`, `graph.index.query.predicate` RPC only; one readiness-key watch in `conformance/`. semteams: core-NATS `agent.*` subjects + `graph.query.entity` RPC; UI SSE `/teams-dispatch/activity` served by `processor/agentic-dispatch` over `pkg/graphview` (KV `WatchAll`). semboids: raw `kv.WatchAll` on `ENTITY_STATES` ×2 and `pkg/graphview` views over `ENTITY_STATES` + `COMMUNITY_INDEX`. At the pin, graph-ingest publishes nothing after apply except a readiness status to the `GRAPH_STATUS` bucket; observation primitives that exist are `graph.OpenCatalogReader`/`CatalogReader.Watch\|WatchAll`, `component.KVWatchPort`, `pkg/graphview.View` (ADR-081; in the 65 but in D4's dormant→seam group), `graph/readiness.Set`, `lifecycle.Manager.Watch\|WatchEvents`. `output/websocket` is composed by semsource and semboids; not by semteams or semconnect. | high |
| 6 | `Storage` is a per-stream/per-port enum `file\|memory` (`component/port_codec.go:361`; `config/streams.go:30`), default `file` (`natsclient/stream.go:883-888`); **no component in the 65 declares it on a default port** (count 0). All framework KV buckets are file-backed by the nats.go zero value (`natsclient/kvspec.go:217-222`). Consumers: SemSource GRAPH `memory`/1h/256 MiB/discard new; semconnect declares **no ingest stream** (graph-ingest input is the `graph.mutation.>` request port); semboids ENTITY `file`/24h/2 GiB/discard old. Pinned docs define restart per primitive (`docs/concepts/03-streams-vs-kv-watches.md:33-61`), not per storage class; I7's "re-ingestion from current source" is SemSource's qualification observation (`qualification_test.go:875-926`), not a pinned framework statement. | high |

## Q1. Reach table for `processor/rule` and `pkg/lifecycle` at the pin

### Q1.1 Size

```text
find processor/rule -maxdepth 1 -name '*.go' -not -name '*_test.go' | xargs cat | wc -l   → 15826  (48 files)
find processor/rule/expression -maxdepth 1 ... | xargs cat | wc -l                        → 1417
find pkg/lifecycle -maxdepth 1 ... | xargs cat | wc -l                                    → 3739  (13 files)
```

`processor/rule` subdirs: `docs/`, `expression/`. Total rule = 17,243 (matches step-back review comment 5929756463).

### Q1.2 Import edges

`go list -mod=mod -f '{{join .Imports "\n"}}' ./processor/rule | grep c360studio`, classified against `my-port65.txt`:

| From | In the 65 (21) | Of those, separated by D4 | Outside the 65 |
| --- | --- | --- | --- |
| `processor/rule` | `agentic`, `agentic/agentrun`, `component`, `config`, `graph`, `graph/readiness`, `internal/graphmutation`, `internal/lifecyclecleanup`, `message`, `metric`, `natsclient`, `pkg/cache`, `pkg/errs`, `pkg/lifecycle`, `pkg/projection`, `pkg/resource`, `pkg/rulepack`, `pkg/types`, `types`, `vocabulary`, `vocabulary/agentic` | `agentic`, `agentic/agentrun`, `pkg/lifecycle`, `pkg/rulepack`, `vocabulary/agentic` | `governance`, `processor/rule/expression` (rule's own sub-package; imports `graph`, `pkg/cache`, `pkg/errs`, all in the 65) |
| `pkg/lifecycle` | `graph`, `internal/graphmutation`, `message`, `natsclient`, `payloadregistry`, `pkg/errs`, `pkg/projection`, `pkg/types`, `vocabulary` (9/9) | none | none |

Import sites in `processor/rule` non-test files for the six packages that are outside the set after D4:

```text
processor/rule/actions.go:15            "github.com/c360studio/semstreams/agentic"
processor/rule/actions.go:16            "github.com/c360studio/semstreams/agentic/agentrun"
processor/rule/actions.go:18            "github.com/c360studio/semstreams/governance"
processor/rule/actions.go:22            "github.com/c360studio/semstreams/pkg/lifecycle"
processor/rule/actions.go:27            agvocab "github.com/c360studio/semstreams/vocabulary/agentic"
processor/rule/actions_lifecycle.go:26  "github.com/c360studio/semstreams/pkg/lifecycle"
processor/rule/config_validation.go:9   "github.com/c360studio/semstreams/agentic"
processor/rule/config.go:14             rulepackcontract "github.com/c360studio/semstreams/pkg/rulepack"
processor/rule/execution_context.go:16  "github.com/c360studio/semstreams/pkg/lifecycle"
processor/rule/matches.go:9             "github.com/c360studio/semstreams/pkg/lifecycle"
processor/rule/verdict_auditor.go:10    "github.com/c360studio/semstreams/governance"
```

(`processor/rule/expression` is imported by 12 rule files: `actions.go:25`, `config_validation.go:13`,
`lifecycle_substitution.go:33`, `expression_factory.go:13`, `execution_context.go:17`, `runtime_config.go:10`,
`stateful_evaluator.go:12`, `matches.go:12`, `message_substitution.go:40`, `rule_factory.go:11`,
`test_rule_factory.go:10`, `typed_substitution.go:67`; it travels with rule.)

### Q1.3 Symbols referenced, by action family

What a rule core would have to carry or cut.

Action types declared at `processor/rule/actions.go:33-75`: `publish`, `add_triple`, `remove_triple`, `update_triple`,
`reconcile_predicates`, `publish_agent` (`:50`), `update_kv`, `deny` (`:56`), `approve` (`:63`), `lifecycle_transition`
(`:67`), `lifecycle_complete` (`:71`), `lifecycle_fail` (`:75`).

| Out-of-set package | Exported identifiers used (non-test) | Sites | Action family / role |
| --- | --- | --- | --- |
| `agentic` | `ToolDefinition`, `TaskMessage`, `ResponseFormat`, `ToolChoice`, `TryChainExecutionEntityID`, `LoopIDFromExecutionEntityID`, `LineageTriplePredicate`, `IsKnownFilesystemPolicy`, `MetadataKeyRelatedLoops`, `MetadataKeyFilesystemPolicy`, `MetadataKeyScratchPaths`, `MetadataKeyDecideActionAllowlist` | `actions.go:191,206,751,1445,1457,1462,1488,1494,1506,1546,1576,1584,1591,1601,1628,1780,1800,1830,1900,1930`; `config_validation.go:358,407` | `publish_agent` (task construction, tool resolution, lineage stamping) and its config validation |
| `agentic/agentrun` | `Mint` | `actions.go:1989` | `publish_agent` run-scope "new" (ADR-053 D4) |
| `vocabulary/agentic` | `LoopRun`, `LoopRunEntityID`, `RunOriginEntityID` | `actions.go:750,752,1841,1868,1884,2005-2007` | `publish_agent` run linkage |
| `governance` | `VerdictEvent`, `DecisionDeny`, `DecisionApprove`, `VerdictSubject` | `actions.go:501` (interface `EmitVerdict`), `:2109,2123,2183`; `verdict_auditor.go:45,55` | `deny` / `approve` verdict audit (publishes to `governance.verdict.>` stream, `verdict_auditor.go:31`) |
| `pkg/rulepack` | `PackIDCharset`, `MaxPackIDBytes`, `ValidateID` | `config.go:151,153,177` | rule-pack ID validation (143-line contract package) |
| `pkg/lifecycle` | `Manager` (concrete, `matches.go:115`), `Participant`, `TransitionSource`, `TransitionSourceRule`, `WorkflowDef`, `ErrAlreadyExists` | `actions.go:504-530` (interface `LifecycleManager`, a subset of `*lifecycle.Manager`), `:541`; `actions_lifecycle.go:56,78,83,269`; `execution_context.go:167-192`; `matches.go:108-130`; `lifecycle_substitution.go:54-78`; `expression_factory.go:34-80,266,293`; `stateful_evaluator.go:32-61`; `processor.go:84-91,369-373,700-702,749`; `runtime_config.go:224-260`; `rule_loader.go:238-240` | `lifecycle_transition\|complete\|fail` actions and `lifecycle.*` expression fields (`lifecycle.phase`, `lifecycle.terminal`, `lifecycle.workflow`, `lifecycle.workflow_def` at `lifecycle_substitution.go`, `stateful_evaluator.go`, `expression_factory.go`) |

Files with out-of-set imports: 7 (`actions.go`, `actions_lifecycle.go`, `config_validation.go`, `config.go`,
`execution_context.go`, `matches.go`, `verdict_auditor.go`); 4 once `pkg/lifecycle` is kept. Files a rule core would
have to edit to cut the agentic/governance edges: `actions.go` (2,339 lines; the `publish_agent`, `deny`, `approve`
executors and their stamp helpers — and the shared `Action` struct at `:89`, whose fields `ResponseFormat
*agentic.ResponseFormat` (`:191`) and `ToolChoice *agentic.ToolChoice` (`:206`) mean every rule-pack JSON decodes
through `agentic` today; the `VerdictAuditor` interface `EmitVerdict(ctx, governance.VerdictEvent)` at `:501`; run
stamping with `agvocab.LoopRun`/`LoopRunEntityID` at `:750-752`), `config_validation.go` (658; `:358,407`),
`verdict_auditor.go` (80; whole file is governance), `config.go` (265; `:151-177`). The `pkg/lifecycle` edge is a wiring
interface (`LifecycleManager`, `actions.go:509-530`) threaded through 11 files; `actions_lifecycle.go` (422) is the
executor for the three `lifecycle_*` actions and returns a typed error when no manager is wired (`:166-168`: "no
lifecycle.Manager wired on the rule processor (call SetLifecycleManager during component…").

### Q1.4 How the rule processor is wired from composition (what a port must reproduce)

```text
processor/rule/factory.go:136   processor, err := NewProcessorWithMetrics(deps.NATSClient, &ruleConfig, deps.MetricsRegistry)
processor/rule/factory.go:148   processor.SetToolRegistry(deps.ToolRegistry)
processor/rule/factory.go:152   processor.SetDecoder(message.NewDecoder(deps.PayloadRegistry))
processor/rule/factory.go:160   if deps.LifecycleManager != nil {
processor/rule/factory.go:161           processor.SetLifecycleManager(deps.LifecycleManager)
processor/rule/processor.go:443 func (rp *Processor) ProjectionBindings() (packID string, contracts []projection.Contract) {
```

`ProjectionBindings` is the only implementer of `service.ProjectionBinder` (`service/rule_pack_bind.go:13-17`; repo-wide
`grep 'func (.*) ProjectionBindings('` → 1 hit). It is bound at boot by `service.ConfigureRulePackMutations(manager)`
(`internal/boot/run.go:330`) and `registerRuleConfigService(manager, ruleManager, logger)` (`run.go:339`;
`internal/boot/rule_config_service.go`, 97 lines) using `rulepkg.NewConfigManager(...)` (`run.go:153`).
`lifecycle.NewManager` is constructed once in the framework at `internal/boot/run.go:220` and plumbed via
`svcDeps.LifecycleManager` (`run.go:287`), with `agentrun.Register(svcDeps.LifecycleManager)` at `run.go:299`.
`internal/boot` is outside the 65 and imports `processor/rule`, `pkg/lifecycle`, `agentic/agentrun`,
`processor/agentic-tools`, `frameworkcapabilities/rulepacks`, `internal/maxdelivery` (full list from `go list` in the
command log). SemSource's `cmd/semsource/run.go` does not use `internal/boot` (inventory A2.2), so none of this wiring
exists in the retained composition path today.

### Q1.5 D4 edits versus what `processor/rule` and `pkg/lifecycle` need

| D4 edit (design.md D4) | Symbol removed | Breaks `processor/rule`? | Breaks `pkg/lifecycle`? |
| --- | --- | --- | --- |
| Drop `component.Dependencies.ToolRegistry` and `component.ToolRegistryReader` | both | **Yes, compile**: `factory.go:148`; `actions.go:542,769`; `processor.go:82,347,687,689` | No |
| Remove `LifecycleManager` field from `component.Dependencies` (`component/dependencies.go:99`) and `service.Dependencies` (`service/dependencies.go:36`) and its plumbing (`service/component_manager.go:49,199-205,1218`) | `Dependencies.LifecycleManager` | **Yes, compile**: `factory.go:160-161`; and removes the only composition→rule path for `lifecycle_*` actions and `lifecycle.*` expression fields | No (the package compiles; nothing constructs or receives it) |
| Do not port `service/rule_pack_bind.go` | `service.ProjectionBinder`, `ProjectionBinders`, `ComponentsImplementing`, `ConfigureRulePackMutations` | **Strands** `Processor.ProjectionBindings()`/`PreflightProjectionMutations`/`SetPredicateReconciler` (`processor.go:443`): rule-pack projection clients are never injected; `grep -rn ConfigureRulePackMutations` non-test → only `internal/boot/run.go:330` | No |
| Do not port `service/milestone_service.go` | `service.NewMilestoneService` | No (rule does not import it) | No. Consumer impact: semteams `cmd/semteams/main.go:941` calls it. |
| Drop three agentic label predicates from `graph-query` | — | No | No |

### Q1.6 Reverse: what in the 65 references `pkg/lifecycle` (cost of keeping it)

```text
component/dependencies.go:79    // LifecycleManager is the shared pkg/lifecycle.Manager that
component/dependencies.go:93    // processor/rule.LifecycleManager) when they want to abstract
component/dependencies.go:99    LifecycleManager *lifecycle.Manager
service/dependencies.go:36      LifecycleManager  *lifecycle.Manager           // Shared Lifecycle harness Manager (ADR-047), plumbed to component deps (rule processor + lifecycle-gateway)
service/component_manager.go:49     lifecycleManager *lifecycle.Manager
service/component_manager.go:199    var lifecycleManager *lifecycle.Manager
service/component_manager.go:205    lifecycleManager = deps.LifecycleManager
service/component_manager.go:219    lifecycleManager:  lifecycleManager,
service/component_manager.go:1218   LifecycleManager: cm.lifecycleManager,
```

Plus `agentic/agentrun` (imports `pkg/lifecycle`; `agentrun.Register(mgr)` registers the `agent-run` workflow — in D4's
separation set).

**Correction to the prior pass:** design.md D4 and inventory A1.2 cite `service/component_manager.go:996-1111` as
`pkg/lifecycle` use. At the pin, `:992-996` is a parameter `lifecycle component.LifecycleComponent` and
`lifecycle.Stop(ctx)`; `:1039` is the same local; `:1108-1111` is `if lifecycle, ok :=
component.AsLifecycleComponent(comp); ok { lifecycle.Initialize() }`. Those are the component lifecycle interface, not
`pkg/lifecycle`. The `pkg/lifecycle` field removal touches five lines in `service` (`:49,199,205,219,1218`) and one in
`component` (`:99`), plus the three import lines (`component/dependencies.go:12`, `service/component_manager.go:25`,
`service/dependencies.go:13`), each a pass-through with no calls on the manager inside the 65.

## Q2. Does SemSource's `sourcelifecycle` duplicate what `pkg/lifecycle` governs?

### Q2.1 `pkg/lifecycle`'s own statement of purpose

```text
pkg/lifecycle/doc.go:1   // Package lifecycle provides a substrate convention layer for
pkg/lifecycle/doc.go:2   // workflow-shaped entities — named instances with declared phases,
pkg/lifecycle/doc.go:3   // restart recovery, operator visibility, and rule integration.
pkg/lifecycle/doc.go:7   // This package is a SUBSTRATE CONVENTION LAYER, not a workflow engine.
pkg/lifecycle/doc.go:16  // It does NOT provide:
pkg/lifecycle/doc.go:17  //   - A runtime, DSL, or state-machine interpreter (apps own work logic)
pkg/lifecycle/doc.go:18  //   - A separate event bus (uses existing NATS KV primitives)
pkg/lifecycle/doc.go:19  //   - A process orchestrator (orchestration stays in the rule engine)
pkg/lifecycle/doc.go:20  //   - A replacement for components (components remain the execution layer)
```

ADR-047 (`docs/adr/047-lifecycle-harness-substrate.md`): problem statement `:68-76` (consumers "inventing per-product
convention for state storage, KV key shape, terminal-state detection, restart recovery, and operator visibility"; `:82`
"semspec hand-rolled ~7,840 LOC of workflow harness code"); scope `:121-123` ("declare workflow-shaped entities and get
framework infrastructure (KV storage, restart recovery, operator API, rule integration) for free"); restart `:571-572`
("Restart recovery by default: rule engine bootstraps from the workflow KV bucket; instances resume from current
phase"); amendment `:6-16,30-36` (Manager owns no per-workflow bucket; state lands as triples in `ENTITY_STATES`).
SemEngine plan: `docs/setup-plan.md:279` "`pkg/lifecycle` governs graph business workflows; it is not automatically
needed to manage component goroutines."

What it models and persists:

| Dimension | `pkg/lifecycle` at the pin |
| --- | --- |
| Instance | a graph entity implementing `Participant` (struct tags `lifecycle:"id"`, `lifecycle:"phase"`, `operator_writable`; `doc.go:53-60`, `tags.go:80-84`) |
| Workflow | `Workflow{Name, EntityIDPattern, Phases, Transitions, PhasePredicate, Schema}` (`workflow.go`; `Transitions` table `transitions.go`) |
| Operations | `Manager.Register/Create/Update/TransitionWith/Complete/Fail/Get/List/Watch/WatchEvents/Despawn/LookupByEntityID/GetWorkflowDefinition` (`manager.go`, `manager_query.go`; subset used by rule at `processor/rule/actions.go:509-525`) |
| Persistence | triples on the entity in `ENTITY_STATES`, written through `internal/graphmutation.Client` (`graph_emit.go:24-72`: `CreateEntity`, `ReconcilePredicates`, `DeleteEntity`); read via `graph.OpenCatalogReader(ctx, nc, graph.BucketEntityStates)` (`manager.go:186`) |
| History | 64 transition records kept on the entity value, "an operational window, not an unbounded audit log" (`transition_records.go:1-5`) |
| Concurrency | CAS re-read/re-validate/re-emit, `updateRetries = 5` → `ErrUpdateRetriesExhausted` (`manager.go:489-545`, `errors.go:106-122`) |
| Restart | no code path of its own; state is the entity; `Watch`/`WatchEvents` bootstrap replays current values (`manager_query.go:170-173`); records "survive restart with" the entity (`:348`) |
| Fencing present | revision-fenced reconcile: "state changes route through graph-ingest via revision-fenced reconcile" (`manager.go:29`), "strict create or revision-fenced reconcile chosen by the component" (`:296`), "revision lets a compound operation fence its next mutation to the exact" observed value (`:523`), `projection.go:13`; plus the CAS retry budget above. This is a revision fence on the entity write, not an effect fence on external work. |
| Not present | timers, signals, journal, **effect** fences (ownership of an external attempt until its terminal outcome is proven), receipts, effect retry, replay scan, generation counter (`grep -i 'timer\|signal\|journal\|receipt\|replay'` non-test → only the `Watch` bootstrap replay note at `manager_query.go:170` and `ErrUpdateRetriesExhausted`) |

### Q2.2 SemSource's `sourcelifecycle`, `sourceintent`, `entitypub/receipt.go`

Full paths: `internal/sourcelifecycle`, `internal/sourceintent`, `internal/entitypub/receipt.go`, at `3604a9ce`.

Sizes: `internal/sourcelifecycle` 1,739 (9 files), `internal/sourceintent` 548 (2 files), `internal/entitypub` 1,256 (7
files, `receipt.go` among them). SemStreams imports across the three: `component`, `config`, `graph`, `message`,
`metric`, `natsclient`, `pkg/buffer`, `pkg/types`, `types`, `vocabulary` — **no `pkg/lifecycle`** (`grep -rln
'semstreams/pkg/lifecycle'` over semsource non-test → 0).

| Dimension | SemSource |
| --- | --- |
| Instance | one source handle's desired/projection state: `sourceintent.Record{Version, Binding, Operation, Phase, RetiredConfig, Scope, Tail, DesiredCommitted, ReplacementGeneration, Seed *SeedSeal, Effect *EffectAttempt, Progress, CreatedAt, UpdatedAt}` (`internal/sourceintent/contract.go:186-203`) |
| Phases | `Prepared`, `Pending`, `Complete`, `Superseded` (`contract.go:52-63`; "durable execution state, not runtime component admission") |
| Blockers | typed `ErrorCode` ×22 with `Retryable` (`contract.go:66-101`, e.g. `applied_tail_unproven`, `conditional_reconcile_unavailable`, `effect_unresolved`, `seed_incomplete`) |
| Settlement evidence | `Receipt{Binding, BatchID, EntityID, Fingerprint, Acknowledged}` per entity per batch (`contract.go:231-237`); `EffectAttempt`/`EffectFence` ("only a proven terminal outcome for this attempt may release its fence", `:263-290`); `TailEvidence` from consumer info `NumPending`/`NumAckPending` (`internal/sourcelifecycle/adapters.go:65-99`); `SeedSeal` ("a crash before the seal cannot grant freshness to a partial expected set", `seed.go:16`) |
| Persistence | product KV bucket `SEMSOURCE_SOURCE_LIFECYCLE` (`journal.go:20`), "product operational state, intentionally outside the graph catalog" (`:19`); `natsclient.EnsureFrameworkBucket` + backing-stream policy verification (`:45-49`); keys `source.<authority>.<handle>` and `receipt.<authority>.<handle>.<generation>.<seed-epoch>.<batch-id>.<entity-id>` (design.md:71-73); file-backed, history 1 (design.md:64-70) |
| Gate | `Gate` serializes desired changes and one replay unit; receipt writes bypass it (`gate.go:1-4`, `coordinator.go:2-3`) |
| Restart | boot `owner.RepairDesired(ctx)` (`cmd/semsource/source_lifecycle_runtime.go:51`; "resumes only committed exact disables", `coordinator.go:184`) then `owner.ReconcileOnce(runCtx)` on a 1 s ticker (`processor/source-manifest/source_lifecycle.go:97-99`; "scans all durable current intents… A failed unit remains eligible next scan", `replay.go:1-2`); design: "bootstrap scan and periodic bounded reconciliation regardless of KV watch delivery. One transient failure does not require another operator request or restart" (`design.md:373-375`) |
| Graph projection | single mutation owner bound before Start (`local_projector.go:1-4`); the lifecycle *record* never lands in `ENTITY_STATES` |

### Q2.3 Why SemSource did not use `pkg/lifecycle` (its own words)

`openspec/changes/replay-source-removal/design.md:53-63`:

> The `kv-or-stream` skill distinguishes facts from queued execution. Here the durable record is the current
> desired/projection state of one source handle, including the fence needed when intent changes. … The
> `orchestration-check` skill puts this operational execution artifact in component-owned storage, not in
> `ENTITY_STATES`. Source-manifest owns desired source operations and generation; supersession executes the
> single graph projection operation. There is no rule chain or second lifecycle authority.

The same section also states three reasons that are constraints of the frozen pin rather than classification
(`design.md:55-59,68-70`):

> An ordinary work stream alone is insufficient because this pin requires finite stream `MaxAge`, while an operator
> may defer application restart indefinitely. The operational KV record has no TTL or binding MaxBytes. It is not a
> KV watch used as an acknowledged queue: the owner performs a full startup scan and bounded periodic full-snapshot
> repair, retries failed effects, and exposes degradation. … The pinned Ensure mechanism reconciles retention/history
> but does NOT establish storage/replica correctness on an existing bucket. After acquisition, inspect authoritative
> backing-stream configuration and require FileStorage and exactly the declared replica policy

The skill's table it cites (`.agents/skills/orchestration-check/SKILL.md:14-18`): "Lifecycle harness | Declared phase
discipline for named entities | Phase graph, transition validation, operator-writable state contract | [does NOT own]
Work execution or hidden private storage". No `.go` file in semsource mentions `pkg/lifecycle`, "lifecycle harness", or
ADR-047 (`grep -rn -i` over `*.go`,`*.md` → the skill table only). The frozen-pin gaps it names are SemStreams issues
SS#1444, #1445, #1446 (`design.md:192,211,323,539`), not `pkg/lifecycle`.

### Q2.4 Conclusion

**Overlapping in part.** Overlap: a durable, named-instance record with a declared phase set, terminal phases
(`IsTerminal` ↔ `Complete`/`Superseded`), and resume-from-durable-state on restart. Disjoint: everything
`sourcelifecycle` exists for is absent from `pkg/lifecycle` (journal bucket with policy verification, effect fences —
`pkg/lifecycle` fences the entity write by revision, not an external attempt —,
per-entity receipts, retryable typed blockers, bootstrap scan + periodic replay, seed seals, generation), and
everything `pkg/lifecycle` is for is absent from `sourcelifecycle` (state as graph triples, rule-engine actions,
operator-writable contract, `Watch`/`WatchEvents`, parent/child workflows). Against the owner's two-halves
statement (comment 5930151428), `sourcelifecycle` implements durable-execution concerns ("replay, settlement,
retries with known outcomes") that `pkg/lifecycle` does not model; the owner's hypothesis that SemSource rolled
its own *because* `pkg/lifecycle` was missing is not what SemSource wrote down — it wrote down a classification
decision. Whether `pkg/lifecycle` *should* have covered this is a design question, not established here.

## Q3. What semteams and semboids use, at symbol level

Method: `grep -rn --include='*.go' --exclude='*_test.go'` for import lines, then `grep -oE '\b<alias>\.[A-Z]\w*'`
per package over non-test files. Import counts are not usage; identifiers are.

### Q3.1 semteams @ `ce22c961` (99 non-test `.go` files)

| Package | Import sites | Identifiers referenced | Notes |
| --- | --- | --- | --- |
| `processor/rule` | `cmd/semteams/main.go:36` (`rulepkg`) | `rulepkg.NewConfigManager` (`main.go:550`) + method `InitializeKVStore` (`:551`) | comment `:545-548`: "write-only CRUD for agentic-tools; the processor-internal one is read+apply". The rule *processor* type enters via `componentregistry.Register(componentRegistry)` (`main.go:777`; "All factories come from semstreams' componentregistry.Register", `:773`). |
| `pkg/lifecycle` | `main.go:32` | `lifecycle.NewManager` (`:920`), `*lifecycle.Manager` field (`:953`) | no own `lifecycle.Workflow{}` or `.Register(` of a product workflow (`grep` → 0); the only workflow is the framework's `agent-run` |
| `agentic/agentrun` | `main.go:20` | `agentrun.Register` (`:929`), `NewMilestoneSubscriber`, `NewNATSLoopTripleReader` (`:939`), `StartConfig`, `AgentStreamName` (`:942`) | plus `service.NewMilestoneService` (`:941`) and `service.WireGraphRuntime` (`:923`) |
| `agentic` | 27 files (`main.go:19`, `product_tools.go:12`, 17 `tools/*/executor.go`, 4 `tools/*/schema.go`, `runanchor`, `chainpause` ×3, `approvalpause` ×2, `commands/*` ×2) | 29 distinct / 330 refs: `ToolResult` 78, `ToolCall` 46, `ToolDefinition` 30, `ToolErrorInternal` 25, `ToolErrorNetwork` 21, `ToolErrorInvalidArgs` 21, `UserResponse` 17, `ToolExecutor` 14, `ToolErrorNotFound` 14, `ToolErrorKind` 13, `UserMessage` 7, `TryLoopExecutionEntityID` 6, … | the tool-executor SDK; `component.ToolRegistryReader` is the seam D4 drops |
| `vocabulary/agentic` | `main.go:39`, `chainpause/decision_handler.go:16`, `approvalpause/pauser.go:10` | 25: `LoopRunEntityID` ×4, `LoopRun`, `LoopWorkflow`, `LoopWorkflowStep`, `LoopUser`, `LoopTask`, `LoopRole`, `LoopReplyTo`, `LoopParent`, `LoopDescription`, `Lesson{SupersededBy,Summary,Status,Severity,RetiredAt,Polarity,ObservedRole,InjectionForm,Evidence,Detail,CreatedAt,Category,AppliesTo}`, `TodoRecord`, `ActionExecutedBy` | |
| `pkg/rulepack` | none | — | `grep 'pkg/rulepack'` → 0 |
| `natsclient` settlement | `natsclient` imported in 12 files; `DeliverySettlement\|SettleDelivery\|Settle*` → **0** | — | |
| absent at the pin | `main.go` imports `engine`, `flowstore`, `flowtemplate`; `flowtemplates/loader.go` imports `flowtemplate` | — | `ls engine flowstore flowtemplate` in the snapshot → "No such file or directory" (retired by #1116 per A1.1). semteams at beta.160 cannot build against the pin as it stands. |

Rule packs shipped: 78 JSON files under `configs/rules/` (`find … | grep -i rule` → 79 incl.
`schemas/rule-processor.v1.json`), groups `research/`, `create-change/`, `proof-readiness/`, `agent-run/`, …

```text
grep -rhoE '"type"\s*:\s*"(…action types…)"' configs | sort | uniq -c
  50 add_triple   40 publish_agent   8 lifecycle_transition   6 reconcile_predicates   4 remove_triple   2 publish
grep -rhoE '"workflow"\s*:\s*"[^"]+"' configs | sort | uniq -c   →   8 "workflow": "agent-run"
```

(`configs/rules/agent-run/{02-dispatched-to-executing,04-executing-to-failed,04b-dispatched-to-failed,05-coordinator-failed-run-anchor,09-…,11-…,13-…}.json`).
No `deny`/`approve`/`lifecycle_complete`/`lifecycle_fail`/`update_kv`/`update_triple` in semteams' packs.

### Q3.2 semboids @ `8c03cc53` (30 non-test `.go` files)

| Package | Import sites | Identifiers referenced |
| --- | --- | --- |
| `processor/rule` | `componentregistry/register.go:12` | `rule.Register` (`:23`; comment "zone transitions → steering modifiers") |
| `pkg/lifecycle` | `cmd/semboids/main.go:27`, `internal/boidgraph/lifecycle.go:6`, `internal/sim/lifecycle.go:14` | `lifecycle.NewManager` (`main.go:177`), `.Register(boidgraph.BoidWorkflow())` (`:178`), `lifecycle.Transitions` (`boidgraph/lifecycle.go:32`), `lifecycle.Workflow` (`:68-70`), `*lifecycle.Manager` (`sim/lifecycle.go:49-50`), `lifecycle.Participant` (`:52`), `lifecycle.ErrAlreadyExists` (`:292`), `Manager.Create(` (`:291`) |
| own workflow | `internal/boidgraph/lifecycle.go:25-36,68-78` | `flock.boid`: phases `active → culled \| expired` (both terminal); `BoidLifecycle{IDField lifecycle:"id"; PhaseField lifecycle:"phase,predicate=flock.lifecycle.phase"}`; comment `:11`: "owns no bucket — the phase triple lands in the SAME ENTITY_STATES entity" |
| `agentic`, `agentic/agentrun`, `vocabulary/agentic`, `pkg/rulepack` | none | `grep` → 0 each |
| `natsclient` settlement | `natsclient` in 3 files; settlement symbols → **0** | |
| other SemStreams imports (distinct) | `metric` 6, `pkg/graphview` 5, `message` 5, `pkg/projection` 4, `component` 4, `payloadregistry` 3, `service` 2, `graph` 2, `vocabulary`, `types`, `processor/graph-ingest`, `processor/graph-index`, `processor/graph-clustering`, `payloadbuiltins`, `output/websocket`, `config` | |

Rule packs: `configs/rules/zone-steering/*.json`; actions `publish` 6, `lifecycle_transition` 1 (`predator.json`, rule
`predator-cull`). `Dependencies.LifecycleManager: lifecycleMgr` is set at `cmd/semboids/main.go:190` (the path
`processor/rule/factory.go:160` reads).

Workload (the verification-fixture facts):

| Fact | Evidence |
| --- | --- |
| Entities | 200 boids (`internal/sim/component.go:70` "ADR-001 defaults: 200 boids at 30Hz"; `configs/flock.json:63-64` `"boids": 200, "tick_hz": 30`) + static zones (`flock.json:66-85`: predator, food, wind) |
| Physics | 30 Hz tick; one aggregated frame per tick, fire-and-forget core NATS to `boids.frames` (`component.go:2-4,31`) — no per-boid substrate traffic |
| Graph load dial | `graph_hz` snapshot cadence, "the load dial, ADR-001 §4… Runtime-adjustable via the boids API", 0 disables (`component.go:48-50`); `flock.json:121` `"graph_hz": 1` |
| Per snapshot | one message per boid to `entity.boid.upsert` (`internal/boidgraph/payload.go:19`), published as ONE async batch joined on all acks (`publisher.go:26-30,146-161`) → ≈200 entity msgs/s at 1 Hz; counters `GraphCounts() (snapshots, entities, dropped)` (`component.go:318`) |
| Stream | `ENTITY`, subjects `entity.>`, `storage: file`, `max_age: 24h`, `max_bytes: 2147483648`, `discard: old`, `replicas: 1` (`flock.json:17-25`) |
| Composition | `sim` (input), `graph-ingest` (jetstream input `entity_stream`, `graph_mutations` request port, `entity_states` KV write), `rule-processor` (core-NATS ports `zone_events` ← `boids.zone.events`, `steering` → `boids.steering`), `websocket` (output), `graph-index`, `graph-clustering` (`flock.json:59-290`; `componentregistry/register.go:21-25,34`) |
| Consumer | `entity_stream` kind `jetstream`; no `max_deliver`/`deliver_policy` declared (`flock.json:133-143`; `grep -n max_deliver` → 0) |
| Lifecycle in the loop | rule `predator-cull` → `lifecycle_transition` to `culled`; sim `runCullWatcher` watches `ENTITY_STATES` for `phase=culled`, then deletes the entity through the mutation client (`sim/lifecycle.go:114-173`) |
| Measurement | `cmd/sweep/main.go:1-12`: sets the dial via the boids API, measures physics fps, scrapes `:9090` for "achieved snapshot/entity rates, snapshot drops, graph-ingest consumer lag, end-to-end latency quantiles, and graph-index write amplification", classifies the window (publisher-bound / ingest-bound / index-bound / downstream-lag / rejection-loss); `LatencyProbe` watches `ENTITY_STATES` for boid entities (`boidgraph/probe.go:36-84`) |

## Q4. graph-ingest's settlement under process replacement

### Q4.1 Consume site and every settlement call (non-test)

Consume: `processor/graph-ingest/component.go:1534-1538` (`c.natsClient.ConsumeStreamWithConfig`, overridable by
`c.consumeStream`); consumer config `:1522-1531` passes `DeliverPolicy` (forced `"all"` when unset, `:1518-1520`),
`AckPolicy`, `MaxDeliver`, `MaxAckPending` from `component.GetConsumerConfig(port)` (`:1515`); `AutoCreate: false`.

| Site | Call | When | Relative to the durable write |
| --- | --- | --- | --- |
| `component.go:1548` | `msg.Ack()` | JetStream metadata missing | before any write; message dropped deliberately ("dropping") |
| `component.go:1566` | `msg.Ack()` | decode/extract failure ("poison message: count + ack-drop") | before any write; dropped deliberately |
| `component.go:1586` | `msg.Nak()` | `ingestPool.SubmitBlocking` failed | no write attempted |
| `keyed_ingest.go:102` | `w.msg.Nak()` | panic in `processIngest` (`OnPanic`) | write state unknown; redelivery relies on idempotent merge + guard |
| `keyed_ingest.go:135` | `work.msg.Term()` | `prepareFactProjection` structural/authority rejection | before write; terminal |
| `keyed_ingest.go:151` | `work.msg.Nak()` | durable guard read failed | before write |
| `keyed_ingest.go:158` | `work.msg.Ack()` | stale redelivery (seq ≤ last applied) | no write; already applied earlier |
| `keyed_ingest.go:183` | `work.msg.Nak()` | `ingestEntity` failed on resident poisoned state | after failed write |
| `keyed_ingest.go:190` / `:198` | `work.msg.Term()` | `ingestEntity` invalid / fatal | after failed write; terminal |
| `keyed_ingest.go:201` | `work.msg.Nak()` | `ingestEntity` transient | after failed write |
| `keyed_ingest.go:215` | `work.msg.Nak()` | durable guard stamp failed **after** a successful `ingestEntity` | write done, not acked → redelivery re-applies (idempotent merge) |
| `keyed_ingest.go:222` | `work.msg.Ack()` | success | **after** `ingestEntity` (`:167`) and `ingestGuardStampDurable` (`:212`) and the in-memory stamp (`:220`); `recordApplied` after the ack (`:229`) |

`grep -c InProgress processor/graph-ingest/{component,keyed_ingest}.go` → 0, 0. The code's own statement of order:
`keyed_ingest.go:204-207` "Guard stamp AFTER side effects, BEFORE ack — durable FIRST, then in-memory."

**Can an ack precede the durable write on any path?** Only on the three ack-and-drop paths (`component.go:1548,1566`;
`keyed_ingest.go:158`), where no write is intended. On every path that intends a write, the ack follows it or a Nak/Term
is issued. The pinned spec states this contract: `openspec/specs/graph-ingest/spec.md:162-172` ("an explicit ack after
the merge completes; a decode/extract/metadata failure or a stale-redelivery guard-drop acknowledges-and-drops (counted,
not redelivered); and a submit failure, a durable-guard read or write failure, or a Process panic naks for redelivery")
and `:224-229` (redelivery after restart is ignored because "the durable guard tier survives the restart").

Residual at the pin (stated, not judged): between `ingestEntity` success (`:167`) and `Ack` (`:222`) a process
replacement leaves an applied-but-unacked message; redelivery re-enters at `:144` and is judged stale by the durable
guard only if `ingestGuardStampDurable` (`:212`) completed; otherwise the merge re-applies (idempotent per
`component.go:1510-1511` "graph-ingest is idempotent (ENTITY_STATES merge/CAS overwrites)").

### Q4.2 What `natsclient/delivery_settlement.go` (SS#759) provides; callers

Surface: `DeliveryDecision{Invalid, Ack, Retry, Terminate, Quarantine}` (`:17-34`), `DeliveryWork` (`:35`), typed errors
`DeliveryMetadataUnavailableError`/`InvalidDeliveryDecisionError`/`DeliveryWorkPanicError` (`:40-91`),
`DeliveryRetryPolicy` (`ImmediateDeliveryRetry`, `DelayedDeliveryRetry`; `:108-126`), `HeartbeatDeliveryPolicy` +
`ValidateHeartbeatDeliveryPolicy` (`:139-155`), `DeliveryResult` with
`Decision/Cause/ControlError/SettlementError/SettlementAttempted/SettlementMethodSucceeded/SettlementMethodFailed/Quarantined/OwnerStopRequired/Err`
(`:224-277`), `SettleDelivery` (`:291`), `SettleDeliveryWithRetry` (`:303`), `ConsumeDeliveryWithHeartbeat` (`:323`).

Callers at the pin (non-test): `internal/deliverylane/deliverylane.go:118` (`ConsumeDeliveryWithHeartbeat`) and `:152`
(`SettleDeliveryWithRetry`) — the only two. `internal/deliverylane` is imported by `agentic/agentrun` (2 files),
`processor/agentic-dispatch`, `processor/agentic-governance`, `processor/agentic-loop`, `processor/agentic-model`,
`processor/agentic-tools`. graph-ingest imports neither. What #759 has that graph-ingest's hand-written path lacks: a
`Quarantine` decision, in-progress heartbeats for long work, bounded settlement retry, and a typed result that can
demand owner stop.

### Q4.3 `MaxDeliver` and what happens after it

| Where | Fact |
| --- | --- |
| Framework default | `component/port_jetstream.go:112` "- MaxDeliver: 3"; `:130` `MaxDeliver: 3,` in `consumerConfigFromFacts`; overridden only when `stream.MaxDeliver() > 0` (`:148-149`) |
| graph-ingest | passes `consumerCfg.MaxDeliver` (`component.go:1529`); default `entity_stream` port declares `StreamName: "ENTITY", Subjects: ["entity.>"], DeliverPolicy: "all"` and no MaxDeliver (`component.go:395`) → effective 3 |
| SemSource | `cmd/semsource/run.go:783-788` `JetStreamPort{StreamName: "GRAPH", Subjects: ["graph.ingest.entity"], DeliverPolicy: "all"}`; `grep -rn -i max_deliver` over semsource `*.go\|*.json\|*.yaml` → 0 → effective 3 (A13 "not established" is now established from the declaration; a `nats consumer info` read would confirm the live value) |
| semconnect | graph-ingest has **no JetStream input**: `deploy/semstreams.json:33-45` (both `d0d06e00` and PR #74 head) declares only `graph_mutations` (`kind: nats-request`, `subject: graph.mutation.>`) and the `entity_states` KV write; `grep -c '"streams"'` → 0 at both commits; `enable_hierarchy: true` (`:47` at `d0d06e00`, `:46` at `dff12657`) |
| semboids | `flock.json:133-143` no `max_deliver` → 3 |
| After MaxDeliver | NATS stops redelivering and emits `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>`. The pin keeps those advisories durably: `config/streams.go:188-196` declares `maxDeliveryEventsStreamConfig` (file, `168h`, 64 MiB, discard old, limits, 1 replica) and `:292` registers it in `frameworkStreams()` as `{name: "MAX_DELIVERY_EVENTS", … fixed: true}` ("this declaration is immutable. The observer and its retained source are one framework guarantee", `:289-291`); `EnsureStreams` (`config/streams.go:410-411`) → `planStreams` → `resolveStreamDeclarations` (`config/stream_bounds.go:441,218`) → `frameworkStreams()` (`:231`). All four consumers call `EnsureStreams`: semsource `cmd/semsource/run.go:256`, semboids `cmd/semboids/main.go:138`, semteams `cmd/semteams/main.go:705`, semconnect PR #74 `cmd/cs-graph-backend/main.go:224`. So porting `config` carries the stream half of the guarantee `streams.go:289-291` describes. The **observer** half — `internal/maxdelivery` ("owns SemStreams' durable visibility for JetStream MaxDeliver exhaustion occurrences", `observer.go:1-6`; capture stream `MAX_DELIVERY_EVENTS`, `:29`) — is started only at `internal/boot/run.go:184`, is outside the 65, and is not composed by any of the four consumers (none uses `internal/boot`). Non-test references to `MAX_DELIVER*` outside `test/`: `config/streams.go:189,292`, `internal/maxdelivery/observer.go:29,36,208`. |

### Q4.4 Tests at the pin and in SemSource

| Test | What "restart" means there |
| --- | --- |
| `processor/graph-ingest/keyed_ingest_integration_test.go:130` `TestIntegration_IngestGuard_DurableSurvivesRestart` | in-process: stamps the durable guard, then `c.ingestGuardMem[0] = newLaneGuard(...)` ("Simulate a restart / full eviction", `:143-145`); no component Stop/Start, no process |
| `processor/graph-ingest/poison_scoping_test.go:58` `TestPoisonRepairRecoveryWithoutRestart` | explicitly without restart |
| `processor/graph-ingest/lifecycle_integration_test.go`, `lifecycle_owner_test.go` | component Start/Stop lifecycle; `grep -E 'exec.Command\|os.Process\|Kill'` in graph-ingest tests → 0 |
| `test/e2e/harness/processbarrier/processbarrier.go:1-6` | "the agentic E2E process-replacement barrier… an operation-specific, test-only NATS protocol"; used by `test/e2e/scenarios/agentic/stage_a_process_replacement.go` and `process_replacement_test.go` only; `grep -l 'graph-ingest\|graphingest\|ENTITY_STATES'` over those files → none |
| semsource `test/setup03a/qualification_test.go` (`TestKnownAnswerCorpus`, `:92`) | runs the real binary (`exec.Command(h.binary, "run", …)`, `:404`); `h.stop(false); h.start(); h.structural("application_restart", true)` (`:246-248`); `pendingBrokerRestart` (`:800-927`): SIGSTOPs the app (`:829`), publishes a captured `graph.ingest.entity` envelope (`:842`), asserts `accepted_unindexed_before_broker_restart` (`:875`), `docker restart` of NATS (`:880`), asserts `memory_transport_not_durable` (`:893-896`), `authority_survives_broker_restart_before_reingest` (`:897`), `content_survives_broker_restart_before_reingest` (`:911`), `broker_restart_reingested` (`:926`), `captureRestartGuard` (`:927`, `:1009-1035`). This is a transport-loss probe on a memory stream, not an unacked-in-flight process replacement on a durable stream. |
| semsource `test/setup03a/removal_replay*_test.go` | replay of source removal (sourcelifecycle), not graph-ingest settlement |

## Q5. Change observation ("live")

### Q5.1 How each consumer observes graph changes today

| Consumer | Watch / subscribe / consume calls (non-test) | Engine APIs | `output/websocket` |
| --- | --- | --- | --- |
| semsource `3604a9ce` | `handler.Watch(...)` in `processor/{cfgfile,video,git,url,audio,image,objectstore,doc}-source/component.go` — source watchers (filesystem/git/URL), not graph; `client.Subscribe(ctx, statusReportSubject, …)` `processor/source-manifest/component.go:269`; `grep 'KeyValue(\|ENTITY_STATES\|OpenCatalogReader\|WatchAll'` non-test → only the `entity_states` KV write port (`run.go:807`) and comments | reads graph via request ports `graph.query.*` (`run.go:868,894`), `graph.index.query.*` (`:904`); `sourcelifecycle.NATSTail.Settled` reads `stream.Consumer(...).Info` (`NumPending`, `NumAckPending`) for applied-tail evidence (`internal/sourcelifecycle/adapters.go:65-99`) | **composed unconditionally** (`websocketComponentConfig`, `run.go:1040-1091`): input `graph_entities` = `JetStreamPort{StreamName: "GRAPH", Subjects: ["graph.ingest.>"]}` (`:1067`), `delivery_mode: at-most-once` (`:1081`), bind default `0.0.0.0:7890`, path `/ws`. It relays ingest *input*, not applied graph state. Whether any client connects: not established (no test or doc found; A13 already records it). |
| semconnect `d0d06e00` | `conformance/cmd/index-readiness/main.go:35` `bucket.Watch(ctx, readiness.KeyGraphIndex, jetstream.UpdatesOnly())` — a readiness probe, not entity changes; no other Watch/Subscribe | `gateway/cs-api/systems.go:79-81` request subjects `graph.index.query.predicate`, `graph.query.entity`, `graph.query.batch`; ports `component.go:264-266` | none (`grep output/websocket\|pkg/graphview\|graph-gateway` → 0). PR #74 head `cmd/cs-graph-backend/main.go` (352 lines): no Watch/Subscribe/websocket (`grep` → 0). |
| semteams `ce22c961` | core NATS `client.Subscribe` on `agent.failed.>` (`chainpause/subscriber.go:21,50`), `agent.approval_pending.>` / `agent.approval_response.>` (`approvalpause/subscriber.go:27,34,90`); no KV watch in `cmd/` | entity reads via request/reply `graph.query.entity` (`chain/entity_reader.go:42,95`); UI live data: SSE `/teams-dispatch/activity` (`ui/src/lib/stores/agentStore.svelte.ts:18,135`; events `loop_created\|loop_updated\|loop_deleted` per `api.generated.ts:16`) served by SemStreams `processor/agentic-dispatch/http_activity.go:376` via `view.SnapshotAndSubscribe(ctx)` (`:289-304`) = `pkg/graphview.View[T]` over KV `WatchAll` (`pkg/graphview/view.go:43,231,465`); `/flowbuilder/status/stream` websocket (`ui/src/lib/services/runtimeWebSocket.ts:75`) targets the flow runtime retired at the pin; `/graphql` (`agentApi.ts:15`) | none |
| semboids `8c03cc53` | `kv.WatchAll(ctx)` on `ENTITY_STATES`: `internal/boidgraph/probe.go:80-84` (latency probe), `internal/sim/lifecycle.go:119-124` (cull watcher); `natsClient.Subscribe(ctx, SteeringSubject, …)` core NATS `boids.steering` (`internal/sim/component.go:520`) | `pkg/graphview` views over `ENTITY_STATES` and `COMMUNITY_INDEX` for the API graph pane (`internal/api/graphstream_views.go:14-15,144`; `graphstream.go:60-200` uses `graphview.Snapshot/Delta/DeltaUpsert/DeltaDelete/DeltaPoison/Subscription`; `service.go:118` `graphview.WatcherSource`) | **composed** (`componentregistry/register.go:8,21` "frames → browser"; `flock.json:207-239`) |

### Q5.2 What graph-ingest / graph-index publish that a consumer could observe

| Producer | Output | Evidence |
| --- | --- | --- |
| graph-ingest | writes `ENTITY_STATES` (`KVWritePort{Bucket: graph.BucketEntityStates}`, `component.go:404`); after apply publishes **nothing on a subject**; the only post-apply emission is readiness status into the `GRAPH_STATUS` KV bucket (`readiness.go:379` `readiness.NewPublisher(bucket, readiness.KeyGraphIngest)`; `:351`) | `grep -n '\.Publish(' processor/graph-ingest/*.go` non-test → `readiness.go:351` only |
| graph-ingest | third KV write: `ENTITY_SUFFIX_INDEX` (`graph.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketEntitySuffixIndex)` `component.go:1214`; `c.suffixBucket.Put` `:2801,2807`, `Delete` `:2824,2830`) — an index, not an observation surface | |
| graph-ingest | redelivery-guard bucket (durable applied-sequence stamps) | `keyed_ingest.go:212` `ingestGuardStampDurable`; bucket provisioned at Start (`keyed_ingest_integration_test.go:134` "Start must provision the durable guard bucket") |
| graph-index | **observes** `ENTITY_STATES` by `component.KVWatchPort{Bucket: graph.BucketEntityStates}` (`component.go:154,183`; `bucket.WatchAll(ctx)` `:974`); writes `OUTGOING_INDEX`, `INCOMING_INDEX`, `ALIAS_INDEX`, `PREDICATE_INDEX` (`:161-170,188-197`; bucket names `graph/constants.go:6-43`) | |
| Streams | `graph.ingest.*` / `entity.>` are **inputs**; no "graph changed" subject is declared anywhere in the 65 (`grep -rn 'graph\.(changed\|applied\|event)'` → 0 non-test) | |

### Q5.3 Exported change-observation surfaces at the pin (none a 03B row)

| Surface | Declaration | Status in the 65 / D4 |
| --- | --- | --- |
| `graph.OpenCatalogReader(ctx, client, name) (CatalogReader, error)` with `CatalogReader{Get, Watch, WatchAll, Keys, ListKeys, ListKeysFiltered, Status}` | `graph/kvcatalog.go:272-280, 294` ("the exact union consumed by current framework catalog readers. The underlying write-capable JetStream handle is never exposed") | in the 65; carried; used by `pkg/lifecycle` (`manager.go:186`) and graph-index (`:1077-1095`) |
| `component.KVWatchPort` — "NATS KV Watch for state observation" | `component/port_kv.go:5-6` | in the 65; carried; used by graph-index's default ports |
| `pkg/graphview.View[T]` — "shared read-side fan-out primitive over a NATS KV bucket (ADR-081): ONE WatchAll feeding one validated in-memory current-state projection… fanned out to N local subscribers with snapshot+delta consistency" | `pkg/graphview/doc.go:1-14`; `view.go:172 New`, `:203 Start`, `:307 WaitCaughtUp`, `:416 Get`, `:438 List`, `:465 SnapshotAndSubscribe`, `:502 Subscribe` | in the 65 (1,193 lines) but D4 row "graph/clustering, graph/llm, **pkg/graphview**, model/wire → carry dormant, then seam, last task of 04A"; pin importers: `processor/graph-query` (2 files), `processor/agentic-dispatch` (2); consumer importers: semboids (5 files) |
| `graph/readiness.Set` / `NewSet` / `Publisher` | `graph/readiness/set.go:27,41`; `publisher.go:57,67` | in the 65; used by semconnect's conformance probe |
| `lifecycle.Manager.Watch` / `WatchEvents` | `pkg/lifecycle/manager_query.go:142,177` (declarations; the shared pattern watch opens at `:212-216`; `doc.go:47-49`) | D4 defer-exclude |

The 03B retained-contract matrix areas (design.md "Retained-contract matrix skeleton") contain none of these (step-back
finding 3 stands).

## Q6. Recovery per storage class

### Q6.1 The `Storage` option

```text
component/port_jetstream.go:13   Storage         string   `json:"storage,omitempty"`        // "file" or "memory" when declared
component/port_codec.go:361      if err := validateOptionalEnum("storage", port.Storage, "file", "memory"); err != nil {
config/streams.go:30             Storage   string   `json:"storage,omitempty"`   // "file" or "memory" (default: file)
natsclient/stream.go:883-888     switch autoConfig.Storage { case "memory": …MemoryStorage; default: …FileStorage }
```

KV buckets: `natsclient/kvspec.go:217-222` `kvConfigFor` sets `Bucket, Description, History, Replicas` (+TTL) and **no
`Storage`**; nats.go `v1.52.0` `jetstream/stream_config.go:611` `FileStorage StorageType = iota` → every framework
bucket (`ENTITY_STATES`, the index buckets, `GRAPH_STATUS`, the guard bucket) is file-backed.

### Q6.2 Who declares what

| Declarer | Stream / bucket | Storage | Evidence |
| --- | --- | --- | --- |
| Any component in the 65, default ports | — | **none declared** | `grep -rn 'JetStreamPort{[^}]*Storage:'` over the 65 non-test → 0 |
| Framework streams (`config/streams.go`) | LOGS `file`; HEALTH, METRICS, FLOWS `memory` ("No persistence needed"); GOVERNANCE_VERDICT_AUDIT `file` ("an audit trail must survive restarts"); MAX_DELIVERIES ledger `file`, 168h | per row | `config/streams.go:113,127,139,151,176,190` |
| graph-ingest default input | `ENTITY` (`entity.>`) | not declared → stream owner's choice | `processor/graph-ingest/component.go:395` |
| SemSource | `GRAPH` (`graph.ingest.{entity,batch,manifest,status,predicates}`) | **`memory`**, `MaxAge 1h`, `MaxBytes 256 MiB`, `Discard new` | `cmd/semsource/run.go:990-998` (A2.2) |
| semconnect (`d0d06e00`, PR #74 head) | **no ingest stream**; graph-ingest input is `graph.mutation.>` `nats-request`; `"streams"` absent | n/a | `deploy/semstreams.json:33-45`; `grep -c '"streams"'` → 0 at both SHAs |
| semboids | `ENTITY` (`entity.>`) | **`file`**, 24h, 2 GiB, discard old, replicas 1 | `configs/flock.json:17-25` |

### Q6.3 What the pinned docs/specs say recovery means

| Source | Statement |
| --- | --- |
| `docs/concepts/03-streams-vs-kv-watches.md:33-39` | KV watch on restart: the processor "receives all current values matching its watch pattern" (graph-index example) |
| `docs/concepts/03-streams-vs-kv-watches.md:57-61` | JetStream consumer on restart: "unacknowledged work may be redelivered; acknowledged work is not replayed. Consumers remain at-least-once and must make effects idempotent." |
| `openspec/specs/graph-ingest/spec.md:162-172` | at-least-once ack after merge; ack-and-drop classes; Nak classes |
| `openspec/specs/graph-ingest/spec.md:224-229` | redelivery after restart ignored via the durable guard |
| `openspec/specs/graph-ingest/spec.md:339,481` | "a restart's replay is invisible to watchers"; "snapshot transport failure keeps the boot recovery contract" (scenario titles) |
| pinned docs on **storage class** recovery | `grep -rn -i 'memory.{0,40}(restart\|durab\|recover\|lost)'` over `docs/concepts docs/operations docs/reference openspec/specs` → no statement tying recovery to `file` vs `memory` for graph streams; only the HEALTH/METRICS/FLOWS comments ("No persistence needed") |
| drafted I7 (`design.md:468-480`) | "recovery after transport loss SHALL be re-ingestion from the current source"; scenario "Memory transport lost at broker restart" — this is SemSource's `qualification_test.go` observation set (`memory_transport_not_durable`, `authority_survives_broker_restart_before_reingest`, `broker_restart_reingested`), generalized |

### Q6.4 Facts that bear on the reframing (stated, not ruled)

Three storage situations exist among the four consumers, each with a different meaning for "recovery":

1. **Memory stream** (SemSource GRAPH): a broker restart loses accepted-but-unapplied messages
   (`qualification_test.go:893-896` proves absence); recovery requires re-publication from a source that still exists.
2. **File stream** (semboids ENTITY; graph-ingest's own default): unacked messages are redelivered after a process or
   broker restart (`docs/concepts/03:59-61`); recovery is redelivery + idempotent merge + durable guard; no re-ingestion
   needed, but MaxDeliver=3 parks a message after three failed deliveries; the advisory lands in the
   `MAX_DELIVERY_EVENTS` stream that `config.EnsureStreams` provisions for every consumer, and nothing reads it unless
   `internal/maxdelivery` (boot-only, outside the 65) is composed.
3. **No stream** (semconnect): graph-ingest is driven by the `graph.mutation.>` request/reply; there is no in-flight
   message to recover; the caller holds the outcome or a commit-unknown (design.md D8).

`Storage` is a declared per-stream choice at the pin; no component in the 65 constrains it, and no pinned spec states an
acknowledged-write contract per class.

## Not established (and what would establish it)

- Live `MaxDeliver` of SemSource's `graph-ingest` consumer: declaration implies 3; a `nats consumer info GRAPH
  <consumer>` against a running 03A stack would confirm.
- Whether any client connects to SemSource's or semboids' `output/websocket`; a runtime trace or an operator statement
  would.
- Whether semteams' `agentic-dispatch` activity SSE is the only live path its UI relies on in production; the UI code
  shows SSE + GraphQL + a flowbuilder websocket, but no runtime trace was taken.
- semconnect's live stream set at the PR #74 head: `deploy/semstreams.json` declares none; the binary may provision
  streams from `config` defaults at boot (`cmd/cs-graph-backend/main.go` uses `config.NewLoader()`, `:115`); a `nats
  stream ls` on the compose stack would show it.
- Whether graph-ingest's applied-but-unacked window (`keyed_ingest.go:167-222`) has ever been exercised under a real
  process kill on a file stream: no test at the pin or in the four consumers does it (`grep` results in Q4.4); the
  `processbarrier` harness exists and is agentic-only.
- Which `pkg/lifecycle` Manager methods semteams' *runtime* reaches (the rule packs' 8 `lifecycle_transition` actions
  call `TransitionWith`; `agentrun.Register` registers the workflow); not measured by execution.
- Any statement by SemSource's authors beyond `design.md:53-63` on why `pkg/lifecycle` was not used; none exists in the
  tree.

## Command log (reproduction)

All commands were run read-only from the trees named at the top. Key ones:

```text
# Q1
go list -mod=mod -f '{{join .Imports "\n"}}' ./processor/rule | grep c360studio      (23 lines; classified vs my-port65.txt)
go list -mod=mod -f '{{join .Imports "\n"}}' ./pkg/lifecycle | grep c360studio       (9 lines, all in the 65)
grep -n -E '"github.com/c360studio/semstreams/(agentic|agentic/agentrun|governance|pkg/lifecycle|pkg/rulepack|vocabulary/agentic|processor/rule/expression)"' processor/rule/*.go | grep -v _test.go
grep -n -oE '\b(agentic|agentrun|governance|rulepackcontract|agvocab|lifecycle)\.[A-Za-z_][A-Za-z0-9_]*' processor/rule/*.go | grep -v _test.go
grep -rn --include='*.go' --exclude='*_test.go' -E 'func \(.*\) ProjectionBindings\(' .          → processor/rule/processor.go:443
grep -rn --include='*.go' --exclude='*_test.go' -E 'lifecycle\.NewManager\(' .                   → internal/boot/run.go:220
grep -n -E 'lifecycle\.[A-Z]|LifecycleManager' component/*.go service/*.go | grep -v _test
sed -n 992,997p service/component_manager.go; sed -n 1036,1040p …; sed -n 1108,1112p …          (local component.LifecycleComponent)
# Q2
grep -rn -i -E 'pkg/lifecycle|lifecycle harness|ADR-047|lifecycle\.Manager' --include='*.go' --include='*.md' semsource-main   → .agents/skills/orchestration-check/SKILL.md:18 only
grep -rln --include='*.go' --exclude='*_test.go' 'semstreams/pkg/lifecycle' semsource-main       → (none)
grep -rn --include='*.go' --exclude='*_test.go' -E '\.RepairDesired\(|\.ReconcileOnce\(' semsource-main
# Q3
grep -rn --include='*.go' --exclude='*_test.go' -E '"github.com/c360studio/semstreams/(processor/rule|pkg/lifecycle|agentic|agentic/agentrun|pkg/rulepack|vocabulary/agentic|natsclient)"' <repo>
grep -rn --include='*.go' --exclude='*_test.go' -oE '\b(rulepkg|rule|lifecycle|agentrun|agvocab|agentic)\.[A-Z][A-Za-z0-9_]*' <repo>
grep -rhoE '"type"\s*:\s*"(publish|add_triple|…|lifecycle_fail)"' configs | sort | uniq -c
grep -rn --include='*.go' --exclude='*_test.go' -E 'DeliverySettlement|SettleDelivery|natsclient\.(Settle|Settlement|NewSettlement|Delivery)' <repo>   → 0 in both
# Q4
grep -n -E '\.(Ack|Nak|NakWithDelay|Term|TermWithReason|InProgress|DoubleAck|AckSync)\(' processor/graph-ingest/*.go | grep -v _test
grep -rn --include='*.go' --exclude='*_test.go' -E 'natsclient\.(SettleDelivery|SettleDeliveryWithRetry|ConsumeDeliveryWithHeartbeat)\(' .
grep -rln --include='*.go' --exclude='*_test.go' 'semstreams/internal/deliverylane"' . | xargs -n1 dirname | sort | uniq -c
grep -rln --include='*.go' --exclude='*_test.go' 'internal/maxdelivery"' .                        → internal/boot/run.go
grep -rn --include='*.go' --exclude='*_test.go' 'MAX_DELIVER' . | grep -v '^./test/'              → config/streams.go:189,292; internal/maxdelivery/observer.go:29,36,208
grep -n EnsureStreams semsource-main/cmd/semsource/run.go semboids/cmd/semboids/main.go semteams/cmd/semteams/main.go sc74-graph-backend-main.go   → :256, :138, :705, :224
grep -rln 'processbarrier' test/e2e --include='*.go'; grep -l -E 'graph-ingest|graphingest|ENTITY_STATES' <those>   → none
grep -rn -i -E 'max_deliver|MaxDeliver' semsource-main/cmd/semsource/*.go semsource-main/configs/*.json   → 0
# Q5
grep -rn --include='*.go' --exclude='*_test.go' -E '\.(Watch|WatchAll|WatchFiltered|WatchEvents|Subscribe|SubscribeSync|QueueSubscribe|ConsumeStream|ConsumeStreamWithConfig)\(' <repo>
grep -rn --include='*.go' --exclude='*_test.go' -E 'func \(.*\) SnapshotAndSubscribe\(' .        → pkg/graphview/view.go:465
grep -rln --include='*.go' --exclude='*_test.go' 'semstreams/pkg/graphview"' . | xargs -n1 dirname | sort | uniq -c
grep -n -E '\.Publish(Async|ToStream|Batch)?\(' processor/graph-ingest/*.go | grep -v _test       → readiness.go:351
# Q6
grep -rn --include='*.go' --exclude='*_test.go' -E 'JetStreamPort\{[^}]*Storage:' <the 65 dirs>   → 0
git -C semconnect show dff1265…:deploy/semstreams.json | grep -c '"streams"'                     → 0
go list -mod=mod -m -f '{{.Dir}}' github.com/nats-io/nats.go → …/jetstream/stream_config.go:611 FileStorage StorageType = iota
```

## Changelog

### 2026-10-01 — review round 1: CHANGES REQUESTED → applied

Review: `scratchpad/03b/scope-inventory-review.md`.

Every finding was re-verified against the trees before the edit (commands in the log above).

- **BLOCKING (Q4 summary row, Q4.3 "After MaxDeliver", Q6.4 item 2).** Removed the claim that MaxDeliver exhaustion has
  no visibility unless `internal/maxdelivery` is composed. Corrected fact: `config` (in the 65) provisions the fixed
  `MAX_DELIVERY_EVENTS` stream (file, 168h, 64 MiB) from `EnsureStreams` (`config/streams.go:188-196,289-292,410-411`;
  `config/stream_bounds.go:218,231,441`), and all four consumers call `EnsureStreams` (semsource `run.go:256`, semboids
  `main.go:138`, semteams `main.go:705`, semconnect PR #74 `cs-graph-backend/main.go:224`). Stated as: porting `config`
  carries the stream half of the guarantee `streams.go:289-291` describes; the open item is the observer
  (`internal/maxdelivery`, boot-only, outside the 65). Replaced the wrong "no other non-test reference to
  `MAX_DELIVERIES` outside `test/`" with the actual hits (`config/streams.go:189,292`;
  `internal/maxdelivery/observer.go:29,36,208`) and added both commands to the log.
- **MEDIUM (a) Q1 summary and Q1.3.** "6 files" → 7 files with out-of-set imports (4 if `pkg/lifecycle` is kept),
  listed.
  Added the shared `Action` struct (`actions.go:89`) fields typed by `agentic` (`:191`, `:206`) — every rule-pack JSON
  decodes through them — and the `governance`-typed `VerdictAuditor` interface (`:501`) and `agvocab` run stamping
  (`:750-752`) to the narrative; they were already in the Q1.3 table.
- **MEDIUM (b) Q2.1 and Q2.4.** "fences not present" corrected: `pkg/lifecycle` has revision-fenced reconcile
  (`manager.go:29,296,523`; `projection.go:13`) and the CAS retry budget; what is absent is an *effect* fence on an
  external attempt. New "Fencing present" row; "overlapping in part" unchanged.
- **MEDIUM (c) Q2.3.** Quoted the SemSource reasons that are pin constraints, not classification (`design.md:55-59`:
  finite stream `MaxAge` rules out an ordinary work stream; startup scan / periodic repair / effect retry duties;
  `:68-70`: pinned Ensure does not verify storage or replicas). Conclusion unchanged.
- **NITS.** `service/component_manager.go:219` added as a sixth pass-through line, plus the three import lines
  (`component/dependencies.go:12`, `service/component_manager.go:25`, `service/dependencies.go:13`) in the Q1 summary,
  Q1.6 and the correction paragraph. `enable_hierarchy` cited at `:47` (d0d06e00) and `:46` (dff12657).
  `Manager.Watch`/`WatchEvents` declarations cited at `manager_query.go:142,177` (the pattern-watch helper stays
  `:212-216`). Q5.2 gained graph-ingest's third KV write, `ENTITY_SUFFIX_INDEX` (`component.go:1214,2801,2807`;
  deletes `:2824,2830`), marked as an index rather than an observation surface.
