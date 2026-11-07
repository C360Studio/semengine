# Tasks: fix-533-keys-cut-short

Light process (owner, 2026-11-07): inventory, proposal and tasks with a spec delta, no design document. Codex reviews
the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal

- [x] 1.1 Inventory reviewed (INVENTORY PASS, 2026-11-07, three MEDIUMs folded in); proposal, tasks and spec delta
  committed; owner approval (2026-11-07, #533 comment 6709379944). Gate: `task spec:check` and `task docs:check`.

## 2. Code

- [ ] 2.1 Failing tests: `KVStore.Keys` cut short (context cancelled after the first key; watcher channel closed
  before the marker) returns no keys and an error; a snapshot repeating a key out of order lists each key once,
  sorted; the two `service` handlers fail a cut-short listing. Gate: each fails on the base commit, failure recorded.
- [ ] 2.2 `KVStore.Keys` lists through `keysByFilter` with `>`, sorts and removes repeats; the two `service` sites
  call `natsclient.FilteredKeys` and remove repeats; `CatalogReader` drops `Keys` with its tests and fakes moved;
  stale comments corrected. Gate: tests in 2.1 pass, `task mutate:check` detects dropping the compact in
  `KVStore.Keys`, `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open. The PR body carries
  `implemented-by:` and `reviewed-by: codex`.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`), with the spec synced. Gate: `task spec:check` and
  `task docs:check`.
