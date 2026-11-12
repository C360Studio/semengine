# Inventory: setup-04b-01-lexical-index

base: `bb045d52` (`main` at `2853486d` plus the proposal commit). Inventory only: no target state, options or
recommendation. Each pin is `` `path:line` — `text` ``, read in this run. Terms: a *tokenizer* turns text into the
words an index keys on; a *posting* is one (word, entity) pair in the index; a *forward list* is the set of words
one entity holds, kept so that the entity can be replaced or deleted.

## 1. The claimed gap

No word index exists: `graph/lexical` is absent; `git grep -n -i -E 'inverted|posting(s)?\b|idf\('` over Go files
finds only the embedder's corpus counter (`graph/embedding/bm25_embedder.go:70` — `termDocCount   map[string]int
// Number of documents containing each term`); `go.mod` names no search library (`bleve|tantivy|meilisearch|sqlite`,
empty). `go.mod:28` — `pgregory.net/rapid v1.3.0` is already present.

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
  white space, repeats removed, at most `graph/query_local_search_types.go:7` — `const MaxSearchTerms = 16`; no
  punctuation stripping, no stopwords. Callers: `localsearch.go:37` and `textsearch.go:53` via `checkedSearchTerms`.
- Not tokenizers: `processor/graph-index/name_index.go:27` — `return strings.ToLower(strings.TrimSpace(name))`
  (whole-name key) and the classifier's regular expressions from `graph/query/classifier.go:34`.

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

**Search verbs and their bounds:**

- `graph/query_verbs.go:88` — `{Name: "search", Subject: "graph.embedding.query.search", ...` and `:122` —
  `{Name: "semantic", Subject: "graph.query.semantic", ...`, forwarded by `processor/graph-query/query.go:33` —
  `"semantic":  {graph.ResponderGraphEmbedding, "search"},`. Outside caller: `pkg/fusion/fusionnats/client.go:153`.
- `graph/query_search_types.go:33` — `type SemanticRequest struct {` (query, limit, scope); `:55` — `type
  SemanticData struct {` (entity ID and similarity per hit, unreadable and failed counts).
- Limit: `:24` — `MaxSimilarityLimit = 100`; 0 means 10; out of range is refused, never clamped
  (`processor/graph-embedding/query.go:215` — `func checkCommon(minRevision uint64, limit int) (int, error) {`).
- Scope: each prefix checked at entry (`query.go:76` — `if err := semtypes.ValidateEntityIDPrefix(prefix); err !=
  nil {`), matched on a dot boundary (`graph/id_prefix.go:26` — `func MatchesAnyIDPrefix(id string, prefixes
  []string) bool {`), applied before sort and limit (`vector_cache.go:128` — `if len(results) > limit {`); ties by
  ID (`graph/embedding/storage.go:132` — `return a.EntityID < b.EntityID`).

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
  With it unregistered, inline text only, counted (`hop1.go:161-162`).
- Names, three spellings: vocabulary label predicates (`vocabulary/registry.go:477` — `func
  DiscoverLabelPredicates() map[string]int {`), of which only `vocabulary/labels.go:17` — `Register(DCTermsTitle,`
  is registered (`labelPriorityPreferred`, `labelPriorityAlternate` at `:28`, `:30` have no reader), read by
  graph-query (`component.go:185`, ordered by `textsearch.go:157` — `func labelOrder(...`) and graph-index
  (`index.go:88`); the suffixes `.name`, `.title`; fusion's `pkg/fusion/lens.go:155` — `Label(e *Entity) string`.

## 4. Adjacent claims

- Issues (`gh issue list --state open --search` on lexical, bm25, tokenizer, stopword, "text search",
  BM25Embedder, keywords, phrase): #10; #616, #535 (router) and #506 (community search) for change 3; #489 (tier 1
  and its fallback test, which after change 4 falls back to this index). Word only: #9, #111, #350, #485-#487.
- Specs changes 3 and 4 amend: `graph-query` "Local search ranks by the pin's text score", "Global search answers
  by its hints"; `graph-embedding` "Tier-0 config"; `embedding-index` and `query-classification` purposes.
  `docs/setup-plan.md:425` — `simultaneous BM25/neural operation. Hybrid retrieval needs separate design. ...`.
- Open pull requests: #611 (`service/**`) and #612 (`processor/graph-ingest/**`) touch no file in the four-change
  plan; #597, #598 touch `go.mod`/`go.sum` only. The overlap is #606 checklist lines not yet opened: 5
  (`processor/graph-query`), 6 (`graph/embedding`, graph-embedding), 8 (`graph/query`, `graph/clustering`). Change 1
  adds files only and conflicts with none.

## 5. Rules a new package meets

