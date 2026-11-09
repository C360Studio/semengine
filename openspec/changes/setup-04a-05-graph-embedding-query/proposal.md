# Proposal: setup-04a-05-graph-embedding-query

Approved-by: Coby Leuschke (owner), 2026-11-08, in the Enjoy session; answers on PR #558 comment 6728991794

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

- `search` and `similar` answer in the verb table's envelope, with strict new request types (design D1, D3, D4).
- The vector cache runs on a `graphview.View`; an unreadable record is counted and heals (D2). The pin's bucket-scan
  fallback and `Storage.ListGeneratedEntityIDs` are not carried (D6).
- Calls are withheld while the index cannot answer (D5, owner question 3); `scope` filters before ranking (ADR-071).
- `searchGraph`: owner question 1; PR B changes nothing in graph-query.

## Capabilities

Modified `graph-embedding`, `embedding-index`; under F2, `entity-state-follower`. The deltas follow the recommended
answers below; another answer changes them before task 2.1.

## Impact

- Pin lines: 371 production, 676 test (`inventory.md` § 1); this PR about 500 + 900 (hand estimate). No new subject
  or bucket; exported surface is listed in D1, D2 and D5.
- Consumers migrate (`MIGRATION.md`, under the owner's 2026-11-02 ruling on PR #504): enveloped `data.results`,
  limits refused not clamped, unknown keys refused (changes ADR-071's compatibility note), score-0 hits gone.
- Merge order: no open PR overlaps; with #557's PR, whichever lands second merges `origin/main`. Retires none;
  advances #477 (stays open for the message logger); stays: inventory § 5.

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
   the shared readiness code; graph-index is unchanged (D5, F2). The status graph-embedding publishes
   still reads degraded, so if fusion waits on it, one bad document still stops fusion: PR C decides that.
4. **Two small changes from the old code:** a `limit` above 100 or below 0 is refused instead of quietly cut, and a
   hit with a score of 0 is dropped, so a query of only stopwords returns nothing instead of a list of unrelated
   entities. Recommended: accept both (D3).
