# embedding-index Delta

## ADDED Requirements

### Requirement: An unavailable provider is named

The Worker SHALL file a generation error matching `embedding.ErrRateLimited` as `rate_limited`, and one matching
`embedding.ErrProviderUnavailable` as `provider_unavailable`, unless an earlier rule files it `timeout` or
`connection_refused`. `embedding.RetryableReason` SHALL accept exactly `connection_refused`, `timeout`,
`rate_limited`, `provider_unavailable` and `internal`.

#### Scenario: Reasons

- **WHEN** each sentinel, a refused connection wrapped in `ErrProviderUnavailable`, and each stored reason are
  classified
- **THEN** each gets its reason, and only the five are retryable (`TestClassifyEmbedErr`, `TestRetryableReason`)

### Requirement: The HTTP embedder marks a busy or failing provider

The HTTP embedder SHALL return an error matching `embedding.ErrRateLimited` for a 429 reply, without waiting on
`Retry-After` or sending again, and an error matching `embedding.ErrProviderUnavailable` for a 5xx reply or a request
that got no reply. Any other non-2xx reply and any untrusted reply SHALL match neither.

#### Scenario: Status classes

- **WHEN** the provider answers 429 with `Retry-After`, 503, or 400, or is closed
- **THEN** the errors are rate-limited (one request), unavailable, neither, and unavailable
  (`TestProviderUnavailableIsMarked`)
