# Proposal: setup-04b-02-lexical-processor

Approved-by:

Track: `design`. Draft: inventory and design to follow in this change.

## Why

Change 1 (`setup-04b-01-lexical-index`, PR #615) put a real word index on `main` as the package `graph/lexical`, but
nothing in production feeds or reads it. This is the second of four changes in the plan on epic #10 (comment
6833147079): a processor that follows `ENTITY_STATES`, rebuilds the index on every start, as graph-index does, and
serves the search verb.

## What Changes

To be settled by the design. From the plan: a processor that keeps a `graph/lexical` index current from
`ENTITY_STATES`, rebuilt on every start, and answers a search request; no vector bucket, dedup bucket or worker pool
at tier 0.

## Non-goals

graph-query's local and global search and the question router (change 3); removal of `BM25Embedder`, the substring
scorers and `EmbeddingClassifier` (change 4); the query parser questions on #624.

## Rulings

Covered (owner, 2026-11-12, #10 comment 6833147079): the four-change plan and its shape.
