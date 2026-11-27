# Inventory: fix-765-warm-reindex

base: 3d823c82 (`origin/main`). Read in this session; nothing run except the searches named. Measurements are
issue #765's step 2 probe (comment 7208287082; raw notes `notes/765-probe.md` at 24f3cc51 on branch
`claude/probe-765-reconcile-cost`), nats-server 2.15.0, nats.go v1.55.0, one run per size, Apple M3 Pro, load 4.6
to 12.

## 1. Why the listings exist (the claimed gap)

They find rows an entity owns that its current value no longer asks for, so replacement holds (graph-index spec,
"Indexes implement replacement"; ADR-077 §4).

- `processor/graph-index/owner_reconcile.go:46` — `existingKeys, err := bucket.KeysByFilter(ctx, ownerFilter)`
- `processor/graph-index/owner_reconcile.go:85` —
  `if _, alreadyStored := existing[key]; alreadyStored && !overwriteExisting {`
- `processor/graph-index/owner_reconcile.go:116` — `return c.reconcileOwnedRows(ctx, "predicate", c.predicateBucket,`
- `processor/graph-index/owner_reconcile.go:142` — `return c.reconcileOwnedRows(ctx, "incoming", c.incomingBucket,`
- `processor/graph-index/name_index.go:75` —
  `return c.reconcileOwnedRows(ctx, "name", c.nameBucket, nameIndexEntityFilter(entityID), desired, true)`
- `processor/graph-index/name_index.go:121` —
  `return c.reconcileOwnedRows(ctx, "alias", c.aliasBucket, aliasIndexEntityFilter(entityID), desired, false)`
- `processor/graph-index/index.go:606` — `keys, listErr := set.bucket.KeysByFilter(ctx, set.filter)` (the delete path,
  same four)
- `natsclient/kv.go:656` — each listing is a filtered watch, `IgnoreDeletes(), MetaOnly()`: one consumer created and
  deleted per listing (probe: exactly 4.00 consumers per entity at every size).

Claimed absence, "nothing persists what was indexed": searched `git grep -n "lastProjections\|computeIndexProjection"`
and the kvcatalog list. Only an in-memory baseline exists, for a metric:

- `processor/graph-index/component.go:89` — `lastProjections sync.Map`
- `processor/graph-index/index.go:498` —
  `projection := computeIndexProjection(state, c.namePredicates, c.isAliasPredicate)`
- `processor/graph-index/index.go:508` — `c.lastProjections.Store(entityID, projection)`

## 2. Index buckets and key shapes (every spelling of "the rows an entity owns")

Opened at `processor/graph-index/index.go:59-63`; catalog at `graph/kvcatalog/kvcatalog.go:169-174`, all derived, owner
graph-index. No production code outside graph-index names the five bucket constants (`git grep -n
"BucketOutgoingIndex\|BucketNameIndex\|BucketPredicateIndex\|BucketIncomingIndex\|BucketAliasIndex" -- '*.go'
':!*_test.go'`: only `graph/constants.go` and `kvcatalog.go`).

| Bucket | Key | Owner filter |
| --- | --- | --- |
| OUTGOING_INDEX | `entity6`, value the list | exact key, `index.go:564` — `outgoingBucket.Put(ctx, entityID, …)` |
| INCOMING_INDEX | `target6.source6.hex(p)` | `incoming_index.go:30` — `wildcardPositions(6) + "." + sourceID + ".*"` |
| PREDICATE_INDEX | `pred3.entity6` | `predicate_index.go:20` — `wildcardPositions(3) + "." + entityID` |
| NAME_INDEX | `sha256(name).entity6.hex(pred)`, value name | `name_index.go:40` — `"*." + entityID + ".*"` |
| ALIAS_INDEX | NAME's layout, empty value | `name_index.go:103` — `return nameIndexEntityFilter(entityID)` |

Other spellings of the fact: the follower watermark (revisions completed, memory, `internal/revlag/watermark.go:28`);
the failed set (memory, `index.go:307-313`); the damage record (memory, `graph/follower/follower.go:126`).

## 3. What a restart must guarantee today

- Replay re-reads current truth: `index.go:215` — `entry, err := c.entities.Get(ctx, entityID)`.
- Deletes while down arrive marked on a keys-only replay (`graph/follower/follower_integration_test.go:498`,
  `TestKeysOnlyEntriesCarryNoValue`, a probe at the pinned versions). A key whose tombstone was itself removed is
  never delivered, today and after: no change.
- Crash mid-write: today the next start lists and converges. Stop lets the write in flight finish and drops unstarted
  work (`index.go:193` — `if c.stopping.Load() {`; spec "Stop writes accepted work"). A kill gives no such promise.
- Vocabulary between boots: read once per start, `index.go:87` —
  `c.aliasPredicates = vocabulary.DiscoverAliasPredicates()`, `index.go:88` —
  `c.namePredicates = vocabulary.DiscoverLabelPredicates()`; products register their own (`vocabulary/labels.go:14`).
  An unchanged entity can need different rows after a restart.
