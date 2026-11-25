# Design: design-rule-core-target-shape

Base: `c729fecdb06596d83965c178210014a9430495a2`. Pin: SemStreams `8b99efe9`. Inventory: `inventory.md`. Bound by
the owner's rulings on PR #752 (comment 7162146299, items 1, 2, 3, 6; comment 7162536253) and on #9 (comment
7160955176).

This page names the four types the rule engine hangs on, which PR builds each, and the pin code each replaces. The
sketches show shape, not every field or final name. PRs 2 to 12 build toward it. What they still decide is listed at
the end.

## 1. The rule definition (PR 2; the expression arm PR 6, the cron arm PR 11)

```go
// Definition is one rule as written in a rules file or the rules KV bucket.
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

// DecodeDefinition is the one way bytes become a Definition: files, the KV bucket and hot reload all call it.
func DecodeDefinition(data []byte) (Definition, error)

// Rule is a loaded rule: its conditions compiled and every action checked by its handler.
type Rule struct {
    Def                                             Definition
    OnEnter, OnExit, WhileTrue, OnRecovery, Actions []LoadedAction
    // plus each kind's loaded parts: compiled conditions (PR 6), parsed schedule (PR 11)
}

// Load turns a Definition into a Rule, or refuses it naming rule, file and cause.
// switch def.Type { case "expression": … case "cron": … default: refuse }
func Load(def Definition, file string, x *ActionExecutor) (*Rule, error)
```

The fields are the pin's (`rule_factory.go:16-70`). Each PR's usage count decides whether a zero-use field stays
(item 9). There are two kinds, and `Load` tells them apart with a `switch`. No interface exists for the kinds.

Not carried:

- `Factory`, `Dependencies`, `Schema`, `Example`, the registry functions and `BaseRuleFactory`
  (`rule_factory.go:79-290`); no product reads them.
- The `init` registration (`expression_factory.go:443-449`) and the `Rule`, `Event` and `EntityStateEvaluator`
  interfaces (`interfaces.go:14-50`; the graph-event path is dropped by ruling).
- `docs/custom-rules.md`, and the map round trip (`rule_loader.go:39`, `config_validation.go:449`).

## 2. An action and its handler (PRs 2, 4, 5)

PR 2 builds the type and the map, PR 4 the `publish` and triple handlers, PR 5 the rest.

```go
// Action is one entry in an action list. The core reads four keys; Body is the whole object, every byte as written.
type Action struct {
    Type          string                           // the handler's name
    When          []expression.ConditionExpression // guard, checked by Fire
    ID            string                           // firing-cap key (optional)
    MaxIterations *int                             // firing cap: nil = 3, 0 = no cap
    Body          json.RawMessage
}

// ActionHandler is one action type. Check runs at load and returns what runs at fire.
type ActionHandler interface{ Check(a Action) (Runner, error) }
type Runner interface{ Run(ctx context.Context, ec *ExecutionContext) error }

// LoadedAction is what a Rule holds: the action, its cap key, and its handler's Runner.
type LoadedAction struct {
    Action
    Key string
    run Runner
}

// NewActionExecutor is the one constructor: the built-in handlers, built from deps, plus the product's handlers.
func NewActionExecutor(deps ExecutorDeps, product map[string]ActionHandler) (*ActionExecutor, error)
```

**One map holds every type.** `publish`, the triple actions, `reconcile_predicates`, `update_kv` and `lifecycle_*`
are handlers in the same map as a product's. Construction refuses a product handler that reuses a built-in name.

**Unknown types are refused at load.** `Load` looks up `a.Type` in the map. A miss refuses the rule, naming type, rule
and file. The same lookup returns the handler, so an action can never reach fire time with no handler.

**The core never decodes the body.** `tool_choice` and `response_format` reach a product's `publish_agent` handler as
the bytes the author wrote. That is the #26 proof (PR 2, against semteams' fixtures at `a331910e`). The page does not
promise how a later re-encode spaces those bytes; PR 2's test measures it.

**Dependencies come at construction, with no setters.** This replaces the four constructors (`actions.go:803-913`)
and the six setters (`:760-799`). A built-in handler whose dependency is absent refuses at load. Example: a triple
action when graph integration is off, which the pin logs and drops (`processor.go:673-674`, `actions.go:538`).

**Firing-cap key.** The key is `ID` when set. Otherwise it is a hash of rule ID, type and `Body`. This replaces the
per-type switch in `action_id.go:63-104`.

**How a product adds a handler.** This follows the `graph-embedding` precedent (`component.go:220-240`). The exact
names are for PR 2's Fable read.

```go
rule.Register(registry)                      // built-in handlers only
rule.RegisterWith(registry, rule.Extensions{ // called from the product's main, never an init
    Handlers: map[string]rule.ActionHandler{"publish_agent": agentaction.New(client, catalog)},
})
```

**#25, reworded.** Old: "core families stay compiled in". New: "every action type, built-in or a product's, is one
handler in the map `NewActionExecutor` builds; an unregistered type is refused at load, naming type and file." The
two proving tests stay:

- a fake handler a product passes to `RegisterWith` is checked at load and run at fire, and an unregistered type is
  refused at load;
- semboids' core-only pack (`configs/rules/zone-steering/*.json`) loads and fires with no product handler.

## 3. The one fire function (PR 7; called by PRs 9, 10, 11)

