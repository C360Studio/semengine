---
name: query-pattern
description: >-
  Choose how code reads the graph: graph-query's, graph-ingest's and graph-index's declared request verbs, the typed
  exact-entity reader, or an owner's bucket access. Use when a component or a consumer needs to read entities or relationships.
---

# How to read the graph

Start from the reader the user names. In this tree the graph is read seven ways, and only these.

## What exists

| Way | For | What you get |
| --- | --- | --- |
| graph-gateway's GraphQL route | Remote apps and web front ends, over HTTP: an entity, a prefix page, the entities with a predicate, the entities within reach of one | Exactly the fields the request selected, checked against the schema (`gateway/graph-gateway/schema.graphql`) |
| `output/websocket` with its default applied-state input | An application kept current as entities change, over a WebSocket on the component's own port (ADR-115); never the message logger's debug SSE watch | A `put` per current entity, `synced`, then `put`, `delete` and `poisoned` frames converging to each key's latest value, not every revision (`output/websocket/README.md`) |
| `graph.NewExactEntityReader` | One entity by ID, from embedded code | The validated entity and its `ENTITY_STATES` revision (`graph.ExactEntity`) |
| graph-ingest's `batch` and `prefix` verbs | Several entities by ID, or a page of entities under an ID prefix | `graph.EntityBatchResponse`, `graph.PrefixQueryResponse` |
| graph-query's verbs | One front door for application code: an entity, a batch, a prefix page, a name, the entities with a predicate, an alias, the links of an entity, the entities within reach of one, counts of a group, a summary of the graph, a keyword search inside one community | The answer of the component behind the verb; every verb but `entity`, `batch`, `prefix` and `hierarchyStats` is a `graph.QueryResponse` |
| graph-index's eight verbs | Relationships in or out of an entity, an alias, the entities with a predicate or a name | A `graph.QueryResponse`: the result in `data`, the revision it reflects in `indexed_revision`, and `producer` |
| Direct bucket access | The bucket's declared owner, and operator diagnostics | Raw entries; no query contract |

Every verb graph-query, graph-ingest and graph-index serve is declared once, in `graph.QueryVerbs()` (`graph/query_verbs.go`),
with its subject and its responder; the function's doc comment names each verb's request and reply types. Each
component serves exactly its own verbs. graph-index's are `outgoing`, `incoming`, `alias`, `predicate`,
`predicateList`, `predicateStats`, `predicateCompound` and `byName`, on `graph.index.query.*`. graph-query's, on
`graph.query.*`, are `entity`, `batch`, `prefix`, `byName`, `predicate`, `entityByAlias`, `relationships`,
`pathSearch`, `hierarchyStats`, `summary`, `localSearch`, `globalSearch`, `searchGraph`, `semantic`, `similar`, `spatial`
and `temporal`; the first five forward to
the verb of the same name behind them, and `semantic`, `similar`, `spatial` and `temporal` forward to the embedding,
spatial and
temporal verbs. `spatial` and `temporal` have no responder in the tree yet and are refused transient `internal` until
change 7; `semantic` and `similar` are answered by graph-embedding, or refused invalid `not_enabled` unless graph-query's
`embedding_enabled` is true (false when left out).
graph-query lands in steps (`processor/graph-query/README.md` lists what it
serves now), so a verb not in the table has no responder yet.

## Choosing

1. **A web front end or other HTTP caller:** POST a GraphQL query to graph-gateway at `/<instance>/graphql`. It
   answers `entity`, `entitiesByPrefix`, `entitiesByPredicate` and `pathSearch` and nothing else; a field the schema
   does not declare is refused with HTTP 400, naming it. Its answers and errors are in `gateway/graph-gateway/README.md`.
2. **A lookup an application makes:** send a classified request on the matching `graph.query.*` verb, read from
   `graph.QueryVerbs()`. Pick the verb by the table's row, not by a verb's name: `entityByAlias` takes
   `{"alias": ...}` and answers the entity whose alias it is, or reads the input as an entity ID; `relationships`
   takes `{"entity_id": ..., "direction": "outgoing" or "incoming"}` and the direction is required.
3. **One entity by ID, from embedded code:** use `graph.NewExactEntityReader`. It is the only typed adapter so
   far.
4. **Several entities, or a prefix, straight from graph-ingest:** send a classified request (`RequestClassified`,
   developer contract § NATS RPC) on the verb's subject, read from `graph.QueryVerbs()`. Never spell the subject as a literal;
   `TestReservedSubjectsDeclaredOnce` fails on one outside its declaration.
5. **Relationships, aliases, predicates or names, straight from graph-index:** send a classified request on the
   matching `graph.index.query.*` verb, read from `graph.QueryVerbs()`. Every one of the eight refuses with transient `index_not_ready`
   during the index's startup rebuild and while it has lost its feed, so retry it; it refuses with fatal
   `graph_state_reset_required` after a stored entity could not be decoded, so do not retry. Compare a write's
   `kv_revision` with the reply's `indexed_revision` to know whether the answer includes that write, the `alias` verb
   included. The `alias` verb also refuses with class invalid and code `alias_ambiguous` when two or more entities
   carry the alias; do not retry it until a holder drops the alias.
6. **Anything else:** there is no admitted read. Raw KV access and subjects are not a fallback for an application;
   direct bucket access is for the declared owner (`natsclient.OpenFrameworkBucket` with the descriptor from
   `graph/kvcatalog`) and for operator diagnostics.

## The path and count verbs

