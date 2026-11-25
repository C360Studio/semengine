# Design: design-rule-core-split-plan

Base: `4ead6360978d008649e0eb5e869086dcac2b36f1`. Pin: SemStreams `8b99efe9`. Inventory: `inventory.md`.

## What this plan settles, and what it leaves

It settles the cut, the order, the issue each PR retires, and which designs go to Fable first. It does not settle any
PR's shape. Each PR is its own OpenSpec change with its own inventory (shape sweep, open issues, pin probe including
the lifecycle suite for its owner, ADRs, the test-side compile drift) and a design of about one page. Line figures
are pin lines, production plus test, from `inventory.md` § Size; split files are divided by the line ranges listed
there, so those PRs' figures are estimates.

## Options considered

- **A. Bottom-up layers (recommended).** Each PR adds a layer that compiles on the ones below. Review surfaces are
  clean and no earlier file is reopened. Cost: nothing a product can start exists until PR 8. That is the
  carried-in-tree precedent (architect contract, "One package per pull request").
- **B. Vertical first slice.** The processor plus the message path, `publish` and `lifecycle_transition` first,
  so semboids runs early, then fill in. Cost: the dispatcher, the evaluator and the processor are each reopened by
  three or four later PRs. Every early PR touches many files, and the early processor carries temporary refusals.
- **C. One PR per lifecycle owner** (`Processor`, `CronScheduler`, `ConfigManager`). Three PRs of 10,000 to 30,000
  lines each, which the guide exists to avoid.
- **D. Defer.** The milestone and epic #9's order already rank this next. Deferring keeps semteams and semboids on
  SemStreams.

## The graph-event path: an owner decision

#101's ruling did not port `graph/events.go` and left rule's use of it to this port: "moves to the mutation-request
protocol or is dropped" (#101 comment 6126673987). At the pin a matching message rule builds an update for its
trigger entity and publishes it to `graph.events.*`, which nothing subscribes to (`inventory.md`, compile probe).
The choice reaches PR 2 (the exported `Rule` interface's `ExecuteEvents`, `interfaces.go:41`), PR 4 (`publisher.go`,
`graph_event_identity.go`), PR 6 (`expression_factory.go:329-366`) and PR 9 (`message_handler.go:207-230`).

- **Drop (recommended).** `ExecuteEvents`, the `Event` type, the graph-event publisher and the trigger identity are
  not carried (about 200 production and 800 test lines). ADR-076 is not ported, with that reason. A firing still
  runs its actions and still sends the `rule_events` port notification. Nothing a consumer reads changes.
- **Move.** Each firing becomes an entity update through graph-ingest's mutation requests
  (`internal/graphmutation/protocol.go`), on the ADR-076 identity, which PR 4 ports. That is a graph write per firing
  that nothing at the pin ever applied, so PR 4's design would also need its write budget and owner.

PR 2's design is not written before this is answered.

## The split (option A)

| PR | Change id | Carries | Lines | Retires | Waits for | Fable first |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `rule-core-01-expression` | package `processor/rule/expression` whole | 1,417 + 2,110 | none | none | no |
| 2 | `rule-core-02-definitions` | `Definition`, `Action` and the action types; the action-family registry; load-time validation, refusing an unknown action type naming type and file; rule-type registry without `init`; file loader | ~1,741 + 1,254 | #26 | 1, the graph-event decision | **yes**: new exported framework surface (#25) |
| 3 | `rule-core-03-context` | `ExecutionContext`, the substitution files, `MatchState` and `StateTracker` | 1,850 + 2,890 | none | 2 | no |
| 4 | `rule-core-04-graph-actions` | `ActionExecutor` and `Execute` dispatching core types and registered families; `publish`, `add_triple`, `remove_triple`, `update_triple`; `DenyVerdict`; the graph-event path as decided | ~899 + 2,498 | #27 | 3 | **yes**: dispatch surface; graph writes |
| 5 | `rule-core-05-state-actions` | `reconcile_predicates` and projection contracts; `update_kv` with its load check and write guard; `lifecycle_transition`, `_complete`, `_fail` | ~1,310 + 2,918 | #25, #294 | 4 | **yes**: writes into framework-owned buckets and lifecycle state |
| 6 | `rule-core-06-expression-rules` | `ExpressionRule` and its factory, `Matches`, payload projection | 747 + 2,291 | none | 3 | no |
| 7 | `rule-core-07-evaluator` | `StatefulEvaluator`, entity evaluation fence, owner lane; cooldown per rule and entity in stored state; firing cap with a metric and a readable fact | 955 + 3,164 | #28, #702 | 5, 6 | **yes**: durability (actions-then-persist, replay at bootstrap) |
| 8 | `rule-core-08-processor` | `Processor` on `component.Lifecycle`, `Register` and its inputs (families, lifecycle manager), strict config, metrics, status | ~2,241 + 2,412 | none | 7 | **yes**: boot path; connection ownership; registration surface |
| 9 | `rule-core-09-message-input` | message path: NATS and JetStream inputs, decode, settlement | ~510 + 1,070 | none | 8 | **yes**: acknowledgement and durability |
| 10 | `rule-core-10-entity-input` | `ENTITY_STATES` watch, bootstrap, generation, debounce, readiness; the entity message as a registered payload | ~1,679 + 3,684 | none | 9 | **yes**: connection ownership; readiness |
| 11 | `rule-core-11-cron` | `CronScheduler`, `ScheduleTracker` (`RULE_SCHEDULES`), cron rules and metrics; the cron module | 1,494 + 2,994 | none | 8 | **yes**: durable timer; its own lifecycle owner |
| 12 | `rule-core-12-hot-reload` | `ConfigManager`, `HotReloadTarget`, runtime apply and validate; activation facts | ~1,031 + 1,467 | none | 10 | **yes**: boot path; writer of `rules.*` in the config bucket |
| 13 | `skill-714-loop-supervision` | the skill, its adapter, the reviewer line; `orchestration-check`, the orchestration contract sections, the "rules carry references" clause | docs | #714 | 7 | no |
| 14 | `feat-703-deadline-timer` | probe of NATS scheduled messages, then the timer | own design | #703 | 12 (probe: none) | **yes**: durability; new surface; ADR-111 Decision 4 |

