# graph-index Delta

## ADDED Requirements

### Requirement: An entity whose rows would not change is not re-indexed

After every row write for an entity succeeds, graph-index SHALL store a mark: a digest of the rows written, a format
constant, and the generations of `ENTITY_STATES` and the index buckets. When an entity's value decodes, its planned
rows have the mark's digest, and the constant and every generation match, graph-index SHALL neither list nor write,
SHALL clear the entity from the failed set, and SHALL complete the revision.

#### Scenario: A warm restart over an unchanged store

- **WHEN** graph-index has indexed a store, is stopped, and is started again with nothing changed
- **THEN** the second start lists no index bucket and writes no index row, `DataFlow` counts every entity read, and
  every read answers as before the stop
- **AND** the test that verifies this is `TestWarmRestartSkipsIndexedEntities`

#### Scenario: A producer re-offers every entity unchanged

- **WHEN** every stored entity is written again with equal statements, so each gets a new revision
- **THEN** graph-index lists no index bucket and writes no index row for them, and every read answers as before
- **AND** the test that verifies this is `TestEqualReofferSkipsIndexing`

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

#### Scenario: The mark's encoding

- **WHEN** the mark for a fixed plan is built
- **THEN** its bytes equal a stored fixture, which changes only with the format constant
- **AND** the test that verifies this is `TestMarkGoldenFixture`

### Requirement: A mark exists only while the rows match it

An apply that writes SHALL delete the entity's mark before its first row write, and SHALL write nothing if that
delete fails. A failed mark write SHALL count as a failed index write. A mark that is missing, does not decode, or
cannot be read SHALL make graph-index apply the entity as before. A damaged value's removal SHALL leave no mark.

#### Scenario: Cut off, then any value under any rules

- **WHEN** an entity's writes are cut off at any point, its value is then A, B or C, the alias predicates are then
  either of two sets, and graph-index starts again
- **THEN** every read answers that value under those rules and nothing else for the entity
- **AND** the test that verifies this is `TestRestartConvergesFromEveryCutPoint`, a generated check over cut points,
  values and rules against a reference model, seeded with one golden mark fixture beside the package's builder

#### Scenario: Deleted while graph-index is down

- **WHEN** an indexed entity is deleted while graph-index is stopped, and graph-index starts again twice
- **THEN** every read answers nothing for it after the first start, and the second start lists nothing for it
- **AND** the test that verifies this is `TestRestartAppliesDeleteWhileDown`

#### Scenario: The mark bucket deleted while graph-index runs

- **WHEN** `GRAPH_INDEX_MARKS` is deleted while graph-index runs, and an indexed entity then changes
- **THEN** none of its rows are written, a request answers `index_not_ready`, and after a restart the change answers
- **AND** the test that verifies this is `TestMarkBucketDeletedWhileRunningWithholds`

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
discarded are not repaired, and that deleting the mark bucket while graph-index is stopped rebuilds every entity.

#### Scenario: The reset

- **WHEN** a row is deleted by hand while graph-index is stopped, the mark bucket is deleted, and graph-index starts
- **THEN** after the first build the deleted row answers again
- **AND** the test that verifies this is `TestDeletingMarksRebuildsEveryEntity`
