# graph-transport-boundary Delta

## ADDED Requirements

### Requirement: A damaged-entity refusal names the entities in detail

Every fatal `graph_state_reset_required` refusal graph-ingest's query verbs answer SHALL carry `detail.entity_ids`, the
IDs of the damaged entities it refuses for, sorted, each once and uncapped: the one entity an entity read names, and
every damaged entity of the page a prefix read fetched. `graph.StateRefusalEntityIDs` SHALL return those IDs from the
error `RequestClassified` returns, and nil for an error that is not such a refusal.

#### Scenario: A damaged entity read over the wire

- **WHEN** B's stored value does not decode and the entity verb is asked for B through `RequestClassified`
- **THEN** the error's code is `graph_state_reset_required` and `graph.StateRefusalEntityIDs` returns [B]
- **AND** the test that verifies this is `TestIntegration_DamagedRefusalNamesEntitiesInDetail`

#### Scenario: A prefix page with two damaged entities

- **WHEN** a prefix holds A, B and C and B and C do not decode
- **THEN** the prefix verb is refused and `graph.StateRefusalEntityIDs` returns [B, C]
- **AND** the test that verifies this is `TestIntegration_DamagedRefusalNamesEntitiesInDetail`
