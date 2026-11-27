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
| NAME_INDEX | `hash(name).entity6.hex(pred)`, value name | `name_index.go:40` — `"*." + entityID + ".*"` |
| ALIAS_INDEX | NAME's layout, empty value | `name_index.go:103` — `return nameIndexEntityFilter(entityID)` |

Other spellings of the fact: the follower watermark (revisions completed, memory, `internal/revlag/watermark.go:28`);
the failed set (memory, `index.go:307-313`); the damage record (memory, `graph/follower/follower.go:126`).
The follower holds ENTITY_STATES's generation: `follower.go:546-556`, `readStream` returns the stream's creation time.

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
- SemSource re-offers its full seed on every boot (#765 body: ENTITY_STATES `last_seq` about 302k for 41.5k entities
  "because every boot re-offers the full seed"; SemSource is "separately cutting our own churn"). graph-ingest's
  stream lane stores an equal re-offer as a new revision: it declines a write only when every set is older
  (`processor/graph-ingest/entity_writes.go:238-240`) and stamps `existing.UpdatedAt = time.Now()` (`:254`); ADR-110
  Q4. Not known: whether a boot's re-offers reach ENTITY_STATES before or after graph-index's snapshot marker, so
  whether they count toward the first build or arrive as live writes after it.
- Damaged: `index.go:480` — `c.reportDamaged(entityID, revision, err)`, then its rows are removed; the record of
  damage is in memory and rebuilt by the replay. Failed: a failed write still completes its revision (`index.go:102`,
  `:233`); readiness is held by the failed set, which the repair loop re-drives every 30 s (`index.go:41`).
- Name priority is in the plan (`index.go:389-391`) and in NAME's stored value, but not in `computeIndexProjection`
  (`index.go:534-537`), so equal projections do not imply equal rows. (The alias trim differs too, `index.go:383`
  against `:540`, but only gives extra work, never a wrong skip.)

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

- #765 (this change, `Addresses`); the 105k plateau stays open. #108: the resume point stays there (ruling item 2),
  decided by measurement after the record lands. #775 (host power cut) and #776 (ported ADRs on outside evidence):
  filed from the ruling. #750: other followers' buffers; untouched.
- #602 (generation next to `indexed_revision`): reads the generation the record stores (collision table). #109
  (follower adoption): untouched.
- #554 (await helpers' own deadline): the new integration tests must not add that shape.
- PR #774: this claim. PR #761 (nats.go 1.55.0 to 1.56.0, Dependabot): any probe cited here is at v1.55.0; merge
  order: either first, task 2.8's measurement records the version it ran on. Other open PRs (#760, #762-#764) are
  dependency bumps that touch no graph-index file.
- Names the word, not the package: #665, #730, #771, #111, #141, #110, #619, #485, #195, #68, #477, #196, #9.

## 6. Governing ADRs