- Damaged: `index.go:480` — `c.reportDamaged(entityID, revision, err)`, then its rows are removed; the record of
  damage is in memory and rebuilt by the replay. Failed: a failed write still completes its revision (`index.go:102`,
  `:233`); readiness is held by the failed set, which the repair loop re-drives every 30 s (`index.go:41`).
- A projection divergence: `index.go:383` (plan trims an alias) against `index.go:540` (projection does not), and
  name priority is in the plan but not the projection, so `computeIndexProjection` equality does not imply equal rows.

## 4. Readiness and watermark rules

`graph/follower/follower.go:278` — `f.bootstrapTarget.Store(f.watermark.Observed())` fixes the bootstrap target at
the snapshot marker; QueryReady refuses until the floor reaches it (`follower.go:433-461`); the envelope is degraded
on failed, damaged or lost feed (`follower.go:530`). Specs: graph-index "Startup-replay queries get a retryable
refusal", "Stop writes accepted work"; entity-state-follower "The initial build completes at the snapshot's
revision", "Every delivered revision reaches the watermark first".
Watermark: low-water of delivered revisions, ascending order checked not trusted (`internal/revlag/watermark.go:28-41`).
None of these changes; a skipped entity still completes its revision.

## 5. Open issues and PRs that overlap

`gh issue list --state open --search "graph-index"` and `gh pr list --state open`, 2026-11-26:

- #765 (this change, `Addresses`); the 105k plateau stays open. #108: this is its decision (resume point against skip
  under equality); retired by this change if the owner agrees. #750: other followers' buffers; untouched.
- #602 (generation next to `indexed_revision`): adjacent, untouched. #109 (follower adoption): untouched.
- #554 (await helpers' own deadline): the new integration tests must not add that shape.
- PR #774: this claim. PR #761 (nats.go 1.55.0 to 1.56.0, Dependabot): any probe cited here is at v1.55.0; merge
  order: either first, the 2.5 measurement records the version it ran on. Other open PRs (#760, #762-#764) are
  dependency bumps that touch no graph-index file.
- Names the word, not the package: #665, #730, #771, #111, #141, #110, #619, #485, #195, #68, #477, #196, #9.

## 6. Governing ADRs

- ADR-077 §4 (`docs/adr/077-bounded-owner-discovery-and-incoming-ownership.md:99`, "lists the stored owner rows") and
  Context `:40` ("without a central reverse manifest"): **changed** by the recommendation; owner question.
- ADR-066 §1 (watermark, failed-set gate): kept. ADR-065 (one writer per key, cited `incoming_index.go:19-21`): kept,
  the record has one writer. ADR-108 D4 (read current truth at apply): kept. ADR-109 D1, D4: kept. ADR-110 Q4 (an
  equal re-arrival raises the revision): kept; with the skip, graph-index's cost per equal re-arrival is two Gets.
- ADR-114 (alias layout): kept.

## Library statements

- Listing cost growing with store size: measured (0.59 ms at 200, 4.8 ms at 10k, 12.3 ms at 40k). Why (a server scan
  for leading-wildcard filters): **not promised**, not probed.
- That an acknowledged Put in one bucket implies earlier acknowledged Puts in other buckets survive a server crash:
  **not promised**, not probed.
- A record Get costing what an entity Get costs (0.24 ms): **assumed**, not measured.

## Collision table (a new durable primitive: per-entity record of indexed rows)

| Dimension | Evidence |
| --- | --- |
| Semantic class | which rows an owner wrote, so old ones can be retracted without a search |
| Owners | graph-index listing (`owner_reconcile.go:46`); TEMPORAL_INDEX_REVERSE, unported (`kvcatalog.go:179`) |
| Owners (cont.) | GRAPH_INGEST_APPLIED_SEQ, an applied mark per entity and stream (`kvcatalog.go:164`) |
| Catalogs | `graph/kvcatalog/kvcatalog.go:161-190` |
| Status | `reindex_events_total`, `kv_operations_total` (`metrics.go`); no status key |
| Lifecycle | none persisted; `lastProjections` lives one process |
| Ownership | one writer per entity key, its lane (ADR-065) |
| Readers | graph-index only (search above) |
| Writers | graph-index lanes only |
| Recovery | full re-derivation on every start (today) |

Adoption sweep: the shape is local to graph-index; one other owner listing with a leading wildcard exists,
`processor/rule/state_tracker.go:137` — `keys, err := st.kv.KeysByFilter(ctx, "*."+entityID)`, on delete only. Not a
shared primitive; no tracking issue proposed.

## Adopter seam

1. Must know: consumers nothing (no reply, subject or config change). A graph-index developer: bump the format
   version with any key or value change. An operator: deleting the record bucket forces a full rebuild.
2. Do nothing: a developer who forgets the bump fails a golden test (loud). An operator who hand-deletes a row while
   down gets a missing row until the entity changes (silent): the declared limit.
3. Find out: compile/test for the developer; doc only for the operator.
4. Should know: nothing; the gap is the operator's hand edit, Decision 3.
