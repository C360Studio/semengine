# graph-transport-boundary Delta

## MODIFIED Requirements

### Requirement: Reserved request subjects have one declaration

Each reserved request subject SHALL be declared once: graph-ingest's (`graph.ingest.query.*`), graph-index's
(`graph.index.query.*`), graph-query's (`graph.query.*`, only the verbs it serves), the derived-index verbs
graph-query forwards to and graph-lexical's (`graph.lexical.query.*`) in the `graph` verb table, and the mutation
family (`graph.mutation.>`) in `internal/graphmutation`. A verb a consumer serves under `graph.query.*` that the table
does not declare SHALL not be reserved.

#### Scenario: graph-index's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `outgoing`, `incoming`, `alias`, `predicate`, `predicateList`, `predicateStats`,
  `predicateCompound` and `byName` on `graph.index.query.*`, each with responder `graph-index`

#### Scenario: graph-query's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `entity`, `batch`, `prefix`, `entityByAlias`, `relationships`, `byName`, `predicate`, `pathSearch`,
  `hierarchyStats`, `summary`, `semantic`, `similar`, `spatial`, `temporal`, `localSearch`, `globalSearch` and
  `searchGraph` on `graph.query.*`, each with responder `graph-query`, `predicate` enveloped with a
  `graph.PredicateData`, `localSearch` enveloped with a `graph.LocalSearchData`, and `globalSearch` and `searchGraph`
  with a `graph.GlobalSearchData`

#### Scenario: The forwarded derived-index verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `search` and `similar` on `graph.embedding.query.*` with responder `graph-embedding`, `bounds` on
  `graph.spatial.query.*` with responder `graph-index-spatial`, and `range` on `graph.temporal.query.*` with responder
  `graph-index-temporal` (`TestQueryVerbsDeclareEachRespondersVerbs`)

#### Scenario: graph-lexical's verb is in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `search` on `graph.lexical.query.*` with responder `graph-lexical`, enveloped with a
  `graph.LexicalSearchData` (`TestQueryVerbsDeclareEachRespondersVerbs`)
