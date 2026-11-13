# Inventory: setup-04b-02-lexical-processor

base: `8262d000` (`claude/lexical-processor`, `origin/main` `f8bdf8b7` plus the claim commit). Inventory only: no
target state, options or recommendation. Each pin is `` `path:line` — `text` ``, read in this run on the worktree
`/Users/coby/Code/c360/semengine/.worktrees/claude-lexical-processor`. Sister pins are SemSource at `4ca2072c`
(its `main`), each file read with
`gh api repos/C360Studio/semsource/contents/<path>?ref=4ca2072c5d9d6bf4ec5ad76dc33b92d61c1614c1`
(docs/inventory-scope.md rule 4). Round 2: revises round 1 (commit `51ba4495`, sha256 `638ccbf4…f2f5`) for the
inventory review at that commit.
Terms: a *plug-in* is the code a `graph/follower` owner supplies (ADR-108 D5); *apply Value* means a plug-in builds
its state from `follower.Entry.Value`, *re-read* means it reads `ENTITY_STATES` again at apply time; an *offloaded
body* is text held in an object store and named by `EntityState.StorageRef`.

Question (docs/inventory-scope.md rule 1): what exists that a processor feeding `graph/lexical` from `ENTITY_STATES`
and serving a search verb touches, duplicates or must not break, and who outside the repo carries its surface.
Repositories read: this one; SemSource (consumer, tier 0: its search and readiness reads, body offload, corpus
facts); the other consumers only for a negative that round 2 could not re-measure (§9). SemStreams: not read;
no pin package is carried (§8).

## 1. The claimed gap

Claim: nothing in production fills or reads a `graph/lexical` index.

- `git grep -n 'graph/lexical' -- '*.go' ':!graph/lexical/*'` gives only
  `internal/harness/contract/lexicaltransport_test.go` (the no-NATS guard). No production importer.
- `graph/lexical/README.md:9` — `No component in this repository fills an index or answers a request with one yet.`
- `openspec/specs/lexical-search/spec.md:7` — `No component fills an index from live data yet.`
- No responder, subject or readiness key names lexical: `git grep -n -i 'lexical' -- graph/query_verbs.go
  graph/readiness processor` is empty.
- A text search verb does exist, on another index: `graph/query_verbs.go:88` — `{Name: "search", Subject:
  "graph.embedding.query.search", Responder: ResponderGraphEmbedding,` (§3). The gap is "no word-index answerer",
  not "no search verb".

## 2. Every current spelling of the fact

The fact: *which entities' text matches a query, ranked*. Its parts: what an entity's text is (2a), how text is
followed from `ENTITY_STATES` and rebuilt (2b), how a match is served and gated (2c).

### 2a. An entity's text, today

