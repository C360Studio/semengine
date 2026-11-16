# Proposal: setup-04c-01-http-embedder

Approved-by:

Track: `design`. Capabilities: `embedding-index` (added), `graph-embedding` (added), `graph-query` (modified, added).

## Why

Tier 1 gets its vectors from outside the process through `embedding.Embedder` (owner, #11, 2026-11-14), but
SemEngine ships no client for a provider. A product moving to tier 1 has to write one, and the search scorecard
(#666) has no real code path to run through. The pin's client (`graph/embedding/http_embedder.go`, 220 lines) is
excluded: it uses the OpenAI SDK, which a tree-wide guard forbids, and the probe shows it accepts replies it should
refuse (inventory, shape sweep S1-S4). Losing the provider must also degrade to word search with search intact
(`docs/setup-plan.md:454-457`), and searchGraph's labelled fallback (#563) was ruled to be built here.

## What Changes

- **PR A, the client.** An `embedding.Embedder` over plain `net/http` that speaks the OpenAI embeddings API as
  semembed serves it. Config: base URL, model, optional API key, optional query prefix. It refuses a reply it cannot
  trust (wrong count, index or width, an empty vector, another model), learns the width from the first reply, and
  is bounded only by its caller's context. The Worker bounds each embed at 30 s (the pin client's default), so a
  hung provider frees its lane. A provider test runs against a semembed-shaped fake in `task verify`, and against a
  real provider through `task test:provider`.
- **PR B, the fallback.** With `embedding_enabled` true, a searchGraph with no word matches, or one refused
  `index_not_ready`, asks the embedding search and labels the reply `fallback`. A failed fallback is named in the
  reply, never hidden. Tier 0 is unchanged. An integration test closes the provider mid-run and shows word search
  answering, the embedding search refusing rather than answering empty, and graph-embedding reading `degraded`.

## Non-goals

Merging word and vector results (#668); long-text splitting (#667); retrying the Worker's failed entities after
recovery; reading `model`'s endpoint registry (the product does that); go-openai or `model/wire`.

## Impact

New exported surface: the client package and `graph.GlobalSearchData.Fallback`. Ledger row
`graph/embedding/http_embedder.go` moves from `defer-exclude` to `adapt`. Docs: graph/embedding and graph-embedding
READMEs, graph-embedding and graph-query `MIGRATION.md`. No new module.

## Retires, and in which pull request

PR B: `Closes #489`, `Closes #563`. PR A: `Addresses #489`. #11 addressed.

## Rulings needed

Placement, the model check, the hang bound, where the real-provider test runs, ADR-037's changed decision, and the
fallback reply shape (design, Open Questions).
