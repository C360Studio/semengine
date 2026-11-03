# graph-transport-boundary

## ADDED Requirements

### Requirement: A prefix page names its total

A `graph.ingest.query.prefix` reply SHALL carry `total`, the length of the request's key listing, before the cursor and
before any limit, on every path. It SHALL be counted in the byte fit. It is true as listed.

#### Scenario: A later page

- **WHEN** a group of N entities is read on its second page, or on a page the byte fit trimmed
- **THEN** `total` is N (`TestPrefixPageNamesItsTotal`)

## MODIFIED Requirements

### Requirement: A pattern query lists keys by filter

A non-empty `Pattern` SHALL list matching keys via `KVStore.KeysByFilter` and SHALL NOT fall back to an exact-key `Get`
when zero keys match.

#### Scenario: A pattern with no matches skips exact-key fallback

- **WHEN** a caller sends a valid `Pattern` that matches zero keys in `ENTITY_STATES`
- **THEN** graph-ingest returns an empty `Entities` result (`{"entities":null,"total":0}`) with no `NextCursor` and
  performs no exact-key `Get` call on the bucket

### Requirement: Reserved request subjects have one declaration

Each reserved request subject SHALL be declared once: graph-ingest's (`graph.ingest.query.*`), graph-index's
(`graph.index.query.*`), graph-query's (`graph.query.*`, only the verbs it serves) and the derived-index verbs
graph-query forwards to (`graph.embedding.query.search`, `graph.embedding.query.similar`, `graph.spatial.query.bounds`,
`graph.temporal.query.range`) in the `graph` verb table, and the mutation family (`graph.mutation.>`) in
`internal/graphmutation`. A verb a consumer serves under `graph.query.*` that the table does not declare SHALL not be
reserved.

#### Scenario: graph-index's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `outgoing`, `incoming`, `alias`, `predicate`, `predicateList`, `predicateStats`,
  `predicateCompound` and `byName` on `graph.index.query.*`, each with responder `graph-index`

#### Scenario: graph-query's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `entity`, `batch`, `prefix`, `entityByAlias`, `relationships`, `byName`, `pathSearch`,
  `hierarchyStats`, `summary`, `semantic`, `similar`, `spatial` and `temporal` on `graph.query.*`, each with responder
  `graph-query`

#### Scenario: The forwarded derived-index verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `search` and `similar` on `graph.embedding.query.*` with responder `graph-embedding`, `bounds` on
  `graph.spatial.query.*` with responder `graph-index-spatial`, and `range` on `graph.temporal.query.*` with responder
  `graph-index-temporal` (`TestQueryVerbsDeclareEachRespondersVerbs`)

### Requirement: The verb table declares each reply's shape

An entry's `Enveloped` SHALL be true when any part of the verb's answer comes from a graph-index or other derived-index
reply, or the verb is one graph-query composes, and false when its responder is an authority or only forwards an
authority's reply. `Reply` SHALL be the type of the reply body, or of `data` when enveloped. A caller SHALL decode by
verb; nothing SHALL decide a reply's shape from its content.

#### Scenario: The table matches the wire

- **WHEN** each declared verb of graph-query answers a request, from a real responder where one exists and from literal
  JSON on a stub for the four verbs whose responders are not yet in the tree
- **THEN** its reply decodes by its entry with unknown fields refused
- **AND** `entity` decodes bare and `entityByAlias` enveloped (`TestGraphQueryRepliesDecodeByTheTable`)

### Requirement: An enveloped reply names the smallest revision

A success on an enveloped verb SHALL be a `graph.QueryResponse` whose `indexed_revision` is the smallest among the
derived-index replies the answer was built from, or 0 when the answer was built from none, which claims nothing.

#### Scenario: One index reply

- **WHEN** `relationships` is built from one graph-index reply at revision R
- **THEN** its `indexed_revision` is R (`TestRelationshipsEnvelopeKeepsIndexRevision`)

#### Scenario: Several index replies

- **WHEN** `pathSearch` is built from graph-index replies at revisions R1 and R2, R1 below R2
- **THEN** its `indexed_revision` is R1 (`TestPathSearchRevisionIsTheSmallest`)

#### Scenario: No index reply

- **WHEN** `hierarchyStats` answers, or `summary` with `exclude_predicates`
- **THEN** its `indexed_revision` is 0 (`TestAuthorityOnlyAnswerClaimsNoRevision`)
