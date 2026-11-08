# Proposal: setup-04a-05-first-consumer-graph-embedding

Approved-by:

Track: `design` (adds spec requirements; touches `graph-embedding` (new), `embedding-index`, `entity-state-follower`).

## Why

Change 5 of epic #9 makes SemSource's composition build against SemEngine alone (04A D2). The owner's direction of
2026-11-08: "change 5, setup-04a-05 (processor/graph-embedding + pkg/fusion)". This is PR A of four (`split.md`):
the graph-embedding component's write path, so that every entity gets a BM25 embedding at tier 0 with no external
provider, and its readiness is honest. The query verbs come in PR B; fusion in C and D.

## What Changes

- Port `processor/graph-embedding` on `component.Lifecycle` (ADR-108 D1) and as a `graph/follower` plug-in on the
  `graph-embedding` `GRAPH_STATUS` key (ADR-108 D2, #109). The pin's own watch, reset latch, status bucket, heartbeat
  and `readiness.go` are not carried.
- `embedding.Worker.Run` runs under the component's one run context (ADR-113). A lost watch while the component runs
  is a named stuck state in `Health` until restart, as the follower's lost feed is (ledger `graph/embedding`).
- The failed map never takes back a failure older than the entity's latest terminal outcome. Its per-reason counts
  reach the envelope through `follower.Config.Failed`, which now returns the count and the reasons together;
  `first_failure_at` is not set (owner, PR #550 comment 6716972354).
- Config refuses unknown keys. `batch_size` (read by nothing at the pin) is dropped; `embedder_type` accepts only
  `bm25`.
- `Health` drops the pin's error-count term (ADR-109 D2).
- Not carried: the HTTP embedder path (#489, #32), `graph.embedding.query.status` (ADR-083), the always-true
  indexing-profile check (pin ADR-054 Phase 1).

## Capabilities

- New `graph-embedding`.
- Modified `entity-state-follower`: the envelope carries the plug-in's failure reasons.
- `embedding-index`: unchanged requirements, new adopter.

## Impact

- Pin lines: 3,253 production, 5,701 test for the package (this PR about 1,500 + 2,300, hand estimate).
- No new subject or bucket. Exported change: `follower.Config.Failed` returns the count and a fresh reasons map
  together, so they always agree (graph-index now, spatial and temporal at their ports return `nil` reasons; envelope
  unchanged for them); `graph.IndexStatusResponse.FirstFailureAt` removed; otherwise the pin's `Register`, `Config`,
  `DefaultConfig`. The two embedding
  buckets are already in `graph/kvcatalog`.
- Consumer migration (`processor/graph-embedding/MIGRATION.md`): SemSource's `batch_size` is refused at boot;
  `first_failure_at` is gone from the status entry;
  `embedder_type: http` is refused at tier 0; `graph.embedding.query.status` is gone (read the `GRAPH_STATUS` key).
- Merge order: #548 first.
- Retires none. Stays: #109, #196, #195 (this package's 12 sites adopt), #110 (PR C), #477 (PR B), #489, #32, #155,
  #482, #11, #484.
- Owner rulings (PR #550 comment 6716972354): four-PR split; probes; G10; fusion reads not-ready from the
  reply (PRs C, D; changes pin ADR-066 and ADR-082). Remaining: `Approved-by:`.
