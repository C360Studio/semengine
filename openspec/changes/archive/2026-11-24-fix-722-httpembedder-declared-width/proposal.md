# Proposal: fix-722-httpembedder-declared-width

Approved-by: cglusky (2026-11-24) <https://github.com/C360Studio/semengine/issues/722#issuecomment-7129098841> sha256:e9870f38ddb9

Track: `quick`. Capability: `embedding-index` (modified). Packages: `graph/embedding/httpembedder`; docs in
`processor/graph-embedding`.

## Why

When a seeded tier 1 stack restarts, it embeds again every entity that hop 1 replays before the provider's first
reply. `httpembedder.Embedder.Dimensions()` is 0 until that reply
(`graph/embedding/httpembedder/httpembedder.go:143-146`), and graph-embedding withholds the dedup key while the width
is 0 (`graph/embedding/dedup.go:101`, `graph/embedding/worker.go:190`). So a stored vector cannot be vouched for, and
it is made again. SemSource measured 10 repeated requests out of 7,271 on its dogfood corpus. The cost grows with
corpus size and provider latency (#722). `Config` has no way to declare the width, and `New` must stay free of
requests so that a provider that is down at boot degrades graph-embedding instead of failing it.

## What changes

- `httpembedder.Config` gains `Dimensions int`: the vector width the model produces. 0, the default, keeps today's
  behaviour, where the width is learned from the first reply. A positive value is the width `Dimensions()` reports
  from construction. A reply of any other width is refused, the same way a reply of a second width is refused today
  (`httpembedder.go:301-304`). `New` refuses a negative value. Construction still sends no request.
- The `embedding-index` Requirement "The HTTP embedder trusts only a whole reply" says `Dimensions` is the configured
  width when one is set, and otherwise 0 until the first accepted reply.
- `processor/graph-embedding/README.md:198-205` tells a product to set the width so that a restart keeps its stored
  vectors.

## Non-goals

- A warm-up request at start (the issue's second option). It would need a request at boot, or a background probe.
- A width for embedders other than `httpembedder`. Their own `Dimensions()` already decides this.
- The scorecard tool's flags (`internal/harness/scorecard`). It measures one run, so a restart costs it nothing.

## Impact

One new exported field, and no behaviour change for a product that does not set it. SemSource sets
`Dimensions: 384` for `Snowflake/snowflake-arctic-embed-s`. A wrong value fails every embed with an error naming
both widths, which is loud, not silent. One PR (#743), `Closes #722`.
