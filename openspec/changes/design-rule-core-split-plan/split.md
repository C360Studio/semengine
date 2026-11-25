# Split: `processor/rule` and `processor/rule/expression`

The detail behind `design.md`. Line figures are pin lines, production plus test, from `inventory.md` § Size; files
split between PRs are divided by the line ranges listed there, so those figures are estimates. Each PR compiles, is
tested and passes `task verify`; its own design re-measures what it carries.

## What each PR carries

| PR | Carries | Lines |
| --- | --- | --- |
| 1 | package `processor/rule/expression` whole | 1,417 + 2,110 |
| 2 | `Definition`, `Action` and the action types; the action-family registry; load-time validation, refusing an unknown action type naming type and file; rule-type registry without `init`; file loader | ~1,741 + 1,254 |
| 3 | `ExecutionContext`, the substitution files, `MatchState` and `StateTracker` | 1,850 + 2,890 |
| 4 | `ActionExecutor` and `Execute` dispatching core types and registered families; `publish`, `add_triple`, `remove_triple`, `update_triple`; `DenyVerdict`; the graph-event path as decided | ~899 + 2,498 |
| 5 | `reconcile_predicates` and projection contracts; `update_kv` with its load check and write guard; `lifecycle_transition`, `_complete`, `_fail` | ~1,310 + 2,918 |
| 6 | `ExpressionRule` and its factory, `Matches`, payload projection | 747 + 2,291 |
| 7 | `StatefulEvaluator`, entity evaluation fence, owner lane; cooldown per rule and entity in stored state; firing cap with a metric and a readable fact | 955 + 3,164 |
| 8 | `Processor` on `component.Lifecycle`, `Register` and its inputs (families, lifecycle manager), strict config, metrics, status | ~2,241 + 2,412 |
| 9 | message path: NATS and JetStream inputs, decode, settlement, the fire path | ~510 + 1,070 |
| 10 | `ENTITY_STATES` watch, bootstrap, generation, debounce, readiness; the entity message as a registered payload | ~1,679 + 3,684 |
| 11 | `CronScheduler`, `ScheduleTracker` (`RULE_SCHEDULES`), cron rules and metrics; the cron module | 1,494 + 2,994 |
| 12 | `ConfigManager`, `HotReloadTarget`, runtime apply and validate; activation facts | ~1,031 + 1,467 |
| 13 | the loop-supervision skill, its adapter, the reviewer line; `orchestration-check`, the orchestration contract sections, the "rules carry references" clause | docs |
| 14 | probe of NATS scheduled messages, then the deadline timer | own design |

Nine of the twelve port PRs run over the 3,000-line guide: 1 (3,527), 3 (4,740), 4 (3,397), 5 (4,228), 6 (3,038),
7 (4,119), 8 (4,653), 10 (5,363) and 11 (4,488). Most of each is tests of one owner. Under "drop", PRs 2, 4, 6 and 9
shrink by about 1,000 lines between them.

Carried: about 15,874 production and 28,752 test lines. Not carried: about 1,150 production lines (`publish_agent`,
`deny`/`approve`, the auditor, the agentic `Action` fields) and 5,690 test lines (their tests, and the owner-proof
test files, 1,156 lines).

## Issues

- #25 retires at PR 7 (`design.md`, "Proofs this plan changes"). #26 at PR 2. #27 at PR 4.
- #28 retires at PR 7, because the chain stops on `*DenyVerdict` in the evaluator (`stateful_evaluator.go:427`). PR 11
  carries cron's copy of the check (`cron_scheduler.go:610`).
- #294 retires at PR 5. All three guard sites land there: `actions.go:2264`, `kv_writer.go:86-92`, and the load check
  `config_validation.go:382-385`, which is why that check moves from PR 2 to PR 5.
- #702 retires at PR 7. PR 6 ports `ExpressionRule` without its in-memory cooldown (`expression_factory.go:31`).
  Nothing starts a rule before PR 8.
- #703 and #714: PRs 14 and 13. The closed #101's graph-event remainder is owner decision 1 in `design.md`.
- Stay open: #196 and #195 are adoption lists (the rule sites are adopted in the PRs that carry them). #350 (ruled
  "projection client only, for now": PR 5's reconcile path goes through it, PR 4's triple actions do not). #704, #24
  and #9 are epics or decisions. #29 is closed.

## Required corrections, by PR (binding basis in brackets)

- PR 1: the regex cache is built in `init` with an eviction callback `internal/cache` no longer has
  (`expression/regex_cache.go:16-27`) [build; owner ruling on eviction callbacks].
- PR 2: rule types register into a global map from `init` (`rule_factory.go:124-142`, `expression_factory.go:443`)
  [AGENTS.md: imports register nothing]. `test_rule_factory.go` is production code only tests use, registered at
  `init` (`:212-218`) [no back doors]; it moves into a `_test.go` file. `gojsonschema` enters `go.mod` for
  `schema_test.go`, or the test checks schemas another way. Under "refuse", `fire_every_n_events` is refused at load
  [surface audit (c)].
