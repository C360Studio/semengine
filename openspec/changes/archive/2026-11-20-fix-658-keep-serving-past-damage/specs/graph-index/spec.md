# graph-index Delta

## ADDED Requirements

### Requirement: A damaged entity is left out and counted

When the stored value graph-index reads for an entity is refused by `graph.UnmarshalEntityState`, or its key is not a
literal entity ID, graph-index SHALL report it to the follower, remove every row it built from the entity's earlier
value, and complete the revision. Every reply SHALL carry `damaged_entities`, the number of damaged entities recorded
when it answered. A valid value or a delete for the key SHALL be applied as for any entity, with no restart.

#### Scenario: Damage stored at start

- **WHEN** A is valid with a name, an alias and a relationship, and B is stored damaged before graph-index starts
- **THEN** after the first build `byName`, `alias` and `outgoing` answer A with `damaged_entities` 1, and `Health`
  names B (`TestDamagedEntityLeftOutAndNamed`)

#### Scenario: A valid entity overwritten with damage

- **WHEN** B is indexed and then overwritten with bytes that do not decode
- **THEN** B's name, alias and relationship rows are gone, the replies count 1, and the revision completes
  (`TestDamagedEntityLeftOutAndNamed`)

#### Scenario: Repair without restart

- **WHEN** B is then written with a valid value
- **THEN** B's rows answer, replies carry no `damaged_entities`, and `Healthy` is true (`TestRepairedEntityReturns`)

### Requirement: A relationship read of a damaged entity is refused

`outgoing` for a damaged entity SHALL be refused fatal `graph_state_reset_required` with the refusal the follower
recorded, as graph-ingest's entity read refuses it. `incoming` for it SHALL answer, since its rows are the other
entities' facts.

#### Scenario: Outgoing and incoming of B

- **WHEN** B is damaged and A holds a relationship to B
- **THEN** `outgoing` on B is refused fatal `graph_state_reset_required`, and `incoming` on B answers A's edge
  (`TestDamagedEntityLeftOutAndNamed`)

## MODIFIED Requirements

### Requirement: Startup-replay queries get a retryable refusal

Each of graph-index's eight verbs SHALL answer class transient, code `index_not_ready`, while its initial build is not
applied or while index writes it requires are unresolved. A stored damaged entity SHALL NOT refuse a verb, except
`outgoing` for that entity. Once the build is applied and no write is unresolved, the same request SHALL succeed,
whatever the lag.

#### Scenario: A request during the replay

- **WHEN** a request on any of the eight verbs arrives before the initial build is applied
- **THEN** it fails with class transient and code `index_not_ready`, and the same request succeeds after the build
- **AND** the test that verifies this is `TestQueryDuringStartupReplayIsIndexNotReady`

### Requirement: Health reads named stuck states only

graph-index's `Healthy` SHALL be false exactly while it is not running or a stuck state holds: a damaged entity
stored, a required index write unresolved, or the feed lost. `ErrorCount` SHALL never lower it.

#### Scenario: A failed write and its repair

- **WHEN** an entity's required writes fail and are later repaired, and errors were counted
- **THEN** `Healthy` is false while the failure holds and true after the repair
- **AND** the test that verifies this is `TestHealthIgnoresErrorCountAndTracksStuckStates`
