# graph-index Delta

## ADDED Requirements

### Requirement: An entity whose rows would not change is not re-indexed

graph-index SHALL keep one record per entity of the rows it wrote, pending or clean, with a format constant and the
generations of `ENTITY_STATES` and the index buckets. When an entity's value decodes, and a clean record with
matching constant and generations holds exactly the planned rows, graph-index SHALL neither list nor write, SHALL
clear the entity from the failed set, and SHALL complete the revision.

#### Scenario: A warm restart over an unchanged store

- **WHEN** graph-index has indexed a store, is stopped, and is started again with nothing changed
- **THEN** the second start lists no index bucket and writes no index row, `DataFlow` counts every entity read, and
  every read answers as before the stop
- **AND** the test that verifies this is `TestWarmRestartSkipsIndexedEntities`

#### Scenario: A producer re-offers every entity unchanged

- **WHEN** every stored entity is written again with equal statements, so each gets a new revision
- **THEN** graph-index lists no index bucket and writes no index row for them, and every read answers as before
- **AND** the test that verifies this is `TestEqualReofferSkipsIndexing`

#### Scenario: A failed entity whose rows are current

- **WHEN** an entity whose clean record matches is in the failed set after a failed read, and the repair pass reads it
- **THEN** it leaves the failed set without a listing, and queries are no longer withheld for it
- **AND** the test that verifies this is `TestRepairSkipClearsFailed`

#### Scenario: The record's encoding

- **WHEN** the record for a fixed plan is built
- **THEN** its bytes equal a stored fixture, which changes only with the format constant
- **AND** the test that verifies this is `TestRecordGoldenFixture`

### Requirement: A changed entity's old rows are found from its record

When a clean record with matching constant and generations differs from the planned rows, graph-index SHALL delete
the rows the record names that the plan drops, put the rows the plan adds or whose value changed, and SHALL NOT list.
With no usable record, graph-index SHALL list as before and then record the entity.

#### Scenario: A live change

- **WHEN** an indexed entity changes from A to B while graph-index runs
- **THEN** every read answers B and nothing of A for it, and no index bucket was listed
- **AND** the test that verifies this is `TestLiveChangeUsesRecord`

#### Scenario: A store indexed before records existed

- **WHEN** graph-index starts over index rows that have no records
- **THEN** each entity is listed once and recorded, and the next start lists none
- **AND** the test that verifies this is `TestRestartWithoutRecordsListsOnce`

#### Scenario: A predicate registered between starts

- **WHEN** a predicate becomes an alias predicate between two starts, and no entity's value changes
- **THEN** after the second start an `alias` request answers each entity carrying it, only those entities were
  written, and no index bucket was listed
- **AND** the test that verifies this is `TestRestartAppliesNewAliasPredicate`

### Requirement: A record names every row its entity can own

Before its first row write an apply SHALL put a pending record of every row the entity may own (a pending record's
whole set carried forward) plus the planned rows, and SHALL write nothing if that put fails; after its last write it
SHALL put a clean record of the planned rows. From a pending record an apply SHALL put every planned row and delete
every other row of its set. A failed record write SHALL count as a failed index write.

#### Scenario: Cut off, then any value under any rules

- **WHEN** an entity's writes are cut off at any point, one or more times in a row (each recovery cut again), its
  value is then A, B or C, the alias predicates are then either of two sets, and graph-index starts again
- **THEN** every read answers that value under those rules and nothing else for the entity
- **AND** the test that verifies this is `TestRestartConvergesFromEveryCutPoint`, a generated check over cut points,
  values and rules against a reference model, each cut state seeded by running a prefix of the apply's own ordered
  operations

#### Scenario: The record agrees with a listing

- **WHEN** a generated history of changes, cuts, re-offers, deletes and a second writer between an apply's record
  read and its pending put is applied, ending with a completed apply
- **THEN** after every step each pending record names at least the rows a listing of the index buckets finds for its
  entity, and at the end each clean record names exactly those rows
- **AND** the test that verifies this is `TestRecordMatchesListingOracle`

#### Scenario: A damaged value

- **WHEN** an indexed entity is overwritten with a value that does not decode, and graph-index starts again
- **THEN** its rows are gone, it is reported damaged on both starts, it has a clean empty record, and the second start
  lists nothing for it
- **AND** the test that verifies this is `TestDamagedRemovalRecordsEmpty`

#### Scenario: Deleted while graph-index is down

- **WHEN** an indexed entity is deleted while graph-index is stopped, and graph-index starts again twice
- **THEN** every read answers nothing for it after the first start, and the second start lists nothing for it
- **AND** the test that verifies this is `TestRestartAppliesDeleteWhileDown`

#### Scenario: The record bucket deleted while graph-index runs

- **WHEN** `GRAPH_INDEX_RECORDS` is deleted while graph-index runs, and an indexed entity then changes
- **THEN** none of its rows are written, a request answers `index_not_ready`, and after a restart the change answers
- **AND** the test that verifies this is `TestRecordBucketDeletedWhileRunningWithholds`

### Requirement: A record write is conditional on the record it read

Each record write SHALL be conditional on a revision: the pending put on the revision the apply read (a create when
there was none), the clean put on the pending record's revision, a record delete on the revision read. A mismatch
SHALL count as a failed index write, with nothing more written, and each retry SHALL read the record again.

#### Scenario: A second graph-index writes the record first

- **WHEN** another writer puts an entity's record between an apply's record read and its pending put
- **THEN** the apply writes no row, the entity is failed, and the repair pass converges it from the stored record so
  that every row the entity can own is named by its record
- **AND** the test that verifies this is `TestSecondWriterFailsRecordWrite`

### Requirement: An entity too large for one record is indexed by listing

When an entity's encoded record would exceed the payload limit the server reports, less a stated allowance,
graph-index SHALL delete its record before any row write, writing nothing if that delete fails, and SHALL apply the
entity by listing with no record. A record put still refused SHALL count as a failed index write.

#### Scenario: A source with many relationships

- **WHEN** a recorded small entity grows past the payload limit, then changes, then shrinks below it and is applied
  once more
- **THEN** every read answers its current relationships at each step, a listing finds no stale row, it has no record
  while large, and it is recorded again at the end
- **AND** the test that verifies this is `TestOversizeRecordFallsBackToListing`

### Requirement: A recreated index bucket is rebuilt

A record written under another generation of `ENTITY_STATES` or of an index bucket SHALL NOT be used.

#### Scenario: INCOMING_INDEX deleted while down

- **WHEN** `INCOMING_INDEX` is deleted while graph-index is stopped, and graph-index starts, is stopped during its
  rebuild, and starts again
- **THEN** after the last first build every `incoming` read answers the same sources as before the deletion
- **AND** the test that verifies this is `TestRestartRebuildsRecreatedIndexBucket`

### Requirement: What a restart does not repair is declared

graph-index's documentation SHALL declare that, under a clean record, rows edited by hand, a bucket emptied in place
or restored with its creation time, rows written by a graph-index that keeps no records, and writes a host power loss
discarded are not repaired, and that deleting the record bucket while graph-index is stopped rebuilds every entity.

#### Scenario: The reset

- **WHEN** a row is deleted by hand while graph-index is stopped, the record bucket is deleted, and graph-index starts
- **THEN** after the first build the deleted row answers again
- **AND** the test that verifies this is `TestDeletingRecordsRebuildsEveryEntity`
