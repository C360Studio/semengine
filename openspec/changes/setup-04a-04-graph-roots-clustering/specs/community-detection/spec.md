# community-detection

## ADDED Requirements

### Requirement: A community is an entity-ID prefix

`clustering.Partitioner.Refresh(ctx, entityIDs)` SHALL place each listed entity in exactly one group per level: at
level 0 the group of its four-position prefix, at level 1 its three-position source prefix, at level 2 its
two-position deployment prefix. A group's ID SHALL be that prefix. A group with one member SHALL get a record. An ID
the canonical parser refuses SHALL be left out of every group and named in the result.

#### Scenario: Three levels

- **WHEN** `Refresh` runs over entities from two sources of one deployment
- **THEN** level 0 holds one group per four-position prefix, level 1 one group per source, level 2 one group
- **AND** the tests that verify this are `TestGroupsArePrefixes` and `TestPartitionMatchesModel`

#### Scenario: A singleton and an unparseable ID

- **WHEN** one entity is alone under its prefix and another listed ID does not parse
- **THEN** the first has a group of one, and the second is in no group and is named in the result
- **AND** the tests that verify this are `TestSingletonRecorded` and `TestUnparseableIDNamed`

### Requirement: Refresh refuses missing inputs

`Refresh` SHALL return a typed error and touch nothing when its context is nil, and `NewPartitioner` SHALL return a
typed error when its storage, summarizer or entity reader is nil.

#### Scenario: Nil context

- **WHEN** `Refresh` is called with a nil context
- **THEN** it returns an error and reads and writes nothing
- **AND** the test that verifies this is `TestRefreshRefusesNilContext`

### Requirement: A group record is bounded

A group record SHALL hold its level, ID, member count, `clustering.MembershipHash` of its members, keywords, a
statistical summary and at most `MaxRepEntities` representative entity IDs, and SHALL NOT hold the member list, so
its size does not grow with the group.

#### Scenario: A large group

- **WHEN** a group has more members than fit in one NATS message
- **THEN** its record is written and holds no member IDs beyond its representatives
- **AND** the test that verifies this is `TestRecordIsBounded`

### Requirement: The community index is rebuilt on change, never emptied first

A refresh SHALL NOT write a record equal to the stored one, SHALL write every changed record before removing
anything, and SHALL then remove the records of groups that no longer exist. Any write failure SHALL make `Refresh`
return an error and remove nothing. A removal failure SHALL be returned to the caller with the partition.

#### Scenario: Unchanged graph

- **WHEN** two refreshes run over the same entity IDs and states
- **THEN** the second adds no revision to `COMMUNITY_INDEX`
- **AND** the tests that verify this are `TestUnchangedRecordNotRewritten` and `TestUnchangedRefreshAddsNoRevision`

#### Scenario: A write fails

- **WHEN** one changed record cannot be written
- **THEN** `Refresh` returns an error and no record is removed
- **AND** the test that verifies this is `TestWriteFailureRemovesNothing`

#### Scenario: Removal fails

- **WHEN** every write succeeds and the removal fails
- **THEN** `Refresh` returns the partition and the removal error in its result
- **AND** the test that verifies this is `TestPruneFailureReturned`

### Requirement: Each entity state is read once per refresh

A refresh SHALL read each listed entity's state at most once, whatever the number of levels.

#### Scenario: Three levels, one read

- **WHEN** `Refresh` runs over N entities
- **THEN** the entity reader is asked for each of them exactly once
- **AND** the test that verifies this is `TestEachStateReadOnce`

### Requirement: Community storage fails closed

`NATSCommunityStorage.GetAllCommunities` SHALL return an error naming the key when an entry cannot be read or
decoded. `GetAllCommunities` and `Prune` SHALL return the context's error when their context ends. An empty bucket
SHALL give no records and no error.

#### Scenario: A corrupt entry

- **WHEN** `COMMUNITY_INDEX` holds a key whose value is not a group record
- **THEN** `GetAllCommunities` returns an error naming that key
- **AND** the test that verifies this is `TestGetAllCommunitiesFailsOnCorruptEntry`

#### Scenario: Cancelled

- **WHEN** the context ends during `Prune` or during `GetAllCommunities`
- **THEN** each returns the context's error
- **AND** the tests that verify this are `TestPruneReportsCancel` and `TestGetAllReportsCancel`

### Requirement: A summary is a function of its members and corpus

`StatisticalSummarizer` SHALL return the same keywords, representative entities and summary text, in the same order,
for the same member entities and the same corpus statistics.

#### Scenario: Tied scores

- **WHEN** a group's entities give two terms the same score and its summary is computed 200 times with the same
  corpus statistics
- **THEN** every result is identical
- **AND** the test that verifies this is `TestSummaryDeterministic`
