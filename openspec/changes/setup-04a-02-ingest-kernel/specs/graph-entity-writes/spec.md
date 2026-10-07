# graph-entity-writes

## ADDED Requirements

### Requirement: One rule per write mode

graph-ingest SHALL change a stored entity through one write path, whatever lane the change arrives on (a stream
message, a mutation request, or a call inside graph-ingest). Each write mode SHALL have one identity rule, the same on
every lane: a create stores a new entity and refuses an existing key; a replace and a conditional replace treat the
statements of one (subject, predicate) as one set and replace that set whole; an append adds a statement only when no
stored statement has the same subject, predicate, datatype, source, context and object; a delete removes the entity
at the caller's expected revision. A conditional replace SHALL report "unchanged" only when the stored statements of
its predicates equal the requested ones in every field. No non-test file of graph-ingest other than the write path's
own SHALL call a write method of the entity bucket.

#### Scenario: Append agrees across lanes

- **WHEN** the same statement is appended through a mutation request and through graph-ingest's in-process append
- **THEN** both leave the same stored statements, and a second append of it on either lane stores nothing

#### Scenario: A replace removes another lane's statements of its predicate

- **WHEN** a statement of predicate P from source A is appended, and then a stream message replaces P with a statement
  from source B
- **THEN** P holds only the stream message's statement, and every other predicate of the entity is unchanged

#### Scenario: A write outside the write path

- **WHEN** a non-test file of graph-ingest other than the write path's calls a write method of the entity bucket
- **THEN** the package's seam test fails naming the file and line

### Requirement: Statement metadata is required

Every statement graph-ingest stores SHALL carry a non-empty `Source` and a non-zero `Timestamp`, and neither SHALL be
taken from the clock because a writer left it out. A write carrying a statement without them SHALL be refused as
`invalid_request`, naming the statement's index and the missing field, and SHALL store nothing. On the stream lane, a
statement that lacks `Source` or `Timestamp` SHALL take the message envelope's source or creation time; a message
whose envelope lacks the needed value SHALL be terminated as poison and counted. Statements graph-ingest derives
itself SHALL name graph-ingest's producer in `Source` and carry the time graph-ingest derived them. `Confidence` and
`Context` SHALL be stored as given and SHALL NOT decide whether a write applies.

#### Scenario: A mutation without a timestamp

- **WHEN** a create, append or reconcile request carries a statement with no `Timestamp`
- **THEN** it is refused as `invalid_request` naming the statement's index and `timestamp`, and nothing is stored

#### Scenario: A stream message without statement metadata

- **WHEN** a `Graphable` payload's statements carry no `Source` or `Timestamp` and its envelope has a source and a
  creation time
- **THEN** the stored statements carry the envelope's source and creation time

#### Scenario: Confidence does not order

- **WHEN** a stream message replaces predicate P with a newer statement of lower confidence, and a later message
  carries an older statement of P with higher confidence
- **THEN** P holds the newer statement

### Requirement: Timestamp orders a replace

On the stream lane, for each predicate an arriving message carries, graph-ingest SHALL replace the stored statements
of that predicate only when the latest `Timestamp` among the message's statements of it is not older than the latest
`Timestamp` among the stored ones; equal timestamps SHALL apply in arrival order. A predicate not applied SHALL keep
its stored statements, the message's other predicates SHALL still apply, and each predicate not applied SHALL be
counted and logged with the entity and predicate. The entity's message type and storage reference SHALL take the
message's values only when none of its predicates was skipped. A conditional replace SHALL be ordered by its expected
revision, not by `Timestamp`.

#### Scenario: An older arrival

- **WHEN** the stored statements of P carry timestamp T and a message carries P at a time before T together with a
  newer predicate Q
- **THEN** P keeps its stored statements, Q is applied, and the count of predicates not applied rises by one

#### Scenario: Equal timestamps

- **WHEN** a message carries P at the same timestamp as the stored statements of P
- **THEN** the message's statements of P replace the stored ones

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

With hierarchy inference enabled, an entity born on any lane SHALL be stored with its hierarchy statements or not at
all. When the inference fails for any part (a container, a forward edge, an inverse edge, the sibling pass),
graph-ingest SHALL store nothing for the entity and return the failure classified as transient; on the stream lane
the message SHALL not be acknowledged, so it is delivered again.

#### Scenario: The inference fails once

- **WHEN** hierarchy inference fails while an entity is born from a stream message, and succeeds on the redelivery
- **THEN** the entity is absent after the first attempt and is stored with its container edges after the second

#### Scenario: Birth on the mutation lane

- **WHEN** hierarchy inference is enabled and an entity is created through a mutation request
- **THEN** the stored entity carries its container edges
