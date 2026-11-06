# Proposal: setup-04a-04-graph-roots-graph-query-search

Approved-by:

Track: `design` (adds spec requirements and touches three capabilities).

## Why

Step 4 of five of `processor/graph-query` (change 4 item 9, epic #9; `split.md` of the archived step 1 change). It
gives `graph.query.globalSearch` the questions that need no text search: one entity, a path from a named entity, a
time range, an area. The owner: "our job when we port is to create a pragamtic, idiomatic solution while noting what
consumer use cses we need to support." The pin's coordinator cannot be carried as it is: it routes by bare strings,
drops the time range from a path question and both filters from a time-and-area question, swallows alias refusals in
its resolver and asks a suffix verb change 2 removed (#405), answers an unresolvable "area of" start node with nothing
(#479), and two of its strategies are reachable only from a language model's output (probe P1). The owner also put
graph-index's silent completion of an entity it cannot index here (#428).

## What Changes

- **`globalSearch`:** request `query`, `min_revision`, decoded strictly. The keyword classifier's hints pick one
  strategy: `entity` (a one-token question resolved to an entity), `path` (step 2's walk from the resolved start,
  both directions, depth 3, 100 nodes), `temporal`, `spatial`. The reply is enveloped `graph.GlobalSearchData`: the
  strategy and its result as the downstream verb gave it, an explicit chain, no summary.
- **Refusals:** two of path, time and area together are `invalid_request`; so is an impossible number (#479 ruling).
  Every other question, and a reference that resolves to nothing, gets `text_search_unsupported`, class invalid, a code
  that lives only until step 5. Alias refusals reach the caller (ADR-114).
- **One resolver**, shared with `entityByAlias`: alias, then a valid ID as itself, else not found; no suffix step.
- **`graph.TemporalRangeRequest`, `graph.SpatialBoundsRequest`:** what the two strategies send; change 7's responders
  decode them. Until then those strategies refuse transient `internal` (no responders), as step 2's forwards do.
- **graph-index:** an entity whose index plan is refused is marked failed, so readiness holds until it is rewritten.
- Consumer breaks in `processor/graph-query/MIGRATION.md`.

Not here: the text tier, the semantic strategy and `searchGraph` (step 5); LLM-only routes (#486).

## Capabilities

- Added requirements: `graph-query` (global search routing, the temporary refusal, the resolver, strategy replies);
  `graph-index` (a refused plan fails the entity).
- Modified: `graph-transport-boundary` (the table gains `globalSearch`).

## Impact

- About 450 production and 1,000 test lines (hand estimate). Pin: about 730 and 390 (`inventory-delta.md` § 9). One
  PR, one developer unit per task.
- **New exported surface, for the owner to accept:** in `graph`, `GlobalSearchRequest`, `GlobalSearchData`,
  `TemporalRangeRequest`, `SpatialBoundsRequest`. New subject `graph.query.globalSearch`. No new bucket, metric or
  config key.
- One owner question (`owner-questions.md`): leave the semantic strategy to step 5.
- Retires #405, #428, #479. No code before the owner approves.
