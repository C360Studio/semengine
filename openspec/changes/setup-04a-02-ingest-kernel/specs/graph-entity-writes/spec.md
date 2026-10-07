# graph-entity-writes

## ADDED Requirements

### Requirement: One rule per write mode

graph-ingest SHALL change a stored entity through one write path, whatever lane the change arrives on (a stream
message, a mutation request, or a call inside graph-ingest). Each write mode SHALL have one identity rule, the same on
every lane: a create stores a new entity and refuses an existing key; a replace and a conditional replace treat the
statements of one (subject, predicate, source) as one set and replace that set whole, keeping the statements of the
same predicate from every other source; an append adds a statement only when no stored statement has the same
subject, predicate, datatype, source, context and object; a delete removes the entity at the caller's expected
revision. A conditional replace SHALL name one source, a request without one SHALL be refused as `invalid_request`
and store nothing, every statement it carries SHALL have that source, and an empty
set SHALL clear only that source's statements of the named predicates. A conditional replace SHALL report "unchanged"
only when the stored statements of its predicates from its source equal the requested ones in every field. No non-test
file of graph-ingest other than the write path's own SHALL call a write method of the entity bucket.

#### Scenario: Append agrees across lanes

- **WHEN** the same statement is appended through a mutation request and through graph-ingest's in-process append
- **THEN** both leave the same stored statements, and a second append of it on either lane stores nothing

#### Scenario: Each source replaces only its own statements

- **WHEN** a statement of predicate P from source A is appended, and then a stream message replaces P with a statement
  from source B
- **THEN** P holds both statements, and a later stream message replacing P from source B leaves source A's statement
  in place

#### Scenario: A conditional replace with an empty set

- **WHEN** predicate P holds statements from sources A and B, and a conditional replace from source A names P with no
  statements
- **THEN** P holds only source B's statements

#### Scenario: A conditional replace without a source

- **WHEN** a reconcile request on the wire carries no `source`
- **THEN** it is refused as `invalid_request` naming `source`, and nothing is stored

#### Scenario: A conditional replace with a statement from another source

- **WHEN** a conditional replace from source A carries a statement whose source is B
- **THEN** it is refused as `invalid_request` naming the statement's index, and nothing is stored

#### Scenario: A write outside the write path

- **WHEN** a non-test file of graph-ingest other than the write path's calls a write method of the entity bucket
- **THEN** the package's seam test fails naming the file and line

### Requirement: A single-value read picks one statement the same way every time

When a predicate holds more than one statement, `EntityState.Triples` SHALL return all of them, and the single-value
reads of `graph` (`EntityState.GetTriple`, `EntityState.GetPropertyValue`) SHALL return the statement with the latest
`Timestamp`; on equal timestamps, the one whose `Source` sorts first; on equal sources, the first stored.

#### Scenario: Two sources hold one predicate

- **WHEN** P holds a statement from source B at time T1 and one from source A at a later time T2
- **THEN** the single-value read of P returns source A's statement, whichever was stored first

### Requirement: Statement metadata is required

