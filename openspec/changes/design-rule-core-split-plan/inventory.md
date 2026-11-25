# Inventory: design-rule-core-split-plan

base: 4ead6360978d008649e0eb5e869086dcac2b36f1 (revision 2 on 47c52a89b72b9ea0983a08d69a7bbed838c18604).
Pin copy: `gh api repos/C360Studio/semstreams/tarball/8b99efe9`, unpacked to a scratch directory.
Question: how to cut the `processor/rule` port into PRs. Repositories read: semengine, semstreams at the pin, and
semteams, semboids, semsource and semconnect at symbol level (`docs/inventory-scope.md`).

## Size

`find processor/rule -name '*.go'` = 163 files: production 53 files 17,243 lines, tests 110 files 34,224 lines
(`wc -l` per file). Per file and per planned PR (`(split)`: the file is divided between PRs; "X": not carried):

| PR | Production | Test | Files (pin lines) |
| --- | --- | --- | --- |
| 1 | 1417 | 2110 | `expression/array_operators_test.go` 318, `expression/evaluator.go` 965, `expression/evaluator_test.go` 1449, `expression/message_path.go` 49, `expression/regex_cache.go` 137, `expression/regex_cache_test.go` 343, `expression/stateless.go` 90, `expression/types.go` 176 |
| 2 | 4082 | 1035 | `action_id.go` 133, `action_id_test.go` 137, `actions.go` 2339 (split), `config_test_helpers_test.go` 12, `config_validation.go` 658 (split), `config_validation_test.go` 493, `doc.go` 142, `entity_pattern_contract.go` 121, `entity_pattern_contract_test.go` 100, `interfaces.go` 51, `predicate_authoring_test.go` 14, `rule_factory.go` 290, `rule_id_token_test.go` 29, `schema.go` 85, `schema_test.go` 154, `status.go` 21, `test_rule_factory.go` 219, `user_response_subject_reservation.go` 23, `user_response_subject_reservation_test.go` 96 |
| 3 | 1850 | 2890 | `caller_substitution.go` 70, `caller_substitution_test.go` 168, `entity_substitution.go` 88, `entity_substitution_test.go` 243, `execution_context.go` 846, `for_each_substitution_test.go` 169, `lifecycle_substitution.go` 307, `lifecycle_substitution_test.go` 508, `message_substitution.go` 81, `message_substitution_test.go` 275, `state_tracker.go` 263, `state_tracker_test.go` 406, `triple_length_substitution_test.go` 194, `triple_triples_substitution_test.go` 211, `triple_value_substitution_test.go` 286, `typed_substitution.go` 195, `typed_substitution_test.go` 430 |
| 4 | 419 | 5650 | `action_failure_metrics_test.go` 163, `actions_subject_override_test.go` 325, `actions_test.go` 4052 (split), `deny.go` 48, `executor_platform_wiring_integration_test.go` 90, `graph_event_identity.go` 49, `graph_event_identity_semantics_test.go` 54, `graph_event_identity_test.go` 449, `publisher.go` 231, `publisher_graph_event_contract_test.go` 283, `triple_mutator.go` 91, `triple_mutator_revision_integration_test.go` 234 |
| 5 | 1025 | 2478 | `actions_lifecycle.go` 422, `actions_lifecycle_test.go` 723, `actions_reconcile_test.go` 445, `config_projection_test.go` 311, `entity_suffix_index_ownership_test.go` 114, `kv_writer.go` 131, `lesson_lifecycle_config_test.go` 50, `operational_bucket_ownership_test.go` 82, `projection_bindings_test.go` 87, `projection_derivation.go` 339, `projection_derivation_test.go` 486, `projection_targets.go` 133, `shared_config_bucket_ownership_test.go` 180 |
| 6 | 747 | 2291 | `expression_factory.go` 449, `expression_factory_test.go` 792, `matches.go` 228, `matches_lifecycle_integration_test.go` 104, `matches_test.go` 688, `payload_projection.go` 70, `payload_projection_integration_test.go` 279, `payload_projection_test.go` 428 |
| 7 | 955 | 3164 | `action_maxiterations_test.go` 417, `entity_evaluation_fence.go` 227, `entity_evaluation_fence_test.go` 139, `fire_every_n_events_test.go` 384, `owner_lane.go` 167, `owner_lane_test.go` 82, `revision_tracking_test.go` 163, `state_cleanup_integration_test.go` 95, `stateful_evaluator.go` 561, `stateful_evaluator_persist_test.go` 96, `stateful_evaluator_test.go` 1420, `stateful_integration_test.go` 94, `stateful_rule_test.go` 274 |
| 8 | 2624 | 2412 | `config.go` 265, `config_ports_test.go` 43, `config_startup_wait_test.go` 90, `config_test.go` 231, `factory.go` 172, `factory_integration_test.go` 194, `factory_platform_refusal_test.go` 66, `factory_startup_wait_test.go` 82, `graph_state_contract_test.go` 45, `health_race_test.go` 34, `lifecycle_integration_test.go` 111, `lifecycle_runtime_test.go` 270, `log_capture_test.go` 61, `main_test.go` 13, `metrics.go` 231, `mock_kv_test.go` 216, `platform_test.go` 7, `processor.go` 1683 (split), `processor_debug_test.go` 309, `processor_platform_refusal_test.go` 88, `reader_capabilities_test.go` 20, `rule_lifecycle_test.go` 28, `rule_loader.go` 273, `status_test.go` 504 |
| 9 | 505 | 1070 | `message_handler.go` 505 (split), `message_handler_test.go` 55, `rule_event_notification_test.go` 51, `rule_integration_test.go` 964 |
| 10 | 1304 | 3684 | `entity_rule_pattern_selection_test.go` 560, `entity_state_identity_test.go` 233, `entity_watcher.go` 1075, `entity_watcher_atomic_bootstrap_test.go` 458, `entity_watcher_debounce_integration_test.go` 514, `entity_watcher_debounce_test.go` 221, `entity_watcher_generation_test.go` 345, `entity_watcher_hardening_integration_test.go` 156, `entity_watcher_integration_test.go` 580, `readiness.go` 229, `readiness_integration_test.go` 236, `readiness_test.go` 381 |
| 11 | 1494 | 2994 | `cron_metrics.go` 293, `cron_metrics_test.go` 184, `cron_rule.go` 199, `cron_rule_test.go` 238, `cron_scheduler.go` 665, `cron_scheduler_integration_test.go` 372, `cron_scheduler_test.go` 1500, `cron_substitution.go` 89, `cron_substitution_test.go` 253, `schedule_tracker.go` 248, `schedule_tracker_test.go` 447 |
| 12 | 741 | 1467 | `kv_config_integration.go` 381, `kv_config_list_test.go` 138, `kv_config_stop_bound_integration_test.go` 98, `kv_config_stop_fence_test.go` 310, `kv_hot_reload_integration_test.go` 290, `runtime_config.go` 360, `runtime_config_lifecycle_test.go` 95, `runtime_config_test.go` 536 |
| X | 80 | 2979 | `actions_foreign_firing_reason_test.go` 153, `actions_run_scope_integration_test.go` 478, `deny_integration_test.go` 276, `example_fan_out_integration_test.go` 381, `lifecycle_owner_test.go` 482, `research_graph_pipeline_integration_test.go` 418, `test_graph_ingest_owner_integration_test.go` 68, `test_helper_escape_proof_integration_test.go` 33, `test_owner_external_integration_test.go` 58, `test_owner_proof_test.go` 186, `test_owner_support_test.go` 113, `test_processor_owner_proof_test.go` 216, `verdict_auditor.go` 80, `verdict_auditor_test.go` 117 |

