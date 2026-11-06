# graph-transport-boundary

## ADDED Requirements

### Requirement: An empty prefix page is fitted at the bytes it sends

graph-ingest SHALL fit an empty `graph.ingest.query.prefix` page to the maximum payload at the length of its reply's
encoding: `"entities":null` for a group with no keys, `"entities":[]` for a page whose keys were all deleted mid-read.

#### Scenario: An empty reply one byte over the maximum payload

- **WHEN** an empty page's reply is one byte longer than the maximum payload
- **THEN** graph-ingest refuses it with class invalid and code `response_too_large`, naming the reply's length as
  `response_bytes` (`TestMarshalFittingPrefixPageMeasuresAnEmptyPageAsSent`)
