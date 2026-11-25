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
`$entity.triple.<path>` and `$message.<path>` SHALL take the longest run of path segments joined by single dots, not
counting a final dot.

#### Scenario: Fixed names joined by dots

- **WHEN** the entity ID is `c360.platform1.robotics.mav1.drone.001` and the template is
  `$entity.org.$entity.platform.chain.$entity.instance`
- **THEN** the result is `c360.platform1.chain.001`

#### Scenario: A sentence ends on a token

- **WHEN** the entity holds `agent.run.entity_id` = `X` and the template is
  `ID is $entity.triple.agent.run.entity_id. Call`
- **THEN** the result is `ID is X. Call`

### Requirement: An unresolved token stays and is reported

A token that does not resolve SHALL stay as written, and SHALL be reported in one warning naming each such token and
the rule ID.

#### Scenario: A longer predicate that is absent is not read as a shorter one

- **WHEN** the entity holds only `agent.lineage.researcher-plan` = `P` and the template is
  `$entity.triple.agent.lineage.researcher-plan-entity`
- **THEN** the result is the template unchanged, and the token is reported unresolved

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
