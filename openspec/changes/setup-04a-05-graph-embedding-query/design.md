# Design: `processor/graph-embedding` step 2, the query verbs

Base `d15e6c4` (PR A merged); pin `8b99efe9`. Inventory: PR A's (`archive/2026-11-08-...-graph-embedding/inventory.md`)
plus this change's `inventory.md`. **Goal.** `graph.embedding.query.search` and `.similar` answer from an in-memory
view of `EMBEDDING_INDEX`, in the tree's envelope, and refuse every case they cannot answer truthfully.
**Non-goals:** graph-query changes (D7), fusion (PRs C, D), tier 1 (#489), BM25 restart-stable statistics (#10).
Pin paths are `pin:processor/graph-embedding/` unless named. "Unit" pin tests ran in the owner's probes (PR A
inventory § 0, rc=0); the two integration-tagged query tests did not run.

## D1. Wire shape

Subjects come only from `graph.QueryVerbs()` rows with responder `graph-embedding` (verify rule; pin G7,
`query.go:24,31`), subscribed after the Worker and the cache are launched, drained first in `cleanup`. Requests, added
to `graph/query_search_types.go` and decoded strictly (unknown key refused naming it, data after the object refused):
`SemanticRequest{query, limit, scope}` and `SimilarRequest{entity_id, limit}`. Replies are
`graph.QueryResponse` with `producer` `graph-embedding`, `indexed_revision` 0 (D4), `data` `SemanticData{results,
embedder_type: "bm25", unreadable_records}` or `SimilarData{results, unreadable_records}`; `results` is `[]`, never
`null`. The pin's echoes (`query`, `entity_id`) and `duration` are not carried; the envelope's `timestamp` replaces
`duration`. No fixed handler timeout: the request's context bounds the work (pin G6, `query.go:106,183`; ADR-112).
*Proof:* `TestRequestsDecodeStrictly` (unknown key, `min_revision`, trailing data), `TestSearchAnswersEnveloped`
(empty view: `results` `[]`), `TestVerbsServeTableSubjects`.

## D2. The vector cache on `pkg/graphview` (#477 ruling, ADR-081)

`VectorCache.Run(ctx)` builds a new `graphview.View[[]float32]` over `EMBEDDING_INDEX` each Run (a view runs once,
`pkg/graphview/view.go:216`) with a decode that keeps a generated record with a vector and maps any other record to
absent; a value that does not decode poisons its key. `FindSimilar(excludeID, query, keep, limit)` returns the ranked
hits and the number of poisoned keys, read under one hold (`View.ListCounted`, `view.go:444`), or an error matching
`graphview.ErrNotReady` before the snapshot, after a lost watch and after Run returned. A poisoned key is not scored; a
valid write heals it. This replaces today's rule that one undecodable record withdraws the cache for the rest of the run
(`graph/embedding/vector_cache.go:94-96`), and follows the text tier's accepted shape: records are independent, the
answer reads the readable ones and says how many it could not (graph-query text design D5-D6, ruling 3). The count is
all unreadable records, not only those in scope. Kept from the tree: scoring only what `keep` accepts,
`ScoredEntityLess` before the limit, a non-positive limit returning none. The Worker keeps its own raw watch (ADR-081
"stay raw" row). Cost not measured: `ListCounted` sorts keys per call where the map scan did not; task 2.2 reports a
benchmark at 50,000 records on the PR, no gate. *Proof:* `TestVectorCacheNotAuthoritative`,
`TestVectorCachePoisonCountedAndHeals`, `TestFindSimilarOrderAndLimit`, `TestVectorCacheRunReportsLostWatch`, the
stop-in-bubble and backlog tests kept.

## D3. Ranking and entry checks

Both verbs: `limit` 0 means `graph.DefaultSimilarityLimit` (10); 1 to `graph.MaxSimilarityLimit` (100) is used as
sent; a negative or larger value is refused `invalid_request` naming `limit` and the bound. The pin clamped silently
(`query.go:121-127`, `:203-209`), a declared-degrade breach; graph-query's `max_nodes` is the precedent
(`processor/graph-query/pathsearch.go:140-143`). A hit scores above 0: a candidate sharing no term with the query
is not a hit. At the pin a stopword-only query embeds to a zero vector (`graph/embedding/bm25_embedder.go`, empty
token list) that passes the length check (`query.go:222`) and scores every entity 0 (`graph/embedding/vector.go:33-35`),
so up to `limit` unrelated entities came back (read, not probed). Ties at the limit resolve by `ScoredEntityLess`
after the full match set is sorted.

