# graph-query Delta

## MODIFIED Requirements

### Requirement: Authority and name replies are forwarded unchanged

`entity`, `batch`, `prefix`, `prefixIDs`, `byName` and `predicate` SHALL forward the request bytes to the matching
`graph.ingest.query.*`, `graph.index.query.byName` or `graph.index.query.predicate` verb and return the reply bytes
unchanged, without decoding, re-encoding or re-checking either.

#### Scenario: Bytes pass through

- **WHEN** a caller of `entity`, `batch`, `prefix`, `prefixIDs`, `byName` or `predicate` sends bytes Q, with an unknown
  field and non-canonical spacing, and the downstream verb replies with such bytes B
- **THEN** the downstream verb receives exactly Q and the caller receives exactly B (`TestForwardPassesBytesBothWays`)
