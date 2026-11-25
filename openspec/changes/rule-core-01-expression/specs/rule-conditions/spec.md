# rule-conditions Specification

## ADDED Requirements

### Requirement: A regex pattern is compiled when its condition is loaded

`ConditionExpression.Compile` SHALL compile a `regex` condition's pattern once and keep it on the condition. It
SHALL refuse a value that is not a string, a pattern containing `$` followed by an ASCII letter, a pattern over the
authoring limits, and a pattern Go's `regexp` rejects. Each refusal is classified invalid and names the field.
Evaluation SHALL never compile a pattern, and refuses an uncompiled `regex` condition. The package holds no regex
cache and no package-level state.

#### Scenario: A literal pattern is compiled once and matches

- **WHEN** a `regex` condition with pattern `^a.*z$` is compiled and evaluated against a field holding "abcz"
- **THEN** it matches, and its result equals `regexp.MatchString` for the same pattern and string

#### Scenario: A templated pattern is refused at load

- **WHEN** `Compile` receives a `regex` condition whose pattern is `$entity.id` or `^x-$now`
- **THEN** it returns an error classified invalid that names the field and says to write a literal dollar sign as `[$]`

#### Scenario: A literal dollar sign written as a class is accepted

- **WHEN** `Compile` receives `[$]now` or an end-anchored pattern such as `^[a-z]+$`
- **THEN** it compiles without error

#### Scenario: An uncompiled regex condition is refused at evaluation

- **WHEN** a `regex` condition that was never compiled is evaluated
- **THEN** evaluation returns an `*EvaluationError` naming the field, and no pattern is compiled

### Requirement: Fields resolve in one fixed order

The evaluator SHALL resolve a condition's field as follows: `$state.*`, `$prev.*` and `$entity.lifecycle.*` from the
state fields; `$message.<path>` from the message along the dotted path; `transition` from the entity's triple and its
`$prev.` value; any other name from the entity's triples first, then from the message. A missing field SHALL make an
optional condition false and a required condition an `*EvaluationError`.

#### Scenario: A required field that is missing is an error

- **WHEN** a condition with `required: true` names a predicate the entity does not carry and the message lacks
- **THEN** evaluation returns an `*EvaluationError`, not false

#### Scenario: An entity triple wins over the message

- **WHEN** a bare field name is both an entity triple and a message key
- **THEN** the entity's value is used

### Requirement: Conditions combine by `and` or `or`

`and` SHALL be true exactly when every condition is true, and `or` exactly when at least one is. An empty condition list
SHALL pass. Any other logic value SHALL be an `*EvaluationError`. An empty logic value SHALL mean `or` [or `and`, if
the owner chooses D7].

#### Scenario: An unknown logic value is refused

- **WHEN** an expression has logic `xor`
- **THEN** evaluation returns an `*EvaluationError`

### Requirement: List operators see every triple of a predicate

`length_eq`, `length_gt`, `length_gte`, `length_lt`, `length_lte` and `array_contains` SHALL read every triple that
carries the predicate, and SHALL treat a predicate the entity does not carry as an empty list. `not_in` SHALL be the
negation of `in`, and `between [lo, hi]` SHALL equal `gte lo` and `lte hi`. `RegisteredOperators` SHALL name exactly
the operators the evaluator runs.

#### Scenario: A missing predicate counts as zero

- **WHEN** `length_eq 0` is evaluated on an entity without the predicate
- **THEN** the condition is true

### Requirement: A stateless caller is told when a condition needs state

`EnsureStatelessResolvable` SHALL refuse a `transition` condition and a `$state.*` or `$prev.*` field, and SHALL refuse
a `$entity.lifecycle.*` field unless the caller resolved lifecycle state. In every other case it returns nil.

#### Scenario: A state field is refused without state

- **WHEN** `EnsureStatelessResolvable` receives a condition on `$state.iteration`
- **THEN** it returns an `*EvaluationError` naming the field
