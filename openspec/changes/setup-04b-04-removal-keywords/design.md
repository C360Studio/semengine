# Design: setup-04b-04-removal-keywords

## Context

Facts: `inventory.md` (INVENTORY PASS at d797b547, PR #661 comment 6880325899), cited by section, and
`notes/probes/keywords-probe.log`. After change 3's PR B, graph-query has no substring scorer (§3). The hashed embedder
is the only production `Embedder` (§4, §5); graph-embedding builds it itself, and fusion's `nl` resolve reaches it
through graph-query's `semantic` (§6). Nothing calls the example classifier (§4). Keywords are counted inside the
group only (§7). Keyword quality on the scorecard corpus, 263 groups by parent directory (§7):

| Keywords | Words held by 20%+ of sections | A group's keywords searched: share of top ten in the group |
| --- | --- | --- |
| Today: most counted | 27.9% | 66.7% |
| Plain TF-IDF against the whole corpus | 2.9% | 77.8% |
| Dunning G2, `lexical.Tokens` | 7.2% | 82.4% |

## Goals / Non-Goals

Goals: plan item 4 (#10 comment 6833147079); fusion's natural-language resolve at tier 0 (change 2's non-goals);
graph-clustering's half of #650; #628's last item. Non-goals: as the proposal lists.

## Decisions

**D1. The example classifier goes (PR A).** `EmbeddingClassifier`, `NewEmbeddingClassifier`, `FindBestMatch`,
`Threshold` and `Example.Vector` are deleted, and `NewClassifierChain(keyword, llm...)` loses its middle parameter.
Tier numbers stay 0 and 3 (`ClassificationResult.Tier` is read by callers; renumbering changes values for no
behaviour). `Example`, `DomainExamples` and `LoadDomainExamples` stay: the LLM tier reads them
(`classifier_llm.go:56,65`). The `use_embeddings` hint stays: it names similarity intent, which graph-query reports
unhonoured at tier 0 and the tier 1 fallback (#563) will honour; renaming it changes the JSON the LLM prompt asks for
(`classifier_llm.go:129`). Rejected: keep the classifier uncalled (the plan removes it by name). Proof:
`TestChainAnswersFromFirstTier` (keyword hint, LLM result, default), `TestChainKeepsExclusions` with the LLM tier
only, `TestQueryImportsNoEmbedding` (`go list -deps ./graph/query` has no `graph/embedding`).

**D2. Fusion's `nl` resolve asks the word index (PR B).** fusionnats sends `graph.LexicalSearchRequest{Query, Limit,
Scope}` to graph-lexical's `search` row of `graph.QueryVerbs()`; seeds keep the hits' order and carry each hit's score
as the mode's own score (`contract.go:119-128` already says the score is the mode's own; a word index score has no
fixed scale). Rejected: a graph-query forward (a new verb beside a row the table already has); `globalSearch` (it
hydrates and groups; fusion hydrates again). Forecloses `nl` without graph-lexical: each resolve is refused transient
`internal` naming `graph.lexical.query.search`, as for any missing responder. ADR-071 holds: scope filters where seeds
are found, now the word index (graph-lexical applies scope before the limit). Proof: `TestNLResolveAsksTheWordIndex`
(subject from the table, request bytes, seed order), `TestVerbLookupNamesMissingRow` for the lexical row.

**D3. What an `nl` answer reports (PR B).** `Resolution` and `Response` drop `unreadable_records` and `failed_records`
(the embedding cache's counts) and gain `bodies_not_indexed` and `excluded`, copied from the word index's reply, each
omitted when empty; symbol and prefix answers carry neither. Without `excluded`, `go test -race` would drop every
entity holding `race` with nothing in the answer saying so. Proof: `TestNLAnswerCarriesWordIndexCounts`,
`TestSymbolAnswerOmitsCounts`.

**D4. Provenance of an `nl` answer** (owner question 2; deltas written for option b): `lexical`. Proof:
`TestProvenanceForMode` (`nl` is `lexical`, an unknown mode stays `embedding`, the conservative default).

**D5. graph-embedding runs only with a product's embedder (PR C;** owner question 3, deltas for option a). The hashed
embedder is deleted. `graphembedding.RegisterWithEmbedder(registry, newEmbedder func() (embedding.Embedder, error))`
registers the factory with the product's embedder; each component built calls `newEmbedder` once and closes what it
returns in `Stop`. Plain `Register` (the shape every component package exports) registers a factory that refuses at
construction, invalid, naming `RegisterWithEmbedder` and tier 1. `embedder_type` is removed (refused as unknown,
naming it); the dedup identity and `search`'s `embedder_type` field come from the embedder's `Model()`, so two
embedders never share dedup records (embedding-index "The dedup record keeps its vector space"). graph-embedding's
tests build it through `RegisterWithEmbedder` with a word-count test embedder, as a product would. Proof:
`TestRegisterWithoutEmbedderRefuses`, `TestProductEmbedderIsUsed` (search embeds through the product's embedder),
`TestEmbedderClosedOnStop`, the lifecycle suite, and the 33 integration tests on the test embedder.

**D6. `embedding_enabled` reads false when left out (PR C).** At tier 0 nothing serves the embedding verbs, so the old
default refused each `semantic` request transient `internal` (inventory §5). Proof: `TestConfigEmbeddingEnabled`.

**D7. Smoke (PR C; #628).** The stack composes seven components, no graph-embedding; the `semantic`, `similar`,
embedding-record and `bm25` checks go; fusion's word question answers the live entities holding the word with
provenance `lexical`. The no-embedder stack goes (the main stack is it); the fusionnats import guard stays as its own
requirement. Proof: `TestSmoke_OneConfigAnswers`, the three restart tests, `TestFusionNATSImportsNoEmbedding`.

**D8. Keywords are the group's distinctive words (PR D;** owner question 1, deltas for option a). When a group's
record is written, each candidate word is scored by the share of the group's members holding it times the log of the
graph's member count over the graph's members holding it; the ten best, ties lexical, need at least two holders in a
group of two or more. Counts come from each member's 32 most frequent words, as today. The maintainer keeps the
graph-wide counts beside the group counts, so a tick still writes only touched groups. Words are `extractWords`'s, not
`lexical.Tokens`' (folded keys such as `thi` and `alway` are not words, §7). Proof: `TestKeywordsScorecard` (the
scorecard corpus grouped by parent directory: share of corpus-common keywords and keyword-search precision, each
against a plain TF-IDF over all of every section's words computed in the test; floors set from the first run and
never worse than today's 27.9% and 66.7%), `TestKeywordsScoredWhenWritten`, `TestTickCost` unchanged.

**D9. graph-clustering reads text through `graph/entitytext` (PR D; #650).** `entityWords` reads
`entitytext.Rule.Values` under a new `text_suffixes` key with graph-lexical's meaning and default, so both see the
same text. Representatives are the smallest members with text under that rule. Proof: `TestKeywordTextMatchesLexical`
(an entity's keyword text equals graph-lexical's `Values` for it), `TestTextSuffixesIsRead`.

**D10. Four pull requests, after #660** (chosen). A: `graph/query` (D1). B: `pkg/fusion`, fusionnats, smoke's NL
check (D2-D4). C: `graph/embedding`, graph-embedding, graph-query's default, smoke (D5-D7); needs A (`graph/query`
imports the hashed embedder, `classifier_embedding.go:71`) and B (smoke's NL check). D: `graph/clustering`,
graph-clustering (D8, D9) and the archive. Order A, B, C, D; each merges `origin/main` before landing. Retires: C
`Closes #628`; D `Closes #650`. Stay: #563 and #489 (tier 1; #489's provider becomes one `Embedder` a product
registers, and its degrade test names the word index); #11, #485 (its premise text changes with D8), #619 (C keeps
the `graph/embedding` floor), #620, #651, #656, #659. Rejected: one pull request (four packages, four capabilities).
Sizes: A deletes 1,154 lines and C 1,099 (inventory §12), C also rewires 39 test call sites; B (about 300
changed) and D (about 600) are estimates.

