# Rule Syntax

A rule is one JSON object. This page covers its keys, what a rule's path (entities or messages) lets its conditions
read, the template values a condition can compare with, and what `Load` refuses. Operators and field resolution are
in [Conditions](conditions.md).

Nothing in this repository runs a rule yet: the keys below say what the rule processor does with them, and `Load`
checks each now.

## Definition structure

```go
type Definition struct {
    ID               string                           // one KV literal token: letters, digits, '-', '/', '_', '='
    Type             string                           // "expression" or "cron"
    Name             string                           // for people; the engine never reads it
    Description      string                           // for people; the engine never reads it
    Enabled          bool                             // absent means true
    Conditions       []expression.ConditionExpression // what must hold (expression rules)
    Logic            string                           // "and" or "or"; absent means "and"
    Cooldown         string                           // a Go duration, such as "5m"; checked at load only
    Entity           EntityConfig                     // the entities the rule watches
    Metadata         map[string]interface{}           // the author's notes; the engine never reads it
    OnEnter          []Action                         // when the conditions start to hold
    OnExit           []Action                         // when they stop holding
    WhileTrue        []Action                         // on each update while they hold
    OnRecovery       []Action                         // on restart, if they still hold
    RerunOnRecovery  bool                             // re-run on_enter on restart when there is no on_recovery
    MaxIterations    int                              // read by conditions as $state.max_iterations; not a limit
    FireEveryNEvents int                              // fire on every Nth match; 0 and 1 mean every match
    Schedule         string                           // cron rules: a 5-field expression or @hourly and the like
    Actions          []Action                         // cron rules: the actions each tick fires
}

type EntityConfig struct {
    Pattern string // a six-position entity ID pattern
}
```

## Complete example

```json
{
  "id": "battery-low-alert",
  "type": "expression",
  "name": "Battery Low Alert",
  "description": "Alert when a drone's battery drops below 20%",

  "conditions": [
    {"field": "drone.telemetry.battery", "operator": "lt", "value": 20, "required": true},
    {"field": "drone.telemetry.status", "operator": "eq", "value": "flying"}
  ],
  "logic": "and",

  "entity": {"pattern": "*.*.robotics.*.drone.*"},

  "metadata": {"severity": "warning", "team": "operations"},

  "on_enter": [
    {"type": "add_triple", "predicate": "alert.battery.status", "object": "low"},
    {"type": "publish", "subject": "alerts.battery.low"}
  ],
  "on_exit": [
    {"type": "remove_triple", "predicate": "alert.battery.status"}
  ]
}
```

Every predicate a condition field or a triple action names must be declared in the vocabulary
(`vocabulary.Register`) before the rule loads.

## Keys

The decode is strict: a key the rule, its `entity` object or a condition does not declare is refused, and so is a
declared key spelt with another case (`"ID"` for `"id"`) or a key repeated in any object.

