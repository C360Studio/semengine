# graph-transport-boundary Delta

## ADDED Requirements

### Requirement: A batch reply accounts for every requested ID

graph-ingest SHALL answer `graph.ingest.query.batch` with every requested entity it can read in `entities` and every
other requested ID in `missing`, with one reason from the closed set: `not_found` for a key the read did not find,
`error` for an empty ID, which is never read, and `damaged` for a stored value the stored-value rule refuses.

#### Scenario: One damaged entity among good ones

- **WHEN** A and C are stored, B holds bytes the stored-value rule refuses, and a batch asks for A, B, C and an absent
  ID
- **THEN** the reply holds A and C, and `missing` lists B as `damaged` and the absent ID as `not_found`; the entity
  verb on B refuses fatal `graph_state_reset_required` (`TestIntegration_BatchQuery_NamesDamagedEntities`)

### Requirement: A damaged entity does not refuse a batch

A damaged entity SHALL NOT make the batch a refusal, even when every requested ID is damaged; it SHALL still be
recorded as the entity verb records it. Any other read fault SHALL fail the whole batch, class transient, code
`internal`. The prefix page SHALL keep refusing a page that holds a damaged entity.

#### Scenario: Every requested entity damaged

- **WHEN** a batch asks only for damaged entities
- **THEN** the reply holds no entities and lists each in `missing` as `damaged`, and is not a refusal
  (`TestIntegration_BatchQuery_AllDamaged`)

#### Scenario: Damage beside another read fault

- **WHEN** one requested entity is damaged and another's read fails for any other reason
- **THEN** the batch is refused with class transient and code `internal` (`TestBatchDamageBesideFaultIsTransient`)
