# Design: setup-04a-04-graph-roots-query

Base: `50e2e90` (claim commit on `d3c1e13`); the branch merges `origin/main` (`c010d0b`, #474, `AGENTS.md` only)
before review. Inventory: `inventory.md` (round 1; this design is a draft until `INVENTORY PASS`). Change 4, PR 7 of
9 (#9 comment 6386856657, item 6). Tier 0 only (#9 comment 6386603659). Draft PR #475. Governing records: none binding
(inventory 6); ADR-043's `UpgradeVectors` plan is changed (D2, owner question 1); ADR-074 (SemEngine) for S10.
Probes: inventory P1-P4.

Coined terms: a *hint* is one entry of `ClassificationResult.Options` (`time_range`, `path_start_node`, …); a *tier*
is one stage of the chain: 0 keyword, 1 example classifier, 3 LLM (no code produces 2); the *default result* is tier
0 with no hints and confidence 1.0, returned when no composed tier produced a hint.

## Options

- **A. One pull request, the package less the adapter, with the corrections D3-D6 (recommended).** 1,429 pin lines.
- **B. Carry the pin as is, less the adapter.** Keeps `context.Background()` in production (S7), which the order
  ruling sends to this PR (#96) and the context rules forbid; keeps the silent build failure (S8) that #96's context
  makes reachable.
- **C. Also do the classifier follow-up now** (#96's `GenerateQuery`, #94, #95). The 2026-10-07 dispositions on #94 and
  #95 put them in one follow-up change "so the diff from the pin is the fix alone"; the drift note on #96
  (6386548133) keeps `GenerateQuery` there. Not taken.
- **D. Typed hints now** (`ClassificationResult` carries `*SearchOptions`; example files decoded into it at load,
  unknown keys refused). Closes S13 and adopter fact (a), but changes the result every consumer reads, and #95's
  follow-up already redesigns each tier's outcome (hint, no-hint, excluded). Recommended for that follow-up (owner
  question 3); issue filed (D9).
- **E. Do nothing.** `processor/graph-query` (item 9) cannot build.

## D1. Files

`graph/query/`: `doc.go` (new; the package doc moves out of `classifier.go:1` with true text), `README.md` (new; the
contract line for the framework API, D2), `classifier.go`, `classifier_chain.go`, `classifier_embedding.go`,
`classifier_llm.go`, `examples.go`, `search_options.go`, tests. Not carried: `classifier_llm_adapter.go` and
`classifier_llm_adapter_test.go` (ruling); `classifier_llm_live_test.go` (`live_llm` tag, `t.Skip`, a live Ollama: a
tier-1 test, #32's); `slice_f1_surface_contract_test.go` (guards a client SemEngine never had and reads packages this
tree does not carry; a real-tree read belongs in `contract`).

## D2. Surface

Kept as at the pin: `KeywordClassifier`, `NewKeywordClassifier`, its `ClassifyQuery`; `ClassifierChain`,
`NewClassifierChain` (variadic LLM tier kept), `ClassifyQuery`; `ClassificationResult`; `EmbeddingClassifier`,
`FindBestMatch`, `Threshold`; `LLMClient`, `LLMClassifier`, `NewLLMClassifier` (the ruling keeps the interface, and
`LLMClassifier` is its one reader; it imports no provider); `Example`, `DomainExamples`, `LoadDomainExamples`;
`SearchOptions` (less three fields), `TimeRange`, `SpatialBounds`, `SearchStrategy` and its eight values, the five
`Aggregation*` names. Framework API by pin consumer-adapter callers (inventory 4): `EmbeddingClassifier`, its
constructor and `FindBestMatch`, `Example`, `DomainExamples`, `LoadDomainExamples`, `NewLLMClassifier`; their contract
line is the ledger row's `contract` field and the README, with a test that calls each as a product would.

Changed: `NewEmbeddingClassifier(ctx context.Context, domains []*DomainExamples, threshold float64)
(*EmbeddingClassifier, error)` (D3).

Not carried (no non-test reader at the pin, inventory 4): `UpgradeVectors`, `InferStrategy`, `HasIndexFilters`,
`SetDefaults`, `DefaultMaxCommunities`, `LoadAllDomainExamples`, `Classifier`, `Embedder`, `bm25Adapter`, the fields
`RequireAllFilters`, `Level`, `MaxCommunities`, and `cosineSimilarity` (`embedding.CosineSimilarity` instead: same
arithmetic, inventory 2; optional, taken because the package already imports `embedding`). ADR-043 (Proposed, a
semteams concern) planned `UpgradeVectors` for a neural swap; dropping it is owner question 1. With it gone the
examples never change after construction, so S11 cannot occur.

## D3. Context (#96, first half)

The example classifier holds `*embedding.BM25Embedder`. Construction and `FindBestMatch` pass the caller's context
to `Generate`; no production line creates a root (architect contract, "Root creation stays at composition"; ADR-113's
one exception is a lifecycle owner's run context, and this package has none). `Generate` stays on the query path:
replacing it with `GenerateQuery` is #96's second half, in the follow-up. `NewEmbeddingClassifier` returns an error,
and no classifier, when the context is nil or has ended or any vector fails (S8: at the pin a failed build left every
example without a vector, so the classifier silently never matched).

## D4. Outcomes, including the terminal ones

| Event | `ClassifierChain.ClassifyQuery` result |
| --- | --- |
| nil chain; nil context; context already ended | `nil` |
| keyword hint | tier 0, the keyword hints, confidence 1.0 |
| no keyword hint; example match at or above threshold | tier 1, a copy of the example's options, the score |
| example tier composed and the context ends during it | `nil` (pin: the default result) |
| LLM tier returns a result | tier 3 |
| LLM tier fails (client error, unparsable reply) and the context is live | the default result (declared at the pin, `classifier_chain.go:100`) |
| LLM tier fails because the context ended | `nil` |
| no composed tier produced a hint, context live | the default result |

`FindBestMatch` returns `(nil, 0)` for a nil classifier, a blank query, a nil or ended context, or an embedding error,
and `(nil, best)` below the threshold; ties go to the first example in load order. `LoadDomainExamples` refuses an
empty path, a missing or empty file and invalid JSON. Every returned `Example` and every result's `Options` map is a
copy: editing it changes no later result (S9; integrity gate, the pin's own consumer copies defensively). Nested
values inside `Options` are not copied; the README says so.

## D5. Docs (surface audit (c))

Package doc: what the package classifies, its tiers, that it reads no store. `Tier`: 0, 1, 3. The chain's doc states
D4's table. `EmbeddingClassifier`: BM25 only, examples fixed at construction. README: the hint names with each
value's type per tier, saying the example tier's values are what JSON decoding gives (P4), and that hints no
`processor/graph-query` code reads (`use_embeddings`, `aggregation_*`, `ranking_intent`, `limit`, `key_terms`,
`predicates`) still stop the ladder at tier 0.

## D6. Retained optional findings

S3 (hint names in five places), S5's `LLMClient` (one implementation, kept by ruling), S10 and S13 (filed, D9), S12
(the pin's weak oracles; the new tests in D8 carry their own oracles), S4's `resolveStrategy` (PR 9's). Changing any
is optional maintenance under Bounded port scope: no binding requirement names it and no in-tree caller trips it
(`processor/graph-query` composes no example tier, inventory 4).

## D7. Ledger, records

- `graph/query` (`adapt`): contract = D2-D4; dependencies `graph/embedding` (in-set); no NATS, no goroutine, no
  provider (`TestQueryImportsNoProviderModule`); known risks: `Generate` learns from queries until #96's follow-up;
  hints lose their types on the example tier (S13, filed); proving tests D8; not carried as D1, D2.
- `graph/query/classifier_llm_adapter.go` (`defer-exclude`): known risks name #32; its test and the live test go with
  it; `DefaultClassificationTimeout` with it.
- ADR-084 and ADR-085 port notes: "`graph/query`" becomes "`graph/query`'s client (retired before the pin)".
  `docs/repository-map.md`: a `graph/query` row. No `cover-check` line (inventory, first bullet).

## D8. Proving tests

Unit, no `time.Sleep` (the pin's four go: two with `UpgradeVectors`, two become an already-ended context). Ported:
the keyword, chain, example, LLM-parse and loader tests, less those of dropped surface (`TestUpgradeVectors*`,
`TestKeywordClassifier_InferStrategy_Aggregation`, `TestLoadAllDomainExamples_*`, `TestCosineSimilarity`). New:

- `TestChainEndedContextReturnsNil`: an ended context at entry gives `nil`; a fake `LLMClient` that ends the context
  and returns `ctx.Err()` gives `nil`, not the default; one that returns a plain error under a live context gives the
  default. Shown failing against the pin's chain (it returns the default) in `notes/`.
- `TestChainKeywordHintStopsTheLadder`: a keyword hint returns tier 0 and a counting fake `LLMClient` is never called.
- `TestNewEmbeddingClassifierRefusesEndedContext`: nil and ended contexts return an error and no classifier.
- `TestFindBestMatchEndedContext`: `(nil, 0)`.
- `TestResultEditsDoNotReachClassifier`: edit a result's `Options` and the returned `Example`; the same query then
  returns the original hints and score. Fails against the pin's aliasing.
- `TestEmbeddingClassifierAsAProduct`: loads two example files with `LoadDomainExamples`, builds the classifier,
  composes it into a chain, and reads a tier-1 result (the framework-API test, D2).
- `TestQueryImportsNoProviderModule` in `internal/harness/contract`: `go list -deps ./graph/query` names neither
  `go-openai` nor `model`, using the embedding guard's helper with its own requirement name; the existing sensitivity
  test covers the helper.

Examples suffice: the inputs do not interact beyond D4's table, which the tests enumerate.

## D9. Issues and merge order

| Issue | Disposition |
| --- | --- |
| #96 | advanced: its context half (D3); stays open for `GenerateQuery` in the classifier follow-up |
| #94, #95 | stay: optional here; the owner's 2026-10-07 dispositions place them in the follow-up after this port |
| #32 | stays: home of the adapter's row |
| #105 | stays: `processor/graph-query`'s split, PR 9 |

Filed by this design (linked without a closing keyword): S10, the hard-coded `located_in` predicate and sensor-shaped
entity pattern (ADR-074; consumer vocabulary in the framework), home the classifier follow-up; S13, hints lose their
types on the example tier, home the same follow-up (owner question 3). Merge order: #476 (graphview, draft) shares
`docs/admission-ledger.yaml` and `docs/repository-map.md`; whichever merges second merges `origin/main` and re-runs
`task verify`. #464 and #465 share no file. Nothing waits on another.
