# graph-clustering

## ADDED Requirements

### Requirement: Every entity change reaches the maintainer in delivery order

graph-clustering SHALL apply each `ENTITY_STATES` delivery to the `graph/clustering` maintainer with the delivered
value and its revision, in delivery order, and SHALL never apply a value read later in place of the delivered one. A
value that does not decode SHALL be applied as a refused entity.

#### Scenario: Upsert, edit, delete

- **WHEN** an entity is written, edited, and deleted, and a tick follows each
- **THEN** its groups' records follow each write, with `entity_revision` at that write's revision, and a group whose
  last member is deleted is removed (`TestRecordsFollowChanges`)

#### Scenario: A value that does not decode

- **WHEN** a live write to `ENTITY_STATES` holds bytes that are not an entity state
- **THEN** the entity contributes nothing, the tick still publishes the other groups, and `Health` and the envelope's
  failure reasons name it (`TestRefusedValueReportedNotWithheld`)

### Requirement: Publication waits for the replay and stops on a broken feed

graph-clustering SHALL publish nothing to `COMMUNITY_INDEX` until the follower's initial build is complete, and SHALL
publish nothing while the follower reports a lost feed or a latched reset, leaving the records as last published.

#### Scenario: During the replay

- **WHEN** the component starts on a populated `ENTITY_STATES` and the replay is not yet applied
- **THEN** `COMMUNITY_INDEX` is unchanged (`TestNothingPublishedBeforeReplay`)

#### Scenario: A revision out of order

- **WHEN** the feed delivers a revision at or below one already observed
- **THEN** no later tick writes or removes a record, and `Health` names the lost feed (`TestLostFeedStopsPublication`)

### Requirement: Removed configuration is refused

graph-clustering SHALL refuse at construction, naming the key and what replaces it, any key it does not define,
including `batch_size`, `enable_llm`, `enhancement_workers`, `min_community_size`, `max_iterations`,
`entity_id_edges`, `semantic_edges` and `allow_ungated_reads`, and SHALL refuse declared output ports.

#### Scenario: A pin configuration

- **WHEN** a configuration carries `entity_id_edges`
- **THEN** construction fails naming `entity_id_edges` (`TestConfigRefusesRemovedKeys`)

### Requirement: Tick failures are not health

A failed tick SHALL be retried on the next tick and SHALL NOT lower `Healthy`; only a lost feed, a latched reset, or a
refused entity are stuck states in `Health`.

#### Scenario: A write fails once

- **WHEN** one tick's write to `COMMUNITY_INDEX` fails and the next succeeds
- **THEN** `Healthy` stays true and the record is written by the second tick (`TestHealthIgnoresTickFailures`)

### Requirement: Stop leaves nothing

After `Stop` returns nil, no goroutine of the component, the follower or the ticker SHALL remain.

#### Scenario: Stop after a tick

- **WHEN** the component has published and `Stop` is called
- **THEN** it returns nil and the lifecycle suite finds nothing running (`TestStopLeavesNothing`)