- Guards: harness-boundaries "Import graph" (no `testing`, testcontainers or `internal/harness` in production
  files), "No sleeps in tests", "No skipped or hidden tests", "No own deadlines in tests", "No reach check after a
  generated run", "Public signatures name no internal type". The graph root's NATS guard covers `./graph` only
  (`internal/harness/contract/graphtransport_test.go:18`); a `go list -deps` test reads the real tree, so it lives
  in `internal/harness/contract` (merge-gate "Tool tests and repository guards are separate packages").
- Coverage: `task cover:check`, 80% on each `scripts/cover-check.sh` target; a new package is not on it until
  added (`:60` — `"graph/clustering unit"` is the form).
- Generated checks (`docs/testing.md:438`, `:452`, `:472`, `:481`): record the decision; named witnesses before
  `rapid.Check`; histories against a model the test owns; seed, completed checks and a replay command recorded.
- Test data: package `testdata/` (`pkg/types/testdata/rapid`, `service/testdata`). A scorecard corpus read from the
  tree's Markdown, as the probes did, would read other real-tree files.
- Ledger: rows are for SemStreams sources (harness-boundaries "Admission ledger"). Packages with none have no row:
  `internal/keycase`, `internal/smoke` (every `go list ./...` package checked against ledger destinations).
  `pkg/graphview`, `graph/follower` have pin sources. Admission: `AGENTS.md:15`, tier 0 under epic #10's ruling.
- No pin package is carried: no pin decision records, shape sweep or pin probe apply.

## 6. Problem shape, and the adopter seam

Shape: an in-memory projection of `ENTITY_STATES`, written entity by entity, read by scoped, limited, ranked
queries. Closest: `vector_cache.go:105` on `pkg/graphview`; change 2 would be the follower's fourth plug-in
(`processor/graph-index/component.go:236`, `processor/graph-embedding/component.go:256`,
`processor/graph-clustering/component.go:192` — `f, err := follower.Open(ctx, follower.Config{`). The collision
table is not triggered by change 1 (no durable, transport or coordination primitive); change 2 triggers one.

Callers: change 2 (write, replace, delete, search, rebuild on start); change 3 (search under a scope, hits grouped
by community); change 4 (community keywords: the tokenizer and word counts); tier 1 (#489, vectors merged by rank).
The path is public, so a product may import it (category 4: contract line and product test).

| Later need | What it asks of this package |
| --- | --- |
| Per-entity token record | A write that takes already-split words, or tokenizer output a store can keep |
| Word positions | Postings whose layout nothing outside reads |
| Typo tolerance, query forms | A parsed query value; an enumerable word list |
| Vectors by rank | A total order (score, then ID) and the match count before the limit |
| Change 2 serving while writing | A stated rule for a search that runs during a write |

Tokenizer adopters: `graph/clustering/terms.go:28` (change 4), `processor/graph-query/localsearch.go:60` (change 3);
`BM25Embedder.tokenize` goes in change 4. Not adopters: graph-index's name key, fusion's `lexicalScore`.
What an adopter must know today: which predicates are text (graph-embedding config), which are names (three
spellings), that offloaded bodies are fetched by the caller, and that a gone entity needs a delete. Doing nothing:
an offloaded entity is indexed on its identity text alone; only `dc.terms.title` counts as a name.

## 7. Measurements (observed; Go 1.26.4, Apple M3 Pro, 36 GB)

Throwaway program outside the tree (`/tmp/lexmem/main.go`, this host only): word-to-postings map (entity number
and count), a forward list per entity, 57-byte entity IDs, words drawn Zipf (s 1.1) from 500,000; positions as one
`[]uint16` per posting. Heap after two GCs, word table excluded. Build includes generating text, not tokenizing.

| Tokens per entity | Positions | Entities | Heap | Per entity | Build |
| --- | --- | --- | --- | --- | --- |
| 20 | no | 1,000,000 | 471 MB | 471 B | 3.6 s |
| 20 | yes | 1,000,000 | 1,187 MB | 1,186 B | 4.5 s |
| 120 | no | 1,000,000 | 1,496 MB | 1,495 B | 27.8 s |
| 120 | yes | 300,000 | 1,572 MB | 5,239 B | 7.2 s |

The last row at one million is extrapolated, about 5.2 GB (5,473 B per entity at 100,000). Each position slice
costs a 24-byte header; a flat layout was not measured.

## 8. Open evidence questions

- Share of SemSource entities with an offloaded body: not measured. `docs/inventory-scope.md:14` allows the
  qualification corpus `test/setup03a`; its README (line 74) lists four functions and one passage, too few. The
  21.6k-entity corpus SemSource offered on #10 would answer it.
- Tokens per real entity, inline and with bodies up to the 4,000-character cap.
- Memory and build time with real tokenizing and a flat position layout.
