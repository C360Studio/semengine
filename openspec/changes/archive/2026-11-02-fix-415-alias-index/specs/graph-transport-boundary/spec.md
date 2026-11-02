# graph-transport-boundary

## MODIFIED Requirements

### Requirement: Index replies carry the revision they reflect

Every successful reply on `graph.index.query.*` SHALL be a `graph.QueryResponse` carrying `data`, `indexed_revision`,
`producer` and `timestamp`, where every `ENTITY_STATES` revision at or below `indexed_revision` is reflected in `data`.
`graph.NewQueryResponse` SHALL refuse an empty producer.

#### Scenario: A reply's revision is a lower bound

- **WHEN** graph-index answers a request with `indexed_revision` R
- **THEN** every write to `ENTITY_STATES` at revision R or below is reflected in the reply
- **AND** the test that verifies this is `TestQueryResponseCarriesIndexedRevisionAndProducer`; the read order that
  makes R a lower bound is checked in review and recorded as an uncovered case

## REMOVED Requirements

### Requirement: Alias replies may resolve stale aliases while issue #415 is open

**Reason**: issue #415 is fixed (ADR-114). The alias read implements replacement like every other index read; the
`graph-index` requirements "Indexes implement replacement" and "An alias resolves to its one current holder" state it.

**Migration**: none. Deployments wipe and start fresh; rows in the raw alias layout are not read.
