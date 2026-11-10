# graph-clustering

The community component of the graph subsystem.

## Overview

`graph-clustering` follows the `ENTITY_STATES` bucket, which graph-ingest writes, and keeps the `COMMUNITY_INDEX`
bucket current. A *community* here is a *group*: every entity whose ID shares the same first 4 parts (level 0), first
3 parts (level 1) or first 2 parts (level 2). Groups are derived from IDs, not detected from relationships (ADR-099),
so no external provider and no other index is involved. For each group the component keeps one bounded record
(counts, keywords, a few sample IDs, never a member list); `graph/clustering` defines the record, and its README
lists the fields.

A *revision* is the number NATS gives each write to a bucket. The component has two parts:

1. **The feed.** `graph/follower` watches `ENTITY_STATES`, replays what is stored, then delivers each change with the
   value it carries. The component hands each value and its revision to the `graph/clustering` Maintainer, in
   delivery order, and reports the revision complete at once. A value that does not decode is handed over as a
   refused entity, which contributes nothing to its groups.
2. **The ticker.** Every `detection_interval` the component asks the Maintainer to write the groups that changed since
   the last tick. The Maintainer removes the record of a group whose last member was deleted, but only after every
   write of the tick succeeded.

graph-query's `localSearch` and the text tier of `globalSearch` and `searchGraph` read these records. Until the first
tick has published, they refuse transient `index_not_ready`.

## Architecture

```text
ENTITY_STATES ──► graph/follower ──► deliver ──► Maintainer ──► ticker ──► COMMUNITY_INDEX
  (KV watch)       (watch, replay,    (this       (graph/         (every      (changed groups
                    watermark,         package)    clustering)     interval)   only)
                    GRAPH_STATUS)
```

`graph/follower` owns the watch, the replay, the watermark and the readiness record it writes to `GRAPH_STATUS` under
the key `graph-clustering`.

## What the readiness key means

The key's `indexed_revision` counts revisions the Maintainer holds, not revisions published. It reaches a write's
revision as soon as the Maintainer has applied it, before the next tick. To know whether a group's record includes a
write, read the record's `entity_revision`, which is exact per group. "Health and readiness" below lists
when the key reads `degraded`.

## Configuration

```json
{
  "type": "processor",
  "name": "graph-clustering",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "config": {"kind": "kv-watch", "bucket": "ENTITY_STATES"}}
      ]
    },
    "detection_interval": "30s"
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the input above | Inputs only. A `ports` block that is present is used as written; one that is absent takes the default |
| `detection_interval` | string | `30s` | Time between publications, a Go duration such as `30s` or `5m`. A value that does not parse or is not above zero is refused |
| `startup_attempts` | int | 30 | Tries to find `ENTITY_STATES` at startup; negative is refused |
| `startup_interval_ms` | int | 500 | Pause between those tries; negative is refused |

The configuration is decoded strictly: a key not listed here is refused at construction, naming it. These keys are
refused with a line saying what replaces them: `batch_size`, `enable_llm`, `enhancement_workers`,
`min_community_size`, `max_iterations`, `entity_id_edges`, `semantic_edges`, `allow_ungated_reads`,
`enable_anomaly_detection` and `anomaly_config`. The input must be exactly one `kv-watch` of every `ENTITY_STATES` key
with no history. The component declares no outputs and refuses a configuration that does. Registering the factory
asks boot for no model registry. `MIGRATION.md` lists what a SemStreams configuration changes.

## Ports

| Direction | Name | Kind | Bucket |
| --- | --- | --- | --- |
| input | `entity_watch` | kv-watch | `ENTITY_STATES` |

The component writes `COMMUNITY_INDEX` itself and declares no output port for it.

## When records are published

The ticker starts a tick only while the follower says the graph state is ready. So nothing is written before the
replay of `ENTITY_STATES` has been applied, and once the follower reports a lost feed (a watch closed, a bucket
recreated, a revision out of order) or a latched reset (a stored value that cannot be decoded during the replay), no
later tick starts. A tick already running when the loss is detected finishes, so a record published just before the
loss was noticed may already be out of date. The first tick runs one `detection_interval` after `Start`. A tick that
takes longer than the interval delays the next; ticks never overlap.

## Health and readiness

`Health` is `Healthy` once started, and unhealthy while any of these holds:

- a stored `ENTITY_STATES` value could not be decoded during the replay (`graph state reset required: <reason>`, until
  the process restarts);
- the `ENTITY_STATES` feed is lost (until the process restarts);
- the last tick left entities out of the records because the Maintainer refused them (reason `refused_entity`, one
  count per entity); the other groups are still published, and the state clears on a tick that refuses none;
- the last tick's publication failed (reason `publication_failed`, count 1); the changed groups stay pending and the
  next tick retries them. The state clears on a tick that succeeds.

The readiness record on `GRAPH_STATUS` carries `failed_count` and `failed_reasons` (the two tick reasons above), and
reads `degraded` while either is non-zero or the feed is lost. A failed write returns no refusal count, so while
publication fails the last known `refused_entity` count is kept until a tick reports one again or succeeds.
Nothing withholds ticks: the component reports 0 to the follower's withhold.
`DataFlow` counts one input per delivered `ENTITY_STATES` entry, update or delete.

## Metrics

Collectors register on the injected `metric.MetricsRegistry`, never on Prometheus' global registry. This component
registers one, `semengine_graph_clustering_publication_failures_total` (counter): ticks whose publication to
`COMMUNITY_INDEX` failed. The follower adds its readiness gauges and counters under the same prefix.

## Known limits

- **No LLM summaries.** A record's summary is a one-line template over its counts. Language-model summaries are tier 2
  (#485) and not present.
- **Replay-time reset is permanent.** A stored `ENTITY_STATES` value that cannot be decoded during the replay stops
  publication until the process restarts.
- **Records update on successful ticks.** `detection_interval` is how often a tick starts, not a bound on how stale a
  record can be: a slow or failing publication, or a lost feed, delays updates further. A group's record is as current
  as the last successful tick; its `entity_revision` says which write it includes.

## Dependencies

- Upstream: graph-ingest writes `ENTITY_STATES`, which this component follows.
- Reads: `graph/follower` (watch, watermark, readiness), `graph/clustering` (Maintainer, storage, record type),
  `graph/kvcatalog`.
- Writes: `COMMUNITY_INDEX`, which this component owns, and its readiness record on `GRAPH_STATUS` through the
  follower.
