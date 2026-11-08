# Inventory: change 5 PR B, `processor/graph-embedding` step 2

base: 3835c0a1cabb11da2830ae2ccaebda9364b4768d (claim commit on origin/main d15e6c4646a3c26ff1af907b5631f4c01def07ab)
pin: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128 (`/tmp/c5pin/src`, local only, the copy PR A's probes ran on)

PR A's inventory (`openspec/changes/archive/2026-11-08-setup-04a-05-first-consumer-graph-embedding/inventory.md`)
stands for the package, its probes (§ 0), ADR table (§ 7) and sweep (§ 8). This file adds only what the read path
needs. Labels: **measured** (command run here), **read** (code read). Repositories read: this tree, the pin copy,
semsource at `0034267` (symbol level, grep), semteams, semconnect, semboids (grep, zero `graph/embedding` imports).

## 0. Probes

No new pin run. The query-path unit tests (`query_failure_honesty_test.go`, `scope_test.go`,
`entity_id_scope_contract_test.go`, `reader_capabilities_test.go`) were in the owner's package-wide runs (rc=0,
`/tmp/c5probe/race.txt`, package-level `ok` only). `scope_integration_test.go` and
`similar_determinism_integration_test.go` are `//go:build integration` and did not run; running them needs Docker
and running downloaded code, not done here. Statements below marked "read" were not probed. No new statement about
how a library times, orders or fails: the cache inherits `pkg/graphview`'s probed behaviour (`view.go:216-245`).

## 1. Sizes (measured, `wc -l`)

| Where | Prod | Test |
| --- | --- | --- |
| pin `processor/graph-embedding/query.go`; its tests (`query_failure_honesty` 297, `scope` 64, `entity_id_scope_contract` 64, `scope_integration` 109, `similar_determinism_integration` 112, `reader_capabilities` 30) | 371 | 676 |
| tree `graph/embedding/vector_cache.go`; `vector_cache_test.go`, `vector_cache_integration_test.go` | 171 | 614 |

## 2. Claimed gaps and spellings

- No request type for the embedding verbs in the tree: `git grep -n 'SearchRequest\|SimilarRequest\|SemanticRequest'
  -- 'graph/*.go'`: 0 hits. Reply types exist: `graph/query_search_types.go:21` — `type SemanticData struct {`, `:27`
  `type SimilarData struct {`; rows `graph/query_verbs.go:87-90`. `:8-9` of the same file leave "What
  IndexedRevision means on the embedding verbs" to these responders.
- The pin's reply shapes: `pin:query.go:58` `SimilarResponse{EntityID, Similar, Duration}`, `:85`
  `SearchResponse{Query, Results, Duration, EmbedderType}`, `:65`/`:93` two hit types; the tree has one,
  `graph.SimilarityHit` (`graph/query_search_types.go:14`).
- Ranking spelled twice at the pin: warm cache `pin:graph/embedding/storage.go` (`FindSimilarFromCache`, `:893-930`)
  and cold scan `pin:query.go:289-356`; ADR-071's BLOCKING defect was a drift between them
  (`docs/adr/071-fusion-domain-scoped-nl-retrieval.md:111-118`). Tree: one, `graph/embedding/vector_cache.go:150`.
- Readiness check spelled at the pin as `ensureBootstrapReady` (`pin:component.go:1723-1737`: reset, watch
  unavailable, bootstrap); tree: `graph/follower/follower.go:377` — `func (f *Follower) QueryReady(ctx
  context.Context) error {`, which also withholds on any failed-map entry (`:388-390`).
- Per-entity miss code: `graph/mutation_responses.go:102` — `ErrorCodeEmbeddingUnavailable = "embedding_unavailable"`.
- Query embedding: `graph/embedding/bm25_embedder.go:124` — `func (b *BM25Embedder) GenerateQuery(` (read-only over
  corpus statistics, `:113-118`); the component builds the embedder inline (`processor/graph-embedding/component.go`,
  `embedding.NewWorker(c.storage, embedding.NewBM25Embedder(...)`), so the handler has no handle on it today.
