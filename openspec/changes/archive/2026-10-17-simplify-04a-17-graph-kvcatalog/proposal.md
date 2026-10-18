# Proposal: simplify-04a-17-graph-kvcatalog

## Why

This is PR 8d of epic #223 (owner, 2026-10-16, #223 comment 6216623583: "**8d — `graph/kvcatalog`**: #133.").

`graph/kvcatalog` holds the one table of KV buckets the framework owns. Around it sits small layering: the table is
rebuilt on every lookup, a map holds one entry, errors name the
`graph` package it left, and a predicate about KV operations lives here although it reads no catalog row. A nil
context given to the retention check comes back as a retryable error.

## What Changes

- **One table.** The catalog is an unexported package-level table, built once. `KVCatalog` stays as
  framework API for products (owner, 2026-10-17) and returns a copy of the table.
- **One family check.** The one-entry `nameFamilies` map becomes a comparison.
- **The write-guard pair stays.** `IsFrameworkOwnedBucket` and `FrameworkOwnedWriteRefusal` are read only by the
  `rule` package, ported in change 6; that port decides whether to merge them (#133 item 2 goes to a follow-up).
- **Errors name `kvcatalog`.** The not-in-catalog refusal is built once. `EnsureCatalogRetentionClean` refuses a nil
  context as invalid before any other work.
- **The tombstone predicate moves.** `IsKVTombstone` moves to `natsclient` (owner question 2); `pkg/lifecycle`'s inline
  copy calls it. The `natsclient` ledger row and README record it.
- **Docs.** Citations that resolve only in SemStreams say so; labels that resolve nowhere go. The `graph` README line
  and ledger row follow.

## Capabilities

None changed. No current spec names these surfaces; the one catalog requirement (`graph-entity-writes`, "One stored
revision per entity") is unchanged.

## Impact

- Files: `graph/kvcatalog` (all files; `tombstone.go` and its test removed); `natsclient/kv.go`,
  `natsclient/kv_tombstone_test.go` (new), `natsclient/kvspec.go:120` (comment); `graph/readiness/watcher.go`;
  `processor/graph-ingest/component.go:1287`; `pkg/lifecycle/manager_query.go:167`, `:260`; `graph/README.md`;
  `natsclient/README.md`; `docs/admission-ledger.yaml` (`graph` and `natsclient` rows, and the `graph/readiness` row's
  dependencies after 8e).
- Error text: the prefix `graph.EnsureCatalogBucket:` (and its siblings) becomes `kvcatalog.…:`.
- Behaviour change: `EnsureCatalogRetentionClean(nil, …)` returns an invalid error naming the nil context, where a
  live client gave a transient one.
- Breaking for consumers at cutover (compiler-found): semconnect's `graph.IsKVTombstone` becomes
  `natsclient.IsKVTombstone`. No consumer calls the other changed names.
- Closes #133. Addresses #223.
