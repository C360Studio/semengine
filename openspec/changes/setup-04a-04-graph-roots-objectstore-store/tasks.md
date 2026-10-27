# Tasks: setup-04a-04-graph-roots-objectstore-store

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171).

## 1. Design and measurement

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review. Gate:
  `task spec:check` and `task docs:check`. INVENTORY PASS by the early Claude review at `69a3f6b`; Codex DESIGN REVIEW
  PASS at `f554bc4` (PR #432 comment 6447833003); owner acceptance 2026-10-27.
- [x] 1.2 Post the nats.go v1.54.0 / nats-server v2.15.1 ObjectStore probe (inventory 11, P1-P10) on PR #432: program,
  command, versions and output. Gate: the comment read back. Posted as PR #432 comment 6445520357.
- [x] 1.3 Owner ruling on design D6: drop the data cache. Ruled at PR #432 comment 6446982969.
- [x] 1.4 Issues to file: none (design D13). G5 no longer arises and G8 is resolved by `StoreContent`'s raw-body
  reference (design D4); S27 is 5b's disposition.

## 2. Tests first

- [x] 2.1 The unit and integration tests of design D9, each shown failing against the pin's shape where the pin differs
  (S8, S9, the never-written half of S10, G1), with the pin tests ported or dropped named in `notes/2.1.md`. Gate: in
  3.x.

## 3. The Store

- [x] 3.1 `config.go`, construction and the retention guard (design D3, D4, D5); `natsfixture` for the integration
  tests (#195). Gate: `go test -race -count=1 ./storage/objectstore`, `task test:integration --
  ./storage/objectstore`.
- [x] 3.2 `Put`, `Get`, `Open`, `List`, `Delete`, and `StoreContent` writing and referencing the raw body and refusing
  any other content (design D3, D4; owner ruling, PR #432 comment 6447702631). Gate: as 3.1 plus `go test -count=5
  -cpu 1 -shuffle=on ./storage/objectstore`, `TestStoreContentReferenceOpensToBody`, `TestStoreContentRefusesExtras`
  and `TestStoreHistoryMatchesModel`.
- [x] 3.3 Metrics on the injected registry (design D7). Gate: as 3.1 and the metric-registry guard in `task lint`.
- [x] 3.4 No data cache, as ruled in 1.3 (design D6): no `internal/cache` import, no `Config.DataCache`, no
  `Store.Close`. Gate: as 3.1; `TestPublicSignatures` (the `harness-boundaries` guard) passes; `go list -deps
  ./storage/objectstore` names no `internal/cache`.

## 4. Ledger, records and docs

- [x] 4.1 Ledger rows of design D11 (`storage/objectstore` new, `storage` to `adapt`, `internal/cache` importer count,
  `docs/adr/063-…` new). Gate: `task ledger:check`.
- [x] 4.2 Carry ADR-063 into `docs/adr/` with its port note (design D11; closes #321). Gate: `task docs:check`.
- [x] 4.3 `storage/doc.go` and `storage/storage.go` examples, `Put` and `Delete` docs (#280 item 1, probe P4); the
  `Close` text in `storage/storeregistry/storeregistry.go:19-21`, `:74-75` and `storage/doc.go:86`,
  `:208`; package `doc.go`, `README.md`, `ExampleNewStoreWithConfig`; `docs/repository-map.md` row; the
  `ContentStorable` doc
  (`message/content_storable.go`): the store keeps only the `body` role's text as raw bytes, a payload with any other
  non-empty content is refused, and non-body text belongs in the graph as triples, emitted by the producer; the storage
  bullet
  in the developer and reviewer contracts, with its row in `AGENTS.md` and `.agents/rules-index.md`, gate `review`
  (design D2, D11). Gate: `task docs:check`, the adapter guard and the rules-index guard in `task verify`.
- [x] 4.4 `storage/objectstore merged` in `scripts/cover-check.sh` (design D9). Gate: `task cover:check` after
  `task test:integration`.

## 5. Evidence and review

- [ ] 5.1 Coverage and the failing-first evidence in `notes/2.1.md`; `task verify` passes on a clean tree at the last
  commit before the archive commit.
- [ ] 5.2 Cross-agent review stop; `reviewed-by:` in the PR body.
- [ ] 5.3 Comments on #280 (item 1 done), #195, #33, #196 and #281 (what moves to 5b), each "implemented in PR #432 at
  `<SHA>`, pending merge" where it applies, read back.

## 6. Close

- [ ] 6.1 Archive (`openspec archive setup-04a-04-graph-roots-objectstore-store`) as the last content commit, syncing
  `object-store` into `openspec/specs/`. Gate: `task spec:check` and `task docs:check` on the archive commit.
