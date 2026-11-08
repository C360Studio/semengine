# Proposal: setup-04a-05-graph-embedding-query

Approved-by:

Track: `design` (adds spec requirements; touches `graph-embedding`, `embedding-index` and, under option F2,
`entity-state-follower`).

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
  `min_revision` is accepted; above 0 it is refused, since these replies claim no revision (design D4).
- The vector cache (`graph/embedding.VectorCache`) keeps its `Run` and `FindSimilar` and runs on a `graphview.View`:
  an unreadable record is counted and heals when rewritten, where today it switches the cache off for the run.
- The pin's bucket-scan fallback and `Storage.ListGeneratedEntityIDs` (its only reader) are not carried: a request
  before the cache has caught up is refused `index_not_ready`.
- Requests are withheld on a reset, a lost feed, the first build and a cache not caught up (ADR-109 D1); which failed
  entities withhold is owner question 3. A lost cache watch is a stuck state until restart.
- `scope` (ADR-071) is checked before the query is embedded and filters before ranking.
- `searchGraph`: owner question 1; PR B changes nothing in graph-query.

## Capabilities

- Modified `graph-embedding` (the verbs), `embedding-index` (the cache's authority on a view); under F2,
  `entity-state-follower` (an optional `Withhold` count). The spec deltas are written for the recommended answers to
  the owner questions; another answer changes them before task 2.1.

## Impact

- Pin lines: query path 371 production, 676 test (`inventory.md` § 1); this PR about 500 + 900 (hand estimate).
- No new subject or bucket. Exported: `graph.SemanticRequest`, `graph.SimilarRequest`, `graph.DefaultSimilarityLimit`,
  `graph.MaxSimilarityLimit`, `UnreadableRecords` and (F2) `FailedRecords` on `SemanticData` and `SimilarData`;
  (F2) `follower.Config.Withhold`; `VectorCache.FindSimilar`'s results; `Storage.ListGeneratedEntityIDs` removed.
- Consumer migration (`processor/graph-embedding/MIGRATION.md`): replies are enveloped under `data.results`; `limit`
  over 100 or below 0 is refused, not clamped; an unknown request key is refused, so `scope` sent to an older server
  no longer degrades to an unscoped search (changes ADR-071's compatibility note, under the owner's 2026-11-02
  ruling on PR #504); a hit scoring 0 is gone; `min_revision` above 0 is refused.
- Merge order: no open PR overlaps (#558 is this claim). #557's smoke test PR, when opened, and this PR: whichever
  lands second merges `origin/main` before review.
- Retires none. Advances #477 (the vector cache site; stays open for the message logger). Stays: inventory § 5.

## Owner questions (plain words)

1. **Should graph search fall back to word-similarity search when it finds nothing?** Recommended: yes, in a small
   graph-query PR right after this one, with the reply saying the fallback answered and never hiding an error. The old
   code did this; the boundary design we agreed in October kept it; the graph-query port left it for now. SemSource's
   `graph_search` tool tells its users when the fallback answered. The other choice is to drop it and leave combined
   search to fusion (design D7).
2. **Can a caller ask these searches to wait until its own write shows up?** Recommended: no. The searches never say
   how up to date they are; a caller that asks for a write to be included gets a plain "not supported" refusal, never
   an answer that silently misses it. The old code offered neither (D4).
3. **Should one document that fails to embed stop all search?** Today's shared readiness check would refuse every
   search while any entity's embedding has failed, and a document whose text cannot be extracted stays failed until
   it is written again. Recommended: no. Refuse only for the failures that can leave an old vector behind (and so a
   wrong hit); otherwise answer from the rest and report how many entities are missing. Costs one optional setting on
   the shared readiness code; graph-index is unchanged (D5, F2).
4. **Two small changes from the old code:** a `limit` above 100 or below 0 is refused instead of quietly cut, and a
   hit with a score of 0 is dropped, so a query of only stopwords returns nothing instead of a list of unrelated
   entities. Recommended: accept both (D3).