Split files, by line range at the pin (estimates where the range is not exact):

- `actions.go`: `:31-535` (types, `Action`, interfaces) PR 2; dispatch, `publish`, triple actions PR 4 (~480);
  `reconcile_predicates` and `update_kv` (`:1328-1444`, `:2253-2339`) PR 5 (~285); not carried: `:565-759`,
  `:765-791`, `:1445-2100`, `:2101-2252` and the agentic `Action` fields (~1,070).
- `config_validation.go`: `:273-562`, `:592-658` PR 2, except the `update_kv` checks (`:375-390`), which go to PR 5;
  `:18-272`, `:563-591` (methods on `*Processor`) PR 12.
- `processor.go`: shell PR 8 (~1,300); message input PR 9 (~150); entity input and debounce PR 10 (~230).
- `message_handler.go`: `:18-245`, `:389-505` PR 9; `:246-388` PR 10.
- `actions_test.go`: PR 4 ~900, PR 5 ~440, not carried ~2,700 (tests named for agent, tool, deny, approve, verdict,
  for-each, run scope and related features: 2,675 lines by `awk` over `func Test` ranges).
- `test_rule_factory.go`: production at the pin, read only by 11 test files; PR 2 moves it into a test file.

## Imports against main

`go list -f '{{.Imports}}' ./processor/rule ./processor/rule/expression` at the pin:

