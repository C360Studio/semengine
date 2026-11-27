# graph-index

The relationship index component of the graph subsystem.

## Overview

`graph-index` follows the `ENTITY_STATES` bucket, which graph-ingest writes, and keeps five index buckets in step with
it so that a caller can ask "what does this entity point to", "what points at it", "which entities carry this
predicate" or "which entity has this name" without reading every entity. It answers those questions as request/reply
verbs on `graph.index.query.*`.

A *revision* is the number NATS gives each write to a bucket. A *watermark* is the highest revision graph-index has
applied with nothing below it still waiting. Every start reads every entity in `ENTITY_STATES` again, so after a start
it is not ready until it has caught up (see Query refusals). An entity whose index rows are already current is read and
skipped, not written again: graph-index keeps a record of the rows it wrote for each entity (see The record).

## Architecture

```text
                                       ┌─► OUTGOING_INDEX      (KV)
ENTITY_STATES ──► graph/follower ──►   │─► INCOMING_INDEX      (KV)
   (KV watch)      (the one watch,     │─► ALIAS_INDEX         (KV)
                    the watermark,     │─► PREDICATE_INDEX     (KV)
                    GRAPH_STATUS)      │─► NAME_INDEX          (KV)
                                       └─► GRAPH_INDEX_RECORDS (KV, the record of each entity's rows)
                                              ▲
graph.index.query.* (request/reply) ──────────┘ read by the eight verbs (not the record)
```

`graph/follower` owns the watch on `ENTITY_STATES`, the replay, the watermark and the readiness record it writes to
`GRAPH_STATUS` under the key `graph-index`. graph-index takes each entry from it, merges entries that arrive close
together (`coalesce_ms`), and applies the current stored state of each entity on a pool of lanes that runs entries of
one entity in order (`internal/dispatch`).

## Indexes

| Index | Key | Value | Answers |
| --- | --- | --- | --- |
| `OUTGOING_INDEX` | the entity ID | the entity's outgoing relationships | what the entity points to |
| `INCOMING_INDEX` | `target.source.hex(predicate)` | none | what points at the target |
| `ALIAS_INDEX` | `hash(alias).entity.hex(predicate)` | none | which entity carries an alias |
| `PREDICATE_INDEX` | `predicate.entity` (a 3-part predicate, a 6-part entity ID) | none | which entities carry a predicate |
| `NAME_INDEX` | `hash(name).entity.hex(predicate)` | the name as written and its label priority | which entities carry a name |

`NAME_INDEX` is graph-index's own and is not a configurable output; the other four are the configured outputs. A
membership key encodes who owns it, so an update replaces exactly the rows its entity owns: the layouts are
ADR-077 and ADR-078 (the alias layout is ADR-114). An alias is trimmed and case-folded before it is hashed, as a
name is, so `SN-1` and ` sn-1 ` are one alias. Any non-empty string is indexed, including one that is not a valid NATS
KV key (for example `two words`): the key holds the hash, not the alias. The alias predicates are discovered once when
graph-index starts; a predicate registered after that is not indexed until the next start.

## The record

`GRAPH_INDEX_RECORDS` holds one entry per entity, under its ID: the *record* of the index rows graph-index wrote for it.
It names each row by its bucket, its key and the SHA-256 of its value, and it carries the creation time of
`ENTITY_STATES` and of each index bucket at the start that wrote it (its *generations*) and a format name. Only the
entity's lane writes it, and only graph-index reads it; no query reads it. It is internal, like `NAME_INDEX`, and is not
a configurable output.

A record is *clean* when it names exactly the rows the last completed apply wrote, and *pending* while an apply is
writing: before its first row write an apply puts a pending record naming every row the entity may own (what the
earlier record named, or a listing found, plus the rows it is about to write), and after its last write it puts the
clean record. So whenever a record exists it names every row its entity can own, however many applies in a row are
cut off. Every record write is conditional on the revision of the record the apply read, so a record another writer
changed in between fails the apply before any row is written; the repair loop then retries from the stored record.

What an apply does:

- **The record equals the planned rows.** Nothing is listed or written, and the entity counts as applied. This is
  what a restart over an unchanged store, and a producer writing an entity again with equal statements, cost: one read
  of the entity and one of its record.
- **The record differs.** The rows the plan drops are deleted, and the rows it adds or whose value changed are put.
- **No usable record.** There is none yet (the first start over a store, or one indexed by a graph-index that kept no
  records), it does not decode, it has another format name, or its generations are not this start's (an index bucket
  or `ENTITY_STATES` was deleted and created again). graph-index then lists the entity's rows in each index bucket, as
  it did before it kept records, and writes them and the record from that listing. A store with no records is listed
  once per entity and recorded; a recreated bucket is rebuilt entity by entity, and a stop partway through loses
  nothing.
