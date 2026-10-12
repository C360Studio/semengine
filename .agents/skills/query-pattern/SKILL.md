---
name: query-pattern
description: >-
  Choose how code reads the graph: graph-ingest's declared request verbs, the typed exact-entity reader, or an owner's
  bucket access. Use when a component or a consumer needs to read entities.
---

# How to read the graph

Start from the reader the user names. In this tree the graph is read three ways, and only these.

## What exists

| Way | For | What you get |
| --- | --- | --- |
| `graph.NewExactEntityReader` | One entity by ID, from embedded code | The validated entity and its `ENTITY_STATES` revision (`graph.ExactEntity`) |
| graph-ingest's `batch` and `prefix` verbs | Several entities by ID, or a page of entities under an ID prefix | `graph.EntityBatchResponse`, `graph.PrefixQueryResponse` |
| Direct bucket access | The bucket's declared owner, and operator diagnostics | Raw entries; no query contract |

Every verb graph-ingest serves is declared once, in `graph.QueryVerbs()` (`graph/query_verbs.go`), with its subject,
its responder and its request and reply types. graph-ingest serves exactly those verbs.

## Choosing

1. **One entity by ID:** use `graph.NewExactEntityReader`. It is the only typed adapter so far, and the only read that
   returns the revision with the value.
2. **Several entities, or a prefix:** send a classified request (`RequestClassified`, developer contract § NATS RPC)
   on the verb's subject, read from `graph.QueryVerbs()`. Never spell the subject as a literal;
   `TestReservedSubjectsDeclaredOnce` fails on one outside its declaration.
3. **Anything else:** there is no admitted read. Raw KV access and subjects are not a fallback for an application;
   direct bucket access is for the declared owner (`natsclient.OpenFrameworkBucket` with the descriptor from
   `graph/kvcatalog`) and for operator diagnostics.

## Points that hold

- Which front door, and where its answer comes from, are separate decisions.
- The `prefix` verb sorts before it pages and clamps its limit to `graph.MaxPrefixQueryLimit`; a caller pages with
  the cursor it returns.
- The query service, the HTTP gateway, an MCP surface and the GraphRAG and PathRAG strategies of the pin's version of
  this skill are not in this tree. graph-query arrives in change 4; until a surface is in the tree and specified, do
  not design against it.
