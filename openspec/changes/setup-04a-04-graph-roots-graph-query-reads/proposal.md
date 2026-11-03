# Proposal: setup-04a-04-graph-roots-graph-query-reads

Approved-by:

Track: `design` (adds spec requirements and touches two capabilities).

## Why

Step 2 of five of `processor/graph-query` (change 4 item 9, epic #9; `split.md` of the archived step 1 change). It
ports every read that needs no community and no classifier. The owner: "our job when we port is to create a pragamtic,
idiomatic solution while noting what consumer use cses we need to support." The pin's versions of these reads answer
wrongly without saying so: a 1,000-row page reads as complete, a down predicate index reads as no predicates (P4), a
traversal takes any depth and node count (P7) and reports an incoming edge backwards (P8).

## What Changes

- **`pathSearch`:** required `max_depth` (at most 10), optional `max_nodes` (default 100, at most 500), required
  `direction`, all validated before any request. Refusals reach the caller. The reply names entities and true edges
  once each, and `truncated` is exact.
- **`hierarchyStats`, `summary`:** each request reads one prefix page, so it costs one key scan of the group.
  `total_entities` is true, from a new `total` on the `prefix` reply; `entities_scanned`, `truncated` and `next_cursor`
  declare a sample, and a caller who wants more pays per page. `summary` gains `prefix`, predicates left out. A
  predicate index that cannot answer refuses `summary`; `exclude_predicates` asks without it (#105, #111 item 2).
- **`semantic`, `similar`, `spatial`, `temporal`:** forwarded byte for byte, with their reply shapes decided here:
  enveloped by the responder, as `byName` is. Each refuses transient `internal` until changes 5 and 7 bring the
  responder.
- ADR-071 ported with a note. Every wire key snake case; each break is a line in `processor/graph-query/MIGRATION.md`.

Not here: communities and `localSearch` (3); classifier and `globalSearch` (4); `searchGraph` (5); the responders of
the four forwards (changes 5, 7).

## Capabilities

- Modified: `graph-query` (two requirements), `graph-transport-boundary` (the table, its marks, the revision rule).
- Added requirements: `graph-query` (path search, counts, predicates, forwards).

## Impact

- About 620 production and 1,250 test lines (hand estimate). Pin: 917 and 1,643 (`inventory.md` § 12, row 9.2). One PR:
  three independent handlers on one shell, one developer's unit per task; not worth two PRs.
- **New exported surface, accepted by the owner 2026-11-03.** In `graph`: `ResponderGraphEmbedding`,
  `ResponderGraphIndexSpatial`, `ResponderGraphIndexTemporal`, `MaxPathSearchDepth`, `MaxPathSearchNodes`,
  `DefaultPathSearchNodes`, `PathSearchRequest`, `PathSearchData`, `PathEntity`, `HierarchyStatsRequest`,
  `HierarchyStatsData`, `HierarchyChild`, `SemanticData`, `SimilarData`, `SimilarityHit`, `SpatialData`, `SpatialHit`,
  `TemporalData`, `TemporalHit`, `PrefixQueryResponse.Total`. `SummaryRequest` and `SummaryData` change. Package
  `graphquery` exports nothing new. Each has its reader in `inventory-delta.md`.
- New subjects, reserved by the table:
  `graph.query.{pathSearch,hierarchyStats,summary,semantic,similar,spatial,temporal}`,
  `graph.embedding.query.{search,similar}`, `graph.spatial.query.bounds`, `graph.temporal.query.range`. No new bucket
  and no new config key: `max_traversal_depth` and `max_traversal_nodes` stay refused.
- The owner answered `owner-questions.md` on 2026-11-03: the true `total` and `prefix` on `summary`, no count verb yet;
  `hierarchyStats` bare, which narrows ruling 1's list.
- No code before the owner approves. Retires no issue; `inventory-delta.md` disposes each one.
