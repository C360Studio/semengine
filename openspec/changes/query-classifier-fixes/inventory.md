# Inventory: query-classifier-fixes

base: `a5f1d0e` (the claim commit on `claude/query-classifier-fixes`, on `origin/main` `d80acfd`, which merged
`graph/query` as PR #475; the claim adds no file). Baseline is the code on main, not the SemStreams pin. Pin paths
(SemStreams `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`), written `pin:<path>:<line>`, are read only for how
`processor/graph-query` (change 4 PR 9, the future reader of hints) reads a hint, from the pin copy at
`/tmp/gq-pin/src` (local only); `pin:processor/graph-query/graphrag.go` is blob-equal to the pin
(`git hash-object` = `gh api …contents/processor/graph-query/graphrag.go?ref=8b99efe9 --jq .sha` =
`0ddc205c05fbef2f25fd7c34dff6e1e3d7c5afe2`).

Status: round 1, inventory. Kind of change: repairs to one carried package, `graph/query`, by the owner's sequencing
(#94 comment 6036107878; #95 comment 6036108178; #96 comment 6036129531; #478 body, ruling #9 comment 6522523604
answer 3; #479 comment 6525285377: items 1 and 2 only).

## Probes (go1.26.10 darwin/arm64, at `a5f1d0e`, local only)

Each probe is an extra test file passed with `go test -overlay` (the tree is not written); sources in
`/tmp/qc-probe/`.

- Q1, the named cases through `NewClassifierChain(NewKeywordClassifier(), nil)`: R14 `map[]`; R17
  `aggregation_field:of aggregation_type:avg`; R21 `aggregation_field:of aggregation_type:sum`; R23, R24, R26, R27
  `map[]`; R28 `aggregation_type:count`; R29 `use_embeddings:true`; R30 `aggregation_field:temperature
  aggregation_type:avg`; R31 `aggregation_type:count`; R32 `path_intent:true` (no start node). Controls: "Do not
  count sensors; average pressure" `aggregation_type:count` (should be avg); "What is the total number of pumps"
  count; "average temperature of sensors" avg/temperature; "which sensor has the highest temperature" `map[]`;
  "top 5 sensors by temperature" `limit:5 ranking_intent:true`. Every named case reproduces on main.
- Q2 (#96): two classifiers over the same three examples; one gets 50 unrelated `FindBestMatch` calls first; the
  same query then scores `0.578967` against `0.707107` on the untouched one.
- Q3 (#479 item 2): "last 3000000 hours" gives `start=2269-02-22`, after `end` (the duration wrapped); "last 120000
  days" `start=2282-11-01`, after `end`; "within 5km of 89.9999,10" `East:25818.9 West:-25798.9 North:90.04`; "within
  5km of 95,10" `North:95.05` and `East:9.48 < West:10.52`; "within 5km of 10,500" `East:500.05`; "within 50000km
  of 0,0" `North:450.45`.

## 1. The claimed gaps

- #94 field binding: `graph/query/classifier.go:77` — ``var aggregationFieldPattern =
  regexp.MustCompile(`(?i)\b(?:average|avg|mean|sum|total|min|max|minimum|maximum)\s+(\w+)\b`)`` binds the next word,
  so `of` (Q1). Ordinary phrases: `:57` — ``countPattern = regexp.MustCompile(`(?i)\b(how\s+many|total\s+number\s+of|count)\b`)``,
  `:63` — ``minPattern = regexp.MustCompile(`(?i)\b(minimum|min)\b`)``, `:64` — ``maxPattern = …(maximum|max)…``:
  no `number of`, `smallest`, `lowest`, `largest`, `highest`. `git grep -n -i 'smallest\|lowest\|largest\|highest'
  -- graph/query`: no hit.
- #95: no negation, quotation or alternative handling. `git grep -n -i 'negat\|quot\|undecid\|exclu' --
  graph/query/*.go`: no non-test hit. The chain stops on any keyword hint, `classifier_chain.go:69-79`, and otherwise
  takes tier 1 (`:83-93`) or tier 3 (`:96-102`) unchecked; the keyword tier exposes only `*SearchOptions`
  (`classifier.go:85` — `func (k *KeywordClassifier) ClassifyQuery(_ context.Context, query string) *SearchOptions {`).
- #96: `classifier_embedding.go:104` — `vecs, err := c.embedder.Generate(ctx, []string{query})`. The read-only path
  exists: `graph/embedding/bm25_embedder.go:124` — `func (b *BM25Embedder) GenerateQuery(ctx context.Context, texts
  []string) ([][]float32, error) {`, doc `:110-122` ("READ-ONLY over the corpus statistics"); same vector math,
  `embed(ctx, texts, false)` (`:125`). No production caller of `GenerateQuery` in the tree (`git grep -n
  'GenerateQuery(' -- '*.go'`: the definition and `embedder.go:24` only). Construction stays on `Generate` (`:65`).
- #478: `examples.go:43` — `if err := json.Unmarshal(data, &examples); err != nil {` fills `Options map[string]any`
  (`:13`); the chain passes them through as is, `classifier_chain.go:89` — `Options:    match.Options,`. README's
  table records the split, `graph/query/README.md:49-63`. No typing anywhere: `git grep -n 'TimeRange\b' --
  graph/query/classifier_embedding.go graph/query/examples.go`: none.
- #479 item 1: `time.Now()` at `classifier.go:223`, `:234`, `:245`, `:255`, `:265`, `:279`, `:293`; zone from
  `now.Location()` (`:224-225`, …). No clock field or argument (`KeywordClassifier struct{}`, `:19`).
- #479 item 2: `:176-177` — `if hours, err := strconv.Atoi(matches[1]); err == nil {` / `return
  k.getLastNHoursRange(hours)`; `:246` — `start := now.Add(-time.Duration(hours) * time.Hour)`; `:256` — `start :=
  now.Add(-time.Duration(days) * 24 * time.Hour)`; `:344-357` parse lat/lon with no range check and `:357` —
  `lonDelta := radius / (111.0 * cosDegrees(lat))`.

## 2. Every current spelling of the facts modelled

| Fact | Spellings |
| --- | --- |
| Hint name → Go type | `SearchOptions` JSON tags (`search_options.go:30-51`); `searchOptionsToMap` (`classifier_chain.go:143-183`, tier 0's subset); `llmOptionsToMap` (`classifier_llm.go:291`, tier 3's subset, `strategy` as `string` at `:299`); README table (`README.md:49-63`); the pin reader's assertions (below) |
| Aggregation trigger words | `classifier.go:57-64` and again in `:77` (field pattern lists the same words) |
| Which hints count as "intent" | `hasExplicitIntent`, `classifier_chain.go:127-138` |
| "Now" | seven `time.Now()` calls, `classifier.go:223-293`; tier 3 parses absolute RFC 3339 times (`classifier_llm.go` `parseLLMResponse`, `:204`) |
| Lat/lon range | none in `graph/query`; `git grep -n -i 'latitude\|> 90\|-90' -- '*.go'`: no validator in the tree |

Pin reader of hints, `processor/graph-query/graphrag.go` (PR 9, carried whole): `:406` —
`pathIntent, _ := cr.Options["path_intent"].(bool)`; `:407` `path_start_node` `.(string)`; `:413` — `if pp, ok :=
cr.Options["path_predicates"].([]string); ok {`; `:530` `query` `.(string)`; `:546` `types` `.([]string)`; `:639`
`strategy` `.(string)`; `:658-659` `time_range`/`geo_bounds` `!= nil`; `:1006` `path_start_node`; `:1197` —
`timeRange, _ := cr.Options["time_range"].(*query.TimeRange)`; `:1265` — `bounds, _ :=
cr.Options["geo_bounds"].(*query.SpatialBounds)`. Comma-ok assertions, no type switch. `limit` is not read.
Other pin readers (consumer adapters, not ported): `gateway/graph-gateway/component.go:1482` ranges the map into a
JSON payload; `processor/research-graph-classify/handler.go:117-119` copies it; `processor/agentic-governance/
injection_classifier.go:163` reads `Example.Options` by its own keys (`id`, `source`). Pin example files
(`configs/domains/{iot,logistics,robotics}.json`) use caller keys (`entity_pattern`, `status_filter`, `operation`,
…) and one known name, `strategy`, always a string (`"pathrag"`).

## 3. Adjacent claims

- Spec `openspec/specs/query-classification/spec.md` (seven requirements; the chain requirement is modified here).
  No other spec names classification (`git grep -l -i classif openspec/specs`: this one).
- Ledger `docs/admission-ledger.yaml:3760` (`graph/query`, `adapt`): `known_risks` names `Generate`, hint types,
  host clock and unbounded numbers; the row is edited here. `:3817` adapter row: untouched.
- In-tree importers of `graph/query`: none (`git grep -n '"github.com/c360studio/semengine/graph/query"' -- '*.go'`
  outside the package: none). Guard: `internal/harness/contract/embeddingprovider_test.go:35`
  (`TestQueryImportsNoProviderModule`); this change adds no import.
- Open pull requests (`gh pr list --state open`, 2026-10-30): #480 (this claim, 0 files); #476 draft graphview, 24
  files under `pkg/graphview/` and its own change, no shared file today (it may add a ledger row and a
  `docs/repository-map.md` row; this change edits the `graph/query` ledger row only); #464 `go.mod`/`go.sum`
  (nats.go; this change adds no module); #465 `package.json`/lock. No capability overlap.

### Open issues naming the package

Queries `gh issue list --state open --search query` (28) and `--search classifier` (9). Kept:

| Issue | Names the package |
| --- | --- |
| #94, #95, #96, #478 | bodies (`graph/query/...` paths) |
| #479 | body, items 1-3 (`classifier.go:44`, `:177-188`, `:225-303`, `:320-367`) |
| #32 | adapter home (`graph/query/classifier_llm_adapter.go`) |
| #105 | `graph/query` as `processor/graph-query`'s import |
| #9 | epic, rulings |

Names the word, not the package: #405, #31, #406, #103, #415, #106, #16, #460, #111, #300, #108, #155, #109,
issues #477, #461, #68, #11, #350, #24, #195, #196, #7, #20.

## 4. Who it is for (new surface)

The change plans no new exported symbol (to be held by the design). Tier-1 typing changes what existing framework
API returns (`FindBestMatch`, `ClassifyQuery`): contract line `README.md` hint table and the ledger row.

## 5. Problem shape

(a) Decode untyped input into a declared type at a construction seam, refusing what does not decode: closest
instance, ported components refusing unknown config keys at construction (`AGENTS.md` rule, `component-registration`
spec) and `LoadDomainExamples`' refusal of invalid JSON (`examples.go:43`). (b) A ladder where an earlier stage's
finding constrains later stages: no instance in the tree (`git grep -n -i 'exclu' -- graph/ pkg/`: no ladder).
(c) A clock. The one instance in the tree is a test hook: `model/breaker.go:35-37` — `// Now is the time source.
Tests inject a clock; production leaves` / `// this nil and time.Now is used.` / `Now func() time.Time` (set only by
`model/breaker_test.go:82`, `:118`, `:155`). No production function takes `now time.Time` (`git grep -n -E '\bnow
time\.Time' -- '*.go' ':!*_test.go'`: none).

## 6. Decision records

No package is carried from the pin; the pin's ADRs were read for the port (archived inventory § 6). Still binding:
ADR-045 (pin) keeps `Classifier` with `ClassifyQuery(ctx, query) *SearchOptions` (`classifier.go:12-15`), so that
signature cannot take a `now`; ADR-074 (`docs/adr/074-canonical-predicate-contract.md`): no predicate is added here;
ADR-113: no lifecycle owner in the package (no goroutine, `git grep -n 'go func' -- graph/query`: none).

## 7. Shape sweep (touched code, on main)

| # | Shape | Finding |
| --- | --- | --- |
| T1 | second spelling | hint name → type in four writers/tables (§ 2): S3 of the port, still open |
| T2 | second spelling | aggregation words listed twice (`classifier.go:57-64`, `:77`) |
| T3 | hidden input | seven `time.Now()` calls (§ 1) |
| T4 | silent degrade | overflowing duration yields an inverted range (Q3); invalid lat/lon yields a box with North > 90 or East < West |
| T5 | silent degrade | a tier-1 hint of the wrong type is dropped by the reader's comma-ok assertion |
| — | classification by error text, copied shell, one-impl interface, context retention | none (`grep -n 'Error()\|strings.Contains\|context.Context$'` over the non-test files: none) |

Collision table: not triggered (no durable, communication or coordination primitive).

## 8. Adopter seam

Reached from outside by PR 9 and by products composing the chain or an `EmbeddingClassifier`.

1. Must know today: (a) a tier-1 value's type depends on JSON (README table); (b) scores drift with query traffic;
   (c) "today" follows the host zone; (d) a negated or quoted operation still becomes a hint; (e) huge or
   out-of-range numbers yield inverted ranges or impossible boxes. Five items.
2. If they do nothing: PR 9 ignores tier-1 `time_range`, `geo_bounds`, `types`, `path_predicates` silently; a search
   for "documents containing \"average temperature\"" runs an aggregation; an impossible range finds nothing.
3. Where they find out: README for (a)-(c); nowhere for (d), (e).
4. Should know: nothing beyond the hint table.
