# graph-index Delta

## ADDED Requirements

### Requirement: An entity indexed at its current revision is not re-indexed

After every row write for an entity succeeds, graph-index SHALL store a mark: the revision, a digest of the rows
written, and the generations of `ENTITY_STATES` and the index buckets. When an entity read has the mark's revision,
its planned rows the mark's digest, and every generation matches this start's, graph-index SHALL neither list nor
write, SHALL clear the entity from the failed set, and SHALL complete the revision. Otherwise it SHALL index as before.

#### Scenario: A warm restart over an unchanged store

- **WHEN** graph-index has indexed a store, is stopped, and is started again with nothing changed
- **THEN** the second start lists no index bucket and writes no index row, `DataFlow` counts every entity read, and
  every read answers as before the stop
- **AND** the test that verifies this is `TestWarmRestartSkipsIndexedEntities`

#### Scenario: A store indexed before marks existed

- **WHEN** graph-index starts over index rows that have no marks
- **THEN** each entity is listed once and marked, and the next start lists none
- **AND** the test that verifies this is `TestRestartWithoutMarksListsOnce`

#### Scenario: A predicate registered between starts

- **WHEN** a predicate becomes an alias predicate between two starts, and no entity's value changes
- **THEN** after the second start an `alias` request answers each entity carrying it, and only those entities were
  listed
- **AND** the test that verifies this is `TestRestartAppliesNewAliasPredicate`

#### Scenario: A failed entity whose rows are current

- **WHEN** an entity whose mark matches is in the failed set after a failed read, and the repair pass reads it
- **THEN** it leaves the failed set without a listing, and queries are no longer withheld for it
- **AND** the test that verifies this is `TestRepairSkipClearsFailed`

### Requirement: A cut-off index write is never skipped

A mark SHALL name only a revision whose every index write succeeded. A mark that is missing, does not decode, or
cannot be read SHALL make graph-index index the entity as before. A failed mark write SHALL NOT mark the entity
failed.

#### Scenario: Cut off, then any value

- **WHEN** an entity changes from A to B, its writes are cut off at any point, its value is then A, B or C, and
  graph-index starts again
- **THEN** every read answers that value and nothing of the others for the entity
- **AND** the test that verifies this is `TestRestartConvergesFromEveryCutPoint`, a generated check over cut points
  and values in {A, B, C} against a reference model

#### Scenario: Deleted while graph-index is down

- **WHEN** an indexed entity is deleted while graph-index is stopped, and graph-index starts again twice
- **THEN** every read answers nothing for it after the first start, and the second start lists nothing for it
- **AND** the test that verifies this is `TestRestartAppliesDeleteWhileDown`

### Requirement: A recreated index bucket is rebuilt

A mark written under another generation of `ENTITY_STATES` or of an index bucket SHALL NOT match.

#### Scenario: INCOMING_INDEX deleted while down

- **WHEN** `INCOMING_INDEX` is deleted while graph-index is stopped, and graph-index starts, is stopped during its
  rebuild, and starts again
- **THEN** after the last first build every `incoming` read answers the same sources as before the deletion
- **AND** the test that verifies this is `TestRestartRebuildsRecreatedIndexBucket`

### Requirement: What a restart does not repair is declared

graph-index's documentation SHALL declare that, under a matching mark, rows edited by hand, a bucket emptied in place
or restored with its creation time, rows written by a graph-index that keeps no marks, and writes a host power loss
discarded are not repaired, and that deleting the mark bucket rebuilds every entity on the next start.

#### Scenario: The reset

- **WHEN** a row is deleted by hand while graph-index is stopped, the mark bucket is deleted, and graph-index starts
- **THEN** after the first build the deleted row answers again
- **AND** the test that verifies this is `TestDeletingMarksRebuildsEveryEntity`
