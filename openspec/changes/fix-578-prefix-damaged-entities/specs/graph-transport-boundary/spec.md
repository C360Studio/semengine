# graph-transport-boundary Delta

## ADDED Requirements

### Requirement: A prefix page names each damaged entity

A `graph.ingest.query.prefix` reply SHALL hold every entity on the page whose stored value decodes, and SHALL list in
`missing`, with reason `error`, each key on the page whose stored value the decode rule refuses. A key whose read
comes back not found SHALL NOT be listed. `missing` SHALL count in the byte fit, and the page's cursor SHALL be its
last key, entity or damaged. Any other read failure SHALL refuse the page with class transient and code `internal`.

#### Scenario: One damaged entity on a page

- **WHEN** a prefix holds A, B and C and B's stored value does not decode
- **THEN** the reply holds A and C, `missing` is B with reason `error`, `total` is 3, and B is recorded and not cached

#### Scenario: A damaged key ends the page

- **WHEN** a page's last key is damaged and more keys follow
- **THEN** `next_cursor` is that key, and the next page starts after it

#### Scenario: A read error beside a damaged entity

- **WHEN** one key's read fails with a KV error and another key is damaged
- **THEN** the page is refused with class transient and code `internal`
