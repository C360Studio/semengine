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
`SemanticRequest{query, limit, scope, min_revision}` and `SimilarRequest{entity_id, limit, min_revision}`, each
embedding `MinRevisionField` (D4). Replies are `graph.QueryResponse` with `producer` `graph-embedding`,
`indexed_revision` 0 (D4), `data` `SemanticData{results, embedder_type: "bm25", unreadable_records, failed_records}` or
`SimilarData{results, unreadable_records, failed_records}`; `results` is `[]`, never `null`. The pin's echoes (`query`,
`entity_id`) and `duration` are not carried; the envelope's `timestamp` replaces `duration`. No fixed handler timeout:
the request's context bounds the work (pin G6, `query.go:106,183`; ADR-112). *Proof:* `TestRequestsDecodeStrictly`
(unknown key, trailing data), `TestSearchAnswersEnveloped` (empty view: `results` `[]`), `TestVerbsServeTableSubjects`.

## D2. The vector cache on `pkg/graphview` (#477 ruling, ADR-081)

`VectorCache.Run(ctx)` builds a new `graphview.View[[]float32]` over `EMBEDDING_INDEX` each Run (a view runs once,
`pkg/graphview/view.go:216`) with a decode that keeps a generated record with a vector and maps any other record to
absent; a value that does not decode poisons its key. `FindSimilar(excludeID, query, keep, limit)` returns the ranked
hits and the number of poisoned keys, read under one hold (`View.ListCounted`, `view.go:444`), or an error matching
`graphview.ErrNotReady` before the snapshot, after a lost watch, and after Run returned: there the view answers
`ErrViewStopped`, which does not wrap `ErrNotReady` (`pkg/graphview/errors.go:25`, `view.go:576-584`), so the cache maps
it. A poisoned key is not scored; a valid write heals it. This replaces today's rule that one undecodable record
withdraws the cache for the rest of the run (`graph/embedding/vector_cache.go:94-96`), and follows the text tier's
accepted shape: records are independent, the answer reads the readable ones and says how many it could not (graph-query
text design D5-D6, ruling 3). The count is all unreadable records, not only those in scope. Kept from the tree: scoring
only what `keep` accepts, `ScoredEntityLess` before the limit, a non-positive limit returning none. The Worker keeps its
own raw watch (ADR-081 "stay raw" row). Cost not measured: `ListCounted` copies and sorts every key while holding the
view's lock (`view.go:445-458`), which holds back the view's writes, and graph-clustering's port will send one `similar`
per entity per refresh; task 2.2 posts a benchmark at 50,000 records with both the call time and the lock hold time, no
gate. *Proof:* `TestVectorCacheNotAuthoritative` (before the snapshot, after a loss, after Run returned),
`TestVectorCachePoisonCountedAndHeals`, `TestFindSimilarOrderAndLimit`, `TestVectorCacheRunReportsLostWatch`, the
stop-in-bubble and backlog tests kept.

## D3. Ranking and entry checks

Both verbs: `limit` 0 means `graph.DefaultSimilarityLimit` (10); 1 to `graph.MaxSimilarityLimit` (100) is used as sent;
a negative or larger value is refused `invalid_request` naming `limit` and the bound. The pin clamped silently
(`query.go:121-127`, `:203-209`), a declared-degrade breach; graph-query's `max_nodes` is the precedent
(`processor/graph-query/pathsearch.go:140-143`). A score of 0 is dropped. At the pin a stopword-only query embeds to a
zero vector (`graph/embedding/bm25_embedder.go`, empty token list) that passes the length check (`query.go:222`) and
scores every entity 0 (`pin:graph/embedding/vector.go:36-38`), so up to `limit` unrelated entities came back (read, not
probed). A candidate sharing no term can still score above 0 when two terms hash to one dimension (`bm25_embedder.go`,
`hashTerm`, FNV modulo the width); that stays. Ties at the limit resolve by `ScoredEntityLess` after the full match set
is sorted.

