# Proposal: setup-04b-02-lexical-processor

Approved-by:

Track: `design`. Capabilities added: `graph-lexical`, `entity-text`. Modified: `graph-transport-boundary`,
`composed-stack`, `graph-embedding`.

## Why

Change 1 (`setup-04b-01-lexical-index`, PR #615) put a real word index on `main` as `graph/lexical`, but nothing in
production feeds or reads it (inventory §1). This is the second of four changes in the plan on epic #10 (comment
6833147079): a processor that follows `ENTITY_STATES`, rebuilds the index on every start, as graph-index does, and
serves the search verb. No vector bucket, dedup bucket or worker pool at tier 0.

## What Changes

- New component `graph-lexical` (`processor/graph-lexical`): a `graph/follower` plug-in that applies each delivered
  entity to an in-memory `lexical.Index`, publishes readiness under its own `GRAPH_STATUS` key, and answers one new
  verb, `graph.lexical.query.search`: scoped, limited, ranked, with the match count before the limit and the
  revision the answer reflects.
- One text rule for every component: new package `graph/entitytext` says which of an entity's values are its name
  and which its body, reads an offloaded body with a 10-second deadline, and caps the text, cutting the body's tail
  and saying so. graph-embedding moves onto it with the same bytes, so its vectors do not change. The word index
  also takes every label predicate as name text, and caps at 16,000 characters where graph-embedding keeps 4,000.
- Offloaded bodies are read on every start, on one body path the follower never waits for, so a store that blocks
  delays the first build by one deadline, not one per entity. A body not in the index is queued, unread (retried,
  and the readiness key reads degraded), missing or without a store, and every reply counts them. A stored
  per-entity word record, which would spare the reads, is shown as an extension point and not built.
- The index's memory is measured before it holds real data, and its private layout moves to word numbers (#623).
- The composed smoke test checks the new component's search through a delete and a restart (#628, first item).
- Docs say plainly that entities without text, such as sensor or drone telemetry, gain nothing from word search.

## Non-goals

graph-query's local and global search and the question router (change 3); removal of `BM25Embedder`, the substring
scorers and `EmbeddingClassifier`, and fusion's natural-language resolve at tier 0 (change 4); the query parser
questions on #624; a stored index or the per-entity word record; tokenizer changes (#622).

## Impact

New: `graph/entitytext`, `processor/graph-lexical`, one row in `graph/query_verbs.go`, one key in
`graph/readiness`, request and reply types in `graph`. Changed: graph-embedding reads text through
`graph/entitytext` (no change in what it embeds); `graph/lexical` internals (layout only); `internal/smoke`;
`AGENTS.md` and `.agents/rules-index.md` (the reserved-subject row gains `graph.lexical.query.*`). No consumer answer
changes: `semantic`, `similar` and fusion are untouched. A product that wants word search registers one more
component (`graph-lexical`). Evidence: `notes/probes/` (object store reads and a slow watch; SemSource body sizes).

## Retires, and in which pull request

Three pull requests under this one change: A, the text rule (`Closes #551`: the shared body read has a deadline);
B, the index layout (`Closes #623`); C, the component, the verb, smoke and docs (`Addresses #628`, its first item;
changes 3 and 4 carry the rest), with the archive.

## Rulings

Covered (owner, 2026-11-12, #10 comment 6833147079): the four-change plan, its shape, and starting without a
per-entity token record provided the base leaves room to add one. Covered (owner, PR #629 design read): bodies read
on every start, no silent switch of SemSource's natural-language path, the setup-plan correction, the 10-second
deadline. Open for the owner: design questions 1 and 2 (the first-build budget, the cap).
