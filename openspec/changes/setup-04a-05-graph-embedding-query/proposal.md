# Proposal: setup-04a-05-graph-embedding-query

Approved-by:

Track: `design` (adds spec requirements; touches `graph-embedding` and `embedding-index`).

## Why

Change 5 of epic #9 makes SemSource's composition build against SemEngine alone. This is PR B of four (`split.md` of
the archived `2026-11-08-setup-04a-05-first-consumer-graph-embedding`, accepted on PR #550 comment 6716972354): the
read path of `processor/graph-embedding`. After it, `graph.embedding.query.search` and `graph.embedding.query.similar`
answer in the tree's wire shape, so graph-query's `semantic` and `similar` forwards stop reporting no responder, and
the port of `processor/graph-embedding` is whole. The owner's #477 ruling (2026-11-08, PR #549 close-out) puts the
vector cache onto `pkg/graphview` in this port.

## What Changes

- `search` and `similar` are served on the subjects their `graph.QueryVerbs()` rows name, as a `graph.QueryResponse`
  whose `data` is `graph.SemanticData` or `graph.SimilarData`. New request types `graph.SemanticRequest` and
  `graph.SimilarRequest`, decoded strictly; `limit` 0 means 10, 1 to 100 is accepted, anything else is refused.
- The vector cache (`graph/embedding.VectorCache`) keeps its API shape and runs on a `graphview.View`: an unreadable
  record is counted and heals when rewritten, where today it withdraws the cache for the rest of the run.
- The pin's bucket-scan fallback and `Storage.ListGeneratedEntityIDs` (its only reader) are not carried: a request
  before the cache has caught up is refused `index_not_ready`.
- Requests are withheld through `follower.QueryReady` (ADR-109 D1). A lost cache watch is a stuck state until restart.
- `scope` (ADR-071) is checked before the query is embedded and filters before ranking.
- `searchGraph`: owner question 1 below; PR B changes nothing in graph-query.

## Capabilities

- Modified `graph-embedding` (the verbs), `embedding-index` (the cache's authority on a view).

## Impact

- Pin lines: query path 371 production, 676 test (`inventory.md` § 1); this PR about 500 + 900 (hand estimate).
- No new subject or bucket. Exported: `graph.SemanticRequest`, `graph.SimilarRequest`, `graph.DefaultSimilarityLimit`,
  `graph.MaxSimilarityLimit`, `UnreadableRecords` on `SemanticData` and `SimilarData`; `VectorCache.FindSimilar`'s
  results; `Storage.ListGeneratedEntityIDs` removed.
- Consumer migration (`processor/graph-embedding/MIGRATION.md`): replies are enveloped under `data.results`; `limit`
  over 100 or below 0 is refused, not clamped; an unknown request key is refused, so `scope` sent to an older server
  no longer degrades to an unscoped search (changes ADR-071's compatibility note, under the owner's 2026-11-02
  ruling on PR #504); hits with no shared term are gone.
- Merge order: no open PR overlaps (#558 is this claim). #557's smoke test PR, when opened, and this PR: whichever
  lands second merges `origin/main` before review.
- Retires none. Advances #477 (the vector cache site; stays open for the message logger). Stays: inventory § 5.

## Owner questions (plain words)

1. **searchGraph's semantic fallback.** At the pin, `searchGraph` answered from the embeddings when global search
   found nothing or the community index was missing; SemSource's `graph_search` tool reports that rung. The accepted
   03B design kept it; change 4 left it out until now. Recommended: bring it back in a small graph-query PR right after
   this one, labelled in the reply, never hiding the embedding refusal (design D7, option S1).
2. **Read-your-writes on the embedding verbs.** Recommended: the reply's `indexed_revision` is 0 ("claims nothing")
   and a request naming `min_revision` is refused by name, as the pin offered neither (D4).
3. **One failed embedding withholds both verbs** until it is fixed, as graph-index does with its failures and as
   `Health` already reads it. Recommended: keep (D5); the alternative needs a new follower hook.
4. **Behaviour changes from the pin:** an out-of-range `limit` is refused, not clamped; a hit that shares no term with
   the query (score 0) is dropped. Recommended: accept both (D3).
