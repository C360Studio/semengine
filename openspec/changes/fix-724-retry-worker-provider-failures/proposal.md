# Proposal: fix-724-retry-worker-provider-failures

Approved-by:

Track: `design`. Capabilities: `graph-embedding` (added), `embedding-index` (added), `graph-query` (modified).
Packages: `graph/embedding`, `graph/embedding/httpembedder`, `processor/graph-embedding`, tests in
`processor/graph-query`.

## Why

When the embedding provider is down, every entity graph-embedding's Worker tries in that window is stored as failed.
Nothing retries it until the entity is written again or the process restarts
(`processor/graph-embedding/README.md:155-156`). So after the provider comes back, graph-embedding's readiness stays
`degraded` and those entities have no vector. This blocks SemSource's tier-1 qualification (#11). A semembed 429
(queue full) ends the same way, filed as `embedder_error` (#732).

Owner rulings, 2026-11-23: #724 comment 7105938051 and #732 comment 7105937422 (option A). Failures caused by the
provider being unavailable (unreachable, timed out, provider error, 429) are retried automatically. Failures caused
by the text or config are not. A 429 is recorded under its own reason. Retry-After is not honoured.

## What changes

- The repair lane, every 30 s, also re-drives Worker failures whose reason says the provider was unavailable. It
  writes the entity's pending record again from its current value, and the Worker embeds it. While no retry has
  succeeded, a pass retries one entity, so a provider that is still down sees one request per 30 s. Once a retry
  gets a vector, the next pass retries the rest. An entity already waiting for the Worker is not retried again.
- `httpembedder` marks a 429 as rate-limited, and marks a 5xx reply or a network failure as provider unavailable.
  `graph/embedding` exports both marks, so a product's own embedder can use them. The Worker files them as
  `rate_limited` and `provider_unavailable`.
- Retried reasons: `connection_refused`, `timeout`, `rate_limited`, `provider_unavailable`, `internal`. Not retried:
  `dimension_mismatch`, `embedder_error` (now a 4xx reply or a malformed reply), `content_error`, `unknown`.
- The provider-loss scenario restores the provider and sees the entity's vector and graph-embedding ready.

## Non-goals

Retry-After. A new config key. Per-entity backoff. Retrying body-store failures (`content_error`). The width-at-restart
issue (#722).

## Impact

New exported surface in `graph/embedding`: `ErrRateLimited`, `ErrProviderUnavailable`, `RetryableReason`. Two new
stored reason values; no migration. One PR (#733), `Closes #724`, `Closes #732`.
