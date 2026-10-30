# Inventory: setup-04a-04-graph-roots-query

base: `50e2e90` (the claim commit on `claude/port-graph-query`, on `d3c1e13`; it adds no file). `origin/main` is
`c010d0b` (#474, `AGENTS.md` only), which the branch merges before review. Pin paths are SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, written `pin:<path>:<line>`, read from the pin tarball copy at
`/tmp/gq-pin/src` (local only). All 15 files of `graph/query` there are blob-equal to
`gh api 'repos/C360Studio/semstreams/contents/graph/query?ref=8b99efe9'` (`git hash-object` of each equals the API's
`sha`; 15 checked, 0 mismatched). Consumer cites (`docs/inventory-scope.md`: dependency closure and imports;
semteams' agentic seams): `gh search code <term> --repo C360Studio/<repo>` against the remotes, hits read with
`gh api …?ref=<sha>` at semsource `e4febc0d`, semconnect `d0d06e00`, semteams `4f45a387`, semboids `8c03cc53`.

Status: round 2, inventory only (round 1 review findings 1-8 folded in). Kind of change: a port of one package,
tier-0 half, by the owner's order ruling
(#9 comment 6386856657, item 6: "`graph/query` (#96 context threading rides it)") and scope ruling (#9 comment
6386603659: `graph/query/classifier_llm_adapter.go` "excluded; `LLMClient` interface stays").

Briefing hypotheses that did not hold, or that the tree answers:

- "`processor/graph-query` (item 9) needs the embedding classifier": it does not. It composes the chain with a nil
  embedding tier at both sites, `pin:processor/graph-query/component.go:274` —
  `classifier: query.NewClassifierChain(query.NewKeywordClassifier(), nil, nil),` and `:443` —
  `c.classifier = query.NewClassifierChain(query.NewKeywordClassifier(), nil, llmClassifier)`. Only consumer adapters
  build an `EmbeddingClassifier` (§ 4).
- "Is a coverage floor owed?": no. `graph/query` is not on the SETUP 03B critical list
  (`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:457-468`; `processor/graph-query` is),
  so no `scripts/cover-check.sh` line and no `d11Targets` entry.
- "Does anything reach an external provider?": only the adapter file (§ 6 probe P3). With it removed the package's
  module imports are `graph/embedding` alone.

## Pin probe

go1.26.4 darwin/arm64, on the pin copy, unit tests (no build tag).

- P1 `go test -race -count=1 -p 1 -cpu 1 -cover ./graph/query`: ok, 1.731 s, coverage 79.2%.
- P2 `go test -count=5 -p 1 -cpu 1 -shuffle=on -v ./graph/query`: ok, 1.246 s, `-test.shuffle 1793360450894564000`.
- P3 `go list -deps ./graph/query` at the pin reaches `graph/llm`, `model`, `model/wire` and
  `github.com/sashabaranov/go-openai`. On a scratch copy (`/tmp/gq-pin/src-t0`) without `classifier_llm_adapter.go`
  and its test: `go vet` clean, P1 ok (coverage 79.6%), and the in-module deps are `graph/embedding` and what it
  imports; `go-openai` and `model` are still reached there only through the pin's `graph/embedding/http_embedder.go`,
  which this tree does not carry (ledger row `graph/embedding/http_embedder.go`, `docs/admission-ledger.yaml:3741`).
- Lifecycle suite: not applicable. The package has no `Start`, `Stop`, `Run` or goroutine (`grep -n 'go func\|\bgo
  [a-zA-Z]'` over the seven non-test files: none). `background-work` does not apply either.
- Test-text rules over the 8 test files (3,600 lines, 85 `func Test`): `time.Sleep` 4 —
  `classifier_embedding_test.go:379` `time.Sleep(10 * time.Millisecond)`, `:510`
  `time.Sleep(50 * time.Millisecond) // Simulate slow embedding service`, `:541` `time.Sleep(5 * time.Millisecond)`,
  `classifier_chain_test.go:374` `time.Sleep(10 * time.Millisecond) // Ensure timeout`. Skips 1 —
  `classifier_llm_live_test.go:26` `t.Skip("Ollama not available at localhost:11434, skipping")`. Build tags other
  than `integration` 1 — `classifier_llm_live_test.go:1` `//go:build live_llm`. `t.Parallel` 0. Real-tree reads 1 file
  — `slice_f1_surface_contract_test.go` parses `processor/graph-query`, `pkg/fusion/fusionnats`,
  `processor/research-graph-classify` and `-execute` sources (`:30-55`).
- P4 (library, encoding/json at go1.26.4, `/tmp/gq-probe/main.go`): `json.Unmarshal` into `map[string]any` gives
  `[]interface {}` for a string array (`.([]string)` fails), `map[string]interface {}` for an object, `float64` for a
  number (`.(int)` fails); `bool` and `string` assert. Output:
  `path_predicates []interface {} ok=false; time_range map[string]interface {};`
  `limit float64 ok=false; path_intent ok=true; strategy ok=true`.

## Files and size

| Pin file | Lines | Imports outside the standard library |
| --- | --- | --- |
| `classifier.go` | 438 | none |
| `classifier_chain.go` | 177 | none |
| `classifier_embedding.go` | 263 | `graph/embedding` |
| `classifier_llm.go` | 321 | none |
| `classifier_llm_adapter.go` | 122 | `graph/llm` |
| `examples.go` | 67 | none |
| `search_options.go` | 163 | none |

1,551 non-test lines (the ruling's figure); 1,429 without the adapter. Under the ~3,000-line bound (#159): one PR.

## 1. The claimed gap

- The package is not in the tree: `ls graph/query` — "No such file or directory". `git grep -n -E 'type
  (Classifier|ClassifierChain|SearchOptions|SearchStrategy|TimeRange|SpatialBounds|Example|DomainExamples|LLMClient|ClassificationResult)\b'
  -- '*.go'`: no hit. `git grep -n -E 'graphrag|"pathrag"|located_in|StrategyGraphRAG' -- '*.go'`: one comment,
  `graph/id_prefix.go:12`. The `graph` package's `graph/query_*.go` files are package `graph`, not this directory.
- Its one in-module import is present: `graph/embedding/bm25_embedder.go:90` — `func NewBM25Embedder(cfg BM25Config)
  *BM25Embedder {`, `:124` — `func (b *BM25Embedder) GenerateQuery(ctx context.Context, texts []string)
  ([][]float32, error) {`, `:132` — `func (b *BM25Embedder) Generate(ctx context.Context, texts []string)
  ([][]float32, error) {`; `embed` refuses a nil context (`:146-151`) and returns `ctx.Err()` once the context has
  ended (`:153-157`): these are its only errors.

## 2. Every current spelling of the facts the package models

| Fact | Spellings |
| --- | --- |
| Embedder interface | `pin:graph/query/classifier_embedding.go:17` — `type Embedder interface {` (`Embed`, `EmbedBatch`, no context); `graph/embedding/embedder.go:9` — `type Embedder interface {` (`Generate`, `GenerateQuery`, with context) |
| Cosine similarity | `pin:…/classifier_embedding.go:244` — `func cosineSimilarity(a, b []float32) float64 {`; `graph/embedding/vector.go:13` — `func CosineSimilarity(a, b []float32) float64 {` (same arithmetic and zero cases; the pin's doc says the range is [0, 1]) |
| The hint names | JSON tags of `SearchOptions` (`pin:…/search_options.go:34-56`); `searchOptionsToMap` (`pin:…/classifier_chain.go:137-177`); `llmOptionsToMap` (`pin:…/classifier_llm.go:279-321`), which writes a different key set (adds `strategy`, `query`, `predicates`, `types`, `key_terms`, `limit` without ranking; omits `geo_bounds`, `aggregation_*`, `ranking_intent`); the reader's literals `pin:processor/graph-query/graphrag.go:406-413`, `:530`, `:546`, `:630-660`, `:1006`, `:1197`, `:1265`; domain example files' `options` objects (`pin:…/examples.go:13`) |
| Strategy from hints | `pin:…/search_options.go:74` — `func (o *SearchOptions) InferStrategy() SearchStrategy {` (test readers only); `pin:processor/graph-query/graphrag.go:633` — `func (c *Component) resolveStrategy(cr *query.ClassificationResult) string {` |
| Community count default | `pin:…/search_options.go:30` — `const DefaultMaxCommunities = 5` (no reader); `pin:processor/graph-query/graphrag.go:29` — `DefaultMaxCommunities = 5` |
| Bounding box | `pin:…/search_options.go:66` — `type SpatialBounds struct {`; `pin:processor/graph-index-spatial/query.go:64` — `North float64 \`json:"north"\`` (not in the tree) |
| Classification tier numbers | `pin:…/classifier_chain.go:9` — `Tier int // Which tier produced the result (0=keyword, 1=BM25, 2=neural)`; the chain returns 1 for any embedding match (`:86`), the LLM tier 3 (`pin:…/classifier_llm.go:88`); no code returns 2 |

## 3. Adjacent claims on the territory

- Specs: none names classification (`ls openspec/specs`: 37 capabilities, no `query*`). `graph-transport-boundary`
  governs `graph.query.*`-family subjects; this package spells no subject (`grep -n '"graph\.'` over its files: none).
  It imports no NATS client directly; it reaches nats.go transitively through `graph/embedding` (`worker.go`,
  `storage.go`), so `go list -deps ./graph/query` lists nats.go.
- Ledger: no `graph/query` row; `graph/llm` is `defer-exclude` (`docs/admission-ledger.yaml:2539`), naming
  `processor/graph-query` as a next reader; `graph/embedding`'s row (`:3676`) says "graph/query (item 6) cannot build
  without it".
- Rulings: #9 comment 6386856657 item 6 (order, "#96 context threading rides it"); #9 comment 6386603659 (no provider
  code in change 4; the adapter excluded; "The pin's package-local interfaces (`query.LLMClient`, …) port with their
  tier-0 implementations; nothing new is invented ahead of a provider", restated on #32 comment 6386603854).
- ADR-084 and ADR-085 port notes say "None of the code named here is ported: … `graph/query` …"
  (`docs/adr/084-readiness-licenses-health-not-absence.md:15`, `docs/adr/085-gate-on-health-report-freshness.md:15`);
  the code they name is `graph/query/client.go`, retired at the pin (`pin:…/slice_f1_surface_contract_test.go:14`).
- Open pull requests (`gh pr list --state open`, 2026-10-30): #475 (this claim, 0 files); #476 draft
  `claude/port-graphview` (0 files today; it will add a ledger row, a `docs/repository-map.md` row and possibly a
  cover-check line, the same files this change edits); #464 (`go.mod`, `go.sum`: nats.go 1.55.0; this package imports
  no NATS client); #465 (`package.json`, `package-lock.json`: markdownlint). #474 merged at 11:32 today.
- Guidance at the pin: the only mention is `pin:.agents/skills/query-pattern/SKILL.md:74` — "`graph/query.Client`
  mixes direct KV and RPC and is not an adopter default.", about the retired client. No contract section or skill
  governs the classifier (`grep -rn -i 'classifier\|graph/query' .agents .claude` at the pin: generic uses only).

### Open issues naming the package

Query: `gh issue list --state open --search "query" --limit 200` (28 hits). Kept, each naming `graph/query` or a
file under it:

| Issue | Where it names the package |
| --- | --- |
| #96 | body: `graph/query/classifier_embedding.go#L109`, `#L43`; PM disposition 6036129531 (the `Embedder` gains a context); drift note 6386548133 ("The `Embedder` context parameter therefore rides graph/query's own port PR"; "The `Generate` to `GenerateQuery` repair stays here, in the classifier follow-up") |
| #94 | body: `graph/query/classifier.go#L54`, `#L365` (aggregation field binding); disposition 6036107878: a follow-up change after this port |
| #95 | body: `classifier.go#L96`, `#L365`, `classifier_chain.go#L65` (negated, quoted, undecided hints); disposition 6036108178: same follow-up as #94 |
| #32 | body and comment 6386603854: `graph/query/classifier_llm_adapter.go`, the home of the excluded file |
| #9 | epic; the rulings above |
| #105 | names `graph/query` only as an import of `processor/graph-query` and `LLMClassifier` as composed there; its work is PR 9's |

Names the word, not the package: #405, #31, #406, #103, #106, #415, #16, #460, #111 (`graph/query_summary_types.go`,
package `graph`), #300, #108, #155, #109, #461, #68, #350, #24, #195, #196, #7, #11, #10, #141.

## 4. Who reads it (surface audit evidence)

Production importers at the pin (`grep -rln 'semstreams/graph/query"' --include='*.go'`, tests excluded) and the
symbols each names:

| Importer | Kind | Symbols |
| --- | --- | --- |
| `processor/graph-query/component.go`, `graphrag.go` | admitted (PR 9) | `ClassifierChain`, `NewClassifierChain`, `NewKeywordClassifier`, `ClassificationResult`, `TimeRange`, `SpatialBounds`, `StrategyExact`, `StrategyGeoGraphRAG`, `StrategyPathRAG`, `StrategySemantic`, `StrategyTemporalGraphRAG`; in the excluded `initLLMClassifier` (`:419-480`): `NewLLMClassifier`, `NewLLMClientAdapter`, `DefaultClassificationTimeout` |
| `gateway/graph-gateway/component.go` | consumer adapter (03B D4, `design.md:223`) | `ClassifierChain`, `NewClassifierChain`, `NewKeywordClassifier`, `EmbeddingClassifier`, `NewEmbeddingClassifier`, `DomainExamples`, `LoadDomainExamples`, `ClassificationResult` |
| `processor/agentic-governance/injection_classifier.go`, `injection_corpus/loader.go`; `cmd/measure-injection-classifier/main.go` | consumer adapter (agentic; semteams) | `EmbeddingClassifier`, `NewEmbeddingClassifier`, `FindBestMatch`, `Example`, `DomainExamples` |
| `processor/research-graph-classify/{component,handler,adapters}.go` | consumer adapter (agentic research) | `ClassifierChain`, `NewClassifierChain`, `NewKeywordClassifier`, `NewLLMClassifier`, `NewLLMClientAdapter`, `DefaultClassificationTimeout`, `ClassificationResult` |

Starter consumers (terms `graph/query`, `EmbeddingClassifier`, `ClassifierChain`, `UpgradeVectors`, `LLMClient`):
no Go use of any symbol. Hits, all prose: semsource `processor/supersession/prefixquery.go:28` (a comment naming the
retired `graph/query.Client`), `openspec/changes/archive/2026-07-31-expose-graphrag-on-mcp/design.md:56`
(`ClassifierChain` answers without an LLM), and `docs/testing/*/dependency-results.json` (closure listings);
semteams `docs/proposals/entity-extraction/README.md:149` names "`graph/query.LLMClient` (`classifier_llm.go:15`)" as
a seam a proposal builds on, which supports keeping the interface. semconnect, semboids: none. SemSource reaches the
chain only through `graph.query.globalSearch`.

Exported surface with no non-test reader anywhere at the pin (each by a pin-wide grep excluding `graph/query/`):
`UpgradeVectors` (`pin:…/classifier_embedding.go:190`); `InferStrategy` (`search_options.go:74`; test
`classifier_test.go:220`); `HasIndexFilters` (`:147`); `SetDefaults` (`:156`); `DefaultMaxCommunities` (`:30`);
`LoadAllDomainExamples` (`examples.go:51`); `Classifier` (`classifier.go:13`, one implementer, never named as a type);
`Embedder` (`classifier_embedding.go:17`, field type and `UpgradeVectors` parameter only); fields
`SearchOptions.RequireAllFilters`, `.Level`, `.MaxCommunities` (written only by `SetDefaults`, read by nothing). Read
in the package only: `Threshold` (chain, `classifier_chain.go:82`), `StrategyGraphRAG`, `StrategyHybridGraphRAG`,
`StrategyAggregation`, `Aggregation*` (classifier and LLM parse), `KeyTerms` (LLM parse into `key_terms`, which no
reader reads). Hints produced but read by no importer: `use_embeddings`, `aggregation_type`, `aggregation_field`,
`ranking_intent`, `limit`, `key_terms`, `predicates`; each still decides `hasExplicitIntent` (`classifier_chain.go:121-132`).

Behaviour a doc describes that no code implements: package doc `pin:…/classifier.go:1` — "Package query provides a
clean interface for reading graph data from NATS KV buckets." (the client is retired); `classifier_chain.go:51` —
"Returns nil if chain is nil or context cancelled." (only checked at entry, `:59-63`; a context ended later falls to
the default result, `:104-109`; the pin's own test accepts either, `classifier_chain_test.go:379-381`); `Tier`'s
"2=neural" (§ 2); `classifier_embedding.go:139` — "examples slice may have been replaced" (never reassigned).

## 5. The problem shape

A tiered fallback ladder whose tier 0 needs no provider and whose provider tier is excluded at this port. Closest
instance in the tree: `graph/embedding` — the `Embedder` seam (`graph/embedding/embedder.go:9`) kept, the HTTP
provider excluded (`docs/admission-ledger.yaml:3741`), held by `TestEmbeddingImportsNoProviderModule`
(`internal/harness/contract/embeddingprovider_test.go:20`). Second shape: a result that hands the caller shared
mutable state (§ 7 S9).

## 6. The pin's decision records

Fetched from the pin copy's `docs/adr` (85 files); `grep -l -i -e 'graph/query' -e ClassifierChain -e
KeywordClassifier -e EmbeddingClassifier -e LLMClassifier -e SearchOptions -e 'query classif'`, then titles read.
Excluded as false positives: ADR-060 and ADR-068, whose hits are `graph/query_contracts.go` and
`graph/query_index_types.go`, files of package `graph`.

| ADR | What it decides about this package |
| --- | --- |
| ADR-043 (Proposed, 2026-05-14) | prompt-injection defense in `agentic-governance` by reusing `EmbeddingClassifier`; its table lists `UpgradeVectors()` "(atomic swap BM25 → neural)" as shipped (`:54`) and phase 4 plans to wire it against semembed (`:233`). Decides nothing this package implements |
| ADR-045 (Phase 1 implemented) | research-graph reuses "the existing `graph/query.Classifier`" rather than duplicating it (`:28-38`; component table `:247`, "Calls `graph/query.Classifier.ClassifyQuery(topic)`"); the code reuses `ClassifierChain`. The pin's F1 guard keeps `Classifier`, `NewKeywordClassifier`, `SearchOptions`, `Example`, `LoadDomainExamples` as stable entry surfaces on purpose (`slice_f1_surface_contract_test.go:33-50`) |
| ADR-024 | layered LLM timeouts; the adapter's `DefaultClassificationTimeout` doc cites the capability timeout it reads |
| ADR-075 | admits the graph-research capability with its "classifier/query path" (`:38`) at SemStreams |
| ADR-079, 081, 084, 085, 090 | name `graph/query/client.go`, retired before the pin; nothing carried here implements them |

SemEngine's own: ADR-113 (context ownership; this package has no lifecycle owner), ADR-074 (canonical three-part
predicates; § 7 S10).

## 7. Shape sweep (pin copy)

| # | Shape | Finding |
| --- | --- | --- |
| S1 | second spelling | `query.Embedder` beside `embedding.Embedder` (§ 2) |
| S2 | second spelling | `cosineSimilarity` beside `embedding.CosineSimilarity` (§ 2) |
| S3 | second spelling | hint names in five places, two map writers with different key sets (§ 2) |
| S4 | second spelling, no reader | `InferStrategy` beside `resolveStrategy`; `DefaultMaxCommunities` twice (§ 2) |
| S5 | layered interface, one impl | `Classifier` (`classifier.go:13`, only `KeywordClassifier`); `Embedder` (only `bm25Adapter`, `classifier_embedding.go:39`) |
| S6 | exported, no reader | the § 4 list |
| S7 | invented root | `classifier_embedding.go:44` — `vecs, err := b.embedder.Generate(context.Background(), []string{text})`; `:55` — `return b.embedder.Generate(context.Background(), texts)` (#96) |
| S8 | silent degrade | `classifier_embedding.go:87-95` — `vectors, err := adapter.EmbedBatch(queries)` / `if err == nil {`: a failed build leaves every example without a vector, and the classifier never matches, with no error. Unreachable at the pin: under `context.Background()` the BM25 embedder's only errors (nil or ended context, `graph/embedding/bm25_embedder.go:146-157`) cannot occur; it becomes reachable when #96 threads the caller's context. `:91` — `if i < len(vectors) {`: a short result is tolerated without notice (BM25 always returns one vector per text) |
| S8b | silent degrade | `classifier_chain.go:94-101`: an LLM tier error falls to the default result with no log, metric or error ("LLM failure is non-fatal", `:100`) |
| S8c | silent degrade | `classifier_llm.go:212-215` — `if strategy := SearchStrategy(resp.Strategy); isValidStrategy(strategy) {`: an unknown strategy is dropped; `:222-229`: a time range that does not parse is dropped ("silently ignore parse failures to be lenient") |
| S8d | silent degrade | `classifier_embedding.go:134-137` — `queryVec, err := embedder.Embed(query)` / `return nil, 0.0`: an embedding error reads the same as "no match" |
| S8e | nil context | the chain (`classifier_chain.go:59-63`) and `FindBestMatch` (`classifier_embedding.go:121-125`) call `ctx.Done()` on a nil context and panic; `KeywordClassifier.ClassifyQuery` ignores its context (`classifier.go:86`, `_ context.Context`); `LLMClassifier.ClassifyQuery` passes a nil context to the client (`classifier_llm.go:77`) |
| S8f | learns from queries | `classifier_embedding.go:44` — `bm25Adapter.Embed` calls `Generate`, which folds each query into the corpus statistics; `GenerateQuery` (`graph/embedding/bm25_embedder.go:124`) is the read-only path (#96, its main point) |
| S9 | shared mutable result | `classifier_embedding.go:174` — `bestMatch = &examples[i]`; `classifier_chain.go:88` — `Options:    match.Options,`: a caller that edits the result edits the classifier's example. A consumer copies defensively for this reason: `pin:processor/research-graph-classify/handler.go:114-119`, test `handler_test.go:319-327` ("defensive copy missing") |
| S10 | domain vocabulary in the framework | `classifier.go:122` and `:129` — `opts.PathPredicates = []string{"located_in"}` (one part, not ADR-074's three). Routine trigger: any question with "zone X" or "area" followed by a word (`zonePattern`, `:44`; "the area of" gives start node `area-of`). Outcome in PR 9, which composes this tier: `tryPathIntentSearch` reads the predicate (`pin:processor/graph-query/graphrag.go:412-415`), resolves the start node (`:422`) and calls `executePathSearchForGlobal` with it (`:430`, `:2570-2577`, `Predicates: predicates`); a filter on a predicate no entity can carry finds nothing, and the search falls through with a Debug log only (`:470-471`). When the start node does not resolve and no community cache is composed (tier 0), PR 9 answers an empty `pathrag` result and stops (`:475-486`), so "the area of the plant" answers nothing. `:81` — the entity pattern (`word-alnum`, e.g. `sensor-001`) yields a partial ID that PR 9 resolves (#405) |
| S11 | mixed vector spaces | `FindBestMatch` reads the embedder under one read lock (`:128-131`) and the vectors under another (`:140-144`); an `UpgradeVectors` between them compares a BM25 query with neural examples. No production caller of `UpgradeVectors` exists (S6) |
| S12 | weak oracles | the temporal tests assert `NotNil` only (`classifier_test.go:255`, `:262`), because of S14; the cancellation tests accept either outcome (`classifier_chain_test.go:379-381`) |
| S13 | typed hints lost by tier | embedding-tier `Options` come from JSON (P4): `path_predicates`, `types`, `time_range`, `geo_bounds`, `limit` fail the reader's type assertions (`pin:processor/graph-query/graphrag.go:413`, `:546`, `:1197`, `:1265`) silently |
| S14 | hidden clock | `classifier.go:225-303`: every range calls `time.Now()` and uses `now.Location()`, so "today" and "yesterday" follow the host's time zone and no test can fix the instant |
| S15 | unbounded numbers | `last N hours/days` (`:177-188`) parse any `int`, and `getLastNHoursRange` (`:248`) — `start := now.Add(-time.Duration(hours) * time.Hour)` overflows `time.Duration` past about 2,562,047 hours (days: `:258`, ×24); `extractSpatialBounds` (`:320-367`) never range-checks latitude or longitude, so `lat` near ±90 makes `cosDegrees` near 0 and the longitude delta unbounded, and a latitude beyond 90 degrees either way inverts East and West. Trigger: user text; none routine |
| — | classification by error text | none (`grep -n 'Error()\|strings.Contains'` over non-test files: none) |
| — | copied shell | none (no lifecycle) |

`GenericJSON`: none (`grep -n 'GenericJSON\|message\.'`: none). Context retention: none (no struct field of type
`context.Context`). Collision table: not triggered (no durable, communication or coordination primitive).

## 8. Adopter seam inventory

Reached from outside by `processor/graph-query` (PR 9) and by products that build their own classifier (the pin's
gateway and governance adapters).

1. What must they know? (a) `Options` keys are string literals and each value's dynamic type depends on the tier
   that produced it (P4, S13); (b) the result aliases the classifier's examples (S9); (c) a nil result means a nil
   chain or an ended context, but at the pin an ended context can also yield a confident default (§ 4); (d) the
   embedding tier stops matching if its build failed (S8); (e) a nil context panics the chain (S8e); (f) a zone or
   area question adds a predicate filter no entity can match (S10). Six items: a design finding.
2. If they do nothing: a temporal or spatial hint from the embedding tier is dropped and the search runs as
   `graphrag`; an edit to the result changes later answers; a cancelled request looks like "no intent"; "sensors in
   zone A" finds nothing by path and falls through.
3. Where they find out: nowhere for (a), (b), (d), (f); a panic for (e); a doc that the code contradicts for (c).
4. What they should have to know: nothing. The gap is (a)-(f).

Intent check: the change moves one boundary, the adapter's tier, ruled on #9 comment 6386603659. Query is admitted at
tier 0 (`docs/tier1-cross-check.md:68`); the LLM classification tier is deferred to #32 by that ruling. No other
capability in `AGENTS.md` "What this is for" changes membership here.