- Present on main: component config graph graph/readiness internal/graphmutation message metric natsclient
  pkg/errs pkg/lifecycle pkg/projection pkg/rulepack pkg/types types vocabulary.
- Moved: pkg/cache -> internal/cache (ledger :829); pkg/resource -> internal/resource (:357);
  internal/lifecyclecleanup -> pkg/lifecyclecleanup (:2021).
- Not carried: agentic (:1853), agentic/agentrun (:1884), vocabulary/agentic (:1929); governance (no row).
- Test-only imports not on main: payloadbuiltins, processor/agentic-tools, agentic/research,
  vocabulary/rulepacks (`rule_integration_test.go:27`, `:820`; no ledger row); internal/semantictest ->
  internal/harness/semantictest (:317).
- Third-party modules not in SemEngine's `go.mod`: `github.com/robfig/cron/v3` v3.0.1 (production:
  `cron_rule.go:21`, `cron_scheduler.go`; pin `go.mod:16`) and `github.com/xeipuuv/gojsonschema` v1.2.0 (test only:
  `schema_test.go`; pin `go.mod:20`).

## Compile probe of the production code against main

The reviewer's scratch tree `/tmp/rvbuild.cOG7` (local only) holds main with the pin's `processor/rule` copied in
and stubs for the uncarried agentic and governance imports. `go build -gcflags=-e ./processor/rule/` reports 49
errors. Each drift point, its home on main or the removal behind it:

| Pin use (file:line) | Main today |
| --- | --- |
| `component.ToolRegistryReader` (`actions.go:542`, `:769`, `processor.go:82`, `:347`, `:687`); `deps.ToolRegistry` (`factory.go:148`) | never ported (#29, closed); leaves with `publish_agent` |
| `component.Metadata` (`processor.go:42`, `:284`, `:412`) | gone; `Discoverable` has no `Meta()` (`component/discovery.go:17-28`) |
| `Registry.RegisterWithConfig`, `component.RegistrationConfig` (`factory.go:15`) | `Registry.Register(Registration)` (`component/registry.go:46`, `:128`; ledger component adaptation 7) |
| `deps.LifecycleManager` (`factory.go:160-161`) | dropped (#102 ruling D; ledger :2475) |
| `gtypes.IsFrameworkOwnedBucket`, `FrameworkOwnedWriteRefusal` (`actions.go:2264-2265`, `config_validation.go:382-385`, `kv_writer.go:87-88`); `SpecFor` (`kv_writer.go:86`); `EnsureCatalogBucket` (`kv_writer.go:92`) | `graph/kvcatalog` (`kvcatalog.go:206`, `:225`, `:246`, `:313`) |
| `gtypes.OpenCatalogReader` (`entity_watcher.go:113`, `:127`, `:1051`) | `graph/kvcatalog/kvcatalog.go:292` |
| `gtypes.UnmarshalEntityState(data, &s)` (`entity_watcher.go:767`, `:1065`) | takes the key first (`graph/entity_predicate_contract.go:288`) |
| `graph.NewExactEntityReader` one result (`triple_mutator.go:31`, `:73`) | returns `(*ExactEntityReader, error)` (`graph/exact_entity.go:46`) |
| `entityState.Version` (`message_handler.go:380`) | dropped (ADR-110, `docs/adr/110-write-seam.md:95`) |
| `graph.IndexStateResetRequired` (`readiness.go:83`) | gone; states are building, ready, degraded (`graph/index_status.go:143-146`); reset reported as `graph.ErrorCodeGraphStateResetRequired` (`graph/state_contract.go:14`) |
| `readiness.BucketGraphStatus` (`readiness.go:183`, `:197`), `readiness.EnsureBucket` (`:191`), `readiness.ProducerNames` (`:224`) | `graph.BucketGraphStatus` (`graph/constants.go:70`); no `EnsureBucket` or `ProducerNames` |
| `readiness.NewPublisher` one result (`readiness.go:199`) | returns `(*Publisher, error)` (`graph/readiness/publisher.go:41`) |
| `metric` `RegisterCounterVec`, `RegisterHistogramVec`, `RegisterGauge`, `RegisterGaugeVec` (`cron_metrics.go:173-179`) | gone; `metric.RegisterOrGet` (`metric/registry.go:29`) |
| `cache.NewCoalescingSet` one result (`processor.go:930`); `CoalescingSet.Close` (`entity_watcher.go:980`) | returns an error; no `Close`, it stops with `Shutdown(ctx)` (`internal/cache/coalescing_set.go:74`, `:195`) |
| `cache.Config.StatsInterval` (`config.go:245`) | refused (`internal/cache/config_test.go:164`) |
| `cache.WithEvictionCallback` (`expression/regex_cache.go:21`) | removed (owner, #9 comment 5950725772) |
| `gtypes.NewEntityUpdateEvent`, `gtypes.EventMetadata` (`expression_factory.go:353`, `test_rule_factory.go:136`) | `graph/events.go` not ported (#101, comment 6126673987; ledger :2212): "rule mutates through the mutation-request protocol … or not at all" |

Not covered: test code. The probe built production files only, so test-side drift is not counted.

The graph-event path at the pin: `ExpressionRule.ExecuteEvents` builds an update for the rule's trigger entity
(`expression_factory.go:349-358`; identity `graph_event_identity.go:22`, ADR-076). `message_handler.go:207-230`
validates and publishes the batch (`publisher.go:81-160`) to `graph.events.*`. No subscriber: at the pin (#101) and in
the consumers (`grep -rln 'graph\.events'` over semteams, semboids, semsource, semconnect: semconnect evidence files
only). No consumer implements `ExecuteEvents` or calls `RegisterRuleFactory` (`grep` over the same four).
`enable_graph_integration` gates only the triple writer (`processor.go:669`); the `rule_events` port notification
(`publisher.go:175-200`) is separate.

What else rides on that path, `fireRuleActions` (`message_handler.go:174-232`):

- The `fire_every_n_events` gate (`:188-196`) sits in front of only `ExecuteEvents`, the `rule_events` notification
  and the `rulesTriggered` counter. A rule's `on_enter`, `on_exit` and `while_true` actions run earlier and ungated in
  the stateful evaluator (`message_handler.go:135-150`, `stateful_evaluator.go:426`), on the entity path too. The
  documented behaviour, "fire its action only every Nth matching event" (`rule_factory.go:44-56`), is not implemented
  for actions. semteams records the miss (`configs/rules/ops/01-run-terminal-observe.json:59`, semstreams#1007) and
  sets the key in no checked-in rule (`grep -rln fire_every_n_events` over semteams and semboids configs: that one
  file, in a note). Cron rules differ: `cron_rule.go:88` stores the key and `cron_scheduler.go:496-504` skips the
  actions on all but every Nth tick (`TestCronScheduler_FireFireEveryNGate`, `cron_scheduler_test.go:405-426`: 2
  executions over 6 ticks at N=3; `cron_rule_test.go:165-176` allows the key on cron rules).
- `total_triggered` (`processor.go:1456`) counts only firings whose graph events published
  (`message_handler.go:227-231`).
- `events_published` (`processor.go:1457`) and `events_published_total` count graph events (`publisher.go:115-118`),
  `publish` actions (`:58-61`) and `rule_events` notifications (`:226-229`).
- `ExecuteEvents` is where the in-memory cooldown is stamped and `shouldTrigger` cleared
  (`expression_factory.go:363-364`).
- ADR-076 decision 4 (pack ID as producer identity) is already kept by `pkg/rulepack` (ledger :3044); decisions 1-3
  and 5 govern the trigger and alert identities.

Orderings the pin's owner-proof tests check that `openspec/specs/lifecycle-suite/spec.md` does not state: `Stop`
drains the JetStream consumers before the run context is cancelled (`lifecycle_owner_test.go:118`); cron's `Stop`
joins an admitted fire before the `Start` context is cancelled (`test_owner_proof_test.go:14`).

## Pin probe

`go test -count=1 -race ./processor/rule/...`: ok. `go test -count=5 -cpu=1 -shuffle=on ./processor/rule/...`: ok
(seeds 1795602496010215000, 1795602495775027000). Text: `time.Sleep` 53 lines in 12 files; `t.Skip` at
`status_test.go:352`; 22 files `//go:build integration`, none with `t.Parallel`; production `context.Background|TODO`:
0; `WithoutCancel`: `stateful_evaluator.go:247` (bounded persist). Not run: the lifecycle suite and the
integration tests.

## Shape sweep (findings; dispositions in split.md)

- Global rule-factory map filled from `init`: `rule_factory.go:124-127`, `expression_factory.go:443-449`,
  `test_rule_factory.go:212-218`.
- Production test factory: `test_rule_factory.go` (219 lines; read by 11 test files).
- Second decode spelling: `rule_loader.go:39` `definitionToRuleMap`; `config_validation.go:449` `definitionFromMap`.
- Four executor constructors: `actions.go:803`, `:820`, `:843`, `:894`; two processor constructors `processor.go:250`,
  `:255`.
- Dead config: `config.go:27` `MessageCache` (written `message_handler.go:92`, never read); `rule_loader.go:154-165`
  (parsed, only logged); `Consumer` struct in `config.go` (0 readers).
- `GenericJSON` from known fields: `message_handler.go:384` (with `time.Time`); `actions.go:2230` (leaves with
  deny/approve).
- Decode failure acknowledged: `message_handler.go:67-71`, `processor.go:1141`.
- Classification by error text: `message_handler.go:465-495` `recordError` picks the error label with
  `strings.Contains(errorMsg, "unmarshal"|"marshal"|"publish"|"execution"|"evaluate"|"validation"|"validate")` and the
  rule name by `strings.Split(errorMsg, "rule ")`.
- Collectors registered with `registry.PrometheusRegistry().MustRegister` (`metrics.go:209`), not
  `metric.RegisterOrGet`.
- Not found: a context stored in a struct (`grep -n -E '^\s+[a-zA-Z]+\s+context\.Context'`: function arguments
  only).

## Open issues (`gh issue list --state open --search rule|expression`)

- Name the package: #25 #26 #27 #28 #294 #702 #703 #714 (milestone), #196 (`processor.go:102`,
  `cron_scheduler.go:54`), #195, #350, #704, #24, #9.
- Use the word only: #730 #712 #141 #110 #109 #635 #155 #554 #614 #84 #284 #734 #535 #134 #665 #484 #60 #300 #683
  #576 #111.
- #29: closed 2026-11-25 (owner's word). #101 (closed) leaves the graph-event path to this port.

## Open PRs

`gh pr list --state open`: #752 only (this claim).

## ADRs

From `grep -o -h -E 'ADR-[0-9]{3}'` over the carried production files, plus records read by title.

| ADR | Title | Decides (for this code) | Cited in | Owning PR |
| --- | --- | --- | --- | --- |
| 023, 034 | ToolChoice threading; structured `response_format` | agentic action fields | `actions.go:180-194`, `:1596` | not carried (E2/E3) |
| 031 | Time-trigger primitive | cron rules in the rule processor; `RULE_SCHEDULES` last-fire records | `cron_*.go`, `schedule_tracker.go` | 11 |
| 036 | Agent-private observable state | conditions on rule-opaque predicates are refused | `config_validation.go:210-217` | 2 |
| 039 | Tool-call governance is rule-driven | deny/approve | `actions.go`, `execution_context.go`, `message_*` | not carried (E4); 3 and 9 check the substitution clauses |
| 041 | Unified condition evaluator | one evaluator for rule and action conditions | `expression/evaluator.go`, `expression_factory.go`, `stateful_evaluator.go` | 1, 6, 7 |
| 045, 053, 028, 026 | rule-chain orchestration; agent-run; orchestration layers | `publish_agent` paths | `actions.go`, `config_validation.go` | not carried (E3) |
| 046 | `for_each` fan-out | iterate an action over a list | `actions.go`, `execution_context.go`, `expression/*` | E3 removes the action; 3 decides the overlay |
| 047, 049 | Lifecycle harness | `lifecycle_*` actions over `pkg/lifecycle` | 11 files | 5 (not in `docs/adr`) |
| 048 | BoundedDispatcher and `.triples` | `.triples` substitution | `execution_context.go` | 3 |
| 055, 091 | Write-intent taxonomy; mutation authority | how rule triple writes enter graph-ingest | `actions.go`, `processor.go`, `metrics.go` | 4, 5 |
| 056 | Authoritative semantic state (superseded by 091) | `$entity` substitution meaning | `actions.go`, `processor.go` | 3, 4 |
| 066 | Honest graph-index readiness | readiness gauges | `processor.go:326` | 10 (ported) |
| 067 | Read-only execution policy | filesystem policy on `publish_agent` | `actions.go:232`, `config_validation.go:402` | not carried (E3) |
| 068 | Graph retention, deletion and GC | a reader never creates `ENTITY_STATES` | `entity_watcher.go:90` | 10 (not in `docs/adr`) |
| 076 | Framework-derived entity identity for rule events | the trigger and alert entity IDs | `graph_event_identity.go` | decisions 1-3 and 5 depend on owner decision 1 (`design.md`); 4 kept in `pkg/rulepack` |
| 083, 088 | Readiness as state; per producer | the rule's `GRAPH_STATUS` key | `processor.go:134` | 10 (ported) |
| 093 | Typed user-response subject ownership | `publish` may not target `user.response.>` | `user_response_subject_reservation.go` | 2 |
| 094 | Boot-only composition, observable rule activation | rule definitions are the one live exception | (by title) | 12 (ported) |
| 102, 104 | Entity-ID segments; unique platform authority | minting under the deployment authority | 6 files | 2, 4, 8 (ported) |
| 110 (SemEngine) | The write seam | no `EntityState.Version`; `update_kv` cannot write framework buckets (`:48`) | (by subject) | 5, 10 |
| 108, 109, 113, 095 (SemEngine or ported) | Processor shell; ready after boot; Start context; one-shot lifecycle | the processor's lifecycle | (by subject) | 8 |
| 111, 112 (SemEngine) | Durable execution; natsclient contract | `RULE_STATE`, `RULE_SCHEDULES`, settlement | (by subject) | 7, 9, 11, 14 |

`docs/adr` on main holds 030 058 060 062 063 066 071 074 077 078 081 083 084 085 087 088 094 095 096 098 099 100 102
104 107 108 109 110 111 112 113 114 115 (`ls docs/adr`).

## Guidance at the pin that returns with the code

- `processor/rule/README.md` (113 lines) and `processor/rule/docs/` (9 files, 4,177 lines: `actions.md`,
  `conditions.md`, `configuration.md`, `custom-rules.md`, `entity-watching.md`, `examples.md`, `operations.md`,
  `state-tracking.md`, `syntax.md`).
- Pin reviewer contract "Rules and orchestration" (`.agents/contracts/semstreams-reviewer.md:254`, 8 lines) and the
  rule clauses of "Graph and state ownership" (`:241-244`), which SemEngine's reviewer contract says return with the
  rule processor (`.agents/contracts/semengine-reviewer.md:325-326`).
- Pin developer contract "Orchestration" (`.agents/contracts/semstreams-developer.md:253-261`, 8 lines).
- Skills: `orchestration-check` (106 lines); the rule half of the pin's `entity-or-bucket` (`SKILL.md:75-110`, "What
  rules evaluate today"), which SemEngine's copy defers (`.agents/skills/entity-or-bucket/SKILL.md:64`).

## Consumers (symbol level)

- semboids: `componentregistry/register.go:34` `rule.Register`; `cmd/semboids/main.go:200` sets
  `Dependencies.LifecycleManager`; packs `configs/rules/zone-steering/*.json` (publish x6, lifecycle_transition x1),
  core NATS input `configs/flock.json:198-233`, `enable_graph_integration: false` (`:209`).
- semteams production: `internal/runtimecatalog/components.go:36` `rule.Register`; `cmd/semteams/main.go:142`
  `NewConfigManager`; `cmd/semteams/rule_config_service.go:33-92` `ConfigManager`, `HotReloadTarget`;
  `enable_graph_integration: true` (`configs/flow-bootstrap.json:317`); `entity_watch_buckets` set.
  `replace_owned` in five files: `configs/rules/dev-via-test/02c-plan-retry-stamp.json`, `02d-plan-retry-driver.json`,
  `07a-cbg-approved-to-coordinator.json`, `07c-cbg-retry-stamp.json`, `07d-cbg-retry-driver.json`.
- semsource: no `processor/rule` import (`grep -rln processor/rule --include=*.go`). semconnect: evidence files only.
- Dead keys set in consumer configs: none (`grep` over semboids/configs and semteams/configs).

## Same-class owners for the family registry (for PR 2's own inventory)

`component.Registry`, `payloadregistry.Registry`, the pin's rule-factory map (`rule_factory.go:124`),
`lifecycle.Manager` workflow registration, `vocabulary.RegisterPack`.

Intent check: no boundary moves. The port set is ruled (#8 Q15, D12).
