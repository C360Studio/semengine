# graph-embedding

The embedding component of the graph subsystem.

## Overview

`graph-embedding` follows the `ENTITY_STATES` bucket, which graph-ingest writes, and keeps one *embedding record* per
entity that has text in the `EMBEDDING_INDEX` bucket. An embedding is a vector of numbers computed from the entity's
text so that similar texts get similar vectors. At tier 0 (no external provider) the only embedder is BM25, which runs
in this process; nothing is sent to a model service. `EMBEDDING_DEDUP` holds content-addressed
embeddings, keyed by the text and the embedder, so identical text is embedded once.

A *revision* is the number NATS gives each write to a bucket. The component has two hops, and a revision counts as
applied only after both:

1. **Hop 1** reads the entity and writes a *pending* record with its text, or removes the record when the entity is
   deleted or has no text.
2. **Hop 2**, the Worker (`graph/embedding`), turns each pending record into a *generated* record (the vector) or a
   *failed* one (a bounded reason). Its result is the record's terminal outcome.

This version carries the write path only. The query verbs (`graph.embedding.query.search` and `.similar`) and the
vector cache they read are not present yet; nothing answers on those subjects.

## Architecture

```text
ENTITY_STATES ──► graph/follower ──► hop 1 ──► EMBEDDING_INDEX (pending) ──► Worker ──► EMBEDDING_INDEX (generated
  (KV watch)       (the one watch,    (this      ▲                          (hop 2)      or failed), EMBEDDING_DEDUP
                    the watermark,    package)   │
                    GRAPH_STATUS)                └── repair lane retries hop 1's failures every 30 s
```

`graph/follower` owns the watch, the replay, the watermark and the readiness record it writes to `GRAPH_STATUS` under
the key `graph-embedding`. Hop 1 runs one entry at a time on the follower's goroutine; with `coalesce_ms` above 0 it
instead merges updates to one entity and reads the entity's current value when the window ends. The component reports
a revision complete to the follower when the Worker reports its terminal outcome, or at once when hop 1 ends it (no
text, deleted, or a generated record already standing). So readiness waits for hop 2.

## Text

Inline text is the string values of the entity's non-relationship triples whose predicate ends in one of
`text_suffixes`, joined with spaces and cut to `max_text_len` characters at a word boundary. An entity whose body is
offloaded to an object store (a `StorageRef`) is embedded from its identity text followed by the body, which the
Worker reads through the store registry. If no store is registered for the body's storage instance, only the inline
text is embedded and `content_unresolved_total` counts the exclusion.

## Configuration

