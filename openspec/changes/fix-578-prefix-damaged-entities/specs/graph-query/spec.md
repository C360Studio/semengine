# graph-query Delta

## ADDED Requirements

### Requirement: A prefix page with a damaged entity is refused

When a prefix page that `hierarchyStats`, `summary`, `localSearch` or `globalSearch` reads lists any entry in
`missing`, the verb SHALL refuse with class fatal and code `graph_state_reset_required`, naming the IDs, and SHALL NOT
answer from the rest of the page.

#### Scenario: Counts over a damaged entity

- **WHEN** `hierarchyStats` reads a page whose `missing` lists one entity
- **THEN** the caller receives fatal `graph_state_reset_required` naming it
