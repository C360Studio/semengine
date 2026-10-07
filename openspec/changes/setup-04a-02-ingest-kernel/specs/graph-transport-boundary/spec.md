# graph-transport-boundary

## ADDED Requirements

### Requirement: Reserved request subjects have one declaration

The request/reply verbs graph-ingest serves (`graph.ingest.query.entity`, `.batch`, `.prefix`) SHALL be declared once,
in a verb table in the `graph` package whose entries name each verb's subject, responder, request type and reply type.
The mutation subject family (`graph.mutation.>`) SHALL be declared once, in `internal/graphmutation`. Every server and
client in the module SHALL take these subjects from their declarations. A contract test run by `task test:unit` SHALL
fail, naming the file and line, when a non-test Go file other than the two declaring files spells one of these
subjects, or the `graph.mutation.` prefix, as a string literal; a sensitivity test SHALL plant such a literal in a
temporary module and require the failure.

#### Scenario: A literal spelling is added

- **WHEN** a non-test file outside the declarations spells `graph.ingest.query.entity` as a string literal
- **THEN** the contract test fails naming the file and line

#### Scenario: The declarations and tests are not reported

- **WHEN** the subjects appear in a declaring file, in a `_test.go` file, or in a comment
- **THEN** the contract test reports nothing for them

### Requirement: The responder serves exactly its declared verbs

graph-ingest SHALL subscribe to the subjects of the verb table's entries whose responder is graph-ingest, and to no
other request subject.

#### Scenario: Subscriptions match the table

- **WHEN** graph-ingest has started and its test adapter lists its request subscriptions
- **THEN** the listed subjects equal the table's subjects for responder graph-ingest

### Requirement: The graph root imports no transport

The `graph` package SHALL import no NATS client package, directly or through its dependencies. A contract test run by
`task test:unit` SHALL fail, naming the package path, when `go list -deps ./graph` lists `natsclient` or a
`github.com/nats-io` path; a sensitivity test SHALL require the failure for a temporary module whose package reaches
such a path.

#### Scenario: A transport import reaches the root

- **WHEN** a file of the `graph` package imports a package that imports `natsclient`
- **THEN** the contract test fails naming the path it found

### Requirement: One reply envelope for the graph query family

A reply on the `graph.query.*` family SHALL be `graph.QueryResponse`, whose JSON carries `data`, `indexed_revision`
(the entity-bucket revision the answer reflects), `producer` (the responding component instance) and `timestamp`;
building one SHALL require the producer and the revision. A request on that family MAY carry `min_revision`,
declared once in `graph`. The module SHALL provide no function that decides from a reply's content whether it is
enveloped. Replies on `graph.ingest.query.*` are authority reads and are not enveloped; the entity verb's reply
carries the key-value revision it read.

#### Scenario: Envelope fields

- **WHEN** a response is built for producer P at revision R and encoded
- **THEN** its JSON has `data`, `indexed_revision` equal to R, `producer` equal to P and `timestamp`, and it decodes
  back equal

### Requirement: The readiness envelope carries its publish time and no legacy fields

`graph.IndexStatusResponse`, the value a producer writes to its `GRAPH_STATUS` key, SHALL carry `published_at`, the
UTC time in RFC 3339 form at which the producer's publisher wrote it, set by the publisher on every write whatever the
caller passed. It SHALL carry no `phase`, `revision` or `last_synced` field; `indexed_revision` is the one spelling of
the revision.

#### Scenario: A published envelope

- **WHEN** a producer publishes its status between times T1 and T2
- **THEN** the stored JSON has `published_at` at or after T1 and at or before T2, and no `phase`, `revision` or
  `last_synced` key
