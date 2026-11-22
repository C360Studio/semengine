# graph-embedding Delta

## MODIFIED Requirements

### Requirement: The embedding verbs answer in the envelope

Each answer SHALL be a `graph.QueryResponse` with producer `graph-embedding` and `indexed_revision` 0, whose `data` is a
`graph.SemanticData` (with `embedder_type` the name the embedder was registered under) or a `graph.SimilarData`. Its
`results` SHALL be a JSON array, its `unreadable_records` SHALL be the number of `EMBEDDING_INDEX` records the cache
could not decode whose keys the request's `scope` accepts (every one when `scope` is empty, and on `similar`), and its
`failed_records` the number of entities whose latest embedding failed in the Worker.

#### Scenario: An empty index

- **WHEN** a `search` request arrives after the first build with no generated record
- **THEN** the reply's `data.results` is `[]` and `indexed_revision` is 0 (`TestSearchAnswersEnveloped`)
