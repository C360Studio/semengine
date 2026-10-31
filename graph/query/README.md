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

`ClassifyQuery` returns `(*ClassificationResult, error)`. The error is set only when the question
holds an impossible number (see "Bounds and refusals"); every other outcome has a nil error.

## Use

```go
domains := []*query.DomainExamples{examples} // from query.LoadDomainExamples
embed, err := query.NewEmbeddingClassifier(ctx, domains, 0.6)
if err != nil {
    return err
}
chain := query.NewClassifierChain(query.NewKeywordClassifier(), embed)
result, err := chain.ClassifyQuery(ctx, "sensors in zone A last 3 hours")
if err != nil {
    return err // the question holds an impossible number; see "Bounds and refusals"
}
if result == nil {
    return ctx.Err() // a nil chain, a nil context or an ended context
}
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

## Time zone

The keyword tier reads the clock once per question. "Today", "yesterday", "this week", "this month"
and "this year" are calendar spans in the zone of that instant, which is the server's own zone
(`time.Local`, which Go's `time` package documentation says comes from the `TZ` environment variable,
else the system's local zone, else UTC; not probed here). A server in
Tokyo and a server in London answer "today" with different spans. A setting to choose the zone is
not present; it waits until a consumer asks for one.

## Bounds and refusals

A question with a number that cannot be a filter is refused, not answered without that filter,
because answering without it would answer a wider question than the one asked. The chain returns no
result and an invalid-input error (`errs.IsInvalid`) that names the phrase and the bound it broke.
These are refused:

- "last N hours" or "last N days" with N below 1 (a negative N included), or N so large that the
  span does not fit a `time.Duration` (about 292 years).
- "within R of lat,lon" with a latitude outside [-90, 90], a longitude outside [-180, 180], or a
  radius that is not a finite number.

Every such phrase in the question is checked, and the refusal happens whichever tiers are composed
and before any tier is asked. `KeywordClassifier.ClassifyQuery` has no error to return, so on its
own it gives no hint for such a phrase and keeps the others.

Not refused, because it is geometry: a box that reaches a pole is clamped to 90 or -90 and spans
longitude -180 to 180. A box across the 180 degree line is not wrapped: its East can exceed 180 or
its West fall below -180, and the reader must handle that.

## What the keyword tier leaves out

An *operation* is one of seven things a question can ask for: count, average, sum, minimum,
maximum, similarity and ranking. A *clause* is the text between `;`, `,`, `.`, `:`, `?` or `!` (each
followed by a space or the end of the question) or the word "but". The keyword tier excludes an
operation, so that it gives no hint for it, when its trigger words are:

- negated: they follow "not", "no", "never", "don't", "do not" or "without" in the same clause;
- quoted: they lie inside a balanced pair of `"` or `“ ”`;
- undecided: they are one of two different operations joined by "or" in one clause, where the
  second trigger comes right after "or", optionally after "the", "a" or "an" ("count sensors or the
  average pressure"). "Count sensors or pumps" joins nouns and excludes nothing.

Exclusion is per operation: "Do not count sensors; average pressure" gives average. Other hints
stand: "Do not count sensors from yesterday" keeps its time range.

When the keyword tier excludes an operation and finds no other hint, the chain returns the default
result and asks no later tier. A later tier answers in the caller's own words, which the framework
cannot read, so it could bring back the operation the question refused. The cost is that a question
whose only operation is excluded gets no tier 1 or tier 3 answer.

## Limits

- Each query is embedded with the read-only `GenerateQuery`, so its score does not depend on the
  queries classified before it; the word statistics come only from the examples, in load order.
- The LLM tier drops a strategy that is not one of the `SearchStrategy` values and a time range
  that does not parse as RFC 3339, and keeps the rest of the response.
- The exclusion rule is made of word rules. "Not only count but average" loses the count, because
  "not" comes before "count" in the same clause.
- A comma ends a clause. In "count, average or sum", the "or" joins average and sum, so they are
  excluded and count stands.
- Quoted filter phrases are still read for numbers: `Find documents containing "last 0 hours"` is
  refused.
- A radius of zero gives no box and no error.
- "like" counts as similarity only as a whole word ("likely" and "liked" do not).
- Classification uses the read-only `GenerateQuery`; examples are learned only when the classifier
  is constructed.
- A zone or area phrase gives `path_intent` and `path_start_node` but no `path_predicates`.
