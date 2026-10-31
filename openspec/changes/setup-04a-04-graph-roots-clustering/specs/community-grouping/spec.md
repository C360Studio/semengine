# community-grouping

## ADDED Requirements

### Requirement: A community is an entity-ID prefix

`clustering.Maintainer` SHALL place each applied entity in exactly one group per level: at level 0 the group of its
four-position prefix `org.platform.system.domain`, at level 1 its source prefix `org.platform.system`, at level 2 its
deployment prefix `org.platform`. A group's ID SHALL be that prefix.

#### Scenario: Three levels

- **WHEN** entities from two sources of one deployment are applied and a tick runs
- **THEN** level 0 holds one record per four-position prefix, level 1 one per source, level 2 one
- **AND** the test that verifies this is `TestMaintainerMatchesModel`

### Requirement: Applying a change depends only on the latest revision

The maintainer SHALL keep each present entity's last-applied revision and contribution; an `Upsert` or `Delete` at
or below that revision SHALL change nothing; a newer `Upsert` SHALL replace the contribution, and a `Delete` SHALL
remove it. The published records after any history SHALL equal those computed from the final entity values alone.
The caller SHALL pass, with each `Upsert`, the revision of the exact value decoded, in increasing order per key.

#### Scenario: Two histories, one final state

- **WHEN** two histories interleave entities in different orders, keep each entity's own revision order, and end with
  the same entity values; obsolete deliveries are generated only for entities still present
- **THEN** the published records are byte-identical
- **AND** the tests that verify this are `TestHistoryOrderIrrelevant` and `TestMaintainerMatchesModel`

#### Scenario: An obsolete delivery

- **WHEN** an entity applied at revision 9 receives an upsert or delete at revision 7
- **THEN** its contribution is unchanged
- **AND** the test that verifies this is `TestObsoleteDeliveryIgnored`

### Requirement: Applying never waits on publication

`Upsert` and `Delete` SHALL return without waiting for a `Tick`'s storage write to finish.

#### Scenario: A blocked write

- **WHEN** a `Tick` is blocked inside a storage write and an `Upsert` is called
- **THEN** the `Upsert` returns while the write is still blocked
- **AND** the test that verifies this is `TestUpsertDuringBlockedWrite`

### Requirement: Refused entities are reported

`Upsert` SHALL return an invalid error naming the entity for a nil state, an ID the canonical parser refuses, or a
state whose ID differs from the given one, and the entity SHALL contribute nothing. While any entity stays refused, a
tick that otherwise publishes everything SHALL return a `*clustering.IncompleteError` naming the refused IDs.

#### Scenario: An unparseable ID

- **WHEN** an entity with an unparseable ID is upserted and a tick runs
- **THEN** `Upsert` returns an error, and `Tick` publishes the other groups and returns an `*IncompleteError` naming it
- **AND** the test that verifies this is `TestRefusedEntityMakesTickIncomplete`

### Requirement: Nothing is published before bootstrap completes

`Tick` SHALL return an error and write nothing when its context is nil, and SHALL return
`clustering.ErrBootstrapIncomplete` and write nothing until `BootstrapDone` has been called.

#### Scenario: Tick during replay

- **WHEN** `Tick` runs before `BootstrapDone`
- **THEN** it returns `ErrBootstrapIncomplete` and `COMMUNITY_INDEX` is unchanged
- **AND** the test that verifies this is `TestTickBeforeBootstrap`

### Requirement: A tick publishes only what changed

After the first tick following bootstrap, a tick SHALL read nothing from `COMMUNITY_INDEX`, SHALL write only records of
groups touched since their last successful publication whose record differs from the one last published, and SHALL
remove the records of groups that became empty.

#### Scenario: k changed entities

- **WHEN** k entities change after the first tick and a tick runs
- **THEN** `COMMUNITY_INDEX` sees no read and only the groups those k entities belong to are written
- **AND** the test that verifies this is `TestTickCost`

#### Scenario: Unchanged graph

- **WHEN** a tick runs with no change since the last
- **THEN** nothing is written and no revision is added to `COMMUNITY_INDEX`
- **AND** the tests that verify this are `TestTickCost` and `TestUnchangedRecordAddsNoRevision`

### Requirement: Removal follows publication