- ADR-077 Context `docs/adr/077-bounded-owner-discovery-and-incoming-ownership.md:40-42`: owner listing is approved
  "provided that ... real-NATS evidence proves the leading-wildcard shape remains bounded under the production
  workload". The port note says the pin's load tests are not ported. Step 2 measured one listing at 0.6 ms with 200
  entities and 12 ms with 40,000, so the condition does not hold. **Amended by this change (owner ruling, PR #774
  comment 7217731180):** §2 (owner discovery) becomes the fallback and test oracle; §4 (`:99`, "lists the stored
  owner rows") diffs against the owner's record. §1's matrix and §5's source-owned INCOMING stand. §6 `:130`
  (baseline only after every projection succeeds) is the clean record's write rule. #776 lists other ported ADRs
  resting on evidence not in this repository.
- ADR-066 §1 (watermark, failed-set gate): kept; a skip completes its revision and clears the failed set.
- ADR-065 (one writer per key) is not ported; its invariant is cited in code (`incoming_index.go:19-21`) and the record
  keeps it, one lane per entity key.
- ADR-108 D4 (read current truth at apply): kept, the skip reads the entity and plans it. ADR-109 D1, D4: kept.
- ADR-110 Q4 (an equal re-arrival raises the revision): kept; graph-index now skips such an entity, so its cost is
  paid down without changing graph-ingest. The resume point is out of scope (ruling item 2; #108).
- ADR-114 (alias layout): kept.

## Library statements

- Listing cost growing with store size: measured (0.59 ms at 200, 4.8 ms at 10k, 12.3 ms at 40k). Why (a server scan
  for leading-wildcard filters): **not promised**, not probed.
- nats-server's file store flushes on a timer: `defaultSyncInterval = 2 * time.Minute`
  (`server/filestore.go:340`, module `github.com/nats-io/nats-server/v2 v2.15.3`, the version `go.mod:17` pins; a
  source read, not a probe; the 2.15.0 test image was not read). Writes are not synced per Put unless the stream
  syncs always. That a later acknowledged Put in one bucket implies earlier ones in other buckets survive a host power
  loss: **not promised**.
- A record Get costing what an entity Get costs (0.24 ms), and a record put half the measured two-put writes
  (0.3 ms): **assumed**, not measured.
- The largest value a KV put accepts: the server's reported payload limit, observed through
  `natsclient/client.go:358` — `func (m *Client) MaxPayload() (int64, error) {`. What nats.go returns on an oversize
  put is **not promised**, so the design compares the encoded size with the observed limit before the put.
- A host power cut: declared, the whole store's question, #775 (ruling item 3).

## Collision table (a new durable primitive: a per-entity record of the rows written)

| Dimension | Evidence |
| --- | --- |
| Semantic class | which rows an owner wrote, so old ones are retracted and an unchanged apply skipped, unlisted |
| Owners | graph-index listing (`owner_reconcile.go:46`); TEMPORAL_INDEX_REVERSE, unported (`kvcatalog.go:179`) |
| Owners (cont.) | GRAPH_INGEST_APPLIED_SEQ, applied mark per entity and stream, operational (`kvcatalog.go:164`) |
| Owners (cont.) | embedding's record: revision and embedder identity, a restart skip (`embedding/storage.go:251`) |
| Generation | stream creation time as generation: `processor/graph-ingest/consume.go:80-92`, `follower.go:546-556` |
| Limits | restored stream declared: entity-state-follower spec "A restored stream is a declared limit"; |
| Limits (cont.) | graph-ingest-recovery "A recreated or restored stream can be mistaken for a redelivery" |
| Catalogs | `graph/kvcatalog/kvcatalog.go:161-190` |
| Status | `reindex_events_total`, `kv_operations_total` (`metrics.go`); no status key |
| Lifecycle | none persisted; `lastProjections` lives one process |
| Ownership | one writer per entity key, its lane (ADR-065's invariant; that ADR is not ported) |
| Readers | graph-index only (search above) |
| Writers | graph-index lanes only |
| Recovery | full re-derivation on every start (today) |

The design adopts the existing shapes. TEMPORAL_INDEX_REVERSE (declared, unported) is the same "record what was
written" shape for one row. From embedding: the stored skip, keyed on the rows rather than a revision, because an
equal re-offer gets a new revision (§3); no compare-and-set, since one lane writes each key (embedding's guard is
one, `storage.go:249-250`); and the length-prefixed encoding of `graph/embedding/dedup.go:79-90`. It takes the
generation from creation time, with the same declared restored-stream limit. #602 (generation next to
`indexed_revision`) reads the same ENTITY_STATES generation the record stores; neither blocks the other.

Adoption sweep: the shape is local to graph-index; one other owner listing with a leading wildcard exists,
`processor/rule/state_tracker.go:137` — `keys, err := st.kv.KeysByFilter(ctx, "*."+entityID)`, on delete only. Not a
shared primitive; no tracking issue proposed.

## Adopter seam

1. Must know: consumers nothing (no reply, subject or config change). A graph-index developer: bump the format
   constant when changing how rows are planned, keyed or cleaned (a golden fixture catches encoding changes; a
   cleanup fix is review only). An operator: one reset, delete the record bucket while graph-index is stopped,
   after a hand edit, an in-place purge, a backup restore or an older graph-index; a power cut is #775.
2. Do nothing: the operator gets missing or stale rows until the entity changes (silent): Decision 3.
3. Find out: doc only, for the operator.
4. Should know: nothing; the gap is Decision 3, put to the owner with a background check as option (b).
