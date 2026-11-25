# Inventory: design-rule-core-target-shape

Base: `c729fecdb06596d83965c178210014a9430495a2`. Pin copy: `gh api repos/C360Studio/semstreams/tarball/8b99efe9`,
unpacked to a scratch directory. Products read at semteams `c8571b34` and semboids `37dbdb0` (`git archive <sha>
configs`, read-only). Scope: the four types the owner named (PR #752 comment 7162146299); no tree-wide sweep.

## 1. The rule definition

- `rule_factory.go:16-70`: `type Definition struct {` (ID … Actions; 20 JSON keys).
- `rule_factory.go:79-91`: `type Factory interface {`; `:94-103` `type Dependencies struct {`; `:105-121` `Schema`,
  `Example`; `:130-215` `RegisterRuleFactory`, `UnregisterRuleFactory`, `GetRuleFactory`, `GetRegisteredRuleTypes`,
  `GetRuleSchemas`, `CreateRuleFromDefinition`; `:217-290` `BaseRuleFactory`.
- `expression_factory.go:443-449`: `func init() { … RegisterRuleFactory("expression", factory)`, the one production
  registration.
- `rule_loader.go:206`: `if def.Type == CronRuleType {`; cron bypasses the factory (comment `:200-205`).
- `interfaces.go:14-26` `type Event interface`, `:30-42` `type Rule interface` (with `ExecuteEvents`), `:47-50`
  `type EntityStateEvaluator interface`. Production implementers of `EvaluateEntityState`: `ExpressionRule` only
  (`expression_factory.go:213`); the others are `test_rule_factory.go:101` and a doc example.
- Second decode spelling: `rule_loader.go:39` `func (rp *Processor) definitionToRuleMap(def Definition)
  map[string]any`; `config_validation.go:449` `func definitionFromMap(ruleID string, ruleMap map[string]any)
  (Definition, error)`.
- A rule that fails to load is logged and skipped: `rule_loader.go:228-233`
  `continue // Skip invalid rules but continue loading others`.
- Product readers of the factory surface: none (`git grep -E "GetRuleSchemas|RegisterRuleFactory|GetRegisteredRuleTypes|rule\.Schema"`
  at both product SHAs: 0).
- Rule types in product rule JSON: `expression` 91, `cron` 1.

## 2. An action and its handler

- `actions.go:89-447`: `type Action struct {`, 35 JSON keys (counted from the tags).
- `actions.go:916-945`: `func (e *ActionExecutor) Execute(…)`, a closed `switch action.Type`, `default: unknown action
  type`.
- Four constructors: `actions.go:803`, `:820`, `:843`, `:894`. Setters: `:760` `setMetrics`, `:769` `SetToolRegistry`,
  `:778` `SetLifecycleManager`, `:787` `SetVerdictAuditor`, `:792` `SetPredicateReconciler`, `:796`
  `setProjectionTargets`. Production build site: `processor.go:665-679`, inside `Start`'s state-tracker setup. The
  reconciler arrives after construction from `service/rule_pack_bind.go:78` (that file is on main).
- With graph integration off, triple actions are dropped: `processor.go:673-674` builds the executor with a nil
  mutator, and `actions.go:538` documents a nil mutator as "logged but not persisted".
- Firing-cap identity: `action_id.go:36-41` `effectiveID` (author `id`, else a fingerprint). `:63-67` fingerprint over
  rule ID, type, a per-type distinguisher and `role`. `:85-104` `actionDistinguisher`, a second per-type switch.
- `for_each`/`for_each_var` have one reader, `publish_agent` (`actions.go:1707-1742`).
- Product action keys (`configs/**/*.json`, `on_enter|on_exit|while_true|on_recovery|actions`): 131 actions.
  - Types: add_triple 57, publish_agent 42, replace_owned 9 (not a pin type), lifecycle_transition 9,
    reconcile_predicates 6, publish 6, remove_triple 2.
  - Every key is a pin `Action` field, and the code for its type reads it (measured for add_triple, remove_triple,
    publish, lifecycle_*).
  - `id` set on 12 actions, `max_iterations` on 6; semboids sets neither.
- Product code that builds executors (tests only): semteams `test/contract/rule_family_consumer_test.go:155`,
  `research_loop_budget_test.go:65,230,473,502`, `autoresearch_reply_integration_test.go:65`,
  `cmd/semteams/approvalpause/rule_order_test.go:50` (`NewActionExecutor`/`NewActionExecutorFull`, then `Execute`).

## 3. The fire path (every copy)

- Evaluator: `stateful_evaluator.go:368-456` `runActions`. `when` at `:379`; cap at `:398-424` (count incremented
  before `Execute`); deny stops the list at `:427`; any other error is logged and counted, and the loop continues.
- Cron: `cron_scheduler.go:491-525`. Every-N on an in-memory `tickCount` at `:491`, then cooldown on
  `lastFiredNanos`, restored from `RULE_SCHEDULES` at `:332`. Action loop `:608-640`, stamp `:644`. A grep of
  `cron_scheduler.go`, `cron_rule.go` and `schedule_tracker.go` finds no reader of `When`, `effectiveID` or
  `ActionIterations`: cron actions ignore `when` and the firing cap. This is a code read, not a probe run.
- Message path: `message_handler.go:40-46` `shouldFireAction` over a per-rule in-memory `atomic.Int64`
  (`snapshotRules`, `:18-34`), applied at `:183-196`, in front of the graph event only.
- Cooldown: `expression_factory.go:31` `lastTriggered time.Time`, checked `:161`/`:219`, stamped `:363` (#702).

## 4. The stored state record

- `state_tracker.go:30-63`: `type MatchState struct {`: RuleID, EntityKey, IsMatching, LastTransition, TransitionAt,
  SourceRevision, LastChecked, Iteration, MaxIterations, FieldValues, ActionIterations.
- Key: `state_tracker.go:226-228` `ruleID + "." + entityKey`; pair key `:258-263`; bucket `processor.go:629`
  `"RULE_STATE"`.
- Order: actions, then `stateTracker.Set` (`stateful_evaluator.go:247-249`); ADR-111 Decision 2 calls this "no claim".
- Cron state: `schedule_tracker.go:57-76` `LastFireRecord` (rule_id, schedule_spec, last_fired_at) in
  `RULE_SCHEDULES`.

## Adjacent claims

- Open PRs: #754 (this claim) and #753 (`claude/rule-core-01-expression`, PR 1). No overlap: the page only names
  `expression.ConditionExpression`, which #753 ports. PR 1 does not wait for this page (owner).
- Issues: #25 (wording already replaced, comment 7162378515), #26 (byte proof, PR 2), #28 (`*DenyVerdict` stops the
  chain; `deny.go` stays), #702 (cooldown and cap, PR 7); #8 Q15 (no `publish_agent`/`deny`/`approve` in core).
- Closest existing instance of "a product hands a component its own implementation at registration":
  `processor/graph-embedding/component.go:220-240`, `Register` plus `RegisterWithEmbedder`. Adopted in `design.md`.
- ADRs: 111 (Decisions 2, 3, 5 bear on `Fire` and the record); 031 (cron, PR 11); 041 (one condition evaluator,
  `when`). This change ports no ADR.
