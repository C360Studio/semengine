# graph-query

The component that answers the graph's read requests on `graph.query.*`.

## Overview

`graph-query` sits in front of graph-ingest, which holds entities, and graph-index, which holds the relationship and
name indexes. It writes nothing. Its one piece of state is a read-only view of the `COMMUNITY_INDEX` bucket, which
holds a record per group of entities and which `localSearch` reads (see below).

`graph.QueryVerbs()` (`graph/query_verbs.go`) decides what it serves: at start it subscribes a handler to each verb
declared for `graph-query`, and fails to start if a declared verb has no handler. Any other `graph.query.*` subject is
left alone, so a consumer can serve its own verb there.

graph-query is ported in steps; steps 1 to 4 are in the tree, and the table below is what they serve. Step 5 adds the
text search (`searchGraph`).
`MIGRATION.md` lists what a consumer changes at its cutover and grows with each step.

## Verbs

| Verb on `graph.query.` | Answers from | Reply |
| --- | --- | --- |
| `entity`, `batch`, `prefix` | `graph.ingest.query.` same name | bare, as graph-ingest sends it |
| `byName` | `graph.index.query.byName` | enveloped, as graph-index sends it |
| `entityByAlias` | `graph.index.query.alias`, then `graph.ingest.query.entity` | enveloped by `graph-query`, `data` a `graph.ExactEntity` |
| `relationships` | `graph.index.query.outgoing` or `incoming` | enveloped by `graph-query`, `data` a list of `graph.RelationshipEdge` |
| `pathSearch` | `graph.ingest.query.entity` for the start, then `graph.index.query.outgoing` and `incoming` | enveloped by `graph-query`, `data` a `graph.PathSearchData` |
| `hierarchyStats` | `graph.ingest.query.prefix`, one page | bare, a `graph.HierarchyStatsData` |
| `summary` | `graph.index.query.predicateList`, then `graph.ingest.query.prefix`, one page | enveloped by `graph-query`, `data` a `graph.SummaryData` |
| `localSearch` | the `COMMUNITY_INDEX` view, then `graph.ingest.query.prefix`, one page | enveloped by `graph-query`, `data` a `graph.LocalSearchData` |
| `globalSearch` | one strategy chosen from the question: the alias and entity verbs, the index's edge verbs, or the time-range or bounds verb | enveloped by `graph-query`, `data` a `graph.GlobalSearchData` |
| `semantic`, `similar` | `graph.embedding.query.search`, `similar` | enveloped by the responder |
| `spatial`, `temporal` | `graph.spatial.query.bounds`, `graph.temporal.query.range` | enveloped by the responder |

The first four, and `semantic`, `similar`, `spatial` and `temporal`, are forwards: the request and reply bytes pass
unchanged, neither decoded nor checked, so `min_revision` reaches graph-index on `byName` and an authority read on
`entity`, `batch` and `prefix` ignores it. The responders of the last four are not in this tree yet: the embedding
responder arrives in change 5, the spatial and temporal ones in change 7. Until then each of the four is refused
transient `internal`, naming the subject it forwards to.

A refusal from any verb behind these reaches the caller with its class, code, detail and message. A request that gets
no answer is refused by `graph-query` itself, class transient, code `internal`, naming the subject: nothing responding,
`query_timeout_ms` passing, the handler's context ending (`natsclient`'s limit on a request handler, or the client
closing), or a lost connection.

### `entityByAlias`

The request is `{"alias": "...", "min_revision": ...}`, and its bytes go to the alias verb unchanged. An alias with a
holder reads that entity. An alias nobody holds reads the input as an entity ID when it is a valid one, so an ID works
here too. Input that is neither is refused, class invalid, code `entity_not_found`, with no second request.

`data` is the entity verb's JSON as it came, not re-decoded into a struct, so a field `graph-query` does not know
stays. An entity reply that is not JSON is refused transient `internal`. `indexed_revision` is the alias reply's, which
is not the entity's `kv_revision`: it says how far the index had read when it answered.

### `relationships`

