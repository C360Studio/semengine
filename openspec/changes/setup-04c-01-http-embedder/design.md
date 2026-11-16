# Design: setup-04c-01-http-embedder

## Context

Premises are pinned in `inventory.md` (INVENTORY PASS). Pin lines: 548 (220 production, 328 test); estimated here:
PR A ~150 production + ~300 test, PR B ~120 production + ~250 test. #563 moves to milestone v0.1.0-alpha.6 with #489,
since PR B closes it.

## Goals / Non-Goals

Goal: the smallest complete tier 1 client, the owed degrade test, and #563. Non-goals: as the proposal says.

## Decisions

**D1. Plain net/http; no SDK, no `model`** (required). The OpenAI SDK fails `TestTreeImportsNoProviderPlumbing`
(`internal/harness/contract/embeddingprovider_test.go:207`, owner ruling 2 on PR #582), and `model` is forbidden by
embedding-index (`spec.md:261-262`). One request shape, about 10 lines of wire types. Neither guard changes.
For the owner's information: pin ADR-037 (Proposed) moved embeddings off the SDK onto `model/wire` over
`model.NewHTTPClient`. Its goal (off the SDK) is kept. Its means are not ported: `model/wire` is `defer-exclude`, and
`model` is forbidden here.

**D2. Placement** (chosen): new package `graph/embedding/httpembedder` (`New(Config)`, `*Embedder`), so
`graph/embedding` keeps "no embedder of its own" (`doc.go:2`, `README.md:114`). Rejected: the pin's path, which would
make that sentence false while the guard still passed.

**D3. The client's behaviour** (each a unit test in the client package, against a fake provider shaped like semembed
`main.rs:18-60`; this is all `task verify` runs):

| Behaviour | Proving test |
| --- | --- |
| `Generate(texts)` posts one `POST <BaseURL>/embeddings` `{"model","input":[...]}`, sends `Authorization: Bearer` only when an API key is set, and returns vectors in input order, placed by `index` | `TestRequestShape`, `TestVectorsPlacedByIndex` (fails on pin shape S1) |
| No texts → no request, empty result | `TestEmptyInputSendsNothing` |
| `GenerateQuery` prepends the query prefix to each text; `Generate` never does; no prefix → same as `Generate` | carried `TestHTTPEmbedder_GenerateQuery_PrependsPrefix`, `..._NoPrefixConfigured` |
| Refused, with no vectors returned: a non-2xx (error names the status and the provider's `error.message`); count ≠ texts; an `index` out of range or repeated; an empty vector; a width other than the learned one (error says "dimension", so the Worker files it `dimension_mismatch`); a non-empty reply `model` ≠ configured (owner question 1) | `TestRefusesUntrustedReply` (table; S2-S4 cases fail on pin shape) |
| `Dimensions()` is 0 until the first accepted reply, then fixed; safe to read while `Generate` runs | carried `TestHTTPEmbedder_Dimensions*` (3) |
| The caller's context ends the request; the error matches `ctx.Err()` | `TestContextEndsRequest` (the handler waits on its request's context) |
| `New` refuses an empty base URL or model, and a base URL that is not `http`/`https` | `TestNewRefusesConfig` |

Dropped pin config (no reader here, inventory §4): `Timeout`, `Logger`, and the three connection knobs. `Close`
returns nil (the shared default transport holds no per-embedder resource).

**D4. A hung embedder frees its Worker lane** (chosen). The Worker calls `Generate` under its run context, which has no
deadline (`worker.go:536`). The query side is already bounded by natsclient's 30 s handler budget
(`natsclient/request.go:363`). So the pin client's 30 s default (`http_embedder.go:103-106`) moves to the Worker:
each embed runs under its context with a 30 s deadline. When that deadline has passed, the error is wrapped with it,
as `graph/entitytext/body.go:52-55` does. A product embedder whose own error does not wrap the deadline is still
filed `timeout`, and the lane goes on. Proof: `TestHungEmbedderFreesWorkerLane` (testing/synctest; one fake blocks
until its context ends and returns the context error, one returns a plain error at that moment). Rejected: a client
bound, which could not be tested without a test-only hook and would leave other product embedders unbounded.

**D5. Real-provider evidence outside verify.** Nothing in `task verify` reaches an outside service; D3's fake is the
only provider verify sees. PR A records one run against semembed 7ceb5281 (local Docker, `http://127.0.0.1:18081/v1`,
`Snowflake/snowflake-arctic-embed-s`, 384) as a throwaway probe program. Its source, command, image build, model and
output go in `notes/probes/semembed-7ceb5281.md`. Against `docs/setup-plan.md:142-146` it proves:

- request and reply semantics: count, `index` order, the served `model` equals the configured one;
- readiness: `Dimensions()` is 0 before the first reply and 384 after it;
- query versus document: the prefixed query vector differs from the raw one;
- context: a cancelled context, and a deadline shorter than one embed, end the call with `ctx.Err()`;
- recovery: with the container stopped, a call fails `connection refused`; after it restarts, the next call succeeds.

Paraphrase, mixed code/docs, and warm/cold workload qualification are #666's and #11's. #666's scorecard is the
first committed real-provider run (owner, 2026-11-16: client first, then the scorecard through it). Which lane runs
it from then on is owner question 2.

