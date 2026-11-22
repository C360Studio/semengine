# Design: fix-706-fusion-nl-merged-search

## Context

Facts: `inventory.md` (INVENTORY PASS at 48c2e10d), cited by section.

- fusion asks graph-lexical alone (§ 1).
- The merge and its fallback live only in graph-query (`merge.go:32`, `textsearch.go:76-125`; § 2 A).
- "The composition runs embeddings" is graph-query's fact (`config.go:33`), reported per reply in `merge`
  (§ 2 B, C). fusionnats has no such knob (`client.go:76`).
- Provenance comes from the mode alone (`lens.go:203-212`, `engine_lens.go:131`).
- searchGraph refuses `scope` (`globalsearch.go:189`) and searches unscoped (`textsearch.go:50-52,81-83`;
  `merge.go:125`).
- ADR-071 requires scope where the seeds are found, before the limit (review additions).

## Goals / Non-Goals

Goals: the owner's ruling on #706; tier 0 unchanged; fusion's contract on damaged seeds, refusals and readiness
unchanged. Non-goals: as in the proposal.

## Decisions

**D1. Which surface an nl resolve asks.**

- (A) **searchGraph, given a `scope` key.** Smallest graph-query diff: one field, passed to both searches. Costs:
  - every hit is read back with its bytes (`textsearch.go:182`), and fusion reads them again (`engine_lens.go:150`);
    the archive rejected this for fusion (removal-keywords `design.md:51-52`);
  - damaged and missing hits leave the list (graph-query spec `:671-708`), so a damaged seed stops reaching fusion as
    `unhydrated error` (fusion spec `:388-403`);
  - the payload fit drops the lowest seeds (`textsearch.go:209`);
  - a one-word question triggers alias and entity reads, and a path question a walk (`globalsearch.go:210-213`);
  - with the word index not ready the text is null and the hits arrive as raw `semantic` bytes
    (`textsearch.go:252-260`);
  - `scope` would also reach `globalSearch`, which shares the request type.
- (B) **A new `mergedSearch` verb returning ranked IDs only (recommended).** It takes `graph.LexicalSearchRequest`, so
  there is no new request type. It answers `graph.MergedSearchData`: `results` (`entity_id`, `score`, `word_rank`,
  `vector_rank`), `matched`, `excluded`, `bodies_not_indexed` and `merge`. It shares searchGraph's ask-and-merge step.
  Costs: one verb row, one reply type, one handler, and moving the shared step out of `mergedText`. In exchange there
  is no read-back, payload cut, side read or null-text case, and damaged seeds reach fusion's batch as today. It is not
  the archive's rejected "forward beside a row": no row returns a merged list.
- (C) **fusionnats asks both searches and merges with `mergeRanks` moved to `graph`.** One merge function, but the
  fallback policy is copied (timeout, broken reply, `index_not_ready`, cancel on word refusal; graph-query spec
  `:752-788,:854-866`). fusionnats would also have to learn whether embeddings run, by probing the embedding subject
  per call. #706's author rejects a second home for the merge.
- (D) **Do nothing.** Ruled out by the owner.

Adopter seam: under B a product learns nothing new. A tier 1 stack (graph-embedding plus `embedding_enabled: true`)
merges by default, and a tier 0 stack answers as today. The provenance on each response says which happened.
Owner question 1.

**D2. Where the seeds came from.**

- (a) **One value per response (recommended).** `fusion.Resolution.Provenance`: `embedding` when `merge` is present
  and honoured, `lexical` otherwise. The engine uses it when set, else `ProvenanceForMode(mode)`, so symbol, prefix
  and other `RetrievalClient`s are unchanged. A Resolve that fails (deferral) keeps the mode's value: no reply said
  what answered.
- (b) **One value per seed.** A `Seed.Provenance` and a new wire field `Node.Provenance`, read from each match's
  ranks. More precise, but two new fields, and a node found by both lists still needs a rule.

Label: `embedding` (recommended). Values are ordered by uncertainty (`contract.go:57`), and an unearned claim takes
the conservative value (`lens.go:196-199`). The alternative is a new `hybrid`, which every consumer's switch must
learn. ADR-062's note is amended either way. Owner questions 2 and 3.

**D3. The score a seed carries.** The reply's `score`: the word score when not merged, the merged score when merged.
The alternative, the word score only, gives vector-only seeds no score. `Seed.Similarity` and `Node.Similarity`
docs change, and `include_scores` readers get a migration note. Ranking is unaffected: it ranks by resolve order
(`engine_lens.go:35-47`).

**D4. mergedSearch's behaviour.** For the same question, limit and scope, its result IDs are searchGraph's match IDs
in the same order whenever nothing is missing or damaged and searchGraph's reply fits. This equality is the oracle.

- It decodes strictly, and refuses a blank query, a limit outside 0 or 1–100, and a malformed scope prefix
  (`semtypes.ValidateEntityIDPrefix`) with `invalid_request` before any request.
- Word search: scope and `min_revision` pass to it. Embedding search: scope, no `min_revision`.
- Word refusal other than `index_not_ready`: returned as it arrived, and the embedding search is cancelled.
- Word index not ready at tier 1: the vector list alone, `merge` with detail `index_not_ready`, revision 0. If the
  embedding search fails too, the `index_not_ready` refusal is returned.
- Embedding search refused, broken or late: the word list, `merge` not honoured, one Warn naming the verb, one count
  on the merge-failed counter.
- `damaged_entities` is the larger of the two replies'. The revision is the word search's.
- At most 100 results of IDs and numbers, so there is no payload fit.

**D5. One pull request.** Graph types and verb row, graph-query handler, fusion, docs. The verb's caller lands with
it.

## Risks / Trade-offs

- **Timeout.** SemSource builds fusionnats with timeout 0, i.e. 5 s (`processor/code-context/component.go:142`;
  `natsclient/request.go:18`). graph-query waits up to `query_timeout_ms`, 15 s by default, for the embedding search
  (`config.go:16`). A slower embedder fails the nl question transient before graph-query falls back to words. That
  error is loud, not silent, and searchGraph already has it. The migration note says to set `query_timeout_ms` below
  the fusion timeout.
- **Error cause.** With no graph-lexical composed, the refusal now comes through graph-query: transient `internal`,
  naming `graph.lexical.query.search`. `natsclient.IsNoResponders` no longer holds on fusion's error
  (`TestNLResolveWithoutWordIndex` is updated).
- **Bounded port scope.** mergedSearch is a second graph-query search verb over the same step. The shared step,
  proven by the equal-order test, keeps it from drifting from searchGraph.

## Migration Plan

Consumers migrate. SemSource:

- Expect `provenance: embedding` on nl answers at tier 1.
- Expect `similarity` to be the merged score with `include_scores`.
- Update `nlDisabledClient`'s comment (`nl_disabled.go:31-45`). Tier 0 is unchanged.
- Keep `query_timeout_ms` below the fusionnats timeout.
- `fusiontest.MemGraph` (`memgraph.go:86`) compiles unchanged; `Provenance` left empty means the mode's value.

## Open Questions

1. **Owner:** a new lean verb (B, recommended), or scope on searchGraph (A)?
2. **Owner:** provenance per response (recommended) or per seed?
3. **Owner:** label a merged answer `embedding` (recommended) or a new `hybrid`?
