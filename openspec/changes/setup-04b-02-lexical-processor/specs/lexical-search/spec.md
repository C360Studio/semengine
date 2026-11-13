## ADDED Requirements

### Requirement: A processor keeps the word index current from ENTITY_STATES

Draft, to be replaced by the design. A processor SHALL fill a `graph/lexical` index from `ENTITY_STATES`, rebuild it
on every start, and answer search requests from it only once the rebuild has caught up.

#### Scenario: A restart rebuilds the index

- **WHEN** the processor restarts after entities were written and deleted
- **THEN** a search answers only the live entities whose words match
