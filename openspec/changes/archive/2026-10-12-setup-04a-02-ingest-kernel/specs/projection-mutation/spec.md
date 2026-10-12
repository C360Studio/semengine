# projection-mutation

## ADDED Requirements

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
- **THEN** the client returns an invalid-class error, the receipt reports not-committed, and no request reaches the
  broker

#### Scenario: Unchanged revision

- **WHEN** the caller reconciles at the current revision R
- **THEN** the mutation is applied and the receipt reports it verified at the new revision

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
- **THEN** the typed client's receipt reports commit-unknown, never not-committed

#### Scenario: Revision conflict

- **WHEN** the entity bucket refuses the update for a revision mismatch
- **THEN** the receipt reports not-committed with the conflict classification

#### Scenario: An append's write stands and the entity then changes

- **WHEN** an append's update is stored but returns an error, and another writer then deletes the entity or stores a
  value graph-ingest refuses
- **THEN** the typed client's receipt reports commit-unknown, never not-committed

### Requirement: The typed client never reads the clock

`projection.MutationClient` SHALL send a statement's `Timestamp` only as the caller gave it, on the statement or in the
request's metadata. A `Create`, `Append` or `Reconcile` with a statement that has no `Timestamp` and metadata with none
SHALL be refused as invalid before any request is sent, and its receipt SHALL report not-committed.

#### Scenario: No timestamp anywhere

- **WHEN** an `Append` carries a statement with no `Timestamp` and its metadata's `Timestamp` is zero
- **THEN** the client returns an invalid-class error, the receipt reports not-committed, and no request reaches the
  broker
