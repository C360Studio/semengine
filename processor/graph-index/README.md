# graph-index

The relationship index component of the graph subsystem.

## Overview

`graph-index` follows the `ENTITY_STATES` bucket, which graph-ingest writes, and keeps five index buckets in step with
it so that a caller can ask "what does this entity point to", "what points at it", "which entities carry this
predicate" or "which entity has this name" without reading every entity. It answers those questions as request/reply
verbs on `graph.index.query.*`.

A *revision* is the number NATS gives each write to a bucket. A *watermark* is the highest revision graph-index has
applied with nothing below it still waiting. The index is rebuilt from `ENTITY_STATES` on every start, so after a
start it is not ready until it has caught up (see Query refusals).

## Architecture

```text
                                       ┌─► OUTGOING_INDEX  (KV)
ENTITY_STATES ──► graph/follower ──►   │─► INCOMING_INDEX  (KV)
   (KV watch)      (the one watch,     │─► ALIAS_INDEX     (KV)
                    the watermark,     │─► PREDICATE_INDEX (KV)
                    GRAPH_STATUS)      └─► NAME_INDEX      (KV)
                                              ▲
graph.index.query.* (request/reply) ──────────┘ read by the eight verbs
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
| `ALIAS_INDEX` | the alias | the canonical entity ID | which entity an alias names |
| `PREDICATE_INDEX` | `predicate.entity` (a 3-part predicate, a 6-part entity ID) | none | which entities carry a predicate |
| `NAME_INDEX` | `hash(name).entity.hex(predicate)` | the name as written and its label priority | which entities carry a name |

`NAME_INDEX` is graph-index's own and is not a configurable output; the other four are the configured outputs. A
membership key encodes who owns it, so an update replaces exactly the rows its entity owns: the layouts are
ADR-077 and ADR-078. An alias that is not a valid NATS KV key (for example `two words`) is left out of
`ALIAS_INDEX`, logged, and counted in `aliases_skipped_total`; the entity's other rows are still written.

## Query verbs

Every verb is declared in `graph.QueryVerbs()` (`graph/query_verbs.go`) with its subject; spell no subject as a
literal. All eight are on `graph.index.query.` followed by the name.

| Verb | Request | Reply `data` |
| --- | --- | --- |
| `outgoing` | `{"entity_id": ...}` | `graph.OutgoingRelationshipsData` |
| `incoming` | `{"entity_id": ...}` | `graph.IncomingRelationshipsData`, sorted by source then predicate |
| `alias` | `{"alias": ...}` | `graph.AliasData` |
| `predicate` | `{"predicate": ..., "value": ..., "limit": ...}` (`value` and `limit` optional) | `graph.PredicateData` |
| `predicateList` | `{"namespace": ...}` (optional) | `graph.PredicateListData` |
| `predicateStats` | `{"predicate": ..., "sample_limit": ...}` | `graph.PredicateStatsData` |
| `predicateCompound` | `{"predicates": [...], "operator": "AND" or "OR", "limit": ...}` | `graph.CompoundPredicateData` |
| `byName` | `{"name": ..., "limit": ...}` | `graph.NameData` |

A successful reply is a `graph.QueryResponse`:

```json
{"data": {}, "indexed_revision": 1234, "producer": "graph-index", "timestamp": "2026-10-27T12:00:00Z"}
```

`indexed_revision` is the watermark read before the handler's first bucket read, so every `ENTITY_STATES` write at or
below it is reflected in `data`. A caller holding the `kv_revision` of its own write compares it with this number to
know whether the answer includes that write. An `alias` reply carries the same number but promises nothing about
which aliases resolve (see Known limits). A refusal is a classified error and carries no envelope.

A malformed request is refused as `invalid_request` before any readiness check, so a bad request fails the same way
during a rebuild.

### Query refusals

Every one of the eight verbs refuses:

- with the transient code `index_not_ready` during the startup rebuild, until the watermark first reaches the
  `ENTITY_STATES` revision that was newest when the rebuild's snapshot ended. Once reached, the index stays ready; a
  later lag does not refuse queries;
- with the transient code `index_not_ready` for as long as the follower has lost its feed (below), and while index
  writes that failed are unresolved;
- with the fatal code `graph_state_reset_required` after a stored `ENTITY_STATES` value could not be decoded (ADR-074).
  The state holds until the process restarts; the stored state has to be wiped and rebuilt first.

A caller treats `index_not_ready` as retry later and `graph_state_reset_required` as do not retry.

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

`Health` is `Healthy` once started, and unhealthy while any of these holds: a stored value could not be decoded
(`graph_state_reset_required`), index writes are unresolved, or the `ENTITY_STATES` feed is lost. The error count is
reported and never lowers `Healthy` by itself (ADR-109). `DataFlow` counts one input per delivered `ENTITY_STATES`
entry, update or delete, before merging.

The follower publishes the readiness record to `GRAPH_STATUS` at once and then every heartbeat, with `indexed_revision`,
`target_revision` and `failed_count`. The state reads `degraded` when index writes are unresolved, the feed is lost, or
the watermark has not moved for 30 seconds while behind.

### Lost feed

The follower declares the feed lost when the watch cannot start, when it closes while the component is running, or when
`ENTITY_STATES`' backing stream is a different stream from the one it bound (the bucket was deleted and created again).
Nothing rebinds the watch: the state holds until the process restarts. How long a watch stalls after a broker restart
(about 20 s in one probe of nats.go v1.54.0, PR #414 comment 6425769680) and when a deleted bucket closes it (about 18 s,
comment 6426443564) are that library's timings and are not promised here. A bucket deleted and created again within one
stall of the watch is caught at the next heartbeat; a stream restored with the same creation time is not.

## Metrics

Collectors register on the injected `metric.MetricsRegistry`, never on Prometheus' global registry. Names are
`semengine_graph_index_<name>`:

| Name | Type | Meaning |
| --- | --- | --- |
| `events_processed_total` | counter | `ENTITY_STATES` entries processed |
| `updates_total` | counter vec | index updates by `index_type` |
| `kv_operations_total` | counter vec | KV operations by `operation` and `kv_bucket` |
| `watch_events_total` | counter vec | watch events by `event_type` |
| `write_failures_total` | counter | entities whose required index writes failed after retry |
| `reindex_events_total` | counter vec | re-indexes by `result` (`changed` or `unchanged`) |
| `reconcile_operations_total` | counter vec | owner reconciliation operations by `index_type`, `operation`, `result` |
| `aliases_skipped_total` | counter | aliases left out of `ALIAS_INDEX` |
| `query_rows_skipped_total` | counter vec | stored index rows a query left out, by `index_type` |

The follower adds `readiness`, `lag`, `bootstrap_complete`, `readiness_state`, `status_publish_failures_total`,
`indexed_revision` and `target_revision` under the same prefix, and the lane pool its `dispatch_*` collectors. A second
graph-index on one registry writes the collectors the first registered.

## Known limits

- **Aliases.** The alias read does not honour replacement: an alias that was dropped or changed, or whose entity was
  deleted, can still resolve, and an alias added and dropped before graph-index read the key is never seen. Issue
  [#415](https://github.com/C360Studio/semengine/issues/415) tracks the fix and its key layout.
- **Overwritten revisions.** `ENTITY_STATES` keeps one revision per key, so a revision overwritten before the watch
  reads it is never delivered. The watermark tolerates this.
- **Rebuild on every start.** The index is rebuilt from `ENTITY_STATES` at every start, and queries are refused for the
  length of the rebuild. Issue #108 holds the decision on resuming instead.
- **Memory.** One string per entity is kept to count `reindex_events_total`.

## Dependencies

- Upstream: graph-ingest writes `ENTITY_STATES`, which this component follows.
- Reads: `graph/follower` (watch, watermark, readiness), `graph/kvcatalog` (the index buckets),
  `internal/dispatch` (the lane pool).
- Consumers read through `graph.QueryVerbs()` and the `graph.QueryResponse` envelope; none reads the index buckets
  directly.
