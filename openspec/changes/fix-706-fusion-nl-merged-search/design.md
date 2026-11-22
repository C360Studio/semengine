# Design: fix-706-fusion-nl-merged-search

## Context

Facts: `inventory.md` (INVENTORY PASS at 48c2e10d), cited by section.

- fusion asks graph-lexical alone (§ 1).
- The merge and its fallback live only in graph-query (`merge.go:32`, `textsearch.go:76-125`; § 2 A).
- "The composition runs embeddings" is graph-query's fact (`config.go:33`), reported per reply in `merge` (§ 2 B, C).
- Provenance comes from the mode alone (`lens.go:203-212`, `engine_lens.go:131`).
- searchGraph refuses `scope` (`globalsearch.go:189`) and searches unscoped (`textsearch.go:50-52,81-83`;
  `merge.go:125`). ADR-071 requires scope where the seeds are found, before the limit (review additions).
- Timeouts: fusionnats at 0 is natsclient's 5 s (`natsclient/request.go:18`; SemSource passes 0,
  `processor/code-context/component.go:142`). graph-query waits up to `query_timeout_ms`, 15 s by default
  (`config.go:16`).

## Goals / Non-Goals

Goals: the owner's ruling on #706; tier 0 unchanged; a tier 1 answer never slower to fail than today's. Fusion's
contract on damaged seeds and refusals is unchanged. Non-goals: as in the proposal.

## Decisions

**D1. Which surface an nl resolve asks.**

- (A) **searchGraph, given a `scope` key.** Smallest graph-query diff. Costs:
  - every hit is read back, and fusion reads them again (`textsearch.go:182`, `engine_lens.go:150`); the archive
    rejected this for fusion (removal-keywords `design.md:51-52`);
  - damaged and missing hits leave the list (graph-query spec `:671-708`), so a damaged seed stops reaching fusion as
    `unhydrated error` (fusion spec `:388-403`);
  - the payload fit drops the lowest seeds (`textsearch.go:209`);
  - one-word and path questions trigger side reads (`globalsearch.go:210-213`);
  - an unready word index gives null text (`textsearch.go:252-260`);
  - `scope` would reach `globalSearch` too.
- (B) **A new `mergedSearch` returning ranked IDs only (recommended).** It takes `graph.LexicalSearchRequest` and
  answers `graph.MergedSearchData`, sharing searchGraph's ask-and-merge step. Costs: one verb row, one reply type, one
  handler, and moving the shared step out of `mergedText`. In exchange there is no read-back, payload cut, side read
  or null text, and damaged seeds reach fusion's batch as today.
- (C) **fusionnats asks graph-lexical's `search` and graph-query's `semantic`, and merges with `mergeRanks` exported
  from `graph`.** No probe is needed: `semantic` refuses `not_enabled` at tier 0 (`query.go:45-46`). #706's Ask
  allows it ("graph-embedding's search with fallback"). Its advantage is that fusionnats owns the embedding deadline
  under its own timeout. Its costs:
  - the owner's words, "uses the merged search";
  - the fallback policy copied into fusionnats (graph-query spec `:752-788,:854-866`);
  - a second exported home for the merge rule;
  - sending both at once needs a goroutine in a client that "starts nothing" (fusion spec `:232-241`), and sending them
    one after the other adds the embedding wait to every question.
- (D) **Do nothing.** Ruled out by the owner.

Adopter seam under B: nothing new to learn. Tier 1 (graph-embedding plus `embedding_enabled: true`) merges by default;
each response's provenance says what happened. Owner question 1.

**D2. Where the seeds came from.**

- (a) **One value per response (recommended).** `fusion.Resolution.Provenance`: `embedding` when `merge` is present
  and honoured (merged, or meaning alone while the word index builds), `lexical` otherwise. The engine uses it when
  set, else `ProvenanceForMode(mode)`; a failed Resolve keeps the mode's value. Other `RetrievalClient`s are
  unchanged.
- (b) **One value per seed.** It adds `Seed.Provenance`, a wire field `Node.Provenance`, and `word_rank`/`vector_rank`
  on the reply to compute it from. A seed in both lists still needs a rule.

Label: `embedding` (recommended). Values are ordered by uncertainty (`contract.go:57`), and an unearned claim takes
the conservative value (`lens.go:196-199`). A new `hybrid` is one every consumer's switch must learn. Owner
questions 2 and 3.

