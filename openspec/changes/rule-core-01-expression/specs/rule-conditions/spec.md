# rule-conditions Specification

## ADDED Requirements

### Requirement: A regex pattern is compiled before evaluation

`Compile` SHALL compile each `regex` pattern in a condition slice and keep it on that element. It SHALL refuse a
non-string value, a pattern holding one of `SubstitutionPrefixes()`, a pattern over 500 characters, and one `regexp`
rejects; each refusal is classified invalid and names index and field. Evaluation SHALL never compile, and refuses a
regex condition not compiled from its current value.

#### Scenario: A literal pattern matches as the standard library does

- **WHEN** a `regex` condition with pattern `^a.*z$` is compiled and evaluated against a field holding "abcz"
- **THEN** it matches, and its result equals `regexp.MatchString` for the same pattern and string

#### Scenario: A pattern holding a substitution token is refused

- **WHEN** `Compile` receives a `regex` condition whose pattern is `$entity.id` or `^x-$now`
- **THEN** it returns an error classified invalid that names the field and says to write the dollar sign as `[$]`

#### Scenario: Other dollar signs are literal

- **WHEN** `Compile` receives `[$]now`, `[$a-z]+`, `^\$USD`, or an end-anchored pattern such as `^[a-z]+$`
- **THEN** it compiles without error

#### Scenario: An uncompiled or edited regex condition is refused at evaluation

- **WHEN** a `regex` condition is evaluated without `Compile`, or after its value was changed since `Compile`
- **THEN** evaluation returns an `*EvaluationError` naming the field, and no pattern is compiled

### Requirement: Fields resolve in one fixed order

The evaluator SHALL read `$state.*`, `$prev.*` and `$entity.lifecycle.*` from the state fields, `$message.<path>` from
the message along the dotted path, and any other name from the entity's triples first, then the message. A missing
field SHALL make an optional condition false and a required one an `*EvaluationError`, except where the list-operator
and `transition` requirements say otherwise.

#### Scenario: A required field that is missing is an error

- **WHEN** a condition with `required: true` and operator `eq` names a predicate neither the entity nor the message
  has
- **THEN** evaluation returns an `*EvaluationError`, not false

#### Scenario: An entity triple wins over the message

- **WHEN** a bare field name is both an entity triple and a message key
- **THEN** the entity's value is used

### Requirement: Transition compares the current value with the previous one

A `transition` condition on a bare field name SHALL be true when the entity's current value equals `value` and its
`$prev.` value is in `from`, or differs from the current value when `from` is absent. With no current value or no
previous value it SHALL be false without an error, even when required; with no entity it is an error only when
required.

#### Scenario: A first evaluation is not a transition

- **WHEN** a required `transition` condition is evaluated with no `$prev.` value for its field
- **THEN** the condition is false and no error is returned

### Requirement: Conditions combine by `and` or `or`

`and` SHALL be true exactly when every condition is true, and `or` exactly when at least one is. An empty condition
list SHALL pass. An empty logic value means `and`. Any other logic value SHALL be an `*EvaluationError`.

#### Scenario: An unknown logic value is refused

- **WHEN** an expression has logic `xor`
- **THEN** evaluation returns an `*EvaluationError`

### Requirement: List operators see every triple of a predicate

On an entity, `length_eq`, `length_gt`, `length_gte`, `length_lt`, `length_lte` and `array_contains` SHALL read every
triple carrying the predicate and SHALL treat a missing predicate as an empty list, even when required, without
reading the message. `not_in` SHALL negate `in`; `between [lo, hi]` SHALL equal `gte lo` and `lte hi`.
`RegisteredOperators` names exactly the operators the evaluator runs.

#### Scenario: A missing predicate counts as zero

- **WHEN** a required `length_eq 0` condition is evaluated on an entity without the predicate
- **THEN** the condition is true

### Requirement: A stateless caller is told when a condition needs state

`EnsureStatelessResolvable` SHALL refuse a `transition` condition and a `$state.*` or `$prev.*` field, and SHALL
refuse a `$entity.lifecycle.*` field unless the caller resolved lifecycle state. Otherwise it returns nil.

#### Scenario: A state field is refused without state

- **WHEN** `EnsureStatelessResolvable` receives a condition on `$state.iteration`
- **THEN** it returns an `*EvaluationError` naming the field
