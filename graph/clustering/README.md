# graph/clustering

Groups the graph's entities by the prefix of their IDs and keeps one small record per group in the
`COMMUNITY_INDEX` bucket. A caller feeds the package entity changes; the package publishes only the groups that
changed. It reads no graph data itself, starts no goroutine and opens no `ENTITY_STATES` watch.

## What a community is

An entity ID has six parts, `org.platform.system.domain.type.instance` ([ADR-102](../../docs/adr/102-entity-id-segment-semantics.md)).
A *community* here is every entity whose ID starts with the same first parts. The *level* is how many parts:

| Level | Prefix | Example |
| --- | --- | --- |
| 0 | `org.platform.system.domain` | `acme.dep1.gcs1.robotics` |
| 1 | `org.platform.system` (the source) | `acme.dep1.gcs1` |
| 2 | `org.platform` (the deployment) | `acme.dep1` |

So one entity is in three communities, and the levels never overlap in name. Nothing is detected: the groups follow
from the IDs, which is the decision of [ADR-099](../../docs/adr/099-community-partition-derived-from-entity-identity.md).
The pin's label propagation is not carried.

The record for a group is a `Community`: its `ID` (the prefix), `Level`, `MemberCount`, `TypeCounts` (members by the
ID's fifth part, the 10 most common), `Keywords` (the 10 most frequent words in members' string values), a one-line
`StatisticalSummary` built from those counts, and `RepEntities` (up to 5 member IDs, the lexically smallest among
members that have words). It never lists the members, so its size does not grow with the group: every field is
capped, and the encoded record stays under `MaxRecordBytes` (8 KiB). The key is `{level}.{prefix}`, for example
`1.acme.dep1.gcs1`.

A record also carries `EntityRevision` (`entity_revision`), the latest entity write it reflects: the highest
`ENTITY_STATES` revision among the `Upsert` and `Delete` calls the maintainer applied for entities whose IDs fall in
the group. That counts every applied call: a delete of an entity the maintainer never held (such as a delete marker
replayed after a restart) and a refused value both raise it. A delivery at or below the revision already applied for
its key is not applied and does not raise it. The mark ends with the group: when the last member goes, the records
go, and the entity that recreates the group gives it that entity's revision. A raised record differs from the one
last written, so the next tick writes it. What it promises, and only when the caller feeds revisions in ascending
order across keys (below): every `ENTITY_STATES` write at or below `entity_revision` is reflected in the record. A
written record never carries 0.

To read one record, a reader finds its key with `GroupKey(entityID, level)`, which refuses a level outside 0 to 2 and an
ID the canonical parser refuses, and checks the value with `DecodeCommunity(key, value)`, which refuses a value that is
not JSON, a record whose level is not 0, 1 or 2, a record whose level and ID do not give that key, and `entity_revision`
0. `GetAllCommunities` checks each entry the same way.
To read every record at one level, a reader lists the keys that start with `LevelPrefix(level)`, the level and a dot,
which refuses a level outside 0 to 2.

A *word* is a run of 3 to `MaxWordBytes` (32) ASCII letters and digits in a string value, ignoring case and a short
list of stop words. A longer run is not a word, and nothing is cut. IDs, predicate names and the objects of
relationships (other entity IDs) add no words. Each entity adds at most its `MaxEntityTerms` (32) most frequent words
to each of its groups, so a word that is outside the top 32 of every member cannot become a keyword.

## Basic use

You give the maintainer each change to an entity and say when the first read of the bucket is complete. Then you call
`Tick` on an interval.

```go
// kv is the COMMUNITY_INDEX bucket, from kvcatalog.EnsureCatalogBucket(ctx, client, graph.BucketCommunityIndex).
storage, err := clustering.NewNATSCommunityStorage(client.NewKVStore(kv))
if err != nil {
    return err
}
m, err := clustering.NewMaintainer(storage)
if err != nil {
    return err
}

// For each delivery of your ENTITY_STATES watch: the revision is that of the value you decoded.
if err := m.Upsert(entityID, revision, state); err != nil {
    // a refused entity: see "Failure signals"
}
// For a delete or purge of a key:
_ = m.Delete(entityID, revision)

// After the watch's initial replay has been applied. With a raw WatchAll that is the nil entry on its
// updates channel, the marker graph/follower already relies on (graph/follower/follower.go:227).
m.BootstrapDone()

// On an interval, from any goroutine:
err = m.Tick(ctx)
```

`graph/clustering/maintainer_product_integration_test.go` (`TestMaintainerAsAProduct`) runs exactly this against a
real NATS server and reads the records back with `storage.GetAllCommunities(ctx)`.

What the caller owes the maintainer:

- **The revision of the exact value.** `Upsert(entityID, revision, state)` takes the KV revision of the very value
  `state` was decoded from. Do not read the entity again and pair the new value with an older revision.
