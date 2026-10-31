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

After the first tick following bootstrap, a tick SHALL read nothing from `COMMUNITY_INDEX`, SHALL
write only records of groups touched by changes applied since their last successful publication and whose record
differs from the one last published, and SHALL remove the records of groups that became empty. A write or removal
failure SHALL make `Tick` return an error and leave those groups pending for the next tick.

#### Scenario: k changed entities

- **WHEN** k entities change after bootstrap and a tick runs
- **THEN** `COMMUNITY_INDEX` sees no read and only the groups those k entities belong to are written
- **AND** the test that verifies this is `TestTickCost`

#### Scenario: Unchanged graph

- **WHEN** a tick runs with no change since the last
- **THEN** nothing is written and no revision is added to `COMMUNITY_INDEX`
- **AND** the tests that verify this are `TestTickCost` and `TestUnchangedRecordAddsNoRevision`

#### Scenario: A write fails

- **WHEN** a record write fails during a tick
- **THEN** `Tick` returns an error and the next tick writes that record
- **AND** the test that verifies this is `TestFailedWriteRetried`

### Requirement: A group record is bounded

A group record SHALL hold its level, ID, member count, the most frequent types, at most `clustering.MaxKeywords`
keywords, a summary, and at most `clustering.MaxRepEntities` representative IDs, and SHALL NOT hold the member list.

#### Scenario: A large group

- **WHEN** a group has more members than fit in one NATS message
- **THEN** its record is written and holds no member IDs beyond its representatives
- **AND** the test that verifies this is `TestRecordIsBounded`

### Requirement: Keywords and representatives come from the group alone

Keywords SHALL be the group's most frequent words from its members' string values, with lexical tie-breaks, and SHALL
NOT include terms from ID positions or predicate names, so a change outside the group never changes them.
Representatives SHALL be the lexically smallest members that carry text.

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