`search`: `query` blank after trimming is `invalid_request`; each non-empty `scope` element passes
`types.ValidateEntityIDPrefix` before the query is embedded (pin `entity_id_scope_contract_test.go:32`, unit); `""`
in `scope` matches all (pin `:60`); scope filters in the cache before scoring (ADR-071). The query is embedded by the
Worker's own `BM25Embedder` through `GenerateQuery`, which reads the corpus statistics without changing them
(`bm25_embedder.go:109-126`); one instance serves both, as at the pin. `similar`: `entity_id` passes
`types.ValidateEntityID` (`invalid_request`); the source vector is read with `Storage.GetEmbedding`, as at the pin;
absent, not generated, empty vector or undecodable is `invalid` `embedding_unavailable`, a per-entity miss
graph-clustering reads by code (pin `query.go:146-157`; `pin:processor/graph-clustering/similarity.go:300-307`;
pin test `query_failure_honesty_test.go:23`, unit). The source is never its own hit. *Proof:* `TestSearchLimitEdges`
(0, 1, 100, 101, -1), `TestSearchQueryEdges` (blank, stopword-only answers `[]`), `TestSearchScopeEdges` (bad prefix
refused with the embedder's statistics unchanged; `[""]`; a scope matching nothing), `TestSimilarEdges` (unknown,
pending, malformed, undecodable, self), `TestTopKAtTie` and `TestScopedSearch` (ported from the pin's
`similar_determinism_integration_test.go:31`, `scope_integration_test.go:25`, integration, not run at the pin).

## D4. What `indexed_revision` means here

The reply reads the view, whose position is an `EMBEDDING_INDEX` revision, not an `ENTITY_STATES` one, and the
Worker's write lands in the bucket before the view applies it. No revision can be claimed without a fence the pin
never had, so the verbs report 0, which claims nothing (`graph/query_response.go:13-15`), and the request types do not
embed `MinRevisionField`, so `min_revision` is refused by name. The pin offered neither (`query.go:51-96`). Rejected:
the follower's floor plus an `EMBEDDING_INDEX` fence (new mechanism with no caller).
*Proof:* `TestRequestsDecodeStrictly`.

## D5. Withheld calls (ADR-109 D1) and the lost view

Each handler asks `follower.QueryReady` first (`graph/follower/follower.go:377-400`): a latched reset is fatal
`graph_state_reset_required`; a lost feed, any entry in the failed map, or an incomplete first build is transient
`index_not_ready`. Then a cache not caught up or lost is transient `index_not_ready`. The pin withheld on reset, lost
watch and bootstrap only (`component.go:1723-1737`); counting failures matches graph-index and the `Health` PR A
shipped (`follower.go:360-374`), and costs: one permanently failed entity withholds both verbs until it converges
(owner question 3). A `VectorCache.Run` that returns while the run context is live latches
`vector cache watch lost; restart the process` in `Health`, as the Worker's loss does (PR A D2); nothing reopens it,
since the bucket is the component's own. Before `Start` and after `Stop` returns there is no responder. *Proof:*
`TestQueriesWithheldUntilReady` (before the first build, after a reset, with a failure), `TestHealthNamesLostCacheWatch`
(integration), `TestNoResponderAfterStop`, lifecycle suite, `-race`.

## D6. Not carried

The bucket-scan fallback (`query.go:289-356`) and `Storage.ListGeneratedEntityIDs`, its only pin reader: D5 withholds
the window the scan covered, and two candidate paths were where ADR-071's warm-cache defect hid. The
`subscribeForRequests` test hook (`query.go:19-22`, a back door, #186). The nil-storage and nil-embedder branches
(`:130`, `:212`, `:270`) and the empty-vector branch (`:222`), unreachable at tier 0. The second scope check (`:267`).

## D7. `searchGraph` (owner question 1)

03B D5 keeps `searchGraph` "under its degraded contract (exact or semantic fallback)"
(`archive/2026-10-01-setup-03b-contract-boundary/design.md:358`); change 4 left the fallback out until change 5 (text
design D7), and the graph-query spec says `searchGraph` "SHALL make no semantic request". SemSource's `graph_search`
reads the fallback's rung (`semsource:processor/mcp-gateway/disclosure.go:29-32`). **S0** keep `searchGraph` as global
search; semantic composition is fusion's (PRs C, D); drops a 03B commitment. **S1 (recommended)** a graph-query PR right
after this one adds a labelled semantic strategy to `searchGraph` when global search answers empty or refuses
`index_not_ready`, returning the first refusal when the semantic call also fails (pin `searchgraph.go:68-115` without
S14's swallowed error); it changes the graph-query requirement above, in its own OpenSpec change. **S2** do it here: two
packages in one PR (#159). S1 keeps 03B's commitment and SemSource's rung at small cost; S0 is simpler but leaves
`graph_search` refusing at tier 0 until graph-clustering writes communities.

## D8. ADRs, issues, guidance, order

Applied: ADR-071 (scope before ranking; its compatibility note, `docs/adr/071-...md:105-107`, is changed by strict
decoding, under the owner's 2026-11-02 ruling on PR #504 that a port may break consumers who then migrate), ADR-081
(convert the cache), ADR-109 D1 (D5), ADR-112 (no fixed timeout), ADR-113 (the view runs under the run context). Pin
ADR-086 (not ported) keeps `similar`'s not-ready and `embedding_unavailable` codes load-bearing for graph-clustering:
kept. ADR-054's "embedding not ready" refusal (not ported) is the per-entity miss of D3. Issues: `inventory.md` § 5.
Guidance: SemStreams' "NATS RPC" contract section and the `query-pattern` skill apply. Order: after PR A (merged); #557
and this PR, second merges `origin/main`; PR D after this and C.
