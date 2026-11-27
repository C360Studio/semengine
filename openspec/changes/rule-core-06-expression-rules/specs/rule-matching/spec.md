# rule-matching Specification

## ADDED Requirements

### Requirement: A loaded rule answers whether it matches an entity

`(*Rule).Matches` and `(*Rule).MatchesWithLifecycle` SHALL answer whether the conditions of a rule `Load` returned hold
for an entity's state, with no rule processor running: a match with no error, no match with no error, or an error
meaning the answer could not be found. A disabled rule and a rule with no conditions SHALL NOT match. A rule's
cooldown SHALL NOT apply and SHALL NOT be refused.

#### Scenario: A matching entity

- **WHEN** a loaded rule's conditions hold for an entity and `Matches` is called
- **THEN** it returns true and no error

#### Scenario: A disabled rule owes nothing

- **WHEN** `Matches` is called on a loaded rule whose `enabled` is false
- **THEN** it returns false and no error

#### Scenario: Missing inputs are refused

- **WHEN** `Matches` is called on a nil rule, or with a nil context or entity state, or `MatchesWithLifecycle` with a
  nil manager
- **THEN** it returns an error and no verdict; for the nil manager the error says to call `Matches` instead

### Requirement: Firing and asking share one match

The firing entity match and `Matches` SHALL share one implementation. Whenever both give a verdict for the
same rule and entity, the verdicts SHALL be equal, and whenever the firing match returns an error, `Matches`
SHALL return one too.

#### Scenario: The two agree

- **WHEN** the firing entity match and `Matches` are both run for one loaded rule and one entity, and both give a
  verdict
- **THEN** the verdicts are equal

### Requirement: A stateless match refuses what it cannot answer

`Matches` and `MatchesWithLifecycle` SHALL return an error, never a verdict, when a condition is a `transition`, reads
`$state.*` or `$prev.*`, reads `$entity.lifecycle.*` through `Matches`, or reads `$entity.lifecycle.*` when the
manager's lookup of the entity failed; that last error SHALL carry the lookup's error. A failed lookup SHALL NOT affect
a rule with no lifecycle condition. An absent required field SHALL be an error for which
`errors.Is(err, expression.ErrRequiredFieldMissing)` holds.

#### Scenario: A state field is refused

- **WHEN** `Matches` is called on a rule with a condition on `$state.iteration`
- **THEN** it returns an error naming the field and no verdict

#### Scenario: A missing required field is an error to the asker

- **WHEN** `Matches` is called for an entity lacking a field a condition marks required
- **THEN** it returns an error for which `errors.Is(err, expression.ErrRequiredFieldMissing)` holds

### Requirement: A stateless match answers only what production asks

`Matches` and `MatchesWithLifecycle` SHALL refuse, classified invalid, a rule with no `entity.pattern`, since such a
rule runs only on messages. For an entity whose ID is outside the rule's `entity.pattern` they SHALL return false with
no error, since the rule is never evaluated for it. A malformed entity ID SHALL be an error.

#### Scenario: A message rule is refused

- **WHEN** `Matches` is called on a loaded rule with no `entity.pattern`
- **THEN** it returns an error classified invalid and no verdict

#### Scenario: An entity the rule does not watch

- **WHEN** `Matches` is called for an entity whose ID does not match the rule's `entity.pattern`
- **THEN** it returns false and no error

### Requirement: A missing required field is no match when firing

The firing entity match and the firing message match SHALL treat an absent required field as no match, with no error.
Every other evaluation error SHALL be returned with the rule ID, never logged at Debug and turned into a bare false.

#### Scenario: An entity without a required field

- **WHEN** the firing entity match runs for an entity lacking a field a condition marks required
- **THEN** it returns false and no error

#### Scenario: A comparison that cannot be made

- **WHEN** the firing message match runs a `gt "abc"` condition on a numeric field
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