```json
{
  "type": "processor",
  "name": "graph-embedding",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "config": {"kind": "kv-watch", "bucket": "ENTITY_STATES"}},
        {"name": "content_store", "config": {"kind": "store-read", "bucket": "MESSAGES"}}
      ]
    },
    "embedder_type": "bm25",
    "workers": 5,
    "coalesce_ms": 0
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the ports above | Inputs only. A `ports` block that is present is used as written; one that is absent takes the defaults |
| `embedder_type` | string | `bm25` | The only value accepted at tier 0; any other, `http` included, is refused |
| `workers` | int | 5 | Concurrent Worker goroutines; negative is refused |
| `max_text_len` | int | 4000 | Characters of text embedded per entity, 0 to 1,000,000. It is part of the dedup key, so changing it re-embeds the entities it affects |
| `text_suffixes` | array | `.title .content .description .summary .text .name .body .abstract .subject` | Predicate suffixes whose string values are embedded |
| `startup_attempts` | int | 30 | Tries to find `ENTITY_STATES` at startup |
| `startup_interval_ms` | int | 500 | Pause between those tries |
| `coalesce_ms` | int | 0 | Window in milliseconds for merging updates to one entity; 0 applies each at once |

The configuration is decoded strictly: a key not listed here is refused at construction, naming it. This includes
`batch_size` and `cache_ttl`. The inputs must be a `kv-watch` of every `ENTITY_STATES` key with no history (exactly
one is required) and, optionally, `store-read` inputs; the store registry is used only when a `store-read` input is
declared. The component declares no outputs and refuses a configuration that does. Registering the factory asks boot
for no model registry. `MIGRATION.md` lists what a SemStreams configuration changes.

## Ports

| Direction | Name | Kind | Bucket |
| --- | --- | --- | --- |
| input | `entity_watch` | kv-watch | `ENTITY_STATES` |
| input | `content_store` | store-read | `MESSAGES` |

The component writes `EMBEDDING_INDEX` and `EMBEDDING_DEDUP` itself and declares no output port for them.

## Health and readiness

`Health` is `Healthy` once started, and unhealthy while any of these holds: a stored `ENTITY_STATES` value could not
be decoded (Health reads `graph state reset required: <reason>`, and queries are refused with the error code
`graph_state_reset_required`), failed entities are unresolved, the `ENTITY_STATES` feed is lost, or the Worker's watch
on `EMBEDDING_INDEX` ended while the component ran. The error count (`ErrorCount`) rises by one when an entity becomes
failed (it was not failed before; see "Failed entities and the repair lane" below) and by one for each stored value
refused this way; a repeated failure of an entity already failed, such as a repair pass failing again, and a failure
older than the entity's latest outcome add nothing. graph-index counts the same way. The count never lowers `Healthy`
by itself (ADR-109). A lost Worker watch and a lost feed hold until the process restarts; nothing rebinds them.
`DataFlow` counts one input per delivered `ENTITY_STATES` entry, update or delete, before merging.

The follower publishes the readiness record to `GRAPH_STATUS` at once and then every heartbeat, with
`indexed_revision`, `target_revision`, `failed_count` and `failed_reasons` (the count of failed entities per bounded
reason). It reads `degraded` when entities are failed, the feed is lost, or the component has been behind for 30
seconds with no revision completed. Any completion counts, so one entity held while others finish reads not ready,
not degraded (owner ruling (a1), PR #550 comment 6721867279). A graph with no entities is caught up once the
snapshot is delivered.

### Failed entities and the repair lane

The component keeps, per entity, the source revision and result of its latest terminal outcome; a failure for an
older revision is not counted, and a success at a revision clears a failure at or below it. The map is filled from
the failed records `EMBEDDING_INDEX` holds at start. Three failures are hop 1's own: a failed record delete, a failed
pending write and a failed entity read. Every 30 seconds the repair lane retries them against the entity's current
value; any later convergence of the entity in hop 1 (hop 1 writes its pending record, removes its record, or finds a
generated record already standing) clears the failure. The Worker's failures are not retried: they wait for a new
revision of the entity or a restart.

## Metrics

Collectors register on the injected `metric.MetricsRegistry`, never on Prometheus' global registry. This component
registers one, `semengine_graph_embedding_content_unresolved_total` (counter): entities whose offloaded body was
left out because no store is registered for its storage instance. The follower adds `readiness`, `lag`,
`bootstrap_complete`, `readiness_state`, `status_publish_failures_total`, `indexed_revision` and `target_revision`
under the same prefix.

## Known limits

- **Offloaded-body fetch has no deadline.** A store that hangs holds one Worker lane until the process stops
  (#551).
- **Coalesced lane drain.** With `coalesce_ms` above 0, a key that is absent or unreadable when the window ends is
  completed at the highest revision, which also drains a re-create the follower delivered during that read. The
  re-create's pending record is still written by a later batch, but readiness can read caught up while it waits
  (#555; the same limit as the pin).
- **Double count after a restart without the store.** If the process restarts with no store registered for an
  offloaded body, the Worker's replay of the old pending record and hop 1's re-read of the entity are two
  exclusions, and `content_unresolved_total` counts both.
- **No retry for the Worker's failures.** See the repair lane above.
- **Hop 1 is serial.** A slow pending write delays delivery of every entry behind it; the heartbeat keeps
  publishing, and `staleness_ms` rises.

## Dependencies

- Upstream: graph-ingest writes `ENTITY_STATES`, which this component follows.
- Reads: `graph/follower` (watch, watermark, readiness), `graph/embedding` (BM25 embedder, Worker, storage),
  `graph/kvcatalog`, `storage/storeregistry` (offloaded bodies).
- Writes: `EMBEDDING_INDEX` and `EMBEDDING_DEDUP`, which this component owns.
