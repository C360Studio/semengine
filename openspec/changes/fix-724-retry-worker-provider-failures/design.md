# Design: fix-724-retry-worker-provider-failures

## Context

Facts are from `inventory.md` (base e925194a), cited by section.

- The repair lane re-drives only hop 1's three failures, every 30 s (§ 1). The Worker retries failed records only
  in its first snapshot after a start (`worker.go:418`).
- Re-driving through hop 1 already works. `reconcileEntity` writes a pending record with `SavePendingGuarded`,
  which overwrites a failed record at any revision (`storage.go:241-243`). The Worker processes every pending put
  it sees live (`worker.go:414-419`). Its outcome reaches the failed map through `onTerminal` (`hop1.go:253-262`):
  a success at or above the failure's revision clears it (`failed.go:84-94`). A repeat failure of an entity already
  failed adds nothing to ErrorCount (`failed.go:153-168`). `Complete` of a revision already completed does nothing
  (`internal/revlag/watermark.go:110-125`).
- `embedder_error` mixes provider faults (5xx, network failures) with refusals of the text or config (4xx, TLS
  certificates, malformed replies). The classifier reads error text (§ 2).
- `pkg/errs` already says whether an error may be retried (`IsTransient`, `ErrRateLimited`, `WrapTransient`), never
  from text; ADR-111 Decision 3 and ADR-060 forbid a second spelling (§ 2, § 3).
- Reasons reach GRAPH_STATUS `failed_reasons` and stored records; no metric label exists. SemSource's qualification
  report records `embedder_error` from injected 503s (adopter seam).

## Goals / Non-Goals

Goals: the owner's two rulings; recovery without a restart; a provider that is still down is not loaded by retries;
no config key. Non-goals: as in the proposal.

## Decisions