**D6. searchGraph's tier 1 fallback, #563** (PR B). Basis: ruling S1 (#563 body: "If the semantic call also fails,
return the first refusal ... never hide it") and the archived design
`openspec/changes/archive/2026-11-14-setup-04b-03-graph-query-lexical/design.md:78-81` ("answer it under `fallback`
with its reason, and return the first answer or refusal if that fails too").

With `embedding_enabled` false, nothing changes: searchGraph answers as globalSearch and makes no semantic request.
With it true, globalSearch runs first:

- **Text has matches.** Its reply, with no fallback.
- **Text has no matches.** Ask `graph.embedding.query.search` with the question and the request's limit.
  - On success: the global reply plus `semantic` (the embedding reply's data as its bytes came) and
    `fallback: {hint: "semantic", detail: "no_matches", honoured: true}`.
  - On refusal: the global reply (the first answer) plus `fallback: {hint: "semantic", detail: "no_matches",
    honoured: false, reason: <message>, code: <code>}`.
- **globalSearch refused `index_not_ready`.** The same fallback with detail `index_not_ready`, and `text` left out
  (null): the word index was not read, so an empty `text` would claim "no word matches". If the fallback also fails,
  the `index_not_ready` refusal is returned (the first refusal).
- **Any other refusal.** Returned, with no fallback.

Difference from S1 and the archived D7, stated rather than asked: on the no-match path there is no first refusal, so
the first answer (empty text) is returned and the fallback's refusal is named in `fallback` rather than dropped (the
pin's S14 defect).

The reply shape adopts what `GlobalSearchData` already has (`graph/query_global_search_types.go:17-40`). `Semantic
json.RawMessage` sits beside `Entity` (a second verb's reply as its bytes came), and `Fallback *GlobalSearchHint`
reuses the hint entry (`Hint`, `Detail`, `Honoured`, `Reason`, `Code`). It is a field of its own, not an entry in
`Hints`: `Hints` lists what the question asked for, while the fallback is graph-query's own step. `Text` is no longer
always there; its doc comment says when it is left out.

Proofs: `TestSearchGraphTierZeroAsksNoEmbedding`, `TestSearchGraphFallsBackWhenNoMatches`,
`TestSearchGraphNamesAFailedFallback`, `TestSearchGraphIndexNotReadyLeavesTextOut` (graph-query unit and integration).

**D6a. The fallback's time.** The fallback is one downstream request through graph-query's `ask`, so it waits at most
`query_timeout_ms` (default 15 s, `processor/graph-query/query.go:168`, `config.go:16`). It never waits past the
handler's context (natsclient's 30 s, `request.go:363`). A fallback that runs out of time is named like any refusal
(`code: internal`, from `ask`), and the word answer is still returned. Proof:
`TestSearchGraphFallbackTimeoutKeepsWordAnswer` (failing first; an embedding responder that never answers;
`query_timeout_ms` set low in the test's config).

**D7. Losing the provider keeps word search** (#489's owed test; PR B, integration, graph-query package, in
`verify`). The stack is graph-ingest, graph-index, graph-lexical, graph-embedding registered with the client against
a fake provider, and graph-query with `embedding_enabled: true` (as `embedding_integration_test.go:46` composes it).

1. Entity A is written, and both indexes are seen to hold it.
2. The fake provider is closed, then entity B is written.
3. Then:
   - globalSearch for B's word answers B;
   - `semantic` for it is refused, never answered empty;
   - searchGraph for B's word answers B with no `fallback`;
   - searchGraph for a word nothing holds answers an empty `text` with `fallback.honoured` false and `code` set;
   - graph-embedding's GRAPH_STATUS record reads `degraded` with `failed_reasons` `{"connection_refused":1}`.

Each wait reads the event it waits on. Proof: `TestLosingTheProviderKeepsWordSearch`.

**D8. Two pull requests, A then B** (chosen). A unblocks #666 (owner: client first, then the scorecard). B needs A's
client. Rejected: one PR (graph/embedding and graph-query reviewed together; #666 waits on #563). #670 edits the same
graph-query README; whichever lands second merges `origin/main`.

## Risks / Trade-offs

- The product has to know the query prefix, and nothing checks it (semembed `main.rs:246`); only the scorecard
  catches it (owner question 3).
- The model check makes a TEI user configure the id TEI echoes.
- After an outage, failed entities wait for a new revision or a restart
  (`processor/graph-embedding/README.md:145-146`).
- `classifyEmbedErr` still classifies by message text (`worker.go:746-758`); D4's wrap and the client's error text
  land in the right class.

## Migration Plan

SemSource: in its `RegisterWithEmbedder` constructor, build the client from `model.ResolveEndpointWithConfig(reg,
model.CapabilityEmbedding)` (URL, Model, APIKey, QueryPrefix). Read `fallback` and `semantic` instead of `strategy` /
`degraded_reason == "global_search_empty_semantic_fallback"` (`disclosure.go:31`), and treat a null `text` as "word
index not read". Both notes go in MIGRATION.md.

## Open Questions (owner only)

1. **Model check (D3).** Should the client refuse a reply naming a model other than the configured one? The plan
   says request model labels alone are insufficient, and semembed can echo a configured name while serving another
   (`docs/setup-plan.md:135-141`). Recommended yes; the cost is that TEI users configure the echoed id.
2. **Which lane runs the committed real-provider test (D5).** It runs outside verify; recommended: #666's scorecard
   as a Task target run before each tier 1 release, its output attached to the release evidence.
3. **Non-blocking: semembed and the query prefix.** Should semembed (a sister repository) take the query prefix, for
   example an `input_type`, so products stop having to know it?
