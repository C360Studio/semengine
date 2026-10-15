# projection-mutation

## ADDED Requirements

### Requirement: One mapping from a mutation request's error to its commit state

`projection.ClassifyMutationError(operation, err)` SHALL return nil for a nil error. Otherwise it SHALL return a
`*projection.MutationError` with the given `Operation`, and every `projection.MutationClient` write SHALL report its
request's failure through it. The error SHALL be not committed when its chain holds a classified reply whose code
graph-ingest sends only before writing (invalid request, structural invalid, message type unregistered, entity ID
invalid, entity ID authority invalid, revision mismatch, entity exists, entity not found, graph state reset required),
with that reply's class, code and detail. It SHALL be not committed, `Kind` `MutationUnavailable`, class transient,
when no responder took the request or the context ended before the request was sent. Every other error, including
natsclient's refusals before sending until #268 is fixed, SHALL be commit-unknown as the requirement "A commit-unknown
error is never retryable" states.

#### Scenario: Context ended before the request was sent

- **WHEN** a typed-client Create, Append or Delete is called with a context that is already cancelled
- **THEN** no request reaches the broker, the error is not committed with class transient, and
  `errors.Is(err, context.Canceled)` reports true

#### Scenario: A reply that does not prove nothing was stored

- **WHEN** the mapping is given graph-ingest's transient `internal` reply
- **THEN** it returns a commit-unknown error with class fatal and code `internal`

### Requirement: The commit state is reported on the error

A `projection.MutationClient` method that fails SHALL return a zero `MutationReceipt` and a `*projection.MutationError`
whose `Commit` reports what is proven. A method that returns no error SHALL have committed the mutation, and its
receipt SHALL hold the resulting revision.

#### Scenario: Refused before sending

- **WHEN** a `Create` is refused for a missing request ID
- **THEN** the error is invalid and not committed, the receipt is zero, and no request reaches the broker

## MODIFIED Requirements

### Requirement: Conditional reconcile at a caller-observed revision

`projection.ReconcileMutation` SHALL carry an expected revision, sent through the typed `MutationClient`. When it is
set, graph-ingest SHALL apply the mutation only if that revision is the entity's current revision, and otherwise
SHALL return a revision-conflict error and SHALL change nothing. The error SHALL name the expected revision, and
SHALL also name the current revision when graph-ingest's own read of the entity found a different one; when the
bucket refuses the write because another write landed after that read, the error SHALL name only the expected
revision, and graph-ingest SHALL NOT read the entity again to learn the current one. The request SHALL name its
source, taken from the mutation's metadata, which `Reconcile` requires as `Create` and `Append` do; the reconcile
replaces only that source's statements of the named predicates.

#### Scenario: Concurrent update between read and reconcile

- **WHEN** the caller read revision R, another writer committed R+1, and the caller reconciles at R
- **THEN** the result is a revision conflict naming R and R+1, and the entity is unchanged at R+1

#### Scenario: Reconcile without a source

- **WHEN** a `ReconcileMutation`'s metadata has no `Source`
- **THEN** the client returns an invalid-class error that reports not-committed, and no request reaches the broker

#### Scenario: A failed read is not committed

- **WHEN** `Reconcile`'s own read of the entity fails, with a classified reply or with an unclassified error such as
  a timeout or a client that is not connected
- **THEN** the error reports not-committed with operation `read-authoritative`, and no mutation request is sent

#### Scenario: Unchanged revision

- **WHEN** the caller reconciles at the current revision R
- **THEN** the mutation is applied, no error is returned, and the receipt holds the new revision

### Requirement: Commit ambiguity is preserved

graph-ingest SHALL classify a mutation failure as not committed only when the failure is proven to precede any storage
effect: an invalid request, a revision conflict, an entity not found, or a stored value refused as graph state
(`graph_state_reset_required`, raised before the write). Every other failure, including a key-value write whose outcome
the server did not confirm, SHALL reach the caller of the typed client as commit-unknown.

One exception is a declared limitation (owner ruling, #91 comment 6113959050). A create refused with
`entity_already_exists` is reported not committed, even though nats.go's own `Create` may have stored an earlier write
in the same call. That happens when the first write was stored but its acknowledgement was malformed, and other writers
then deleted the key and created it again: nats.go writes a second time when its first write fails and the key reads
as deleted (nats.go v1.54.0 `jetstream/kv.go:1072-1079`), and that second write is refused because the key exists.

#### Scenario: Backend write timeout

- **WHEN** the entity bucket's update reaches the server and its reply is lost or times out
- **THEN** the typed client's error reports commit-unknown, never not-committed

#### Scenario: Revision conflict

- **WHEN** the entity bucket refuses the update for a revision mismatch
- **THEN** the error reports not-committed with the conflict classification

#### Scenario: An append's write stands and the entity then changes

- **WHEN** an append's update is stored but returns an error, and another writer then deletes the entity or stores a
  value graph-ingest refuses
- **THEN** the typed client's error reports commit-unknown, never not-committed

### Requirement: The typed client never reads the clock

`projection.MutationClient` SHALL send a statement's `Timestamp` only as the caller gave it, on the statement or in the
request's metadata. A `Create`, `Append` or `Reconcile` with a statement that has no `Timestamp` and metadata with none
SHALL be refused as invalid before any request is sent, and its error SHALL report not-committed.

#### Scenario: No timestamp anywhere

- **WHEN** an `Append` carries a statement with no `Timestamp` and its metadata's `Timestamp` is zero
- **THEN** the client returns an invalid-class error that reports not-committed, and no request reaches the broker
