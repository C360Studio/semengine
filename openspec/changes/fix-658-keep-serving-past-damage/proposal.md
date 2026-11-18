# Proposal: fix-658-keep-serving-past-damage

Approved-by: cglusky (2026-11-18) <https://github.com/C360Studio/semengine/issues/658#issuecomment-6981220198> sha256:12d098cd7eaf

Track: `design`. Capabilities modified: `entity-state-follower`, `graph-index`, `graph-embedding`, `graph-lexical`,
`graph-clustering`, `graph-readiness`, `graph-transport-boundary`, `graph-query`, and with Q3 `fusion` and
`graph-gateway`.

## Why

One damaged entity in `ENTITY_STATES` (a stored value the stored-value rule refuses) stops graph-index,
graph-embedding and graph-lexical for the life of the process: every request is refused fatal
`graph_state_reset_required`, and a restart replays the same value and stops again. A damaged value already stored at
start also stops graph-clustering's publication. The owner ruled that the components keep serving (#657 ruling 3,
moved up to now on #658, 2026-11-18), and that "a damaged record is never left out of a reply without the reply saying
so" (#657 ruling 1).

## What Changes

- The shared watcher (`graph/follower`) records each damaged entity by key instead of stopping. The entry clears when
  a valid value or a delete for that key is applied. No refusal reason stops a component any more.
- graph-index, graph-embedding, graph-lexical and graph-clustering leave the damaged entity out: rows built from its
  last good value are removed (Q1), and its revision completes, so the first build finishes.
- Every enveloped reply carries `damaged_entities`, the count it may have left out. A request about the damaged entity
  itself (`outgoing`, `similar`) is refused as graph-ingest's entity read refuses it.
- graph-query passes the count on in its own replies (Q2: and marks its searches `truncated`); fusion and the GraphQL
  gateway pass it on too (Q3).
- Health names up to 10 damaged entities and how to repair them; the readiness envelope reads `degraded` with a
  `damaged_entity` count. A repair is picked up without a restart. The `reset_required` readiness state, which nothing
  produces any more, is removed, with `IndexStatusResponse.Code` and `Reason`.
- ADR-108 and ADR-109 are amended; ADR-074 and ADR-084 get a note. Docs and the ledger follow.

## Questions for the owner

- **Q1. What the components keep from a damaged entity's last good value.** Recommended: nothing. Its rows are removed
  and the reply's count says one entity was left out. The other choice keeps answering from the old value, which the
  store no longer holds.
- **Q2. Searches while an entity is damaged.** Recommended: graph-query's word and graph searches say `truncated`
  while any entity is damaged. semsource reads only `truncated`, so it learns of the gap with no change of its own.
  The cost: a global search counts damage anywhere, so one damaged entity makes every global search say `truncated`
  until it is repaired.
- **Q3. Fusion and the GraphQL gateway.** Both keep only a reply's data today, so the count would stop there.
  Recommended: carry it through both in this change and merge the serving change last, so no answer ever leaves a
  damaged entity out unsaid. For fusion this changes two methods of the interface semsource's test double implements
  (`Neighbors`, `Names` return a small struct with the count), so semsource's `MemGraph` must be updated when it moves
  to this release. Faster: graph-query only, with an issue for the two; fusion answers and GraphQL replies would omit
  a damaged entity unsaid until then.

## Out of scope

graph-ingest's verbs and poison inventory; refusal detail; the embedding count's scope.

## Impact

Packages: `graph`, `graph/follower`, `processor/graph-index`, `processor/graph-embedding`, `processor/graph-lexical`,
`processor/graph-clustering`, `processor/graph-query`, and with Q3 `pkg/fusion`, `pkg/fusion/fusionnats`,
`gateway/graph-gateway`. Three PRs, merged in order: the graph-query relay, then fusion and the gateway (each a new
draft when claimed), then the serving change on draft #693, which closes #658 and carries the archive. No other open PR
overlaps.
