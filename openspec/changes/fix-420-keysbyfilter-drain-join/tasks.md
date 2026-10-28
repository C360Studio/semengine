# Tasks: fix-420-keysbyfilter-drain-join

Light process, owner 2026-10-28: proposal, tasks and spec delta; no design document; Codex reviews the code.

## 1. Proposal

- [ ] 1.1 Proposal, tasks and spec delta committed; Claude inventory review of the proposal. Gate: `task spec:check`
  and `task docs:check`.

## 2. Code

- [ ] 2.1 A unit test in `natsclient/kv_watcher_ownership_test.go`, under `testing/synctest`, failing on the base
  commit: with a scripted watcher whose update channel closes only after more than 5 s of the bubble's clock past
  `Stop`, `KeysByFilter` returns before the channel closes. Gate: the test fails on base (`go test -overlay` if it
  cannot compile there).
- [ ] 2.2 `keysByFilter` drains until the channel closes with no timeout; `filteredWatcherDrainTimeout`,
  `errFilteredWatcherDeliveryIncomplete` and `TestKVStoreFilteredTerminalWindowUsesVirtualClock` are removed; the
  `nats-kv-keys` citations cite the modified requirement; the ledger sentence is dropped. Gate: the test in 2.1 and
  `TestIntegration_KVStoreFilteredNativeDeliveryClosure` pass; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record on the PR. The PR body carries `implemented-by: claude (opus)` and
  `reviewed-by: codex`.
  Hold: Codex code review, started by the owner.

## 4. Archive

- [ ] 4.1 Archive as the last content commit (`openspec archive`, syncing the modified requirement into
  `openspec/specs/transport-client/spec.md`). Gate: `task spec:check` and `task docs:check`.
