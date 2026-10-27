# graph-transport-boundary Specification

## Purpose
This capability fixes where graph-ingest meets NATS. The request subjects it serves are declared once (the query
verbs in the `graph` package's verb table, the mutation subjects in `internal/graphmutation`), and a contract test
refuses any other spelling of them. graph-ingest answers exactly those verbs and does no request work before `Start`
completes; the `graph` package imports no NATS client; query replies are not wrapped in a message envelope, and the
entity query's reply carries the revision it read; and the readiness status a producer writes to `GRAPH_STATUS`
carries no legacy fields.

## Requirements

### Requirement: Reserved request subjects have one declaration

Each reserved request subject SHALL be declared once: graph-ingest's (`graph.ingest.query.*`) and graph-index's
(`graph.index.query.*`) verbs in the `graph` verb table, each entry naming verb, subject and responder, and the
mutation family (`graph.mutation.>`) in `internal/graphmutation`. Entity and batch requests SHALL be Go types declared
in `graph` (`EntityRequest`, `EntityBatchRequest`).

#### Scenario: Caller and responder share the request type

- **WHEN** a caller sends an entity request and graph-ingest answers it
- **THEN** both use the `graph.EntityRequest` type declared in `graph`

#### Scenario: graph-index's verbs are in the table

- **WHEN** `graph.QueryVerbs()` is read
- **THEN** it holds `outgoing`, `incoming`, `alias`, `predicate`, `predicateList`, `predicateStats`,
  `predicateCompound` and `byName` on `graph.index.query.*`, each with responder `graph-index`

### Requirement: Servers and clients take subjects from the declarations

Every server and client in the module SHALL take the reserved request subjects from their declarations.

#### Scenario: A client needs a reserved subject

- **WHEN** a server or client in the module needs a verb's subject or the mutation family
- **THEN** it reads it from the verb table or from `internal/graphmutation`, not from a literal

### Requirement: A contract test refuses other spellings of a reserved subject

A contract test run by `task test:unit` SHALL fail, naming the file and line, when a non-test Go file other than the two
declaring files spells one of these subjects, or the `graph.mutation.` prefix, as a string literal; a sensitivity test
SHALL plant such a literal in a temporary module and require the failure.

#### Scenario: A literal spelling is added

- **WHEN** a non-test file outside the declarations spells `graph.ingest.query.entity` as a string literal
- **THEN** the contract test fails naming the file and line

#### Scenario: The declarations and tests are not reported

- **WHEN** the subjects appear in a declaring file, in a `_test.go` file, or in a comment
- **THEN** the contract test reports nothing for them

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

### Requirement: graph-ingest does no request work before Start completes

graph-ingest's query and mutation handlers SHALL do no work until `Start` has completed. Until then a request SHALL
get a transient error matching `errs.ErrNotStarted` from the handler, and SHALL read no entity, change no state and
record no mutation outcome or rejection; the requester receives the refusal as a transient error with no code. This
holds for a handler the transport runs during a registration that then fails, which a failed `Start`'s rollback has no
subscription to drain.

#### Scenario: A query during a failed Start

- **WHEN** a batch query for a stored entity reaches graph-ingest's handler during a request registration that then
  fails, and `Start` fails with that registration's error
- **THEN** the handler returns a transient error matching `errs.ErrNotStarted` without reading the entity, the
  rollback leaves nothing held, and a following `Stop` returns nil

#### Scenario: A mutation during a failed Start

- **WHEN** a create mutation reaches graph-ingest's handler during a request registration that then fails, and
  `Start` fails with that registration's error
- **THEN** the handler returns a transient error matching `errs.ErrNotStarted`, the entity is not stored, no mutation
  outcome or rejection is recorded, the rollback leaves nothing held, and a following `Stop` returns nil

### Requirement: The graph root imports no transport

The `graph` package SHALL import no NATS client package, directly or through its dependencies. A contract test run by
`task test:unit` SHALL fail, naming the package path, when `go list -deps ./graph` lists `natsclient` or a
`github.com/nats-io` path; a sensitivity test SHALL require the failure for a temporary module whose package reaches
such a path.

#### Scenario: A transport import reaches the root

- **WHEN** a file of the `graph` package imports a package that imports `natsclient`
- **THEN** the contract test fails naming the path it found

### Requirement: Authority reads are not enveloped

Replies on `graph.ingest.query.*` are authority reads and SHALL NOT be enveloped: the entity verb's reply SHALL carry
the key-value revision it read, under the JSON key `kv_revision`. The module SHALL provide no function that decides
from a reply's content whether it is enveloped.

#### Scenario: An entity read carries its revision

- **WHEN** the entity verb's reply for entity E is a bare `graph.ExactEntity` with revision R
- **THEN** the reply's JSON holds R under `kv_revision`, and the reader returns E and revision R

### Requirement: The readiness envelope carries no legacy fields

`graph.IndexStatusResponse`, the value a producer writes to its `GRAPH_STATUS` key, SHALL carry no `published_at`,
`phase`, `revision` or `last_synced` field; `indexed_revision` is the one spelling of the revision. graph-ingest SHALL
publish `bootstrap_complete` true on every envelope, its boot work having finished before `Start` returned.

#### Scenario: A published envelope

- **WHEN** a producer publishes its status
- **THEN** the stored JSON has no `published_at`, `phase`, `revision` or `last_synced` key

#### Scenario: graph-ingest with a backlog standing

- **WHEN** graph-ingest has started and messages are outstanding on its bound consumer
- **THEN** its envelope has `bootstrap_complete` true and `ready` false

### Requirement: Prefix queries support literal prefixes and wildcard patterns

`graph.PrefixQueryRequest` on `graph.ingest.query.prefix` SHALL accept either a 1–6 position literal `Prefix`
(`prefix`), an exact 6-position wildcard `Pattern` (`pattern`, where each position is a canonical literal segment or
`*`), or neither (matching all entities). When `Pattern` is empty, `json.Marshal` SHALL omit `"pattern"` from the JSON
wire representation.

#### Scenario: A six-position wildcard pattern matches entities across systems

- **WHEN** `ENTITY_STATES` holds entities under multiple `system` segments and a caller requests
  `graph.PrefixQueryRequest{Pattern: "o.p.*.intel.platform.*"}` on `graph.ingest.query.prefix`
- **THEN** graph-ingest returns only the entities matching `o.p.*.intel.platform.*` in lexicographic ID order

### Requirement: Prefix and Pattern are mutually exclusive

Setting both `Prefix` and `Pattern` on the same request SHALL be rejected before any KV operation with a classified
invalid error (`errs.ErrorInvalid`, code `invalid_request`).

#### Scenario: Prefix and Pattern are mutually exclusive

- **WHEN** a caller sends a `graph.PrefixQueryRequest` with both `Prefix` and `Pattern` non-empty
- **THEN** graph-ingest rejects the request before any KV read with a classified invalid error (`code:
  "invalid_request"`)

### Requirement: A pattern is validated before any KV operation

A non-empty `Pattern` SHALL be validated before any KV operation via `pkg/types.ValidateEntityIDPattern` (failing with
`errs.ErrorInvalid`, code `entity_id_pattern_invalid` when malformed).

#### Scenario: An invalid pattern is rejected before any KV read

- **WHEN** a caller sends a `graph.PrefixQueryRequest` whose `Pattern` has fewer or more than six positions, contains
  `>`, or contains a malformed segment
- **THEN** graph-ingest rejects the request before any KV read with a classified invalid error (`code:
  "entity_id_pattern_invalid"`)

### Requirement: A pattern query lists keys by filter

A non-empty `Pattern` SHALL list matching keys via `KVStore.KeysByFilter` and SHALL NOT fall back to an exact-key `Get`
when zero keys match.

#### Scenario: A pattern with no matches skips exact-key fallback

- **WHEN** a caller sends a valid `Pattern` that matches zero keys in `ENTITY_STATES`
- **THEN** graph-ingest returns an empty `Entities` result (`{"entities":null}`) with no `NextCursor` and performs no
  exact-key `Get` call on the bucket

### Requirement: A pattern query pages like a prefix query

A non-empty `Pattern` SHALL apply the same lexicographic key sorting, cursor continuation, limit clamping, and
`max_payload` byte-fitting as literal prefix queries.

#### Scenario: A pattern query pages across sorted matches

- **WHEN** the keys matching `Pattern` exceed `Limit` or the `max_payload` byte budget and the caller pages using each
  response's `NextCursor` until `NextCursor` is empty
- **THEN** the concatenated pages contain the exact sorted set of matching entities with no gaps or duplicates

### Requirement: Mutation replies carry four outcomes

A mutation reply's `outcome` SHALL be one of `applied`, `unchanged`, `entity_not_found` or `failed`. Every other
failure SHALL travel as a classified error, never as an outcome.

#### Scenario: A failure that is not an outcome

- **WHEN** a create request is refused with code `invalid_request`
- **THEN** the refusal travels as a classified error, and the reply carries no outcome for it

### Requirement: Mutation replies are counted by outcome

graph-ingest SHALL count each canonical mutation reply once on `graph_mutation_outcomes_total`, and an append once per
subject result. The `outcome` label SHALL be the reply's outcome, or the failure's classified code for a `failed`
subject or a refused request, or `internal` when the error carries no code (ruling L, #91 comment 6062681355).

#### Scenario: A create with an unregistered message type

- **WHEN** a create request is refused with code `message_type_unregistered`
- **THEN** `graph_mutation_outcomes_total{operation="entity.create",outcome="message_type_unregistered"}` increases
  by one

#### Scenario: A create refused as an invalid request

- **WHEN** a create request with no entity is refused with code `invalid_request`
- **THEN** `graph_mutation_outcomes_total{operation="entity.create",outcome="invalid_request"}` increases by one

#### Scenario: An append to three subjects

- **WHEN** an append applies to one subject, finds a second absent, and leaves a third unchanged
- **THEN** `graph_mutation_outcomes_total{operation="triple.append"}` increases by one under each of `applied`,
  `entity_not_found` and `unchanged`

### Requirement: A refused mutation request is counted and logged

A refused request SHALL also count once on `mutation_rejections_total` and log one warning line; a refusal that carries
detail logs that detail, and an authority refusal logs no entity identity.

#### Scenario: A revision mismatch

- **WHEN** a reconcile request names an expected revision the entity no longer has
- **THEN** graph-ingest logs exactly one warning line, which carries the expected revision, and
  `graph_mutation_outcomes_total{operation="entity.reconcile",outcome="revision_mismatch"}` increases by one

### Requirement: Query refusals carry a code

Once `Start` has completed, every refusal graph-ingest's query handlers return on `graph.ingest.query.entity`,
`graph.ingest.query.batch` and `graph.ingest.query.prefix` SHALL carry a classified code. A request that arrives
before `Start` completes is answered as "graph-ingest does no request work before Start completes" says, with no
code. Giving a refusal a code SHALL NOT change its class.

#### Scenario: A failed key listing

- **WHEN** listing keys for a prefix query fails
- **THEN** graph-ingest refuses it with class transient and code `internal`

### Requirement: Query refusal codes

The codes of query refusals SHALL be: `invalid_request` for a malformed request, entity ID or cursor;
`entity_not_found`; the `pkg/types` entity-ID code for a malformed prefix or pattern; `response_too_large` when no page
fits the server's maximum payload; `graph_state_reset_required` for a refused stored value; and `internal` for any
other failure.

#### Scenario: A malformed batch request

- **WHEN** a batch request is not valid JSON
- **THEN** graph-ingest refuses it with class invalid and code `invalid_request`

### Requirement: An empty batch request gets an empty response

A batch request with no IDs SHALL be answered with an empty `graph.EntityBatchResponse`.

#### Scenario: A batch request with no IDs

- **WHEN** a batch request carries no IDs
- **THEN** graph-ingest answers with an empty `graph.EntityBatchResponse`, not a refusal

### Requirement: One decision on graph-mutation ports

Whether a component port is a graph-mutation port, and whether it has the valid shape, SHALL be decided in
`internal/graphmutation` (`ClassifyPort`), and `internal/flowgraph` and graph-ingest SHALL both use that decision. A
port SHALL be a member when its interface type is `semengine.graph.mutation`, or when any one of its NATS subjects
starts with `graph.mutation.`.

#### Scenario: A port is a member by interface or by subject

- **WHEN** a port's interface type is `semengine.graph.mutation`, or any one of its NATS subjects starts with
  `graph.mutation.`
- **THEN** `ClassifyPort` reports it a member

### Requirement: A graph-mutation port has one valid shape

A member SHALL be valid only when it is a required `nats-request` port whose subjects are exactly `graph.mutation.>` and
whose interface is `semengine.graph.mutation` `v1`.

#### Scenario: The valid provider

- **WHEN** a port is a required `nats-request` port on `graph.mutation.>` with interface `semengine.graph.mutation`
  `v1`
- **THEN** `ClassifyPort` reports it a member and valid, and both callers accept it

### Requirement: Flowgraph refuses an invalid member

Flowgraph SHALL refuse a member that is not valid, on an input or an output, with an error wrapping
`errs.ErrInvalidConfig` that names the component and the port.

#### Scenario: A nats port on a graph.mutation subject

- **WHEN** a flow declares a `nats` output on `graph.mutation.entity.create` beside one valid provider
- **THEN** flowgraph refuses it with `component requester port legacy is not a required nats-request
  semengine.graph.mutation v1 port on graph.mutation.>`

### Requirement: graph-ingest refuses an invalid member input

graph-ingest SHALL refuse a member input that is not valid, in `Config.Validate` and when it builds its mutation routes.
When that input is its first member input, the refusal SHALL name the port; a member input after the first is refused
as ambiguous. Each caller keeps its own refusal texts, direction rule and provider count.

#### Scenario: A JetStream input with one graph.mutation subject

- **WHEN** graph-ingest's configuration declares the valid provider and a JetStream input on stream `EVENTS` whose
  subjects are `events.>` and `graph.mutation.entity.create`
- **THEN** `Config.Validate` refuses it with an invalid-class error containing `graph mutation provider port is
  ambiguous`

#### Scenario: A graph-ingest input with the wrong interface version

- **WHEN** graph-ingest's configuration declares a JetStream input `entity_stream` on `entity.>` and a required
  `nats-request` input `mutations` on `graph.mutation.>` with interface `semengine.graph.mutation` `v2`
- **THEN** `Config.Validate` refuses it with an invalid-class error containing `graph mutation provider port
  "mutations" must be a required semengine.graph.mutation v1 nats-request input on graph.mutation.>`

### Requirement: No stream captures a reserved request subject

`composition.Analyze` SHALL report one `reserved_subject_capture` error for each stream subject filter that shares at
least one concrete subject with a reserved request subject: overlap, not cover. A filter that shares no concrete
subject with a reserved subject SHALL NOT be reported.

#### Scenario: A wide, exact or crossing filter captures a request subject

- **WHEN** an explicit stream declares `graph.>`, or `graph.ingest.query.entity`, or a JetStream output declares
  `*.mutation.create`
- **THEN** each filter yields one `reserved_subject_capture` error naming its stream, the filter, and
  `graph.ingest.query.entity` (the first two) or `graph.mutation.>` (the third)
  (`TestValidateRefusesStreamCapturingReservedSubject`)

#### Scenario: Neighbouring namespaces are not refused

- **WHEN** streams declare `graph.ingest.events.>`, `graph.ingestion.>`, `graph.mutations.created` and
  `graph.ingest.query`
- **THEN** no `reserved_subject_capture` finding is reported (`TestValidateDoesNotRefuseNeighbouringSubjects`)

### Requirement: Stream filters checked for capture

`composition.Analyze` SHALL check these stream subject filters:

- each subject of an explicit `streams` entry, with finding component `config`, the stream named by its key;
- each stream subject of a JetStream output port, with the instance and port as finding component, the stream named
  by the port's stream name or, when it has none, by `config.DeriveStreamName` of its first subject, as provisioning
  names it.

#### Scenario: A JetStream output without a stream name

- **WHEN** a JetStream output port has no stream name and one of its subjects captures a reserved subject
- **THEN** the finding names the stream `config.DeriveStreamName` gives for the port's first subject, and its component
  is the instance and port

### Requirement: Reserved subjects are read from their declarations

The reserved subjects SHALL be the subjects of `graph.QueryVerbs()` and the mutation subject family `graph.mutation.>`,
read from their declarations.

#### Scenario: Reserved subjects follow the declarations

- **WHEN** a stream filter overlaps a subject in `graph.QueryVerbs()` or the family `graph.mutation.>`
- **THEN** `composition.Analyze` reports it, without a second list of subjects in `composition`

### Requirement: A capture finding names the first reserved subject

The finding SHALL name the stream, the filter and the first reserved subject captured: query verbs in table order, then
the mutation family.

#### Scenario: The overlap judgment matches enumeration

- **WHEN** every filter of one to four tokens over a fixed token set with `*` and `>` is checked against each reserved
  subject
- **THEN** a filter is reported exactly when a concrete subject enumerated from a reserved subject matches it, naming
  the first such reserved subject in order (`TestReservedSubjectOverlapMatchesEnumeration`)

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
