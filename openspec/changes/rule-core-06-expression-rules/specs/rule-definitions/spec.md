# rule-definitions Specification

## MODIFIED Requirements

### Requirement: Load returns a ready rule or refuses it

`Load` SHALL return a rule only when every condition and every action `when` has compiled and every action has passed
the check of its type's handler in the handler set given to `Load`. Otherwise it SHALL return an error classified
invalid that names the rule ID, the source (a file path, `inline_rules[<index>]`, or `rules.<id>@<revision>`), the
position (such as `on_enter[1].when[0]`) and the cause. A disabled rule SHALL be checked the same way. Nothing SHALL
skip a refused rule and continue. An expression rule with no `entity.pattern` runs on messages and SHALL be refused
when a condition's field or value names `$entity.`; one with an `entity.pattern` runs on entities and SHALL be refused
when a condition's field or value names `$message.`.

#### Scenario: An unregistered action type is refused at load

- **WHEN** a rule's `on_enter[0]` has a type that is not in the handler set given to `Load`
- **THEN** `Load` fails naming the type, the rule, the source and `on_enter[0]`

#### Scenario: Rule-level checks

- **WHEN** a rule has a dotted ID, a negative `fire_every_n_events`, a condition on an undeclared or rule-opaque
  predicate, a refused regex pattern, or `rerun_on_recovery` with neither `on_enter` nor `on_recovery`
- **THEN** `Load` fails naming the rule, the source and the cause, whether the rule is enabled or not

#### Scenario: A condition its path can never fill

- **WHEN** a rule with no `entity.pattern` has a condition whose value is `$entity.id`, or a rule with an
  `entity.pattern` has a condition on `$message.x`
- **THEN** `Load` fails naming the rule, the source and `conditions[0]`, whether the rule is enabled or not

#### Scenario: An enabled cron rule is refused until timers exist

- **WHEN** an enabled rule has `type: "cron"`
- **THEN** `Load` fails naming the rule and the source, and saying cron rules are not yet supported

#### Scenario: A disabled cron rule gets the common checks

- **WHEN** a disabled rule has `type: "cron"` and passes the checks every rule gets
- **THEN** `Load` returns it

#### Scenario: A core-only pack loads with no product handler

- **WHEN** semboids' zone-steering rules are loaded with the set `NewHandlers` builds from no product handlers
- **THEN** every rule loads
