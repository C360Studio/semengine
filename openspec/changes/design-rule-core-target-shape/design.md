# Design: design-rule-core-target-shape

Base: `c729fecdb06596d83965c178210014a9430495a2`. Pin: SemStreams `8b99efe9`. Inventory: `inventory.md`. Bound by
the owner's rulings on PR #752 (comment 7162146299, items 1, 2, 3, 6; comment 7162536253) and on #9 (comment
7160955176).

This page names the four types the rule engine hangs on, which PR builds each, and the pin code each replaces. The
sketches show shape, not every field or final name. Item 1 already chose between this shape and the split plan's
typed `Action` with a registry beside the switch. **Framework API** marks a name a product calls; every other name
may be unexported.

## 1. The rule definition (PR 2; the expression arm PR 6, the cron arm PR 11)

```go
// Definition is one rule as written in a rules file or the rules KV bucket. Framework API.
type Definition struct {
    ID, Type, Name, Description, Logic, Cooldown, Schedule string
    Enabled                                         bool
    Conditions                                      []expression.ConditionExpression
    Entity                                          EntityConfig
    Metadata                                        map[string]any
    OnEnter, OnExit, WhileTrue, OnRecovery, Actions []Action
    RerunOnRecovery                                 bool
    MaxIterations, FireEveryNEvents                 int
    RelatedPatterns                                 []string
}

// DecodeDefinition is the one way bytes become a Definition, for files, the KV bucket and hot reload alike.
func DecodeDefinition(data []byte) (Definition, error)

// Rule is a loaded rule: conditions compiled, every action checked by its handler.
type Rule struct {
    Def                                             Definition
    OnEnter, OnExit, WhileTrue, OnRecovery, Actions []LoadedAction
    // plus each kind's loaded parts: compiled conditions (PR 6), the parsed schedule (PR 11)
}

// Load checks a Definition and returns a Rule, or refuses it naming rule, file and cause. It does no I/O.
// switch def.Type { case "expression": … case "cron": … default: refuse }
func Load(def Definition, file string, handlers map[string]ActionHandler) (*Rule, error)
```

The fields are the pin's (`rule_factory.go:16-70`). Each PR's usage count decides whether a zero-use field stays
(item 9). There are two kinds, told apart by a `switch`. `Load` keeps the pin's checks: ADR-036's condition check
(`config_validation.go:294-310`) and the operator and condition checks the factory held (`rule_factory.go:240-290`).

Not carried:

- `Factory`, `Dependencies`, `Schema`, `Example`, the registry functions and `BaseRuleFactory`
  (`rule_factory.go:79-285`).
- The `init` registration (`expression_factory.go:443-449`).
- The `Rule`, `Event` and `EntityStateEvaluator` interfaces (`interfaces.go:14-50`).
- `docs/custom-rules.md`, and the map round trip (`rule_loader.go:39`, `config_validation.go:449`).

## 2. An action and its handler (PRs 2, 4, 5)

PR 2 builds the type and the map. PR 4 builds `publish` and the triple actions. PR 5 builds `reconcile_predicates`,
`update_kv` and `lifecycle_*`.

```go
// Action is one entry in an action list. The core reads four keys. Body is the whole object as written.
// Framework API.
type Action struct {
    Type          string                           // the handler's name
    When          []expression.ConditionExpression // guard
    ID            string                           // firing-cap key (optional)
    MaxIterations *int                             // firing cap: nil means 3, 0 means no cap
    Body          json.RawMessage
}

// ActionHandler is one action type. Framework API.
type ActionHandler interface {
    Check(a Action) error                                          // at load: no dependencies, no I/O
    Run(ctx context.Context, a Action, ec *ExecutionContext) error // at fire
}

// LoadedAction is what a Rule holds.
type LoadedAction struct {
    Action
    Key string // the cap key
}

// NewActionExecutor is the executor's one constructor: built-in handlers from deps, plus the product's. No setters.
func NewActionExecutor(deps ExecutorDeps, product map[string]ActionHandler) (*ActionExecutor, error)
```

**One map holds every type.** Built-in types are handlers in the same map as a product's. A product handler that
reuses a built-in name is refused.

**Unknown types are refused at load.** `Load` looks up each `Type` in the map. A miss refuses the rule, naming type,
rule and file. `Fire` dispatches through the same map, so no action reaches fire time without a handler.

**`Check` runs without dependencies, and `Run` runs with them.** Because `Check` needs nothing, preflight,
`Config.Validate` and `SaveRule` can run it before the executor exists (`rule_loader.go:117`, `config.go:169`,
`kv_config_integration.go:316`). The executor is built once, in `Start`, after main injects the reconciler. The
Processor keeps `SetPredicateReconciler`, because main's `service.ProjectionBinder` requires it
(`service/rule_pack_bind.go:16-20`). This replaces the executor's four constructors and six setters
(`actions.go:760-913`) and the switch (`:916-945`).

**The core never decodes a body.** `tool_choice` and `response_format` reach a product's `publish_agent` handler as
written, which is the #26 proof (PR 2). The page does not promise how a re-encode spaces the bytes; PR 2's test
measures it.

**Firing-cap key.** The key is `ID` when set, otherwise a hash of rule ID, type and `Body`. This replaces
`action_id.go:63-104`.

**How a product adds a type.** Exact names go to PR 2's Fable read. The precedent is
`processor/graph-embedding/component.go:220-240`.

```go
rule.Register(registry) // built-in types only
rule.RegisterWith(registry, rule.Extensions{ // Framework API; called from the product's main, never an init
    Handlers: map[string]rule.ActionHandler{"publish_agent": agentaction.New(client)},
})
```

