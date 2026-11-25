# Design: design-rule-core-split-plan

Base: `4ead6360978d008649e0eb5e869086dcac2b36f1`. Pin: SemStreams `8b99efe9`. Inventory: `inventory.md`.

## What this plan settles, and what it leaves

It settles the cut, the order, the issue each PR retires, and which designs go to Fable first. It does not settle any
PR's shape. Each PR is its own OpenSpec change with its own inventory (shape sweep, open issues, pin probe including
the lifecycle suite for its owner, ADRs) and a design of about one page. Line figures below are pin lines, production
plus test, of what each PR is expected to carry: measured per file, estimated where a file is split (`actions.go`,
`processor.go`, `config_validation.go`, `message_handler.go`, `actions_test.go`).

## Options considered

- **A. Bottom-up layers (recommended).** Each PR adds a layer that compiles on the ones below. Review surfaces are
  clean and no earlier file is reopened. Cost: nothing a product can start exists until PR 8. That is the carried-in-tree
  precedent (architect contract, "One package per pull request").
- **B. Vertical first slice.** The processor plus the message path, `publish` and `lifecycle_transition` first,
  so semboids runs early, then fill in. Cost: the dispatcher, the evaluator and the processor are each reopened by
  three or four later PRs. Every early PR touches many files, and the early processor carries temporary refusals.
- **C. One PR per lifecycle owner** (`Processor`, `CronScheduler`, `ConfigManager`). Three PRs of 10,000 to 30,000
  lines each, which the guide exists to avoid.
- **D. Defer.** The milestone and epic #9's order already rank this next. Deferring keeps semteams and semboids on
  SemStreams.

## The split (option A)

| PR | Change id | Carries | Lines (est.) | Retires | Waits for | Fable first |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `rule-core-01-expression` | package `processor/rule/expression` whole | 1,417 + 2,110 | none | none | no |
| 2 | `rule-core-02-definitions` | `Definition`, `Action` and the action types; the action-family registry; load-time validation, refusing an unknown action type naming type and file; rule-type registry without `init`; file loader | ~1,730 + 1,250 | #26 | 1 | **yes**: new exported framework surface (#25) |
| 3 | `rule-core-03-context` | `ExecutionContext`, the substitution files, `MatchState` and `StateTracker` | 1,850 + 2,890 | none | 2 | no |
| 4 | `rule-core-04-graph-actions` | `ActionExecutor` and `Execute` dispatching core types and registered families; `publish`, `add_triple`, `remove_triple`, `update_triple`; graph-event publisher; `DenyVerdict` | ~900 + 2,500 | #27 | 3 | **yes**: dispatch surface; graph writes |
| 5 | `rule-core-05-state-actions` | `reconcile_predicates` and projection contracts; `update_kv` and its write guard; `lifecycle_transition`, `_complete`, `_fail` | ~1,310 + 2,680 | #25, #294 | 4 | **yes**: writes into framework-owned buckets and lifecycle state |
| 6 | `rule-core-06-expression-rules` | `ExpressionRule` and its factory, `Matches`, payload projection | ~750 + 2,290 | none | 3 | no |
| 7 | `rule-core-07-evaluator` | `StatefulEvaluator`, entity evaluation fence, owner lane; cooldown per rule and entity in stored state; firing cap with a metric and a readable fact | 955 + 3,160 | #28, #702 | 5, 6 | **yes**: durability (actions-then-persist, replay at bootstrap) |
| 8 | `rule-core-08-processor` | `Processor` on `component.Lifecycle`, `Register` and its inputs (families, lifecycle manager), strict config, metrics, status | ~2,440 + 2,200 | none | 7 | **yes**: boot path; connection ownership; registration surface |
| 9 | `rule-core-09-message-input` | message path: NATS and JetStream inputs, decode, settlement | ~500 + 1,070 | none | 8 | **yes**: acknowledgement and durability |
| 10 | `rule-core-10-entity-input` | `ENTITY_STATES` watch, bootstrap, generation, debounce, readiness; the entity message as a registered payload | ~1,680 + 3,680 | none | 9 | **yes**: connection ownership; readiness |
| 11 | `rule-core-11-cron` | `CronScheduler`, `ScheduleTracker` (`RULE_SCHEDULES`), cron rules and metrics | 1,494 + 2,994 | none | 8 | **yes**: durable timer; its own lifecycle owner |
| 12 | `rule-core-12-hot-reload` | `ConfigManager`, `HotReloadTarget`, runtime apply and validate; activation facts | ~1,030 + 1,467 | none | 10 | **yes**: boot path; writer of `rules.*` in the config bucket |
| 13 | `skill-714-loop-supervision` | the skill, its adapter, the reviewer line; the pin's `orchestration-check` skill and orchestration contract sections | docs | #714 | 7 | no |
| 14 | `feat-703-deadline-timer` | probe of NATS scheduled messages, then the timer | own design | #703 | 12 (probe: none) | **yes**: durability; new surface; ADR-111 Decision 4 |

The table carries about 44,300 pin lines. The rest of the 51,467 is not carried: `publish_agent` and its helpers
(`actions.go:565-759`, `:1445-2090`), `deny`/`approve` (`:2101-2252`), `verdict_auditor.go`, their tests (about
4,500 lines), and the owner-proof test files (1,184 lines) that the lifecycle suite replaces.

## Merge order

