# rule-definitions Specification

## ADDED Requirements

### Requirement: A rule definition has one strict decode

`DecodeDefinition(data, source)` SHALL decode one rule object, and every source of rules SHALL call it. It SHALL
refuse a key that `Definition` or a condition does not declare, naming the key, the rule and the source. An absent
`enabled` SHALL mean enabled, an absent condition `required` SHALL mean not required, and an absent `logic` SHALL mean
`and`.

#### Scenario: An undeclared key is refused

- **WHEN** a rule carries `related_patterns`, `entity.watch_buckets` or a misspelt key such as `on_entr`, or a
  condition carries `requried`
- **THEN** the decode fails, classified invalid, naming the key, the rule ID and the source

#### Scenario: Absent keys

- **WHEN** a rule omits `enabled` and `logic`, and a condition omits `required`
- **THEN** the rule is enabled, its conditions combine by `and`, and a missing field is not an evaluation error

### Requirement: A round trip keeps a definition's values

A definition marshalled and decoded again SHALL have every field equal to the one decoded from the original bytes,
except each action's `Body`, which SHALL be equal in canonical form (numbers as written, keys sorted, no spacing, no
HTML escaping), not byte for byte.

#### Scenario: Stored form decodes equal

- **WHEN** a rule's file bytes are decoded, and the resulting definition is marshalled and decoded again
- **THEN** every field is equal and each action's `Body` is equal in canonical form, so its firing-cap key is
  unchanged, though the marshalled bytes are compacted and HTML-escaped

### Requirement: Load returns a ready rule or refuses it

`Load` SHALL return a rule only when every condition and every action `when` has compiled and every action has passed
the check of its type's handler in the handler set given to `Load`. Otherwise it SHALL return an error classified
invalid that names the rule ID, the source (a file path, `inline_rules[<index>]`, or `rules.<id>@<revision>`), the
position (such as `on_enter[1].when[0]`) and the cause. A disabled rule SHALL be checked the same way. Nothing SHALL
skip a refused rule and continue.

#### Scenario: An unregistered action type is refused at load

- **WHEN** a rule's `on_enter[0]` has a type that is not in the handler set given to `Load`
- **THEN** `Load` fails naming the type, the rule, the source and `on_enter[0]`

#### Scenario: Rule-level checks

- **WHEN** a rule has a dotted ID, a negative `fire_every_n_events`, a condition on an undeclared or rule-opaque
  predicate, a refused regex pattern, or `rerun_on_recovery` with neither `on_enter` nor `on_recovery`
- **THEN** `Load` fails naming the rule, the source and the cause, whether the rule is enabled or not

#### Scenario: An enabled cron rule is refused until timers exist

- **WHEN** an enabled rule has `type: "cron"`
- **THEN** `Load` fails naming the rule and the source, and saying cron rules are not yet supported

#### Scenario: A disabled cron rule gets the common checks

- **WHEN** a disabled rule has `type: "cron"` and passes the checks every rule gets
- **THEN** `Load` returns it

#### Scenario: A core-only pack loads with no product handler

- **WHEN** semboids' zone-steering rules are loaded with the set `NewHandlers` builds from no product handlers
- **THEN** every rule loads

### Requirement: The action handler set is built once

`NewHandlers(product)` SHALL be the one way to build the handler set `Load` takes: the nine built-in types plus the
product's map. It SHALL refuse a product type name that reuses a built-in name, an empty name or a nil handler,
classified invalid and naming the type. `Load` SHALL refuse a handler set that `NewHandlers` did not build.

#### Scenario: A product name reuses a built-in one

- **WHEN** `NewHandlers` is given a product handler named `publish`
- **THEN** it fails, classified invalid, naming `publish`

#### Scenario: No product handlers

- **WHEN** `NewHandlers` is given no product handlers
- **THEN** the set holds the nine built-in types and no other

### Requirement: An action is its body

An action's `Body` SHALL hold the action's JSON object byte for byte as decoded, and SHALL be the action: `Type`,
`When`, `ID` and `MaxIterations` SHALL be read-only copies the decode fills from it, and the core SHALL read only those
four keys. Marshalling an action SHALL write its `Body`. A rule stored and read back keeps its values, not its bytes.

#### Scenario: A product's settings pass through untouched

- **WHEN** semteams' rule-family fixtures (commit `a331910e`) are decoded
- **THEN** each `on_enter[0]` body equals its bytes in the file, and its `tool_choice` and `response_format` decode
  equal to the fixtures' expected values

### Requirement: Load refuses an action not built by decoding

`Load` SHALL refuse an action with no `Body`, or whose four core fields differ from what its `Body` decodes to,
saying to build the action by decoding its JSON.

#### Scenario: An action not built by decoding is refused

- **WHEN** a rule given to `Load` holds an action written as a Go literal with `Type` set and no `Body`, or an action
  whose `Type` was changed after decode
- **THEN** `Load` fails naming the position, the rule and the source, and saying to build the action by decoding

### Requirement: Built-in actions refuse settings they do not read

Each built-in action type SHALL refuse at load a body key that neither the core nor that type reads, naming the key,
the action's position, the rule and the source. `publish` SHALL refuse a declared subject in `user.response.>`; the
triple types SHALL refuse a predicate that is not a declared literal; and `max_iterations` SHALL be 0 or more.

#### Scenario: A misspelt setting

- **WHEN** an `add_triple` action carries `predicat`
- **THEN** `Load` fails naming `predicat`, the position, the rule and the source

#### Scenario: Reserved subject and dynamic predicate

- **WHEN** a `publish` action declares `user.response.x`, or an `add_triple` predicate holds `$`
- **THEN** `Load` fails naming the cause

### Requirement: The firing-cap key is the action's id or its canonical body

An action's firing-cap key SHALL be its `id` when set, and otherwise a hash of the rule ID, the type and the whole
body in canonical form (numbers as written, keys sorted, no spacing, no HTML escaping), `max_iterations` and `when`
included.

#### Scenario: Storing and reformatting keep the key

- **WHEN** an action body holding `&` is stored and read back compacted and escaped, or its file is reformatted
- **THEN** its key is unchanged

#### Scenario: An edit changes the key

- **WHEN** any value in the body changes, `max_iterations` included, and no `id` is set
- **THEN** its key changes; with `id` set, the key is the `id`
