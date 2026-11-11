# graph-gateway

## ADDED Requirements

### Requirement: The gateway answers declared read operations through the verb table

The graph gateway SHALL answer the GraphQL root fields `entity`, `entitiesByPrefix`, `entitiesByPredicate` and
`pathSearch` by requesting the `graph.query.*` verbs `entity`, `prefix`, `predicate` and `pathSearch`. It SHALL take
each subject from `graph.QueryVerbs()` and SHALL write nothing to the graph. Its schema SHALL declare no mutation or
subscription. Remote reads of the graph use these operations (pin ADR-090, decision 6).

#### Scenario: The four reads answer from a real stack

- **WHEN** graph-ingest, graph-index, graph-query and the gateway run on one NATS server with seeded entities, and a
  client posts each of the four operations
- **THEN** each answer holds the seeded entity IDs and triples asked for (`TestGatewayAnswersFourOperations`)

#### Scenario: A mutation is refused

- **WHEN** a client posts `mutation { entity(id: "x") { kvRevision } }`
- **THEN** the answer is HTTP 400 with an `errors` entry and no request reaches NATS
  (`TestMutationAndIntrospectionRefused`)

### Requirement: Requests are parsed and validated against the schema

The gateway SHALL parse each request and validate it against its schema before sending any request. It SHALL refuse,
with HTTP 400 and an error naming the field or argument, a document that does not parse, an unknown field or argument,
a variable of the wrong type, more than one root field other than `__typename`, and `__schema` or `__type`. It SHALL
NOT choose an operation by searching the query text.

#### Scenario: A field the schema does not declare

- **WHEN** a client asks `pathSearch(startEntity: "a", direction: "outgoing", maxDepth: 1) { edges { from } }`
- **THEN** the answer is HTTP 400 naming `edges` (`TestSchemaRefusesUnknownFieldNamingIt`)

#### Scenario: A name inside an argument does not route

- **WHEN** a client asks `entity(id: "x.y.pathsearch.trajectory.a.b") { kvRevision }`
- **THEN** the request goes to the `entity` verb only (`TestGatewayAnswersFourOperations`)

### Requirement: An answer holds exactly the selected fields

The gateway SHALL answer each root field with the fields its selection set names, under their aliases, through
fragments, with `__typename` answered from the schema. It SHALL return no other field. An answer holds exactly the
selected names, at every level, for every selection and every reply.

#### Scenario: A narrow selection

- **WHEN** a client asks `entity(id: $id) { kvRevision }` for a stored entity
- **THEN** `data.entity` has the one key `kvRevision` (`TestProjectionHoldsExactlySelectedNames`)

### Requirement: Answers follow the gateway's schema and are checked

Every root field SHALL be nullable. `entity` SHALL answer `{entity {id triples}, kvRevision}` from a stored value
decoded by `graph.UnmarshalEntityState`, refusing a missing entity or a zero KV revision (pin ADR-091, §4).
`entitiesByPrefix` SHALL answer `{entities, total, next_cursor}` after `graph.ValidateDecodedEntityStates`, an empty
`prefix` matching every entity. `entitiesByPredicate` SHALL answer `{entities}`.

#### Scenario: A reply that does not fit

- **WHEN** a verb's reply lacks a field the schema declares non-null, or the `entity` reply carries `kv_revision` 0
- **THEN** that root field is `null`, the rest of `data` stands, the error names its path, and no partial value is
  returned (`TestReplyNotFittingSchemaNamesPath`, `TestEntityZeroRevisionRefused`)

#### Scenario: A prefix page holding an invalid entity

- **WHEN** a `prefix` reply holds an entity that `graph.ValidateDecodedEntityStates` refuses
- **THEN** `data.entitiesByPrefix` is `null` with an error naming the path, HTTP 200 (`TestPrefixReplyValidated`)

### Requirement: Path answers name each edge as the graph holds it

`pathSearch` SHALL answer `entities` as `{id, depth}`, `truncated`, and `paths`, where `paths[k]` is the run of edges
from the start to `entities[k]` and `paths[0]` is empty; each step names the edge as the graph holds it. `direction`
SHALL be accepted in any letter case. `predicates` SHALL accept `[String]`, refusing a null item with HTTP 400.

#### Scenario: A path through an incoming edge

- **WHEN** the graph holds B -p-> A and a client asks `pathSearch` from A with `direction: "INCOMING"`, `maxDepth: 1`
- **THEN** `entities` is A at 0 and B at 1, and `paths` is `[[], [{from: B, predicate: p, to: A}]]`
  (`TestPathsFollowEdgesBack`, `TestDirectionIsCaseInsensitive`)

### Requirement: Refusals keep their class and code

A classified refusal SHALL answer HTTP 200 with the root field `null` and `errors[0].extensions` carrying `class` and
`code` (ADR-060). A request with no responder SHALL answer HTTP 500 with a transient error naming the subject. A request
past `query_timeout_ms` SHALL answer HTTP 504. A method other than POST SHALL answer HTTP 405.

#### Scenario: An entity that does not exist

- **WHEN** a client asks `entity` for an ID with no record
- **THEN** the answer is HTTP 200, `data.entity` is `null`, and the error carries the not-found class and code
  (`TestClassifiedRefusalCarriesClassAndCode`)

#### Scenario: graph-query is not composed

- **WHEN** nothing serves `graph.query.entity`
- **THEN** the answer is HTTP 500 naming `graph.query.entity` (`TestNoResponderNamesSubject`)

### Requirement: Requests are served only while running

The gateway SHALL answer HTTP 503 to every request before `Start` and once `Stop` begins, admitting requests through
`internal/httpadmission.Requests`. `Stop` SHALL return once no admitted request is running, or once its context ends,
with that error.

#### Scenario: A request in flight at Stop

- **WHEN** a request is waiting on its verb and `Stop` is called
- **THEN** `Stop` returns after that request has answered, and a later request gets 503
  (`TestStopWaitsForInFlightRequest`, `TestRefusedOutsideStartStop`)

### Requirement: Configuration is strict and read

The gateway SHALL accept `ports`, with exactly one required `graph_queries` NATS-request output on `graph.query.*`,
interface `graph.query v1`, and `query_timeout_ms` (default 60000). It SHALL refuse at construction any other key,
naming it, and any other port. It SHALL be mounted at
`/<instance>/graphql` by the service manager.

#### Scenario: A pin key

- **WHEN** a config holds `enable_playground`
- **THEN** construction fails naming `enable_playground` (`TestConfigRefusesRemovedKeys`)

#### Scenario: The timeout is read

- **WHEN** `query_timeout_ms` is 50 and the verb never answers
- **THEN** the answer is HTTP 504 (`TestQueryTimeoutIsRead`)
