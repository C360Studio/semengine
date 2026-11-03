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
| graph-query's six verbs | One front door for application code: an entity, a batch, a prefix page, a name, an alias, or the links of an entity | The answer of the component behind the verb; `entityByAlias`, `relationships` and `byName` are a `graph.QueryResponse` |
| graph-index's eight verbs | Relationships in or out of an entity, an alias, the entities with a predicate or a name | A `graph.QueryResponse`: the result in `data`, the revision it reflects in `indexed_revision`, and `producer` |
| Direct bucket access | The bucket's declared owner, and operator diagnostics | Raw entries; no query contract |

Every verb graph-query, graph-ingest and graph-index serve is declared once, in `graph.QueryVerbs()` (`graph/query_verbs.go`),
with its subject and its responder; the function's doc comment names each verb's request and reply types. Each
component serves exactly its own verbs. graph-index's are `outgoing`, `incoming`, `alias`, `predicate`,
`predicateList`, `predicateStats`, `predicateCompound` and `byName`, on `graph.index.query.*`. graph-query's, on
`graph.query.*`, are `entity`, `batch`, `prefix`, `byName`, `entityByAlias` and `relationships`; the first four forward
to the verb of the same name behind them. graph-query lands in steps (`processor/graph-query/README.md` lists what it
serves now), so a verb not in the table has no responder yet.

## Choosing

1. **A lookup an application makes:** send a classified request on the matching `graph.query.*` verb, read from
   `graph.QueryVerbs()`. Pick the verb by the table's row, not by a verb's name: `entityByAlias` takes
   `{"alias": ...}` and answers the entity whose alias it is, or reads the input as an entity ID; `relationships`
   takes `{"entity_id": ..., "direction": "outgoing" or "incoming"}` and the direction is required.
2. **One entity by ID, from embedded code:** use `graph.NewExactEntityReader`. It is the only typed adapter so
   far, and the only read that returns the revision with the value.
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

## Reading a reply

Decode by the verb's row in `graph.QueryVerbs()`, never by looking at the bytes. `QueryVerb.Enveloped` says whether
the reply is a `graph.QueryResponse`; `QueryVerb.Reply` is the type of the reply, or of its `data` when enveloped.
Do not guess the shape from the reply's content.

`graph-query`'s `entityByAlias` and `relationships` replies are enveloped and were bare lists or entities at the pin, so
code written against the pin must change (`processor/graph-query/MIGRATION.md`).

## Reading your own write

Every enveloped verb takes an optional `min_revision`, the `kv_revision` of your write (`graph.MinRevisionField`). If
the index has not reached it, the verb refuses with transient `revision_not_reached`, the revision reached in the
detail, and never waits; retry it. A bare verb (`entity`, `batch`, `prefix`) ignores the field: graph-ingest, which
answers it, is the authority. On `graph.query.*`, `byName`, `entityByAlias` and `relationships` honour it.

## Points that hold

- Which front door, and where its answer comes from, are separate decisions.
- The `prefix` verb sorts before it pages and clamps its limit to `graph.MaxPrefixQueryLimit`; a caller pages with
  the cursor it returns.
- graph-index's replies are enveloped (`graph.QueryResponse`); graph-ingest's replies are not; graph-query's follow
  the table row for each verb.
- A refusal is a classified error and carries no envelope. `graph-query` passes a refusal from the verb behind it on
  with its class, code and detail; a request that gets no answer is transient `internal`.
- The HTTP gateway, an MCP surface, and the path search, community, summary, semantic and `searchGraph` verbs of the
  pin's graph-query are not in this tree yet. They arrive in later steps of graph-query (change 4); until a verb is
  in the table, do not design against it.
