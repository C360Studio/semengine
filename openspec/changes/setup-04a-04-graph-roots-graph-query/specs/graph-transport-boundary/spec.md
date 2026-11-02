# graph-transport-boundary

## MODIFIED Requirements

### Requirement: Reserved request subjects have one declaration

Each reserved request subject SHALL be declared once: graph-ingest's (`graph.ingest.query.*`), graph-index's
(`graph.index.query.*`) and graph-query's (`graph.query.*`, only the verbs it serves) in the `graph` verb table, each
entry naming verb, subject, responder, whether its reply is enveloped, and its reply type; and the mutation family
(`graph.mutation.>`) in `internal/graphmutation`. Entity and batch requests SHALL be Go types declared in `graph`
(`EntityRequest`, `EntityBatchRequest`). A verb a consumer serves under `graph.query.*` that the table does not declare
SHALL not be reserved.

#### Scenario: Caller and responder share the request type

- **WHEN** a caller sends an entity request and graph-ingest answers it
- **THEN** both use the `graph.EntityRequest` type declared in `graph`

#### Scenario: graph-index's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `outgoing`, `incoming`, `alias`, `predicate`, `predicateList`, `predicateStats`, `predicateCompound`
  and `byName` on `graph.index.query.*`, each with responder `graph-index`

#### Scenario: graph-query's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `entity`, `batch`, `prefix`, `entityByAlias`, `relationships` and `byName` on `graph.query.*`, each
  with responder `graph-query`

### Requirement: The responder serves exactly its declared verbs

Each responder, graph-ingest, graph-index and graph-query, SHALL subscribe to the subjects of the verb table's entries
whose responder it is, and to no other request subject, and SHALL fail `Start` naming a declared verb it has no handler
for. When `Start` returns, the server SHALL route each of those subjects to it from any connection (ruled on #144).

#### Scenario: Subscriptions match the table

- **WHEN** a responder has started and its test adapter lists its request subscriptions
- **THEN** the listed subjects equal the table's subjects for that responder
- **AND** the tests that verify this are `TestGraphIngestServesExactlyTheDeclaredVerbs` and
  `TestGraphIndexServesExactlyTheDeclaredVerbs`
- **AND** graph-query's proof, `TestGraphQueryServesExactlyTheDeclaredVerbs`, is count and behaviour rather than a list:
  it holds one recorded subscription per row declared for it, each of those subjects gets its refusal naming the row
  behind it, and a `graph.query.*` subject outside the table has no responder until a consumer subscribes, which a
  wildcard subscription would answer
- **AND** neither proof catches a subscription to one more literal subject that the responder does not record

#### Scenario: Routable when Start returns

- **WHEN** a responder's `Start` has returned and another connection sends a request on each listed subject
- **THEN** every request reaches a responder; none gets "no responders"

## ADDED Requirements

### Requirement: The verb table declares each reply's shape

An entry's `Enveloped` SHALL be true when any part of the verb's answer comes from a graph-index reply, and false when
its responder is an authority or only forwards an authority's reply. `Reply` SHALL be the type of the reply body, or of
`data` when enveloped. A caller SHALL decode by verb; nothing SHALL decide a reply's shape from its content.

#### Scenario: The table matches the wire

- **WHEN** each declared verb of graph-query answers a real request
- **THEN** its reply decodes by its entry with unknown fields refused
- **AND** `entity` decodes bare and `entityByAlias` enveloped (`TestGraphQueryRepliesDecodeByTheTable`)

### Requirement: An enveloped reply names the smallest revision

A success on an enveloped verb SHALL be a `graph.QueryResponse` whose `indexed_revision` is the smallest among the
graph-index replies the answer was built from.

#### Scenario: One index reply

- **WHEN** `relationships` is built from one graph-index reply at revision R
- **THEN** its `indexed_revision` is R (`TestRelationshipsEnvelopeKeepsIndexRevision`)
