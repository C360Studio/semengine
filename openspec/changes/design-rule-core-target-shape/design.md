# Design: design-rule-core-target-shape

Base: `c729fecdb06596d83965c178210014a9430495a2`. Pin: SemStreams `8b99efe9`. Inventory: `inventory.md`. Bound by
the owner's rulings on PR #752 (comment 7162146299, items 1, 2, 3, 6; comment 7162536253) and #9 (comment 7160955176).
The four types the rule engine hangs on, the PR that builds each, and the pin code each replaces. Sketches show
shape, not every field or final name; item 1 already chose this shape over a typed `Action` with a registry beside
the switch. **Framework API** marks what a product calls; every other name may be unexported.

## 1. The rule definition (PR 2; expression arm PR 6, cron arm PR 11)

```go
type Definition struct { /* the pin's fields, rule_factory.go:16-70 */ } // Framework API
func DecodeDefinition(data []byte) (Definition, error) // the one decode: files, KV bucket, hot reload
type Rule struct{ Def Definition; OnEnter, OnExit, WhileTrue, OnRecovery, Actions []LoadedAction } // checked
func Load(def Definition, file string, handlers map[string]ActionHandler) (*Rule, error) // no I/O
```

`Load` returns a Rule with conditions compiled and every action checked, or refuses it naming rule, file and cause.
Each PR's usage count decides whether a zero-use field stays (item 9). The two kinds are a `switch def.Type` in
`Load`. `Load` keeps ADR-036's condition check (`config_validation.go:294-310`) and the factory's operator and
condition checks (`rule_factory.go:240-290`). Not carried: `Factory`, `Dependencies`, `Schema`, the registry functions
and `BaseRuleFactory` (`rule_factory.go:79-285`), the `init` registration (`expression_factory.go:443-449`), the
`Rule`, `Event` and `EntityStateEvaluator` interfaces (`interfaces.go:14-50`), `docs/custom-rules.md`, and the map
round trip (`rule_loader.go:39`, `config_validation.go:449`).

## 2. An action and its handler (PRs 2, 4, 5)

```go
type Action struct { // Framework API. The core reads four keys; Body is the whole object as written.
    Type          string
    When          []expression.ConditionExpression
    ID            string // firing-cap key (optional)
    MaxIterations *int   // firing cap: nil means 3, 0 means no cap
    Body          json.RawMessage
}
type ActionHandler interface { // Framework API
    Check(a Action) error                                          // at load: no dependencies, no I/O
    Run(ctx context.Context, a Action, ec *ExecutionContext) error // at fire
}
type LoadedAction struct { Action; Key string } // Key: the firing-cap key
// The executor's one constructor: built-in handlers from deps, plus the product's. No setters.
func NewActionExecutor(deps ExecutorDeps, product map[string]ActionHandler) (*ActionExecutor, error)
```

PR 2 builds the type and the map; PR 4 `publish` and the triple actions; PR 5 `reconcile_predicates`, `update_kv` and
`lifecycle_*`. Built-in types are handlers in the same map as a product's; a product handler reusing a built-in name is
refused. `Load` looks up each `Type` in the map and refuses a miss, naming type, rule and file; `Fire` dispatches
through the same map, so no action reaches fire time without a handler. `Check` needs nothing, so preflight,
`Config.Validate` and `SaveRule` can run it before the executor exists (`rule_loader.go:117`, `config.go:169`,
`kv_config_integration.go:316`). The executor is built once in `Start`, after main injects the reconciler; the Processor
keeps `SetPredicateReconciler`, which main's `service.ProjectionBinder` requires (`service/rule_pack_bind.go:16-20`).
This replaces four constructors and six setters (`actions.go:760-913`) and the closed switch (`:916-945`).

The core never decodes a body, so `tool_choice` and `response_format` reach a product's `publish_agent` handler as
written: the #26 proof (PR 2, which also measures what a re-encode does to spacing). The cap key is `ID`, else a
hash of rule ID, type and `Body` (replaces `action_id.go:63-104`).