**D3. The score a seed carries.** The reply's `score`: the word score when not merged, the merged score when merged.
Word-score-only would give vector-only seeds no score. Ranking is unaffected (`engine_lens.go:35-47`). The
`Seed.Similarity` and `Node.Similarity` docs change.

**D4. mergedSearch's behaviour.** For the same query and limit, its result IDs equal searchGraph's match IDs in
order, whenever searchGraph left no hit missing or damaged and its reply fit. This equality is the oracle.

- It decodes strictly and refuses, before any request, a blank query, a limit outside 0 or 1–100, and a malformed
  scope prefix (`semtypes.ValidateEntityIDPrefix`). Scope reaches both searches; `min_revision` the word search alone.
- A word refusal other than `index_not_ready` is returned as it arrived, and the embedding search is cancelled.
- Word index not ready, embedding answers: the embedding list, `merge` honoured with detail `index_not_ready`,
  revision 0. Both fail: the `index_not_ready` refusal.
- Embedding refused, broken, late (D6), or holding a hit whose ID is not a literal entity ID: the word list, `merge`
  not honoured. searchGraph keeps the same rule for a broken vector ID (`textsearch.go:98-103,158-169`).
- A word hit whose ID is not a literal entity ID is refused, as searchGraph refuses it (`textsearch.go:144-154`).
- A failed merge logs one Warn naming the verb and adds one to `searchMergeFailed` (`metrics.go:16`), which stays
  shared and unlabelled; its help text widens to both verbs.
- The reply holds `results` (`entity_id`, `score`), the word search's `excluded` and `bodies_not_indexed`, and
  `merge`. `matched` and ranks are left out because nothing reads them under D2(a). It carries the word search's
  revision and the larger `damaged_entities`. At most 100 results, so no payload fit.

**D5. One pull request.** Graph types and verb row, graph-query handler, fusion, docs. The verb's caller lands with
it.

**D6. How long mergedSearch waits for the embedding search.** The default caller budget is 5 s; an embedder taking
5–15 s would turn questions that answer today into failures.

- (a) **min(`query_timeout_ms`, 4 s) (owner, 2026-11-22).** It leaves about 1 s of a caller's default 5 s for the word
  search and the reply. A slow embedder gives today's word answer, labelled `lexical` and counted. Measured against
  semembed (arctic-embed-s, local, 2026-11-22): a query takes about 0.01 s idle and 0.33 s beside graph-embedding's
  default 5 workers; heavy batching (4 × 32 documents) queues it to 6.5 s; a starting semembed refuses at once.
- (b) **A config key.** A fact every product must learn.
- (c) **Raise fusionnats' default above 15 s.** fusionnats would be predicting graph-query's config.

The same hazard in searchGraph is a defect this change does not fix; it is #712.

## Risks / Trade-offs

- A tier 1 embedder that always answers in 4–5 s never merges through fusion. The provenance (`lexical`), the
  Warn and the counter say so; searchGraph is unaffected.
- With no graph-lexical, fusion's error comes through graph-query: transient `internal` naming
  `graph.lexical.query.search`, and `natsclient.IsNoResponders` no longer holds (`TestNLResolveWithoutWordIndex`).
- mergedSearch is a second search verb over the same step. The equal-order test keeps the two from drifting apart.

## Migration Plan

Consumers migrate. SemSource:

- Expect `provenance: embedding` on nl answers at tier 1, and `similarity` as the merged score with
  `include_scores`.
- At tier 1, while the word index builds after a start, expect answers from meaning alone (`embedding`, no
  `excluded`) where a deferral came before.
- Update `nlDisabledClient`'s comment (`nl_disabled.go:31-45`). Tier 0 is unchanged.
- `fusiontest.MemGraph` (`memgraph.go:86`) compiles unchanged: an empty `Provenance` means the mode's value.

## Open Questions

1. **Owner (2026-11-22):** B, a new small search that returns only the ranked IDs and filters by ID prefix.
2. **Owner (2026-11-22):** one label for the whole answer.
3. **Owner (2026-11-22):** `embedding` only when meaning results are in the answer (merged, or meaning alone while the
   word index builds); `lexical` whenever the answer came from words alone, at any tier.