- graph-embedding's extractor: `processor/graph-embedding/hop1.go:228` — `func (c *Component) extractText(state
  *graph.EntityState) string {`: string values whose predicate ends in a configured suffix, relationships skipped,
  joined by one space (`:244` — `return strings.Join(parts, " ")`). No name/body split: one string.
- Its suffixes: `processor/graph-embedding/config.go:27` — `var defaultTextSuffixes = []string{".title", ".content",
  ".description", ".summary", ".text", ".name", ".body",`; a non-empty `text_suffixes` replaces them
  (`config.go:135-136`).
- Its cap: `processor/graph-embedding/config.go:23` — `const maxSourceTextLenBM25 = 4000` (characters), cut at a
  word by `graph/embedding/worker.go:701` — `func truncateAtWord(text string, maxLen int) string {`.
- Offloaded body: `processor/graph-embedding/hop1.go:152` — `case state.StorageRef != nil &&
  c.storeRegistered(state.StorageRef.StorageInstance):` keeps identity text only (`:159` — `record.IdentityText =
  c.extractText(&state)`); the body is fetched later, off the follower goroutine, by the Worker
  (`graph/embedding/worker.go:744` — `func (w *Worker) fetchTextFromStorage(ctx context.Context, ref *StorageRef)
  (text string, bodyTruncated bool, err error) {`, read capped at `:770` — `data, err :=
  io.ReadAll(io.LimitReader(reader, readCap))`), joined identity first (`:681` — `text = record.IdentityText +
  identityBodySeparator + body`).
- Unregistered store: inline text only, counted and warned once (`hop1.go:161-162`, `:214` — `func (c *Component)
  reportOffloadedBodyExcluded(key, instance string) {`).
- Store resolver source: `component/dependencies.go:62` — `StoreRegistry *storeregistry.Registry`, read at
  `processor/graph-embedding/component.go:170` — `storeRegistry = deps.StoreRegistry`.
- Names, three spellings (unchanged since change 1 inventory §3): vocabulary label predicates
  (`vocabulary/registry.go:477` — `func DiscoverLabelPredicates() map[string]int {`), read by graph-index
  (`processor/graph-index/index.go:88` — `c.namePredicates = vocabulary.DiscoverLabelPredicates()`) and graph-query
  (`processor/graph-query/component.go:185` — `c.labelPredicates = labelOrder(vocabulary.DiscoverLabelPredicates())`);
  the suffixes `.name`/`.title`; fusion's product-supplied `Label`.
- graph-clustering's words: `graph/clustering/terms.go:57` — `func entityWords(state *graph.EntityState)
  []string {`: every non-relationship string value, no suffix filter, its own tokenizer (`:28` — `func
  extractWords(text string) []string {`) and stopwords (`:16`); predicate names and the ID add none (`:55-56`).
- graph-query's local-search text: `processor/graph-query/localsearch.go:95` — `func (e searchEntity) score(terms
  []string) float64 {` matches each term against every predicate name and every string value, lower-cased
  (`:103-110`), plus the ID's type.
- The content-role vocabulary: `message/content_storable.go:84` — `ContentRoleBody = "body"`, `:92` —
  `ContentRoleTitle = "title"` (also `ContentRoleAbstract` `:88`). The store keeps only the body role
  (`message/content_storable.go:15-17` — "objectstore.Store.StoreContent stores only the text of / the \"body\"
  role, as raw bytes"; enforced by `storage/objectstore/content.go:72` — `func bodyOnly(cs
  message.ContentStorable) (string, error) {`), so a `StorageRef` body is the body role by construction.
- So four current rules answer "what is an entity's text": graph-embedding's suffixes, graph-clustering's every
  string value, graph-query's predicate names plus string values, and the content roles.
- What `graph/lexical` wants: `graph/lexical/index.go:37-39` — `type Document struct {` / `Name string` /
  `Body string`. No current extractor produces that split; the content roles name one (title, body) for payloads
  that use them. Change 1 left the mapping here:
  `openspec/changes/archive/2026-11-12-setup-04b-01-lexical-index/design.md:174` — `2. Change 2: which predicates
  feed Name and Body, including file paths for R3; and a per-entity word cap, chosen by`.

### 2b. Following `ENTITY_STATES` and rebuilding on start

- Two primitives keep in-process state current from one KV watch with a ready-after-replay gate
  (`git grep -n 'WatchAll\|graphview.New\|follower.Open' -- '*.go' ':!*_test.go'`; `storagereport` watches its
  own bucket and is not in the class):
  - `graph/follower`: `graph/follower/follower.go:138` — `func Open(ctx context.Context, cfg Config) (*Follower,
    error) {`; watch at `:229` — `watcher, err := f.reader.WatchAll(ctx)`; `Config` at `:62-88` (`Deliver`,
    `Failed`, `Withhold`, `Key`). Three plug-ins: `processor/graph-index/component.go:236`,
    `processor/graph-embedding/component.go:256`, `processor/graph-clustering/component.go:192` — `f, err :=
    follower.Open(ctx, follower.Config{`.
  - `pkg/graphview` (ADR-081): `pkg/graphview/view.go:163` — `func New[T any](source WatcherSource, decode
    DecodeFunc[T], opts ...Option) (*View[T], error) {`, "ONE WatchAll feeding one validated in-memory current-state
    projection" (`pkg/graphview/doc.go:1-3`); readers refused before the replay and after a loss
    (`pkg/graphview/errors.go:12-15` — `ErrNotReady`); hooks `OnApply`, `OnCaughtUp`, `OnWatcherLost`, `OnPoison`
    (`view.go:55-65`). Its one `ENTITY_STATES` user: `output/websocket/state.go:109` — `reader, err :=
    kvcatalog.OpenCatalogReader(ctx, c.natsClient, graph.BucketEntityStates)`, `:113` — `view, err :=
    graphview.New(reader, c.decodeState, graphview.WithHooks(graphview.Hooks{`. graph-embedding's `VectorCache` is a
    graphview over `EMBEDDING_INDEX` (`graph/embedding/vector_cache.go:64` — `view, err :=
    graphview.New(indexSource{c.storage.index}, decodeVector)`).
  - What each gives a word index (read, not chosen):

    | | `graph/follower` | `pkg/graphview` |
    | --- | --- | --- |
    | Readiness key | publishes a `GRAPH_STATUS` envelope per key (`follower.go:342`) | none |
    | Revision | floor watermark of completed revisions, `Indexed()` (`:369`) | highest applied, `Applied()` (`view.go:392`) and per-key revision (`Get`, `:403`) |
    | Reset | latched, fatal `graph_state_reset_required` (`:364`, `:414-416`) | none; a bad value poisons its key (`view.go:614-615`) |
    | Repair | the plug-in's own lane; follower reads its `Failed` count | none |
    | Loss | lost feed latched, `QueryReady` refuses (`:318`, `:418-419`); out-of-order delivery and a re-created stream declared (`:1-17`) | fails closed on watch loss only (`view.go:247`); assumes revision-ascending order (`:628`) |
    | Held state | none; the plug-in holds its own | a copy of each decoded `T` per key |
    | Readers | `QueryReady`, `Stuck`, the envelope | `Get`, `List`, `SnapshotAndSubscribe` with deltas |

- Rebuild on start, every plug-in: the follower's snapshot-then-live watch (spec `entity-state-follower`
  "The initial build completes at the snapshot's revision", `openspec/specs/entity-state-follower/spec.md:75`).
  graph-index rewrites its five buckets from the replay (#108, open: "Every restart rebuilds all five index buckets
  from scratch"); graph-clustering rebuilds its in-memory maintainer; graph-embedding re-reads each entity but skips
  work whose generated record stands (`processor/graph-embedding/hop1.go:180` — `if !saved {`, comment `:181-182`
  "A generated record at this revision or a newer one already stands"), so it does not refetch an unchanged
  offloaded body on restart.
- Apply styles:
  - Re-read: graph-index (`processor/graph-index/index.go:122` — `func (c *Component) deliver(ctx context.Context, e
    follower.Entry) {`, submits key and revision to a keyed pool, `processor/graph-index/component.go:265` — `if err
    := c.startIndexPool(poolCtx, f); err != nil {`) and graph-embedding (`hop1.go:25-31` comment: "Hop 1 reads the
    entity's current value rather than applying the entry's Value"; `Get` at `hop1.go:70` — `entry, err :=
    c.entities.Get(ctx, key)`).
  - Apply Value inline, on the follower's goroutine: graph-clustering (`processor/graph-clustering/component.go:276`
    — `if err = graph.UnmarshalEntityState(e.Key, e.Value, &state); err != nil {`), with a per-key revision guard in
    `graph/clustering/maintainer.go:130` — `if revision <= old.revision {`.
  - The rule that binds the second style: `openspec/specs/entity-state-follower/spec.md:204-205` — "a plug-in that
    applies `Value` SHALL ignore a delivery at or below the revision it last applied for that key". ADR-108's
    amendment allows it for a plug-in with one apply path and no repair lane (`docs/adr/108-processor-shell.md:13-18`).
- `graph/lexical.Index` holds no revision: `graph/lexical/index.go:80-84` (`type entity struct {` / `name []string`
  / `body []string` / `length int`). A plug-in applying Value must keep the per-key revision itself.

### 2c. Serving a text match, and gating it

- Search verbs and scorers (four scorers, change 1 inventory §2, re-read): `BM25Embedder` + `VectorCache` behind
  `graph.embedding.query.search`; graph-query's substring scorers `processor/graph-query/localsearch.go:95` and
  `processor/graph-query/textsearch.go:138`; fusion's `pkg/fusion/engine_lens.go:617` — `func lexicalScore(q, name
  string) float64 {` (whole query against a display name).
- The request/reply: `graph/query_search_types.go:33-38` — `type SemanticRequest struct {` (`Query`, `Limit`,
  `Scope`, `MinRevisionField`); `:55-60` — `type SemanticData struct {` (`Results []SimilarityHit`, `EmbedderType`,
  `UnreadableRecords`, `FailedRecords`); `:14-17` — `type SimilarityHit struct {` (`EntityID`, `Similarity
  float64`). Bounds `:21` — `DefaultSimilarityLimit = 10`, `:24` — `MaxSimilarityLimit = 100`.
- `graph/lexical` answers `Result{Hits []Hit; Matched int}` (`graph/lexical/index.go:58-61`); `Hit.Score` is an
  unbounded BM25F sum (`index.go:194-204`), not a similarity in [0, 1]. `SemanticData` has no pre-limit count.
- Gate: every graph verb refuses before its first build through `Follower.QueryReady`
  (`graph/follower/follower.go:410` — `func (f *Follower) QueryReady(ctx context.Context) error {`), code
  `index_not_ready` (`:452-453` — `func notReady(why string) error {` / `return
  errs.ClassifiedCode(errs.ErrorTransient, graph.ErrorCodeIndexNotReady, ...`), fatal
  `graph_state_reset_required` on a latched reset (`:414-416`). graph-embedding adds its own cache-loss refusal
  (`processor/graph-embedding/query.go:151` — `func (c *Component) queryReady(ctx context.Context) error {`).
- Revision claim: graph-index replies carry `indexed_revision` and honour `min_revision`
  (`openspec/specs/graph-index/spec.md:222` "An unmet `min_revision` is refused"). The embedding verbs claim none
  (`processor/graph-embedding/query.go:232` — `response, err := graph.NewQueryResponse(graph.ResponderGraphEmbedding,
  0, data)`; spec `openspec/specs/graph-embedding/spec.md:101` "The embedding verbs claim no revision"), because
  their answer reads `EMBEDDING_INDEX`, not the follower's applied state.
- Index internals that bear on serving (#623, measured at `6af06ca1`, `index.go` unchanged since): about 60 bytes
  per held word, 9.5 KB per entity at 157 held words, about 9.5 GB at one million such entities; build about 16,500
  entities a second on one goroutine; search time follows entities matched (0.7 ms at 2,475 matched to 19.7 ms at
  40,100, on 50,000 entities), under the read lock (`index.go:184-185` — `ix.mu.RLock()` / `defer ix.mu.RUnlock()`),
  so a write waits. Scope is applied after scoring every holder (`index.go:194-210`): a scoped search costs what the
  global search of the same words costs.

## 3. The search verb: subjects, overlap, and what change 3 consumes

- Subjects are spelled only in `graph/query_verbs.go` (AGENTS.md § 2); the reserved-subject guard reads the table,
  so a new family is covered without editing the guard
  (`internal/harness/contract/reservedsubjects_test.go:32` — `for _, verb := range graph.QueryVerbs() {`).
- The table names six responders (`graph/query_verbs.go:23-42`); the transport spec enumerates them by name
  (`openspec/specs/graph-transport-boundary/spec.md:13-19` and the scenario `:36-40`), so a new responder or family
  edits that requirement.
- Overlapping verbs today:
  - `graph.embedding.query.search` (`query_verbs.go:88`), forwarded by `graph.query.semantic` (`:122`;
    `processor/graph-query/query.go:33` — `"semantic":  {graph.ResponderGraphEmbedding, "search"},`).
  - `graph.embedding.query.similar` and `graph.query.similar` (`query_verbs.go:90`, `:124`): entity-to-entity, the
    embedder its only tier 0 answerer. Change 1 named `SimilarTo` as a later extension
    (`.../2026-11-12-setup-04b-01-lexical-index/design.md:131-132`).
  - `graph.query.localSearch`, `globalSearch`, `searchGraph` (`query_verbs.go:116-121`): substring scorers over one
    group's page (`graph/query_local_search_types.go:5-7` — `const MaxSearchTerms = 16`, `:24-26` truncation rule).
  - graph-query's switch: `processor/graph-query/query.go:44` — `if downstream.responder ==
    graph.ResponderGraphEmbedding && !c.config.EmbeddingEnabled {`; spec `openspec/specs/graph-query/spec.md:868`
    "Embedding verbs can be turned off": `semantic` and `similar` refuse `not_enabled`.
  - The no-embedder stack: `openspec/specs/composed-stack/spec.md:93-98` — with no graph-embedding "A
    natural-language question SHALL be refused `not_enabled`, never answered empty." Tier 0 without graph-embedding
    therefore has no NL answer today.
  - `searchGraph` "SHALL make no semantic request" (`openspec/specs/graph-query/spec.md:858-861`); open #563 (owner
    ruling S1, `status:blocked`) wants it to fall back to the embedding `search` verb, labelled.
- Callers of the text verb: `pkg/fusion/fusionnats/client.go:153` — `raw, err := c.request(ctx, c.verbs.semantic,
  graph.SemanticRequest{Query: query, Limit: limit, Scope: scope})`, row looked up by name `semantic` on
  `graph-query` (`:42`); seeds carry `Similarity` with `HasSimilarity: true` (`:164`), copied to
  `fusion.Node.Similarity` (`pkg/fusion/engine_lens.go:259-262`). Fusion documents the scale:
  `pkg/fusion/retrieval.go:57` — `// Semantic resolve reports cosine similarity; symbol and prefix report none.`; and
  puts it on its public wire as `pkg/fusion/contract.go:129` — ``Similarity *float64 `json:"similarity,omitempty"` ``,
  present when `IncludeScores` is set (`:120-122`): an exported field of a public package's reply. Fusion
  defers on `index_not_ready` (`openspec/specs/fusion/spec.md:10-14`) and passes on
  `unreadable_records`/`failed_records` (`:67-70`).
- What change 3 consumes (plan, #10 comment 6833147079, item 3): "Local search is the index scoped to the group's ID
  prefix; global search is the index over everything, with hits then grouped by community". So it reads: a scoped
  search (prefix list), a limit, a ranked total order, and a pre-limit match count for `EntitiesTruncated`
  (`graph/query_local_search_types.go:35`, `graph/query_global_search_types.go:49`); and the query refusals #624
  asks it to decide (`ErrQueryTooLong`, `ErrReservedSyntax`, `graph/lexical/query.go:14-23`). `lexical.Result`
  carries `Matched` (`index.go:60`); no current reply type carries it.

## 4. Readiness and restart

- `index_not_ready` semantics in graph-embedding: spec `openspec/specs/graph-embedding/spec.md:166-176` — before the
  first build, on a lost feed or a held hop-1 failure, transient `index_not_ready`; a Worker failure does not
  withhold; a cache not yet snapshotted or lost refuses too. graph-index: `openspec/specs/graph-index/spec.md:56-61`
  and the ordering rule `:240-245` (validate, then `index_not_ready`, then `min_revision`).
- Readiness keys: `graph/readiness/watcher.go:50-65` (`graph-index`, `graph-embedding`, `graph-clustering`,
  `graph-ingest`, `rule`); `:67-70` "Declaring a key here does NOT make it mandatory." The follower derives its
  gauge subsystem from the key (`graph/follower/follower.go:64-66`).
- Rebuild cost: unmeasured for graph-index (#108, open). For the word index: build rate above (§2c) is the only
  number; decode, `Get` and body fetch are not in it.
- Restart in the composed stack: every restart test runs `checkAnswers`, which runs `checkSearch`
  (`internal/smoke/smoke_integration_test.go:313` — `checkSearch(t, o, s, f, w, deleted)`), which waits on one
  `semantic` answer as its ordering event (`:517-534`) and requires `EmbedderType` `bm25` (`:535` —
  `if got.data.EmbedderType != "bm25" || ...`). The restart tests call it at `:841`, `:1001`, `:1065`, `:1114`.
- Other smoke surfaces a new component or a tier-0 NL answer edits:
  - The full composition: `internal/smoke/stack_integration_test.go:150-151` (components list) and `:152` —
    `indexKeys: []string{readiness.KeyGraphIndex, readiness.KeyGraphEmbedding, readiness.KeyGraphClustering},`,
    which `awaitIndexed` waits on (`:734`); the factories `:177-185` (`var componentFactories =
    map[string]func(*component.Registry) error{`); the config `stackConfig` (`:74-101`).
  - The graph-only composition: `:156-162` (`components: []string{"graph-index", "graph-ingest", "graph-query"},`).
  - The no-embedder test that proves `composed-stack` `:93-98`: `internal/smoke/noembedder_integration_test.go:181-191`
    asserts an NL question is refused `not_enabled` naming `semantic`. A tier-0 NL answer without graph-embedding
    changes this test and that requirement together.
  - The fixture's entities have names only, no body and no `StorageRef`, and no object store is composed
    (`internal/smoke/smoke_integration_test.go:92-107`, `stack_integration_test.go:74-101`).

## 5. Adjacent claims

### 5a. Open issues

Searches: `gh issue list --state open --search` on lexical, "word index", follower, graph-embedding, graph-index,
smoke, search, StorageRef, offloaded, embedder, readiness, rebuild; hits read.

| Issue | What it asks | Bearing on change 2 |
| --- | --- | --- |
| #628 | smoke search onto the word index, split over changes 2-4 | Its first bullet is change 2's: "adds the new processor to the composed stack and a smoke check that its search verb answers the fixture's live entities and drops deleted ones, through a restart" |
| #623 | index memory 60 B/word; "Done when" the processor design starts from measured cost and accepts it or changes the layout | Its "Done when" names this design |
| #622 | acronym plurals split (`IDs` -> `i`,`ds`); digit-before-capital unmeasured on SemSource code | Tokenizer, `graph/lexical`; item 2 needs SemSource data the processor first makes reachable |
| #624 | 16-word limit, reserved syntax; decide "when search is wired to people and agents" | Names change 3 as decider; change 2's verb is the first wire a pasted query reaches |
| #620 | re-measure `NameWeight` on SemSource names, `status:blocked` on "SemSource's runner reaching `graph/lexical`" | Change 2 is what lets that runner reach it |
| #616 | router misroutes on trigger words | change 3 |
| #506 | keyword index per prefix in graph-index | Plan item 3 says change 3 "dissolves #506"; §2c: a scoped lexical search costs the global match count |
| #563 | `searchGraph` falls back to embedding `search`, labelled (`status:blocked`) | Which search verb the fallback asks once a word index exists |
| #570 | what a product's `main` must know to boot | A new component adds a factory and a config block a `main` must know (§9) |
| #574 | smoke for a long NATS outage | A fourth follower plug-in is one more component it would cover |
| #108 | graph-index rebuild-on-restart unmeasured | same rebuild shape |
| #109 | one follower and processor shell, adopted by followers | a new follower plug-in adopts both |
| #551 | offloaded-body fetch has no deadline | applies to any body fetch this processor makes |
| #602 | replies carry the `ENTITY_STATES` generation | applies if the verb claims a revision |
| #489 | tier 1 provider and its tier-0 fallback test | "losing the provider degrades to tier 0 with search intact" (`docs/setup-plan.md:79-80`) |
| #477 | `pkg/graphview` adoption | `VectorCache` is a graphview; graphview is the second projection mechanism (§2b) |
| #564 | graph-embedding false "work not drained" warning | none unless placed in graph-embedding |
| #606 | port-history comment clean-up, batches 6 (`processor/graph-embedding` + `graph/embedding`), 7 (`processor/graph-index`), 8 (`graph/follower`, `graph/lexical`), 10 (`internal/smoke`) unopened | file overlap when those batches open |
| #619, #621 | stale cover-check count; repository-map rows | a new package adds a cover target and a map row |

Word only, not the territory: #9, #10 (epic), #11, #68, #110, #111, #134, #141, #155, #195, #196, #284, #300,
and #350, #484-#487, #502, #535, #554, #571, #590, #604, #605, #625.

### 5b. Open pull requests (`gh pr list --state open --json number,title,changedFiles,files`)

- #629 (this claim, draft): `openspec/changes/setup-04b-02-lexical-processor/` only.
- #626 (draft, `claude/comment-history-batch-5`, `processor/graph-query`, comments only): 0 files pushed. Overlaps
  if change 2 edits `processor/graph-query` (a forward or a switch).
- #627 (draft, `claude/comment-history-batch-3`, `processor/graph-ingest` tests, comments only): 0 files pushed. No
  overlap expected; change 2 has no stated reason to edit graph-ingest.
- #597, #598: `go.mod`/`go.sum` only.

### 5c. Specs and decision records touched

- Specs: `lexical-search` (purpose line `:7`, README line `:9`), `entity-state-follower` (`:200-205`),
  `graph-embedding` (`:54` "Tier-0 config", `:101`, `:166`), `graph-query` (`:451` "Local search ranks by the pin's
  text score", `:858`, `:868`), `graph-transport-boundary` (`:13-40`), `composed-stack` (`:13-36` names
  "graph-ingest, graph-index, graph-embedding, graph-clustering, graph-query, graph-gateway and an applied-state
  `websocket`"; `:93-98`), `fusion` (`:10`, `:67`), `graph-readiness`.
- ADRs in `docs/adr`: ADR-108 (one follower, plug-in contract, Value-apply amendment), ADR-109 (ready after boot),
  ADR-066 (honest readiness, 30 s degraded), ADR-083/ADR-088 (readiness per producer), ADR-063 (store resolver;
  `:362-370` "those bodies WILL embed — shifting BM25/neural embedding and search output"), ADR-062 (fusion), ADR-071
  (scope on NL retrieval), ADR-081 (`graphview`), ADR-113 (one run context). `git grep -n -i 'lexical\|BM25' --
  docs/adr` names no lexical index decision.
- Plan: `docs/setup-plan.md:77-80` (tier 0 includes "lexical BM25 (in process)"; tier 1 degrades "with search
  intact"), `:446-450` (04B: "stale/update semantics, index rebuild, and restart").

## 6. Rules a new processor meets

- Lifecycle: `component.Lifecycle` shell (AGENTS.md § 1; ADR-108 D3); one run context from `Start`, ended in `Stop`
  (ADR-113); strict config refusing unknown keys; `Register` per component package, aggregated only in `main`;
  collectors only on the injected registry; `lifecycle-suite` adapter.
- Follower: a plug-in never holds a watcher, reset, watermark or publisher (ADR-108 D5); applying Value needs the
  per-key revision guard (§2b).
- Verbs: subjects from the table; strict request decode; `index_not_ready` before reads; responder serves exactly
  its declared verbs (`graph-transport-boundary` `:84-88`).
- Tests: no sleeps, no own deadlines, integration under plain `-tags integration`, a shared NATS through the reset,
  oracles shown able to fail.
- Tier-0 provider guard: each tier-0 processor is held by a contract test to import no model or provider code
  (`internal/harness/contract/embeddingprovider_test.go:51-53` graph-embedding, `:63-65` graph-clustering; the
  sensitivity loop names both packages, `:79` — `for _, pkg := range []string{"./processor/graph-embedding",
  "./processor/graph-clustering"} {`). A new tier-0 processor has the same obligation.
- Ledger: no pin source, so no admission-ledger row (change 1 inventory §6); a new package is a
  `scripts/cover-check.sh` target only when added.

## 7. Problem shape and the collision table

Shape: an in-memory projection of `ENTITY_STATES`, rebuilt from a replay on every start, read by scoped, limited,
ranked request/reply queries gated on that replay. Closest instances:

- `output/websocket` (`output/websocket/state.go:109-115`): a `pkg/graphview` over `ENTITY_STATES`, gated on the
  replay (`OnCaughtUp`) and on loss (`OnWatcherLost`), serving clients from memory; it holds the stored bytes per key
  and answers no request/reply verb and publishes no readiness key.
- graph-clustering: a follower plug-in, in-memory, apply-Value, rebuilt on start; serves no verb, publishes records
  to `COMMUNITY_INDEX`.
- graph-embedding's `VectorCache`: a `pkg/graphview` (`graph/embedding/vector_cache.go:64`) over `EMBEDDING_INDEX`,
  not `ENTITY_STATES`, serving the `search` verb (`:105` — `func (c *VectorCache) FindSimilar(...`); graph-embedding
  gates the verb on both its follower and the view (`processor/graph-embedding/query.go:151-161`).

No instance both projects `ENTITY_STATES` into memory and serves a request/reply verb from it. The two mechanisms
that keep such a projection are compared in §2b.

The collision table is triggered: the change adds a communication primitive (a verb or a responder) and a readiness
producer.

| Dimension | Evidence |
| --- | --- |
| Semantic class | Ranked entities whose text matches a query, at tier 0 |
| Owners | graph-embedding (`search`, `similar`; served from a graphview, `vector_cache.go:64`); graph-query (`localSearch`, `globalSearch`, `searchGraph`, forwards `semantic`/`similar`); fusion `lexicalScore` (one display name); graph-index `byName` (whole-name key, `processor/graph-index/name_index.go:27`); graph-clustering keywords (`graph/clustering/terms.go:28`). Projection mechanisms: `graph/follower` (three plug-ins) and `pkg/graphview` (`output/websocket/state.go:113` on `ENTITY_STATES`, `vector_cache.go:64` on `EMBEDDING_INDEX`) |
| Catalogs | Verb table `graph/query_verbs.go:65-130`; derived buckets `graph/kvcatalog/kvcatalog.go:186-187` (`EMBEDDING_INDEX`, `EMBEDDING_DEDUP`, owner `graph-embedding`); no lexical bucket (plan: none at tier 0) |
| Status | `GRAPH_STATUS` keys `graph/readiness/watcher.go:50-65`; `Health` stuck states per component; `SemanticData.UnreadableRecords`/`FailedRecords`; SemSource reads `graph-embedding` as "semantic index" (§9) |
| Lifecycle | Rebuilt on every start: the three follower plug-ins and both graphviews (a view runs once, `graph-view-subscription` "A view runs once"; its owner replaces it with a new one, `pkg/graphview/doc.go`); graph-embedding keeps vectors across restarts in `EMBEDDING_INDEX`; nothing removes a projection but restart |
| Ownership | Each follower plug-in one per process; no lease or partition; an in-memory index is per process, so two processes hold two copies. How two responders on one subject share requests was not examined (same for graph-index and graph-embedding today) |
| Readers | graph-query forwards; `fusionnats` (`semantic`); SemSource fusion NL and `graph_search`; `internal/smoke`; websocket clients through its graphview (`Get`, `SnapshotAndSubscribe`, `view.go:403`, `:480`) |
| Writers | graph-ingest is the only `ENTITY_STATES` writer (AGENTS.md § 2); the index's only writer would be its follower plug-in |
| Recovery | Follower: lost feed latched until restart, reset fatal, degraded after 30 s with no completion (`graph/follower/follower.go:89-96`); graph-embedding repair lane `hop1.go:289`; graph-index repair loop; graph-clustering none. graphview: fails closed on watch loss only (`view.go:247`), poisons a bad key (`:614-615`), no reset, no repair, no order or stream-generation check; websocket records loss as `appliedLost` (`state.go:115`) |

## 8. Pin decision records

No pin package is carried: `graph/lexical` is new and the processor is new, so no pin probe, shape sweep or
pin-ADR listing applies. The ported ADRs this repo holds that govern the surfaces it adopts are in §5c.

## 9. Adopter seam inventory

Surfaces reached from outside: the search verb's subject and reply, a component type and its config, a readiness
key, and what a word search finds. The adopters: SemSource (its composition root and its tools), a product's `main`
(#570), a config author.

SemSource facts at `4ca2072c`, every pin read with `gh api ...?ref=` (header):

- It already builds on SemEngine: `go.mod:36` — `github.com/c360studio/semengine
  v0.1.0-alpha.4.0.20261112145006-7bf638643b98`; it registers SemEngine factories by hand
  (`cmd/semsource/run.go:314-322`, `"graph-embedding":  graphembedding.Register,` at `:317`).
  `docs/setup-plan.md:429-430` ("its mainline stays on SemStreams until tier 1") no longer matches SemSource `main`.
- It composes graph-embedding with `bm25` by default (`run.go:768` — `embedderType := "bm25"`), turns semantic off
  only for `none` (`:789-790` — `graphQueryConfig["embedding_enabled"] = false`), and restates all nine default
  suffixes (`.abstract` and `.subject` included, as in `processor/graph-embedding/config.go:27-28`) and adds only
  `.signature` and `.comment`, because a list replaces the defaults (`run.go:874-886`).
- Its NL path: lenses route any query with white space to NL (`source/fusion/lens/code/code.go:35-39`,
  `source/fusion/lens/docs/docs.go:53-66`), which is `graph.query.semantic` through fusion. Its `graph_search` tool
  sends `graph.query.searchGraph` (`processor/mcp-gateway/query_tools.go:107`). No SemSource production code reads a
  similarity value or sets `IncludeScores`: GitHub code search on its default branch (not bound to a ref) finds
  `include_scores` only in `test/setup03a/qualification_test.go` and `HasSimilarity` in
  `processor/code-context/exact_seed.go`, whose one hit (`:32`, read at the ref) is a comment, plus two test files.
- Its readiness reads name graph-embedding as the semantic index: `internal/graphstatus/reader.go:37-38` —
  `// KeyGraphEmbedding is the semantic index: it backs code_search.` / `KeyGraphEmbedding = "graph-embedding"`;
  read at `processor/source-manifest/workbench_capabilities.go:31`, `:293` and `processor/mcp-gateway/tools.go:144`.
- Bodies: every doc passage is offloaded, with no inline fallback (`handler/doc/entities.go:372-375` — "There is no
  inline fallback any more: a passage without a handle is a passage whose body cannot be hydrated / and whose text
  never reaches the semantic index"); every body-bearing code entity carries a `StorageRef` to its verbatim source
  (`processor/ast-source/component.go:849-853`; containers excepted, `processor/ast-source/bodystore.go:40-41`).
  Inline text is the title and, for code, `code.doc.signature`/`code.doc.comment`. Passage bounds: ceiling 2,000
  bytes, hard max 6,000 (`docs/design/text-ingestion-and-retrieval.md:64`). Corpora: 21.6k entities (Go
  dogfood) and 32k (`:152`).
- Other consumers (semconnect, semteams, semboids, semsparql): not measured in round 2. Round 1's negative (no
  production caller of the search verbs) came from local checkouts, against rule 4; the `gh api` code search that
  would replace it hit GitHub's code-search rate limit. No premise in this inventory depends on it.

The four questions, for the adopter as SemSource's developer or a product `main` author today:

1. **What must they know?** (a) A new component type exists, must be registered in `main` and given a config block
   (SemSource registers each factory by hand, `run.go:314-322`). (b) Which predicates are text: four rules answer that
   today (§2a), and a word index would add or pick one. (c) That an offloaded body is indexed only when a store is
   registered for its `StorageInstance`. (d) Which verb answers NL at tier 0, and whether `embedding_enabled` still
   gates it. (e) Which readiness key now means "search is ready" (SemSource reads `graph-embedding`). (f) That a
   score is no longer a similarity in [0, 1] if the reply keeps `SimilarityHit`.
2. **If they do nothing?** Traced on today's code: they keep graph-embedding and get today's hashed-vector answers;
   the word index is not built; nothing tells them. If they add the component and register no store, bodies are
   left out with one warning and a counter (today's graph-embedding behaviour, `hop1.go:214-226`). If the
   processor answers the same `semantic` verb SemSource already sends, their callers change answer with no code
   change.
3. **Where do they find out?** Registration: boot error (`requireRegisteredFactories`, `run.go:309-312` comment). A
   missing store: log line and counter. Readiness meaning: doc at best. Score scale: a framework doc
   (`pkg/fusion/retrieval.go:57`, "cosine similarity"), which goes false if a BM25F score reaches `Similarity`; no
   consumer reads it today.
4. **What should they have to know?** At most: register it. The gaps between 1 and 4 are (b), (c), (d), (e).

Prediction to observe: whether to fetch a body is decided today by the caller predicting a store registration
(`hop1.go:202-208`); the follower already observes the entity's `StorageRef`.

## 10. Measurements and what is not known

Measured by others and cited, not re-run: #623 (index memory, build and search time); change 1 probes
(`openspec/changes/archive/2026-11-12-setup-04b-01-lexical-index/notes/probes/memory-probe.log`,
`sequence-probe.log`). No probe was run for this inventory.

Arithmetic, not measurement: at #623's 60 B per held word, a SemSource passage near its 2,000-byte ceiling (order
of 300 words) would be about 18 KB, so the 32k-entity corpus about 0.6 GB; this assumes the passage's word count,
which is not measured.

Not known:

- The share of SemSource entities whose text is mostly in an offloaded body: the code says every doc passage and
  every body-bearing code entity (§9); the count per corpus is not measured.
- The time to refetch every offloaded body from the object store on a start, at 21.6k, 32k or a million entities.
- Decode plus `Get` plus `Put` time per entity during a replay; graph-index's rebuild time (#108).
- Memory at a million real entities (only #623's repeated-corpus floor).
- How two instances of one responder share a subject (not read for any responder).
