# Design: setup-04c-01-http-embedder

## Context

Premises are pinned in `inventory.md`. Pin lines: 548 (220 production, 328 test); estimated here: PR A ~150
production + ~300 test, PR B ~120 production + ~250 test.

## Goals / Non-Goals

Goal: the smallest complete tier 1 client, the owed degrade test, and #563. Non-goals: as the proposal says.

## Decisions

**D1. Plain net/http; no SDK, no `model`** (required). The OpenAI SDK fails `TestTreeImportsNoProviderPlumbing`
(`internal/harness/contract/embeddingprovider_test.go:207`, owner ruling 2 on PR #582), and `model` is forbidden by
embedding-index (`spec.md:261-262`). One request shape, about 10 lines of wire types. Neither guard changes.
Rejected: porting the SDK (the guard fails); `model.NewHTTPClient` (the spec forbids `model`). ADR-037's goal (off
the SDK) is kept; its means (`model/wire`, `model.NewHTTPClient`) are changed (owner question 5).

**D2. Placement** (owner question 1). Recommended: new package `graph/embedding/httpembedder` (`New(Config)`,
`*Embedder`), so `graph/embedding` keeps "no embedder of its own" (`doc.go:2`). Alternative: the pin's path and
names in `graph/embedding`.

**D3. The client's behaviour** (each a test in the client package, unit, fake provider shaped like semembed
`main.rs:18-60`):

| Behaviour | Proving test |
| --- | --- |
| `Generate(texts)` posts one `POST <BaseURL>/embeddings` `{"model","input":[...]}`, sends `Authorization: Bearer` only when an API key is set, and returns vectors in input order, placed by `index` | `TestRequestShape`, `TestVectorsPlacedByIndex` (fails on pin shape S1) |
| No texts → no request, empty result | `TestEmptyInputSendsNothing` |
| `GenerateQuery` prepends the query prefix to each text; `Generate` never does; no prefix → same as `Generate` | carried `TestHTTPEmbedder_GenerateQuery_PrependsPrefix`, `..._NoPrefixConfigured` |
| Refused, with no vectors returned: a non-2xx (error names the status and the provider's `error.message`); count ≠ texts; an `index` out of range or repeated; an empty vector; a width other than the learned one (error says "dimension", so the Worker files it `dimension_mismatch`); a non-empty reply `model` ≠ configured (owner question 2) | `TestRefusesUntrustedReply` (table; S2-S4 cases fail on pin shape) |
| `Dimensions()` is 0 until the first accepted reply, then fixed; safe to read while `Generate` runs | carried `TestHTTPEmbedder_Dimensions*` (3) |
| The caller's context ends the request; the error matches `ctx.Err()` | `TestContextEndsRequest` (the handler waits on its request's context) |
| `New` refuses an empty base URL or model, and a base URL that is not `http`/`https` | `TestNewRefusesConfig` |

Dropped pin config (no reader here, inventory §4): `Timeout`, `Logger`, and the three connection knobs. `Close`
returns nil (the shared default transport holds no per-embedder resource).

**D4. A hung embedder frees its Worker lane** (owner question 3; recommended a). The Worker calls `Generate` under
its run context (`worker.go:536`), so the pin client's 30 s default (`http_embedder.go:103-106`) moves to the Worker:
each embed runs under its context with a 30 s deadline. A call that ends at the deadline becomes `failed timeout`,
and the lane goes on. This is the hung-store shape (`graph/entitytext/body.go:17`). Proof:
`TestHungEmbedderFreesWorkerLane` (testing/synctest, with a fake embedder that blocks until its context ends).

**D5. The real-provider run** (owner question 4). `TestProviderContract` (client package): with
`SEMENGINE_EMBEDDING_URL` unset it runs against the fake, so `task verify` runs it. With it set (plus
`SEMENGINE_EMBEDDING_MODEL`, `SEMENGINE_EMBEDDING_QUERY_PREFIX`), it embeds two documents and a query against that
provider. It checks count, equal widths equal to `Dimensions()`, and that the prefixed query differs from the raw
one. It logs the served model and width, and it fails when the provider cannot be reached (#11: unavailable
evidence, never a skip). `task test:provider` runs it once with `-v`, outside `verify`. The output for semembed
7ceb5281 with `Snowflake/snowflake-arctic-embed-s` (384) goes in `notes/2.3.md`. #666's scorecard can grow from
this test.

**D6. searchGraph's tier 1 fallback, #563** (PR B; reply shape is owner question 6). With `embedding_enabled`
false, unchanged: as globalSearch, no semantic request. With true: globalSearch first. If its text has matches → its
reply, no `fallback`. If it has none → ask `graph.embedding.query.search` with the question and the request's
limit. On success → the global reply plus `fallback: {reason: "no_matches", semantic: <the embedding reply's data,
as its bytes came>}`. On refusal → the global reply plus `fallback: {reason: "no_matches", code, message}` (the
error is named, ADR-084). If globalSearch is refused `index_not_ready` → the same fallback with reason
`index_not_ready` and an empty text; if that also fails → the `index_not_ready` refusal. Any other refusal → returned,
no fallback. Proofs: `TestSearchGraphFallsBackWhenNoMatches`, `TestSearchGraphNamesAFailedFallback`,
`TestSearchGraphIndexNotReadyFallback`, `TestSearchGraphTierZeroAsksNoEmbedding` (graph-query unit and integration).

**D7. Losing the provider keeps word search** (#489's owed test; PR B, integration, graph-query package, in
`verify`). The stack is graph-ingest, graph-index, graph-lexical, graph-embedding registered with the client against
a fake provider, and graph-query with `embedding_enabled: true` (as `embedding_integration_test.go:46` composes it).
Entity A is written, and both indexes are seen to hold it. The fake provider is closed, then entity B is written.
Then: globalSearch for B's word answers B; `semantic` for it is refused, never answered empty; searchGraph for B's
word answers B with no `fallback`; searchGraph for a word nothing holds answers empty with `fallback.code` set;
graph-embedding's GRAPH_STATUS record reads `degraded` with `failed_reasons` `{"connection_refused":1}`. Each wait
reads the event it waits on. Proof: `TestLosingTheProviderKeepsWordSearch`.

**D8. Two pull requests, A then B** (chosen). A unblocks #666 (owner: client first, then the scorecard). B needs A's
client. Rejected: one PR (graph/embedding and graph-query reviewed together; #666 waits on #563).

## Risks / Trade-offs

- The query prefix is something the product has to know, and nothing checks it (semembed `main.rs:246`); only the
  scorecard catches it.
- The model check makes a TEI user configure the id TEI echoes.
- After an outage, failed entities wait for a new revision or a restart
  (`processor/graph-embedding/README.md:145-146`).
- `classifyEmbedErr` still classifies by message text (`worker.go:746-758`); the client's errors are shaped to land
  in the right class.

## Migration Plan

SemSource: in its `RegisterWithEmbedder` constructor, build the client from `model.ResolveEndpointWithConfig(reg,
model.CapabilityEmbedding)` (URL, Model, APIKey, QueryPrefix). Read `fallback` in place of `strategy` /
`degraded_reason == "global_search_empty_semantic_fallback"` (`disclosure.go:31`). Both notes go in MIGRATION.md.

## Open Questions

1. Placement (D2).
2. Model check (D3).
3. Hang bound (D4): a Worker bound; a client bound; or none, filed.
4. Real-provider run (D5).
5. ADR-037's changed means (D1).
6. Fallback reply shape (D6).
