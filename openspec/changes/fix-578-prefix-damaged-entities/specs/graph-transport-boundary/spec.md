# graph-transport-boundary Delta

## MODIFIED Requirements

### Requirement: Reserved request subjects have one declaration

Each reserved request subject SHALL be declared once: graph-ingest's (`graph.ingest.query.*`), graph-index's
(`graph.index.query.*`), graph-query's (`graph.query.*`, only the verbs it serves), the derived-index verbs
graph-query forwards to and graph-lexical's (`graph.lexical.query.*`) in the `graph` verb table, and the mutation
family (`graph.mutation.>`) in `internal/graphmutation`. A verb a consumer serves under `graph.query.*` that the table
does not declare SHALL not be reserved.

#### Scenario: graph-ingest's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `entity`, `batch`, `prefix` and `prefixIDs` on `graph.ingest.query.*`, each with responder
  `graph-ingest`, bare, `prefixIDs` with a `graph.PrefixIDsResponse` (`TestQueryVerbsDeclareEachRespondersVerbs`)

#### Scenario: graph-index's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `outgoing`, `incoming`, `alias`, `predicate`, `predicateList`, `predicateStats`,
  `predicateCompound` and `byName` on `graph.index.query.*`, each with responder `graph-index`

#### Scenario: graph-query's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `entity`, `batch`, `prefix`, `prefixIDs`, `entityByAlias`, `relationships`, `byName`,
  `predicate`, `pathSearch`, `hierarchyStats`, `summary`, `semantic`, `similar`, `spatial`, `temporal`,
  `localSearch`, `globalSearch` and `searchGraph` on `graph.query.*`, each with responder `graph-query`, `prefixIDs`
  bare with a `graph.PrefixIDsResponse`, `predicate` enveloped with a `graph.PredicateData`, `localSearch` enveloped
  with a `graph.LocalSearchData`, and `globalSearch` and `searchGraph` with a `graph.GlobalSearchData`

#### Scenario: The forwarded derived-index verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `search` and `similar` on `graph.embedding.query.*` with responder `graph-embedding`, `bounds` on
  `graph.spatial.query.*` with responder `graph-index-spatial`, and `range` on `graph.temporal.query.*` with responder
  `graph-index-temporal` (`TestQueryVerbsDeclareEachRespondersVerbs`)

#### Scenario: graph-lexical's verb is in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `search` on `graph.lexical.query.*` with responder `graph-lexical`, enveloped with a
  `graph.LexicalSearchData` (`TestQueryVerbsDeclareEachRespondersVerbs`)

### Requirement: Query refusals carry a code

Once `Start` has completed, every refusal graph-ingest's query handlers return on `graph.ingest.query.entity`,
`graph.ingest.query.batch`, `graph.ingest.query.prefix` and `graph.ingest.query.prefixIDs` SHALL carry a classified
code. A request that arrives before `Start` completes is answered as "graph-ingest refuses request work until Start
completes" says, with no code. Giving a refusal a code SHALL NOT change its class.

#### Scenario: A failed key listing

- **WHEN** listing keys for a prefix query or a prefix ID listing fails
- **THEN** graph-ingest refuses it with class transient and code `internal`

## ADDED Requirements

### Requirement: A prefix ID listing reads no stored value

`graph.ingest.query.prefixIDs` SHALL take a `graph.PrefixQueryRequest` and answer a bare `graph.PrefixIDsResponse`:
`ids`, the page's keys in lexicographic order, never `null`; `total`, as a `graph.ingest.query.prefix` reply names
it; and `next_cursor`, omitted when no key follows the page. It SHALL read no stored value except the existence check
of an exact six-part prefix, so a damaged key SHALL be listed, and no damaged entity SHALL be recorded.

#### Scenario: A damaged entity is listed

- **WHEN** a prefix holds A, B and C, B's stored value does not decode, and `prefixIDs` lists the prefix
- **THEN** the reply is `ids` [A, B, C] with `total` 3, B is not recorded as damaged, and
  `graph.ingest.query.prefix` on the same prefix still refuses with `graph_state_reset_required`
  (`TestPrefixIDsListsDamagedEntity`)

### Requirement: A prefix ID listing pages as the prefix verb does

`graph.ingest.query.prefixIDs` SHALL check the request, list, sort, drop repeated keys, count, apply the cursor and
clamp the limit as `graph.ingest.query.prefix` does. Its page SHALL be the longest leading run of the selected keys
whose whole encoded reply fits the server's maximum payload, with `next_cursor` its last ID when keys follow. A
cursor SHALL be sent back to the verb that gave it.

#### Scenario: Walking the cursor

- **WHEN** keys, limits and maximum payloads are generated (seed recorded) and the cursor is followed to the end
- **THEN** the pages joined hold each sorted distinct key once, every `total` is the key count, every reply fits, and
  each page is the longest run that fits (`TestPrefixIDsCursorWalkMatchesSortedKeys`)

#### Scenario: A first key that cannot fit

- **WHEN** the maximum payload is smaller than a reply holding the first key
- **THEN** graph-ingest refuses with class invalid and code `response_too_large`
