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

The request/reply verbs graph-ingest serves (`graph.ingest.query.entity`, `.batch`, `.prefix`) SHALL be declared once,
in a verb table in the `graph` package whose entries name each verb's name, subject and responder. The entity and batch
requests SHALL be Go types declared in `graph` (`EntityRequest`, `EntityBatchRequest`), used by the caller and the
responder alike. The mutation subject family (`graph.mutation.>`) SHALL be declared once, in `internal/graphmutation`.
Every server and client in the module SHALL take these subjects from their declarations. A contract test run by `task
test:unit` SHALL fail, naming the file and line, when a non-test Go file other than the two declaring files spells one
of these subjects, or the `graph.mutation.` prefix, as a string literal; a sensitivity test SHALL plant such a literal
in a temporary module and require the failure.

#### Scenario: A literal spelling is added

- **WHEN** a non-test file outside the declarations spells `graph.ingest.query.entity` as a string literal
- **THEN** the contract test fails naming the file and line

#### Scenario: The declarations and tests are not reported

- **WHEN** the subjects appear in a declaring file, in a `_test.go` file, or in a comment
- **THEN** the contract test reports nothing for them

### Requirement: The responder serves exactly its declared verbs

graph-ingest SHALL subscribe to the subjects of the verb table's entries whose responder is graph-ingest, and to no
other request subject. When `Start` returns, the server SHALL route each of those subjects to graph-ingest from any
connection (ruled on #144).

#### Scenario: Subscriptions match the table

- **WHEN** graph-ingest has started and its test adapter lists its request subscriptions
- **THEN** the listed subjects equal the table's subjects for responder graph-ingest

#### Scenario: Routable when Start returns

- **WHEN** graph-ingest's `Start` has returned and another connection sends a request on each listed subject
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
wire representation. Setting both `Prefix` and `Pattern` on the same request SHALL be rejected before any KV operation
with a classified invalid error (`errs.ErrorInvalid`, code `invalid_request`). A non-empty `Pattern` SHALL be validated
before any KV operation via `pkg/types.ValidateEntityIDPattern` (failing with `errs.ErrorInvalid`, code
`entity_id_pattern_invalid` when malformed), SHALL list matching keys via `KVStore.KeysByFilter`, SHALL NOT fall back to
an exact-key `Get` when zero keys match, and SHALL apply the same lexicographic key sorting, cursor continuation, limit
clamping, and `max_payload` byte-fitting as literal prefix queries.

#### Scenario: A six-position wildcard pattern matches entities across systems

- **WHEN** `ENTITY_STATES` holds entities under multiple `system` segments and a caller requests
  `graph.PrefixQueryRequest{Pattern: "o.p.*.intel.platform.*"}` on `graph.ingest.query.prefix`
- **THEN** graph-ingest returns only the entities matching `o.p.*.intel.platform.*` in lexicographic ID order

#### Scenario: Prefix and Pattern are mutually exclusive

- **WHEN** a caller sends a `graph.PrefixQueryRequest` with both `Prefix` and `Pattern` non-empty
- **THEN** graph-ingest rejects the request before any KV read with a classified invalid error (`code:
  "invalid_request"`)

#### Scenario: An invalid pattern is rejected before any KV read

- **WHEN** a caller sends a `graph.PrefixQueryRequest` whose `Pattern` has fewer or more than six positions, contains
  `>`, or contains a malformed segment
- **THEN** graph-ingest rejects the request before any KV read with a classified invalid error (`code:
  "entity_id_pattern_invalid"`)

#### Scenario: A pattern query pages across sorted matches

- **WHEN** the keys matching `Pattern` exceed `Limit` or the `max_payload` byte budget and the caller pages using each
  response's `NextCursor` until `NextCursor` is empty
- **THEN** the concatenated pages contain the exact sorted set of matching entities with no gaps or duplicates

#### Scenario: A pattern with no matches skips exact-key fallback

- **WHEN** a caller sends a valid `Pattern` that matches zero keys in `ENTITY_STATES`
- **THEN** graph-ingest returns an empty `Entities` result (`{"entities":null}`) with no `NextCursor` and performs no
  exact-key `Get` call on the bucket

### Requirement: Mutation replies carry four outcomes

A mutation reply's `outcome` SHALL be one of `applied`, `unchanged`, `entity_not_found` or `failed`. Every other
failure SHALL travel as a classified error, never as an outcome. graph-ingest SHALL count each canonical mutation
reply once on `graph_mutation_outcomes_total`, and an append once per subject result. The `outcome` label SHALL be
the reply's outcome, or the failure's classified code for a `failed` subject or a refused request, or `internal` when
the error carries no code (ruling L, #91 comment 6062681355). A refused request SHALL also count once on
`mutation_rejections_total` and log one warning line; a refusal that carries detail logs that detail, and an
authority refusal logs no entity identity.

#### Scenario: A create with an unregistered message type

- **WHEN** a create request is refused with code `message_type_unregistered`
- **THEN** `graph_mutation_outcomes_total{operation="entity.create",outcome="message_type_unregistered"}` increases
  by one

#### Scenario: A create refused as an invalid request

- **WHEN** a create request with no entity is refused with code `invalid_request`
- **THEN** `graph_mutation_outcomes_total{operation="entity.create",outcome="invalid_request"}` increases by one

#### Scenario: A revision mismatch

- **WHEN** a reconcile request names an expected revision the entity no longer has
- **THEN** graph-ingest logs exactly one warning line, which carries the expected revision, and
  `graph_mutation_outcomes_total{operation="entity.reconcile",outcome="revision_mismatch"}` increases by one

#### Scenario: An append to three subjects

- **WHEN** an append applies to one subject, finds a second absent, and leaves a third unchanged
- **THEN** `graph_mutation_outcomes_total{operation="triple.append"}` increases by one under each of `applied`,
  `entity_not_found` and `unchanged`

### Requirement: Query refusals carry a code

Once `Start` has completed, every refusal graph-ingest's query handlers return on `graph.ingest.query.entity`,
`graph.ingest.query.batch` and `graph.ingest.query.prefix` SHALL carry a classified code. A request that arrives
before `Start` completes is answered as "graph-ingest does no request work before Start completes" says, with no
code. The codes: `invalid_request` for a malformed request, entity ID or cursor; `entity_not_found`; the `pkg/types`
entity-ID code for a malformed prefix or pattern; `response_too_large` when no page fits the server's maximum
payload; `graph_state_reset_required` for a refused stored value; and `internal` for any other failure. Giving a
refusal a code SHALL NOT change its class. A batch request with no IDs SHALL be answered with an empty
`graph.EntityBatchResponse`.

#### Scenario: A malformed batch request

- **WHEN** a batch request is not valid JSON
- **THEN** graph-ingest refuses it with class invalid and code `invalid_request`

#### Scenario: A failed key listing

- **WHEN** listing keys for a prefix query fails
- **THEN** graph-ingest refuses it with class transient and code `internal`

### Requirement: One decision on graph-mutation ports

Whether a component port is a graph-mutation port, and whether it has the valid shape, SHALL be decided in
`internal/graphmutation` (`ClassifyPort`), and `internal/flowgraph` and graph-ingest SHALL both use that decision. A
port SHALL be a member when its interface type is `semengine.graph.mutation`, or when any one of its NATS subjects
starts with `graph.mutation.`. A member SHALL be valid only when it is a required `nats-request` port whose subjects
are exactly `graph.mutation.>` and whose interface is `semengine.graph.mutation` `v1`. Flowgraph SHALL refuse a
member that is not valid, on an input or an output, with an error wrapping `errs.ErrInvalidConfig` that names the
component and the port. graph-ingest SHALL refuse a member input that is not valid, in `Config.Validate` and when it
builds its mutation routes. Each caller keeps its own refusal texts, direction rule and provider count.

#### Scenario: A nats port on a graph.mutation subject

- **WHEN** a flow declares a `nats` output on `graph.mutation.entity.create` beside one valid provider
- **THEN** flowgraph refuses it with `component requester port legacy is not a required nats-request
  semengine.graph.mutation v1 port on graph.mutation.>`

#### Scenario: A JetStream input with one graph.mutation subject

- **WHEN** graph-ingest's configuration declares the valid provider and a JetStream input on stream `EVENTS` whose
  subjects are `events.>` and `graph.mutation.entity.create`
- **THEN** `Config.Validate` refuses it with an invalid-class error containing `graph mutation provider port is
  ambiguous`

#### Scenario: The valid provider

- **WHEN** a port is a required `nats-request` port on `graph.mutation.>` with interface `semengine.graph.mutation`
  `v1`
- **THEN** `ClassifyPort` reports it a member and valid, and both callers accept it