## Risks / Trade-offs

- A group not written since the graph changed keeps keywords scored against older counts: at 5% churn about half of
  untouched groups would score differently (§7). A restart re-scores every group (bootstrap marks each dirty,
  `maintainer.go:364`). No reader decides on keywords after change 3.
- graph-embedding cannot run until a product registers an embedder; #489 supplies the first one in the tree.
- `nl` without graph-lexical composed is refused per request, not at boot.

## Migration Plan

Consumers migrate (`MIGRATION.md` of each package): SemSource removes graph-embedding and `embedder_type: bm25` from
its tier 0 composition (boot fails naming the key otherwise) and composes graph-lexical; its code-search gate
(`internal/graphstatus/reader.go:37-38`, `source_status`'s `embedding`, workbench `semantic`) reads `graph-lexical`;
its UI and `configs/tiers/README.md:293` expect `provenance: lexical` on word questions; `fusiontest.MemGraph` builds
`Resolution{Seeds, BodiesNotIndexed, Excluded}`. Products drop the middle `NewClassifierChain` argument.

## Open Questions

1. **Keywords against the whole graph** (spec community-grouping `:148-153`; ADR-099's port note). (a) Score when the
   group is written against graph-wide counts, writing only touched groups (recommended): corpus-common keywords
   27.9% → 2.9%, precision 66.7% → 77.8% (TF-IDF row, all groups freshly scored; the top-32 approximation not
   measured); no extra writes (`TestTickCost` holds); about half of untouched groups lag at 5% churn; memory for one
   graph-wide map, not measured. (b) Re-score every group each tick and write those that changed: no lag; writes
   about half of untouched groups at 5% churn on the probe, not measured at scale. (c) Keep today's rule: no work,
   27.9% corpus-common.
2. **Provenance of an `nl` answer** (ADR-062's tiers). (a) `deterministic`: no new value, but ADR-062 gives it to
   exact lookup and walks, and a ranked word match is neither. (b) `lexical` (recommended): one new value naming how
   the seeds were found; SemSource's UI fixtures and `configs/tiers/README.md:293` change (read at 4ca2072c; no
   SemSource Go branches on the value, rg). (c) Keep `embedding`: false once no embedding is read.
3. **How graph-embedding gets an embedder at tier 0** (new exported surface). (a) `RegisterWithEmbedder`, a
   product-supplied `Embedder` (recommended): one function; tests use the product path, not a back door; #489's
   provider becomes one such embedder. (b) Bring #489's HTTP embedder forward: 220 pin lines plus its model registry
   and an HTTP test server; 04C work now. (c) Keep the hashed embedder as an opt-in until #489: contradicts the plan's
   "`BM25Embedder` … go".
