# graph-index

## ADDED Requirements

### Requirement: An entity that cannot be planned is held as failed

When graph-index cannot build an entity's index rows from a stored value it accepted, it SHALL mark the entity failed,
counting it once in `ErrorCount` and in `semengine_graph_index_write_failures_total`, and hold readiness until the
entity is rewritten or deleted. It SHALL NOT complete the revision with the entity's earlier rows standing. Every value
`graph.UnmarshalEntityState` accepts SHALL be plannable.

#### Scenario: Accepted values plan

- **WHEN** generated entity states at the byte bounds of entity IDs, predicates, aliases and names pass
  `graph.UnmarshalEntityState`
- **THEN** each builds an index plan, with the seed and the completed checks recorded
  (`TestContractValidStateAlwaysPlans`)
