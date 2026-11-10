# composed-stack

## ADDED Requirements

### Requirement: The graph stack answers GraphQL reads

The composed smoke stack SHALL include graph-gateway. Over the process's HTTP port at `/graph-gateway/graphql` it SHALL
answer `entity`, `entitiesByPrefix`, `entitiesByPredicate` and `pathSearch` for the entities it ingested.

#### Scenario: The four reads over HTTP

- **WHEN** the smoke stack has ingested its fixture and a client posts each of the four operations to
  `/graph-gateway/graphql`
- **THEN** each answer is HTTP 200 with no `errors` and holds the fixture's entity IDs and triples asked for
  (`TestSmokeAnswersGraphQL`)
