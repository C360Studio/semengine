# Inventory: fix-706-fusion-nl-merged-search

base: d4a5c14aaa5d65d46c9512997d9ac806053cb5fd (origin/main b2e581dd plus the claim commit)
Track: design (changes the fusion capability, and graph-query's if searchGraph's request changes).
Pin: carries no package from the pin. No pin probe is run; the SemSource claim is not relied on
(the pin's behaviour is already recorded in-tree, § 3).

## 1. The claimed gap

- `pkg/fusion/fusionnats/client.go:100-101` — `case fusion.ResolveModeNL:` / `return c.resolveWords(ctx, q.Query,
  q.Scope, q.Limit)`
- `pkg/fusion/fusionnats/client.go:161` — `raw, err := c.request(ctx, verb, graph.LexicalSearchRequest{Query: query,
  Limit: limit, Scope: scope})`
- `pkg/fusion/fusionnats/client.go:26` — `entity, batch, prefixIDs, byName, relationships, wordSearch graph.QueryVerb`
  (no searchGraph, semantic or embedding row)
- `pkg/fusion/fusionnats/client.go:43` — `{graph.ResponderGraphLexical, "search", &v.wordSearch},`
- `pkg/fusion/contract.go:61` — `ProvenanceEmbedding     Provenance = "embedding"     // seeds came from semantic
  search`. Outside tests it is set only as the catch-all default, `pkg/fusion/lens.go:210` — `return
  ProvenanceEmbedding`
- Confirmed: no nl path reaches embeddings. Searched: `git grep -n "searchGraph\|semantic\|embedding"
  pkg/fusion/fusionnats/*.go` (non-test) found only doc prose.

## 2. Every current spelling of the facts being modelled

Fact A, "a merged word+vector list":

- `processor/graph-query/merge.go:32` — `func mergeRanks(words []graph.LexicalHit, vectors []graph.SimilarityHit)
  []mergedHit {`. This is the only one: `git grep -n -w -e mergeRanks -e mergeK -- '*.go'` (non-test) finds merge.go and
  its caller only.
- `processor/graph-query/textsearch.go:76-125` — `mergedText`: both searches at `graph.MaxSimilarityLimit` depth, the
  merge, then the cut to the limit.

Fact B, "the composition runs embeddings":

- `processor/graph-query/config.go:33` — `` EmbeddingEnabled bool `json:"embedding_enabled" ...` ``
- `processor/graph-query/query.go:45-46` — the `semantic` and `similar` handlers are `c.notEnabled(verb)` when the key
  is false. The refusal carries code `not_enabled` (`graph/mutation_responses.go:111`).
- `processor/graph-query/query.go:70-73` — `subjects.semantic` is set only when `EmbeddingEnabled`.
- `pkg/fusion/fusionnats/client.go:76` — `func New(nats requester, timeout time.Duration) (*Client, error) {`.
  fusionnats holds no such fact.

Fact C, "where the seeds came from":

- `pkg/fusion/lens.go:203-212` — `ProvenanceForMode`: nl→lexical, symbol/prefix→deterministic, any other→embedding.
- `pkg/fusion/engine_lens.go:123,131,171,185,315` — `ProvenanceForMode(mode)` is set on every response.
  `:295` `deferredResponse` sets `ProvenanceDeterministic`, and the callers then overwrite it.
- `pkg/fusion/retrieval.go:75-80` — `type Resolution struct { Seeds; BodiesNotIndexed; Excluded; DamagedEntities }`. No
  provenance field.
- `graph/query_global_search_types.go:35` — `` Merge *GlobalSearchHint `json:"merge,omitempty"` ``.
  It is honoured with Detail `merged` (`merge.go:18`) or `index_not_ready` (`textsearch.go:253`).
- `graph/query_global_search_types.go:114-115` — `WordRank` / `VectorRank` on each match.

## 3. Adjacent claims

Specs:

- fusion `openspec/specs/fusion/spec.md:243-257` — Subjects come from the verb table (names graph-lexical's `search` row
  for nl).
- fusion `:286-295` — A word search passes on what it could not read.
- fusion `:327-345` — An NL answer reports what the word index left out.
- fusion `:347-355` — An NL answer says the word index found its seeds (`TestProvenanceForMode`).
- fusion `:357-386` — An NL resolve asks the word index (`TestNLResolveAsksTheWordIndex`,
  `TestNLWordIndexRefusalReturned`, `TestNLIndexNotReadyDefers`, `TestNLResolveWithoutWordIndex`).
- fusion `:388-403` — A damaged seed is unhydrated, not a failed read.
- composed-stack `openspec/specs/composed-stack/spec.md:33-34` — the tier 0 smoke expects nl fusion "with provenance
  `lexical`".
- composed-stack `:101-110` — fusionnats imports no `graph/embedding`, `processor/graph-embedding` or `graph/readiness`.
- graph-query `openspec/specs/graph-query/spec.md:528-538` (searchGraph answers as global search when the key is false),
  `:540-570` (failed merge named; word index not ready), `:596-612` (word searches; no word index → transient internal
  naming the subject), `:614-631` (Search requests decode strictly), `:671-708` (matches read back as stored; payload
  fit; damaged hit named), `:727-750` (merged reply declares what it left out), `:868-908` (merge, ranks, both at once).

ADRs (in tree; the change carries no pin package):

- `docs/adr/062-deterministic-graph-fusion.md:28-33` — the port note adding `lexical`; `:188` — "`embedding` when seeds
  came from semantic search".
- `docs/adr/071-fusion-domain-scoped-nl-retrieval.md:16-19` — "scope filters where the seeds are found, now in the word
  index, which applies it before the limit. The answer's provenance is `lexical`."

Prior designs:

- `openspec/changes/archive/2026-11-15-setup-04b-04-removal-keywords/design.md:36-53` (D2): nl moved from graph-query's
  `semantic` to graph-lexical. Rejected there: "a graph-query forward (a new verb beside a row the table already has);
  `globalSearch` (it hydrates and groups; fusion hydrates again)" (`:50-52`).
- `openspec/changes/archive/2026-11-16-setup-04c-02-hybrid-search/proposal.md:36` — Non-goals: "Merging in
  `globalSearch` or `localSearch`".
- Pin behaviour as recorded in the tree (why no probe is needed): `pkg/fusion/MIGRATION.md:77-78` — "SemStreams sent an
  `nl` resolve to the graph's semantic (embedding) search"; `:85` — "An `nl` answer's `provenance` is `lexical`. It was
  `embedding`."

Ledger:

- `docs/admission-ledger.yaml:4817-4893` — the `pkg/fusion/fusionnats` row. Its contract (4) names graph-lexical's
  search for nl; `:4877` — "#563 stays open (fusionnats never calls searchGraph)" (#563 is now CLOSED). The row needs
  editing.
- `docs/admission-ledger.yaml:4512-4524` — the ADR-071 row's known_risks names the nl request.

Open PRs (`gh pr list --state open --json number,title,changedFiles,files`):

- #711 draft — this claim, 0 files.
- #710 draft — `openspec/changes/fix-708-709-damage-test-gaps/{.openspec.yaml,proposal.md,tasks.md}`. No file or
  capability overlap; merge order is free.
- #695–#698 — Dependabot `go.mod`/`go.sum`/`package*.json`. No overlap.

Open issues (`gh issue list --state open --search <fusion|fusionnats|searchGraph|graph-query|provenance>`, hits read):

- #706 — this change; retire with `Closes #706`.
- #659 — fusion's 40-seed batch read fails on `response_too_large`. Interacts: searchGraph also reads matches back with
  entity bytes, and fits to max_payload by dropping its lowest matches (spec graph-query `:690-697`). Stays.
- #621 — docs: pkg/fusion has no repository-map row. Stays (writer).
- #110 — pkg/fusion envelope copy (readiness). Stays; unrelated to the nl path.
- #657 — decision: one rule for how reads report damaged records. Interacts with how damaged hits reach fusion. Stays.
- #682 — tier 1 reranking over hybrid candidates; a later reader of whatever merged list fusion consumes. Stays.
- #195 — natsfixture.Open adoption; fusionnats has already adopted it. Stays.
- Name the word, not the package: #111, #486, #487, #535, #570, #602, #630, #707, #709, #68, #108, #109, #155, #196,
  #300, #350, #24, #26, #704, #477, #564, #667, #536, epics #9 #10 #11 #484.

## 4. Who it is for (no new surface proposed yet)

- Named consumer: SemSource code-context, `processor/code-context/component.go:136` (fusionnats.New, per ledger
  `:4822-4823`). Its `Fuse` call is at `:334`, with scope defaulted at `:326-327` (`req.Scope = c.defaultScope()`). Read
  at semsource d3665365, the local checkout.
- Its test fake builds `Resolution` with named fields (`source/fusion/fusiontest/memgraph.go:77`).
  Adding a field compiles; removing or renaming one breaks it.

## 5. Problem shape

"Read a merged answer, falling back to words alone and saying which one answered." Closest existing instance:
`processor/graph-query/textsearch.go:95-124` with the hint at `merge.go:146-166`. Its consumers read the outcome from
the reply's `merge` hint, not from config. Fusion's existing equivalent for readiness is the same pattern: readiness is
read from each reply (`pkg/fusion/retrieval.go:12-14`, ADR-109).

## 6. Pin decision records

None: the change carries no package from the pin.

## Answers to the brief's questions, as measured

Q1. searchGraph request and reply:

- Request: `graph/query_global_search_types.go:10-15` — `Query`, `Level`, `Limit`, `MinRevisionField`. No `Scope`.
  `processor/graph-query/globalsearch.go:189` — `decodeStrict`, so a `scope` key is refused `invalid_request`.
- Inner searches are unscoped: `textsearch.go:50-52` and `:81-83` build `LexicalSearchRequest` without `Scope`;
  `merge.go:125` — `graph.SemanticRequest{Query: question, Limit: graph.MaxSimilarityLimit}`.
- Both downstream responders already take scope: `processor/graph-lexical/query.go:95-100`,
  `processor/graph-embedding/query.go:91-92`.
- Limit: 0 or 1–100 (`globalsearch.go:199`); fusion sends 40 (`pkg/fusion/engine_lens.go:31`).
- Reply carries:
  - per-match `Score` (the merged RRF score when merged, otherwise the word score), plus `word_rank`/`vector_rank`;
  - `Excluded`, `BodiesNotIndexed` and `Matched` from the word search (`textsearch.go:193-203`);
  - envelope `DamagedEntities` = the largest of word, vector and hint reads (`:113,191,215`);
  - a `Damaged` list of hits left out (spec `:699-708`), where today such a seed reaches fusion's hydration as
    unhydrated `error` (fusion spec `:388-399`);
  - `Merge` with Honoured/Detail.
- Extra work fusion does not need:
  - every match is read back with its entity bytes via batch (`textsearch.go:182`), and fusion reads them again
    (`engine_lens.go:150`);
  - the reply is fit to max_payload by dropping its lowest matches (`textsearch.go:209`);
  - the question is classified; a one-word question is verified as an entity reference, and a path hint walks depth 3 /
    100 nodes (`globalsearch.go:47-48,64-80,210-213`);
  - hits are grouped by community (`textsearch.go:56,208`).
- When the word index is not ready at tier 1, `text` is null and the hits are only in the raw `semantic` bytes, not read
  back (`textsearch.go:252-260`). At tier 0 the `index_not_ready` refusal is returned (spec `:558-570`).

Q2. Graph-query absent, graph-lexical present:

- fusionnats needs graph-query rows for everything but the nl resolve (`client.go:38-42`); hydration is always
  `graph.query.batch` (`client.go:275`).
- Every real-responder fusion test composes graph-query (`pkg/fusion/fusionnats/client_integration_test.go:76-123`), and
  so does the smoke stack (`internal/smoke/stack_integration_test.go:91-92`).
- No test, doc or config runs fusion without graph-query. `pkg/fusion/MIGRATION.md:151` says only that a stack with no
  graph-embedding answers nl when it composes graph-lexical.
- The case that changes is graph-query present with graph-lexical absent. Graph-query's own spec already refuses it
  transient `internal` naming `graph.lexical.query.search` (graph-query spec `:609-612`).
  `TestNLResolveWithoutWordIndex` (`client_integration_test.go:385-412`) asserts the error names that subject and keeps
  `natsclient.IsNoResponders`. The second assertion would no longer hold through a graph-query hop. That is a test and
  spec edit, not a consumer break.

Q3. Provenance today:

- One value per response, from the mode alone (§ 2, Fact C). Deferrals keep the mode's value
  (`TestDeferralProvenanceFollowsMode`, `pkg/fusion/engine_search_counts_test.go:123-133`).
- The values are only deterministic/lexical/embedding/llm (`contract.go:57-63`); there is no "merged" value.
- Readers: tests (`lens_test.go:156`, `engine_search_counts_test.go:89,132-133`, `fusionnats/client_test.go:446`,
  `internal/smoke/smoke_integration_test.go:550`). SemSource production code reads no `Provenance`
  (`git grep -n Provenance` in processor/code-context, processor/mcp-gateway and source/fusion: only manifest actor
  provenance). The value reaches its MCP clients on the wire.

Q4. Capabilities touched:

- fusion, for certain: `:243-257`, `:286-295`, `:347-355`, `:357-386`, and `:388-403` if damaged hits move.
- composed-stack `:33-34`, whose wording must still hold at tier 0.
- graph-query if its request or reply changes: `:614-631`, `:596-612`, `:671-708`.
- graph-transport-boundary `:38-40` only if a verb is added.

Q5. Existing tests that pin today's behaviour:

- `fusionnats/client_test.go:123` `TestSubjectsComeFromVerbTable`, `:178` `TestVerbLookupNamesMissingRow`,
  `:297` `TestNLResolveAsksTheWordIndex`, `:357` `TestWordIndexCountsReachResolution`.
- `lens_test.go` `TestProvenanceForMode`; smoke `:550`; graph-query `searchgraph_merge_integration_test.go:250-797` (the
  merge's own proofs).

## Collision table

Not triggered. No durable, communication or coordination primitive is proposed in the inventory phase. A new verb or
request key, if the design chooses one, triggers it then.

## Intent check

Not triggered. No boundary, tier or capability moves; both search halves are already admitted at tier 0 and tier 1.

## Adopter seam (SemSource code-context; any product on fusionnats)

1. What they must know today:
   - that fusion's nl answers ignore embeddings at tier 1;
   - that `graph.query.searchGraph` is the only merged surface;
   - that `provenance: lexical` is all they will ever see.
2. If they do nothing: a tier 1 composition (graph-embedding plus `embedding_enabled: true`) answers fusion nl questions
   from words alone, labelled `lexical`. No error, log or counter says embeddings were skipped. It is silent, and it is
   the path #706 reproduces.
3. Where they find out: nowhere. They find out only by reading the wire (#706's own reproduction).
4. What they should have to know: nothing. Whether vectors were merged is a fact graph-query already observes and
   reports (`merge` hint); fusionnats has no config for it (`client.go:76`). A knob on fusionnats would make the product
   predict a fact the framework observes.

## Open evidence questions for the reviewer

- Is ADR-071's "before the limit" binding on a merged list, so that both inner searches, not the merge output, must be
  scoped?
- Is a merged list's provenance `embedding` under ADR-062 `:188` ("seeds came from semantic search"), given that some
  seeds came from words alone?

## Additions from the inventory review

Owner's words on #706 (2026-11-21): "Fusion's natural-language resolve uses the merged search when the composition runs
embeddings, and the seeds say where they came from." Whether that means one provenance per response or one per seed is
left to the design and the owner.

Score readers (a wire field changes meaning):

- `Seed.Similarity` (`pkg/fusion/retrieval.go:57-59`) and the wire field `Node.Similarity`
  (`pkg/fusion/contract.go:120-122`) are documented as the word index's score for an nl resolve. A merged searchGraph
  match carries the RRF score instead (`graph/query_global_search_types.go:105-106`).
- The engine ranks by resolve order, not by score (`pkg/fusion/engine_lens.go:35-47, 258`), so ranking is unaffected;
  consumers that set `IncludeScores` read a different number.
- SemSource keeps `Similarity` only in its symbol-mode wrapper `exactSeedClient` (`processor/code-context/exact_seed.go:28-33`).

Ledger rows also touched:

- `pkg/fusion` row (`docs/admission-ledger.yaml:4695`; text at `:4712`, `:4732`, `:4762`) and the ADR-062 row
  (`:4794`) say an nl answer's provenance is `lexical`.
- `processor/graph-query` row (`:4369`) still describes searchGraph as fallback-only, "when word search matches nothing"
  (`:4419-4421`, `:4436-4437`), stale since the merge landed. The design fixes it or files it.

SemSource code on the nl path (read-only, local checkout):

- `nlDisabledClient` (`processor/code-context/nl_disabled.go:31-45`) refuses nl itself at tier 0, which composes graph-query
  without graph-lexical (`configs/tiers/README.md:39-40`). Its comment names `graph.lexical.query.search`.
- `http_error.go:69-90` maps any transient classified error to 503, so a graph-query hop does not break it; it maps an
  invalid `not_enabled` to 501 "semantic search is not enabled", which matters if an option routes through graph-query's
  `semantic` verb at tier 0.
- SemSource always sends a scope (`component.go:326-327`), so a scope-less searchGraph cannot serve its nl resolves.

graph-query spec requirements the fallback leans on:

- `openspec/specs/graph-query/spec.md:752-788` (every question gets a word search; the word search comes before any hint;
  a refused word search cancels the embedding search), `:813-821` (the reply's revision is the word index's), `:854-866`
  (losing the embedding provider keeps word search).
- fusion `:405-415`, every read passes on the damaged count.

Line corrections: `globalsearch.go:199` is `:198`; the SemSource fake builds `Resolution` at `memgraph.go:86`.

Answers to the open evidence questions:

- Scope reaches both inner searches. ADR-071 picks filtering at the candidate source (`:80-96`) and rejects a post-filter
  because it "cannot recover a domain that falls *entirely* below the over-fetch window" (`:130-140`); resolved
  decision (c) rules out a second filter path (`:204-207`). Scoping the merge's output is the rejected option.
  Both responders already scope: graph-lexical via `Options.Scope` (`processor/graph-lexical/query.go:95-100`),
  graph-embedding via its `keep` filter (`processor/graph-embedding/query.go:91-92`).
- Provenance of a merged answer is not decided by the tree. Response-level leans conservative: `contract.go:57` orders the
  values by increasing uncertainty; `lens.go:196-199` gives a seed that "has not earned the stronger claim" the more
  conservative value; ADR-062's note (`:28-33`) defines `lexical` as neither exact lookup nor semantic search. That reading
  gives `embedding` when the `merge` hint is honoured, `lexical` otherwise. Per-seed provenance is measurable from each
  match's `word_rank`/`vector_rank` (`graph/query_global_search_types.go:114-115`).
