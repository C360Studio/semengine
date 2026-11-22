# Proposal: fix-706-fusion-nl-merged-search

Approved-by:

Track: `design`. Capabilities: `fusion` (modified, renamed, removed, added), `graph-query` (modified, added),
`graph-transport-boundary` (modified).

## Why

At tier 1, `graph.query.searchGraph` ranks a question by merging word matches and meaning matches. Fusion never
sees that merge. `fusionnats` sends every natural-language (`nl`) resolve to graph-lexical's word search alone
(`pkg/fusion/fusionnats/client.go:100-101,161`). So SemSource's `code_search`, `code_context` and `doc_context` stay
word-only when embeddings run, and every answer says `provenance: lexical`. The owner ruled on #706 (2026-11-21):
"Fusion's natural-language resolve uses the merged search when the composition runs embeddings, and the seeds say
where they came from." `v0.1.0-alpha.7` waits for this fix.

## What changes

- graph-query gains `graph.query.mergedSearch`. It takes the word search's own request (`query`, `limit`, `scope`,
  `min_revision`) and answers the ranked list searchGraph would build: entity IDs, scores, the word index's
  `excluded` and `bodies_not_indexed`, and the `merge` hint. It reads no entity back. Both inner searches are scoped
  (ADR-071). It waits at most 2.5 s for the embedding search. At tier 0 the list is the word list alone. The
  ask-both-and-merge step is shared with searchGraph.
- `fusionnats` sends an `nl` resolve to `mergedSearch`. Seeds keep the reply's order and score.
- `fusion.Resolution` gains `Provenance`: `embedding` when the reply's `merge` hint is honoured, else `lexical`. A
  response carries the resolve's provenance when one was set, otherwise its mode's.

## Non-goals

Per-seed provenance (owner question 2). A new provenance value (owner question 3). Scope on `searchGraph` or
`globalSearch`. searchGraph's own embedding wait (#712). #659, #682, #657.

## Impact

- Tier 0: unchanged; the same word list, `provenance: lexical`. Tier 1: nl seeds come from the merge, labelled
  `embedding`. An embedder slower than 2.5 s gives the word answer, labelled `lexical`, with one Warn and one
  merge-failed count.
- Tier 1, while the word index is still building after a start: an nl question is answered from meaning alone,
  labelled `embedding`, where today it defers `index_not_ready`. It still defers when the embedding search fails too.
- With `include_scores`, a merged answer's `similarity` is the merged score (1/(60+place) summed over both lists).
- No graph-query or no graph-lexical composed: nl is still refused transient `internal`, naming the subject nothing
  answered.
- Consumers migrate (`pkg/fusion/MIGRATION.md`, `processor/graph-query/MIGRATION.md`); SemSource's
  `nlDisabledClient` comment names the old subject.
- Docs: the fusion and graph-query README, doc.go and MIGRATION files; the query-pattern skill; ADR-062 and ADR-071
  port notes; ledger rows `pkg/fusion/fusionnats`, `pkg/fusion`, ADR-062, ADR-071 and `processor/graph-query`.

## Retires, and in which pull request

One pull request: `Closes #706`. #659, #657, #682 and #621 stay open.
Merge order: independent of #710 (no shared files or capabilities); either may merge first.