- Limit conventions in the tree: `graph/query_path_types.go:8-11` (`MaxPathSearchNodes`, `DefaultPathSearchNodes`),
  `processor/graph-query/pathsearch.go:140-143` (0 means default); pin clamps (`query.go:121-127`, `:203-209`).

## 3. Adjacent claims

- Open PRs (`gh pr list --state open`, 2026-11-08): #558 only (this claim, 0 files). #557 (composed smoke test) is an
  issue with no PR yet; it will touch graph-embedding's integration surface: whichever merges second merges
  `origin/main`.
- Specs: `embedding-index` "The vector cache answers only while authoritative" (`embedding-index/spec.md:61-97`)
  changes; `graph-query` "searchGraph answers as global search" (`spec.md:846-849`, "SHALL make no semantic
  request") and the forward requirement (`:220-226`) stay; `graph-transport-boundary` `:38` already lists the rows.
- `graph-query` spec `:121-125`: "Every verb SHALL accept `min_revision`. On an enveloped verb it SHALL reach every
  derived-index read unchanged", and a success for M carries `indexed_revision` at least M; `semantic` and `similar`
  forward the bytes unchanged (`processor/graph-query/query.go:32-33`). This binds the embedding requests (design D4).
- Failure withholding: `graph/follower/follower.go:388-390` withholds on any `Failed` count; tier-0 trigger
  `content_error` (`graph/embedding/storage.go:493`), not re-driven by the repair lane
  (`processor/graph-embedding/failed.go:44-50`); the pin withheld on none
  (`pin:processor/graph-embedding/component.go:1723-1737`).
- Accepted design commitment: 03B D5, `archive/2026-10-01-setup-03b-contract-boundary/design.md:358` — searchGraph
  "keep at tier 0/1 under its degraded contract (exact or semantic fallback ...)"; change 4 text design D7 deferred
  the fallback to change 5 (`archive/2026-11-07-...-graph-query-text/design.md:136-146`).
- Ledger: `graph/embedding` item (7) (`docs/admission-ledger.yaml:3784-3787`) describes the cache's authority rule
  this PR changes; `processor/graph-embedding` known_risks lists "the query verbs and the vector cache (PR B)".
- ADR-071 port note (`docs/adr/071-...md:3-15`) says the responder and `Scope` are not present.
- Consumers: semsource calls `graph.query.searchGraph` (`semsource:processor/mcp-gateway/query_tools.go:107`) and
  reads the fallback's `degraded_reason` (`disclosure.go:29-32`); no semsource call to `graph.embedding.query.*`
  (grep, 0). Pin graph-clustering, ported right after change 5 (#9 comment 6724212898), calls `similar` with
  `{entity_id, limit}` and decodes `similar` (`pin:processor/graph-clustering/similarity.go:21`, `:49-66`), reading
  `embedding_unavailable` by code (`:300-307`).

## 4. Who it is for (new surface)

`graph.SemanticRequest`, `graph.SimilarRequest`, the two limit constants and `UnreadableRecords`: decoded and set by
graph-embedding's handlers (plumbing); sent by `pkg/fusion/fusionnats` (PR D, SemSource's fusion path) and the
graph-clustering port. `VectorCache.FindSimilar`'s new results: its one production caller is the handler.

## 5. Open issues naming the carried surface

Queries (`gh issue list --state open --search <term>`): `graph-embedding`, `embedding`, `VectorCache`, `graphview`,
`searchGraph`, `similar`, `bm25`, `query.search`, `ADR-071`, `scope`. Kept, with disposition:

| Issue | Disposition |
| --- | --- |
| #477 graphview adoption | advanced, not closed: the vector cache site (owner ruling); stays for the message logger |
| #111 silent degrades, item 3's last site | stays; if S0, its last pin site is not carried (comment); if S1, the follow-up PR |
| #109, #196, #195 | stays: graph-embedding adopted in PR A; new tests use `natsfixture.Open`; other packages remain |
| #489, #32, #11, #155 | stays: tier 1 and the model package; tier 0 has no absent embedder |
| #551 offloaded fetch deadline | stays: the Worker's fetch, not the read path |
| #557 smoke test | stays: its own PR (merge order above) |
| #10 BM25 restart-stable ranking | stays (04B): query statistics are process-local and restart from documents embedded since start |
| #110 fusion envelope copy | stays (PR C) |
| #141 `UpdateWithRetryRead` adoption | stays: names `graph/embedding/storage.go:308,367,438`; those loops are not touched by this PR, only `ListGeneratedEntityIDs` goes; its graph/embedding part is done (2026-10-28 comment) |

Names the word, not the package: #9, #134, #15, #16, #24, #26, #27, #68, #81, #84, #284, #294, #484, #487 (tier-2 LLM
answer in graph-query, a `searchGraph` hit), #506, #554.