- **Increasing revisions per key.** Deliveries for one key must arrive in increasing revision order, as from one
  ordered watch. The maintainer ignores a delivery at or below the revision it already holds for that key, and that
  is its only defence; it cannot reorder. This is the caller's precondition, not something the package checks across
  keys or tests against a real watch.
- **Ascending revisions across keys, for `entity_revision` to hold.** `entity_revision`'s promise needs every
  delivery, whatever its key, to arrive in increasing revision order. Fed out of order across keys, the records'
  counts and words still come out right, but a record can name a revision above a write it does not yet reflect.
  That a KV watch delivers this order is not promised here:
  [ADR-066](../../docs/adr/066-honest-graph-index-readiness.md) states it without a probe. `processor/graph-clustering`,
  which wires the watch, probes it at the pinned nats.go and nats-server versions (`TestWatchOrderAcrossKeys`). The
  order it observed is an observation, not a promise
  ([ADR-112](../../docs/adr/112-natsclient-contract.md)).
- **Revision 0 is refused.** A KV entry's first revision is 1 (`TestKVRevisionStartsAtOne` probes it on the pinned
  nats.go and nats-server; it is not promised beyond that), so 0 means the caller passed none.
- **`BootstrapDone` once, after the replay.** `Tick` before it returns `ErrBootstrapIncomplete` and writes nothing,
  because a record built from half a replay would be published as if it were the graph.

The methods are safe to call from the watch goroutine and a ticker goroutine at once. `Upsert` and `Delete` never wait
for the storage writes of a running `Tick`.

## What a tick costs

The maintainer keeps the groups in memory and updates them as changes arrive: a changed entity subtracts its old
contribution (one member, one count for its type, its words) and adds the new one. A restart rebuilds them from the
watch's replay, so the first tick after a start publishes every group once.

A `Tick` recomputes only the groups touched since their last publication, and writes a record only when it differs
from the one last written. After the first tick it reads nothing from `COMMUNITY_INDEX` and holds no handle on
`ENTITY_STATES`. A write that changes no count, word or representative still raises `entity_revision`, so it writes
its entity's three records once. The cost of a group is in proportion to its distinct words (a top-k selection, not
a full sort), and it is paid while `Upsert` and `Delete` wait on the same lock. `TestTickCost` counts the storage
calls: k changed entities cause writes only to their groups, and an unchanged graph writes nothing.

The first tick lists the bucket's keys once. After every changed record is written it removes the records of groups
that no longer exist (the groups that became empty, and keys left from an earlier run). If any write fails nothing is
removed, so the old records stay readable ([ADR-085](../../docs/adr/085-gate-on-health-report-freshness.md), decision
7: stale, never empty).

## Failure signals

- `ErrBootstrapIncomplete` (`errors.Is`): `Tick` before `BootstrapDone`.
- A storage error from `Tick`: a write, a removal or the first listing failed, or the context ended before the pending
  work was done. The work stays pending and the next `Tick` retries it. A `Tick` with nothing pending does not look at
  the context.
- `*IncompleteError` (`errors.As`): every group was published, but some entities are refused. `Refused` holds their
  IDs, sorted; `Error()` names the first ten. `Tick` returns nil only when all is published and nothing is refused.
- A refusal from `Upsert`, an invalid error naming the entity: a nil state, an ID that is not a valid 6-part entity
  ID, or a state whose ID differs from `entityID`. A refused entity contributes nothing and stays in the refused set
  until a valid `Upsert` or a `Delete` for it.
- `GetAllCommunities` fails, naming the key, on an entry it cannot read or that `DecodeCommunity` refuses (SemStreams
  skipped such entries), and returns the context's error if the context ends. An empty bucket is no records and
  no error.

## What is not here

- **Language-model summaries and the summary store** (SemStreams' `CommunitySummarizer`, `StatisticalSummarizer`,
  enhancement worker and `COMMUNITY_SUMMARIES`): tier 2. The record's `StatisticalSummary` is a
  template over the counts.
- **Label propagation** and the edge providers that fed it: removed by ADR-099. A detection overlay over chosen
  relationship predicates may return as a later change, off by default, in a namespace nothing reads for serving.
- **The watch and the ticker.** `processor/graph-clustering` follows `ENTITY_STATES` through `graph/follower`, whose
  entries carry the delivered value and its revision, calls `Upsert` and `Delete` with them, and runs `Tick` every
  `detection_interval` once the replay is applied. It does not read records back: `GetAllCommunities` served the
  pin's anomaly lane, which is never ported, and has no caller in this tree.
- **A reader's consistency rule.** Which generation of the bucket a reader sees during a tick is for the
  `processor/graph-query` item, not decided here.
- **A per-group member list and a membership hash.** Members are found through the prefix queries of the graph's
  query lane.

The decoder in semboids reads a member list from a record; it does not read these bounded records.
