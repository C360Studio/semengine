# Tasks: fix-533-keys-cut-short

Light process (owner, 2026-11-07): inventory, proposal and tasks with a spec delta, no design document. Codex reviews
the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [x] 1.1 Inventory reviewed (INVENTORY PASS, 2026-11-07, three MEDIUMs folded in); proposal, tasks and spec delta
  committed; owner approval (2026-11-07, #533 comment 6709379944). Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [x] 2.1 Failing tests (`59120fd`): the `Keys` rows of `TestKVStoreFilteredCancellationFinishesNativeDelivery` and
  `TestKVStoreFilteredWatcherSnapshotAndStopErrors` fail on base (no error, `[]string{"partial"}` returned);
  `TestGraphTriplesListingCutShortFails` fails on base (200 with a partial reply). `TestKVStoreKeysListsEachKeyOnceSorted`
  passes on base (nats.go compacts) and is shown able to fail in 2.2. The message-logger site has no test of its own
  (`notes/2.1.md`). Gate: each fails on the base commit, failure recorded.
- [x] 2.2 `KVStore.Keys` lists through `keysByFilter` with `>`, sorts and removes repeats; the two `service` sites
  call `natsclient.FilteredKeys` and remove repeats; `CatalogReader` drops `Keys` with its tests and fakes moved;
  stale comments corrected (`d57604c`, `5eddaa4`). Amendment (owner, #533 comment 6709709257): a Stop error after a
  complete snapshot no longer fails a listing (`03c2d50`; `stop_error_after_complete_snapshot` failed first with
  `filtered watcher Stop: native unsubscribe failed`). `task mutate:check` detects dropping the compact (3 of 3
  mutant runs failed `TestKVStoreKeysListsEachKeyOnceSorted`); `task verify` passed on a clean tree at `03c2d50`.
  Gate: tests in 2.1 pass, `task mutate:check` detects dropping the compact in
  `KVStore.Keys`, `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`), with the spec synced. Gate: `task spec:check` and
  `task docs:check`.
