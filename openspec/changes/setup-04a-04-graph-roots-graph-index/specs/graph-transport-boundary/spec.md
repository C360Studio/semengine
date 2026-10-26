# graph-transport-boundary

## MODIFIED Requirements

### Requirement: Reserved request subjects have one declaration

The request/reply verbs of graph-ingest (`graph.ingest.query.*`: entity, batch, prefix) and of graph-index
(`graph.index.query.*`, eight verbs) SHALL be declared once, in a verb table in the `graph` package whose entries name each
verb's name, subject and responder. The entity and batch requests SHALL be Go types declared in `graph`
(`EntityRequest`, `EntityBatchRequest`), shared by caller and responder. The mutation subject family
(`graph.mutation.>`) SHALL be declared once, in `internal/graphmutation`.

#### Scenario: Caller and responder share the request type

- **WHEN** a caller sends an entity request and graph-ingest answers it
- **THEN** both use the `graph.EntityRequest` type declared in `graph`

#### Scenario: graph-index's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `outgoing`, `incoming`, `alias`, `predicate`, `predicateList`, `predicateStats`,
  `predicateCompound` and `byName` on `graph.index.query.*`, each with responder `graph-index`

### Requirement: The responder serves exactly its declared verbs

Each responder, graph-ingest and graph-index, SHALL subscribe to the subjects of the verb table's entries whose
responder it is, and to no other request subject, and SHALL fail `Start` naming a declared verb it has no handler for.
When `Start` returns, the server SHALL route each of those subjects to it from any connection (ruled on #144).

#### Scenario: Subscriptions match the table

- **WHEN** graph-ingest or graph-index has started and its test adapter lists its request subscriptions
- **THEN** the listed subjects equal the table's subjects for that responder
- **AND** the tests that verify this are `TestGraphIngestServesExactlyTheDeclaredVerbs` and `TestGraphIndexServesExactlyTheDeclaredVerbs`

#### Scenario: Routable when Start returns

- **WHEN** a responder's `Start` has returned and another connection sends a request on each listed subject
- **THEN** every request reaches a responder; none gets "no responders"

## ADDED Requirements

### Requirement: Index replies carry the revision they reflect

Every successful reply on `graph.index.query.*` SHALL be a `graph.QueryResponse` carrying `data`, `indexed_revision`,
`producer` and `timestamp`, where every `ENTITY_STATES` revision at or below `indexed_revision` is reflected in `data`,
except on `alias` replies (see the next requirement). `graph.NewQueryResponse` SHALL refuse an empty producer.

#### Scenario: A reply's revision is a lower bound

- **WHEN** graph-index answers a request with `indexed_revision` R
- **THEN** every write to `ENTITY_STATES` at revision R or below is reflected in the reply, except on an `alias`
  reply while issue #415 is open
- **AND** the test that verifies this is `TestQueryResponseCarriesIndexedRevisionAndProducer`; the read order that
  makes R a lower bound is checked in review and recorded as an uncovered case

### Requirement: Alias replies may resolve stale aliases while issue #415 is open

While issue #415 is open (owner ruling, PR #414 comment 6426254115, item 3), an `alias` reply SHALL carry the
follower's watermark as `indexed_revision`, as every reply does, and MAY resolve an alias that was dropped or changed,
or whose entity was deleted, to the old entity. The reply's revision promises nothing about which aliases resolve.

#### Scenario: A dropped alias still resolves

- **WHEN** an entity drops alias A at revision R and graph-index has indexed through R
- **THEN** an `alias` request for A may still answer the old entity, with `indexed_revision` at least R
