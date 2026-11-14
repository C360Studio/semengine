# Inventory: setup-04b-04-removal-keywords

base: b6d97ad04f6fa583df6bdab3ba643a8605849d2d (main a8dd7912 + empty claim commit; worktree
.worktrees/claude-lexical-removal-keywords). Track: design (touches graph-embedding, embedding-index,
query-classification, fusion, community-grouping, composed-stack, entity-text and graph-query specs). SemSource read
at its local checkout, commit 4ca2072c5d9d6bf4ec5ad76dc33b92d61c1614c1 (read from the branch ref
`.git/refs/heads/main`, which `.git/HEAD` names).

Terms. *Word index*: `graph/lexical`, served by graph-lexical on `graph.lexical.query.search`. *Hashed embedder*:
`graph/embedding.BM25Embedder`, words hashed into 384 slots and ranked by cosine. *Example classifier*:
`graph/query.EmbeddingClassifier`, tier 1 of the classifier chain. *NL resolve*: fusion's `nl` mode
(`fusion.ResolveModeNL`). *Keywords*: `clustering.Community.Keywords`. *PR B*: change 3's draft #660.

## 1. Open issues naming the territory

Queries (2026-11-14, `gh issue list --state open --search <q>`): embedding, bm25, BM25Embedder, classifier,
clustering, keywords, terms, fusion, fusionnats, graph-query, semantic, entitytext, "tier 0", smoke. Each hit's body
and comments read.

| Issue | What it asks | Bearing on this change |
| --- | --- | --- |
| #650 | Read text through `graph/entitytext`; graph-clustering's half is `graph/clustering/terms.go:57` | in scope (plan item 4); graph-query's half is PR B's |
| #628 | Smoke has no check on the old search path; change 4's item: "removes the `bm25` embedder assertions" | in scope; closes when smoke reads no old path |
| #563 | searchGraph's labelled tier 1 fallback. Ruled 6877352156: built with #489, stays open | no work here; D7 of change 3 is the plug-in point |
| #489 | Tier 1 HTTP embedder; owes "losing the provider degrades to tier 0 with search intact" | stays (04C); after this change tier 0 search is the word index, which that test must name |
| #11 | Epic 04C, tier 1 neural retrieval | stays; graph-embedding becomes wholly its territory |
| #485 | Tier 2 LLM summaries; "at tier 0 `graph/clustering` keeps a deterministic statistical summary (keywords counted within each group)" | stays; its premise text changes with the keyword rule |
| #506 | Keyword index per prefix | PR B retires it (change 3 proposal); keywords stop gating there |
| #564 | graph-embedding logs a false drain warning on every clean stop (`graph/embedding/worker.go:314-317`) | stays; untouched by removal |
| #658, #657 | One damaged entity stops graph-index, graph-embedding, graph-lexical; ruled "later, not now" | stays; graph-clustering's skip (`processor/graph-clustering/component.go:276-286`) is unchanged |
| #659 | Fusion's 40-seed batch read fails on `response_too_large` (`pkg/fusion/engine_lens.go:31`, `:135`) | stays; NL mode keeps it whatever it resolves through |
| #621 | repository-map has no `pkg/fusion` rows | stays; same file edited here (`docs/repository-map.md:74,76,91`) |
| #111 | Silent degrades; item 3's NL sites are not carried (comments 2026-11-07) | stays (item 4, change 7) |
| #486, #487, #484 | Tier 2 classifier, synthesis, epic | stay; #486 joins the chain whose tier 1 this change empties |
| #535, #616 | Keyword classifier places; router | PR B's; the keyword tier is untouched here |
| #656 | A batch read refuses every entity when one is damaged; fusionnats reads each ID alone on that refusal (fusion spec `:323-339`) | stays; the hydrate step after any NL resolve source |
| #620 | Re-measure `NameWeight` on SemSource names | stays; §7's probe has no SemSource data either |
| #651 | Tokenizer keeps `base64Encode` as one word | stays; bears on question 5 if keywords use `lexical.Tokens` |
| #619 | cover-check target count is stale | stays; the `graph/embedding merged` floor (`scripts/cover-check.sh:54`) changes here |
| #109, #110, #141, #155, #195, #196, #477, #68, #635, #570, #574, #571, #630, #641, #602, #108, #300, #350, #24, #84, #36, #361, #9, #10, #134, #536, #554 | Name the word, a sibling package or an epic, not this territory | n/a |

