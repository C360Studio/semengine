# Proposal: fix-657-damaged-read-reports

Approved-by:

Track: `quick` (owner, 2026-11-22). Capabilities: `graph-entity-writes`, `graph-transport-boundary`, `graph-index`,
`embedding-index`, `graph-embedding`. Packages: `graph`, `graph/follower`, `graph/embedding`, `pkg/graphview`,
`processor/graph-ingest`, `processor/graph-index`; `pkg/errs` comment only.

## Why

The owner's rulings on #657 (2026-11-14, 2026-11-22) say a damaged record is never left out of a reply without the
reply saying so. Four places on `main` fall short: a refusal for a damaged entity names its IDs only in message text
(`graph/state_contract.go:33-36`, `processor/graph-ingest/query.go:764-779`), although the error body already carries
`detail` across the wire (`natsclient/errors.go:106-135`); graph-index's value filter answers a damaged entity with
transient `internal` (`processor/graph-index/query.go:324-327`); graph-index skips malformed index rows with only a
log and a metric (`query.go:735-739`); and the embedding `unreadable_records` counts the whole store, not the
request's scope (`graph/embedding/vector_cache.go:112-113,126`).

## What Changes

- Every fatal `graph_state_reset_required` refusal for damaged entities carries `detail.entity_ids`: the damaged IDs,
  sorted, no cap (one ID for an entity read, every damaged ID of the page for a prefix page). Class, code and message
  are unchanged. A refusal with no key carries no `entity_ids`.
- `graph.StateRefusalEntityIDs(err) []string` returns those IDs from a refusal, read in process or decoded by
  `RequestClassified` (where JSON makes the list `[]any`), and nil for any other error. It is framework API for
  products: no caller in this repository; its contract is the spec and a product-style test over `RequestClassified`.
- graph-index's `predicate` verb with a `value` refuses fatal `graph_state_reset_required`, with `entity_ids`, only
  when the filter itself reads a stored value `UnmarshalEntityState` refuses. Other read faults stay transient
  `internal`. An entity graph-index has already recorded damaged has no membership row and stays counted in
  `damaged_entities`, as the ADR-109 amendment of 2026-11-19 says.
- The seven graph-index replies that list index rows (incoming, alias, predicate, predicateList, predicateStats,
  predicateCompound, byName) carry `data.unreadable_records`, the rows skipped because they are not their index's
  layout. The metric and the log line stay. A row deleted after the listing is not counted.
- `unreadable_records` on `search` counts only the unreadable `EMBEDDING_INDEX` records whose keys the request's
  `scope` accepts; with no scope, and on `similar`, every one, as today.
- The stale comments at `graph/state_contract.go:62-67` and `pkg/errs/errs.go:302-307` are corrected.
- Tests, each shown failing on `main`: `entity_ids` read back with the helper over `RequestClassified` from
  graph-ingest's entity and prefix verbs and graph-index's `outgoing`; the value filter refused fatal; skipped rows
  counted per reply; a scoped search counting one of two unreadable records.

## Non-goals

- Carrying `entity_ids` into GraphQL `extensions`, or relaying the new count through graph-query, fusion or the
  gateway.
- An `outgoing` row that does not decode (still transient `internal`: skipping it would answer "no relationships").
- The 10-ID sample in message text and `Health`.

## Impact

Wire additions only: a `detail` object on existing refusals, `data.unreadable_records` on seven graph-index reply
types, and a smaller `unreadable_records` on scoped searches. One new exported function. `pkg/graphview`
`ListCounted` takes a key filter (one caller). One PR, `Closes #657`, merged after #718.
