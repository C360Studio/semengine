# Conditions

A condition compares one field with a value. A rule's `conditions` and an action's `when` guard are both lists of
conditions, checked by one evaluator (`processor/rule/expression`, ADR-041). This page covers the fields a condition
can read, every operator, how lists combine, and what makes evaluation fail.

## Condition structure

```go
type ConditionExpression struct {
    Field    string      `json:"field"`          // what to read, e.g. "drone.telemetry.battery"
    Operator string      `json:"operator"`       // e.g. "lt", "eq", "regex"
    Value    interface{} `json:"value"`          // what to compare with: 20, "active", true, [1, 2]
    Required bool        `json:"required"`       // a missing field is an error rather than false
    From     interface{} `json:"from,omitempty"` // transition only: allowed previous value or values
}
```

A list of conditions is a `LogicalExpression`:

```go
type LogicalExpression struct {
    Conditions []ConditionExpression `json:"conditions"`
    Logic      string                `json:"logic"` // "and" (also when empty) or "or"
}
```

## Where a field is read from

The evaluator reads each condition's field from one place, chosen by its name, in this order:

1. `$state.<name>`, `$prev.<field>` and `$entity.lifecycle.<name>` come from the state fields the caller passes:
   the rule's match state, previous values, and lifecycle values the caller resolved. They are never read from a
   triple or the message.
2. `$message.<path>` comes from the inbound message, walking nested objects along the dotted path
   (`$message.tool_args.command`).
3. Any other name is a predicate. With an entity, the evaluator reads the first triple carrying that predicate; if
   the entity has none, it reads the message along the same dotted path. With no entity, it reads the message only.

When both the entity and the message hold a bare name, the entity wins. Write `$message.<path>` when the source
matters.

Two operators read fields differently: the list operators (below) read every triple of a predicate, and `transition`
reads the entity only.

## Missing fields and `required`

`required` defaults to false.

- **Optional (`required: false`)**: a missing field makes the condition false; evaluation of the list continues.
- **Required (`required: true`)**: a missing field is an evaluation error, not false.

Two exceptions:

- On an entity, a list operator treats a missing predicate as an empty list, even when required, and does not read
  the message. `length_eq 0` then matches. To require the predicate, add a scalar condition on the same field.
- `transition` with no current or no previous value is false without an error, even when required (see below).

## Comparing values

Comparison operators (`eq`, `ne`, `lt`, `lte`, `gt`, `gte`, `between`) and the equality inside `in`, `not_in`,
`array_contains` and `transition` compare two values in one way:

- If both are numbers, they compare as numbers. A number is any Go numeric type, a `json.Number`, or text that parses
  as a number. Text or a `json.Number` spelling `NaN` or an infinity counts as text, not a number.
- Otherwise both are turned into text (`fmt` `%v`) and compared byte by byte. `true` equals `"true"`; `"apple"` is
  less than `"banana"`.

Comparison never fails: there is no type check and no coercion error. Text that looks numeric compares as a number,
so `"9.9"` is less than `"10.0"`, and the version `"1.10"` is less than `"1.9"`. A rule that needs text order uses
`regex` or `contains`, not `lt` or `gt`.

Operators do not depend on the field's type: `contains` on the number `12345` reads it as the text `"12345"`.

## Comparison operators

| Operator | True when | Example value |
| --- | --- | --- |
| `eq` | field equals value | `"drone"`, `25.0`, `true` |
| `ne` | field does not equal value | `"offline"` |
| `lt`, `lte` | field is less than (or equal to) value | `20` |
| `gt`, `gte` | field is greater than (or equal to) value | `100` |
| `between` | `min <= field <= max` | `[10, 30]` |

`between` needs a list of exactly two values, `[min, max]`; anything else is an evaluation error. With `min` above
`max` it is never true.

```json
{"field": "sensor.measurement.celsius", "operator": "between", "value": [10, 30]}
```

## Text operators

Each reads the field and the value as text.

| Operator | True when |
| --- | --- |
| `contains` | the field contains the value |
| `starts_with` | the field starts with the value |
| `ends_with` | the field ends with the value |
| `regex` | the pattern matches anywhere in the field (anchor it with `^` and `$` to match the whole) |

```json
{"field": "fleet.drone.id", "operator": "starts_with", "value": "acme.logistics"}
{"field": "fleet.drone.id", "operator": "regex", "value": "^acme\\..*\\.drone\\.\\d+$"}
```

### Regex patterns

A pattern uses Go's regular expression syntax (RE2). It is checked and compiled once, when its rule is loaded
(`expression.Compile`), never while a message is checked. The load refuses the rule when a pattern:

- is not a string;
- holds a substitution token: `$now`, `$entity.`, `$state.`, `$schedule.` or `$message.`
  anywhere in the pattern. The rule engine replaces these tokens in a value before it is evaluated, so the pattern
  that ran would not be the one that was checked;
- is longer than 500 characters;
- is not valid Go regular expression syntax (`regexp.Compile` refuses it).