## 2. Overlapping open PRs, and order

`gh pr list --state open` (file lists via `gh api .../pulls/<n>/files`): #661 (this claim, 0 files), #660 (PR B,
draft, 0 files pushed; its worktree holds local commit 8eefecdd, task 2.2), #597 and #598 (Dependabot, `go.mod` and
`go.sum` only: no overlap).

PR #660 overlaps on `internal/smoke/smoke_integration_test.go` (its 8eefecdd changes 64 lines), the graph-query search
files, `graph/query_*search_types.go` (the record fields that carry keywords) and the graph-query spec. Change 3's
proposal: "Code merges after change 3's PR B". This change's code PRs merge after #660; the second to land merges
`origin/main`.

Spec overlap with change 3 (in flight): its composed-stack delta
(`openspec/changes/setup-04b-03-graph-query-lexical/specs/composed-stack/spec.md:5-20`) modifies "The graph stack
boots from one configuration", which asserts `semantic`, `similar` and one embedding record per entity; this change
rewrites the same requirement. Its graph-lexical delta (`specs/graph-lexical/spec.md:5`, "A search reply is scoped,
limited, ranked and counted") overlaps if fusion's NL resolve reads that reply. Both archive with PR B first.

## 3. What PR B leaves for this change

Read from change 3's `tasks.md` and `design.md` and #660's worktree at 8eefecdd (read only):

- Substring scorers. 8eefecdd moves `searchEntity.score`, `checkedSearchTerms` and `rankEntities` into
  `processor/graph-query/textsearch.go:251-332` for the text tier; task 2.3 deletes "today's text tier and its group
  score" (`textGroups`, `groupScore`, `:113-153`). After PR B no substring scorer is left in graph-query. `git grep -n
  -i substring -- '*.go' ':!*_test.go'` on base finds no other text scorer (hits: `graph/clustering/terms.go:69`, a
  comment; `graph/embedding/worker.go:742`, `natsclient/errors.go:35`, `processor/graph-ingest/query.go:93`,
  `vocabulary/predicate_contract.go:69`, none a scorer). Premise to re-check when #660 lands.
- The keyword classifier stays in graph-query: every question is classified for hints (change 3 D5),
  `query.NewClassifierChain(query.NewKeywordClassifier(), nil)` (`processor/graph-query/globalsearch.go:326`).
- `semantic`, `similar` and `embedding_enabled` are untouched by PR B (change 3 delta graph-query `:245`).
- Smoke: task 2.6 leaves "the bm25 assertions" here.
- The similarity hint stays: PR B reports it unhonoured (`processor/graph-query/globalsearch.go:426-427` at 9f9ac44d,
  `hints.similarity` named "a similarity"); see §4 for its name.
- Keywords after PR B are read for display only. At 8eefecdd: `processor/graph-query/localsearch.go:185` (record in
  the local reply) and, until 2.3 lands, `textsearch.go:139-145` (the gate) and `:177-178,216-217` (reply and the
  "Key themes" sentence). The change 3 delta keeps `keywords` and `community_summary` in both replies (delta
  `:44-47`, `:63-67`).

## 4. Callers of the removed surface

gopls `references`, run with and without `-tags=integration`, gave the same results.

- `BM25Embedder` (`graph/embedding/bm25_embedder.go:61`), `BM25Config` (`:75`), `NewBM25Embedder` (`:90`): outside
  their own two files, only `processor/graph-embedding/component.go:87` — `embedder *embedding.BM25Embedder` — and
  `:325` — `c.embedder = embedding.NewBM25Embedder(embedding.BM25Config{})`; plus
  `graph/query/classifier_embedding.go` (`:29`, `:71`). `stem` and `stopwords` (`:277`, `:18`) have no caller
  outside the file.
- `Embedder` interface (`graph/embedding/embedder.go:9`): implementers are `BM25Embedder` and two test fakes
  (`graph/embedding/review_findings_test.go:258`, `worker_fakes_test.go:146`). Readers:
  `graph/embedding/worker.go:145,178`. With the hashed embedder gone, no production type implements it.
- `EmbeddingClassifier` (`graph/query/classifier_embedding.go:27`): `classifier_chain.go:30` — `embedding
  *EmbeddingClassifier // T1` — `:37` (the `NewClassifierChain` parameter) and `:126` (the tier 1 branch); tests
  `classifier_chain_test.go:17,579,585`. `NewEmbeddingClassifier` (`:47`): tests only —
  `classifier_clock_test.go:292`, `classifier_exclusion_test.go:121,134,151`,
  `classifier_outcomes_test.go:140,148,155,310,336`. No production caller.
  `Example.Vector` (`graph/query/examples.go:14`) is written only by it; `Example` and `DomainExamples` are also read by
  the LLM tier (`classifier_llm.go:56,65,173,176`). `LoadDomainExamples` (`examples.go:25`) has no production caller.
- The `use_embeddings` hint, named for the tier this change takes off tier 0: `SearchOptions.UseEmbeddings`
  (`graph/query/search_options.go:37`), set by the keyword tier (`classifier.go:151-153`), counted and mapped by the
  chain (`classifier_chain.go:181,204-205`), the LLM tier (`classifier_llm.go:34,129,220,272,313-314`) and the example
  classifier (`classifier_embedding.go:207-208`); read by graph-query as `hints.similarity`
  (`processor/graph-query/globalsearch.go:367`).
- `graph/embedding` importers: `graph/query/classifier_embedding.go` and processor/graph-embedding only (production);
  `internal/smoke/smoke_integration_test.go` (test).
- graph-embedding composed in tests outside its package: `pkg/fusion/fusionnats/client_integration_test.go:116`,
  `processor/graph-query/embedding_integration_test.go:32,87` (138 lines),
  `internal/smoke/stack_integration_test.go:71,90-91,152,182,734`.
- Smoke: `checkSearch` (`internal/smoke/smoke_integration_test.go:515-607`): `:544` asserts `EmbedderType == "bm25"`;
  `:563` fuses an NL question; `:572` wants `fusion.ProvenanceEmbedding`; `:510-513` the comment on the hashed embedder;
  `checkEmbeddingRecords` (`:377`); `internal/smoke/doc.go:6,9`; `noembedder_integration_test.go:181-191` wants the NL
  question refused `not_enabled` naming `semantic`.
- Contract tests: `TestEmbeddingClassifierAsAProduct` (`graph/query/classifier_outcomes_test.go:291`, the product
  test of query-classification `:35-41`); `TestEmbeddingImportsNoProviderModule`
  (`internal/harness/contract/embeddingprovider_test.go:20`); `TestFusionNATSImportsNoEmbedding`
  (`internal/harness/contract/fusionnats_test.go:15`); coverage floor
  `"graph/embedding merged"` (`scripts/cover-check.sh:54`).
- Specs: query-classification `:5`, `:35-41`, `:73`, `:110-120`, `:122-136`, `:220-231`, `:233-252`, `:254-266`;
  graph-embedding `:4`, `:54-67` ("Tier-0 config": refuse any `embedder_type` other than `bm25`), `:82`;
  embedding-index `:4`; entity-text `:4,13,47`; composed-stack `:4`, `:15`, `:27-29`, `:105-116`; fusion `:68-86`,
  `:263-276`, `:305-314`; community-grouping `:128-160`.
- Docs: `.agents/skills/query-pattern/SKILL.md:30-35` (graph-embedding answers `semantic`, `similar`);
  `graph/embedding/README.md:25`, `doc.go:2,13-14`; `graph/query/README.md:15,36,50-60`, `doc.go:8`;
  `processor/graph-embedding/README.md:9,65,75,134,189`, `MIGRATION.md:21-24`; `processor/graph-query/README.md:15-16,
  34,43,317`, `MIGRATION.md:118-119,156-157,209-216`; `docs/repository-map.md:74,76,91`; `docs/contract.md:33`
  ("keyword search (BM25, computed in process)"); `docs/setup-plan.md:78,91,125` ("text relevance by BM25 … BM25 is in
  process").
- Ledger: `graph/embedding` (`docs/admission-ledger.yaml:3834-3907`; `:3837`, `:3883` "BM25 is a lexical embedder, not
  a correct lexical index"); `graph/query` (`:3927-4000`; `:3932`, `:3939-3989`); `processor/graph-embedding`
  (`:4591-4667`; `:4594`, `:4601-4603`); `pkg/fusion` (`:4668`); `processor/graph-query` (`:4327`);
  `pkg/fusion/fusionnats` (`:4782-4863`); `processor/graph-clustering` (`:4864`, if a text-suffix key is added);
  `graph/clustering` (`:4110-4181`; `:4134`, `:4159`).
- ADR text: `docs/adr/062-deterministic-graph-fusion.md:97,168,181-182`; `071-fusion-domain-scoped-nl-retrieval.md:5-18,
  49-51`; `063-store-substrate-and-resolver.md:365`.

## 5. graph-embedding at tier 0 today

- Its only embedder is the hashed embedder (`component.go:325`); the Worker and the `search` verb share it
  (`query.go:84` `c.embedder.GenerateQuery`). Config keys (`processor/graph-embedding/config.go:29-48`): `ports`,
  `embedder_type`, `workers`, `max_text_len`, `text_suffixes`, `startup_attempts`, `startup_interval_ms`,
  `coalesce_ms`. `embedder_type` accepts only `bm25`, empty means `bm25` (`:69-71`, `:121-122`); `max_text_len` 0 is
  4,000 for BM25 (`:24`). `search` replies `embedder_type` (`query.go:101`).
- Remove the hashed embedder and nothing in the tree can be embedded: no production `Embedder` remains (section 4), the
  HTTP embedder is `defer-exclude` (ledger `:3908`, #489), and the component builds its embedder itself, with no way
  for a product to pass one (`component.go:325`; `embedder.go:7` says "BM25 here, or a product's own").
- Tests that start the component: 33 integration tests in 6 files (`startEmbedding` / `startAfter`, 39 call sites),
  plus `lifecycle_test.go`, `factory_test.go`, `config_test.go`. All run the hashed embedder.
- graph-query forwards `semantic` and `similar` (`graph/query_verbs.go:135-138`) while `embedding_enabled` is true,
  its default when left out (`processor/graph-query/component.go:69-70`, `config.go:33`). False refuses invalid
  `not_enabled` with no request; true with no graph-embedding refuses each request transient `internal` (spec
  `:875`, `TestNoRespondersIsTransient`).
- No tier-0 answer for `similar` exists outside graph-embedding: `git grep -n -i "more like\|similar" --
  graph/lexical processor/graph-lexical` finds only the shared limit constants
  (`processor/graph-lexical/query.go:116-123`).
- Consumers. SemSource (local checkout, read-only, pins semengine `v0.1.0-alpha.4.0.20261112145006-7bf638643b98`)
  defaults `embedder_type` to `bm25` (`cmd/semsource/run.go:768`), composes graph-embedding unless `none`
  (`:868-890`), sets `embedding_enabled` false only for `none` (`:789-791`), accepts `none`, `bm25`, `http`
  (`config/config.go:259`), and has a disabled-readiness path (`processor/source-manifest/readiness.go:56-69`). No
  SemSource Go calls `semantic`, `similar` or `graph.embedding.query.*` directly (rg, 0 hits); its route to them is
  fusion's NL resolve. It does read graph-embedding's readiness as the gate for code search:
  `internal/graphstatus/reader.go:37-38` ("KeyGraphEmbedding is the semantic index: it backs code_search"); the
  `embedding` field of the agent-facing `source_status` tool (`processor/mcp-gateway/tools.go:143-147`); the workbench
  `semantic` capability (`processor/source-manifest/workbench_capabilities.go:31,293`, `component.go:747`).
  graph-lexical publishes its readiness under `graph-lexical` (`graph/readiness/watcher.go:59`). semteams, semconnect,
  semboids: no import of `graph/embedding`, `graph/query`, `pkg/fusion` or graph-embedding (rg, 0 hits).

## 6. Fusion's NL resolve

- Today: `fusionnats.Client.Resolve` sends `nl` to `resolveSemantic` (`pkg/fusion/fusionnats/client.go:100`,
  `:151-177`): a `graph.SemanticRequest{Query, Limit, Scope}` to graph-query's `semantic` row, seeds with
  `Similarity`, and the reply's `unreadable_records`, `failed_records` copied into the `Resolution`. Rows are looked
  up among `ResponderGraphQuery` rows only (`:30-55`); spec fusion `:263-266`: "Each request SHALL go to the subject
  of its graph-query row".
- Provenance: `nl` is `ProvenanceEmbedding` (`pkg/fusion/lens.go:195-204`; `contract.go:60` "seeds came from semantic
  search"). Similarity is the mode's own score (`contract.go:119-128`, `retrieval.go:48-62`); counts
  (`contract.go:180-186`, spec fusion `:68-86`, `:305-314`).
- The word index's verb takes the same request fields (`graph/query_lexical_types.go:10-15` against
  `graph/query_search_types.go:33-38`): `query`, `limit`, `scope` (entity-ID prefixes, before the limit),
  `min_revision`. Its reply has `score` (BM25F, "on no fixed scale, comparable only within one reply", `:17-22`),
  `matched`, `excluded`, `bodies_not_indexed`; no `unreadable_records` or `failed_records`. No graph-query row
  forwards it (`graph/query_verbs.go:73-145`).
- Readiness: if NL resolve moves off graph-embedding, SemSource's code-search gate (§5) still names the
  `graph-embedding` key, a producer that no longer serves those seeds (ADR-062: only `ready` licenses not-found).
- Lenses: SemSource's docs lens resolves `nl` by default (`source/fusion/lens/docs/docs.go:57,65`), the code lens on
  `:38`; `processor/code-context/component.go:297` marshals `fusion.Response` (provenance included) to its callers.
- Tests: `pkg/fusion/fusionnats/client_test.go` (30 lines naming semantic/NL), `client_integration_test.go:116`,
  smoke `:563-573`, `noembedder_integration_test.go:181-191`.

## 7. Community keywords

- Rule (`graph/clustering/terms.go`): `extractWords` (`:28`) keeps runs of ASCII letters and digits, 3 to 32 bytes,
  lower-cased, minus 30 stopwords (`:16-22`). `entityWords` (`:57`) reads every non-relationship string value, not
  `entitytext` (#650), and keeps each entity's 32 most frequent words (`MaxEntityTerms`, `types.go:11`). The maintainer
  counts, per group and level, how many members contributed each word (`maintainer.go:353-361`, `:376-379`, `:414`);
  `newCommunity` keeps the 10 most counted (`terms.go:89`, `MaxKeywords`, `types.go:9`) and puts five in the summary
  (`:119`).
- Spec community-grouping `:148-153`: keywords "SHALL be the group's most frequent words … so a change outside the
  group never changes its keywords"; `:88-105`: a tick writes only touched groups (`TestTickCost`). ADR-099 port note
  (`docs/adr/099-…:9-17`): "kept from changes, never by rescanning", bounded by construction.
- What a group's words could be compared against, in the tree: the level-2 group (the 2-position deployment prefix)
  already holds every member's top-32 word counts (`maintainer.go:353-361` runs per level); `lexical.Tokens`
  (`graph/lexical/tokens.go:45`) folds plurals into index keys that are not always words ("cach", `:36`); the
  lexical index has no exported per-word document count (`graph/lexical/index.go`, exported: `Put`, `Delete`, `Len`,
  `Search`). graph-clustering has no `text_suffixes` key (`processor/graph-clustering/config.go:46-53`); graph-lexical
  has one (`processor/graph-lexical/config.go:32`).
- Measured (`notes/probes/keywords-probe.log`, scorecard corpus, 2,000 sections, 500 questions). Reference: two textbook
  keyness scores of a word in a group against the whole corpus (TF-IDF over groups; Dunning's log-likelihood G2),
  written in the probe. "Generic": a keyword held by 20% or more of all sections. "Kw-query P@10": the group's
  keywords searched on the word index, share of the first ten hits inside the group, over groups of at least ten
  members only (5 of the 21 groups; 46 of the 263).

| Grouping | Keywords | Generic | Kw-query P@10 | Answer's group ranked first by keyword overlap |
| --- | --- | --- | --- | --- |
| 21 groups (corpus IDs; one holds 1,758) | today's | 27.1% | 58.0% | 55.8% (largest-group baseline 88.8%) |
| | G2, extractWords | 7.7% | 74.0% | 56.4% |
| | G2, lexical.Tokens | 10.5% | 76.0% | 57.6% |
| 263 groups (file's parent directory) | today's | 27.9% | 66.7% | 12.2% (baseline 11.6%) |
| | TF-IDF, extractWords | 2.9% | 77.8% | 16.0% |
| | G2, lexical.Tokens | 7.2% | 82.4% | 13.2% |

Today's top words for the largest group: `test not graph its that one check after task only`; G2: `notes task
test shall inventory design simplify 04a specification mutant`. Drift (the `lexical.Tokens` variants):
removing every 20th section changes the G2 keywords of
97 of the 201 groups that lost no member (TF-IDF: 103); today's rule, 0 by construction. The drift count includes
reordering inside the ten. Folded forms appear as keywords under `lexical.Tokens` (`thi`, `alway`). Limit: the
corpus is one repository's Markdown, where both predicates are text suffixes, so `entitytext` and "every string
value" select the same text here; SemSource data is not measured (#620).

## 8. Every spelling of "which words a group is about"

| Spelling | Where |
| --- | --- |
| Keywords (top 10 counted) | `graph/clustering/terms.go:89` |
| Summary's "Key themes" (first five keywords) | `terms.go:119`; graph-query's answer sentence `textsearch.go:216-217` at 8eefecdd (until PR B 2.3) |
| Type counts | `terms.go:84-88` |
| Representatives (smallest text-bearing IDs) | `maintainer.go:276` |

No other component computes words per group: `git grep -l -i keyword -- '*.go' ':!*_test.go'` outside
`graph/clustering`, `graph/query` (the keyword classifier, a different meaning) and graph-query returns only the reply
types (`graph/query_global_search_types.go`, `query_local_search_types.go`) and a classifier comment
(`graph/query_search_types.go:111`).

## 9. Decision records

This change carries no package from the pin; it removes and edits ported code. Records whose decisions it touches:

- ADR-062 (`docs/adr/062-…:181-182`): provenance `embedding` "when seeds came from semantic search"; `:168`
  "Semantic … covers NL".
- ADR-071 (`071-…:60-100`): scope on NL resolve filters "at the candidate source", plumbed to the embedding search.
- ADR-099 (`099-…:9-17`): groups kept from changes, records bounded; keyword rule stated in its port note `:13`.
- ADR-087 (`087-…:72`): `COMMUNITY_INDEX` is detector-exclusive, keywords included; change 3's proposal stops its
  Decision 5 (readiness) applying to search, which leaves keywords descriptive.
- ADR-066 §3 (`066-…:284-295`): graph-embedding's own terminal-outcome watermark (only text-bearing entities are
  embedded), which any embedder question 3 settles keeps.
- ADR-083, ADR-088, ADR-109: readiness per producer; a tier-0 composition without graph-embedding has no
  `graph-embedding` key (composed-stack `:113`), which consumers aggregate.
- ADR-054 (pin, not ported; ledger `:4642`): indexing eligibility for embedding. No other pin record names the
  hashed embedder or the example classifier: pin ADR list filtered for embed, bm25, search, classif, communit,
  summar, fusion, lexical, semantic, query, tier, keyword, rag gives 045, 054, 056, 059-062, 071, 086, 087, 091, 099,
  102, 106, 107; 045 (research-graph, not ported) and 056 (superseded) read, neither governs these packages.

## 10. Intent check (tier membership moves)

| Capability (AGENTS.md "What this is for") | State after this change | Ruling |
| --- | --- | --- |
| Index, query (word search) | admitted, tier 0, the word index | #10 comment 6833147079, items 1-4 |
| Embedding retrieval (`semantic`, `similar`) | moves to tier 1 ("off at tier 0") | same, item 4 |
| Example classifier (tier 1 of the chain) | excluded | same, item 4 |
| Fusion | admitted, tier 0; NL resolve source to be designed | change 2 non-goals (archived proposal) |
| Community grouping | admitted, tier 0 | ADR-099; #483 ruling 2026-10-31 |
| Ingest, vocabulary, provenance, tier ladder, durable execution, rules | unchanged | no ruling needed |

## 11. Adopter seam inventory

Who: (a) a SemSource developer whose docs and code lenses resolve `nl`; (b) a deployment author with
`embedder_type: bm25` (SemSource's default); (c) an agent reading community keywords in a search reply; (d) an agent
reading SemSource's `source_status` or workbench readiness before code search.

1. What they must know today. (a) `nl` answers from graph-embedding through graph-query and is labelled `embedding`;
   (b) `embedder_type` takes only `bm25`; graph-query's `embedding_enabled` must be false when graph-embedding is not
   composed; (c) keywords are a group's commonest words; (d) the `embedding` readiness entry gates code search.
2. If they do nothing. (a) If graph-embedding stops serving at tier 0, `nl` is refused: invalid `not_enabled` with
   `embedding_enabled` false, transient `internal` per request with it left true (section 5). (b) Depends on the
   design: a refused `bm25` fails at boot naming the key (loud); an accepted one that embeds nothing would be silent.
   (c) Keyword meaning changes with no shape change (silent; no reader decides on them after PR B). (d) The agent
   waits on, or trusts, graph-embedding's key while seeds come from the word index, keyed `graph-lexical`: a not-ready
   word index reads as ready, or a disabled embedder reads as no code search (silent).
3. Where they find out. (a) typed runtime error per request; (b) boot error or nowhere; (c) nowhere; (d) nowhere.
4. What they should have to know is the design's question. Recorded gap: (a) predicts which component answers words.

Collision table: not triggered by the inventory; the design triggers it if it adds a stored statistic or a verb.

## 12. Size (lines on base; production / test)

| Package | Today | Removed or changed (measured where removal is whole) |
| --- | --- | --- |
| `graph/embedding` | 2,243 / 7,133 | `bm25_embedder.go` 449 + test 650; comments in `doc.go`, `embedder.go`, `dedup.go:25-41` |
| `graph/query` | 1,807 / 4,050 | `classifier_embedding.go` 275 + tests 548 + 331; chain `:30,37,126-135`; 10 test sites (section 4) |
| processor/graph-embedding | 1,423 / 3,339 | `component.go:87,325`, `config.go:15,24,30-31,69-71,121-122`; tests depend on the design |
| `pkg/fusion/fusionnats` | 574 / 1,694 | `client.go:25,42,100,151-177`; `client_test.go` 30 lines; integration `:116` |
| `pkg/fusion` | 2,273 / 3,963 | `lens.go:79-81,195-204`, `contract.go:60,119-128,180-186`, `retrieval.go:48-73` |
| `graph/clustering` | 872 / 2,290 | `terms.go` 150 + test 230; `maintainer.go:353-379,414` |
| `internal/smoke` | 21 / 2,736 | `checkSearch` 93 lines (`:515-607`), `doc.go`, `noembedder_integration_test.go:181-191` |

Build order, measured from imports: `graph/query` imports `NewBM25Embedder` (`classifier_embedding.go:71`) and
processor/graph-embedding builds it (`component.go:325`), so both change before or with its deletion; smoke's NL check
(`:563-573`) fails if graph-embedding stops serving before fusion's NL source changes.

## 13. Open questions for the design, each with its settling measurement

1. Keywords against the whole graph: the plan asks for "the words distinctive to a group"; the spec says a change
   outside the group never changes its keywords and a tick writes only touched groups (§7). Measured: about half of
   untouched groups change under 5% churn. Settle: the write cost of re-scoring every group per tick at 50,000 and
   1,000,000 entities (groups per level × `MaxRecordBytes`), against keeping the score inside the group's own
   counts or recomputing on a slower cadence; and whether any reader needs keywords current (after PR B, none decides
   on them). Touches ADR-099's port note and an accepted spec requirement: owner question if kept.
2. What `nl` provenance means when seeds come from the word index (ADR-062 honesty envelope; SemSource marshals it,
   §6). Settle: list every reader of `provenance` in SemSource and SemEngine and what each does on `embedding` versus
   `deterministic`; whether a word-index ranking is ingest-order independent (lexical-search spec scorecard laws).
   Owner question: it edits ADR-062's tiers.
3. How graph-embedding runs with no tier-0 embedder (§5): its 33 integration tests need one, and a test-only embedder
   on the component would be a back door. Settle: whether a product-supplied `Embedder` on registration (new framework
   API, owner-approved contract) or #489's HTTP embedder is the first production implementer, and what `bm25` does at
   boot. Owner question where it adds exported surface or pulls 04C work forward.
4. Fusion's NL source: graph-lexical's verb directly (fusionnats looks only at graph-query rows, `client.go:30-55`) or
   a graph-query row. Settle: the fusion spec requirement it changes (`:263-266`), and what fills `unreadable_records`
   and `failed_records` (spec `:68-86`) from a reply that has neither; run fusion's NL over the scorecard corpus against
   the plain reference.
5. Keyword text rule: `entitytext` with which suffixes (graph-clustering has no `text_suffixes`), and whether display
   words are folded index keys. Settle: on SemSource entities, count string values `entitytext` drops.

Searches that closed categories empty: no production `Embedder` besides the hashed one; no production caller of
`NewEmbeddingClassifier` or `LoadDomainExamples`; no consumer Go caller of `semantic`, `similar`,
`graph.embedding.query.*`, `BM25Embedder` or `EmbeddingClassifier`; no producer of group words outside
`graph/clustering`.

Skipped: #660's later tasks (2.3 onward) are not written yet, so §3 rests on change 3's tasks and design. Sister
repositories were read at their local checkouts, read only, with `rg` (no git commands). The keyword probe measures one
corpus with weak grouping; no SemSource data. No pin probe: no pin package is carried.
