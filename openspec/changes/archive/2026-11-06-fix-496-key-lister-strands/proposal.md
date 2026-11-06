# Proposal: fix-496-key-lister-strands

Approved-by: owner, 2026-11-06 in Enjoy, design at `32e2e14` approved after Codex's design review;
recorded on #496 (comment 6694405203)

Track: `design` (adds a spec requirement in two capabilities, `storage-report` and a new `lifecycle-query`).

## Why

Issue #496. `pkg/lifecycle`'s `Manager.List` and storagereport's `reclaim` list keys through nats.go's `ListKeys`.
At v1.55.0 its goroutine can be left running for good, with the watch delivery behind it
(`inventory.md` § 1, § 2):

- **`List`** returns early on an ordinary read error. Bad bytes under one key are enough. If more than 256 keys
  are unread at that point, the lister goroutine blocks on a send it cannot leave (`jetstream/kv.go:1450`).
- **`reclaim`**: if the service stops mid-listing, the lister leaves through `ctx.Done` (`kv.go:1451`) while the
  delivery is blocked on the full buffer (`kv.go:1320`).

Both can also return a short key list with no error (`inventory.md` § 1, 1b):

- **`List`** returns a short participant list as success.
- **`reclaim`** decides which rows to delete from a partial listing.

The repository already has the listing that avoids all of this: `natsclient.FilteredKeys`. It reads the watch to
its end marker, and returns no keys and an error when the context ends or the channel closes first. It always stops
the watch and waits for its delivery to close (`openspec/specs/transport-client/spec.md:743-791`). It left nats.go's
`ListKeysFiltered` for the same weakness (#222). Using it at both sites adds no mechanism (ADR-112; owner, #170).

## What Changes

- **`List`** reads its keys with `natsclient.FilteredKeys(ctx, bucket, ">")`, then reads each entity as today.
  - A listing that does not complete is an error, never a short list.
  - The local reader interface trades `ListKeys` for `WatchFiltered`.
- **`reclaim`** reads its keys the same way.
  - The exported `ReportStore` trades `ListKeys` for `WatchFiltered`; `jetstream.KeyValue` still satisfies it.
  - The unreachable `ErrNoKeysFound` branch goes.
- **`kvcatalog.CatalogReader`** gains `WatchFiltered`. It also drops `ListKeys` and `ListKeysFiltered` (owner,
  2026-11-06; `design.md` D2): an exported `KeyLister` is the #496 strand for any consumer that stops
  reading early, and after this change no framework code reads it.
  - Consumer migration: `natsclient.FilteredKeys(ctx, reader, pattern)`.
- **Tests first** (`tasks.md`):
  - Real-server tests for the `List` and `reclaim` strands, each failing on the base commit.
  - Unit tests for the cut-short listings.
- **Spec:** one requirement for `List` (new capability `lifecycle-query`) and one for `reclaim` (`storage-report`).

## Not covered

- The callers of nats.go `Keys()` (#533).
- `List`'s whole-bucket scan (ADR-049's deferred index).
- Duplicate keys nats.go may report on a busy bucket, and `List` paging before sort/dedup: unchanged, filed as
  #534 (`design.md`, residual R4).

## Capabilities

- New: `lifecycle-query`.
- Modified: `storage-report`.
- No change to `transport-client`: its requirements already state what `FilteredKeys` does.