- **The record would not fit in one message.** If the pending or clean record is longer than the payload limit the
  NATS server reports (`max_payload`) less 1,024 bytes kept for the subject and headers of its put,
  graph-index deletes the stored record before any row write and applies the entity by listing, with no record. Every
  later apply of that entity lists again until its record fits. Each such apply logs at Info and counts
  `reconcile_operations_total{index_type="record",operation="too_large"}`. Under a 1 MiB limit only an entity with
  thousands of rows reaches this: a relationship is one row of about 150 to 250 bytes in the record for typical IDs
  and predicates. That a record of exactly the limit less 1,024 bytes is accepted is shown on the pinned server by
  `TestOversizeRecordFallsBackToListing`; how many bytes the subject and headers take is not promised.

**Size.** The record bucket holds every index key a second time: each row costs its bucket name, its key and 48
bytes (two 8-byte lengths and the 32-byte hash), and each record about 230 bytes more for its format name and
generations. One empty record stays for every entity ever deleted, so the bucket grows with every entity
`ENTITY_STATES` has held, not only the live ones.

**The format name.** The record's format name changes in two cases only: the record's encoding changed, or a release
fixes a defect that left rows no record names. A release that plans or keys rows differently, or changes the alias or
name predicates, needs no change: the record holds each row's literal bucket, key and value hash, so the new plan
differs from it and the old rows are deleted. A changed format name makes every record unusable, so the next start
lists every entity once: about 38 minutes for 40,000 entities on one lane, inferred from one measurement on one host
(a listing costs more as the store grows).

## Query verbs

Every verb is declared in `graph.QueryVerbs()` (`graph/query_verbs.go`) with its subject; spell no subject as a
literal. All eight are on `graph.index.query.` followed by the name.

| Verb | Request | Reply `data` |
| --- | --- | --- |
| `outgoing` | `{"entity_id": ...}` | `graph.OutgoingRelationshipsData` |
| `incoming` | `{"entity_id": ...}` | `graph.IncomingRelationshipsData`, sorted by source then predicate |
| `alias` | `{"alias": ...}` | `graph.AliasData`: the one live entity carrying the alias, or no ID |
| `predicate` | `{"predicate": ..., "value": ..., "limit": ...}` (`value` and `limit` optional) | `graph.PredicateData` |
| `predicateList` | `{"namespace": ...}` (optional) | `graph.PredicateListData` |
| `predicateStats` | `{"predicate": ..., "sample_limit": ...}` | `graph.PredicateStatsData` |
| `predicateCompound` | `{"predicates": [...], "operator": "AND" or "OR", "limit": ...}` | `graph.CompoundPredicateData` |
| `byName` | `{"name": ..., "limit": ...}` | `graph.NameData` |

Every verb but `outgoing` reads index rows a request lists and leaves out a stored row that is not its index's layout,
answering from the well-formed rows. Its `data.unreadable_records` says how many rows that request left out, `0` when
none; each one is also logged and counted in `query_rows_skipped_total`. A row deleted after the listing is not counted.

A successful reply is a `graph.QueryResponse`:

```json
{"data": {}, "indexed_revision": 1234, "producer": "graph-index", "timestamp": "2026-10-27T12:00:00Z"}
```

`indexed_revision` is the watermark read before the handler's first bucket read, so every `ENTITY_STATES` write at or
below it is reflected in `data`. A caller holding the `kv_revision` of its own write compares it with this number to
know whether the answer includes that write, for an `alias` reply too. A refusal is a classified error and carries no envelope.

### Reading at a revision

Every verb accepts an optional `min_revision`, the `kv_revision` of a write the answer must include
(`graph.MinRevisionField`, a number; `0` or absent means any). If the indexed revision is below it, the verb refuses
with transient `revision_not_reached` (`graph.ErrorCodeRevisionNotReached`), the revision reached in the detail. The
verb does not wait; the caller retries. In `graph.QueryVerbs()` all eight verbs are marked `Enveloped`, the mark that
says a reply is a `graph.QueryResponse`; `graph-query`'s `byName`, `predicate`, `entityByAlias` and `relationships`
pass the field through.

A malformed request is refused as `invalid_request` before any readiness check, so a bad request fails the same way
during a rebuild.

### Query refusals