**D1. Which reasons are retried.** Retried: `connection_refused`, `timeout`, `rate_limited`, `provider_unavailable`
and `internal` (SemEngine's own NATS read or write, neither text nor config). Not retried: `dimension_mismatch`,
`embedder_error`, `content_error` (a missing body and a store outage alike; outside the ruling), `unknown`. The fact
lives once, in `embedding.RetryableReason(reason) bool`, beside the closed reason set; its one production caller is
`processor/graph-embedding/failed.go`. Owner question 2.

**D2. Telling a provider outage from a refusal.**

- (A, recommended) Reuse `pkg/errs`. httpembedder returns, for a 429, an error matching `errs.ErrRateLimited` (and so
  `errs.IsTransient`); for 500, 502, 503, 504 and for a request that got no reply, `errs.WrapTransient`; a TLS
  certificate failure (`*tls.CertificateVerificationError`) and every other reply stay unclassified. The Worker files,
  in order: the call's deadline → `timeout`; `errors.Is(err, errs.ErrRateLimited)` → `rate_limited`; text "connection
  refused" → `connection_refused`; text "timeout" → `timeout`; `errs.IsTransient` → `provider_unavailable`; text
  "dimension" → `dimension_mismatch`; else `embedder_error`. Typed marks come before text where both could match.
  The deadline check covers `IsTransient`'s `context.DeadlineExceeded`. A shutdown's `context.Canceled` is caught
  before any record is written (`worker.go:827-830`). Cost: one new reason beyond the ruled `rate_limited`, no new
  sentinel. Adopter seam: a product embedder that does nothing still has refused connections and timeouts retried;
  its other outages are filed `embedder_error`, read `degraded`, and wait for a restart, as today. It gets retries by
  returning a transient `errs` error, which ADR-060 consumers already use. The `graph/embedding` README says so.
  The text checks still read the whole message, so `WrapTransient`'s action word must contain none of "timeout",
  "connection refused" or "dimension"; a 5xx whose body says "dimension" is filed `provider_unavailable`.
- (B) Two new sentinels in `graph/embedding`. Cost: a second spelling of `pkg/errs`, against ADR-111 Decision 3.
- (C) Retry `embedder_error` as a whole. Cost: refused texts and config errors retried forever, against the ruling.
- `model.ErrorKind` is not reused: it has no production reader (#155), and httpembedder may not import `model`.

Owner question 1.

**D3. Pacing.**

- (A) Flat: each pass re-drives every retryable failure. Cost: during an outage of N entities, each 30 s costs N
  reads, 2N writes and N error logs.
- (B, recommended) One, then all. A pass re-drives every retryable failure if, since the previous pass, the Worker
  stored a vector for an entity that was failed (a skipped or deleted outcome does not count). Otherwise it
  re-drives one, the next after the last one re-driven in entity ID order, so one entity whose own text keeps failing
  cannot hold back the rest. The cost while down matches graph-lexical's one failed read per store per pass. Its rule
  cannot be copied, because its read is synchronous, while the Worker's outcome arrives after the pass ends; so the
  lane decides from outcomes reported since the previous pass. Provider concurrency is capped by `workers` either way.
- (C) Per-entity exponential backoff. Cost: per-entity timers; recovery waits up to the cap after a long outage.

In every option, an entity whose pending record was written after its failure is waiting for the Worker and is not
re-driven until the Worker reports. A failure seeded at Start waits too, because the Worker's first snapshot retries
it. The choice is one pure `failedMap` method, `retryTargets`: retryable, not waiting, one or all, the cursor. The lane
re-checks "waiting" under `hop1Mu` before it re-drives, since a delivery may queue the entity in between. Owner
question 3.

**D4. How a retry reaches the Worker.** (A, recommended) Through hop 1's `reconcileEntity`, as hop 1's repairs do:
it reads current truth (ADR-108 Decision 4), so an entity deleted, damaged or left without text is ended, not
embedded. (B) A new `Storage` method flipping a failed record to pending. Cost: new surface that bypasses current truth.

**D5. A NATS fault during a retry (declared limit).** If hop 1's read or write fails during a re-drive, the entity is
counted under hop 1's reason and withholds the verbs, as any hop-1 fault does (`failed.go:100-111`). Once hop 1
converges, the entity is counted not failed until the Worker reports (`failed.go:113-121`), and a repeat Worker failure
counts again in ErrorCount. Declared in the README's known limits and filed as #734, not fixed: the trigger is a NATS
fault during a retry, nothing is lost, and the Worker's outcome restores the count. The fix would keep the Worker's
reason under hop 1's, at the cost of another field and a fault-injected test.

**D6. Proof.**

- Which entities a pass picks: `TestRetryTargets`, a unit test of `retryTargets` with exact sets.
- Retry through NATS: `processor/graph-embedding` integration tests call `c.repair` and assert only invariants that
  an extra background pass cannot break.
- End to end: `TestLosingTheProviderKeepsWordSearch`. A closed `httptest` server cannot return on its address, and
  binding its port again breaks the fixed-port rule, so the fake answers 503 while down and answers again when
  restored; the test waits up to one 30 s pass. `connection_refused` stays proven by `TestClassifyEmbedErr`. Owner
  question 4.

Loop supervision (#714's questions): the goal is a stored vector; the Worker measures it, not the lane; unbounded
by design, as hop 1's lane; it ends on a vector, a reason not retried, or a new revision; `degraded` notices no
progress.

## Invariants (spec homes)

- A retry never moves the failed map backwards ("The failed map never moves backwards").
- At most one re-drive per entity waits for the Worker; a reason not retried is never re-driven ("Provider failures
  are retried").

## Risks / Trade-offs

- A flood's hop 1 writes are serial, one lock per entity, so deliveries interleave (README "Hop 1 is serial").
- An entity whose Worker outcome never arrives (failed record write lost, panic) waits, as today.
- `TestRepairConvergesStranded` uses `timeout` as "not retried" (`repair_integration_test.go:180`); it moves to
  `dimension_mismatch`.

## Migration Plan

None. A new reason read by an older build reads as `unknown`. SemSource's next tier-1 fault run reads
`provider_unavailable`, then clears.

## Open Questions (owner)

1. D2: reuse `pkg/errs` and split `embedder_error` (A)?
2. D1: retry `internal`; leave `content_error`?
3. D3: one-then-all pacing?
4. D6: the end-to-end outage becomes a 503 instead of a closed server?
