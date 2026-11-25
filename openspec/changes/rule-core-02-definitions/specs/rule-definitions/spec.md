# rule-definitions Specification

## ADDED Requirements

### Requirement: A rule definition has one strict decode

`DecodeDefinition(data, source)` SHALL decode one rule object, and every source of rules SHALL call it. It SHALL
refuse a key that `Definition` or a condition does not declare, naming the key, the rule and the source. An absent
`enabled` SHALL mean enabled, an absent condition `required` SHALL mean not required, and an absent `logic` SHALL mean
`and`. A definition marshalled and decoded again SHALL equal the one decoded from the original bytes.

#### Scenario: Stored form decodes equal

- **WHEN** a rule's file bytes are decoded, and the resulting definition is marshalled and decoded again
- **THEN** the two definitions are equal, though the marshalled bytes are compacted and HTML-escaped

#### Scenario: An undeclared key is refused

- **WHEN** a rule carries `related_patterns`, a misspelt key such as `on_entr`, or a condition carries `requried`
- **THEN** the decode fails, classified invalid, naming the key, the rule ID and the source

#### Scenario: Absent keys

- **WHEN** a rule omits `enabled` and `logic`, and a condition omits `required`
- **THEN** the rule is enabled, its conditions combine by `and`, and a missing field is not an evaluation error

### Requirement: Load returns a ready rule or refuses it

`Load` SHALL return a rule only when every condition and every action `when` has compiled and every action has passed
its handler's check. Otherwise it SHALL return an error classified invalid that names the rule ID, the source (a file
path, `inline_rules[<index>]`, or `rules.<id>@<revision>`), the position (such as `on_enter[1].when[0]`) and the
cause. A disabled rule SHALL be checked the same way. Nothing SHALL skip a refused rule and continue.

#### Scenario: An unregistered action type is refused at load

- **WHEN** a rule's `on_enter[0]` has a type that is neither built in nor in the handlers given to `Load`
- **THEN** `Load` fails naming the type, the rule, the source and `on_enter[0]`

#### Scenario: Rule-level checks

- **WHEN** a rule has a dotted ID, a negative `fire_every_n_events`, a condition on an undeclared or rule-opaque
  predicate, a refused regex pattern, or `rerun_on_recovery` with neither `on_enter` nor `on_recovery`
- **THEN** `Load` fails naming the rule, the source and the cause, whether the rule is enabled or not

#### Scenario: An entity watch bucket other than ENTITY_STATES

- **WHEN** a rule's `entity.watch_buckets` names a bucket other than `ENTITY_STATES`
- **THEN** `Load` fails with the classified code `entity_watch_bucket_unsupported`
  (`ErrorCodeEntityWatchBucketUnsupported`), naming the bucket

#### Scenario: An enabled cron rule is refused until timers exist

- **WHEN** an enabled rule has `type: "cron"`
- **THEN** `Load` fails naming the rule and the source, and saying cron rules are not yet supported

#### Scenario: A disabled cron rule gets the common checks

- **WHEN** a disabled rule has `type: "cron"` and passes the checks every rule gets
- **THEN** `Load` returns it

#### Scenario: A core-only pack loads with no product handler

- **WHEN** semboids' zone-steering rules are loaded with no product handler
- **THEN** every rule loads

### Requirement: An action keeps its body as written

An action's `Body` SHALL hold the action's JSON object byte for byte as decoded. The core SHALL read only `type`,
`when`, `id` and `max_iterations` from it.

#### Scenario: A product's settings pass through untouched

- **WHEN** semteams' rule-family fixtures (commit `a331910e`) are decoded
- **THEN** each `on_enter[0]` body equals its bytes in the file, and its `tool_choice` and `response_format` decode
  equal to the fixtures' expected values

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
