# graph-index Delta

## ADDED Requirements

### Requirement: An entity indexed as it stands is not re-indexed

graph-index SHALL keep, per entity, a record of the rows it last indexed for it. When an entity's stored value
decodes, and the rows it asks for equal a complete record, graph-index SHALL neither list nor write that entity's
rows, and SHALL complete its revision. An entity with no record, or a record in a format this code does not know,
SHALL be listed as before and then recorded.

#### Scenario: A warm restart over an unchanged store

- **WHEN** graph-index has indexed a store, is stopped, and is started again with nothing changed
- **THEN** the second start lists no index bucket and writes no index row, and every read answers as before the stop
- **AND** the test that verifies this is `TestWarmRestartSkipsIndexedEntities`

#### Scenario: A store indexed before records existed

- **WHEN** graph-index starts over index rows that have no records
- **THEN** each entity is listed once and recorded, and the next start lists none
- **AND** the test that verifies this is `TestRestartWithoutRecordsListsOnce`

#### Scenario: A predicate registered between starts

- **WHEN** a predicate becomes an alias predicate between two starts, and an entity's value is unchanged
- **THEN** after the second start an `alias` request for that entity's value answers the entity
- **AND** the test that verifies this is `TestRestartAppliesNewAliasPredicate`

### Requirement: A cut-off index write leaves no stale row

Every row an entity owns SHALL be named by its record, or the entity SHALL have no record. A record SHALL read
complete only after every row it names is stored and every row it no longer names is deleted. When an entity's
writes were cut off, the next apply SHALL remove every row its record names that the current value does not ask for,
without listing.

#### Scenario: Cut off between A and B

- **WHEN** an entity changes from A to B, its writes are cut off at any point, and graph-index starts again
- **THEN** every read answers B and nothing of A for that entity
- **AND** the test that verifies this is `TestRestartConvergesFromEveryCutPoint`, a generated check over cut points
  against a reference model

#### Scenario: Deleted while graph-index is down

- **WHEN** an indexed entity is deleted while graph-index is stopped, and graph-index starts again
- **THEN** every read answers nothing for it, and its record is gone
- **AND** the test that verifies this is `TestRestartAppliesDeleteWhileDown`

### Requirement: A recreated index bucket is rebuilt in full

When an index bucket was created after the records were written, graph-index SHALL trust no record on that start,
and SHALL list and record every entity it applies.

#### Scenario: INCOMING_INDEX deleted while down

- **WHEN** `INCOMING_INDEX` is deleted while graph-index is stopped, and graph-index starts again
- **THEN** after the first build every `incoming` read answers the same sources as before the deletion
- **AND** the test that verifies this is `TestRestartRebuildsRecreatedIndexBucket`
