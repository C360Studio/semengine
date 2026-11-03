# graph-index

## ADDED Requirements

### Requirement: An unmet `min_revision` is refused

Each of graph-index's eight verbs SHALL accept `min_revision` (`graph.MinRevisionField`; zero means none) and, before
its first bucket read, compare it with the revision the reply would carry. At or below that revision the verb SHALL
answer as usual. Above it, the verb SHALL refuse with class transient, code `revision_not_reached`, and detail
`indexed_revision` naming that revision. No reply SHALL wait for the revision.

#### Scenario: The boundary

- **WHEN** a follower at revision R receives a request on any of the eight verbs with `min_revision` R, then with R+1
- **THEN** the first is answered with `indexed_revision` at or above R
- **AND** the second is refused with `revision_not_reached` and detail R (`TestMinRevisionBoundary`)

#### Scenario: A strict decoder accepts the field

- **WHEN** a `predicateList` request carries `min_revision`
- **THEN** it is not refused as an unknown field (`TestMinRevisionBoundary`)

### Requirement: `index_not_ready` keeps its meaning

A request SHALL be validated first, then refused `index_not_ready` if the index is not ready, and only then compared
with `min_revision`. An unmet `min_revision` SHALL NOT be reported as `index_not_ready` (ADR-109 Decision 4).

#### Scenario: Not ready and behind

- **WHEN** a request carries a `min_revision` above the follower's revision while the follower withholds readiness
- **THEN** it is refused `index_not_ready`, not `revision_not_reached` (`TestMinRevisionBoundary`)
