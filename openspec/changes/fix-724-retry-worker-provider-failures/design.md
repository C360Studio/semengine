# Design: fix-724-retry-worker-provider-failures

## Context

Facts are from `inventory.md` (base e925194a), cited by section.

- The repair lane re-drives only hop 1's three failures, every 30 s (§ 1: `failed.go:44-50,209-216`,
  `hop1.go:283,307-313`). The Worker retries failed records only in its first snapshot after a start (`worker.go:418`).
- Re-driving through hop 1 already works mechanically. `reconcileEntity` writes a pending record with
  `SavePendingGuarded`, which overwrites a failed record at any revision (`storage.go:241-243`). The Worker processes
  every pending put it sees live (`worker.go:414-419`). Its outcome reaches the failed map through `onTerminal`
  (`hop1.go:253-262`): a success at or above the failure's revision clears it (`failed.go:84-94`). A repeat failure
  of an entity already failed does not add to ErrorCount (`failed.go:153-168`). The follower's `Complete` of a
  revision already completed does nothing (`internal/revlag/watermark.go:110-125`).
- `embedder_error` mixes provider faults (5xx, network failures other than a refused connection) with refusals of
  the text or config (4xx, malformed reply). The classifier reads error text (§ 2: `worker.go:786-805`).
- Reasons reach GRAPH_STATUS `failed_reasons` and stored records. No consumer keys on a value, and there is no
  metric label (§ adopter seam).
- Existing pacing against a failing dependency: graph-lexical makes one failed read per store per pass
  (`processor/graph-lexical/bodies.go:15-18`).

## Goals / Non-Goals

Goals: the owner's two rulings; recovery without a restart; a provider that is still down is not loaded by retries;
no config key. Non-goals: as in the proposal.

## Decisions

**D1. Which reasons are retried.** Retried: `connection_refused`, `timeout`, `rate_limited`, `provider_unavailable`
and `internal`. `internal` is a fault in SemEngine's own NATS reads or writes, caused by neither the text nor the
config. Not retried: `dimension_mismatch` (config), `embedder_error`, `content_error`, `unknown`.
`content_error` mixes a missing body (data) with a store outage; retrying it is outside the ruling. Owner question 2.
The fact lives in one place, `embedding.RetryableReason(reason) bool`, next to the closed reason set. Its one
production caller is `processor/graph-embedding/failed.go`.

**D2. Splitting `embedder_error`.**

- (A, recommended) httpembedder wraps 5xx replies and transport errors in `embedding.ErrProviderUnavailable`, and
  429 in `embedding.ErrRateLimited`. The Worker files them `provider_unavailable` and `rate_limited`, checked with
  `errors.Is`. A refused connection and a timeout keep their reasons, because their checks run first. Cost: two
  sentinels and one new reason beyond the ruled `rate_limited`. Adopter seam: a product embedder that does nothing
  still gets refused connections and timeouts retried. Its other outage errors are filed `embedder_error` and wait
  for a restart, which is today's behaviour, visible as `degraded` with that reason. To have them retried it wraps
  `ErrProviderUnavailable`; the `graph/embedding` README says so.
- (B) Retry `embedder_error` as a whole and add only `rate_limited`. Cost: fewer symbols, but text refusals
  (400/413/422) and config errors (401/404, wrong model) are retried forever at the pace in D3, against the ruling.
- (C) Do nothing for 5xx. Cost: a provider error, which the ruling retries, is not retried.

Owner question 1.

**D3. Pacing.**

- (A) Flat: every pass re-drives every retryable failure. During an outage of N entities, each 30 s costs N reads,
  2N writes and N error logs.
- (B, recommended) One, then all. A pass re-drives every retryable failure if, since the previous pass, the Worker
  stored a vector for an entity that was failed. Otherwise it re-drives one: the next after the last one re-driven,
  in entity ID order, so an entity whose own text keeps failing cannot hold back the rest. This follows
  graph-lexical's one attempt per pass. Provider concurrency is capped by `workers` either way.
- (C) Per-entity exponential backoff. Cost: per-entity timers, and recovery takes up to the backoff cap after a long
  outage.

In every option, an entity whose pending record was written after its failure is waiting for the Worker. It is not
re-driven again until the Worker reports. Failures seeded at Start wait too, because the Worker's first snapshot
retries them. Owner question 3.

**D4. How a retry reaches the Worker.** (A, recommended) Through hop 1's `reconcileEntity`, as the hop 1 repairs
do: it reads the current value, so an entity deleted, damaged or left without text is ended rather than embedded.
(B) A new `Storage` method that flips a failed record to pending. Cost: new surface, and it bypasses current truth
(ADR-108 Decision 4). Under (A), a re-driven entity stays counted failed, and readiness stays `degraded`, until the
Worker's outcome. That is unchanged from today's model.

**D5. Proof.**

- Pacing and retry: `processor/graph-embedding` integration tests call `c.repair` directly, as
  `TestRepairConvergesStranded` does, with a switchable test embedder. That is deterministic and needs no 30 s wait.
- End to end: `TestLosingTheProviderKeepsWordSearch` (`processor/graph-query/provider_loss_integration_test.go`).
  A closed `httptest` server cannot come back on its address, and binding the old port again breaks the rule
  against fixed ports. So the fake provider answers 503 while "down" (`provider_unavailable`) and answers again when
  restored. The test then waits up to one 30 s pass for B's vector and `ready`. `connection_refused` stays proven by
  `TestClassifyEmbedErr`. Owner question 4.

## Invariants (spec homes)

- A retry never moves the failed map backwards; it only adds outcomes through `onTerminal`
  (graph-embedding "The failed map never moves backwards").
- At most one re-drive per entity waits for the Worker at a time (graph-embedding delta, "Provider failures are
  retried").
- A reason that is not retryable is never re-driven by the lane (same requirement).

## Risks / Trade-offs

- In a flood, hop 1 writes are serial, one lock per entity, so follower deliveries interleave (README "Hop 1 is
  serial").
- An entity whose Worker outcome never arrives (failed record write lost, panic) waits for a restart or a new
  revision, as today.
- A vector reused from the dedup bucket counts as the provider answering. The worst case is one flood that fails
  fast.
- `TestRepairConvergesStranded` uses `timeout` to mean "not retried" (`repair_integration_test.go:180`). It moves to
  `dimension_mismatch`.

## Migration Plan

None. A new reason stored by this build and read by an older one is read as `unknown`.

## Open Questions (owner)

1. D2: split `embedder_error` (A), or retry it whole (B)?
2. D1: retry `internal`? Leave `content_error` alone?
3. D3: one-then-all pacing?
4. D5: the end-to-end outage becomes a 503 instead of a closed server?
