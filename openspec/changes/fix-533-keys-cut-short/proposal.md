# Proposal: fix-533-keys-cut-short

Approved-by: owner, 2026-11-07 in Enjoy, proposal at `9f4710f` approved as drafted; recorded on #533 (comment 6709379944)

## Why

Issue #533. nats.go v1.55.0 `KeyValue.Keys` (`jetstream/kv.go:1403-1423`) ranges its watcher's update channel until
the end-of-snapshot marker. When the channel closes first (the caller's context ends, the connection closes), it
returns the keys gathered so far with no error, or `ErrNoKeysFound` when it gathered none. A short list then reads as
the whole bucket. v1.55.0 is the newest release and `Keys` on nats.go `main` is unchanged.

Nine production sites list a whole bucket this way (`inventory.md` § 2). Six go through `natsclient.KVStore.Keys`; the
two `service` HTTP handlers call nats.go `Keys` directly. The worst effect: a cut-short listing at boot in
`config/manager.go:1179` reads as a first boot and pushes the file config over the stored one, and one in
`config/manager.go:1481` drops config entries. Clustering's maintainer never removes stale records it did not list.
MEDIUM by #185: it needs a deadline or a connection close inside a listing window.

`natsclient`'s filtered listing (`keysByFilter`, behind `KVStore.KeysByFilter` and `natsclient.FilteredKeys`) already
reads to the marker and returns no keys and an error when cut short. It opens the same watch as nats.go `Keys`
(`>` with `IgnoreDeletes`, `MetaOnly`; by reading). Light process (owner, 2026-11-07): inventory, proposal and tasks
with a spec delta, no design document; tests first; Codex reviews the code. All nine sites in one PR (owner).

Overlap: PR #530 touches no file here and no `transport-client` spec (checked 2026-11-07), so merge order is free.
Issue #512 (repeats and omissions under concurrent overwrites) stays open: this change only keeps nats.go's repeat removal.

## What Changes

- `KVStore.Keys` lists through `keysByFilter(ctx, kv.bucket, ">")` under its operation timeout, then sorts and
  removes repeats (`sort.Strings`, `slices.Compact`), as nats.go `Keys` did and as `pkg/lifecycle` `List` does (#534).
  An empty bucket still returns nil and no error. This fixes the six sites behind it.
- `service/graph_triples_http.go:256` and `service/message_logger_http.go:392` call `natsclient.FilteredKeys(ctx, r,
  ">")`, then remove repeats (and sort where they do not already). Their `ErrNoKeysFound` branches go.
- `kvcatalog.CatalogReader` drops `Keys`: no reader in the repository calls it after the move, and its doc says the
  method set is exactly what readers use. Migration note: list a catalog bucket through `natsclient.FilteredKeys`
  (`kvcatalog_integration_test.go:96-98` already does). `TestCatalogReader_ExportedSurfaceIsReadOnly` and the two
  integration callers (`inventory.md` § 4) move with it.
- Amendment (owner ruling, 2026-11-07, #533 comment 6709709257): once the snapshot is complete and the watcher's
  update channel has closed, an error from the watcher's `Stop` no longer fails any of the three listings, as nats.go
  `Keys` ignores it. Without this, config boot failed under credentials that deny consumer deletes
  (`TestStopHonoursItsDeadlineWhenAConsumerDeleteIsDenied`). The `nonbenign_stop_after_closure` case changes to
  expect the keys and no error. A listing cut short still fails.
- Test fakes that answer `Keys` over a nil embedded interface (clustering `fakeKV`, the two `service` readers) answer
  `WatchFiltered` instead. Stale comments (`inventory.md` § 9) are corrected; dead fake methods go.
- Tests, first: a cut-short `KVStore.Keys` (context cancelled after the first key; watcher channel closed before the
  marker) returns no keys and an error; a snapshot that repeats a key out of order lists each key once, sorted; the
  two `service` handlers reply with an error, not a partial list, when the listing is cut short. Each fails on base.
- Examples suffice, no generated checks (`docs/testing.md`): the change has two input shapes (a listing cut short, a
  listing with repeats) and no interacting inputs; one fixed snapshot per shape covers them.

## Capabilities

### Modified Capabilities

- `transport-client`: `KVStore.Keys` joins the key listings that complete the same way, cut short returns no keys,
  and the store timeout applies; it returns keys sorted, each once. A Stop error after a complete snapshot no
  longer fails any listing. Two requirement headers are renamed to drop
  "Both" and "The two".
