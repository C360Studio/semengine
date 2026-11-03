# graph-query

## ADDED Requirements

### Requirement: The component serves exactly its declared verbs

The graph-query component SHALL subscribe to the subject of every `graph.QueryVerbs()` entry whose responder is
`graph-query`, and to no other request subject. `Start` SHALL fail naming a declared verb with no handler. A verb
another component serves under `graph.query.*` that the table does not declare SHALL be left to that component.

#### Scenario: A consumer verb beside the component

- **WHEN** graph-query has started and another subscriber serves `graph.query.sources`
- **THEN** a request on `graph.query.sources` is answered by that subscriber alone
- **AND** a request on each declared verb is answered by graph-query (`TestGraphQueryServesExactlyTheDeclaredVerbs`)

### Requirement: Authority and name replies are forwarded unchanged

`entity`, `batch`, `prefix` and `byName` SHALL forward the request bytes to the matching `graph.ingest.query.*` or
`graph.index.query.byName` verb and return the reply bytes unchanged, without decoding, re-encoding or re-checking
either.

#### Scenario: Bytes pass through

- **WHEN** a caller of `entity`, `batch`, `prefix` or `byName` sends bytes Q, with an unknown field and non-canonical
  spacing, and the downstream verb replies with such bytes B
- **THEN** the downstream verb receives exactly Q and the caller receives exactly B (`TestForwardPassesBytesBothWays`)

### Requirement: A refusal reaches the caller intact

Every verb SHALL return a downstream classified refusal with its class, code and detail unchanged. When a downstream
request reports no responders, the component SHALL refuse with class transient and code `internal`, naming the
subject. A downstream request that times out SHALL stay transient.

#### Scenario: A relayed refusal

- **WHEN** a downstream verb refuses with class C, code X and detail D
- **THEN** the caller's classified error has class C, code X and detail D (`TestForwardedRefusalKeepsClassCodeDetail`)

#### Scenario: No responders

- **WHEN** a downstream request reports no responders
- **THEN** the caller receives a transient `internal` refusal naming that subject (`TestNoRespondersIsTransient`)

### Requirement: Alias lookup refuses what the alias verb refuses

`entityByAlias` SHALL ask the alias verb first and SHALL return any refusal it gives, `alias_ambiguous`,
`index_not_ready`, `internal` and `revision_not_reached` among them, without reading an entity (ADR-114).

#### Scenario: A shared alias

- **WHEN** a query names an alias two entities share
- **THEN** the caller receives the `alias_ambiguous` refusal, not "not found" (`TestEntityByAlias`)

### Requirement: Alias lookup answers in one envelope

The request SHALL be `alias` with optional `min_revision`. A held alias SHALL read its holder. An alias with no holder
SHALL read the input as an entity ID when it is a valid one. A success SHALL be a `graph.QueryResponse` from producer
`graph-query` whose `data` is the entity verb's JSON as it came, a `graph.ExactEntity` not re-decoded into a struct,
and whose `indexed_revision` is the alias reply's.

#### Scenario: An ID given to the alias verb

- **WHEN** a query names a valid entity ID that no entity holds as an alias, and that entity exists
- **THEN** the reply's `data` is that entity and `indexed_revision` is the alias reply's (`TestEntityByAlias`)

### Requirement: A broken alias or entity reply is refused

An alias reply that does not decode or names no `producer`, and an entity reply that is not JSON, SHALL be refused
transient `internal`, naming the subject.

#### Scenario: An entity reply that is not JSON

- **WHEN** the alias verb names a holder and the entity verb replies with bytes that are not JSON
- **THEN** the caller receives a transient `internal` refusal naming the entity verb's subject (`TestEntityByAlias`)

### Requirement: An unknown alias that is no entity ID is not found

An alias with no holder whose input is not a valid entity ID SHALL be refused with class invalid and code
`entity_not_found`, without a second request.

#### Scenario: An unknown alias

- **WHEN** a query names `sn-1`, which no entity holds as an alias
- **THEN** the caller receives `entity_not_found` and the entity verb receives no request (`TestEntityByAlias`)

### Requirement: Relationships are enveloped and refuse an unknown direction

