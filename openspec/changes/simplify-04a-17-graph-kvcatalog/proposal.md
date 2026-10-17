# Proposal: simplify-04a-17-graph-kvcatalog

## Why

This is PR 8d of epic #223 (owner, 2026-10-16, #223 comment 6216623583: "**8d — `graph/kvcatalog`**: #133.").

`graph/kvcatalog` holds the one table of KV buckets the framework owns. Around it sits small layering: the table is
rebuilt on every lookup, a check and its refusal are always called as a pair, a map holds one entry, errors name the
`graph` package it left, and a predicate about KV operations lives here although it reads no catalog row. A nil
context given to the retention check comes back as a retryable error.

## What Changes

- **One table.** The catalog is an unexported package-level table, built once. `KVCatalog` is removed (owner
  question 1; if kept, it returns a copy).
- **One family check.** The one-entry `nameFamilies` map becomes a comparison.
- **One generic-write check.** `CheckGenericWrite(bucket) error` replaces `IsFrameworkOwnedBucket` and
  `FrameworkOwnedWriteRefusal`; the refusal names its owner once.
- **Errors name `kvcatalog`.** The not-in-catalog refusal is built once. `EnsureCatalogRetentionClean` refuses a nil
  context as invalid before any other work.
- **The tombstone predicate moves.** `IsKVTombstone` moves to `natsclient` (owner question 2); `pkg/lifecycle`'s inline
  copy calls it.
- **Docs.** Citations that resolve only in SemStreams say so; labels that resolve nowhere go. The `graph` README line
  and ledger row follow.

## Capabilities

None changed. No current spec names these surfaces; the one catalog requirement (`graph-entity-writes`, "One stored
revision per entity") is unchanged.

## Impact

- Files: `graph/kvcatalog` (all files; `tombstone.go` and its test removed); `natsclient/kv.go`,
  `natsclient/kv_tombstone_test.go` (new), `natsclient/kvspec.go:120` (comment); `graph/readiness/watcher.go`;
  `processor/graph-ingest/component.go:1287`; `pkg/lifecycle/manager_query.go:260`; `graph/README.md`;
  `docs/admission-ledger.yaml` (`graph` row, and the `graph/readiness` row's dependencies after 8e).
- Error text: the prefix `graph.EnsureCatalogBucket:` (and its siblings) becomes `kvcatalog.…:`; the operational
  refusal names its owner once, dropping "its writes belong to".
- Behaviour change: `EnsureCatalogRetentionClean(nil, …)` returns an invalid error naming the nil context, where a
  live client gave a transient one.
- Breaking for consumers at cutover (compiler-found): semconnect's `graph.IsKVTombstone` becomes
  `natsclient.IsKVTombstone`. No consumer calls the other changed names.
- Closes #133. Addresses #223.
