# Proposal: simplify-04a-19-graph-ingest

## Why

This is PR 10 of epic #223, the last of the simplification wave: `processor/graph-ingest` after PR #93. Owner,
2026-10-18: claim it with #287 and #295, and for #216 "Keep the fix out; add the unhealthy test and comment on #216"
(#216 comment 6249597351).

graph-ingest reports unhealthy for the rest of the process after one processing error, and never reports a backlog it
cannot read. Its readiness envelope says `bootstrap_complete` false until a backlog first reads zero, which under
steady traffic may never happen, and carries a `published_at` nobody reads (ADR-109). After a write whose reply was
lost, it skips the cache and poison bookkeeping, so queries can serve the old entry and a repaired entity can keep
the component unhealthy. Around this sit test-only fields on the production type and carried layering (#139).

## What Changes

- **Bookkeeping after a failed write** (#174): the cache entry is invalidated and a recorded poison entry is re-checked
  after any write that returns an error, not only after a commit.
- **The latch leaves** (#183): `bootstrap_complete` is true once `Start` returns; `bootstrap_scope` keeps its meaning.
- **`Health()` reads stuck states** (#183): poison, or a failed backlog observation on the latest tick; the error
  count no longer lowers `Healthy`. A test shows a deleted durable consumer reads unhealthy and recovers (#216; the
  stall itself is not fixed).
- **`published_at` leaves** (#183): from `graph.IndexStatusResponse` and `graph/readiness`'s publisher.
- **Test-only fields** (#295, #139): the heartbeat and clock overrides and the cache-repopulation hook leave;
  `handleMessage` leaves.
- **#139**: rulings I, J and K; tests build through the factory and the nil guards go; four receiver-free methods
  become functions; the reserved-source literal uses its constant; generated-code headers go so lint reads every
  file; `doc.go` and carried citations are corrected.
- **Help text** (#287) for `mutation_rejections_total`.

## Capabilities

- `graph-transport-boundary`: MODIFIED, the envelope requirement (no `published_at`; graph-ingest's
  `bootstrap_complete`).
- `graph-ingest-recovery`: MODIFIED wording of the interrupted-work requirement; ADDED "graph-ingest's health reads its
  stuck states".

## Impact

- Files: `processor/graph-ingest` (production and tests), `graph/index_status.go` and its test,
  `graph/readiness/publisher.go` and its test, `processor/graph-ingest/README.md`, `docs/admission-ledger.yaml`
  (`processor/graph-ingest`, `graph`, `graph/readiness` rows).
- Wire: `published_at` leaves the `GRAPH_STATUS` JSON; graph-ingest's `bootstrap_complete` is true from its first
  envelope. No reader of either is known.
- Behaviour: `Healthy` no longer falls on a processing error, and falls while the backlog cannot be read. With
  `ports` absent from the config, the default ports apply (ruling K); a config that set `ingest_lanes` below 1 is
  clamped by `ApplyDefaults` instead of `Validate`.
- Metrics: one help string.
- Closes #139 (owner question 1), #174, #183, #287, #295. Addresses #223. Does not close #216.
