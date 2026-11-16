# Inventory: setup-04b-01-lexical-index

base: `bb045d52` (`main` at `2853486d` plus the proposal commit). Inventory only: no target state, options or
recommendation. Each pin is `` `path:line` — `text` ``, read in this run. Terms: a *tokenizer* turns text into the
words an index keys on; a *posting* is one (word, entity) pair in the index; a *forward list* is the set of words
one entity holds, kept so that the entity can be replaced or deleted; *corpus statistics* are the entity count,
each word's entity count and the average length that BM25 reads.

## 1. The claimed gap

No word index exists. `graph/lexical` is absent. `git grep -n -i -E 'inverted|postings?|idf\('` over Go files gives
8 hits: `graph/embedding/bm25_embedder.go:406` — `// BM25(t,d) = IDF(t) * (tf * (k1 + 1)) / ...` (a comment) and
seven unrelated uses of "inverted" (`natsclient/storagereport/storage_pressure.go:169`,
`processor/graph-ingest/query.go:126` and five test files). `go.mod` names no search library
(`bleve|tantivy|meilisearch|sqlite`, empty). `go.mod:28` — `pgregory.net/rapid v1.3.0` is already present.

## 2. Every current spelling of the fact

**Tokenizers and stopword lists (three, none shared):**

- `graph/embedding/bm25_embedder.go:242` — `func (b *BM25Embedder) tokenize(text string) []string {`: Unicode
  letters and digits, 2+ bytes, stopwords, then `:278` — `func stem(word string) string {`. Stopwords `:18` —
  `var stopwords = map[string]bool{`, 84 words (`:18-41`).
- `graph/clustering/terms.go:28` — `func extractWords(text string) []string {`: ASCII letters and digits, 3 to 32
  bytes (`:13` — `const minWordBytes = 3`; `graph/clustering/types.go:13` — `MaxWordBytes = 32`), longer runs
  dropped, no stemming. Stopwords `terms.go:16` — `var stopWords = map[string]bool{`, 30 words; 29 are in the
  embedder's list, `as` is not.
- `processor/graph-query/localsearch.go:60` — `func searchTerms(query string) []string {`: lower-case, split on
  white space, repeats removed; more than `graph/query_local_search_types.go:7` — `const MaxSearchTerms = 16`
  distinct terms is refused, not cut (`localsearch.go:51` — `if len(terms) > graph.MaxSearchTerms {`). No
  punctuation stripping, no stopwords. Callers: `localsearch.go:37` and `processor/graph-query/textsearch.go:53`,
  both `terms, err := checkedSearchTerms(request.Query)`.
- Not tokenizers: `processor/graph-index/name_index.go:27` — `return strings.ToLower(strings.TrimSpace(name))`
  (whole-name key) and the classifier's regular expressions from `graph/query/classifier.go:34`.

**Corpus statistics (one spelling, in the embedder):** fields `bm25_embedder.go:66-71` (`:68` — `docCount int`,
`:70` — `termDocCount   map[string]int // Number of documents containing each term`); learned only by
`Generate` through `:351` — `func (b *BM25Embedder) updateStats(tokens []string) {`, never subtracted (no decrement
of `docCount`, `termDocCount` or `totalDocLength` anywhere: `git grep -n -w termDocCount`, 5 production hits, one
`++`), so they start again empty on restart; `GenerateQuery` only reads them (`:109-123`). A word in no document
counts as one (`:395-396` — `if df == 0 {` / `df = 1 // Smoothing for unseen terms`); IDF is floored (`:400` —
`if idf < 0.01 {`); `:95` — `cfg.K1 = 1.5 // Standard BM25 default`, `:98` —
`cfg.B = 0.75 // Standard BM25 default`.

**Scorers (four, none BM25 over an index):**

- `BM25Embedder` hashes words into slots (`bm25_embedder.go:429` — `func (b *BM25Embedder) hashTerm(term string)
  int {`), 384 by default, ranked by cosine in `graph/embedding/vector_cache.go:105` — `func (c *VectorCache)
  FindSimilar(...`. Constructed at `processor/graph-embedding/component.go:321` — `c.embedder =
  embedding.NewBM25Embedder(embedding.BM25Config{})` and `graph/query/classifier_embedding.go:71` — `bm25 :=
  embedding.NewBM25Embedder(embedding.BM25Config{`, whose `NewEmbeddingClassifier` (`:47`) has no production caller.
- `processor/graph-query/localsearch.go:95` — `func (e searchEntity) score(terms []string) float64 {`: 1.0 for the
  ID's type, 0.5 for a substring of any predicate or string value; `:21` — `const minLocalSearchScore = 0.3`.
