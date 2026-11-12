# Proposal: fix-579-graph-query-embedding-enabled

Approved-by:

Track: `design` (adds a `graph-query` requirement; modifies `graph-query` and `composed-stack`).

## Why

A composition can leave graph-embedding out: graph-ingest, graph-index and graph-query answer by name, prefix and
structure without it. Today graph-query still forwards `semantic` and `similar` there. Nothing answers, so the caller
gets a transient `internal` refusal naming `graph.embedding.query.search` and retries it without success (#579). The
composition knows it has no embedder; graph-query should say so at once.

## What Changes

- graph-query gets one config key, `embedding_enabled` (bool). It is on when a configuration leaves it out, so every
  existing composition, SemSource's included, behaves as today.
- When it is `false`, `semantic` and `similar` refuse every request before sending anything. The refusal has class
  `invalid` and the new code `not_enabled` (`graph.ErrorCodeNotEnabled`), and its message names the verb. It is not
  `embedding_unavailable`, which means one entity has no vector. Each such request counts in `DataFlow` and is not
  an error in `Health`.
- When it is `true`, nothing changes. Every other verb is unchanged either way. No-responders is never read as "not
  enabled".
- The no-embedder smoke stack sets `embedding_enabled: false`. Its word question now expects invalid `not_enabled`
  naming the verb instead of transient `internal` naming `graph.embedding.query.search`.

## Rulings

Owner, 2026-11-12, #579 (two comments): the key, its default, the code, its class and message, the smoke stack's
setting. BM25 is graph-embedding's tier-0 embedder, so "no embedder" means only a composition without graph-embedding.
Issue #579 supersedes the alpha.3 ruling on the no-embedder word question (PR #577 comment 6749007774).

## Capabilities

- Modified `graph-query` (one requirement added, three modified). Modified `composed-stack` (the no-embedder
  requirement).

## Impact

- `processor/graph-query` (config, handler table, tests), `graph` (one constant, its doc comment), `internal/smoke`
  (one config line, one assertion), docs and two ledger rows. About 40 production lines and 150 test lines, by hand
  estimate.
- Consumers: none must change. A consumer that composes without graph-embedding may set the key to get the clear
  refusal.
- `Closes #579`.
