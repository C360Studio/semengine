# rule-definitions Specification

## ADDED Requirements

### Requirement: Load refuses a condition its path can never fill

An expression rule with no `entity.pattern` runs on messages: `Load` SHALL refuse it when a condition's field starts
with `$entity.` or a string value contains `$entity.`. One with an `entity.pattern` runs on entities: `Load` SHALL
refuse it when a condition's field starts with `$message.` or a string value contains `$message.`. The refusal names
the rule, the source and `conditions[<i>]`, enabled or not.

#### Scenario: An entity token in a message rule

- **WHEN** a rule with no `entity.pattern` has a condition whose value is `zone-$entity.id`
- **THEN** `Load` fails naming the rule, the source and `conditions[0]`

#### Scenario: A message field in an entity rule

- **WHEN** a disabled rule with an `entity.pattern` has a condition on `$message.x`
- **THEN** `Load` fails naming the rule, the source and `conditions[0]`
