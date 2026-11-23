# Proposal: fix-667-truncation-count

Approved-by:

Track: `quick`. No capability changes: no spec states graph-embedding's metrics or the HTTP embedder's handling of a
cut, and no requirement is added or changed. Packages: `graph/embedding`, `graph/embedding/httpembedder`,
`processor/graph-embedding`.

## Why

A deployment cannot see how many entities were embedded from only the opening of their text (#667). semembed marks
a text it cut to fit the model's input window with `"truncated": true` on that text's reply entry, but
`httpembedder` ignores the field. graph-embedding's own text cap (`max_text_len`, 4,000 characters by default) is
counted by the Worker through `WorkerMetrics.IncTruncated`, but graph-embedding's receiver for it is an empty stub
(`processor/graph-embedding/metrics.go`), so neither cut reaches a metric.

## What Changes

- `graph/embedding` gains an optional interface an `Embedder` may also implement, `TruncationMarker`, whose
  `GenerateMarked` returns each text's vector with whether the provider cut it (`MarkedVector`). The Worker embeds
  documents through it when the embedder has it. `Embedder` itself is unchanged, so every existing embedder still
  works and reports no provider cuts.
- `WorkerMetrics.IncTruncated()` becomes `IncTruncated(by string)`, `by` being `provider` or `text_cap`. The Worker
  counts a provider cut once per entity whose vector came back marked from the provider, and its own cap as today.
- `httpembedder` reads `truncated` from each reply entry and implements `TruncationMarker`. A reply whose
  `truncated` is not a boolean is refused like any other malformed reply.
- graph-embedding registers `semengine_graph_embedding_truncated_total{by}` on the injected registry through
  `metric.RegisterOrGet`, both series starting at 0, and its `IncTruncated` adds to it. The README lists it.
- Tests: `httpembedder` returns the mark per text; graph-embedding, run against a semembed-shaped provider that
  marks a text, counts `by="provider"`; run with a small `max_text_len`, counts `by="text_cap"` for the long entity
  only.

## Non-goals

- Splitting long text into pieces, and the SemSource count (#667 step 3).
- A provider cut on a vector reused from the dedup bucket is not counted again: the dedup record does not keep the
  mark. Query-side cuts (`GenerateQuery`) are not counted.
- How `httpembedder` answers a provider's 429.

## Impact

`WorkerMetrics` and `TruncationMarker` are exported surface of `graph/embedding`; `WorkerMetrics`' one production
implementer is graph-embedding's `embeddingMetrics`. One PR (#728), `Closes #667`.
