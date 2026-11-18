# graph-embedding Delta

## ADDED Requirements

### Requirement: A damaged entity is left out and counted

When hop 1 reads a stored value `graph.UnmarshalEntityState` refuses, graph-embedding SHALL report it to the follower,
remove the entity's `EMBEDDING_INDEX` record, and complete the revision. Both verbs SHALL carry `damaged_entities`:
`search` the damaged entities within its `scope`, all when it has none; `similar` all of them. A valid value or a
delete for the key SHALL be applied as for any entity, with no restart.

#### Scenario: Damage stored at start and repaired

- **WHEN** A is embedded, B is stored damaged before the component starts, and later B is written with a valid value
- **THEN** after the first build `search` answers A with `damaged_entities` 1; after the repair B is a hit and the
  count is gone (`TestDamagedEntityLeftOutOfSearch`)

#### Scenario: A valid entity overwritten with damage

- **WHEN** B is embedded and then overwritten with bytes that do not decode
- **THEN** B is no longer a hit and `search` counts 1 (`TestDamagedEntityLeftOutOfSearch`)

### Requirement: Similar to a damaged entity is refused

`similar` whose `entity_id` is damaged SHALL be refused fatal `graph_state_reset_required` with the refusal the
follower recorded, as graph-ingest's entity read refuses it.

#### Scenario: Similar to B

- **WHEN** B is damaged
- **THEN** `similar` with `entity_id` B is refused fatal `graph_state_reset_required`
  (`TestDamagedEntityLeftOutOfSearch`)

## MODIFIED Requirements

### Requirement: The embedding verbs are withheld until they can answer

Before answering, each verb SHALL ask the follower's `QueryReady`: a lost feed, an incomplete first build or a held
hop-1 failure (`delete_failed`, `pending_write_failed`, `entity_read_failed`) SHALL be refused transient
`index_not_ready`. A Worker failure SHALL NOT withhold an answer, nor SHALL a damaged entity. A vector cache that has
not applied its initial snapshot, or whose watch is lost, SHALL be refused transient `index_not_ready`.

#### Scenario: Before the first build

- **WHEN** a request arrives before the follower's first build is complete
- **THEN** it is refused transient `index_not_ready`, and the same request answers once the build completes
  (`TestQueriesWithheldUntilReady`)

#### Scenario: A hop-1 failure

- **WHEN** the failed map holds one `entity_read_failed` entry after the first build
- **THEN** both verbs are refused transient `index_not_ready` until that entity converges
  (`TestQueriesWithheldUntilReady`)

#### Scenario: A document whose text cannot be extracted

- **WHEN** the failed map holds one `content_error` entry and no hop-1 failure
- **THEN** `search` answers from the other vectors with `failed_records` 1 (`TestWorkerFailureAnswersAndCounts`)
