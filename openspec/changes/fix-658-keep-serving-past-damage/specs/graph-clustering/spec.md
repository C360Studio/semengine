# graph-clustering Delta

## MODIFIED Requirements

### Requirement: Every entity change reaches the maintainer in delivery order

graph-clustering SHALL apply each `ENTITY_STATES` delivery to the `graph/clustering` maintainer with the delivered
value and its revision, in delivery order, and SHALL never apply a value read later in place of the delivered one. A
value that does not decode SHALL be reported to the follower as a damaged entity and applied as a delete of the
entity's contribution at its revision.

#### Scenario: Upsert, edit, delete

- **WHEN** an entity is written, edited, and deleted, and a tick follows each
- **THEN** its groups' records follow each write, with `entity_revision` at that write's revision, and a group whose
  last member is deleted is removed (`TestRecordsFollowChanges`)

#### Scenario: A value that does not decode

- **WHEN** a live write to `ENTITY_STATES` holds bytes that are not an entity state
- **THEN** the entity contributes nothing, the tick still publishes the other groups, `Health` names the entity, and
  the envelope's `failed_reasons` name `damaged_entity` 1 (`TestRefusedValueReportedNotWithheld`)

#### Scenario: A valid member overwritten with damage

- **WHEN** an entity in a group is overwritten with bytes that do not decode, and a tick follows
- **THEN** the group's record no longer counts it, the other groups are unchanged, and `Health` names the entity
  (`TestDamagedOverwriteLeavesGroup`)

#### Scenario: Repair

- **WHEN** the damaged entity is then written with a valid value and a tick follows
- **THEN** its groups count it again, `Health` no longer names it, and no restart was needed
  (`TestDamagedOverwriteLeavesGroup`)

### Requirement: Publication waits for the replay and stops on a broken feed

graph-clustering SHALL publish nothing to `COMMUNITY_INDEX` until the follower's initial build is complete, and SHALL
start no tick once the follower reports a lost feed. A tick already running when the loss is detected MAY finish. A
damaged entity, stored at start or written live, SHALL NOT stop publication.

#### Scenario: During the replay

- **WHEN** the component starts on a populated `ENTITY_STATES` and the replay is not yet applied
- **THEN** `COMMUNITY_INDEX` is unchanged (`TestNothingPublishedBeforeReplay`)

#### Scenario: A lost feed

- **WHEN** the follower reports a lost feed (a revision at or below one already observed, or the bucket deleted)
- **THEN** no tick that starts afterwards writes or removes a record, and `Health` names the lost feed
  (`TestLostFeedStopsPublication` deletes the bucket; `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery` covers the
  out-of-order trigger)

#### Scenario: A damaged value in the replay

- **WHEN** the component starts with one valid and one damaged entity stored
- **THEN** the first tick publishes the valid entity's groups and `Health` names the damaged one
  (`TestDamagedReplayValuePublishesTheRest`)
