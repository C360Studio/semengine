# graph-gateway Delta

## MODIFIED Requirements

### Requirement: Answers follow the gateway's schema and are checked

Every root field SHALL be nullable. `entity` SHALL answer `{entity {id triples}, kvRevision}` from a stored value
decoded by `graph.UnmarshalEntityState`, refusing a missing entity or a zero KV revision (pin ADR-091, §4).
`entitiesByPrefix` SHALL answer `{entities, total, next_cursor}` after `graph.ValidateDecodedEntityStates`, an empty
`prefix` matching every entity, and SHALL refuse a page whose `missing` lists any entity. `entitiesByPredicate` SHALL
answer `{entities}`.

#### Scenario: A reply that does not fit

- **WHEN** a verb's reply lacks a field the schema declares non-null, or the `entity` reply carries `kv_revision` 0
- **THEN** that root field is `null`, the rest of `data` stands, the error names its path, and no partial value is
  returned (`TestReplyNotFittingSchemaNamesPath`, `TestEntityZeroRevisionRefused`)

#### Scenario: A prefix page holding an invalid entity

- **WHEN** a `prefix` reply holds an entity that `graph.ValidateDecodedEntityStates` refuses
- **THEN** `data.entitiesByPrefix` is `null` with an error naming the path, HTTP 200 (`TestPrefixReplyValidated`)

#### Scenario: A prefix page naming a damaged entity

- **WHEN** a `prefix` reply lists an entity in `missing`
- **THEN** `data.entitiesByPrefix` is `null` with an error naming the path and the entity, HTTP 200