A tick SHALL remove a record, including on the first tick after bootstrap the keys of groups that do not exist, only
after every changed record has been written. A write failure, a removal failure, or the context ending SHALL make
`Tick` return an error and leave the unwritten records and the cleanup pending for the next tick, so the old records
stay readable.

#### Scenario: The first publication fails

- **WHEN** `COMMUNITY_INDEX` holds a stale key from an earlier run and the first tick after bootstrap fails to write a
  record
- **THEN** `Tick` returns an error and the stale record is still there; the next successful tick writes the record and
  then removes the stale key
- **AND** the tests that verify this are `TestFirstTickFailedPublicationKeepsOldRecords` and `TestFailedWriteRetried`

#### Scenario: Cancelled during a tick

- **WHEN** the context ends while a tick writes or removes records
- **THEN** `Tick` returns the context's error and the next tick completes the pending work
- **AND** the test that verifies this is `TestTickReportsCancel`

### Requirement: A group record is bounded

A group record SHALL hold its level, ID, member count, at most `MaxKeywords` types, at most `MaxKeywords` keywords, a
summary, and at most `MaxRepEntities` representative IDs, SHALL NOT hold the member list, and its encoding SHALL be at
most `clustering.MaxRecordBytes` whatever the members' values. A word SHALL be a run of 3 to `MaxWordBytes` ASCII
`a-z` and `0-9` characters; a longer run SHALL NOT be a word and SHALL NOT be truncated into one.

#### Scenario: A large group

- **WHEN** a group has more members than fit in one NATS message, `MaxKeywords` types whose type positions are
  246 bytes, `MaxRepEntities` 256-byte representative IDs and `MaxKeywords` 32-byte keywords
- **THEN** its record holds no member IDs beyond its representatives and encodes to at most `MaxRecordBytes`
- **AND** the test that verifies this is `TestRecordIsBounded`, which fails if an eleventh type is kept

#### Scenario: Long literals

- **WHEN** members' string values hold ten 120 KB alphanumeric literals
- **THEN** none is a keyword and the encoded record is at most `MaxRecordBytes`
- **AND** the test that verifies this is `TestLongLiteralIsNotAWord`

### Requirement: Keywords and representatives come from the group alone

Keywords SHALL be the group's most frequent words over its members' contributions, with lexical tie-breaks, where
each member contributes only its `MaxEntityTerms` most frequent words (an approximation: a word outside every member's
top `MaxEntityTerms` is never a keyword). Terms from ID positions or predicate names SHALL NOT count, so a change
outside the group never changes its keywords. Representatives SHALL be the lexically smallest text-bearing members.

#### Scenario: Keywords and representatives

- **WHEN** every member shares its predicates and ID positions, and the smallest text-bearing member is deleted
- **THEN** no keyword comes from a predicate or ID position, and the next smallest text-bearing member becomes a
  representative
- **AND** the tests that verify this are `TestKeywordsIgnoreIDAndPredicates` and `TestRepresentativesWithoutScan`

### Requirement: A product can maintain the index

A product SHALL be able to build a `Maintainer` over a `NATSCommunityStorage`, feed it entity changes, and publish
`COMMUNITY_INDEX` with no other SemEngine component.

#### Scenario: From outside the package

- **WHEN** a test in package `clustering_test` applies entity changes, calls `BootstrapDone` and `Tick`
- **THEN** `COMMUNITY_INDEX` holds one record per group at each level
- **AND** the test that verifies this is `TestMaintainerAsAProduct`

### Requirement: Community storage fails closed

`NATSCommunityStorage.GetAllCommunities` SHALL return an error naming the key when an entry cannot be read or
decoded, SHALL return the context's error when its context ends, and SHALL give no records and no error for an empty
bucket.

#### Scenario: A corrupt entry

- **WHEN** `COMMUNITY_INDEX` holds a key whose value is not a group record
- **THEN** `GetAllCommunities` returns an error naming that key
- **AND** the test that verifies this is `TestGetAllCommunitiesFailsOnCorruptEntry`

#### Scenario: Cancelled

- **WHEN** the context ends during `GetAllCommunities`
- **THEN** it returns the context's error
- **AND** the test that verifies this is `TestGetAllReportsCancel`