Any other dollar sign is left alone: `^\$USD`, `[$a-z]+` and an end anchor (`^[a-z]+$`) are accepted. To match a
token's text, write its dollar sign as `[$]`: `[$]now` matches the text `$now`, and the substituter does not touch it.

A condition built in Go code must go through `expression.Compile` before it is evaluated. Evaluating a `regex`
condition that was not compiled, or whose `value` changed after compiling, is an evaluation error. A copy made with
Go assignment keeps the compiled pattern; a copy made through JSON does not, and needs compiling again.

## List operators

| Operator | True when | Value |
| --- | --- | --- |
| `in` | the field equals one of the values | a list: `["active", "standby"]` |
| `not_in` | the field equals none of the values | a list |
| `length_eq` | the list has exactly N items | an integer N |
| `length_gt`, `length_gte` | more than (at least) N items | an integer N |
| `length_lt`, `length_lte` | fewer than (at most) N items | an integer N |
| `array_contains` | one item equals the value | any value |

`in` and `not_in` need a list value; anything else is an evaluation error.

The `length_*` operators and `array_contains` read every triple carrying the predicate, so a join can count the
completion triples its children stamp on a parent:

```json
{"field": "workflow.join.completed", "operator": "length_eq", "value": 3}
```

For the `length_*` operators, N may be an `int`, `int32` or `int64`, a number (its fraction dropped) or integer
text (`"3"`);
anything else is an evaluation error. The field is counted as a list; a single text value counts as one item, a
null as none, and any other single value is an evaluation error. `array_contains` treats a single value as a list of
one.

## Transition

`transition` is true when a field has just changed to a target value, optionally from one of a set of previous
values.

```json
{
  "field": "workflow.plan.status",
  "operator": "transition",
  "from": ["created", "rejected"],
  "value": "drafting"
}
```

| Key | Required | Meaning |
| --- | --- | --- |
| `field` | yes | a predicate on the entity |
| `operator` | yes | `"transition"` |
| `value` | yes | the target value |
| `from` | no | the allowed previous value, or a list of them |

It is true when the entity's current value equals `value` and:

- with `from`, the previous value equals one of `from`;
- without `from`, the previous value differs from the current one.

It is false, without an error even when required, when the entity has no triple for the field, or when there is no
previous value: the first evaluation of an entity is never a transition. The previous value is the state field
`$prev.<field>`, which the caller supplies.

Its three limits:

1. **Entity rules only.** It reads the entity's triples. With no entity it is false, or an evaluation error when
   required, so a rule that checks messages without an entity cannot use it. On a `$state.`, `$prev.`,
   `$entity.lifecycle.` or `$message.` field it is an unsupported-operator error.
2. **"Previous" is the value at this rule's last evaluation of the entity**, not the value just before the current
   one. If quick changes reach the rule together, A then B then C can read as A to C. This is not yet checked: the
   rule engine's entity watcher, which decides how changes reach a rule, is not yet in this repository.
3. **Equality only.** Previous values are recorded as text (by the rule engine, not yet in this repository), and
   `from` and `value` are compared for equality (numeric text still compares as a number, so `"5"` equals `5`).
   A threshold crossing ("battery fell below 20") is an ordinary condition, `lt 20`, whose rule acts when it starts
   to match.

## Combining conditions

`logic` decides how a list's results combine:

- `and`, or no `logic` at all: true when every condition is true.
- `or`: true when at least one condition is true.

Any other value is an evaluation error. An empty list of conditions is true. Every condition is evaluated, and the
first error stops the list.

```json
{
  "conditions": [
    {"field": "fleet.drone.type", "operator": "eq", "value": "quadcopter"},
    {"field": "drone.telemetry.battery", "operator": "lt", "value": 20}
  ],
  "logic": "and"
}
```

## Evaluation errors

Evaluation returns an `*expression.EvaluationError` naming the field and operator when:

| Cause | Example |
| --- | --- |
| a required field is missing | `required: true` on a predicate the entity and message lack |
| the operator is unknown | `"operator": "approx"` |
| the logic is unknown | `"logic": "xor"` |
| the value has the wrong shape | `in` with a single value, `between` with three, `length_eq` with `"three"` |
| the field has the wrong shape | `length_eq` on a number |
| a regex condition is not compiled from its current value | evaluating a condition that skipped `Compile` |

Problems the load catches are refused when the rule loads, before any evaluation: an invalid or templated regex
pattern, and a field that does not start with `$` and is not a declared predicate, or is a rule-opaque one. The field
check is the same for a rule's `conditions` and an action's `when`. What the rule engine does with an evaluation
error at run time is the engine's to say; it is not yet in this repository.

## Telling a stateless caller what it cannot answer

`expression.EnsureStatelessResolvable(condition, lifecycleResolved)` lets code that checks a condition outside a
running rule engine find out first whether it can. It refuses `transition`, `$state.` and `$prev.` fields, and
`$entity.lifecycle.` fields unless the caller resolved lifecycle values itself, so the caller gets an error rather
than a confident false.