```go
// Fire runs one action list of one rule under its gates and reports what happened.
// It reads and writes no store: the caller loads st before and saves it after (actions first, state after).
func (x *ActionExecutor) Fire(ctx context.Context, r *Rule, list []LoadedAction, st *FireState,
    ec *ExecutionContext, now time.Time) FireResult

type FireResult struct {
    Gated   string          // "every_n" or "cooldown" when no action ran
    Actions []ActionOutcome // in list order, up to where the list stopped
}

type ActionOutcome struct {
    Key, Type string
    Skipped   string // "when" or "cap"
    Err       error  // the handler's error; a *DenyVerdict stops the list (#28)
}
```

**Order of the gates.** On each call `Fire`:

1. counts the call;
2. applies `fire_every_n_events`;
3. applies cooldown;
4. stamps the fire time;
5. for each action, checks `when`, then the cap (a cap skip moves a metric), then counts and runs it.

Steps 1 to 3 keep the pin's cron order (`cron_scheduler.go:491-525`), so a call stopped by cooldown still counts
toward N. Step 5 keeps the evaluator's order (`stateful_evaluator.go:378-424`).

**Callers.** The message, entity and cron paths all call `Fire`, and nothing else runs an action list. This replaces
`runActions` (`stateful_evaluator.go:368-456`), cron's copy (`cron_scheduler.go:491-525`, `:608-644`), the message
path's every-N counter (`message_handler.go:40-46`, `:183-196`) and the in-memory cooldown
(`expression_factory.go:31`, `:161`, `:219`, `:363`).

| Gate | Expression rules (message, entity) | Cron rules |
| --- | --- | --- |
| every-N | yes (ruling on #9) | yes, as at the pin |
| cooldown | yes, per rule and entity (#702) | yes, per rule |
| `when` | yes | yes (the pin ignores it; no product cron action sets it) |
| firing cap | yes | none: `max_iterations` on a cron action is refused at load (the pin ignores it) |

`Fire` takes `now` and touches no store. That lets PR 7's generated check (item 10) drive it against a reference
model with no clock and no NATS.

## 4. The stored state record (PR 3 carries `MatchState`; PR 7 adds `FireState`)

```go
// MatchState is one rule's stored state for one entity or entity pair, in RULE_STATE under "<rule id>.<entity key>".
type MatchState struct {
    RuleID, EntityKey         string
    IsMatching                bool
    LastTransition            string
    TransitionAt, LastChecked time.Time
    SourceRevision            uint64
    Iteration                 int
    FieldValues               map[string]string
    FireState                 // embedded: the gate state, stored with the match state
}

// FireState is what Fire's gates read and write.
type FireState struct {
    FiredAt          time.Time      // cooldown: per rule and entity, survives a restart (#702)
    MatchCount       int64          // fire_every_n_events
    ActionIterations map[string]int // firing cap, by LoadedAction.Key; carried forward, never reset (#702 asks)
}
```

The fields are the pin's (`state_tracker.go:30-63`). `ActionIterations` moves into `FireState`, and `FiredAt` and
`MatchCount` are new. PR 3 decides whether the stored `MaxIterations` copy stays. The evaluator loads the record,
`Fire` changes it, and the evaluator saves it after the actions run (`stateful_evaluator.go:247-249`).

**Every-N is left open.** PR 7 decides whether every-N counts per rule or per rule and entity, and the shape holds
either answer. Per rule and entity: `MatchCount` lives in this record as shown. Per rule: the message and entity
callers fill `st.MatchCount` from a per-rule record and save it back, and `Fire` does not change. Cron passes a
per-rule `FireState`. PR 11 decides where cron stores it; the pin restores last fire from `RULE_SCHEDULES`
(`cron_scheduler.go:332`) and keeps the tick count in memory (`:491`).

## Options considered

- **A, recommended (ruled by item 1).** Four keys plus a raw body, and one handler map.
- **B, overruled by item 1.** The split plan's 35-field typed `Action` with a closed switch, plus a registry beside
  it. That is two ways to run an action, and every new type adds a field to a shared struct.
- **C.** Keep the 35-field struct and hand it to every handler. The agentic fields stay in core, so #8 Q15 and #26
  fail.

## Who carries it

- **semteams** passes its `publish_agent` handler (and any type of its own, `replace_owned` included) to
  `RegisterWith`. If it does nothing, a rule using that type is refused at boot, naming type, rule and file.
- **semteams' contract tests** that call `NewActionExecutorFull` and `Execute` move to `NewActionExecutor` and `Load`
  plus `Fire`.
- **semboids** needs no handler: it uses only `publish` and `lifecycle_transition`, with no triple action while graph
  integration is off.
- What a product must know: the handler's name, and that `Run` gets an `*ExecutionContext`.

## Left to the PR designs (the page fixes none of these)

- Item 4: the lane and the fence (PRs 7, 8, 10). They sit around load and save, not inside `Fire`.
- Item 7: what a failed action means under ADR-111 Decision 3 (PR 7). `FireResult` reports every outcome, so any
  answer fits.
- Item 8: the deadline-timer probe (PR 11, PR 14).
- Item 9: which zero-use fields stay (each PR).
- Item 10: the generated check (PR 7).
- Every-N keying, and whether an exit counts toward N (PR 7).
- The cap fact a rule can read (PR 7).
- `ExecutionContext`'s exported surface for product handlers (PRs 2, 3).
- The lifecycle manager at registration and the one `Processor` constructor (PR 8; replaces `processor.go:250-255`).
- Whether a refused rule stops boot or is skipped. The pin skips it (`rule_loader.go:228-233`); PR 2 proposes, PR 8
  decides.
- **Invariants** (PRs 2 and 7 give each a spec home):
  - a loaded rule has a handler for every action;
  - `Fire` runs no action when a gate stops the call;
  - no action runs more than its cap.
