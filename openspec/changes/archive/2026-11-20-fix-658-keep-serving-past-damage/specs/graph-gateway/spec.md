# graph-gateway Delta

## ADDED Requirements

### Requirement: An answer counts the damaged entities its replies left out

When a root field's reply carries `damaged_entities` above 0, the answer SHALL carry top-level
`extensions.damagedEntities`, the largest such count among the request's root fields, and SHALL carry no such key
otherwise. No root field's selected fields change.

#### Scenario: One root field over a reply with a count

- **WHEN** a client asks `pathSearch` and graph-query's reply carries `damaged_entities` 1
- **THEN** the answer carries `extensions.damagedEntities` 1 and `data.pathSearch` holds exactly the selected fields
  (`TestAnswerCarriesDamagedCount`)