- **`pathSearch`** takes `start_entity`, `direction` (`outgoing`, `incoming` or `both`) and `max_depth` (1 to 10), all
  required, and optionally `max_nodes` (1 to 500, default 100) and `predicates`. A value outside a limit is refused
  before any request. Its cost is requests, at most one plus `max_nodes`, twice that for `both`; nothing bounds elapsed
  time (ADR-112, "Time"). `truncated` is exact: true means an entity was left out. A hub whose edge list does not fit
  one reply refuses the whole walk with `response_too_large`, naming the entity. Narrowing `predicates` cannot shrink
  that reply, since a hop sends only `entity_id` and `min_revision` and `predicates` filters after it; narrowing can
  only avoid reaching the hub, and changing `direction` helps only if it avoids the oversized direction.
- **`hierarchyStats` and `summary`** each read one prefix page, so each request costs one key scan of the group, and a
  count covers that page and no more. `total_entities` is the whole group as listed, live and not a snapshot (#512).
  `truncated` false says only that this is the last page. On a first page with `truncated` true, the counts and examples
  are of the lexically smallest IDs: a sample. Send `next_cursor` as `cursor` for the next page. `hierarchyStats` is
  bare. `summary` refuses when the predicate index cannot answer; `exclude_predicates` asks without it, and `prefix`
  needs it.
- **`localSearch` and `globalSearch`** find entities by the words of their text. Each asks graph-lexical's word index
  (BM25, with quoted phrases and `-` exclusions), reads the hits back from graph-ingest, and returns the matches best
  first with the entity as stored. `localSearch` takes `entity_id` (a full ID that names the group, at `level` 0 to 2,
  default 0), `query` and optionally `limit` (1 to 100, default 10), and searches that group only. `globalSearch`
  takes a free-text `query`, `level` and `limit`, searches every entity and lists the groups holding the matches.
  `searchGraph` is the same handler under a second name, with the same request and `data`.
  - Word search helps only entities that have text. Metric-heavy entities, such as sensor or drone telemetry, have no
    words to match and gain nothing from it; reach them by prefix, predicate, name, alias or relationship.
  - A group's `COMMUNITY_INDEX` record is attached when there is one, and never decides the answer: `record_state` is
    `read` (the `record` holds `member_count`, `community_summary` and `keywords`), `none`, `not_ready` or `unreadable`.
    Neither verb waits for `processor/graph-clustering`.
  - `truncated` is true when more entities matched than `limit` returned (`matched` says how many), a hit was gone when
    it was read back (`missing`), or matches were left out to fit `max_payload`. Page with a larger `limit`, not a cursor.
  - `globalSearch` answers every question by its words and lists what else the question seemed to ask for in `hints`:
    `entity` (a one-word question), `path` (a start to walk from), `time_range`, `area`, `aggregation`, `similarity`,
    `ranking`, `classification`. Each hint says whether it was `honoured`; only `entity` and `path` ever are, and then
    `data.entity` or `data.path` holds the answer. A hint never refuses the search. A keyword classifier reads ordinary
    words as hints ("max retries" is an aggregation), so check `honoured` before trusting that a hint was applied.
  - One damaged hit refuses a search until it is fixed (issue #656): the `batch` read refuses every entity when one is
    damaged.
  - A refusal from graph-lexical (`index_not_ready` while it rebuilds, `revision_not_reached`) or from `batch` reaches
    the caller as it arrived; no word index running is a transient `internal`.
  - To read a known entity, prefer `entity` or `entityByAlias`; search is for words you cannot route yourself.
- **`indexed_revision` of 0** on a `summary` with predicates excluded claims nothing. Do not compare it with a write's
  `kv_revision` or retry on it.

## Reading a reply

Decode by the verb's row in `graph.QueryVerbs()`, never by looking at the bytes. `QueryVerb.Enveloped` says whether
the reply is a `graph.QueryResponse`; `QueryVerb.Reply` is the type of the reply, or of its `data` when enveloped.
Do not guess the shape from the reply's content.

`graph-query`'s `entityByAlias` and `relationships` replies are enveloped and were bare lists or entities at the pin, and
the keys of `pathSearch`, `hierarchyStats` and `summary` changed, so code written against the pin must change (`processor/graph-query/MIGRATION.md`).

## Reading your own write

Every enveloped verb takes an optional `min_revision`, the `kv_revision` of your write (`graph.MinRevisionField`). If
the index has not reached it, the verb refuses with transient `revision_not_reached`, the revision reached in the
detail, and never waits; retry it. A bare verb (`entity`, `batch`, `prefix`, and `hierarchyStats` on `graph.query.*`)
ignores the field: graph-ingest, which answers it, is the authority. On `graph.query.*`, `byName`, `predicate`,
`entityByAlias`, `relationships`, `pathSearch`, `localSearch`, `globalSearch`, `searchGraph` and `summary` (unless it
excludes predicates) honour it. The search verbs pass it to the word index, which holds them to a write it has applied.

## Points that hold

- Which front door, and where its answer comes from, are separate decisions.
- The `prefix` verb sorts before it pages and clamps its limit to `graph.MaxPrefixQueryLimit`; a caller pages with
  the cursor it returns.
- graph-index's replies are enveloped (`graph.QueryResponse`); graph-ingest's replies are not; graph-query's follow
  the table row for each verb.
- A refusal is a classified error and carries no envelope. `graph-query` passes a refusal from the verb behind it on
  with its class, code and detail; a request that gets no answer is transient `internal`.
- The HTTP gateway (graph-gateway) answers four operations through a declared schema, never by searching the query
  text; it serves no `trajectory`, `globalSearch` or introspection (`gateway/graph-gateway/MIGRATION.md`). An MCP
  surface is not in this tree. Until a verb is in the table, do not design against it.
