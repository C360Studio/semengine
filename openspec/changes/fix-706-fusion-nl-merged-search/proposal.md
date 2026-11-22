# Proposal: fix-706-fusion-nl-merged-search

Approved-by:

Track: `design`. Capabilities: `fusion` (modified, renamed), `graph-query` (added), `graph-transport-boundary`
(modified).

## Why

At tier 1, `graph.query.searchGraph` ranks a question by merging word matches and meaning matches. Fusion never
sees that merge. `fusionnats` sends every natural-language (`nl`) resolve to graph-lexical's word search alone
(`pkg/fusion/fusionnats/client.go:100-101,161`). As a result, SemSource's `code_search`, `code_context` and
`doc_context` stay word-only when embeddings run, and every answer says `provenance: lexical`. The owner ruled
on #706 (2026-11-21): "Fusion's natural-language resolve uses the merged search when the composition runs embeddings,
and the seeds say where they came from." `v0.1.0-alpha.7` waits for this fix.

## What changes

- graph-query gains `graph.query.mergedSearch`. It takes the word search's own request (`query`, `limit`, `scope`,
  `min_revision`). It answers the ranked list searchGraph would build: entity IDs, scores, word and vector ranks,
  the word index's counts, and the `merge` hint. It reads no entity back. Both inner searches are scoped (ADR-071).
  At tier 0 the list is the word list alone. The ask-both-and-merge step is shared with searchGraph, so the merge
  and its fallback live in one place.
- `fusionnats` sends an `nl` resolve to `mergedSearch` instead of graph-lexical's `search`. Seeds keep the reply's
  order and score.
- `fusion.Resolution` gains `Provenance`. fusionnats sets it to `embedding` when the reply's `merge` hint says
  vectors were merged, and to `lexical` otherwise. A response carries the resolve's provenance when one was set,
  otherwise its mode's (`ProvenanceForMode` is unchanged).

## Non-goals

Per-seed provenance (owner question 2). A new provenance value (owner question 3). Scope on `searchGraph` or
`globalSearch`. The 40-seed batch payload failure (#659). Reranking (#682). The damaged-record rule (#657).

## Impact

- At tier 0 the answer is unchanged: the same word list, `provenance: lexical`. At tier 1, nl seeds come from the
  merge and are labelled `embedding`.
- With `include_scores`, a merged answer's `similarity` is the merged score, 1/(60+place) summed over both lists,
  not the word index's score.
- A stack without graph-query, or without graph-lexical, still refuses nl transient `internal`, naming the subject
  nothing answered.
- Consumers migrate (`pkg/fusion/MIGRATION.md`, `processor/graph-query/MIGRATION.md`). SemSource's
  `nlDisabledClient` comment names the old subject. A product whose fusionnats timeout is below graph-query's
  `query_timeout_ms` sets one under the other (design Risks).
- Docs: the fusion and graph-query README, doc.go and MIGRATION files; ADR-062 and ADR-071 port notes; ledger rows
  `pkg/fusion/fusionnats`, `pkg/fusion`, ADR-062, ADR-071 and `processor/graph-query` (its stale fallback-only
  text, `:4419-4421,:4436-4437`).

## Retires, and in which pull request

One pull request: `Closes #706`. #659, #657, #682 and #621 stay open.
Merge order: independent of #710 (no shared files or capabilities); either may merge first.