- PR 4: the core compiles with no `agentic*` or `governance` import [#27]. `graph.NewExactEntityReader`'s error is
  handled (`triple_mutator.go:31`) [build]. `ExecutionContext`'s substitution methods, read today only by
  `publish_agent` (`actions.go:1711`, `:1767`), become the family-facing contract, or are dropped; PR 2's design
  settles which.
- PR 5: the write-guard pair is read from `graph/kvcatalog` [build; #294]. A `lifecycle_*` action with no manager
  fails at fire time (`actions_lifecycle.go:12-14`); it is refused at load instead, in line with task 4.9.
- PR 8: the manager arrives at registration, not `deps.LifecycleManager` (`factory.go:160`) [#102 ruling D].
  Registration uses `Registry.Register(Registration)` (`factory.go:15`) and no `component.Metadata` (`processor.go:42`)
  [build]. The four dead keys are dropped: `MessageCache` would export `internal/cache.Config` (`config.go:27`), is
  written but never read (`message_handler.go:90-92`), and its default sets the refused `StatsInterval`
  (`config.go:245`); `buffer_window_size` and `alert_cooldown_period` are parsed and only logged
  (`rule_loader.go:154-165`); `consumer` has no reader [surface audit (b); unknown keys refused]. Collectors go
  through `metric.RegisterOrGet`, not `MustRegister` (`metrics.go:209`) [metric-registry rule]. Lifecycle state
  lives in `component.Lifecycle`. `events_published` is documented as what it counts after decision 1. `Stop` drains
  the JetStream consumers before the run context ends, proved through `Start` and `Stop` (with PR 9)
  [acknowledged durability].
- PR 9: the JetStream consumer is created through natsclient's policy entry point [AGENTS.md]. A message that fails
  to decode is acknowledged after a log line (`message_handler.go:67-71`, `processor.go:1141`): it is settled
  explicitly or declared as a drop [fail closed]. `recordError` labels errors by searching their text
  (`message_handler.go:465-495`); it gets typed labels at each call site [classification by error text].
  `total_triggered` counts what decision 1 says (`processor.go:1456`, `message_handler.go:227-231`) [a metric that
  stops moving without a cause is a silent degrade]. `fire_every_n_events` per decision 2. `vocabulary/rulepacks`
  (test-only, `rule_integration_test.go:820`) is replaced.
- PR 10: `entityStateToMinimalMessage` builds a `GenericJSON` carrying `time.Time` and `EntityState.Version`
  (`message_handler.go:376-384`); it moves to a registered payload with no version [#9 ruling; ADR-110]. Readiness
  uses main's API: `graph.BucketGraphStatus`, `NewPublisher`'s error, no `EnsureBucket`, `ProducerNames` or
  `reset_required` state (`readiness.go:83-224`). The watch uses `kvcatalog.OpenCatalogReader` and the keyed
  `UnmarshalEntityState` (`entity_watcher.go:113`, `:767`). The debounce set's constructor error is handled and it
  stops with `Shutdown(ctx)`, not `Close` (`processor.go:930`, `entity_watcher.go:980`) [build].
  `fire_every_n_events` per decision 2.
- PR 11: cron metrics go through `metric.RegisterOrGet` (`cron_metrics.go:173-179`) [build]. `robfig/cron/v3` v3.0.1
  enters `go.mod` with a ledger dependency line and a clean `task vuln`. `CronScheduler` keeps lifecycle state in
  `internal/lifecycleguard`. Its `Stop` joins an admitted fire before the `Start` context ends, proved through the
  scheduler's own `Start` and `Stop` [acknowledged durability].
- PR 12: `ConfigManager` keeps lifecycle state in `internal/lifecycleguard`.
- Every PR rewrites the tests it carries to the harness rules (53 sleeps in 12 files, one skip) and measures the
  test-side compile drift the inventory's probe did not cover.

Optional findings for the PR designs to dispose of: the map round trip `definitionToRuleMap`/`definitionFromMap`
(`rule_loader.go:39`, `config_validation.go:449`); four `NewActionExecutor*` constructors (`actions.go:803-894`) and
two `NewProcessor*` (`processor.go:250-255`).

## ADRs and guidance each PR carries

The owning PR for each ADR is in `inventory.md` § ADRs. Records not yet in `docs/adr` that a PR may port: 031 (11),
036 (2), 041 (1), 047 and 049 (5), 048 (3), 068 (10), and 076 decisions 1-3 and 5 (4, only under "move"). The rest,
which leave with `publish_agent` and deny/approve, are not ported, citing Q15.

- `processor/rule/docs/syntax.md` and `conditions.md` with PRs 1 and 6; `custom-rules.md` with PR 2; `actions.md`
  with PRs 4 and 5; `state-tracking.md` with PR 7; `configuration.md`, `operations.md` and the package `README.md`
  with PR 8; `entity-watching.md` and the rule half of `entity-or-bucket` with PR 10; `examples.md` with PR 12.
- The reviewer's `$entity.triple.*` clause with PR 3. "Rules carry references", the pin's "Rules and orchestration"
  and "Orchestration" sections and `orchestration-check` with PR 13.