- `processor/graph-query/textsearch.go:138` — `func groupScore(record clustering.Community, terms []string)
  float64 {`: substring tests on a community's summary and keywords; caller `:121`.
- `pkg/fusion/engine_lens.go:617` — `func lexicalScore(q, name string) float64 {`: the whole query against one
  display name (exact, prefix, contains), not word-based.

**Search verbs, their bounds and the embedder setting:**

- `graph/query_verbs.go:88` — `{Name: "search", Subject: "graph.embedding.query.search", ...` and `:122` —
  `{Name: "semantic", Subject: "graph.query.semantic", ...`, forwarded by `processor/graph-query/query.go:33` —
  `"semantic":  {graph.ResponderGraphEmbedding, "search"},`. Outside caller: `pkg/fusion/fusionnats/client.go:153`.
- Entity to entity: `query_verbs.go:90` — `{Name: "similar", Subject: "graph.embedding.query.similar", ...` and
  `:124` — `{Name: "similar", Subject: "graph.query.similar", ...`, served at `processor/graph-embedding/query.go:25`
  — `"similar": c.handleSimilar,`; spec `graph-embedding` "Similar needs a generated source vector" (`:155`). The
  embedder is its only tier 0 answerer.
- `graph/query_search_types.go:33` — `type SemanticRequest struct {` (query, limit, scope); `:55` — `type
  SemanticData struct {` (entity ID and similarity per hit, unreadable and failed counts) with `:57` —
  `` EmbedderType      string          `json:"embedder_type,omitempty"` `` naming the embedder that answered.
