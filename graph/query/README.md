# graph/query

Turns a natural-language question into search hints. It reads no graph data, stores nothing and
starts no goroutine. A *hint* is one entry of `ClassificationResult.Options`, a map keyed by the
`SearchOptions` JSON names (`time_range`, `path_start_node`, ...).

## How a question is classified

`ClassifierChain.ClassifyQuery(ctx, question)` asks its two tiers in order and answers from the
first one that produces a hint. A *tier* is one stage of the chain:

| Tier | Type | What it does | Needs |
| --- | --- | --- | --- |
| 0 | `KeywordClassifier` | Regular expressions over the question; confidence 1.0 | Nothing |
| 3 | `LLMClassifier` | Asks a language model and parses its JSON answer; confidence 0.9 | An `LLMClient` you write; this package holds none. Few-shot examples from domain example files (`LoadDomainExamples`) are optional |

No code produces tiers 1 or 2; the numbers are kept because callers read `ClassificationResult.Tier`.
Tier 0 needs no external provider, and the package holds no embedding code.

The *default result* is tier 0, no hints, confidence 1.0. The chain answers it when no composed
tier produced a hint, and when the LLM tier fails while the context is live (the error is not
reported; call `LLMClassifier.ClassifyQuery` yourself to see it). A nil chain, a nil context or an
ended context gets `nil`, never the default result.

A hint from the keyword tier ends the chain, so tier 3 never runs for that question. That
holds for every hint the keyword tier produces, including hints a caller does not read.

`ClassifyQuery` returns `(*ClassificationResult, error)`. The error is set only when the question
holds an impossible number or, with the keyword tier composed, names two places (see "Bounds and
refusals"); every other outcome has a nil error.

## Use

```go
domains := []*query.DomainExamples{examples} // from query.LoadDomainExamples; may be nil
chain := query.NewClassifierChain(query.NewKeywordClassifier(), query.NewLLMClassifier(client, domains))
result, err := chain.ClassifyQuery(ctx, "sensors in zone A last 3 hours")
if err != nil {
    return err // an impossible number or two places; see "Bounds and refusals"
}
if result == nil {
    return ctx.Err() // a nil chain, a nil context or an ended context
}
```

`client` is your `LLMClient`. Without one, `query.NewClassifierChain(query.NewKeywordClassifier(), nil)`
is the keyword tier alone. `NewLLMClassifier` keeps the domains' examples and their `Options` maps;
do not edit them after the call.

## Hint names and value types

A hint's Go type is the same whichever tier produced it.

| Hint | Tier 0 | Tier 3 |
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

Tier 0 never produces `path_predicates`, `strategy`, `query`, `predicates`, `types` or `key_terms`;
the types in its column are the `SearchOptions` field types.

## Copies

A result's `Options` map is the caller's own: deleting or adding a key changes no later result.
The values inside an entry may be shared, so do not edit what a pointer (`*TimeRange`,
`*SpatialBounds`) or a slice points to.

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

With the keyword tier composed, a question with an entity reference and a zone or area phrase naming another
place is refused too, before any tier is asked, with or without path words: "what is connected to sensor-001
in zone-b" and "show sensor-001 in zone-b" get an invalid-input error saying `the question names two places,
sensor-001 and zone-b; ask about one`. Two zone or area phrases naming different places are refused the same
way, whether a name is written with a hyphen or a space: "what is connected to zone a in zone b" names `zone-a`
and `zone-b`. One place written twice ("zone a, zone-a", case aside) is one place. The keyword tier reads one
path start, so it would answer a path from one place and drop the other. A phrase that only looks like a zone
("in the zone of the plant" reads `zone-of`) is refused the same way when the question also names an entity or
another zone. A full entity ID is an entity reference whether or not a segment has a hyphen: "what is connected
to c360.test.x.y.z.001 in zone b" names `c360.test.x.y.z.001` and `zone-b`. An entity reference inside the zone
phrase ("zone-b" itself, "sensor-zone-1") is the same place. These refusals are loud and can be wrong: an
ordinary hyphenated word ("in real-time", "non-zero") or "zone or area" plus a word ("the area with pumps") beside a
place is refused as a second place, a known limit.
`KeywordClassifier.ClassifyQuery` on its own still answers a path from one of them. Two entity references with
no zone phrase ("from pump-1 to pump-2") are not refused; the keyword tier reads the first.

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
result and does not ask the LLM tier. The LLM tier answers in the caller's own words, which the
framework cannot read, so it could bring back the operation the question refused. The cost is that a
question whose only operation is excluded gets no tier 3 answer.

## Limits

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
- A zone or area phrase gives `path_intent` and `path_start_node` but no `path_predicates`.
