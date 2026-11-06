# Proposal: setup-04a-04-graph-roots-graph-query-communities

Approved-by: owner, 2026-11-05 in Enjoy: design at `48c1b89` approved, with question 1 answered "accept" in
`owner-questions.md` and the five new exports.

Track: `design` (adds spec requirements and touches three capabilities).

## Why

Step 3 of five of `processor/graph-query` (change 4 item 9, epic #9; `split.md` of the archived step 1 change). It
gives graph-query one honest read of `COMMUNITY_INDEX` and the search inside one entity's community, `localSearch`.
The owner: "our job when we port is to create a pragamtic, idiomatic solution while noting what consumer use cses we
need to support." The pin's version cannot be carried: it reads member lists the records no longer hold, guards them
with a generation, lease and fence (431 lines and test hooks, #406), loads up to 10,000 members a request, and falls
back to a semantic search when an entity has no community. Rulings 2 and 3 on PR #504 settle the read; the owner left
step 3 one choice, the field on the record saying how current it is.

## What Changes

- **`clustering.Community.EntityRevision`** (`entity_revision`): the `ENTITY_STATES` revision of the latest entity
  write applied to the group. A touched group is rewritten at the next tick, so a write that changes no count now
  writes its three records once. `clustering.GroupKey` and `clustering.DecodeCommunity` are exported, so graph-query
  finds and checks a record as the writer keys it and storage reads it.
- **A community view in graph-query:** one `graphview.View[clustering.Community]` over `COMMUNITY_INDEX`, run under
  graph-query's first run context (ADR-113). A missing bucket fails nothing: `localSearch` refuses, the other verbs
  answer. A lost watch refuses until a new view has caught up.
- **`localSearch`:** `entity_id`, `query`, `level` (0 to 2), `min_revision`. It reads E's group record from the view
  and one prefix page of the group, ranks the page by the pin's text score, and answers enveloped, `indexed_revision`
  being the record's `entity_revision`. `entities_truncated` says the page had a continuation or matches were left out
  to fit `max_payload`. It refuses `index_not_ready` with no view, or with no record while E exists; the entity verb's
  `entity_not_found` when both are absent; `internal` naming an unreadable record's key; `revision_not_reached` when
  `min_revision` is above the record's revision.
- A counter of truncated searches, the measurement #506 waits on (ruling 2). ADR-087 ported with a note (owner,
  ruling 6 of PR #504 comment 6585009466). Consumer breaks in `processor/graph-query/MIGRATION.md`.

Not here: the poison-counting list, labels and digests (step 5, with the text tier that reads them); the classifier and
`globalSearch` (4); `searchGraph` (5); the wiring of `COMMUNITY_INDEX`'s writer (change 7).

## Capabilities

- Added requirements: `graph-query` (local search, the community view, truncation count); `community-grouping` (the
  record's revision; one key and one decoder).
- Modified: `graph-query` (revision, counting); `community-grouping` (history, bounds); `graph-transport-boundary`
  (the table).

## Impact

- About 450 production and 1,000 test lines (hand estimate). Pin: about 880 and 820 (`graphrag.go:66-158`, `:279-400`,
  `:2218-2397`, `community_cache.go`, the supervisor in `component.go`, and their tests; `inventory-delta.md`). One PR,
  one developer unit per task.
- **New exported surface, for the owner to accept:** in `graph`, `LocalSearchRequest`, `LocalSearchData`,
  `MaxSearchTerms`; in `graph/clustering`, `Community.EntityRevision`, `GroupKey`, `DecodeCommunity`. Readers in
  `inventory-delta.md`. New subject `graph.query.localSearch`. No new bucket and no new config key.
- One owner question (`owner-questions.md`): a revision check on local search can keep refusing for a group nobody
  writes to.
- Retires #406, #522 and #527 (from Codex's task 3.2 review). No code before the owner approves.