`relationships` SHALL take `entity_id` and `direction`, forwarding the request bytes unchanged, and SHALL read
`direction` as `outgoing` or `incoming`, and SHALL refuse any other value, or none, with `invalid_request`. A success
SHALL be a `graph.QueryResponse` from producer `graph-query`, `indexed_revision` being the index reply's and `data` a
non-null list of `graph.RelationshipEdge`, each with `predicate`, `from_entity_id` and `to_entity_id`.

#### Scenario: Row keys on the wire

- **WHEN** a real index holds one relationship and a caller asks for it in each direction
- **THEN** `data` compares equal to the literal JSON row, with exactly the keys `predicate`, `from_entity_id` and
  `to_entity_id`, in both directions (`TestRelationshipsRowKeys`)

#### Scenario: A direction the component does not read

- **WHEN** a request sends `direction` `both`, or none
- **THEN** the caller receives `invalid_request` and no index verb receives a request
  (`TestRelationshipsRefusesUnknownDirection`)

### Requirement: A broken index reply is refused

An index reply to `relationships` that does not decode or names no `producer` SHALL be refused transient `internal`.

#### Scenario: A broken index reply

- **WHEN** the index replies `{}`
- **THEN** the caller receives a transient `internal` refusal, not an empty list
  (`TestRelationshipsRefusesBrokenIndexReply`)

### Requirement: Revision requests answer or refuse, never wrongly succeed

Every verb SHALL accept `min_revision`. On an enveloped verb it SHALL reach graph-index unchanged; on a bare verb it
SHALL be ignored, an authority read reflecting every committed write. A success on an enveloped verb to `min_revision`
M SHALL carry `indexed_revision` at or above M.

#### Scenario: Reading a write back

- **WHEN** a caller writes entity E, receives `kv_revision` W, and requests `relationships` for E with `min_revision`
  W
- **THEN** it receives a reply with `indexed_revision` at or above W reflecting the write, or a `revision_not_reached`
  refusal
- **AND** never a success with `indexed_revision` below W (`TestReadYourWrites`)

#### Scenario: A revision far ahead

- **WHEN** `byName`, `relationships` in either direction, or `entityByAlias` carries a `min_revision` far above the
  index's revision
- **THEN** the caller receives `revision_not_reached` with its class and detail
  (`TestMinRevisionRefusalThroughGraphQuery`)

### Requirement: Requests are counted for operators

Every verb SHALL count its request once in `DataFlow` on entry and SHALL count one error in `Health` whenever
`graph-query` itself refuses because no usable answer arrived: a downstream request got no answer, or a reply that
`entityByAlias` or `relationships` reads does not decode, names no `producer` or is not JSON. A responder's classified
refusal and a caller's `invalid_request` SHALL NOT count as an error.

#### Scenario: An unanswered forward and a refusal

- **WHEN** one forward is refused by its responder and another finds no responder
- **THEN** `DataFlow` shows both requests and `Health` shows one error (`TestForwardCountsRequestsAndUnansweredOnes`)

#### Scenario: A verb that makes two requests

- **WHEN** `entityByAlias` reads a holder through the alias and entity verbs, and later finds no responder on the
  entity verb
- **THEN** each request to `entityByAlias` is one in `DataFlow`, and only the second is an error in `Health`
  (`TestComposedVerbsCountAsForwardsDo`)

#### Scenario: A broken reply

- **WHEN** an alias or index reply does not decode or names no `producer`, or an entity reply is not JSON
- **THEN** the request is one error in `Health`, while a responder's refusal and a caller's `invalid_request` add none
  (`TestComposedVerbsCountAsForwardsDo`)

### Requirement: Configuration is strict and read

Decoding SHALL refuse an unknown key, naming it. `query_timeout_ms` SHALL be the deadline on the wait for each
downstream reply, default 15000; like the handler's context, it does not end a wait to write to a broker that has
stopped reading (ADR-112, "Time"). A configuration with no ports SHALL take the default required `graph_queries`
NATS-request port on `graph.query.*`, and any other input port shape SHALL be refused.

#### Scenario: A key the pin ignored

- **WHEN** a configuration sets `max_traversal_depth`
- **THEN** construction fails naming that key (`TestUnknownConfigKeyIsRefused`)

#### Scenario: The timeout is read

- **WHEN** a downstream verb never replies and `query_timeout_ms` is small
- **THEN** the caller receives a transient refusal before its own request timeout (`TestQueryTimeoutIsRead`)
