# community-grouping

## ADDED Requirements

### Requirement: A community is an entity-ID prefix

`clustering.Partitioner.Refresh(ctx)` SHALL read every stored entity through graph-ingest's `prefix` verb and place
each in exactly one group per level: at level 0 the group of its four-position prefix, at level 1 its three-position
source prefix, at level 2 its two-position deployment prefix. A group's ID SHALL be that prefix. A group with one
member SHALL get a record.

#### Scenario: Three levels

- **WHEN** `Refresh` runs over entities from two sources of one deployment
- **THEN** level 0 holds one group per four-position prefix, level 1 one group per source, level 2 one group
- **AND** the tests that verify this are `TestGroupsArePrefixes` and `TestPartitionMatchesModel`

#### Scenario: A singleton

- **WHEN** one entity is alone under its four-position prefix
- **THEN** it has a level-0 group of one
- **AND** the test that verifies this is `TestSingletonRecorded`

### Requirement: An incomplete refresh is an error

When an entity ID the canonical parser refuses is left out of every group, or removing the records of vanished
groups fails, `Refresh` SHALL return the partition together with a non-nil `*clustering.IncompleteError` naming the
skipped IDs and carrying the removal error. A nil error SHALL mean the refresh was complete.

#### Scenario: An unparseable ID

- **WHEN** one stored entity's ID does not parse
- **THEN** that entity is in no group, the other groups are written, and `errors.As` on the returned error finds an
  `*IncompleteError` naming the ID
- **AND** the test that verifies this is `TestUnparseableIDIsAnError`

#### Scenario: Removal fails

- **WHEN** every write succeeds and removing a vanished group's record fails
- **THEN** `Refresh` returns the partition and an `*IncompleteError` that unwraps to the removal error
- **AND** the test that verifies this is `TestPruneFailureIsAnError`

### Requirement: Partitioner refuses missing inputs

`clustering.NewPartitioner` SHALL return an error when its requester, storage or summarizer is nil, and `Refresh`
SHALL return an error and read and write nothing when its context is nil.

#### Scenario: Nil context

- **WHEN** `Refresh` is called with a nil context
- **THEN** it returns an error and reads and writes nothing
- **AND** the test that verifies this is `TestRefreshRefusesNilContext`

### Requirement: A group record is bounded

A group record SHALL hold its level, ID, member count, `clustering.MembershipHash` of its members, keywords, a
statistical summary and at most `clustering.MaxRepEntities` representative entity IDs, and SHALL NOT hold the member
list, so its size does not grow with the group.

#### Scenario: A large group

- **WHEN** a group has more members than fit in one NATS message
- **THEN** its record is written and holds no member IDs beyond its representatives
- **AND** the test that verifies this is `TestRecordIsBounded`

### Requirement: The community index is rebuilt on change, never emptied first

A refresh SHALL NOT write a record equal to the stored one, SHALL write every changed record before removing
anything, and SHALL then remove the records of groups that no longer exist. Any read or write failure SHALL make
`Refresh` return an error and no partition, and remove nothing.

#### Scenario: Unchanged graph

- **WHEN** two refreshes run over the same entities
- **THEN** the second adds no revision to `COMMUNITY_INDEX`
- **AND** the tests that verify this are `TestUnchangedRecordNotRewritten` and `TestUnchangedRefreshAddsNoRevision`

#### Scenario: A write fails

- **WHEN** one changed record cannot be written
- **THEN** `Refresh` returns an error and no record is removed
- **AND** the test that verifies this is `TestWriteFailureRemovesNothing`

### Requirement: Each entity state is read once per refresh

A refresh SHALL read each stored entity's state at most once, whatever the number of levels and pages.

#### Scenario: Several pages, three levels

- **WHEN** `Refresh` runs over N entities served across several prefix pages
- **THEN** each entity is delivered and used exactly once
- **AND** the test that verifies this is `TestEachStateReadOnce`

### Requirement: A product can run a refresh

A product composing graph-ingest SHALL be able to construct a `Partitioner` from a NATS client, a
`NATSCommunityStorage` and a `StatisticalSummarizer` and refresh `COMMUNITY_INDEX` with no other SemEngine component.

#### Scenario: Refresh from outside the package

- **WHEN** a test in package `clustering_test` stores entities through graph-ingest and calls `Refresh`
- **THEN** `COMMUNITY_INDEX` holds one record per group at each level
- **AND** the test that verifies this is `TestRefreshAsAProduct`

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
