# Proposal: setup-04b-04-removal-keywords

Approved-by:

Track: `design`. Modified capabilities: `query-classification`, `fusion`, `graph-embedding`, `graph-query`,
`composed-stack`, `community-grouping`, `graph-clustering`, `entity-text`.

## Why

Tier 0 search is the word index once change 3 lands, but three older spellings of "find by words" stay in the tree
(inventory §4): the hashed embedder (384 slots, cosine), which graph-embedding runs at tier 0 and fusion's
plain-language questions read through `semantic`; the example classifier, which nothing calls; and community keywords
that are a group's commonest words (27.9% of them are words held by a fifth of all sections,
`notes/probes/keywords-probe.log`). This is change 4 of the plan on epic #10 (comment 6833147079): "`BM25Embedder`, the
substring scorers and `EmbeddingClassifier` go; graph-embedding stays as the tier 1 home, off at tier 0. Community
keywords become the words distinctive to a group."

## What Changes

- `graph/query` loses the example classifier; the chain is keyword rules, then the optional LLM tier.
- Fusion's `nl` resolve asks graph-lexical's `search` directly. Its answers say `bodies_not_indexed` and `excluded`
  where they said `unreadable_records` and `failed_records`, and their provenance is owner question 2 (recommended:
  a new `lexical`).
- The hashed embedder is deleted. graph-embedding runs only with an embedder a product registers (owner question 3);
  plain `Register` gives a factory that refuses at boot, naming tier 1. `embedder_type` is gone. graph-query's
  `embedding_enabled` reads false when left out.
- Smoke composes no graph-embedding, so no check reads the old search path (#628); the no-embedder stack folds into
  the main one.
- graph-clustering reads text through `graph/entitytext` with its own `text_suffixes` (#650), and keywords are scored
  against the whole graph's counts when a group is written (owner question 1), with a required known-answer test
  against a plain reference.

## Non-goals

Change 3's work (graph-query search, the router, its substring scorer); the tier 1 provider and fallback
(#489, #563); a stored index; renaming the `use_embeddings` hint; tokenizer changes (#651).

## Impact

Consumers migrate (design, Migration Plan). SemSource: drop graph-embedding and `embedder_type: bm25` from its tier 0
composition, compose graph-lexical, gate code search on the `graph-lexical` readiness key, read the new provenance
and counts. Products calling `NewClassifierChain` drop the middle argument.

## Retires, and in which pull request

PR A (graph/query): none. PR B (fusion): none. PR C (graph-embedding off at tier 0): `Closes #628`. PR D (keywords,
with the archive): `Closes #650` (graph-query's half lands with change 3's PR B). Open issues that
stay: #563, #489, #11, #485, #619, #620, #651, #656 and #659 (design D10).

## Rulings

Covered: plan item 4 (#10 comment 6833147079); consumers migrate (owner, 2026-11-02); the fallback of #563 is
built with #489 (6877352156). Open: design questions 1 to 3.

Changed ADR decisions, for approval with this proposal: ADR-062's provenance tiers gain `lexical` and "Semantic …
covers NL" becomes the word index at tier 0 (question 2); ADR-071's scope is applied by the word index, its decision
(filter at the candidate source) unchanged; ADR-099's port note says keywords are scored against the whole graph
(question 1). Each PR updates the port note it touches.
