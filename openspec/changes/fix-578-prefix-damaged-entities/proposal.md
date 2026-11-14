# Proposal: fix-578-prefix-damaged-entities

Approved-by:

Track: `design`. Capabilities modified: `graph-transport-boundary`, `fusion`, `graph-query`, `graph-gateway`.

## Why

When one stored entity on a prefix page cannot be decoded, graph-ingest refuses the whole page with
`graph_state_reset_required` (`processor/graph-ingest/query.go:330-333`). The caller loses every good entity on the
page and cannot tell which one is damaged. ADR-109 Decision 1 says a damaged entity refuses only itself
(`docs/adr/109-ready-after-boot.md:77`). Fusion's prefix lens therefore fails a whole answer for one bad record
(#578). Owner's choice (2026-11-14): "Mirror batch: return the rest plus a Missing list".

## What Changes

- `graph.PrefixQueryResponse` gains `missing`, the batch reply's `[]MissingEntity` (omitted when empty). graph-ingest's
  prefix read returns every entity it decoded and lists each damaged one with reason `error`. Total, sort order,
  cursor and the byte fit behave as today; a damaged key counts in the fit and can be the page's last key.
- A key that comes back not found on this read stays unlisted, as today. A KV or transport read error, or an ended
  context, still refuses the whole page.
- The damaged entity is still recorded and lowers graph-ingest's health, and it is not cached.
- fusionnats' prefix resolve keeps a damaged ID as a seed, so hydration lists it as unhydrated `error` and the answer
  stands.
- graph-query's count, summary and search verbs, and graph-gateway's `entitiesByPrefix`, refuse a page that lists a
  damaged entity, as they do today. This keeps a partial page from looking complete.
- The batch verb does not change.
- A migration note tells direct readers of `graph.query.prefix` to read `missing`.

## Impact

`graph`, `processor/graph-ingest`, `pkg/fusion/fusionnats`, `processor/graph-query`, `gateway/graph-gateway`, and
four specs. Draft #654 also edits `processor/graph-query`; this change merges first.
