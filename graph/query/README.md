# graph/query

Turns a natural-language question into search hints. It reads no graph data, stores nothing and
starts no goroutine. A *hint* is one entry of `ClassificationResult.Options`, a map keyed by the
`SearchOptions` JSON names (`time_range`, `path_start_node`, ...).

## How a question is classified

`ClassifierChain.ClassifyQuery(ctx, question)` asks its tiers in order and answers from the first
one that produces a hint. A *tier* is one stage of the chain:

| Tier | Type | What it does | Needs |
| --- | --- | --- | --- |
| 0 | `KeywordClassifier` | Regular expressions over the question; confidence 1.0 | Nothing |
| 1 | `EmbeddingClassifier` | The most similar example from domain example files, compared as BM25 vectors in the process; a copy of the example's options, confidence is the similarity score | Example files (`LoadDomainExamples`) |
| 3 | `LLMClassifier` | Asks a language model and parses its JSON answer; confidence 0.9 | An `LLMClient` you write; this package holds none |

No code produces tier 2. Tier 0 needs no external provider.

The *default result* is tier 0, no hints, confidence 1.0. The chain answers it when no composed
tier produced a hint, and when the LLM tier fails while the context is live (the error is not
reported; call `LLMClassifier.ClassifyQuery` yourself to see it). A nil chain, a nil context or an
ended context gets `nil`, never the default result.

A hint from the keyword tier ends the chain, so tiers 1 and 3 never run for that question. That
holds for every hint the keyword tier produces, including hints a caller does not read.

## Use

```go
domains := []*query.DomainExamples{examples} // from query.LoadDomainExamples
embed, err := query.NewEmbeddingClassifier(ctx, domains, 0.6)
if err != nil {
    return err
}
chain := query.NewClassifierChain(query.NewKeywordClassifier(), embed)
result := chain.ClassifyQuery(ctx, "sensors in zone A last 3 hours")
```

`NewEmbeddingClassifier` returns an error, and no classifier, for a nil or ended context or when an
example cannot be embedded. The examples are fixed once it returns, and it uses BM25 vectors only.
`NewEmbeddingClassifier` keeps its own copy of each example and its `Options` map, with known hint
names decoded into new values; a value under a name that is not a `SearchOptions` JSON name is still
the caller's own and shared, so do not edit it after the call. `NewLLMClassifier` keeps the domains'
examples and their `Options` maps; do not edit them after the call.

## Hint names and value types

A hint's Go type is the same whichever tier produced it. Tier 1 decodes each example option that
is a `SearchOptions` JSON name into the type below when `NewEmbeddingClassifier` runs, and refuses
the example when it does not decode; other names come back as `encoding/json` decoded them.

| Hint | Tiers 0 and 1 | Tier 3 |
| --- | --- | --- |
| `time_range` | `*TimeRange` | `*TimeRange` |
| `geo_bounds` | `*SpatialBounds` | not produced |
| `use_embeddings` | `bool` | `bool` |
| `path_intent`, `ranking_intent` | `bool` | not produced |
| `path_start_node`, `aggregation_type`, `aggregation_field` | `string` | not produced |
| `path_predicates` | `[]string` | not produced |
| `limit` | `int` | `int` |
| `strategy`, `query` | `string` | `string` |
| `predicates`, `types`, `key_terms` | `[]string` | `[]string` |

Tier 0 never produces `path_predicates`, `strategy`, `query`, `predicates`, `types` or
`key_terms`; tier 1 returns only the hints the matching example lists.

## Copies

A result's `Options` map is the caller's own, and so are the `Example` that `FindBestMatch`
returns, its `Options` map and its `Vector`. Editing them changes no later result. The values
inside a tier-1 `Options` map are not copied: a pointer (`*TimeRange`, `*SpatialBounds`), a slice
(`[]string`, or `[]any` under a caller's own name) or a map is shared with the classifier and with
every other result from the same example, so do not edit what it points to.

## Limits

- Each query is embedded with the read-only `GenerateQuery`, so its score does not depend on the
  queries classified before it; the word statistics come only from the examples, in load order.
- The LLM tier drops a strategy that is not one of the `SearchStrategy` values and a time range
  that does not parse as RFC 3339, and keeps the rest of the response.
- The keyword tier reads the host clock for "today", "last week" and the like, and does not bound
  the number in "last N hours" or a latitude.
- A zone or area phrase gives `path_intent` and `path_start_node` but no `path_predicates`.