1, 2, 3; then 4 and 6 in parallel (5 follows 4); 7; 8; then 9 and 11 in parallel; 10 after 9; 12 after 10. PR 13
after 7. PR 14's probe at any time, its code after 12. PRs from 8 on all edit `processor.go`; the later one merges
`origin/main`. semboids qualifies after 9 (its three packs use `publish` and `lifecycle_transition` on a core NATS
input). semteams qualifies after 12, once it registers `publish_agent` itself and migrates `replace_owned`.

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
- #294 retires at PR 5, with all three guard sites in front of the porter.
- #702 retires at PR 7. PR 6 ports `ExpressionRule` without its in-memory cooldown (`expression_factory.go:31`), so
  nothing carries the shared timestamp. Nothing starts a rule before PR 8.
- #703 and #714: PRs 14 and 13.
- Stay open: #196 and #195 are adoption lists (the rule sites are adopted in the PRs that carry them). #350 (ruled
  "projection client only, for now": PR 5's reconcile path goes through it, PR 4's triple actions do not). #704, #24
  and #9 are epics or decisions. #29 is closed.

## Required corrections, by PR (binding basis in brackets)

- PR 1: the regex cache is built in `init` with an eviction callback `internal/cache` no longer has
  (`expression/regex_cache.go:16-27`) [build; owner ruling on eviction callbacks].
- PR 2: rule types register into a global map from `init` (`rule_factory.go:124-142`, `expression_factory.go:443`)
  [AGENTS.md: imports register nothing]. `test_rule_factory.go` is production code only tests use, registered at
  `init` (`:212-218`) [no back doors]. Its tests move it into a `_test.go` file.
- PR 4: the core compiles with no `agentic*` or `governance` import [#27]. `ExecutionContext`'s substitution
  methods, read today only by `publish_agent` (`actions.go:1711`, `:1767`), become the family-facing contract, or
  are dropped. PR 2's design settles which.
- PR 5: a `lifecycle_*` action with no manager fails at fire time (`actions_lifecycle.go:12-14`). It is refused at
  load instead, in line with task 4.9.
- PR 8: the manager arrives at registration, not `deps.LifecycleManager` (`factory.go:160`) [#102 ruling D]. The four
  dead keys are dropped. `MessageCache` would export `internal/cache.Config` (`config.go:27`) and is written but never
  read (`message_handler.go:90-92`). `buffer_window_size` and `alert_cooldown_period` are parsed and only logged
  (`rule_loader.go:154-165`). `consumer` has no reader [surface audit (b); unknown keys refused]. Lifecycle state lives
  in `component.Lifecycle`.
- PR 9: the JetStream consumer is created through natsclient's policy entry point [AGENTS.md]. A message that fails
  to decode is acknowledged after a log line (`message_handler.go:67-71`, `processor.go:1141`): it is settled
  explicitly or declared as a drop [fail closed].
- PR 10: `entityStateToMinimalMessage` builds a `GenericJSON` carrying `time.Time` (`message_handler.go:376-384`). It
  moves to a registered payload [#9 ruling; architect contract]. Readiness uses `main`'s API, not
  `EnsureBucket`/`ProducerNames` (`readiness.go:191`, `:224`).
- PR 11 and PR 12: `CronScheduler` and `ConfigManager` keep lifecycle state in `internal/lifecycleguard`.
- Every PR rewrites the tests it carries to the harness rules: 53 sleeps in 12 files, one skip.

Optional findings for the PR designs to dispose of: the map round trip `definitionToRuleMap`/`definitionFromMap`
(`rule_loader.go:39`, `config_validation.go:449`); four `NewActionExecutor*` constructors (`actions.go:803-894`) and
two `NewProcessor*` (`processor.go:250-255`).

## ADRs each PR design cites (pin numbers; "ported" means already in `docs/adr`)

1 and 6: ADR-041. 2: ADR-093, ADR-094 (ported), ADR-046 (its `for_each` fields leave with `publish_agent`). 3: ADR-048,
ADR-111. 4: ADR-055, ADR-091, ADR-054, ADR-060 (ported), ADR-102 (ported); ADR-039, ADR-053, ADR-028 and ADR-045
leave with the uncarried actions (ruled, Q15). 5: ADR-091, ADR-074 (ported), ADR-047 and ADR-049 (not ported with
`pkg/lifecycle`; PR 5 ports or says why). 7: ADR-111, ADR-113. 8: ADR-108, ADR-109, ADR-113, ADR-095, ADR-083, ADR-088,
ADR-094 (ported). 9: ADR-112. 10: ADR-083, ADR-088, ADR-109. 11: ADR-031, ADR-111. 12: ADR-094 (its rule-activation
half). 14: ADR-111, ADR-112.

## Who carries the surface (adopter seam)

semboids calls `rule.Register` (`componentregistry/register.go:34`) and sets `Dependencies.LifecycleManager`.
semteams calls `rule.Register`, `NewConfigManager`, `ConfigManager` and `HotReloadTarget` in production
(`internal/runtimecatalog/components.go:36`, `cmd/semteams/main.go:142`, `rule_config_service.go:33-92`). What
they must know after the port:

- register any family their packs use, `publish_agent` for semteams;
- hand the lifecycle manager to the rule registration;
- drop the four dead keys.

Doing nothing gives a boot error for each: an unregistered type with type and file named, a `lifecycle_*` action
with no manager (PR 5's correction), or an unknown key. None of these fails silently. PR 2's Fable read decides the
registration surface for both families and the manager, so PR 8 implements an approved shape.
