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
  `failed_reasons` name `refused_entity` (`TestRefusedValueReportedNotWithheld`)

### Requirement: The readiness key counts what the maintainer holds

graph-clustering's `graph-clustering` key SHALL report a revision indexed once the maintainer has applied it, before
it is published; a reader that needs a group's published state SHALL read the record's `entity_revision`.

#### Scenario: Applied before the tick

- **WHEN** an entity is written and no tick has run since
- **THEN** the key's `indexed_revision` reaches the write's revision while the group's record still carries the
  earlier `entity_revision` (`TestRecordsFollowChanges`)

### Requirement: Publication waits for the replay and stops on a broken feed

graph-clustering SHALL publish nothing to `COMMUNITY_INDEX` until the follower's initial build is complete, and SHALL
start no tick once the follower reports a lost feed or a latched reset. A tick already running when the loss is
detected MAY finish.

#### Scenario: During the replay

- **WHEN** the component starts on a populated `ENTITY_STATES` and the replay is not yet applied
- **THEN** `COMMUNITY_INDEX` is unchanged (`TestNothingPublishedBeforeReplay`)

#### Scenario: A revision out of order

- **WHEN** the feed delivers a revision at or below one already observed
- **THEN** no tick that starts afterwards writes or removes a record, and `Health` names the lost feed
  (`TestLostFeedStopsPublication`)

### Requirement: A failing publication is a stuck state

While the last tick returned an error other than `*clustering.IncompleteError`, graph-clustering SHALL report
`Healthy` false naming the failure, count it in `Failed` under `publication_failed`, and count each failed tick on
`graph_clustering_publication_failures_total`; the next successful tick SHALL clear the state. Refused entities SHALL
count under `refused_entity` and SHALL NOT stop publication.

#### Scenario: Every write fails

- **WHEN** every tick's write to `COMMUNITY_INDEX` fails, and then one succeeds
- **THEN** while they fail `Healthy` is false, the envelope reads `degraded` with `publication_failed` 1, and the
  counter rises once per tick; after the success `Healthy` is true and the record is written
  (`TestPersistentPublicationFailureIsStuck`)

### Requirement: Removed configuration is refused

graph-clustering SHALL refuse at construction, naming the key and what replaces it, any key it does not define,
including `batch_size`, `enable_llm`, `enhancement_workers`, `min_community_size`, `max_iterations`,
`entity_id_edges`, `semantic_edges` and `allow_ungated_reads`, and SHALL refuse declared output ports. Each kept key
SHALL change behaviour when set.

#### Scenario: A pin configuration

- **WHEN** a configuration carries `entity_id_edges`
- **THEN** construction fails naming `entity_id_edges` (`TestConfigRefusesRemovedKeys`)

#### Scenario: Kept keys are read

- **WHEN** `detection_interval`, `startup_attempts` or `startup_interval_ms` is set
- **THEN** the tick interval or the startup wait follows it (`TestDetectionIntervalIsRead`, `TestConfigStartupWait`)

### Requirement: Stop leaves nothing

After `Stop` returns nil, no goroutine of the component, the follower or the ticker SHALL remain.

#### Scenario: Stop after a tick

- **WHEN** the component has published and `Stop` is called
- **THEN** it returns nil and the lifecycle suite finds nothing running (`TestStopLeavesNothing`)
