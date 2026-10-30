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
Both `NewEmbeddingClassifier` and `NewLLMClassifier` keep the domains' examples and their `Options`
maps; do not edit them after the call.

## Hint names and value types

A value's Go type depends on the tier that produced it.

| Hint | Tier 0 | Tier 1 | Tier 3 |
| --- | --- | --- | --- |
| `time_range` | `*TimeRange` | `map[string]any` | `*TimeRange` |
| `geo_bounds` | `*SpatialBounds` | `map[string]any` | not produced |
| `use_embeddings` | `bool` | `bool` | `bool` |
| `path_intent`, `ranking_intent` | `bool` | `bool` | not produced |
| `path_start_node`, `aggregation_type`, `aggregation_field` | `string` | `string` | not produced |
| `path_predicates` | not produced | `[]any` | not produced |
| `limit` | `int` | `float64` | `int` |
| `strategy`, `query` | not produced | `string` | `string` |
| `predicates`, `types`, `key_terms` | not produced | `[]any` | `[]string` |

Tier 1 returns whatever hints the matching example file lists, so a name is present only if the
example has it. Its values are what `encoding/json` decodes from that file: a JSON array is `[]any`, an
object is `map[string]any` and a number is `float64`. A reader that asserts `[]string` or `int` on
a tier 1 value does not match. This package does not convert them; the table shows what ships.

## Copies

A result's `Options` map is the caller's own, and so are the `Example` that `FindBestMatch`
returns, its `Options` map and its `Vector`. Editing them changes no later result. Values nested
inside an `Options` entry (a slice or a map) are shared with the classifier and are not copied.

## Limits

- Each query is embedded with `Generate`, which adds it to the BM25 embedder's word statistics, so
  later scores depend on the queries seen so far. Issue #96 tracks the read-only alternative.
- The LLM tier drops a strategy that is not one of the `SearchStrategy` values and a time range
  that does not parse as RFC 3339, and keeps the rest of the response.
- The keyword tier reads the host clock for "today", "last week" and the like, and does not bound
  the number in "last N hours" or a latitude.
- A zone or area phrase gives `path_intent` and `path_start_node` but no `path_predicates`.