Every one of the eight verbs refuses:

- with the transient code `index_not_ready` during the startup rebuild, until the watermark first reaches the
  `ENTITY_STATES` revision that was newest when the rebuild's snapshot ended. Once reached, the index stays ready; a
  later lag does not refuse queries;
- with the transient code `index_not_ready` for as long as the follower has lost its feed (below), and while index
  writes that failed are unresolved.

A stored `ENTITY_STATES` value that cannot be decoded makes that one entity *damaged* (ADR-074). The index removes the
rows it built from the entity's earlier value and goes on indexing every other entity. A damaged entity does not stop
the verbs:

- every reply counts every damaged entity in `damaged_entities` (omitted at 0), since any of them may be one it left
  out. The reply's other rows are right;
- `outgoing` of a damaged entity is refused with the fatal code `graph_state_reset_required`, because the caller named
  the entity the index cannot answer for;
- a valid write of the entity or a delete of its key clears the damage with no restart.

A caller treats `index_not_ready` as retry later and `graph_state_reset_required` as do not retry.

### Alias lookups

The `alias` verb answers the one live entity that carries the alias, under any number of alias predicates. If no
entity carries it, the reply holds no ID. If two or more entities do, the verb refuses with class `invalid` and code
`alias_ambiguous` (`graph.ErrorCodeAliasAmbiguous`), the message naming the alias; it answers again once all but one
holder has dropped the alias or been deleted and graph-index has indexed that change. An alias that is empty after
trimming is refused as `invalid_request`. A stored `ALIAS_INDEX` row that is not in the layout above is left out of the
answer, logged, counted in `query_rows_skipped_total` under `alias`, and counted in the reply's `unreadable_records`.

## Configuration

