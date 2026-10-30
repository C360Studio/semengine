# Proposal: setup-04a-04-graph-roots-query

## Why

Change 4 (`setup-04a-04-graph-roots`) of epic #9 ports the graph roots one package per pull request, in the order the
owner set on 2026-10-24 (#9 comment 6386856657). Item 6 is "`graph/query` (#96 context threading rides it)"; item 9,
`processor/graph-query`, imports it, so it lands first. The package classifies a natural-language search question
into search hints (a time range, a bounding box, a path start, a strategy) through a ladder: keyword patterns first
(tier 0), then an optional example classifier on BM25 vectors, then an optional LLM tier. The owner's bar: "graph is
in many ways our secret sauce. it needs to be solid… i would rather not bring in a pkg that causes instant debt".
Tier 0 only (#9 comment 6386603659): the provider file `classifier_llm_adapter.go` stays out; the `LLMClient`
interface stays.

Approved-by:

## What Changes

- Port `graph/query` without `classifier_llm_adapter.go`: `KeywordClassifier`, `ClassifierChain`,
  `ClassificationResult`, `EmbeddingClassifier`, `LLMClassifier` with its `LLMClient` interface, the domain-example
  loader, `SearchOptions`, `TimeRange`, `SpatialBounds` and the strategy and aggregation names.
- The adapter and its test are not carried: a `defer-exclude` ledger row naming #32. `DefaultClassificationTimeout`
  goes with it (its one reader, `processor/graph-query`'s `initLLMClassifier`, is excluded by the same ruling).
- No invented context (#96, first half): the example classifier calls `graph/embedding`'s BM25 embedder with the
  caller's context. `NewEmbeddingClassifier` takes a context and returns an error, where the pin built a classifier
  that silently never matched when its vectors failed. The pin's `Embedder` interface and its adapter go: their only
  other user was `UpgradeVectors`, which nothing calls.
- An ended context yields no classification (`nil`), as the pin's doc says, never the confident default the pin's
  code returned after a late cancel.
- A caller that edits a result no longer edits the classifier's stored examples.
- Dead surface is not carried: `UpgradeVectors`, `InferStrategy`, `HasIndexFilters`, `SetDefaults`,
  `DefaultMaxCommunities`, `LoadAllDomainExamples`, the `Classifier` and `Embedder` interfaces, and the fields
  `RequireAllFilters`, `Level`, `MaxCommunities`. The private cosine copy gives way to `embedding.CosineSimilarity`.
- Docs say only what ships: no KV client, tiers 0, 1 and 3, no neural upgrade.

## Capabilities

- New `query-classification`.

## Impact

- New `graph/query`: 1,429 pin lines (1,551 less the adapter); one pull request, under the ~3,000-line bound.
- One import edge, `graph/query` → `graph/embedding`. No `graph/llm`, `model` or `go-openai`
  (`TestQueryImportsNoProviderModule`). No subject, bucket, stream, config key or goroutine.
- Changed exported surface: `NewEmbeddingClassifier(ctx, domains, threshold) (*EmbeddingClassifier, error)`. No
  starter consumer imports the package; `processor/graph-query` (PR 9) does not build an example classifier.
- Not on the coverage-floor list (SETUP 03B D10), so no `cover-check` target.
- Issues: retires none. Advances #96 (its context half; the `GenerateQuery` repair stays there). #94, #95 stay for
  the classifier follow-up; #32 stays as the adapter's home; #105 stays with PR 9. Files two issues (design D9).