The request is `{"entity_id": "...", "direction": "...", "min_revision": ...}`, and its bytes go to the verb for that
direction unchanged. `direction` is required and is `outgoing` or `incoming`; anything else is `invalid_request`, and
no request goes out. An entity with no links in that direction answers `[]`. Each row in `data` has `predicate`,
`from_entity_id` and `to_entity_id`, whichever way it was asked for, and `indexed_revision` is the index reply's.

### `pathSearch`

The request is `{"start_entity": "...", "direction": "...", "max_depth": 3, "max_nodes": 100, "predicates": [...],
"min_revision": ...}`. `start_entity`, `direction` and `max_depth` are required. `direction` is `outgoing`, `incoming`
or `both`. `max_depth` is 1 to 10, and `max_nodes`, the most entities in the reply with the start counted, is 1 to 500
(0 or absent means 100). A value outside a limit is refused `invalid_request`, naming the bound, before any request
goes out; nothing is clamped. `predicates`, when present, keeps only edges holding one of them. An unknown key is
refused.

The walk reads the start through the entity verb, so a start with no record is that verb's `entity_not_found`. It then
asks the index for each entity's edges, nearest first. `data` has `entities` (each with its `depth`, the start at 0),
`edges` and `truncated`. Every entity and edge appears once, and `edges[i]` is the edge that first reached
`entities[i+1]`, named as the graph holds it (`from_entity_id` to `to_entity_id`), whichever way the walk crossed it, so
a path is the edges followed back. `truncated` is true exactly when the walk found an entity it had no room for; false
means `entities` is everything in reach. `indexed_revision` is the smallest of the revisions the hops answered at.

A hop that fails ends the walk with no partial answer; the refusal keeps its class, code and detail, and its message
names the entity being read. That includes a hub: an entity whose edge list in one direction is too large for one
reply is refused `response_too_large`, class invalid, naming that entity, so a caller can find it.
Narrowing `predicates` does not shrink that reply: a hop sends only `entity_id` and `min_revision`, and `predicates`
filters after the hop. Narrowing can only avoid reaching the hub in the first place, and changing `direction` helps
only when it avoids the oversized direction.

Cost is counted in requests: at most one plus `max_nodes`, and twice that for `both`. Nothing bounds the elapsed time of
a walk: the handler's context ends a wait for a reply, but not a wait to write behind a broker that has stopped reading
(ADR-112, "Time").

### `hierarchyStats` and `summary`

Both read one page of entity IDs under a prefix from graph-ingest, so each request costs one scan of that group's keys
and a caller who wants more pays per page. A count covers that page and no more.

