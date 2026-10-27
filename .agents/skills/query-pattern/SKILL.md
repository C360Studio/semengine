---
name: query-pattern
description: >-
  Choose how code reads the graph: graph-ingest's and graph-index's declared request verbs, the typed exact-entity
  reader, or an owner's bucket access. Use when a component or a consumer needs to read entities or relationships.
---

# How to read the graph

Start from the reader the user names. In this tree the graph is read four ways, and only these.

## What exists

| Way | For | What you get |
| --- | --- | --- |
| `graph.NewExactEntityReader` | One entity by ID, from embedded code | The validated entity and its `ENTITY_STATES` revision (`graph.ExactEntity`) |
| graph-ingest's `batch` and `prefix` verbs | Several entities by ID, or a page of entities under an ID prefix | `graph.EntityBatchResponse`, `graph.PrefixQueryResponse` |
| graph-index's eight verbs | Relationships in or out of an entity, an alias, the entities with a predicate or a name | A `graph.QueryResponse`: the result in `data`, the revision it reflects in `indexed_revision`, and `producer` |
| Direct bucket access | The bucket's declared owner, and operator diagnostics | Raw entries; no query contract |

Every verb graph-ingest and graph-index serve is declared once, in `graph.QueryVerbs()` (`graph/query_verbs.go`),
with its subject and its responder; the function's doc comment names each verb's request and reply types. Each
component serves exactly its own verbs. graph-index's are `outgoing`, `incoming`, `alias`, `predicate`,
`predicateList`, `predicateStats`, `predicateCompound` and `byName`, on `graph.index.query.*`.

## Choosing

1. **One entity by ID:** use `graph.NewExactEntityReader`. It is the only typed adapter so far, and the only read that
   returns the revision with the value.
2. **Several entities, or a prefix:** send a classified request (`RequestClassified`, developer contract § NATS RPC)
   on the verb's subject, read from `graph.QueryVerbs()`. Never spell the subject as a literal;
   `TestReservedSubjectsDeclaredOnce` fails on one outside its declaration.
3. **Relationships, aliases, predicates or names:** send a classified request on the matching `graph.index.query.*`
   verb, read from `graph.QueryVerbs()`. Every one of the eight refuses with transient `index_not_ready` during the
   index's startup rebuild and while it has lost its feed, so retry it; it refuses with fatal
   `graph_state_reset_required` after a stored entity could not be decoded, so do not retry. Compare a write's
   `kv_revision` with the reply's `indexed_revision` to know whether the answer includes that write; the `alias`
   verb is the exception until issue #415 is fixed: it can still return an alias that was dropped or whose entity was
   deleted.
4. **Anything else:** there is no admitted read. Raw KV access and subjects are not a fallback for an application;
   direct bucket access is for the declared owner (`natsclient.OpenFrameworkBucket` with the descriptor from
   `graph/kvcatalog`) and for operator diagnostics.

## Points that hold

- Which front door, and where its answer comes from, are separate decisions.
- The `prefix` verb sorts before it pages and clamps its limit to `graph.MaxPrefixQueryLimit`; a caller pages with
  the cursor it returns.
- graph-index's replies are enveloped (`graph.QueryResponse`); graph-ingest's replies are not.
- The query service, the HTTP gateway, an MCP surface and the GraphRAG and PathRAG strategies of the pin's version of
  this skill are not in this tree. graph-query arrives later in change 4; until a surface is in the tree and
  specified, do not design against it.
