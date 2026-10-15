# projection-mutation

## ADDED Requirements

### Requirement: A commit-unknown error is never retryable

A `*projection.MutationError` that reports commit-unknown (`Commit` is `CommitUnknown`, `Kind` is
`MutationCommitUnknown`), whether `projection.MutationClient` or `pkg/lifecycle`'s graph emitter returns it, SHALL
carry `Class` `errs.ErrorFatal` whatever error it wraps. For that error, and for any error that wraps it,
`errs.IsTransient` SHALL report false, `errs.IsFatal` SHALL report true, and `errs.Classify` SHALL return
`errs.ErrorFatal`. `errors.As` to `*errs.ClassifiedError` SHALL yield class fatal with the reply's `Code` and `Detail`.
`errors.Is`, and `errors.As` to any other type, SHALL still reach the error it wraps, and the `MutationError`'s `Code`
and `Detail` SHALL be the reply's.

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
