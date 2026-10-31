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

The maintainer SHALL keep each entity's last-applied revision and contribution. An `Upsert` or `Delete` at or below
that revision SHALL change nothing. A newer `Upsert` SHALL replace the entity's contribution, and a `Delete` SHALL
remove it and keep its revision so that an older `Upsert` arriving later changes nothing. The published records after
any history SHALL equal those computed from the final entity values alone.

#### Scenario: Two histories, one final state

- **WHEN** two shuffled histories of upserts, deletes and obsolete deliveries end with the same entity values
- **THEN** the published records are byte-identical
- **AND** the tests that verify this are `TestHistoryOrderIrrelevant` and `TestMaintainerMatchesModel`

#### Scenario: A late older upsert after a delete

- **WHEN** an entity is deleted at revision 9 and an upsert at revision 7 arrives afterwards
- **THEN** the entity stays absent from every group
- **AND** the test that verifies this is `TestTombstoneBlocksOlderUpsert`

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

After the first tick following bootstrap, a tick SHALL read no value from `COMMUNITY_INDEX` or `ENTITY_STATES`, SHALL
write only records of groups touched by changes applied since their last successful publication and whose record
differs from the one last published, and SHALL remove the records of groups that became empty. A write or removal
failure SHALL make `Tick` return an error and leave those groups pending for the next tick.

#### Scenario: k changed entities

- **WHEN** k entities change after bootstrap and a tick runs
- **THEN** the storage sees no read and writes only the groups those k entities belong to
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
Keywords SHALL be the group's own most frequent terms with lexical tie-breaks, so a change outside the group never
changes them.

#### Scenario: A large group

- **WHEN** a group has more members than fit in one NATS message
- **THEN** its record is written and holds no member IDs beyond its representatives
- **AND** the test that verifies this is `TestRecordIsBounded`

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