Every statement graph-ingest stores SHALL carry a non-empty `Source` and a non-zero `Timestamp`, and neither SHALL be
taken from the clock by graph-ingest, or as a default for a value the writer left out; a writer that originates a
statement MAY assert its own time. A write carrying a statement without them SHALL be refused as `invalid_request`,
naming the statement's index and the missing field, and SHALL store nothing. On the stream lane, a statement that lacks
`Source` or `Timestamp` SHALL take the message envelope's source or creation time before its replace set is formed; a
message whose envelope lacks the needed value SHALL be terminated as poison and counted. The stream lane SHALL refuse a
message any of whose statements, once stamped, carries a reserved source, the names graph-ingest and `pkg/lifecycle`
write under (`graph-ingest-indexing-profile`, `graph-ingest-hierarchy` and `semengine-lifecycle`): the message SHALL be
terminated as poison with an `invalid_request`-class error naming the statement's index and the source, counted on
`predicate_contract_rejections_total` with `reason="reserved_source"`, and nothing of it SHALL be stored. The mutation
lane accepts the reserved sources, since `pkg/lifecycle` writes there under its own. Statements graph-ingest derives
itself (the indexing profile, hierarchy statements, a hierarchy container's statements) SHALL name graph-ingest's
producer in `Source` and SHALL carry the triggering message's time: on the stream lane the envelope's creation time, on
the mutation and in-process lanes the latest `Timestamp` among the write's own statements. A create on the mutation or
in-process lane that carries no statement SHALL be refused as `invalid_request`. `Confidence` and `Context` SHALL be
stored as given and SHALL NOT decide whether a write applies.

#### Scenario: A mutation without a timestamp

- **WHEN** a create, append or reconcile request carries a statement with no `Timestamp`
- **THEN** it is refused as `invalid_request` naming the statement's index and `timestamp`, and nothing is stored

#### Scenario: A stream message without statement metadata

- **WHEN** a `Graphable` payload's statements carry no `Source` or `Timestamp` and its envelope has a source and a
  creation time
- **THEN** the stored statements carry the envelope's source and creation time

#### Scenario: Derived statements take the triggering time

- **WHEN** an entity is born from a stream message whose envelope was created at time T, with hierarchy inference
  enabled
- **THEN** its indexing-profile and hierarchy statements, and the statements of any container created for it, carry
  timestamp T and graph-ingest's producer as source

#### Scenario: A stream message names a reserved source

- **WHEN** a `Graphable` payload carries a statement whose `Source` is `graph-ingest-hierarchy`, or carries none and
  its envelope's source is `semengine-lifecycle`
- **THEN** the message is terminated as poison with an `invalid_request`-class error naming the statement's index and
  the source, the refusal is counted, and nothing of the message is stored

#### Scenario: Confidence does not order

- **WHEN** a stream message replaces predicate P with a newer statement of lower confidence, and a later message from
  the same source carries an older statement of P with higher confidence
- **THEN** P holds the newer statement

### Requirement: Timestamp orders a replace

On the stream lane, for each (predicate, source) set an arriving message carries, graph-ingest SHALL replace the
stored statements of that predicate from that source only when the latest `Timestamp` in the message's set is not
older than the latest `Timestamp` among those stored statements; equal timestamps SHALL apply in arrival order. A set
not applied SHALL leave its stored statements as they are, the message's other sets SHALL still apply, and each set
not applied SHALL be counted and logged with the entity, predicate and source. The entity's message type and storage
reference SHALL take the message's values only when none of its sets was skipped. A conditional replace SHALL be
fenced by its expected revision, not ordered by `Timestamp`.

#### Scenario: An older arrival

- **WHEN** the stored statements of P from source A carry timestamp T, and a message from source A carries P at a time
  before T together with a newer predicate Q
- **THEN** P keeps its stored statements, Q is applied, and the count of sets not applied rises by one

#### Scenario: Another source's older statement

- **WHEN** the stored statements of P from source A carry timestamp T, and a message from source B carries P at a time
  before T
- **THEN** source B's statement of P is stored beside source A's, and nothing is counted as not applied

#### Scenario: Equal timestamps

- **WHEN** a message carries P from source A at the same timestamp as the stored statements of P from source A
- **THEN** the message's statements replace them

### Requirement: The revision is the only fence

A stored entity SHALL carry no version number of its own; the key-value revision SHALL be the only concurrency
fence and the only revision a reply reports. graph-ingest SHALL still read a stored value that carries a `version`
key.

#### Scenario: No version in a stored entity

- **WHEN** graph-ingest stores an entity
- **THEN** the stored JSON has no `version` key, and a stored value that has one decodes

### Requirement: One stored revision per entity

The entity bucket SHALL keep one revision per key (`History` 1). SemEngine SHALL offer no point-in-time read of an
entity's state: an entity's history is its input stream. This constraint changes only with a design for a named
consumer need.

#### Scenario: The bucket's descriptor

- **WHEN** the catalog descriptor of `ENTITY_STATES` is read
- **THEN** it declares `History` 1

### Requirement: Birth with hierarchy fails closed

With hierarchy inference enabled, an entity born on a lane that infers hierarchy (the stream lane and the in-process
lane) SHALL be stored with its hierarchy statements or not at all. When the inference fails for any part (a
container, a forward edge, an inverse edge, the sibling pass), graph-ingest SHALL store nothing for the entity and
return the failure classified as transient; on the stream lane the message SHALL not be acknowledged, so it is
delivered again. An entity created through a mutation request SHALL get no hierarchy statements.

#### Scenario: The inference fails once

- **WHEN** hierarchy inference fails while an entity is born from a stream message, and succeeds on the redelivery
- **THEN** the entity is absent after the first attempt and is stored with its container edges after the second

#### Scenario: Birth on the mutation lane

- **WHEN** hierarchy inference is enabled and an entity is created through a mutation request
- **THEN** the stored entity carries the request's statements and its indexing profile, and no hierarchy statement