**#25, reworded.** Old: "core families stay compiled in". New: "every action type, built-in or a product's, is one
handler in one map; an unregistered type is refused at load, naming type and file." The two proving tests stay:

- a fake handler a product passes to `RegisterWith` is checked at load and run at fire, and an unregistered type is
  refused at load;
- semboids' core-only pack (`configs/rules/zone-steering/*.json`) loads and fires with no product handler.

## 3. The one fire function (PR 7; called by PRs 9, 10, 11)

```go
// Fire runs one action list of one rule under its gates and reports each action's outcome. It reads and writes
// only the FireState it is handed. The caller loads the rule state before and saves it after.
func (x *ActionExecutor) Fire(ctx context.Context, r *Rule, list []LoadedAction, st *FireState,
    ec *ExecutionContext, now time.Time) FireResult

type FireResult struct {
    Gated   string          // "every_n" or "cooldown" when no action ran
    Actions []ActionOutcome // in list order, up to where the list stopped
}

type ActionOutcome struct {
    Key, Type string
    Skipped   string // "when" or "cap"
    Err       error
}
```

**What `Fire` does.** It applies `fire_every_n_events`, cooldown, each action's `when`, and the firing cap. Then it
runs the action through its handler. A `*DenyVerdict` error stops the list, as #28 keeps it.

**Callers.** The message, entity and cron paths all call `Fire`, and nothing else runs an action list. This replaces
`runActions` (`stateful_evaluator.go:368-456`), cron's copy (`cron_scheduler.go:491-525`, `:608-644`), the message
path's every-N counter (`message_handler.go:40-46`, `:183-196`) and the in-memory cooldown
(`expression_factory.go:31`, `:161`, `:219`, `:363`).

**Cron.** On cron rules `when` applies, and `Fire` applies no firing cap, as at the pin. An explicit
`max_iterations` on a cron action is refused at load, so it is not silently ignored.

**Testing.** `Fire` takes `now` and touches no rule state store; its handlers still do their own NATS I/O. So PR 7's
generated check (item 10) can drive the gates against a reference model.

## 4. The stored state record (PR 3 carries `MatchState`; PR 7 adds `FireState`)

```go
// MatchState is one rule's state for one entity or entity pair, in RULE_STATE under "<rule id>.<entity key>".
type MatchState struct {
    RuleID, EntityKey         string
    IsMatching                bool
    LastTransition            string
    TransitionAt, LastChecked time.Time
    SourceRevision            uint64
    Iteration                 int
    FieldValues               map[string]string
    FireState                 // embedded: gate state is stored with match state
}

// FireState is what Fire's gates read and write.
type FireState struct {
    FiredAt          time.Time      // cooldown
    MatchCount       int64          // fire_every_n_events
    ActionIterations map[string]int // firing cap, by LoadedAction.Key
}
```

The fields are the pin's (`state_tracker.go:30-63`). `ActionIterations` moves into `FireState`; `FiredAt` and
`MatchCount` are new. Storing `FiredAt` per rule and entity is #702's change: the window survives a restart. The
evaluator loads the record and saves it after the actions run (`stateful_evaluator.go:247-249`; ADR-111 Decision 2).
PR 7 decides whether every-N counts per rule or per rule and entity, with a test that N matches give the ruled count.
Cron's `FireState` is per rule, and PR 11 decides where it is stored.

## Who carries it

- **semteams** passes `publish_agent`, and any type of its own, to `RegisterWith`. If it does nothing, a rule using
  that type is refused when it loads, naming type, rule and file. Whether that stops boot is open (below).
- **semteams' contract tests** that call `NewActionExecutorFull` and `Execute` move to `NewActionExecutor` and
  `Fire`.
- **semteams' `ConfigManager` and `HotReloadTarget` seam** (`rule_config_service.go:34-35`, `:92`) changes with the
  map round trip, in PR 12.
- **semboids** needs no handler: it uses only `publish` and `lifecycle_transition`.

## Left to the PR designs (one constraint each)

- **Gate order, stamp time, and what cooldown gates (PR 7).** The design states its order against the pin's two:
  - message: cooldown inside the match, then every-N;
  - cron: every-N, then cooldown, with the stamp after dispatch.

  Rule count today: 0 rules set every-N and 2 set cooldown.
- **Which state each gate keeps, and whether an exit counts toward N (PR 7).**
- **Item 7: what a failed action means under ADR-111 Decision 3 (PR 7).** `FireResult` reports every outcome.
- **Pack-wide projection-contract derivation (PR 5).** Only `reconcile_predicates`' own code decodes its body for
  this.
- **Which handler set `SaveRule` and `Config.Validate` check against (PRs 8, 12).** It includes the product's types.
- **What a product handler gets from the framework (PRs 2, 3, 4).** `ExecutionContext`'s substitution methods, the
  rule's triple writer (`triple_mutator.go:27`, which records the rule's own revisions), and a publisher.
- **Whether a refused rule stops boot or is skipped (PRs 2, 8).** The pin skips it with a log
  (`rule_loader.go:228-233`).
- **Item 4 (PRs 7, 8, 10), item 8 (PRs 11, 14), item 9 (each PR).**
- **The lifecycle manager at registration, and the one `Processor` constructor (PR 8; `processor.go:250-255`).**
- **Invariants (PRs 2, 7 give each a spec home):**
  - a loaded rule has a handler for every action;
  - no action runs when a gate stops the call;
  - no action runs more than its cap.
