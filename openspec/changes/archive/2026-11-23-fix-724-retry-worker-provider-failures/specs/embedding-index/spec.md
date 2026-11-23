# embedding-index Delta

## ADDED Requirements

### Requirement: An unavailable provider is named

The Worker SHALL file a generation error as `timeout` when its call's deadline passed; else as `rate_limited` when it
matches `errs.ErrRateLimited`; else `connection_refused` or `timeout` by its text as today; else as
`provider_unavailable` when `errs.IsTransient` reports it; else `dimension_mismatch` or `embedder_error` as today.
`embedding.RetryableReason` SHALL accept exactly `connection_refused`, `timeout`, `rate_limited`,
`provider_unavailable` and `internal`.

#### Scenario: Reasons

- **WHEN** a rate-limited error, a transient error, a refused connection marked transient, a plain error, and each
  stored reason are classified
- **THEN** they are `rate_limited`, `provider_unavailable`, `connection_refused`, `embedder_error`, and only the five
  are retryable (`TestClassifyEmbedErr`, `TestRetryableReason`)

### Requirement: The HTTP embedder marks a busy or failing provider

The HTTP embedder SHALL return, for a 429 reply, an error matching `errs.ErrRateLimited`, without waiting on
`Retry-After` or sending again; and, for a 500, 502, 503 or 504 reply or a request that got no reply, an error
`errs.IsTransient` reports. A TLS certificate failure, any other non-2xx reply and any untrusted reply SHALL NOT be
transient.

#### Scenario: Status classes

- **WHEN** the provider answers 429 with `Retry-After`, 503, 400 or 501, is closed, or presents an untrusted
  certificate
- **THEN** the errors are rate-limited after one request, transient, not transient, not transient, transient, and
  not transient (`TestProviderUnavailableIsMarked`)