- `total_entities` is the whole group as listed, before the cursor and any limit. It is live and as listed, not a
  snapshot: under concurrent overwrites a listing can omit a key (#512). A repeated key counts once.
- `entities_scanned` is the rows on this page. `truncated` is true exactly when `next_cursor` is set, so false says only
  that this is the last page, not that the counts are the whole group. Send `next_cursor` as `cursor` for the next page.
- On a first page with `truncated` true, the type counts, children and examples are of the lexically smallest IDs, a
  sample of the group. A page read with a cursor covers that page.

`hierarchyStats` takes `{"prefix": "...", "cursor": "..."}`; an empty prefix counts the whole graph. Its reply is bare.
`children` are `{prefix, count}` for the prefix one segment longer, by count then prefix. A prefix that is itself a
full entity ID has no children; on the first page of an ID that exists, `entities_scanned` is 1, and for an absent ID,
or a cursor past it, 0. `min_revision` is accepted and ignored, since no index is
read.

`summary` takes `prefix`, `cursor`, `examples_per_type` (default 2; negative is refused), `exclude_predicates` and
`min_revision`. It lists the predicate counts first, then reads the page, so the two are not one snapshot. Predicate
counts cover the whole graph, so `prefix` is refused unless `exclude_predicates` is set. A predicate index that is down
or behind refuses the whole request with its own class, code and detail; `exclude_predicates: true` asks for the
entities alone, and `min_revision` is then ignored. An empty body is refused; send `{}`. `predicates` is `null` when
excluded and `[]` when there are none.

With predicates excluded, `summary` reads no index and reports `indexed_revision` 0, which claims nothing. Do not
compare it with a write's `kv_revision`: it never catches up.

A page graph-query cannot count is refused transient `internal`, naming the subject: one with no `entities` or `total`
key, or a row whose ID does not parse or is not under the prefix. An empty group is an answer, not that refusal.

### `localSearch`

Searches one group of entities, such as everything under `acme.ops.src.docs`, for the words of a query. A *group* is
every entity sharing the first 4 (level 0), 3 (level 1) or 2 (level 2) parts of an ID; `graph/clustering` keeps one
record per group in `COMMUNITY_INDEX`: its member count, keywords, a summary and a few sample IDs.

The request is `{"entity_id": "...", "query": "...", "level": 0, "min_revision": ...}`. `entity_id` is a full six-part
ID, and the group searched is the one it belongs to at `level` (0 to 2, absent means 0). `query` is lowercased and
split on white space with repeats removed, and must leave 1 to 16 terms. A bad field is `invalid_request`, naming it,
before anything is read. An unknown key is refused.

It reads the group's record from the view, then one page of at most 1,000 entity IDs under the group's prefix from
graph-ingest, in ID order. Each entity on the page is scored on the terms: a term that equals the entity's type (the
fifth ID part) counts 1.0; otherwise 0.5 if it is a substring of a predicate name or a string value; the sum is divided
by the number of terms. An entity scoring 0.3 or more matches. Matches come best first, ties by entity ID, so the same
graph answers the same way.

`data` (a `graph.LocalSearchData`) has the record's `community_id`, `level`, `member_count`, `community_summary` and
`keywords`; `entities`, the matches exactly as graph-ingest sent them; `entities_scanned`, the entities read;
`entities_truncated`; and `answer`, a sentence such as `Found 3 matching entities among 40 read from
acme.ops.src.docs.` followed by the record's summary. `entities_truncated` is true when the group had more than the one
page, or when matches were left out so the reply fits the connection's `max_payload`. The matches kept are then the
best of what was read, not of the whole group, and a match further down the page order can be missed.
`indexed_revision` is the record's `entity_revision`, the latest entity write the record reflects.

A request with `min_revision` above the record's `entity_revision` is refused transient `revision_not_reached` before
the page is read. The revision must come from a write to an entity in the group searched: a write elsewhere does not
move the record, so the refusal can last until something in the group changes.

Other refusals:

- Transient `index_not_ready` while `COMMUNITY_INDEX` does not exist, its view is catching up, or its watch is lost,
  and when the entity exists but its group has no record yet. Nothing in the framework writes `COMMUNITY_INDEX` until
  `processor/graph-clustering` arrives in change 7, so until then every `localSearch` on an existing entity is refused
  this way.
- When the group has no record and the entity does not exist, the entity verb's own refusal, such as `entity_not_found`.
- Transient `internal`, counted in `ErrorCount`, for a record that cannot be read (the message names its key), for a
  page that is broken or has no responder, and for a reply that cannot fit `max_payload` even with no entities.

### `globalSearch`

Answers a free-text question by choosing one strategy from what the question says and running it. This build answers
only the questions that have an exact answer without a text index; it refuses the rest, and says so, rather than
return an empty or partial answer.

The request is `{"query": "...", "min_revision": ...}`. `query` is required and not blank. An unknown key is refused,
and so are the pin's `level`, `max_communities`, `summarize_threshold` and `include_*` keys, by name.

How the question is read:

- **One word is a name.** A question with no white space is looked up as an entity reference, before anything else
  reads it, so an ID or alias that holds a word such as `today`, `zone-a`, `link` or `max` is not mistaken for a time
  range, a path or a count. The lookup is `entityByAlias`'s: the alias index first (a holder wins), else the word as a
  six-part entity ID. A partial ID such as `sensor-001` resolves only as an alias.
- **Any longer question is classified once** by `graph/query`'s keyword classifier (no language model), which reports
  hints: a path from a start entity, a time range, an area, a count or ranking, a similarity. A strategy answers only
  when its hint is the only hint in the question.