```json
{
  "type": "processor",
  "name": "graph-index",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "config": {"kind": "kv-watch", "bucket": "ENTITY_STATES"}}
      ],
      "outputs": [
        {"name": "outgoing_index", "config": {"kind": "kv-write", "bucket": "OUTGOING_INDEX"}},
        {"name": "incoming_index", "config": {"kind": "kv-write", "bucket": "INCOMING_INDEX"}},
        {"name": "alias_index", "config": {"kind": "kv-write", "bucket": "ALIAS_INDEX"}},
        {"name": "predicate_index", "config": {"kind": "kv-write", "bucket": "PREDICATE_INDEX"}}
      ]
    },
    "workers": 1,
    "coalesce_ms": 0
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the ports above | Inputs and outputs. A `ports` block that is present is used as written; one that is absent takes the defaults |
| `workers` | int | 1 | Index lanes, 1 to 16. `0` means the default, one lane |
| `coalesce_ms` | int | 0 | Window in milliseconds for merging updates to one entity; 0 applies each at once |
| `startup_attempts` | int | 30 | Tries to find `ENTITY_STATES` at startup |
| `startup_interval_ms` | int | 500 | Pause between those tries |

The configuration is decoded strictly: a key not listed here is refused at construction, naming it. This includes
`batch_size`, which the SemStreams version accepted and nothing read; a configuration that still sets it fails at boot
until the line is deleted. The one input must be a `kv-watch` of every `ENTITY_STATES` key with no history, and the
four outputs must be exactly the four index buckets above; an input or output that is anything else is refused.

## Ports

| Direction | Name | Kind | Bucket |
| --- | --- | --- | --- |
| input | `entity_watch` | kv-watch | `ENTITY_STATES` |
| output | `outgoing_index` | kv-write | `OUTGOING_INDEX` |
| output | `incoming_index` | kv-write | `INCOMING_INDEX` |
| output | `alias_index` | kv-write | `ALIAS_INDEX` |
| output | `predicate_index` | kv-write | `PREDICATE_INDEX` |

## Health and readiness

`Health` is `Healthy` once started, and unhealthy while any of these holds: a stored value could not be decoded (a
damaged entity), index writes are unresolved, or the `ENTITY_STATES` feed is lost. Its `Stuck` text names the count, up
to 10 damaged entity IDs with the reason for each, and the repair: write a valid value for the key or delete it. The
error count is reported and never lowers `Healthy` by itself (ADR-109). The `ENTITY_STATES` watch carries keys only,
and graph-index reads each entity itself to apply it. `DataFlow` counts one input per successful entity read, with the
byte length of the value read: writes merged into one apply count once, and a delete, whose read finds nothing,
counts none. A key the KV key check refuses is never read and counts none; a key that passes it but is not a literal
entity ID is read, counted, and then reported damaged. `watch_events_total` counts every delivered entry by kind.

The follower publishes the readiness record to `GRAPH_STATUS` at once and then every heartbeat, with `indexed_revision`,
`target_revision` and `failed_count`. The state reads `degraded` when index writes are unresolved, a damaged entity is
stored (`failed_reasons` then holds `damaged_entity`, the count of them, with no IDs), the feed is lost, or
the follower has been behind for 30 seconds with no completed write. Any completed write counts, so one entity held
while others finish reads not ready, not degraded.

### Lost feed

The follower declares the feed lost when the watch cannot start, when it closes while the component is running, or
when `ENTITY_STATES`' backing stream is a different stream from the one it bound (the bucket was deleted and created
again). Nothing rebinds the watch: the state holds until the process restarts. How long a watch stalls after a broker
restart (about 20 s in one probe of nats.go v1.54.0) and when a deleted bucket closes it (about 18 s in another) are
that library's timings and are not promised here. A bucket deleted and created again within one stall of the watch is
caught at the next heartbeat; a stream restored with the same creation time is not.

## Metrics

Collectors register on the injected `metric.MetricsRegistry`, never on Prometheus' global registry. Names are
`semengine_graph_index_<name>`:

| Name | Type | Meaning |
| --- | --- | --- |
| `events_processed_total` | counter | `ENTITY_STATES` entries processed |
| `updates_total` | counter vec | index updates by `index_type`; `alias` counts once per entity indexed or deleted |
| `kv_operations_total` | counter vec | KV operations by `operation` and `kv_bucket`; `kv_bucket="record"` counts record reads, puts and deletes, and `operation="list"` the owner listings |
| `watch_events_total` | counter vec | watch events by `event_type` |
| `write_failures_total` | counter | entities whose required index writes failed after retry |
| `reindex_events_total` | counter vec | applies by `result`: `unchanged` when the record already held the planned rows, else `changed` |
| `reconcile_operations_total` | counter vec | row and record operations by `index_type`, `operation`, `result`; `index_type="record"` counts record puts, deletes and `too_large` applies |
| `query_rows_skipped_total` | counter vec | stored index rows a query left out, by `index_type` |

The follower adds `readiness`, `lag`, `bootstrap_complete`, `readiness_state`, `status_publish_failures_total`,
`indexed_revision` and `target_revision` under the same prefix, and the lane pool its `dispatch_*` collectors. A second
graph-index on one registry writes the collectors the first registered.

## Known limits

- **Overwritten revisions.** `ENTITY_STATES` keeps one revision per key, so a revision overwritten before the watch
  reads it is never delivered. The watermark tolerates this.
- **Replay on every start.** Every start reads every entity in `ENTITY_STATES` again, and queries are refused until it
  has. An entity whose record matches is not written, so this costs a read of the entity and of its record. Resuming
  instead is an open decision.
- **What a restart does not repair.** While an entity's clean record matches its planned rows, a start does not look at
  the rows themselves, so it does not repair: rows edited or deleted by hand; an index bucket emptied in place, or
  restored from a backup with its creation time; rows written by a graph-index that kept no records, when records
  already exist; rows left by two graph-index processes running on one store at once; and writes the NATS host lost to
  a power cut. What SemEngine promises after the NATS host loses power is an open question for the whole store, not
  for graph-index alone.
- **One graph-index per store.** Two graph-index processes writing one store at once can leave a row no record names.
  Stop the old process before starting the new one, as in an update. Nothing refuses a second process; the record's
  conditional writes catch only a second writer between one apply's read and its first write.
- **The reset.** Deleting `GRAPH_INDEX_RECORDS` while graph-index is stopped makes the next start list every entity
  once, repair its rows from `ENTITY_STATES`, and record it again: the same cost as a changed format name. Do this
  after any of the cases above, and after two processes overlapped. Deleting it while graph-index runs fails the
  pending record of every entity that changes after it, so those entities are failed and queries are refused until a
  restart.
- **Too large to record.** An entity whose record would not fit in one message is listed on every apply (see The
  record). A record within the limit that the server still refuses is a failed write: that entity stays failed, and
  queries are refused, until it changes.

## Dependencies

- Upstream: graph-ingest writes `ENTITY_STATES`, which this component follows.
- Reads: `graph/follower` (watch, watermark, readiness), `graph/kvcatalog` (the index buckets),
  `internal/dispatch` (the lane pool).
- Consumers read through `graph.QueryVerbs()` and the `graph.QueryResponse` envelope; none reads the index buckets
  directly.
