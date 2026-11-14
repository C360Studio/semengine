# Proposal: fix-578-prefix-damaged-entities

Approved-by: cglusky (2026-11-14) sha256:93fe60c877d4

Track: `design`. Capabilities modified: `graph-transport-boundary`, `graph-query`, `fusion`.

## Why

When one stored entity under a prefix cannot be decoded, graph-ingest refuses the whole prefix page with
`graph_state_reset_required` (`processor/graph-ingest/query.go:330`). Fusion's prefix search builds its seeds from
that page, so one damaged record fails the whole answer (#578). The owner ruled on #657 (comment 6877432505,
ruling 4): the fix is a listing of IDs only, and the full-entity prefix page keeps refusing. Ruling 1 says a caller
that wants to read around damage lists IDs first.

## What Changes

- A new verb, `prefixIDs`. graph-ingest answers it on `graph.ingest.query.prefixIDs`, and graph-query forwards it
  unchanged on `graph.query.prefixIDs`. It takes a `graph.PrefixQueryRequest`. It answers a new
  `graph.PrefixIDsResponse`: `ids`, `total`, and `next_cursor`. Paging, total and validation are the prefix verb's.
  It reads no stored value, so a damaged entity is listed like any other, nothing is recorded and health does not
  change.
- Fusion's prefix search lists IDs with `prefixIDs` and reads the entities through batch, as the `nl` search already
  does. A damaged entity is answered as unhydrated `error`, through the existing one-at-a-time re-read.
- No change to the prefix verb, its reply or its tests; graph-query's counts, summary and searches (they keep
  refusing, by ruling 1); the gateway; the batch verb (#656).

## Impact

The change touches `graph`, `processor/graph-ingest`, `processor/graph-query`, `pkg/fusion/fusionnats` and three
specs. Draft #654 also edits `processor/graph-query`. Whichever PR is approved first merges first, and the other
merges `origin/main`.
