# rule-substitution Specification

## ADDED Requirements

### Requirement: A template is read once

`ExecutionContext.SubstituteVariables` SHALL read a template once, left to right, replacing each token it resolves
with that token's value. Text it inserts SHALL NOT be read again, and text outside tokens SHALL come out unchanged. A
template holding no `$` SHALL be returned as it is.

#### Scenario: A value that looks like a token is kept

- **WHEN** an entity's triple `test.fixture.note` holds `see $message.secret`, the message data holds `secret`, and
  the template is `note: $entity.triple.test.fixture.note`
- **THEN** the result is `note: see $message.secret`, and no token is reported unresolved

### Requirement: A token ends where its name ends

The tokens SHALL be `$now` and those beginning with one of `expression.SubstitutionPrefixes()`. A fixed name (`$now`,
`$entity.id`, the six `$entity.<segment>` names, the four `$entity.lifecycle.*` names, `$state.iteration`,
`$state.max_iterations`) SHALL be a token only when the character after it is not a letter, digit or `_`.
`$entity.triple.<path>` SHALL take the longest run of segments of `[a-z0-9_-]` joined by single dots, and
`$message.<path>` of `[A-Za-z0-9_]`, not counting a final dot.

#### Scenario: Fixed names joined by dots

- **WHEN** the entity ID is `c360.platform1.robotics.mav1.drone.001` and the template is
  `$entity.org.$entity.platform.chain.$entity.instance`
- **THEN** the result is `c360.platform1.chain.001`

#### Scenario: A sentence ends on a token

- **WHEN** the entity holds `agent.run.entity_id` = `X` and the template is
  `ID is $entity.triple.agent.run.entity_id. Call`
- **THEN** the result is `ID is X. Call`

### Requirement: An unresolved token stays and is reported

Text starting with `$now` or a prefix of `expression.SubstitutionPrefixes()` that does not resolve SHALL reach
through the longest run of dot-joined `[A-Za-z0-9_-]` segments, a final dot excluded, SHALL stay as written, and SHALL
be named, with the rule ID, in one warning. An absent predicate's `.value`, `.length` or `.triples` (`""`, `0`, `[]`)
counts as resolved. Text starting with any other `$` SHALL stay and SHALL NOT be reported.

#### Scenario: A longer predicate that is absent is not read as a shorter one

- **WHEN** the entity holds only `agent.lineage.researcher-plan` = `P` and the template is
  `$entity.triple.agent.lineage.researcher-plan-entity`
- **THEN** the result is the template unchanged, and the token is reported unresolved

#### Scenario: Names that extend or miss a fixed name

- **WHEN** the template is `$nowhere $entity.idx $entity.lifecycle.x $state.last_transition $schedule.last_fired_at`
  and the context has an entity ID, a state and a lifecycle manager knowing the entity
- **THEN** the result is the template unchanged, and the warning names exactly those five

#### Scenario: Names outside the namespaces

- **WHEN** the template is `$caller.id and $related.id`
- **THEN** the result is the template unchanged, and nothing is reported

### Requirement: The values of a predicate

`$entity.triple.<predicate>.triples` SHALL be a JSON array of every value of the predicate on the entity: each scalar
triple one value, each list-shaped object its elements. `$entity.triple.<predicate>.length` SHALL be the number of
values `.triples` lists, and `0` when the predicate is absent. Bare `$entity.triple.<predicate>` SHALL be the first
triple's object.

#### Scenario: A predicate stamped as several scalar triples

- **WHEN** the entity holds three triples on `gather.child.completed` with objects `a`, `b` and `c`
- **THEN** `.length` gives `3` and `.triples` gives `["a","b","c"]`

#### Scenario: One list-valued triple

- **WHEN** the entity holds one triple on `coordinator.decision.subtopics` whose object is the list `["x","y"]`
- **THEN** `.length` gives `2`

### Requirement: The value of a 3-part predicate

`$entity.triple.<predicate>.value` SHALL be the suffix form only when `<predicate>` is a valid 3-part predicate
(`vocabulary.ParsePredicate`); it SHALL then give the first triple's object, and `""` with no report when the
predicate is absent. Otherwise the whole path SHALL be read as the predicate.

#### Scenario: An absent predicate gives an empty string

- **WHEN** the entity has no triple on `gather.child.result` and the template is
  `[$entity.triple.gather.child.result.value]`
- **THEN** the result is `[]`, and nothing is reported

#### Scenario: A predicate whose last part is value

- **WHEN** the entity holds `autoresearch.best.value` = `0.82` and the template is
  `$entity.triple.autoresearch.best.value`
- **THEN** the result is `0.82`, read as the predicate `autoresearch.best.value`, since `autoresearch.best` is not a
  3-part predicate

### Requirement: One token keeps its type

`ExecutionContext.SubstituteVariablesTyped` SHALL return a value with its own type, and true, only when the whole
template is one bare `$entity.triple.<predicate>`, `$message.<path>`, `$state.iteration` or `$state.max_iterations`
token that resolves. Otherwise it SHALL return false, and the caller substitutes as text.

#### Scenario: A number stays a number

- **WHEN** the entity holds `test.fixture.score` = `0.82` as a float and the template is
  `$entity.triple.test.fixture.score`
- **THEN** the typed result is the float `0.82`

#### Scenario: Text around a token

- **WHEN** the template is `score=$entity.triple.test.fixture.score`
- **THEN** `SubstituteVariablesTyped` returns false
