# graph-index Delta

## RENAMED Requirements

- FROM: `### Requirement: DataFlow counts deliveries`
- TO: `### Requirement: DataFlow counts entity reads`

## MODIFIED Requirements

### Requirement: DataFlow counts entity reads

graph-index's follower SHALL watch `ENTITY_STATES` keys only (`follower.Config` keys-only field), and graph-index's
`DataFlow` SHALL count one input per entity it reads from `ENTITY_STATES` to apply it, with the byte length of the
value it read, and SHALL not count index writes. Writes the coalescer folds into one apply SHALL count as one read; a
delete, whose read finds no entity, SHALL count none. `watch_events_total` SHALL still count each delivered entry by
kind.

#### Scenario: One entity with three relationships

- **WHEN** one entity with three relationships is written once
- **THEN** `DataFlow().Messages` rises by one

#### Scenario: Coalesced updates are each counted

- **GIVEN** `coalesce_ms` above zero
- **WHEN** one entity is written three times with values of different sizes, each write read before the next
- **THEN** `DataFlow().Messages` rises by three and `DataFlow().Bytes` by the sum of the three values' lengths
- **AND** the test that verifies this is `TestDataFlowCountsEachEntityRead`

#### Scenario: A replay counts the stored bytes

- **WHEN** graph-index starts over stored entities, one written twice, and the replay is applied
- **THEN** `DataFlow().Messages` is the number of stored entities and `DataFlow().Bytes` the summed lengths of the
  values stored last
- **AND** the test that verifies this is `TestDataFlowCountsReplayedBytes`
