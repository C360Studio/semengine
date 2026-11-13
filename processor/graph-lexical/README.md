# graph-lexical

The word-search component of the graph subsystem.

## Overview

`graph-lexical` follows the `ENTITY_STATES` bucket, which graph-ingest writes, and keeps an in-memory word index
(`graph/lexical`) of each entity's text. A *word index* maps each word to the entities that contain it, so a query
such as `pump -telemetry` finds entities by the words in their name and body and ranks them with BM25, a standard
scoring formula that rewards a word that is frequent in an entity and rare across all entities. Nothing is sent to a
model or other service, so it runs at tier 0, and the index is never written to disk: it is rebuilt from
`ENTITY_STATES` on every start. Which of an entity's values count as its text is decided by `graph/entitytext`, the
same rule graph-embedding uses.

Each process holds its own index. A *revision* is the number NATS gives each write to a bucket; a revision counts as
applied only once its text is in the index, and the component reports that to `graph/follower`, which publishes the
readiness record to `GRAPH_STATUS` under the key `graph-lexical`.

## Who word search helps

Only entities that have text: documents, notes, descriptions, names. An entity whose facts are numbers, such as
sensor or drone telemetry, has no words to match and gains nothing from word search. Such entities are reached
through the structure of the graph instead (by ID prefix, predicate, relationship, name and alias, or group); the
`graph/lexical` README lists the read requests. Two limits apply today: the graph has no numeric range filter (the
`predicate` request matches a stored value only as an exact string), and no component serves a time-range request,
so "temperature above 80" or "readings from the last hour" cannot be asked of the graph yet.

## Query verb

The component answers `graph.lexical.query.search`. The request is JSON:

```json
{"query": "pump -telemetry", "limit": 10, "scope": ["acme.ops.docs"], "min_revision": 0}
```

| Field | Meaning |
| --- | --- |
| `query` | Plain words, quoted phrases and `-` exclusions; the rules are in the `graph/lexical` README |
| `limit` | 0 means 10; 1 to 100 is used as given; anything else is refused |
| `scope` | Entity ID prefixes; an entity must start with one of them. Empty or `""` means every entity. The scope applies before the limit |
| `min_revision` | Answer only once the index holds at least this revision; otherwise a transient `revision_not_reached` error |

An unknown request key is refused, naming it. The reply is a `graph.QueryResponse` whose data holds:

- `results`: the best matches, each an `entity_id` and a `score`, ordered by score and then entity ID. A score has no
  fixed scale and is comparable only within one reply.
- `matched`: how many entities matched within the scope, before the limit. `matched` above the number of results
  means the limit cut the answer.
- `bodies_not_indexed`: how many entities in the whole index, not only the hits, have an offloaded body the index
  does not hold (next section). Such an entity is found only by its inline text.

While the first build is unfinished, and while the `ENTITY_STATES` feed is lost, the verb refuses with the
transient code `index_not_ready`. After that, no body state withholds it.

## Entity text and offloaded bodies

An entity's *name* is its text values whose predicate ends in `.name` or `.title`, plus the values of the
vocabulary's label predicates; its *body* is its other text values, then the offloaded body. Text values are the string
values of predicates ending in one of `text_suffixes`. Words in the name count twice when ranking.

An entity's body can be *offloaded*: stored in an object store, with the entity holding only a reference to it. Such
an entity is indexed by its inline text at once and by its body once a single goroutine has read the body from the
store. Until then, a body not in the index is in one of four states, counted on the gauge
`semengine_graph_lexical_bodies_not_indexed{state}`:

| State | Meaning | What happens next |
| --- | --- | --- |
| `queued` | Not read yet | It is read in turn |
| `unread` | A read failed, or the store was skipped after another read on it failed | Retried every 30 seconds. It is the only failure: the readiness record reads degraded with the reason `body_unread`, and `Health` names it |
| `missing` | The store has no such object | Not asked for again until the entity changes |
| `no_store` | No store is registered for the body's storage instance | Looked up again every 30 seconds |

A read that fails for any reason other than a missing object skips that store until the next 30-second pass, so a
store that does not answer costs one 10-second read deadline per pass, not one per body. The first missing body and
the first unregistered store since start are logged once each.

## The text cap

`max_text_len` is the most characters of an entity's body that are indexed. Text past it is cut at a word boundary,
and the name is never cut. The default is 16,000 characters, long enough that a code body is almost never cut;
graph-embedding's default is 4,000, so the two can index different amounts of the same entity. The gauge
`semengine_graph_lexical_bodies_cut` counts the entities whose body is cut, and the first cut since start is logged
once, naming the entity and the cap. Words past the cap cannot be found. A larger value reads more of each offloaded
body and holds more words; the accepted range is 0 to 1,000,000.

File paths are not indexed unless a product adds the suffix of its path predicate to `text_suffixes`.

## Configuration

```json
{
  "type": "processor",
  "name": "graph-lexical",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {"name": "entity_watch", "config": {"kind": "kv-watch", "bucket": "ENTITY_STATES"}}
      ]
    },
    "max_text_len": 16000
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the input above | Exactly one input, a `kv-watch` of every `ENTITY_STATES` key with no history; no outputs. A `ports` block that is present is used as written |
| `text_suffixes` | array | `.title .content .description .summary .text .name .body .abstract .subject` | Predicate suffixes whose string values are indexed as text |
| `max_text_len` | int | 16000 | Characters of body indexed per entity, 0 to 1,000,000; 0 means the default |
| `startup_attempts` | int | 30 | Tries to find `ENTITY_STATES` at startup |
| `startup_interval_ms` | int | 500 | Pause between those tries |

The configuration is decoded strictly: a key not listed here is refused at construction, naming it. Offloaded bodies
are read from the stores in the composition's store registry, which the component looks up at each read; no store
port is configured.

A product adds the component to its composition by calling `graphlexical.Register` on its component registry and
listing `graph-lexical` among its components.

## Memory and restart cost

The index is in memory and is rebuilt on every start, so a restart costs a rebuild and the index holds whatever fits
in the process.

- **Memory**, measured on one host: 50,000 entities with about 1.2 KB of text each took 110 MB of heap (about 2.2 KB
  per entity); 32,000 entities with about 640 bytes of text each took 44 MB (about 1.4 KB per entity). One million
  entities is a projection, not a measurement: about 1.4 GB at the smaller text size and about 2.2 GB at the larger.
  Real text has more distinct words than the repeated test text, so treat these as floors.
- **Search time** grows with the number of entities that match. A query on a common word against a very large index
  holds the index's read lock while it scores, and writes wait; this was estimated at 200 to 400 ms for a million
  entities, not measured here.
- **First build.** Every offloaded body is read on every start. For 32,000 entities with offloaded bodies of about
  600 characters, the first search was answered after about 16 seconds on one Apple M3 Pro machine against a local
  NATS server in Docker, about 0.4 to 0.5 ms per body; 100,000 took 50 to 54 seconds. On another host or against a
  remote store the time is not measured. Until the build is applied, search is refused with `index_not_ready`.

## Metrics

All on the registry the composition injects.

| Gauge | Meaning |
| --- | --- |
| `semengine_graph_lexical_index_entities` | Entities in the index |
| `semengine_graph_lexical_index_words` | Words in the index, every occurrence in every entity counted |
| `semengine_graph_lexical_bodies_not_indexed{state}` | Offloaded bodies not in the index, by the four states above |
| `semengine_graph_lexical_bodies_cut` | Entities whose body is cut at `max_text_len` |
