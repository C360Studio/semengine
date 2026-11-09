---
name: query-pattern
description: >-
  Choose how code reads the graph: graph-query's, graph-ingest's and graph-index's declared request verbs, the typed
  exact-entity reader, or an owner's bucket access. Use when a component or a consumer needs to read entities or relationships.
---

# How to read the graph

Start from the reader the user names. In this tree the graph is read five ways, and only these.

## What exists

| Way | For | What you get |
| --- | --- | --- |
| `graph.NewExactEntityReader` | One entity by ID, from embedded code | The validated entity and its `ENTITY_STATES` revision (`graph.ExactEntity`) |
| graph-ingest's `batch` and `prefix` verbs | Several entities by ID, or a page of entities under an ID prefix | `graph.EntityBatchResponse`, `graph.PrefixQueryResponse` |
| graph-query's verbs | One front door for application code: an entity, a batch, a prefix page, a name, an alias, the links of an entity, the entities within reach of one, counts of a group, a summary of the graph, a keyword search inside one community | The answer of the component behind the verb; every verb but `entity`, `batch`, `prefix` and `hierarchyStats` is a `graph.QueryResponse` |
| graph-index's eight verbs | Relationships in or out of an entity, an alias, the entities with a predicate or a name | A `graph.QueryResponse`: the result in `data`, the revision it reflects in `indexed_revision`, and `producer` |
| Direct bucket access | The bucket's declared owner, and operator diagnostics | Raw entries; no query contract |

Every verb graph-query, graph-ingest and graph-index serve is declared once, in `graph.QueryVerbs()` (`graph/query_verbs.go`),
with its subject and its responder; the function's doc comment names each verb's request and reply types. Each
component serves exactly its own verbs. graph-index's are `outgoing`, `incoming`, `alias`, `predicate`,
`predicateList`, `predicateStats`, `predicateCompound` and `byName`, on `graph.index.query.*`. graph-query's, on
`graph.query.*`, are `entity`, `batch`, `prefix`, `byName`, `entityByAlias`, `relationships`, `pathSearch`,
`hierarchyStats`, `summary`, `localSearch`, `globalSearch`, `searchGraph`, `semantic`, `similar`, `spatial` and
`temporal`; the first four forward to
the verb of the same name behind them, and `semantic`, `similar`, `spatial` and `temporal` forward to the embedding,
spatial and
temporal verbs. Those four have no responder in the tree yet and are refused transient `internal` until changes 5 and 7.
graph-query lands in steps (`processor/graph-query/README.md` lists what it
serves now), so a verb not in the table has no responder yet.

## Choosing

1. **A lookup an application makes:** send a classified request on the matching `graph.query.*` verb, read from
   `graph.QueryVerbs()`. Pick the verb by the table's row, not by a verb's name: `entityByAlias` takes
   `{"alias": ...}` and answers the entity whose alias it is, or reads the input as an entity ID; `relationships`
   takes `{"entity_id": ..., "direction": "outgoing" or "incoming"}` and the direction is required.
2. **One entity by ID, from embedded code:** use `graph.NewExactEntityReader`. It is the only typed adapter so
   far.
3. **Several entities, or a prefix, straight from graph-ingest:** send a classified request (`RequestClassified`,
   developer contract § NATS RPC) on the verb's subject, read from `graph.QueryVerbs()`. Never spell the subject as a literal;
   `TestReservedSubjectsDeclaredOnce` fails on one outside its declaration.
4. **Relationships, aliases, predicates or names, straight from graph-index:** send a classified request on the
   matching `graph.index.query.*` verb, read from `graph.QueryVerbs()`. Every one of the eight refuses with transient `index_not_ready`
   during the index's startup rebuild and while it has lost its feed, so retry it; it refuses with fatal
   `graph_state_reset_required` after a stored entity could not be decoded, so do not retry. Compare a write's
   `kv_revision` with the reply's `indexed_revision` to know whether the answer includes that write, the `alias` verb
   included. The `alias` verb also refuses with class invalid and code `alias_ambiguous` when two or more entities
   carry the alias; do not retry it until a holder drops the alias.
