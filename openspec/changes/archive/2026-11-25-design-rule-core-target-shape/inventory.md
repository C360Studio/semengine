# Inventory: design-rule-core-target-shape

Base: `c729fecdb06596d83965c178210014a9430495a2`. Pin copy: `gh api repos/C360Studio/semstreams/tarball/8b99efe9`,
unpacked to a scratch directory. Products read at semteams `c8571b34` and semboids `37dbdb0` (`git archive <sha>
configs`, `git show <sha>:<path>`, read-only). Scope: the four types the owner named (PR #752 comment 7162146299).

## 1. The rule definition

- `rule_factory.go:16-70`: `type Definition struct {` (20 JSON keys).
- `rule_factory.go:79-285`, not carried: `Factory` `:79-91`, `Dependencies` `:94-103`, `Schema`/`Example` `:105-121`,
  registry functions `:130-215`, `BaseRuleFactory` `:217-285`.
  - `BaseRuleFactory.ValidateExpression` (`:240-285`) validates conditions.
  - `isValidOperator` (`:287-290`; callers `config_validation.go:253`, `expression_factory.go:411`) checks operators.
  - Both checks are kept and move to `Load`'s expression arm (PR 6), unless PR 1 (#753) carries operator validation
    in `expression`.
- `expression_factory.go:443-449`: `func init() { … RegisterRuleFactory("expression", factory)`, the one production
  registration. Cron bypasses the factory: `rule_loader.go:206` `if def.Type == CronRuleType {`.
- Interfaces:
  - `interfaces.go:14-26` `type Event interface`; `:30-42` `type Rule interface` (with `ExecuteEvents`); `:47-50`
    `type EntityStateEvaluator interface`.
  - The only production implementer of `EvaluateEntityState` is `ExpressionRule` (`expression_factory.go:213`).
- Second decode spelling: `rule_loader.go:39` `definitionToRuleMap`; `config_validation.go:449` `definitionFromMap`.
- A rule that fails to load is logged and skipped: `rule_loader.go:228-233`
  `continue // Skip invalid rules but continue loading others`.
- No product reads the factory surface:
  `git grep -E "GetRuleSchemas|RegisterRuleFactory|GetRegisteredRuleTypes|rule\.Schema"` at both product SHAs
  returns 0.
- Rule types in product rule JSON: `expression` 91, `cron` 1.

## 2. An action and its handler

- `actions.go:89-444`: `type Action struct {`, 35 JSON keys.
- `actions.go:916-945`: `Execute`, a closed `switch action.Type`, with `default: unknown action type`.
- Constructors and setters:
  - Four constructors: `actions.go:803`, `:820`, `:843`, `:894`.
  - Six setters: `:760`, `:769`, `:778`, `:787`, `:792`, `:796`.
  - The production build is in `Start`'s state-tracker setup (`processor.go:665-679`).
  - With graph integration off the mutator is nil (`:673-674`), which drops triple writes ("logged but not
    persisted", `actions.go:538`).
- Load-time action checks: `ValidateDefinition` (`config_validation.go:273-292`) → `validateActionLists`
  (`:337-434`):
  - reserved `user.response.>` subject on `publish`/`publish_agent`/`approve` (`:340`; type set in
    `user_response_subject_reservation.go:16-22`);
  - `related_loops` (publish_agent only);
  - a literal, declared predicate on triple actions;
  - `update_kv` refuses framework-owned buckets;
  - `max_iterations >= 0`;
  - `run_scope` and `filesystem_policy` (publish_agent).
- `ValidateDefinition`'s five callers. Three have no executor:
  - `Config.Validate` (`config.go:169`);
  - `ConfigManager.SaveRule` (`kv_config_integration.go:316`), which semteams runs (`cmd/semteams/main.go:142`,
    `rule_config_service.go:33-92`);
  - preflight `prepareInitialRules` (`rule_loader.go:103-147`, call at `:117`).
  - The other two are `config_validation.go:554` and `rule_factory.go:196`.
- Pack-wide projection derivation:
  - `deriveEffectiveProjectionContracts` and `buildProjectionTargetIndex` (`projection_derivation.go:31-100`, run
    from `rule_loader.go:125-134`) and `validateRuleReconcileActions` all decode the typed
    `ProjectionGroup`/`Predicate` of every `reconcile_predicates` action across all rules.
  - This runs before any reconciler exists.
- Reconciler order:
  1. `PreflightProjectionMutations` loads the rules (`processor.go:455-459`).
  2. `ProjectionBindings` returns the derived contracts (`:443-450`).
  3. Main's `service.ConfigureRulePackMutations` builds the client and calls `SetPredicateReconciler`
     (`service/rule_pack_bind.go:79-104`; the method is required by main's `service.ProjectionBinder`, `:16-20`).
  4. `Start` builds the executor and pushes the reconciler and the target index into it (`processor.go:723-725`).
- Fire-time dependencies a product's `publish_agent` uses:
  - the rule's triple writer, `newTripleMutator` (`triple_mutator.go:27`), which records the rule's own revisions so
    it does not re-fire on its own writes; `publish_agent` writes through it at `actions.go:2062-2073`;
  - the reserved-subject check after substitution (`actions.go:1159`, `:1768`, `:2170`).
- `lifecycle_*` with no manager fails at fire, not at load (`requireLifecycleManager`, `actions_lifecycle.go:166-171`).
- Firing-cap identity: `action_id.go:36-41` `effectiveID`; `:63-67` the fingerprint; `:85-104` a per-type switch.
- `for_each`/`for_each_var` have one reader, `publish_agent` (`actions.go:1707-1742`).
- Product actions: 131.
  - Types: add_triple 57, publish_agent 42, replace_owned 9 (not a pin type), lifecycle_transition 9,
    reconcile_predicates 6, publish 6, remove_triple 2.
  - Every key is a pin `Action` field read by its type (checked for add_triple, remove_triple, publish,
    lifecycle_*).
  - `id` set on 12, `max_iterations` on 6; semboids sets neither.
- Product code that builds executors (tests): semteams `rule_family_consumer_test.go:155`,
  `research_loop_budget_test.go:65,230,473,502` (nil mutator), `autoresearch_reply_integration_test.go:65`,
  `approvalpause/rule_order_test.go:50`.
- Product hot-reload seam: semteams holds `*rulepkg.ConfigManager` and `[]rulepkg.HotReloadTarget`
  (`rule_config_service.go:34-35`, `:92`). `HotReloadTarget`'s methods take `map[string]any`
  (`kv_config_integration.go:31-34`).

## 3. The fire path (every copy)

- Evaluator: `stateful_evaluator.go:368-456` `runActions`. `when` at `:379`; the cap at `:398-424`, counted before
  `Execute`; deny stops the list at `:427`; any other error is logged and counted, and the loop continues.
- Cron, `cron_scheduler.go`:
  - every-N on an in-memory tick at `:491`, then cooldown at `:517`;
  - action loop `:608-640`;
  - fire time stamped after dispatch at `:643-644`, restored from `RULE_SCHEDULES` at `:332`.
  - No reader of `When`, `effectiveID` or `ActionIterations` in the cron files (grep, not a probe run); the pin says
    so at `config_validation.go:422-424`.
- Message and entity paths:
  - The expression rule's cooldown gates the match, not the actions: `Evaluate` and `EvaluateEntityState` return
    false inside the window (`expression_factory.go:161`, `:219`). The evaluator then reads
    `CurrentlyMatching: false` (`message_handler.go:307-330`).
  - Its only stamp is in `ExecuteEvents` (`:363`), which the owner dropped.
  - Every-N runs after the match, on a per-rule in-memory counter (`message_handler.go:40-46`, `:183-196`).
  - So the message path runs cooldown before every-N, and cron runs every-N before cooldown.
- Product use: `fire_every_n_events` 0 rules; `cooldown` 2 rules (ruling item 9).

## 4. The stored state record

- `state_tracker.go:30-63`: `type MatchState struct {`, with its key `ruleID + "." + entityKey` (`:226-228`), pair
  key `:258-263`, bucket `"RULE_STATE"` (`processor.go:629`).
- Order: actions, then `stateTracker.Set` (`stateful_evaluator.go:247-249`). ADR-111 Decision 2 calls this "no
  claim".
- Cron: `schedule_tracker.go:57-76` `LastFireRecord` in `RULE_SCHEDULES`.

## Decision records

- **ADR-093** (pin `docs/adr/093-typed-user-response-subject-ownership.md`, not on main): the rule engine refuses
  `publish`, `publish_agent` and `approve` subjects in `user.response.>`, both as declared and after substitution.
  - Bears on § 2: once `publish_agent` and `approve` leave the core, the core no longer sees their subjects.
  - The split plan gives this ADR to PR 2.
- **ADR-036**: `validateConditionFields` refuses conditions on rule-opaque predicates at load
  (`config_validation.go:294-310`). Bears on § 1 (`Load`).
- **ADR-111** Decisions 2, 3, 5 bear on § 3 and § 4. ADR-031 (cron) bears on PR 11. ADR-041 (one condition
  evaluator) bears on `when`.
- This change ports no ADR.

## Adjacent claims

- Open PRs: #754 (this claim) and #753 (PR 1, `expression`). The page only names `expression.ConditionExpression`.
- Issues: #25 (wording replaced, comment 7162378515), #26, #28 (`deny.go` stays), #702, #8 Q15.
- Closest instance of "a product hands a component its own implementation at registration":
  `processor/graph-embedding/component.go:220-240` (`Register` + `RegisterWithEmbedder`).