| The question | Strategy | `data` holds |
| --- | --- | --- |
| One word, or a path question whose start is found ("what is connected to pump-42") | `entity` for a word; `path` for a path | `entity`: the entity verb's reply as it came. `path`: the `pathSearch` result from the start, both directions, depth 3, 100 entities, all predicates, with no entity states (call `batch` for those) |
| A time range alone ("what changed in the last 2 hours") | `temporal` | the time-range verb's `results` and `truncated`, `limit` 100 |
| An area alone ("what is within 5km of 40.7,-74.0") | `spatial` | the bounds verb's `results` and `truncated`, `limit` 100 |

`indexed_revision` is the one the strategy's own read reported: the alias reply's for `entity`, the walk's for `path`,
the responder's for `temporal` and `spatial`. `min_revision` reaches each of them. The reply never loads an entity
beside the result.

What it refuses:

| The question | Refusal |
| --- | --- |
| Two of path, time range and area; an entity beside a zone or area ("what is connected to sensor-001 in zone-b"); two different zones; an impossible number ("the last 0 hours") | class invalid, code `invalid_request`, naming both places or the phrase |
| A count ("how many"), ranking ("top 5") or similarity ("similar to") hint, with or without others; no hint at all; path words with no start; a name or ID that names no entity | class invalid, code `text_search_unsupported`: this build has no text search, step 5 adds it, and asking again unchanged will not help |
| A refusal from the alias or entity verb behind a lookup (`alias_ambiguous`, `index_not_ready`, `internal`, `revision_not_reached`, and the entity verb's others) | the same refusal, as it arrived. `entity_not_found` from the entity verb is the exception: it becomes `text_search_unsupported` |
| A time-range or area question, until change 7 | transient `internal`, naming the subject, counted as an error in `Health`: their responders are not in the tree |

`text_search_unsupported` counts no error in `Health`, like a caller's `invalid_request`. A classification that ends
with the request, or a hint the route cannot read, is transient `internal` and counts one.

Known limits of the keyword classifier, accepted by the owner and tracked in #535:

- "From A to B" reads one start. The walk begins at A and B is dropped without a word.
- A path start is the hyphenated word the classifier finds, never a whole ID: "what is connected to
  acme.plant.robotics.gcs.drone-x.001" walks from the alias `drone-x`. Name the start by alias.
- The refusal of an entity beside a zone or area is loud and can be wrong. An ordinary hyphenated word ("in
  real-time", "non-zero") or "zone" or "area" plus a word ("the area with pumps") beside a place can be read as a
  second place and refused `invalid_request`. The failure is a refusal, never a wrong answer.

Not carried from the pin: the semantic strategy and its type-filter fallback, the `graphrag` community search, the
pin's `count`, `duration_ms`, `answer`, `community_summaries` and `degraded` reply fields. `MIGRATION.md` has the
decoder changes.

### A reply `graph-query` cannot use

An index reply to either verb that does not decode, or names no `producer` (`{}` decodes), is refused, class transient,
code `internal`, naming the subject, so it never reads as no holder or no rows. So is an entity reply to `entityByAlias`
that is not JSON. Nothing else about the entity reply is checked.

## Configuration

An empty configuration is enough. This is the same configuration written out:

```json
{
  "type": "processor",
  "name": "graph-query",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {
          "name": "graph_queries",
          "required": true,
          "config": {
            "kind": "nats-request",
            "subject": "graph.query.*",
            "interface": {"type": "graph.query", "version": "v1"}
          }
        }
      ]
    },
    "query_timeout_ms": 15000
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the port above | A `ports` block that is present is used as written; one that is absent takes the default |
| `query_timeout_ms` | int | 15000 | How long `graph-query` waits for the reply to each request it makes to graph-ingest or graph-index, in milliseconds. `0` means the default. Negative is refused, and so is anything above 9223372036854, which does not fit a Go duration. |

The default is above graph-ingest's 10 second budget on `batch` and `prefix`, so the slowest of those can finish. A
larger value gains little past `natsclient`'s limit on a request handler, 30 seconds by default, which also ends the
wait for a reply.

Both are deadlines on that wait, not on the whole request. `nats.go` takes one lock for every write on a connection, so
behind a broker that has stopped reading a request can be held past either one, for as long as a write ahead of it holds
that lock. ADR-112's "Time" section has the measurements. `entityByAlias` makes two requests, and each gets its own
`query_timeout_ms`.

The configuration is decoded strictly: a key not listed here is refused at construction, naming it. In the SemStreams
version `query_timeout` took a number of nanoseconds, and keys nothing read, such as `max_traversal_depth`, were
accepted and dropped. A configuration that still sets one of them fails at boot until the line is changed or deleted.

## Ports

| Direction | Name | Kind | Subject |
| --- | --- | --- | --- |
| input | `graph_queries` | nats-request | `graph.query.*` |

The one input is required and has to be this port: a different name, subject, kind or interface, a `timeout` other
than `1s` (the value it takes when none is written) or `retries`, which nothing would read, a second input, or any
output is refused. Its `description` is yours to set, and so is `external`, which says requests come from outside the
composition. A composition runs one graph-query: a second instance, or another component declaring a NATS-request input
on one of these verbs, is the `request_subject_conflict` error that refuses boot (`composition.Analyze`).

## Health

`Health` is running or stopped, from `component.Lifecycle`. It keeps no state that could leave it degraded, and
`ErrorCount` never lowers `Healthy`.

`DataFlow` counts every request a verb receives, with its bytes, once however many requests it makes downstream; the
replies' bytes are not counted. `ErrorCount`, and `DataFlow`'s `ErrorRate` with it, counts the requests `graph-query`
refuses because no usable answer arrived, once per request: a downstream request got no answer, so an outage of
graph-ingest or graph-index shows here, or a reply `entityByAlias` or `relationships` reads was broken, so a broken
responder shows here too. So does a `localSearch` that cannot read its record or its page, or fit its reply. A
refusal the responder sent is an answer and is not counted: "no such entity" is ordinary traffic. Neither is a
caller's `invalid_request`.

## Metrics

Collectors register on the injected `metric.MetricsRegistry`, never on Prometheus' global registry; with none, they
count and register nowhere. A second `graph-query` on one registry counts into the collector the first registered.

| Name | Type | Meaning |
| --- | --- | --- |
| `semengine_graph_query_search_truncated_total` | counter vec | `localSearch` answers cut short, by `verb` |

A `localSearch` answer is cut short when its group had more entities than the one page read (at most 1,000), or
matches were left out to fit `max_payload`: both set `entities_truncated`, and each adds one to the counter. A search
over a large community can miss a match further down the page order, so this count is how many answers were cut short;
it says when a keyword index per community (#506) is due. A refused request, and an answer that is whole, count nothing.

## Starting and stopping

`Start` needs a NATS client, and its context bounds the subscribing only. It starts one goroutine that keeps a view of
`COMMUNITY_INDEX`, under a run context that `Stop` ends, not `Start`'s context. `Start` neither waits for the bucket nor
fails when it does not exist: until it does, `localSearch` is refused `index_not_ready` and every other verb answers.
While the bucket is missing or its watch is lost, the goroutine tries again every 5 seconds, and the view refuses until
a new one has caught up. Each failure is logged at `Warn` when it is the first, differs from the last, or follows a
view that had opened, and at `Debug` otherwise.

`Stop` drains each verb's subscription, ends the view and returns once every running handler and the view have
returned. If its context ends first, it returns that error and keeps the rest for the next `Stop`. If the view could not
stop its watch cleanly, the `Stop` that waited for it returns that failure, and a later `Stop` returns nil, since there
is nothing left to retry (`TestShutdownFailureKeepsAWatchCleanupFailure`). If a failed `Start` cannot clean up after
itself, `Start` reports both errors and a later `Stop` finishes the job.

## Dependencies

- Registered by a composition root through `Register`; importing the package registers nothing.
- Consumers read `graph.QueryVerbs()` for subjects and spell none as a literal.