A product adds its types from its own `main`, never an `init`. `rule.Register(registry)` registers the built-ins
only; `rule.RegisterWith(registry, rule.Extensions{Handlers: map[string]rule.ActionHandler{"publish_agent": h}})`
(Framework API) adds the product's. The precedent is `processor/graph-embedding/component.go:220-240`; PR 2's Fable
read settles the names.

**#25, reworded:** "every action type, built-in or a product's, is one handler in one map; an unregistered type is
refused at load, naming type and file." Its two tests stay: a fake handler passed to `RegisterWith` is checked at
load and run at fire, and an unregistered type is refused at load; semboids' core-only pack
(`configs/rules/zone-steering/*.json`) loads and fires with no product handler.

## 3. The one fire function (PR 7; called by PRs 9, 10, 11)

```go
// Fire runs one action list under its gates, reading and writing only st; the caller loads and saves state.
func (x *ActionExecutor) Fire(ctx context.Context, r *Rule, list []LoadedAction, st *FireState,
    ec *ExecutionContext, now time.Time) FireResult
```

`Fire` applies `fire_every_n_events`, cooldown, each action's `when` and the firing cap, then runs the action
through its handler. A `*DenyVerdict` stops the list (#28). `FireResult` says whether a gate stopped the call and
gives each action's outcome in order: run, skipped by `when` or cap, or the handler's error. The message, entity and
cron paths all call it, and nothing else runs an action list. It replaces `runActions`
(`stateful_evaluator.go:368-456`), cron's copy (`cron_scheduler.go:491-525`, `:608-644`), the message path's every-N
counter (`message_handler.go:40-46`, `:183-196`) and the in-memory cooldown (`expression_factory.go:31`, `:161`,
`:219`, `:363`). On cron, `when` applies and no cap does, as at the pin; an explicit `max_iterations` on a cron action
is refused at load so it is not silently ignored. `Fire` takes `now` and touches no rule state store (handlers still
do NATS I/O), so PR 7's generated check (item 10) can drive the gates against a reference model.

## 4. The stored state record (PR 3 carries `MatchState`, PR 7 adds `FireState`)

```go
// One rule's state for one entity or pair, in RULE_STATE under "<rule id>.<entity key>".
type MatchState struct { /* the pin's fields, state_tracker.go:30-63, less ActionIterations */ ; FireState }
type FireState struct {
    FiredAt          time.Time      // cooldown
    MatchCount       int64          // fire_every_n_events
    ActionIterations map[string]int // firing cap, by LoadedAction.Key
}
```

Gate state is stored with match state. `FiredAt` per rule and entity is #702's change: the window survives a restart.
The evaluator saves the record after the actions run (`stateful_evaluator.go:247-249`; ADR-111 Decision 2). PR 7 picks
per-rule or per-rule-and-entity counting for every-N, with a test that N matches give the ruled count. Cron's
`FireState` is per rule; PR 11 picks its store.

## Who carries it

semteams passes `publish_agent` and its own types to `RegisterWith` (doing nothing refuses those rules at load,
naming type, rule and file); its contract tests move to `NewActionExecutor` and `Fire`; its `ConfigManager` and
`HotReloadTarget` seam changes in PR 12. semboids needs no handler.

## Left to the PR designs

- PR 7: gate order, stamp time, and whether cooldown gates the match or the list, stated against the pin's two
  orders (`inventory.md` § 3); every-N keying and whether an exit counts; item 7 (failed actions, ADR-111 Decision 3).
- PR 5: pack-wide projection-contract derivation, where only `reconcile_predicates`' own code decodes its body.
- PRs 2, 3, 4: what a product handler gets from the framework (`ExecutionContext`'s substitution methods, the triple
  writer `triple_mutator.go:27`, a publisher).
- PRs 2, 8: whether a refused rule stops boot (the pin skips it, `rule_loader.go:228-233`).
- PRs 8, 12: the handler set `SaveRule` and `Config.Validate` check against; PR 8: the lifecycle manager at
  registration and one `Processor` constructor (`processor.go:250-255`). Items 4, 8, 9: PRs 7/8/10, 11/14, each.
- Spec homes (PRs 2, 7) for three invariants: a loaded rule has a handler for every action; no action runs when a
  gate stops the call; no action runs more than its cap.
