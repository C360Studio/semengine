# Proposal: setup-04c-02-hybrid-search

Approved-by:

Track: `design`. Capability: `graph-query` (modified, added).

## Why

At tier 1 a question can be answered from two lists: the word index's and the embedding index's. searchGraph asks
the embedding index only when words find nothing (`processor/graph-query/textsearch.go:72`). On the frozen scorecard
the two lists merged find the answer in the first ten 62.4% of the time, against 56.0% for words and 52.0% for
vectors, but that number came from a throwaway program. No product could get that ranking. The owner ruled
(2026-11-16, #668 and #666): build the merge in searchGraph first, then put tier 1's floor on the merged row, about
60%.

## What Changes

- **PR A, the merge** (`processor/graph-query`, `graph`). With `embedding_enabled` true, searchGraph asks the word
  search and the embedding search, each 100 deep, and merges them by reciprocal rank: each entity scores the sum of
  1 / (60 + its place) over the lists that hold it.
  - `text.matches` is the merged list, each match saying its `word_rank` and `vector_rank`.
  - `merge` (renamed from the unreleased `fallback`) says whether the embedding list was read, and why not.
  - A provider that is down leaves the word answer, labelled, never a refusal. Tier 0 and `globalSearch` are
    unchanged.
- **PR B, the tier 1 scorecard** (`internal/harness/scorecard`, `Taskfile.yml`). `task scorecard:tier1 --
  -url ... -model ... -query-prefix ...` boots the real components on NATS with any OpenAI-shaped embedding
  provider. It asks the 500 questions three ways (words, vectors, searchGraph's merge) and prints the model,
  dimensions, commits, timings and three rows. It exits non-zero below the floor. Sections are embedded by body only
  (design D7). Run by hand before each tier 1 release, never in `task verify`. The first run's output, and the floor
  set from it, land in this change.

## Non-goals

Merging in `globalSearch` or `localSearch`; reranking (#682); long-text splitting (#667); entity-text formatting
(#683); waiting on semembed#6 or the four-model rerun (decided after this lands); a real provider in `task verify`.

## Impact

`graph.GlobalSearchData.Fallback` → `.Merge` (JSON `fallback` → `merge`), and `graph.TextSearchMatch` gains
`WordRank`, `VectorRank`. Neither field is in a release yet. `natsfixture.New` takes a narrow interface; every caller
compiles unchanged. Docs: graph-query README, `doc.go`, MIGRATION.md.

## Retires, and in which pull request

PR A: `Addresses #668`. PR B: `Closes #668`, `Closes #666`. #11 addressed.

## Rulings needed

Where the scorecard runs, the floor values, and the reply shape (design, Open Questions).