`search`: `query` blank after trimming is `invalid_request`; each non-empty `scope` element passes
`types.ValidateEntityIDPrefix` before the query is embedded (pin `entity_id_scope_contract_test.go:32`, unit); `""` in
`scope` matches all (pin `:60`); scope filters in the cache before scoring (ADR-071). The query is embedded by the
Worker's own `BM25Embedder` through `GenerateQuery`, which reads the corpus statistics without changing them
(`bm25_embedder.go:109-126`); one instance serves both, as at the pin. `similar`: `entity_id` passes
`types.ValidateEntityID` (`invalid_request`); the source vector is read with `Storage.GetEmbedding`, as at the pin.
Absent, not generated, or an empty vector is class invalid, code `embedding_unavailable`, the per-entity miss
graph-clustering reads as "no neighbours" (pin `query.go:146-157`;
`pin:processor/graph-clustering/similarity.go:296-307`; pin test `query_failure_honesty_test.go:23`, unit). An
undecodable source record stays class invalid with no code, as at the pin (`pin:graph/embedding/storage.go:520-521`), so
graph-clustering counts it toward its abort instead of reading an empty neighbour set. The source is never its own hit.
*Proof:* `TestSearchLimitEdges` (0, 1, 100, 101, -1), `TestSearchQueryEdges` (blank, stopword-only answers `[]`),
`TestSearchScopeEdges` (bad prefix refused with the embedder's statistics unchanged; `[""]`; a scope matching nothing),
`TestSimilarEdges` (unknown, pending, malformed, undecodable uncoded, self), `TestTopKAtTie` and `TestScopedSearch`
(ported from the pin's `similar_determinism_integration_test.go:31`, `scope_integration_test.go:25`, integration, not
run at the pin).

## D4. What `indexed_revision` means here

The reply reads the view, whose position is an `EMBEDDING_INDEX` revision, not an `ENTITY_STATES` one, and the Worker's
write lands in the bucket before the view applies it. No revision can be claimed without a fence the pin never had, so
the verbs report 0, which claims nothing (`graph/query_response.go:13-15`). The graph-query spec says every graph-query
verb accepts `min_revision` and passes it unchanged to the derived-index read, and that a success for `min_revision` M
carries `indexed_revision` at least M (`openspec/specs/graph-query/spec.md:121-125`); `semantic` and `similar` forward
the bytes (`processor/graph-query/query.go:32-33`). So the requests accept `min_revision`: 0 is answered; above 0 is
refused class invalid, `invalid_request`, saying the embedding verbs claim no revision. That is a refusal, never a wrong
success, and it is not retried, where a transient `revision_not_reached` would be retried for ever. No graph-query spec
change. The pin offered neither (`query.go:51-96`). Rejected: refusing the key by name (breaks the graph-query
requirement); the follower's floor plus an `EMBEDDING_INDEX` fence (new mechanism with no caller). *Proof:*
`TestMinRevisionAboveZeroRefused`, `TestSemanticForwardsMinRevision` (through graph-query's forward).

## D5. Withheld calls (ADR-109 D1) and the lost view

A latched reset is fatal `graph_state_reset_required`; a lost feed or an incomplete first build is transient
`index_not_ready`; a cache not caught up or lost is transient `index_not_ready`. Which failures withhold is owner
question 3. `follower.QueryReady` withholds on any failed-map entry (`graph/follower/follower.go:388-390`). The trigger
at tier 0 is `content_error` (`graph/embedding/storage.go:493`, text extraction failed), a Worker failure the repair
lane does not retry (`processor/graph-embedding/failed.go:44-50`): one document whose text cannot be extracted would
refuse both verbs until that entity is written again. The pin did not withhold on failures
(`pin:processor/graph-embedding/component.go:1723-1737`), and a pending entity is left out of an answer today without
withholding. **F1** keep `QueryReady` as is: no new code; one bad document stops all search. **F2 (recommended)**
withhold only on hop 1's three failures (`delete_failed`, `pending_write_failed`, `entity_read_failed`), the ones that
can leave a stale vector in the index and so a wrong hit; a Worker failure leaves the entity with no vector, so the
answer reads the rest and reports `failed_records`, the count of such entities, as D2 reports unreadable records. Cost:
one optional field on `follower.Config`, `Withhold func() int64`, the count of failures that withhold queries, nil
meaning `Failed`'s count (graph-index unchanged), plus an `entity-state-follower` spec delta. `Health` and the envelope
keep counting every failure, as PR A shipped, so under F2 a Worker failure still marks the published status degraded; if
fusion gates on that status (ADR-082; PR A inventory § 7), one bad document would stop SemSource's fusion path again,
which PR C's inventory decides. A `VectorCache.Run` that returns while the run context is live latches `vector cache
watch lost; restart the process` in `Health`, as the Worker's loss does (PR A D2); nothing reopens it, since the bucket
is the component's own. Before `Start` and after `Stop` returns there is no responder. *Proof:*
`TestQueriesWithheldUntilReady` (before the first build, after a reset, with a hop-1 failure),
`TestWorkerFailureAnswersAndCounts` (F2: a `content_error` entity, `failed_records` 1, other hits served),
`TestGraphIndexWithholdUnchanged`, `TestHealthNamesLostCacheWatch` (integration), `TestNoResponderAfterStop`, lifecycle
suite, `-race`.

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