| Key | Type | Absent means | Meaning |
| --- | --- | --- | --- |
| `id` | string | refused | The rule's identity, one KV literal token (no `.`) |
| `type` | string | refused | `expression`, or `cron` |
| `name`, `description` | string | `""` | For people; nothing reads them |
| `enabled` | bool | `true` | Whether the rule runs; a disabled rule is checked at load all the same |
| `conditions` | array | refused (expression) | At least one condition for an expression rule |
| `logic` | string | `"and"` | `and`: every condition holds; `or`: at least one does |
| `cooldown` | string | `""` | A Go duration; see [Cooldown](#cooldown) |
| `entity` | object | no pattern | `pattern`: the entities the rule watches |
| `metadata` | object | none | The author's notes; nothing reads them |
| `on_enter`, `on_exit`, `while_true`, `on_recovery` | array | `[]` | Actions, by when they fire |
| `rerun_on_recovery` | bool | `false` | Re-run `on_enter` on restart for a rule with no `on_recovery` |
| `max_iterations` | int | `0` | The value a condition reads as `$state.max_iterations` |
| `fire_every_n_events` | int | `0` | Fire the actions on every Nth match only; never negative |
| `schedule`, `actions` | string, array | | Cron rules only |

An enabled cron rule is refused at load, as nothing in this repository yet fires a rule on a schedule. A disabled one
loads.

## Entities or messages: the rule's path

`entity.pattern` decides what a rule runs on:

- **With a pattern**, the rule is an entity rule, matched against entities whose ID the pattern matches. Its
  conditions read the entity's triples and `$entity.` values.
- **With no pattern**, the rule is a message rule, matched against messages. Its conditions read the message's payload
  through `$message.`.

```json
{"entity": {"pattern": "acme.*.robotics.*.drone.*"}}
```

A pattern has exactly six positions, each a literal segment or `*`, which matches one segment. `>` is refused: it
would hide a pattern with the wrong number of positions.

```text
acme.*.robotics.*.drone.*     # drones from any platform and system
*.*.*.*.sensor.*              # all sensors
acme.logistics.*.*.*.*        # everything in logistics
```

A condition that reads what its path never has is refused at load, at `conditions[<i>]`, enabled or not: in a rule
with no pattern, a field starting with `$entity.` or a text value containing `$entity.` anywhere (`zone-$entity.id`
too); in a rule with a pattern, the same for `$message.`. Such a field or value could never be filled, so the
condition would quietly read nothing.

A message payload is readable when its type implements `message.RuleReadable`, which declares the fields a rule may
read; a `message.GenericJSONPayload` declares its whole data map. A rule cannot read a payload that declares none:
its message conditions never hold for it, and it is reported once, at Warn, per loaded rule and payload type.

## Conditions

```json
{"field": "drone.telemetry.battery", "operator": "lt", "value": 20, "required": true}
```

| Key | Type | Meaning |
| --- | --- | --- |
| `field` | string | What to read: a declared predicate, or a `$message.`, `$state.`, `$prev.` or `$entity.lifecycle.` field |
| `operator` | string | A registered operator ([Conditions](conditions.md)) |
| `value` | any | What to compare with; text may hold template values (below) |
| `from` | string or array | The allowed previous values, for `transition` only |
| `required` | bool | Match only when the field is present |

A field without `$` must be a declared predicate, and not a rule-opaque one (a predicate declared as holding
private content, which no rule may match on). An action's `when` guard is a list of
conditions with the same check.

### A missing required field

`required: true` means "match only when the field is present". When a rule is matched and a required field is
absent, the rule does not match, with no error, on the entity path and the message path alike. Under `or`, an absent
required field stops the whole rule from matching, even when another condition holds. The evaluator on its own
reports the absence as an error wrapping `expression.ErrRequiredFieldMissing`
([Conditions](conditions.md#missing-fields-and-required)); the match reads that error and answers no match.

Any other evaluation error, such as `length_eq` with the value `"abc"`, is an error of the match, not a quiet no
match.

## Template values in a condition

A text `value` may hold template values, filled from the entity or the message before the condition is compared. A
template value that does not resolve is an error of the match naming the rule, the field and the token; the condition
is never compared with the unfilled text.

| Template | Filled with |
| --- | --- |
| `$entity.id` | The entity's ID |
| `$entity.org`, `.platform`, `.system`, `.domain`, `.type`, `.instance` | That part of the entity ID |
| `$entity.triple.<predicate>` | The first value of the predicate on the entity; unresolved when it has none |
| `$entity.triple.<predicate>.value` | The same, or `""` when the entity has none (the predicate must have three parts) |
| `$entity.triple.<predicate>.length` | How many values the predicate has, `0` when none; a list counts each item |
| `$entity.triple.<predicate>.triples` | Every value, as JSON array text; `[]` when none |
| `$entity.lifecycle.phase`, `.terminal`, `.workflow`, `.workflow_def` | The entity's lifecycle state, when a lifecycle manager knows it |
| `$message.<path>` | A field the message's payload declares |
| `$state.iteration`, `$state.max_iterations` | The rule's match state for the entity, where the caller has it |
| `$now` | The current time, UTC, RFC 3339 |

`.value` lets a condition compare two of the entity's triples and stay quiet on entities that lack the second:

```json
{"field": "openspec.change.validated", "operator": "eq", "value": "$entity.triple.openspec.change.revision.value"}
```

On an entity without `openspec.change.revision` the value fills with `""` and the condition is false; nothing is
logged.

`Matches`, which answers whether a loaded rule matches an entity with no rule processor running, has no match state
and no lifecycle lookup, so a value holding `$state.` or `$entity.lifecycle.` does not resolve there and is an error.

A `regex` value holds no template value, as `Load` refuses a pattern holding one, so a pattern is matched as written.
A `$` that starts no template name, such as the end-of-text anchor in `^done$`, stays as written.

## Actions

An action is a JSON object with a `type`. Every action may carry `when` (a list of conditions that must also hold),
`id` and `max_iterations` (its firing cap: absent means 3, `0` means no cap). The other keys belong to its type.

```json
{
  "on_enter": [
    {"type": "add_triple", "predicate": "alert.battery.status", "object": "active"},
    {"type": "publish", "subject": "alerts.new"}
  ]
}
```

| Type | Keys it reads |
| --- | --- |
| `publish` | `subject`, `properties` |
| `add_triple` | `subject`, `predicate`, `object`, `ttl` |
| `remove_triple` | `subject`, `predicate`, `object` |
| `update_triple` | `subject`, `predicate`, `object`, `ttl`, `properties` |
| `reconcile_predicates` | `subject`, `predicate`, `object`, `ttl`, `projection_contract`, `projection_group` |
| `update_kv` | `bucket`, `key`, `payload`, `merge` |
| `lifecycle_transition` | `workflow`, `phase`, `reason`, `set` |
| `lifecycle_complete` | `workflow` |
| `lifecycle_fail` | `workflow`, `reason` |

A built-in action refuses a key its type does not read. A product adds its own types through `NewHandlers`; its
handler checks its actions' keys. An `add_triple`, `remove_triple` or `update_triple` action's `predicate` must be a
declared literal, never a template. A
`publish` subject may not be in `user.response.>`.

## Cooldown

```json
{"cooldown": "5m"}
```

`cooldown` must be a Go duration (`30s`, `5m`, `1h`); `Load` refuses anything else. Nothing in this repository applies
it yet: a rule does not wait between firings, and asking whether a rule matches (`Matches`) ignores it.

## What Load refuses

`Load` checks every rule, disabled ones included, and refuses, classified invalid and naming the rule ID, the source,
the position (such as `on_enter[1].when[0]`) and the cause:

- an `id` that is not one KV literal token, and a `type` other than `expression` or `cron`;
- an expression rule with no conditions, a condition with no field or no operator, an unknown operator, a `logic`
  other than `and` or `or`, and a `cooldown` that is not a duration;
- a condition field that is not a declared predicate and does not start with `$`, or that is rule-opaque;
- a condition that reads what the rule's path never has (`$entity.` without a pattern, `$message.` with one);
- a bad or templated `regex` pattern;
- an `entity.pattern` that is not six positions of literal segments or `*`;
- a negative `fire_every_n_events` or action `max_iterations`, and `rerun_on_recovery` with no `on_enter` or
  `on_recovery`;
- an action of a type not in the handler set, or one its handler refuses;
- an enabled cron rule.