- Config: `processor/graph-embedding/config.go:35` — `` EmbedderType string `json:"embedder_type" ... ``;
  anything but `bm25` is refused at tier 0 (`:73` — `if c.EmbedderType != embedderBM25 {`).
- Limit: `query_search_types.go:24` — `MaxSimilarityLimit = 100`; 0 means 10; out of range is refused, never
  clamped (`processor/graph-embedding/query.go:215` — `func checkCommon(minRevision uint64, limit int) (int, error)
  {`).
- Scope: each prefix checked at entry (`query.go:76` — `if err := semtypes.ValidateEntityIDPrefix(prefix); err !=
  nil {`), matched on a dot boundary (`graph/id_prefix.go:26` — `func MatchesAnyIDPrefix(id string, prefixes
  []string) bool {`), applied before sort and limit (`vector_cache.go:128` — `if len(results) > limit {`); ties by
  ID (`graph/embedding/storage.go:132` — `return a.EntityID < b.EntityID`).
- Local search: one group at a level (`graph/query_local_search_types.go:11` — `// EntityID is a literal entity
  ID; the search runs in its group at Level, ...`), one page of it. Global and `searchGraph`: at most five groups
  (`textsearch.go:18` — `const maxTextGroups = 5`). Both replies say when matches are a sample:
  `query_local_search_types.go:35` and `graph/query_global_search_types.go:49` — `EntitiesTruncated bool ...`.

## 3. What the text of an entity is today

- Text: string values whose predicate ends in a configured suffix, relationships skipped, joined by spaces
  (`processor/graph-embedding/hop1.go:228` — `func (c *Component) extractText(state *graph.EntityState) string {`);
  nine default suffixes, `.name` and `.title` among them (`config.go:27` — `var defaultTextSuffixes =
  []string{".title", ...`). Cap: `config.go:23` — `const maxSourceTextLenBM25 = 4000` characters, cut at a word
  (`graph/embedding/worker.go:701` — `func truncateAtWord(text string, maxLen int) string {`). The probe corpus
  averaged 123 tokens a section (`notes/probes/retrieval-probe.log:3`); real entity sizes are not measured.
- Offloaded bodies: `graph/types.go:35` — `StorageRef *message.StorageReference ...`. With the store registered,
  hop 1 keeps only identity text (`hop1.go:159` — `record.IdentityText = c.extractText(&state)`); the worker
  reads the body at embed time (`worker.go:134` — `Streamable(instance string) (storage.StreamableStore, bool)`,
  read capped at `:769`) and joins them (`:681` — `text = record.IdentityText + identityBodySeparator + body`).
  With it unregistered, inline text only, counted (`hop1.go:161` — `if state.StorageRef != nil {`, `:162` —
  `c.reportOffloadedBodyExcluded(key, state.StorageRef.StorageInstance)`).
- Names, three spellings. (1) Vocabulary label predicates (`vocabulary/registry.go:477` — `func
  DiscoverLabelPredicates() map[string]int {`), of which only `vocabulary/labels.go:17` — `Register(DCTermsTitle,`
  is registered; `:28` — `labelPriorityPreferred = 0` and `:30` — `labelPriorityAlternate = 2` have no reader.
  Read by graph-query (`processor/graph-query/component.go:185` — `c.labelPredicates =
  labelOrder(vocabulary.DiscoverLabelPredicates())`, applied by `localsearch.go:129` — `func (e searchEntity)
  label(predicates []string) string {`, spec `graph-query` "A match's label is its most salient label predicate",
  `:791`) and graph-index (`processor/graph-index/index.go:88` — `c.namePredicates =
  vocabulary.DiscoverLabelPredicates()`). (2) The suffixes `.name`, `.title`. (3) Fusion's `pkg/fusion/lens.go:155`
  — `Label(e *Entity) string`, supplied by the product.

## 4. Entities with no text, and how they are read today

- An entity whose `extractText` is empty, with no registered offloaded body, has no embedding record: `hop1.go:165`
  — `if record.SourceText == "" {` calls `:166` — `c.removeTextless(ctx, key, revision)`, which deletes any earlier
  record and completes the revision (`:195-196`). It is never a `search` or `similar` hit, and as the source of a
  `similar` request it is refused with code `embedding_unavailable` (`processor/graph-embedding/query.go:136` —
  `return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEmbeddingUnavailable,`; spec
  `graph-embedding` `:157-158`). Community keywords take words only from string values
  (`graph/clustering/terms.go:55` — `// come only from string values; the ID's positions, predicate names and the
  object of a`), so it adds no keywords.
- Its groups need no text: `graph/clustering/doc.go:6-7` — `org.platform (ADR-099, with the levels ADR-102 gives).
  Nothing is detected: an entity's` / `groups follow from its ID alone.`
- Read paths that need no text. `grep -c -E 'Name: +"' graph/query_verbs.go` gives 32 verbs: seven text or vector
  verbs (`search` and `similar` on graph-embedding, `semantic`, `similar`, `localSearch`, `globalSearch`,
  `searchGraph` on graph-query), the four spatial and temporal verbs below, and these 21, which read structure:
  - graph-ingest: entity (`:47` — `Name: "entity", Subject: "graph.ingest.query.entity", ...`), batch (`:68` —
    `{Name: "batch", Subject: "graph.ingest.query.batch", ...`), prefix (`:70`).
  - graph-index: outgoing (`:72`) and incoming (`:74`) relationships, alias (`:76`), predicate (`:78`), predicateList
    (`:80`), predicateStats (`:82`), predicateCompound (`:84`), byName (`:86`, label predicates only). graph-index
    records every predicate an entity holds (`processor/graph-index/index.go:310` —
    `plan.predicatesUsed[triple.Predicate] = true`); the predicate verb's value filter compares strings exactly
    (`processor/graph-index/query.go:366` — `if triple.Predicate == predicate && objectString(triple.Object) ==
    value {`).
  - graph-query: entity, batch, prefix, byName, predicate, entityByAlias, relationships (`:96-108`), pathSearch
    (`:110` — `{Name: "pathSearch", Subject: "graph.query.pathSearch", ...`), hierarchyStats (`:112`), summary
    (`:114`).
- Declared but served by nothing in the tree: `:92` — `{Name: "bounds", Subject: "graph.spatial.query.bounds", ...`
  and `:94` — `{Name: "range", Subject: "graph.temporal.query.range", ...` (`:57` — `// (search, similar, bounds,
  range), of which only search and similar have a responder in`); graph-query still forwards to them
  (`processor/graph-query/query.go:35-36`), and its own `spatial` and `temporal` verbs (`:126`, `:128`) answer
  only through them. `ls processor` lists no spatial or temporal component.
- When they get responders: the ledger says "their responders arrive in changes 5 and 7"
  (`docs/admission-ledger.yaml:2165` — `graph.temporal.query.range; their responders arrive in changes 5 and 7)
  ...`); search and similar arrived in change 5, and `processor/graph-index-spatial` and
  `processor/graph-index-temporal` are in change 7's list (`openspec/changes/archive/2026-10-01-setup-04a-foundation/
  design.md:176-177`). Open issue #109 says "spatial/temporal become plug-ins" on the shared `ENTITY_STATES`
  follower, landing in changes 4, 5 and 7. No ledger row exists for either package yet.

## 5. Adjacent claims

- SemSource's ask (#10 comment 6692832477), the request the proposal cites. R1: the top-k and its order identical
  across ingest order and a restart mid-ingest, checked rank for rank on its 33 questions. R2: a quoted phrase
  that keeps a flag (`"docker compose down -v"`) and a dotted predicate (`"entity.lifecycle.stale"`) whole. R3:
  exclusion that removes rather than demotes: `retry backoff -test` must return no hit matching `_test.go`
  (`expect_none`), which names a file, not a word in the body. It offers its corpus (21.6k entities), runner and
  graded baseline and its question shape (`scripts/scorecard/questions.json` in SemSource), and says: "What we are
  not asking for: a new query language, aggregations, or anything beyond term-level semantics over the bodies we
  already emit."
- Issues (`gh issue list --state open --search` on lexical, bm25, tokenizer, stopword, "text search",
  BM25Embedder, keywords, phrase, then read): #10; #616, #535 (router), #506 (community search) and #563
  (`searchGraph` falls back to the embedding `search` verb; SemSource's `graph_search` reports which answered,
  `semsource:processor/mcp-gateway/disclosure.go:29-32` per the issue) for change 3; #108 (graph-index's rebuild on
  restart is unmeasured; change 2 rebuilds the same way); #551 (the offloaded-body fetch has no deadline of its
  own); #477 (`pkg/graphview` adoption); #489 (tier 1 and its fallback test). Word only: #9, #111, #350,
  #485-#487.
- Spec requirements later changes meet: `graph-query` "Local search ranks by the pin's text score" (`:451`),
  "Local search refuses what it cannot answer" (`:493`), "Truncated searches are counted" (`:562`), "Global search
  answers by its hints" (`:573`), "A match's label is its most salient label predicate" (`:791`), "searchGraph
  answers as global search" (`:858`); `graph-embedding` "Tier-0 config" (`:54`), "Search checks its query and
  scope first" (`:138`), "Similar needs a generated source vector" (`:155`); `fusion` "A word-search answer reports
  what it could not read" (`:67`), "A word search passes on what it could not read" (`:304`); the purposes of
  `embedding-index` and `query-classification`. `docs/setup-plan.md:425` — `simultaneous BM25/neural operation.
  Hybrid retrieval needs separate design. ...`.
- Open pull requests (`gh pr list --state open`): #615 (this change); #611 (`service/**`) and #612
  (`processor/graph-ingest/**`) touch no file in the four-change plan; #597, #598 touch `go.mod`/`go.sum` only.
  Change 1 adds files and edits `docs/repository-map.md` (`proposal.md:52`), likely also `scripts/cover-check.sh`;
  no open pull request touches them. Later overlap: #606 checklist lines not yet opened, 5
  (`processor/graph-query`), 6 (`graph/embedding`, graph-embedding), 8 (`graph/query`, `graph/clustering`).

## 6. Rules a new package meets

- Guards: harness-boundaries "Import graph" (no `testing`, testcontainers or `internal/harness` in production
  files), "No sleeps in tests", "No skipped or hidden tests", "No own deadlines in tests", "No reach check after a
  generated run", "Public signatures name no internal type". The graph root's NATS guard covers `./graph` only
  (`internal/harness/contract/graphtransport_test.go:18`); a `go list -deps` test reads the real tree, so it lives
  in `internal/harness/contract` (merge-gate "Tool tests and repository guards are separate packages").
- Time: unit tests run once under `-race` and five times shuffled at one CPU, each under `-timeout 5m` per package
  (`openspec/specs/merge-gate/spec.md:15-19`); that bounds the scorecard corpus and the Rapid runs.
- Coverage: `task cover:check`, 80% on each `scripts/cover-check.sh` target; a new package is not on it until
  added (`:60` — `"graph/clustering unit"` is the form).
- Oracles: tests use an independent oracle and are shown able to fail (`AGENTS.md:124`).
- Fuzzing: code that parses outside strings gets a native fuzz target (`docs/testing.md:415` — `Use a native Go
  fuzz target ... for code that parses, decodes or validates outside bytes or`); the query parser and tokenizer do.
- Generated checks: `docs/testing.md:438` — `### Decide whether generated checks are needed`, `:452` — `### Show
  that each assertion runs`, `:472` — `### Check a history against a reference model`, `:481` — `### What a
  generated run leaves behind`. `task mutate:check` refuses to start while an untracked file sits under the
  package's `testdata/rapid/` (`:371`).
- Test data: package `testdata/` (`pkg/types/testdata/rapid`, `service/testdata`). A scorecard corpus read from the
  tree's Markdown, as the probes did, would read other real-tree files.
- Ledger: rows are for SemStreams sources (harness-boundaries "Admission ledger"). Packages with no row as a
  destination: `internal/httpadmission`, `internal/keycase`, `internal/kvwatch`, `internal/lifecycleguard`,
  `internal/smoke` (every `go list ./...` package checked against each row's `destination`). `pkg/graphview` and
  `graph/follower` have pin sources. Admission: `AGENTS.md:15`; tier 0, owner ruling on #10 (comment 6833147079).
- No pin package is carried: no pin decision records, shape sweep or pin probe apply.

## 7. Problem shape, and the adopter seam

Shape: an in-memory projection of `ENTITY_STATES`, written entity by entity, read by scoped, limited, ranked
queries. Closest: `vector_cache.go:105` on `pkg/graphview`; change 2 would be the follower's fourth plug-in
(`processor/graph-index/component.go:236`, `processor/graph-embedding/component.go:256`,
`processor/graph-clustering/component.go:192` — `f, err := follower.Open(ctx, follower.Config{`). The collision
table is not triggered by change 1 (no durable, transport or coordination primitive); change 2 triggers one.

Callers: change 2 (write, replace, delete, search, rebuild on start); change 3 (search under a scope, hits grouped
by community); change 4 (community keywords: the tokenizer and word counts); SemSource through the verbs, holding
R1 to R3; tier 1 (#489, vectors merged by rank). The path is public, so a product may import it (contract line and
product test).

| Later need | What it asks of this package |
| --- | --- |
| Per-entity token record | A write that takes already-split words, or tokenizer output a store can keep |
| Word positions | Postings whose layout nothing outside reads |
| Typo tolerance, query forms | A parsed query value; an enumerable word list |
| Vectors by rank | A total order (score, then ID) and the match count before the limit |
| Change 2 serving while writing | A stated rule for a search that runs during a write |
| Entity-to-entity similarity (`similar`) | An entity's own words usable as a query, or the verb's fate settled in change 4 |
| Scope and truncation counts | Scope before limit; what was matched, read and left out, for `EntitiesTruncated` and fusion |
| The `embedder_type` key and reply field | Nothing of the package; changes 2 and 4 decide what `bm25` names afterwards |
| R2 dotted names and flags, R3 path exclusion | Text the tokenizer keeps for phrases; which fields an excluded word is tested on |

Tokenizer adopters: `graph/clustering/terms.go:28` (change 4), `processor/graph-query/localsearch.go:60` (change 3);
`BM25Embedder.tokenize` goes in change 4. Not adopters: graph-index's name key, fusion's `lexicalScore`.
What an adopter must know today: which predicates are text (graph-embedding config), which are names (three
spellings), that offloaded bodies are fetched by the caller, and that a gone entity needs a delete. Doing nothing:
an offloaded entity is indexed on its identity text alone; only `dc.terms.title` counts as a name.

## 8. Measurements (observed; Go 1.26.4, Apple M3 Pro, 36 GB)

`notes/probes/memory-probe.go.txt`, run outside the tree; output `notes/probes/memory-probe.log`. It builds a
word-to-postings map (entity number and count), a forward list per entity, 57-byte entity IDs, and words drawn Zipf
(s 1.1) from 500,000; positions as one `[]uint16` per posting. Heap after two GCs. Build includes generating text.

| Tokens per entity | Positions | Entities | Heap | Per entity | Build |
| --- | --- | --- | --- | --- | --- |
| 20 | no | 1,000,000 | 471 MB | 471 B | 3.6 s |
| 20 | yes | 1,000,000 | 1,187 MB | 1,186 B | 4.5 s |
| 120 | no | 1,000,000 | 1,496 MB | 1,495 B | 27.8 s |
| 120 | yes | 300,000 | 1,572 MB | 5,239 B | 7.2 s |

The last row at one million is extrapolated, about 5.2 GB (5,473 B per entity at 100,000). Not measured: tokenizing;
the word strings' own bytes, allocated before the baseline (`memory-probe.go.txt:43-47`); query time; the cost of
replacing or deleting an entity; a flat position layout instead of one 24-byte slice header per posting.

## 9. Open evidence questions

- Share of SemSource entities with an offloaded body: not measured. `docs/inventory-scope.md:14` allows the
  qualification corpus `test/setup03a`; at SemSource `main` `4ca2072c` its `README.md:74` lists four functions and
  one passage, too few. The 21.6k-entity corpus SemSource offered on #10 would answer it.
- Tokens per real entity, inline and with bodies up to the 4,000-character cap.
- Query time, replace and delete cost, and memory with real tokenizing, at one million entities.
