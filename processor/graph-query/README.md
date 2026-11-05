# graph-query

The component that answers the graph's read requests on `graph.query.*`.

## Overview

`graph-query` sits in front of graph-ingest, which holds entities, and graph-index, which holds the relationship and
name indexes. It keeps no state of its own and writes nothing.

`graph.QueryVerbs()` (`graph/query_verbs.go`) decides what it serves: at start it subscribes a handler to each verb
declared for `graph-query`, and fails to start if a declared verb has no handler. Any other `graph.query.*` subject is
left alone, so a consumer can serve its own verb there.

graph-query is ported in steps; steps 1 and 2 are in the tree, and the table below is what they serve.
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
reply is refused `response_too_large`, class invalid, naming that entity, so a caller can find it and narrow
`predicates` or `direction`.

Cost is counted in requests: at most one plus `max_nodes`, and twice that for `both`. Nothing bounds the elapsed time of
a walk: the handler's context ends a wait for a reply, but not a wait to write behind a broker that has stopped reading
(ADR-112, "Time").

### `hierarchyStats` and `summary`

Both read one page of entity IDs under a prefix from graph-ingest, so each request costs one scan of that group's keys
and a caller who wants more pays per page. A count covers that page and no more.

- `total_entities` is the whole group as listed, before the cursor and any limit. It is live and as listed, not a
  snapshot: under concurrent overwrites a listing can repeat a key and omit another (#512).
- `entities_scanned` is the rows on this page. `truncated` is true exactly when `next_cursor` is set, so false says only
  that this is the last page, not that the counts are the whole group. Send `next_cursor` as `cursor` for the next page.
- On a first page with `truncated` true, the type counts, children and examples are of the lexically smallest IDs, a
  sample of the group. A page read with a cursor covers that page.

`hierarchyStats` takes `{"prefix": "...", "cursor": "..."}`; an empty prefix counts the whole graph. Its reply is bare.
`children` are `{prefix, count}` for the prefix one segment longer, by count then prefix. A prefix that is itself a
full entity ID has no children and an `entities_scanned` of 1. `min_revision` is accepted and ignored, since no index is
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
responder shows here too. A refusal the responder sent is an answer and is not counted: "no such entity" is ordinary
traffic. Neither is a caller's `invalid_request`.

## Starting and stopping

`Start` needs a NATS client, and its context bounds the subscribing only. `Stop`
drains each verb's subscription, and returns once every running handler has returned. If its context ends first, it
returns that error and keeps the rest for the next `Stop`. If a failed `Start` cannot clean up after itself, `Start`
reports both errors and a later `Stop` finishes the job.

## Dependencies

- Registered by a composition root through `Register`; importing the package registers nothing.
- Consumers read `graph.QueryVerbs()` for subjects and spell none as a literal.
