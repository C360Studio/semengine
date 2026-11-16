# Proposal: setup-04c-02-hybrid-search

Approved-by: owner (Coby Leuschke) (2026-11-16), in Enjoy; recorded on #668 (comment 6921682966) sha256:69bceccc1c94

Track: `design`. Capabilities: `graph-query` (modified, added), `nats-fixture` (modified).

## Why

At tier 1 a question can be answered from two lists: the word index's and the embedding index's. searchGraph asks
the embedding index only when words find nothing (`processor/graph-query/textsearch.go:72`). On the frozen scorecard
a throwaway program merged the two lists, all of both, and found the answer in the first ten 62.4% of the time,
against 56.0% for words and 52.0% for vectors. No product could get that ranking. The owner ruled (2026-11-16, #668
and #666): build the merge in searchGraph first, then put tier 1's floor on the merged row, about 60%.

## What Changes

- **PR A, the merge** (`processor/graph-query`, `graph`). With `embedding_enabled` true, searchGraph asks the word
  search and the embedding search together, each 100 deep, and merges them by reciprocal rank: each entity scores
  the sum of 1 / (60 + its place) over the lists that hold it.
  - `text.matches` is the merged list, each match saying its `word_rank` and `vector_rank`.
  - `merge` (renamed from the unreleased `fallback`) says whether the embedding list was read, and why not.
  - A provider that is down leaves the word answer, labelled, never a refusal. Tier 0 and `globalSearch` are
    unchanged.
- **PR B, the tier 1 scorecard** (`internal/harness/scorecard`, `internal/harness/natsfixture`, `Taskfile.yml`).
  `task scorecard:tier1 -- -url ... -model ... -query-prefix ...` boots the real components on NATS with any
  OpenAI-shaped embedding provider.
  - It asks the 500 questions three ways (words, vectors, searchGraph's merge) and prints the model, dimensions,
    commits, timings and three rows.
  - It exits non-zero below tier 1's floor.
  - Sections are embedded by body only (design D7).
  - It is run by hand before each tier 1 release, never in `task verify`. Its first run sets the floor.
  - `natsfixture.New` takes a four-method interface so the program can start NATS from the one image pin.

## Non-goals

Merging in `globalSearch` or `localSearch`; reranking (#682); long-text splitting (#667); entity-text formatting
(#683); waiting on semembed#6 or the four-model rerun (decided after this lands); a real provider in `task verify`.

## Impact

`graph.GlobalSearchData.Fallback` becomes `.Merge` (JSON `fallback` becomes `merge`), and `graph.TextSearchMatch`
gains `WordRank` and `VectorRank`. None of these is in a release. The released change is at tier 1: alpha.5's
searchGraph answered as globalSearch. With `embedding_enabled` true, its order and `score` now come from the merge.
Every `natsfixture.New` caller compiles unchanged. Docs: `processor/graph-query/README.md`, `doc.go`,
`MIGRATION.md`, the query-pattern skill, `docs/repository-map.md`, `docs/test-infrastructure.md`.

## Retires, and in which pull request

PR A: `Addresses #668`. PR B: `Closes #668`, `Closes #666`. #11 addressed.

## Rulings needed

Narrowing `natsfixture.New`, the floor, and the reply shape (design, Open Questions).
