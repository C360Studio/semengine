# projection-mutation Specification

## Purpose
`projection.MutationClient` (`pkg/projection`) is the typed client a caller uses to ask graph-ingest to change an
entity over NATS. This capability states that a reconcile (replacing one source's statements of named predicates)
can be made conditional on the entity revision the caller observed; that a failure is reported as not committed only
when it is proven to precede any storage effect, every other failure reaching the caller as commit-unknown (the
write may or may not have been stored), with one declared limitation; and that the client sends only the timestamps
the caller gave and never reads the clock.

## Requirements

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

### Requirement: A commit-unknown error is never retryable

A `*projection.MutationError` that reports commit-unknown (`Commit` is `CommitUnknown`, `Kind` is
`MutationCommitUnknown`), whether `projection.MutationClient` or `pkg/lifecycle`'s graph emitter returns it, SHALL
carry `Class` `errs.ErrorFatal` whatever error it wraps. For that error, and for any error that wraps it,
`errs.IsTransient` SHALL report false, `errs.IsFatal` SHALL report true, and `errs.Classify` SHALL return
`errs.ErrorFatal`. `errors.As` to `*errs.ClassifiedError` SHALL yield class fatal with the reply's `Code`, and with the
reply's `Detail` when that `Code` is not empty (a reply with no code passes no detail on). `errors.Is`, and
`errors.As` to any other type, SHALL still reach the error it wraps, and the `MutationError`'s `Code` and `Detail`
SHALL be the reply's.

#### Scenario: Deadline after the request was sent

- **WHEN** a lifecycle emit's mutation request is sent and fails with `context.DeadlineExceeded` before any reply
- **THEN** the error is commit-unknown with class fatal, `errs.IsTransient` reports false, and
  `errors.Is(err, context.DeadlineExceeded)` reports true

#### Scenario: A transient reply after a possible write

- **WHEN** graph-ingest answers a typed-client write with class transient and code `internal`
- **THEN** the error is commit-unknown with class fatal and code `internal`, `errs.IsTransient` reports false, and
  `errors.As` to `*errs.ClassifiedError` yields class fatal and code `internal`

### Requirement: A request no responder took keeps the transient class

A `*projection.MutationError` reporting that no mutation responder took the request (`Kind` `MutationUnavailable`,
`Commit` `CommitNotCommitted`) SHALL carry `Class` `errs.ErrorTransient`, from both `projection.MutationClient` and
`pkg/lifecycle`'s graph emitter. This requirement covers the `Class` field only.

#### Scenario: No responders

- **WHEN** a typed-client write fails with `nats.ErrNoResponders`
- **THEN** the error is `MutationUnavailable`, not committed, and its `Class` field is transient

### Requirement: One mapping from a mutation request's error to its commit state

`projection.ClassifyMutationError(operation, err)` SHALL return nil for a nil error. Otherwise it SHALL return a
`*projection.MutationError` with the given `Operation`, and every `projection.MutationClient` write and every
`pkg/lifecycle` graph emitter write SHALL report its request's failure through it. The lifecycle emitter MAY wrap the
result in `lifecycle.ErrEntityNotFound` or `lifecycle.ErrAlreadyExists`, and the wrapped error SHALL still be reached
by `errors.As`. The error SHALL be not committed when its chain holds a classified reply whose code graph-ingest sends
only before writing (invalid request, structural invalid, message type unregistered, entity ID invalid, entity ID
authority invalid, revision mismatch, entity exists, entity not found, graph state reset required), with that reply's
class, code and detail. It SHALL be not committed, `Kind` `MutationUnavailable`, class transient, when no responder
took the request or the context ended before the request was sent. Every other error, including natsclient's
refusals before sending until #268 is fixed, SHALL be commit-unknown as the requirement "A commit-unknown error is
never retryable" states.

#### Scenario: Context ended before the request was sent

- **WHEN** a typed-client Create, Append or Delete is called with a context that is already cancelled
- **THEN** no request reaches the broker, the error is not committed with class transient, and
  `errors.Is(err, context.Canceled)` reports true

#### Scenario: A reply that does not prove nothing was stored

- **WHEN** the mapping is given graph-ingest's transient `internal` reply
- **THEN** it returns a commit-unknown error with class fatal and code `internal`

#### Scenario: A lifecycle write answered after a possible write

- **WHEN** a lifecycle emitter create, reconcile or delete receives graph-ingest's transient `internal` reply
- **THEN** the error is a commit-unknown `*projection.MutationError` with class fatal, `errs.IsTransient` reports
  false, and the error does not match `lifecycle.ErrEmitFailed`

#### Scenario: A lifecycle write whose context ended before sending

- **WHEN** a lifecycle emitter write is called with a context that is already cancelled
- **THEN** no request reaches the broker, the error is not committed, `MutationUnavailable`, class transient, and
  `errors.Is(err, context.Canceled)` reports true

### Requirement: The commit state is reported on the error

A `projection.MutationClient` method that fails SHALL return a zero `MutationReceipt` and a `*projection.MutationError`
whose `Commit` reports what is proven. A method that returns no error SHALL have committed the mutation, and its
receipt SHALL hold the resulting revision.

#### Scenario: Refused before sending

- **WHEN** a `Create` is refused for a missing request ID
- **THEN** the error is invalid and not committed, the receipt is zero, and no request reaches the broker
