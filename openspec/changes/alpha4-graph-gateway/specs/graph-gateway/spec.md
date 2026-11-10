# graph-gateway

## ADDED Requirements

### Requirement: Read-only GraphQL reads over the graph query verbs

The graph gateway SHALL answer the GraphQL operations `entity`, `entitiesByPrefix`, `entitiesByPredicate`,
`pathSearch` and `trajectory` by reading the graph through `graph.query.*`, and SHALL write nothing to the graph.
The design replaces this placeholder with the full requirement set.

#### Scenario: an entity read answers from graph-query

- **WHEN** a client posts an `entity(id)` query for a stored entity
- **THEN** the gateway answers with that entity as graph-query returns it
