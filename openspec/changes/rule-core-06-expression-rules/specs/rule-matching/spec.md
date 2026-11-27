# rule-matching Specification

## ADDED Requirements

### Requirement: A loaded rule answers whether it matches an entity

`(*Rule).Matches` SHALL answer whether the conditions of a rule `Load` returned hold for an entity's state, with no
rule processor running: a match with no error, no match with no error, or an error meaning the answer could not be
found. A rule's cooldown SHALL NOT apply and SHALL NOT be refused.

#### Scenario: A matching entity

- **WHEN** a loaded rule's conditions hold for an entity inside its `entity.pattern` and `Matches` is called
- **THEN** it returns true and no error

### Requirement: A stateless match checks in a fixed order

`Matches` SHALL apply these in order, the first that applies deciding: a nil rule, context or state, refused; a
malformed entity ID, an error; a disabled rule, a rule with no `entity.pattern`, an entity outside the pattern, or no
conditions, false, as the rule processor never evaluates that rule for that entity; then the stateless refusals, then
evaluation. Each false comes with no error.

#### Scenario: Missing inputs are refused

- **WHEN** `Matches` is called on a nil rule, or with a nil context or entity state
- **THEN** it returns an error and no verdict

#### Scenario: A malformed entity ID is an error, even for a disabled rule

- **WHEN** `Matches` is called on a loaded disabled rule for an entity state whose ID is not a valid entity ID
- **THEN** it returns an error and no verdict

#### Scenario: A disabled rule owes nothing, even without a pattern

- **WHEN** `Matches` is called on a loaded disabled cron rule, which has no `entity.pattern` and no conditions, for an
  entity with a well-formed ID
- **THEN** it returns false and no error

#### Scenario: An enabled rule with no pattern owes nothing

- **WHEN** `Matches` is called on a loaded enabled rule with no `entity.pattern`, for an entity with a well-formed ID
- **THEN** it returns false and no error

#### Scenario: An entity the rule does not watch

- **WHEN** `Matches` is called for an entity whose ID is well formed and does not match the rule's
  `entity.pattern`
- **THEN** it returns false and no error

### Requirement: Firing and asking share one match

The firing entity match and `Matches` SHALL share one implementation. For an enabled rule with an `entity.pattern`, an
entity inside it, and conditions that read no `transition`, `$state.*`, `$prev.*` or `$entity.lifecycle.*` in either
field or value, the firing match run with no lifecycle lookup and `Matches` SHALL give the same answer: equal verdicts,
or an error from both.

#### Scenario: The two agree inside the pattern

- **WHEN** the firing entity match, run with no lifecycle lookup, and `Matches` are both run for one loaded rule with an
  `entity.pattern` and one entity inside it, the rule's conditions reading no `transition`, `$state.*`, `$prev.*` or
  `$entity.lifecycle.*` in field or value
- **THEN** both return the same verdict, or both return an error

### Requirement: Outside the pattern, only Matches checks the pattern

The firing entity match SHALL NOT check the rule's `entity.pattern`, as its caller picks rules by it; `Matches` SHALL
answer false for an entity outside the pattern.

#### Scenario: An entity outside the pattern

- **WHEN** the firing entity match and `Matches` are run for one loaded rule and an entity with a well-formed ID outside
  its `entity.pattern`, whose state satisfies the rule's conditions
- **THEN** the firing match returns true and `Matches` returns false, both with no error

### Requirement: A stateless match refuses what it cannot answer

`Matches` SHALL return an error, never a verdict, when a condition is a `transition` or its field reads `$state.*`,
`$prev.*` or `$entity.lifecycle.*`. The error for `$entity.lifecycle.*` SHALL say that the condition cannot be
answered without a lifecycle lookup.

#### Scenario: A state field is refused

- **WHEN** `Matches` is called on a rule with a condition on `$state.iteration`
- **THEN** it returns an error naming the field and no verdict

#### Scenario: A lifecycle field is refused

- **WHEN** `Matches` is called on a rule with a condition on a `$entity.lifecycle.` field
- **THEN** it returns an error naming the field and saying it cannot be answered without a lifecycle lookup

### Requirement: A missing required field is no match

The firing entity match, the firing message match and `Matches` SHALL treat an absent required field as no match,
with no error. Every other evaluation error SHALL be returned with the rule ID, never logged at Debug and turned into
a bare false.

#### Scenario: An entity without a required field

- **WHEN** the firing entity match or `Matches` runs for an entity lacking a field a condition marks required
- **THEN** it returns false and no error

#### Scenario: A comparison that cannot be made

- **WHEN** the firing message match runs a `length_eq "abc"` condition on a field
- **THEN** it returns an error naming the rule ID and the field

### Requirement: A condition is never compared with an unfilled template

Before evaluation, a condition value holding a substitution token SHALL be filled from the entity or the message under
the caller's context. If any token stays unfilled, every match SHALL return an error naming the rule ID, the field and
the token, and SHALL NOT log it. A `regex` condition SHALL keep the pattern `Load` compiled.

#### Scenario: A missing value fails the match

- **WHEN** a condition's value is `$entity.triple.x.y.missing`, the entity has no such predicate, and a triple holds
  that text
- **THEN** the firing match and `Matches` each return an error naming the token, and neither returns true

#### Scenario: A regex holding a literal dollar sign

- **WHEN** a loaded rule has a `regex` condition whose pattern ends in `$`
- **THEN** the match evaluates it with the compiled pattern and returns a verdict

### Requirement: A message rule reads the payload's declared fields

A message match SHALL read `$message.*` conditions and condition values from the fields a payload declares by
implementing `message.RuleReadable`, through one projection that the firing path also uses for an action's message
data. A field the payload does not declare SHALL NOT be readable. A `message.GenericJSONPayload` SHALL be read through
its data map.

#### Scenario: A typed payload's declared field

- **WHEN** a message rule's condition reads a field a typed payload declares in `RuleFields`
- **THEN** the condition resolves against the declared value

#### Scenario: An undeclared field is not reachable

- **WHEN** a condition reads a struct field the payload leaves out of `RuleFields`
- **THEN** the field is absent

### Requirement: An unreadable payload is reported once per rule and type

When a message's payload does not implement `message.RuleReadable`, the message match SHALL return no match and no
error, SHALL NOT affect any other rule, and SHALL log at Warn the rule ID, the payload type and the remedy, once for
each loaded rule and payload type. Matching SHALL be safe for concurrent use, and the report SHALL be the only state a
match writes. A rule loaded again SHALL report again.

#### Scenario: Repeated unreadable messages

- **WHEN** many goroutines match one loaded rule against messages of one unreadable payload type
- **THEN** exactly one Warn names that rule and payload type
