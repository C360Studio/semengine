# transport-client

## ADDED Requirements

### Requirement: An unclassified handler error is sent as fatal

When a `SubscribeForRequests` handler returns an unclassified error (no `*errs.ClassifiedError` in its chain and none of
the sentinels `errs.IsTransient`, `errs.IsFatal` or `errs.IsInvalid` list), the reply SHALL carry
`X-Error-Class: fatal`. A `RequestClassified` caller SHALL receive an error for which `errs.IsFatal` reports true and
`errs.IsTransient` reports false.

#### Scenario: A plain handler error

- **WHEN** a handler returns `errors.New("marshal failed")`
- **THEN** the reply's `X-Error-Class` is `fatal`, and the requester's error reports `errs.IsFatal` true and
  `errs.IsTransient` false