5. **Anything else:** there is no admitted read. Raw KV access and subjects are not a fallback for an application;
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
- **`localSearch`** searches one community: the group of entities that share the first 4, 3 or 2 parts of an ID
  (`level` 0, 1 or 2, default 0). It takes `entity_id` (a full ID in the group), `query` (1 to 16 distinct words) and
  optionally `level`. It reads the group's record from `COMMUNITY_INDEX`, then one page of the group's entities, and
  ranks that page by how well the words match. The page is at most 1,000 entities in ID order, so on a larger group the
  sample is the lexically smallest IDs, `entities_truncated` is true, and a match later in the order is missed. Until
  `processor/graph-clustering` (change 7) writes `COMMUNITY_INDEX` it refuses transient `index_not_ready`; retry it.
  Its `indexed_revision` is the group record's latest entity write, so to read your own write send the `kv_revision` of
  a write to an entity in the group you search. A revision from a write to another group can stay above the record, and
  the request is then refused `revision_not_reached` until the searched group next changes.
- **`globalSearch`** takes a free-text `query` and picks one strategy from the question's words, with no language
  model. `searchGraph` is the same handler under a second name, with the same request and `data`. It answers the
  questions that have an exact answer, searches the rest as text over `COMMUNITY_INDEX`, and refuses a question it
  cannot answer whole:
  - A question of one word is an entity reference (an alias, else a full six-part ID): `strategy` `entity`.
  - "What is connected to pump-42": a path from `pump-42`, both directions, 3 hops, 100 entities: `path`, IDs and
    edges only (call `batch` for states).
  - "What changed in the last 2 hours": `temporal`. "What is within 5km of 40.7,-74.0": `spatial`. Until change 7 both
    are refused transient `internal`, since their responders are not in the tree.
  - Two of path, time and area, an entity beside a zone or area, or an impossible number is `invalid_request`. A
    count or other aggregation ("how many", "max retries"), and path words, a similarity or a ranking beside a start,
    time range or area, are `invalid_request` naming what it cannot do. A named path start that names no entity is
    `entity_not_found`.
  - Everything else is the `text` strategy: a question with no hint, path words with no start ("http connection
    pool"), a similarity or ranking alone, a one-word name that names no entity. It reads up to five groups'
    records at `level` (0 to 2, default 0) that score above 0 for the words, one page of each, and replies with
    `matches` (`id`, `label`, `score`, `entity`), the `groups` read, `entities_truncated` and `answer`. Matches are
    a sample of at most one page per group. Hints it could not apply are named in `unhonoured_hints`. Until change 7
    writes `COMMUNITY_INDEX` it is refused transient `index_not_ready`; retry it.
  - Known limits of the keyword classifier (owner-accepted, #535): "from A to B" walks from A and drops B with no
    signal, so a successful reply does not mean both ends were honoured; "in real-time" beside a place can be refused
    as a second place, a loud refusal of an answerable question.
  - To read a known entity, prefer `entity` or `entityByAlias`; `globalSearch` is for a question you cannot route
    yourself.
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
ignores the field: graph-ingest, which answers it, is the authority. On `graph.query.*`, `byName`, `entityByAlias`,
`relationships`, `pathSearch`, `localSearch`, `globalSearch`, `searchGraph` and `summary` (unless it excludes
predicates) honour it. `localSearch` compares it with the searched group's record, not the whole index (see above);
the text strategy of `globalSearch` compares it with the highest revision among the records at its `level`, which a
write anywhere reaches once clustering has published it.

## Points that hold

- Which front door, and where its answer comes from, are separate decisions.
- The `prefix` verb sorts before it pages and clamps its limit to `graph.MaxPrefixQueryLimit`; a caller pages with
  the cursor it returns.
- graph-index's replies are enveloped (`graph.QueryResponse`); graph-ingest's replies are not; graph-query's follow
  the table row for each verb.
- A refusal is a classified error and carries no envelope. `graph-query` passes a refusal from the verb behind it on
  with its class, code and detail; a request that gets no answer is transient `internal`.
- The HTTP gateway and an MCP surface are not in this tree yet. `searchGraph` is in graph-query's table (step 5);
  until a verb is in the table, do not design against it.
