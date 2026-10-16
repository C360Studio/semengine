# projection-mutation

## MODIFIED Requirements

### Requirement: One mapping from a mutation request's error to its commit state

`projection.ClassifyMutationError(operation, err)` SHALL return nil for a nil error. Otherwise it SHALL return a
`*projection.MutationError` with the given `Operation`, and every `projection.MutationClient` write and every
`pkg/lifecycle` graph emitter write SHALL report its request's failure through it. The lifecycle emitter MAY wrap the
result in `lifecycle.ErrEntityNotFound` or `lifecycle.ErrAlreadyExists`, and the wrapped error SHALL still be reached
by `errors.As`. The error SHALL be not committed when its chain holds a classified error whose code graph-ingest sends
only before writing (invalid request, structural invalid, message type unregistered, entity ID invalid, entity ID
authority invalid, revision mismatch, entity exists, entity not found, graph state reset required), with that
error's class, code and detail. The typed client's own refusals before sending (a missing context, an operation not
admitted, a request that does not encode as JSON, a create without an entity) SHALL be such an error, with class
invalid and code `invalid_request`, and SHALL send no request. The error SHALL be not committed, `Kind`
`MutationUnavailable`, class transient, in three cases: no responder took the request; natsclient refused it before
sending (`natsclient.ErrNotConnected`, `natsclient.ErrCircuitOpen`), with `errors.Is` still reaching that error; or
the context ended before the request was sent. Every other error SHALL be commit-unknown as the requirement "A
commit-unknown error is never retryable" states.

#### Scenario: Context ended before the request was sent

- **WHEN** a typed-client Create, Append or Delete is called with a context that is already cancelled
- **THEN** no request reaches the broker, the error is not committed with class transient, and
  `errors.Is(err, context.Canceled)` reports true

#### Scenario: natsclient refused the request before sending

- **WHEN** a typed-client write's request is refused with `natsclient.ErrNotConnected` or `natsclient.ErrCircuitOpen`
- **THEN** the error is not committed, `MutationUnavailable`, class transient, and `errors.Is` reaches the
  natsclient error

#### Scenario: A request the client cannot encode

- **WHEN** a typed-client Append carries a statement whose object does not encode as JSON
- **THEN** no request reaches the broker, and the error is not committed, `MutationInvalid`, class invalid, code
  `invalid_request`

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