Carried: about 15,874 production and 28,752 test lines. Not carried: about 1,150 production lines (`publish_agent`,
`deny`/`approve`, the auditor, the agentic `Action` fields) and 5,690 test lines (their tests, and the owner-proof
test files, 1,156 lines, that the lifecycle suite replaces). Under "drop", PRs 2, 4, 6 and 9 shrink by the
graph-event lines above.

## Merge order

1, 2, 3; then 4 and 6 in parallel (5 follows 4); 7; 8; then 9 and 11 in parallel; 10 after 9; 12 after 10. PR 13
after 7. PR 14's probe at any time, its code after 12. PRs from 8 on all edit `processor.go`; the later one merges
`origin/main`. semboids qualifies after 9 (its three packs use `publish` and `lifecycle_transition` on a core NATS
input). semteams qualifies after 12, once it registers `publish_agent` itself and migrates its five `replace_owned`
files.

## Issues

- #25 retires at PR 5. PR 2 adds the registry and refuses an unregistered type at load, naming type and file
  (owner, #8, task 4.9). PR 4 dispatches registered families. PR 5 completes the core types, so semboids' 7 actions
  load and run with no family registered. PRs 2 and 4 name #25 without a closing keyword.
- #26 retires at PR 2. The agentic half of its property (family decode equals `agentic.ToolChoice` decode) cannot
  run here. PR 2's design states the byte-preservation check that replaces it, against semteams' fixtures at
  `a331910e` (#26 comment).
- #27 retires at PR 4, with the import guard. "semteams 40/40 dispatched" is semteams' own check at its cutover;
  PR 4's design says so.
- #28 retires at PR 7, because the chain stops on `*DenyVerdict` in the evaluator (`stateful_evaluator.go:427`). PR 11
  carries cron's copy of the check (`cron_scheduler.go:610`).
- #294 retires at PR 5. All three guard sites land there: `actions.go:2264`, `kv_writer.go:86-92`, and the load check
  `config_validation.go:382-385`, which is why that check moves from PR 2 to PR 5.
- #702 retires at PR 7. PR 6 ports `ExpressionRule` without its in-memory cooldown (`expression_factory.go:31`), so
  nothing carries the shared timestamp. Nothing starts a rule before PR 8.
- #703 and #714: PRs 14 and 13. #101 (closed): its graph-event remainder is decided above.
- Stay open: #196 and #195 are adoption lists (the rule sites are adopted in the PRs that carry them). #350 (ruled
  "projection client only, for now": PR 5's reconcile path goes through it, PR 4's triple actions do not). #704, #24
  and #9 are epics or decisions. #29 is closed.

## Required corrections, by PR (binding basis in brackets)

- PR 1: the regex cache is built in `init` with an eviction callback `internal/cache` no longer has
  (`expression/regex_cache.go:16-27`) [build; owner ruling on eviction callbacks].
- PR 2: rule types register into a global map from `init` (`rule_factory.go:124-142`, `expression_factory.go:443`)
  [AGENTS.md: imports register nothing]. `test_rule_factory.go` is production code only tests use, registered at
  `init` (`:212-218`) [no back doors]; it moves into a `_test.go` file. `gojsonschema` enters `go.mod` for
  `schema_test.go`, or the test checks schemas another way.
- PR 4: the core compiles with no `agentic*` or `governance` import [#27]. `graph.NewExactEntityReader`'s error is
  handled (`triple_mutator.go:31`) [build]. `ExecutionContext`'s substitution methods, read today only by
  `publish_agent` (`actions.go:1711`, `:1767`), become the family-facing contract, or are dropped. PR 2's design
  settles which.
- PR 5: the write-guard pair is read from `graph/kvcatalog` [build; #294]. A `lifecycle_*` action with no manager
  fails at fire time (`actions_lifecycle.go:12-14`); it is refused at load instead, in line with task 4.9.
- PR 8: the manager arrives at registration, not `deps.LifecycleManager` (`factory.go:160`) [#102 ruling D].
  Registration uses `Registry.Register(Registration)` (`factory.go:15`) and no `component.Metadata` (`processor.go:42`)
  [build]. The four dead keys are dropped. `MessageCache` would export `internal/cache.Config` (`config.go:27`) and
  is written but never read (`message_handler.go:90-92`); its default sets the refused `StatsInterval`
  (`config.go:245`). `buffer_window_size` and `alert_cooldown_period` are parsed and only logged
  (`rule_loader.go:154-165`). `consumer` has no reader [surface audit (b); unknown keys refused]. Collectors go
  through `metric.RegisterOrGet`, not `MustRegister` (`metrics.go:209`) [metric-registry rule]. Lifecycle state
  lives in `component.Lifecycle`.
- PR 9: the JetStream consumer is created through natsclient's policy entry point [AGENTS.md]. A message that fails
  to decode is acknowledged after a log line (`message_handler.go:67-71`, `processor.go:1141`): it is settled
  explicitly or declared as a drop [fail closed]. `recordError` labels errors by searching their text
  (`message_handler.go:465-495`); it gets typed labels at each call site [classification by error text].
- PR 10: `entityStateToMinimalMessage` builds a `GenericJSON` carrying `time.Time` and `EntityState.Version`
  (`message_handler.go:376-384`); it moves to a registered payload with no version [#9 ruling; ADR-110]. Readiness
  uses main's API: `graph.BucketGraphStatus`, `NewPublisher`'s error, no `EnsureBucket`, `ProducerNames` or
  `reset_required` state (`readiness.go:83-224`). The watch uses `kvcatalog.OpenCatalogReader` and the keyed
  `UnmarshalEntityState` (`entity_watcher.go:113`, `:767`). The debounce set's constructor error is handled and its
  lifetime ends with its context, not `Close` (`processor.go:930`, `entity_watcher.go:980`) [build].
- PR 11: cron metrics go through `metric.RegisterOrGet` (`cron_metrics.go:173-179`) [build]. `robfig/cron/v3` v3.0.1
  enters `go.mod` with a ledger dependency line and a clean `task vuln`. `CronScheduler` keeps lifecycle state in
  `internal/lifecycleguard`.
- PR 12: `ConfigManager` keeps lifecycle state in `internal/lifecycleguard`.
- Every PR rewrites the tests it carries to the harness rules (53 sleeps in 12 files, one skip) and measures the
  test-side compile drift the inventory's probe did not cover. `vocabulary/rulepacks` (test-only,
  `rule_integration_test.go:820`) is replaced in PR 9.

Optional findings for the PR designs to dispose of: the map round trip `definitionToRuleMap`/`definitionFromMap`
(`rule_loader.go:39`, `config_validation.go:449`); four `NewActionExecutor*` constructors (`actions.go:803-894`) and
two `NewProcessor*` (`processor.go:250-255`).

## ADRs and guidance each PR carries

The owning PR for each ADR is in `inventory.md` § ADRs. Records not yet in `docs/adr` that a PR may port: 031 (11),
036 (2), 041 (1), 047 and 049 (5), 048 (3), 068 (10), 076 (4, only under "move"); the rest that leave with
`publish_agent` and deny/approve are not ported, citing Q15. Guidance:

- `processor/rule/docs/syntax.md` and `conditions.md` with PRs 1 and 6; `custom-rules.md` with PR 2;
  `actions.md` with PRs 4 and 5; `state-tracking.md` with PR 7; `configuration.md`, `operations.md` and the
  package `README.md` with PR 8; `entity-watching.md` and the rule half of `entity-or-bucket` with PR 10;
  `examples.md` with PR 12.
- The reviewer's `$entity.triple.*` clause with PR 3. "Rules carry references", the pin's "Rules and orchestration"
  and "Orchestration" sections and `orchestration-check` with PR 13.

## Who carries the surface (adopter seam)

semboids calls `rule.Register` (`componentregistry/register.go:34`) and sets `Dependencies.LifecycleManager`
(`cmd/semboids/main.go:200`). semteams calls `rule.Register`, `NewConfigManager`, `ConfigManager` and
`HotReloadTarget` in production (`internal/runtimecatalog/components.go:36`, `cmd/semteams/main.go:142`,
`rule_config_service.go:33-92`). What they must know after the port:

- register any family their packs use, `publish_agent` for semteams;
- hand the lifecycle manager to the rule registration;
- drop the four dead keys.

Doing nothing gives a boot error for each: an unregistered type with type and file named, a `lifecycle_*` action
with no manager (PR 5's correction), or an unknown key. None of these fails silently. PR 2's Fable read decides the
registration surface for both families and the manager, so PR 8 implements an approved shape.
