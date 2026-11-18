# graph-transport-boundary Delta

## ADDED Requirements

### Requirement: An enveloped reply counts the damaged entities it left out

`graph.QueryResponse` SHALL carry `damaged_entities`, omitted at 0: the number of damaged entities (stored values
`graph.UnmarshalEntityState` refuses) the producer held and left out when it answered. graph-index, graph-embedding
and graph-lexical SHALL set it on every reply. graph-query SHALL set its own enveloped replies to the largest
`damaged_entities` among the enveloped replies the answer was built from, 0 when built from none, and SHALL forward the
replies it forwards unchanged.

#### Scenario: A producer with damage

- **WHEN** graph-index answers while one damaged entity is recorded
- **THEN** the reply carries `damaged_entities` 1, and a reply with none recorded carries no such key
  (`TestQueryResponseDamagedEntitiesOmittedAtZero`, `TestDamagedEntityLeftOutAndNamed`)

#### Scenario: Several index replies

- **WHEN** `pathSearch` is built from graph-index replies carrying `damaged_entities` 2 and 3
- **THEN** its reply carries 3 (`TestComposedReplyCarriesLargestDamagedCount`)