## 6. Shape sweep of the read path (pin copy)

| # | Shape | Pin | Disposition |
| --- | --- | --- | --- |
| Q1 | fixed timeout | `ctx, cancel := context.WithTimeout(requestCtx, 30*time.Second)` (`query.go:106`, `:183`) | required (PR A G6; ADR-112) |
| Q2 | inline subjects | `query.go:24`, `:31` | required (verify rule) |
| Q3 | second spelling: two ranking paths | `query.go:279-356` | optional; dropped (design D6) with its only reader |
| Q4 | silent clamp | `if limit > 100 { limit = 100 }` (`query.go:125-127`, `:207-209`) | required (fail closed, declared degrade) |
| Q5 | copied readiness check | `ensureBootstrapReady` (`component.go:1723`) | required (ADR-108 D2, PR A G2) |
| Q6 | back door | `subscribe := c.subscribeForRequests` (`query.go:19-22`) | required (#186) |
| Q7 | unreachable branches | nil storage `:130`, `:270`; nil embedder `:212`; empty vector `:222` | optional; dropped |
| Q8 | second check | `validateEntityIDScope` at `:198` and `:267` | optional; once at entry |
| Q9 | second spelling of the wire hit | `SimilarEntity`, `SearchResult` (`:65`, `:93`) | required (verb table types) |
| Q10 | zero-score hits | zero query vector passes `:222`; every candidate scores 0 | owner question 4 |

Classification by error text: none (`grep -n 'strings\.' pin:query.go`: 0).

## 7. Problem shapes

Serving projection over one KV watch: `processor/graph-query/community.go:82-94` on `graphview.View`. Adopted for the
view; not adopted: its reopen loop (`:38-62`), since `EMBEDDING_INDEX` is created by this component at Start and a
lost watch is the Worker's case (PR A D2). Readiness rides the reply: `follower.QueryReady` (graph-index,
`processor/graph-index/query.go`). Request/reply from the verb table: `processor/graph-query/query.go:99-120`.
Adoption sweep: no new primitive. Collision table (serving state over `EMBEDDING_INDEX`): owners `VectorCache`
(converted) and the Worker's watch (stays raw, ADR-081 table `docs/adr/081-graph-view-subscription.md:235-237`);
writers: graph-embedding only (`graph/kvcatalog/kvcatalog.go:186`); readers: the handlers; recovery: a new view per
Run; no status key or catalog entry added.

## 8. Adopter seam

Who: a fusion lens author (PR D's client), graph-clustering's port, SemSource's `graph_search` tool.

1. Must know: replies are `data.results` in an envelope; `limit` 0 to 100; `min_revision` above 0 is refused; a
   transient `index_not_ready` is retried, `embedding_unavailable` is not.
2. Do nothing: an old decoder reads empty `similar` (silent) — only graph-clustering's pin client, which is ported,
   not adopted; a `limit` of 500 is refused (loud); an unscoped caller is unchanged.
3. Find out: typed runtime error for limit, keys and readiness; `MIGRATION.md` for the reply shape (doc level, the
   finding is the renamed field; the port of graph-clustering migrates its decoder).
4. Should know: nothing beyond the limit range.
