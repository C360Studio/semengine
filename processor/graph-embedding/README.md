# graph-embedding

The embedding component of the graph subsystem.

## Overview

`graph-embedding` follows the `ENTITY_STATES` bucket, which graph-ingest writes, and keeps one *embedding record* per
entity that has text in the `EMBEDDING_INDEX` bucket. An embedding is a vector of numbers computed from the entity's
text so that similar texts get similar vectors. graph-embedding has no embedder of its own: the product supplies one
(see "Registering an embedder" below), so the component runs from tier 1 up and a tier 0 composition leaves it out.
`EMBEDDING_DEDUP` holds content-addressed embeddings, keyed by the text and the embedder, so identical text is
embedded once.

A *revision* is the number NATS gives each write to a bucket. The component has two hops, and a revision counts as
applied only after both:

1. **Hop 1** reads the entity and writes a *pending* record with its text, or removes the record when the entity is
   deleted or has no text.
2. **Hop 2**, the Worker (`graph/embedding`), turns each pending record into a *generated* record (the vector) or a
   *failed* one (a bounded reason). Its result is the record's terminal outcome.

The component also answers two query verbs, `graph.embedding.query.search` (rank entities against a text query) and
`graph.embedding.query.similar` (rank entities against one entity's vector). Both read a vector cache kept in memory
from `EMBEDDING_INDEX`; "Query verbs" below says what they accept and return.

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

Which values are text comes from `graph/entitytext`, the rule `graph-lexical` uses too. The two read different views
of it: graph-embedding reads only values whose predicate ends in one of `text_suffixes`, while graph-lexical also
takes every registered label predicate as name text, whatever its suffix, and the two caps differ. Inline text here
is the string values of the entity's non-relationship triples whose predicate ends in one of `text_suffixes`, joined
with spaces and cut to `max_text_len` characters, preferring a word boundary, otherwise at the cap. An entity whose
body is offloaded to an object store (a `StorageRef`) is embedded from its identity text followed by the body, which the
Worker reads through the store registry. If no store is registered for the body's storage instance, only the inline
text is embedded and `content_unresolved_total` counts the exclusion.

## Registering an embedder

A product registers graph-embedding with its embedder, an `embedding.Embedder`. For a provider that serves the
OpenAI-shaped embeddings request over HTTP, such as semembed, build it with `graph/embedding/httpembedder`; any other
`embedding.Embedder` works the same way:

```go
err := graphembedding.RegisterWithEmbedder(registry, "semembed", func() (embedding.Embedder, error) {
    return httpembedder.New(httpembedder.Config{
        BaseURL: "http://127.0.0.1:8081/v1", // must end in /v1 for semembed
        Model:   "Snowflake/snowflake-arctic-embed-s",
    })
})
```

The client tells the provider whether each request holds search queries or documents (`input_type`), and semembed
puts the model's own query or document instruction in front of each text, so a product on semembed needs no prefix of
its own. For a provider that does not add instructions itself, set `QueryPrefix` to the model's query instruction; for
one that refuses the `input_type` field, set `OmitInputType`.

The name (`"semembed"` here) must not be empty. It is the type part of the dedup key, so two embedders never share a
stored vector, and `search` replies it as `embedder_type`. The constructor is called once in each `Start`, never when
the component is created, and `Stop` closes the embedder it returned. If `Start` fails after building the embedder, it
closes the embedder; if that close fails too, `Start` returns both errors and the next `Stop` closes it again.

The factory plain `Register` adds refuses construction, class invalid, naming `RegisterWithEmbedder` and tier 1. A
composition that lists graph-embedding without an embedder therefore fails when the component is created, rather than
running without one.

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
    "workers": 5,
    "coalesce_ms": 0
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the ports above | Inputs only. A `ports` block that is present is used as written; one that is absent takes the defaults |
| `workers` | int | 5 | Concurrent Worker goroutines; negative is refused |
| `max_text_len` | int | 4000 | Characters of text embedded per entity, 0 to 1,000,000. A change takes effect at the restart that applies it: every stored vector is embedded again from the new text (see "Changing the embedder") |
| `text_suffixes` | array | `.title .content .description .summary .text .name .body .abstract .subject` | Predicate suffixes whose string values are embedded |
| `startup_attempts` | int | 30 | Tries to find `ENTITY_STATES` at startup |
| `startup_interval_ms` | int | 500 | Pause between those tries |
| `coalesce_ms` | int | 0 | Window in milliseconds for merging updates to one entity; 0 applies each at once |

The configuration is decoded strictly: a key not listed here is refused at construction, naming it. This includes
`batch_size`, `cache_ttl` and `embedder_type` (the embedder is named when it is registered). The inputs must be a
`kv-watch` of every `ENTITY_STATES` key with no history (at least one is required) and, optionally, `store-read`
inputs; the store registry is used only when a `store-read` input is declared. The component declares no outputs and
refuses a configuration that does. Registering the factory asks boot for no model registry: the product owns its
embedder and whatever the embedder connects to. `MIGRATION.md` lists what a SemStreams configuration changes.

## Ports

| Direction | Name | Kind | Bucket |
| --- | --- | --- | --- |
| input | `entity_watch` | kv-watch | `ENTITY_STATES` |
| input | `content_store` | store-read | `MESSAGES` |

The component writes `EMBEDDING_INDEX` and `EMBEDDING_DEDUP` itself and declares no output port for them.

## Health and readiness

`Health` is `Healthy` once started, and unhealthy while any of these holds: a stored `ENTITY_STATES` value could not
be decoded (a *damaged* entity; `Stuck` names the count, up to 10 IDs with their reasons, and the repair: write a valid
value for the key or delete it), failed entities are unresolved, the `ENTITY_STATES` feed is lost, the Worker's
watch on `EMBEDDING_INDEX` ended while the component ran, or the vector cache's watch on `EMBEDDING_INDEX` ended
(Health reads `vector cache watch lost; restart the process`). The error count (`ErrorCount`) rises by one when an
entity becomes failed (it was not failed before; see "Failed entities and the repair lane" below) and by one for each
stored value refused this way; a repeated failure of an entity already failed, such as a repair pass failing again,
and a failure older than the entity's latest outcome add nothing. graph-index counts the same way. The count never
lowers `Healthy` by itself (ADR-109). A lost Worker watch, a lost cache watch and a lost feed hold until the process
restarts; nothing rebinds them.
`DataFlow` counts one input per delivered `ENTITY_STATES` entry, update or delete, before merging.

The follower publishes the readiness record to `GRAPH_STATUS` at once and then every heartbeat, with
`indexed_revision`, `target_revision`, `failed_count` and `failed_reasons` (the count of failed entities per bounded
reason). It reads `degraded` when entities are failed, the feed is lost, or the component has been behind for 30
seconds with no revision completed. Any completion counts, so one entity held while others finish reads not ready,
not degraded. A graph with no entities is caught up once the
snapshot is delivered.

### Failed entities and the repair lane

The component keeps, per entity, the source revision and result of its latest terminal outcome; a failure for an
older revision is not counted, and a success at a revision clears a failure at or below it. The map is filled from
the failed records `EMBEDDING_INDEX` holds at start. Three failures are hop 1's own: a failed record delete, a failed
pending write and a failed entity read. Every 30 seconds the repair lane retries them against the entity's current
value; any later convergence of the entity in hop 1 (hop 1 writes its pending record, removes its record, or finds a
generated record already standing) clears the failure. The Worker's failures are not retried: they wait for a new
revision of the entity or a restart.

## Query verbs

Both verbs answer a `graph.QueryResponse` with producer `graph-embedding`, from `Start` until `Stop` returns. Each
reply's `data` holds `results` (a JSON array, `[]` when empty; each hit is an `entity_id` and a `similarity`),
`unreadable_records` (records in `EMBEDDING_INDEX` that could not be decoded; on a `search` with a `scope`, only those
whose keys the scope accepts) and `failed_records` (entities whose
latest embedding failed, so they cannot be hits). `search` also sends `embedder_type`, the name the embedder was
registered under, and embeds its query with the embedder's `GenerateQuery`. A 0 in both
counts does not prove every entity is searchable: an entity whose embedding is still pending is left out and counted
in neither.

| Subject | Request keys | Ranks |
| --- | --- | --- |
| `graph.embedding.query.search` | `query` (required, not blank), `limit`, `scope`, `min_revision` | entities against the query text |
| `graph.embedding.query.similar` | `entity_id` (required), `limit`, `min_revision` | entities against that entity's vector; the entity itself is never a hit |

- **Limit.** 0 means 10, 1 to 100 is used as sent, and anything else is refused `invalid_request`, not cut down.
- **Scope.** `scope` is a list of dot-delimited entity ID prefixes; an entity is a candidate if it matches any of them,
  and an empty prefix matches every entity. The filter runs before ranking, so a limit of 1 gives the best entity
  inside the scope. A prefix that fails `types.ValidateEntityIDPrefix` is refused `invalid_request`.
- **Hits.** Ranked by similarity, ties by entity ID. A hit scoring 0 is dropped.
- **Strict requests.** A key the request does not define, or data after the JSON object, is refused
  `invalid_request`, naming what was refused.
- **No revision claim.** The verbs do not say how current the cache is. `indexed_revision` is always 0, and a
  `min_revision` above 0 is refused `invalid_request` rather than answered.
- **`similar` needs a vector.** An entity with no record, a record that is not generated, a generated record with no
  vector, or one made under another embedder identity is refused class invalid, code `embedding_unavailable`; a record that
  cannot be decoded is refused class invalid with no code.
- **Changing the embedder.** Each generated record stores the identity it was made under: the registered type, the
  embedder's model and width, and `max_text_len`. A restart against the same NATS state with any of those changed
  regenerates every vector the previous embedder left: hop 1 replaces such a record with a pending one, and the Worker
  embeds it again. Until it has, the verbs score only vectors made under the running identity, so an entity whose
  vector is not regenerated yet is not a hit and is refused as a `similar` source, rather than compared across two
  vector spaces. A restart with the same embedder keeps every stored vector, provided the embedder reports its width
  from the start: one that reports 0 until its first response cannot vouch for a stored vector, so the entities hop 1
  replays before that response are embedded again.

### When a call is withheld

Before answering, each verb asks the follower whether the graph state is ready. It is refused transient
`index_not_ready` before the first build completes, while the `ENTITY_STATES` feed is lost, while a hop-1 failure
(`delete_failed`, `pending_write_failed`, `entity_read_failed`) stands, and while the cache has not applied its first
snapshot or has lost its watch. A damaged entity does not withhold either verb: the component removes the entity's
vector, `search` and `similar` answer from the other vectors, and each reply counts the damaged entities it may have
left out in `damaged_entities` (omitted at 0; `search` counts only those inside its scope). `similar` from a damaged
entity is refused fatal `graph_state_reset_required`. A valid write or a delete of the key clears the damage with no
restart. A failure in the Worker
(hop 2, such as a `content_error` for a document whose text cannot be extracted) does not withhold anything: the
verbs answer from the other vectors and `failed_records` counts it. The status the component publishes still reads
degraded while such a failure stands. A malformed request is refused `invalid_request` whatever the readiness.

## Metrics

Collectors register on the injected `metric.MetricsRegistry`, never on Prometheus' global registry. This component
registers two counters:

- `semengine_graph_embedding_content_unresolved_total`: entities whose offloaded body was left out because no store
  is registered for its storage instance.
- `semengine_graph_embedding_truncated_total{by}`: entities embedded from only the opening of their text. `by` is
  `text_cap` when the text was cut at `max_text_len`, and `provider` when the embedder's provider cut it to fit the
  model's input window. Only an embedder that reports such a cut is counted under `provider`: `httpembedder` reads
  semembed's `"truncated": true` reply mark; a product's own embedder reports it by implementing
  `embedding.TruncationMarker`. A vector reused from the dedup bucket is not counted again.

The follower adds `readiness`, `lag`,
`bootstrap_complete`, `readiness_state`, `status_publish_failures_total`, `indexed_revision` and `target_revision`
under the same prefix.

## Known limits

- **A hung body store holds a lane for one deadline.** An offloaded body read ends 10 s after it begins; a store
  that hangs holds one Worker lane that long, then the record is failed `content_error` and the lane goes on. A store
  that ignores its context is not bounded by the deadline.
- **A hung embedder holds a lane for one deadline.** Each embedder call ends 30 s after it begins; an embedder that
  hangs holds one Worker lane that long, then the record is failed `timeout`, whatever error the embedder returned,
  and the lane goes on. An embedder that ignores its context is not bounded by the deadline.
- **Double count after a restart without the store.** If the process restarts with no store registered for an
  offloaded body, the Worker's replay of the old pending record and hop 1's re-read of the entity are two
  exclusions, and `content_unresolved_total` counts both.
- **No retry for the Worker's failures.** See the repair lane above.
- **Hop 1 is serial.** A slow pending write delays delivery of every entry behind it; the heartbeat keeps
  publishing, and `staleness_ms` rises.

## Dependencies

- Upstream: graph-ingest writes `ENTITY_STATES`, which this component follows. The product supplies the embedder.
- Reads: `graph/follower` (watch, watermark, readiness), `graph/embedding` (the `Embedder` interface, Worker,
  storage), `graph/entitytext` (which values are text, the offloaded-body read), `graph/kvcatalog`, `storage/storeregistry`
  (offloaded bodies).
- Writes: `EMBEDDING_INDEX` and `EMBEDDING_DEDUP`, which this component owns.
