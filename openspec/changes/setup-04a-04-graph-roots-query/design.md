# Design: setup-04a-04-graph-roots-query

Base: `d522bf6` (the branch with `origin/main` `c010d0b` merged). Inventory: `inventory.md` (round 2; this design is
a draft until `INVENTORY PASS`). Change 4, PR 7 of 9 (#9 comment 6386856657, item 6). Tier 0 only (#9 comment
6386603659). Draft PR #475. Governing records: none binding (inventory 6); ADR-043's `UpgradeVectors` plan changes
(owner question 1); ADR-045 and the pin's F1 guard keep `Classifier` (D2); ADR-074 (SemEngine) for D5. Probes:
inventory P1-P4.

Coined terms: a *hint* is one entry of `ClassificationResult.Options` (`time_range`, `path_start_node`, …); a *tier*
is one stage of the chain: 0 keyword, 1 example classifier, 3 LLM (no code produces 2); the *default result* is tier
0 with no hints and confidence 1.0, returned when no composed tier produced a hint.

## Owner questions

1. Drop `UpgradeVectors`, the swap from BM25 to neural example vectors? Nothing calls it at the pin or in any starter
   consumer; SemStreams ADR-043 (Proposed, semteams' prompt-injection work) planned to. Recommended: drop; tier 1
   (#32) brings a neural path back if wanted.
2. Accept the changed constructor `NewEmbeddingClassifier(ctx, domains, threshold) (*EmbeddingClassifier, error)`
   (D3). No starter consumer calls it; `processor/graph-query` builds no example classifier.
3. Leave hint values untyped in this PR (Option D)? Recommended: yes; the follow-up with #94 and #95 redesigns each
   tier's outcome and fixes the types there (issue filed, D9).

## Options

- **A. One pull request, the package less the adapter, with D3-D5 (recommended).** 1,429 pin lines.
- **B. Carry the pin as is, less the adapter.** Keeps `context.Background()` in production (S7), which the order
  ruling sends to this PR (#96) and the context rules forbid; keeps the silent build failure (S8) that #96's context
  makes reachable, and the routine empty path search of S10.
- **C. Also do the classifier follow-up now** (#96's `GenerateQuery`, #94, #95). The owner's 2026-10-07 dispositions
  put them in one follow-up "so the diff from the pin is the fix alone"; #96's drift note (6386548133) keeps
  `GenerateQuery` there, and the order ruling scopes item 6 to "context threading". Not taken, though D3 edits that line.
- **D. Typed hints now** (`ClassificationResult` carries `*SearchOptions`; example files decoded at load, unknown
  keys refused). Closes S13 but changes the result every consumer reads, while #95's follow-up already redesigns tier
  outcomes. Owner question 3.
- **E. Do nothing.** `processor/graph-query` (item 9) cannot build.

## D1. Files

`graph/query/`: `doc.go` (new; the package doc leaves `classifier.go:1` with true text), `README.md` (new; the
framework-API contract line, D2), `classifier.go`, `classifier_chain.go`, `classifier_embedding.go`,
`classifier_llm.go`, `examples.go`, `search_options.go`, tests. Not carried: `classifier_llm_adapter.go` and its test
(ruling); `classifier_llm_live_test.go` (`live_llm` tag, `t.Skip`, live Ollama: #32's); `slice_f1_surface_contract_test.go`
(guards a client SemEngine never had and reads packages this tree does not carry).

## D2. Surface

Kept as at the pin: `Classifier` (one implementer and no reader, kept because ADR-045 names it as the primitive to
reuse, `:28-38`, `:247`, and the pin's F1 guard keeps it on purpose, `slice_f1_surface_contract_test.go:44`),
`KeywordClassifier`, `NewKeywordClassifier`; `ClassifierChain`, `NewClassifierChain`, `ClassificationResult`;
`EmbeddingClassifier`, `FindBestMatch`, `Threshold`; `LLMClient`, `LLMClassifier`, `NewLLMClassifier` (the ruling
keeps the interface; semteams' entity-extraction proposal builds on it, inventory 4); `Example`, `DomainExamples`,
`LoadDomainExamples`; `SearchOptions` (less three fields), `TimeRange`, `SpatialBounds`, `SearchStrategy` and its eight
values, the five `Aggregation*` names. Framework API (pin consumer-adapter callers, inventory 4, plus `Classifier`):
contract line in the ledger row and README, each with a test that calls it as a product would.

Not carried (no non-test reader at the pin): `UpgradeVectors`, `InferStrategy`, `HasIndexFilters`, `SetDefaults`,
`DefaultMaxCommunities`, `LoadAllDomainExamples`, `Embedder`, `bm25Adapter`, the fields `RequireAllFilters`, `Level`,
`MaxCommunities`, and `cosineSimilarity` (`embedding.CosineSimilarity` instead: same arithmetic; optional, taken as
the package already imports `embedding`). Without `UpgradeVectors` the examples never change, so S11 cannot occur.

## D3. Context and construction (#96, first half)

The example classifier holds `*embedding.BM25Embedder` and passes the caller's context to `Generate` at construction
and in `FindBestMatch`; no production line creates a root (architect contract, "Root creation stays at composition";
ADR-113's one exception is a lifecycle owner's, and this package has none). `Generate` stays on the query path (S8f,
the second half of #96, Option C). `NewEmbeddingClassifier` returns an error, and no classifier, for a nil or ended context,
an embedding error, or fewer vectors than examples (S8: unreachable at the pin, reachable once the context is threaded).
`LLMClassifier.ClassifyQuery` refuses a nil context with an invalid-input error before calling the client (S8e; an
exported context boundary that can return an error).

## D4. Outcomes, including the terminal ones

| Event | `ClassifierChain.ClassifyQuery` |
| --- | --- |
| nil chain | `nil` |
| nil context | `nil` (pin: panics, S8e) |
| context already ended | `nil` |
| keyword hint | tier 0, the keyword hints, confidence 1.0 |
| no keyword hint; example match at or above threshold | tier 1, a copy of the example's options, the score |
| context ends during a later tier that produced nothing | `nil` (pin: the default result) |
| LLM tier returns a result | tier 3 |
| LLM tier fails, context live | the default result; the doc says so (S8b: no tier-0 trigger, no LLM client in the tree) |
| no composed tier produced a hint, context live | the default result |

`FindBestMatch` returns `(nil, 0)` for a nil classifier, a blank query, a nil or ended context (pin: a nil context
panics) and, after D3, any embedding error, which only an ended context causes (S8d); `(nil, best)` below the
threshold; ties go to the first example in load order. `LoadDomainExamples` refuses an empty path, a missing or empty
file and invalid JSON. Every returned `Example` and result `Options` map is a copy: editing it changes no later result
(S9; integrity gate; the pin's own consumer copies defensively). Nested values inside `Options` are not copied; the
README says so.

## D5. The keyword tier stops adding `located_in` (S10)

A zone or area phrase keeps its path intent and start node but adds no predicate filter. Basis: SemEngine owns no
consumer's domain semantics (`AGENTS.md`, "What this is for"); ADR-074's predicates have three parts; and the trigger
is routine in the tier PR 9 composes, where the filter makes every such path search come back empty (inventory S10).
The start-node half (bare "area X" becoming `area-X`, and PR 9's empty answer for an unresolved start at tier 0) is
parser and PR 9 work: filed (D9) and named to PR 9.

## D6. Docs and retained findings

Docs (surface audit (c)): package doc (what it classifies, its tiers, no store); `Tier` 0, 1, 3; the chain doc states
D4, including the LLM failure; `EmbeddingClassifier`: BM25 only, examples fixed; `LLMClassifier`: an unknown strategy
and an unparsable time range are dropped (S8c); README: hint names with each value's type per tier (P4), and that
hints PR 9 does not read still stop the ladder at tier 0. Retained as optional (Bounded port scope; no binding
requirement names them and no routine trigger reaches them): S3, S4's `resolveStrategy` (PR 9's), S8b and S8c beyond
the doc sentence, S12, S13, S14, S15. S13, S14 and S15 are filed (D9).

## D7. Ledger, records

- `graph/query` (`adapt`): contract D2-D5; dependencies `graph/embedding` (in-set; nats.go through it); no goroutine,
  no provider (`TestQueryImportsNoProviderModule`); known risks: `Generate` learns from queries until #96's follow-up;
  hints lose types on the example tier (S13); host-clock ranges and unbounded numbers (S14, S15); proving tests D8.
- `graph/query/classifier_llm_adapter.go` (`defer-exclude`): known risks name #32; its test, the live test and
  `DefaultClassificationTimeout` go with it.
- ADR-084 and ADR-085 port notes: "`graph/query`" becomes "`graph/query`'s client (retired before the pin)";
  `docs/repository-map.md` row. No `cover-check` line (not on the 03B D10 list).

## D8. Proving tests

Unit, no `time.Sleep` (two of the pin's four go with `UpgradeVectors`; two become an already-ended context). Ported:
the keyword, chain, example, LLM-parse and loader tests, less dropped surface (`TestUpgradeVectors*`,
`TestKeywordClassifier_InferStrategy_Aggregation`, `TestLoadAllDomainExamples_*`, `TestCosineSimilarity`). New, each
changed behaviour shown failing against the pin's shape in `notes/`:

- `TestChainKeywordHintStopsTheLadder`: tier 0, and a counting fake `LLMClient` is never called.
- `TestChainEndedContextReturnsNil`: nil and ended contexts give `nil`; a fake `LLMClient` that ends the context and
  returns `ctx.Err()` gives `nil`; one returning a plain error under a live context gives the default result.
- `TestLLMClassifierRefusesNilContext`; `TestNewEmbeddingClassifierRefusesEndedContext` (nil, ended);
  `TestFindBestMatchEndedContext` (nil, ended: `(nil, 0)`).
- `TestResultEditsDoNotReachClassifier`: edit a result's `Options` and the returned `Example`; the same query returns
  the original hints and score.
- `TestZoneQueryAddsNoPredicate`: "sensors in zone A" and "zone-cold-storage" give path intent, the start node, and no
  `path_predicates` hint.
- `TestEmbeddingClassifierAsAProduct`: two files through `LoadDomainExamples`, a classifier, a chain, a tier-1 result;
  `Classifier` held as an interface value.
- `TestQueryImportsNoProviderModule` (`internal/harness/contract`): the embedding guard's helper on `./graph/query`
  with its own requirement name.

Examples suffice: the inputs interact only as D4's table, which the tests enumerate.

## D9. Issues and merge order

| Issue | Disposition |
| --- | --- |
| #96 | advanced: its context half (D3); stays open for `GenerateQuery` |
| #94, #95 | stay: the owner's 2026-10-07 dispositions place them in the follow-up after this port |
| #32 | stays: home of the adapter's row |
| #105 | stays: PR 9 |

Filed (linked without a closing keyword), home the classifier follow-up: S13 (hint types by tier); one issue for the
keyword tier's inputs (S14 host clock, S15 unbounded numbers, D5's start-node half), naming PR 9's empty answer.
Merge order: #476 (graphview, draft) shares `docs/admission-ledger.yaml` and `docs/repository-map.md`; whichever
merges second merges `origin/main` and re-runs `task verify`. #464 and #465 share no file.
