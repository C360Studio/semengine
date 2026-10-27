# Tasks: setup-04a-04-graph-roots-objectstore-store

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171).

## 1. Design and measurement

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review. Gate:
  `task spec:check` and `task docs:check`.
- [ ] 1.2 Post the nats.go v1.54.0 / nats-server v2.15.1 ObjectStore probe (inventory 11, P1-P10) on PR #432: program,
  command, versions and output. Gate: the comment read back.
- [ ] 1.3 Owner ruling on question 1 (design D6: drop the data cache, or keep it behind the Component only).
  Hold: owner ruling on question 1.
- [ ] 1.4 File the issues of design D13 (G5 orphan binaries; G8 envelope against ADR-063's read contract; S27 constant
  instance name), linked from PR #432 without a closing keyword. Gate: each issue read back.

## 2. Tests first

- [ ] 2.1 The unit and integration tests of design D9, each shown failing against the pin's shape where the pin differs
  (S8-S11, G1), with the pin tests ported or dropped named in `notes/2.1.md`. Gate: in 3.x.

## 3. The Store

- [ ] 3.1 `config.go`, construction and the retention guard (design D3, D4, D5); `natsfixture` for the integration
  tests (#195). Gate: `go test -race -count=1 ./storage/objectstore`, `task test:integration --
  ./storage/objectstore`.
- [ ] 3.2 `Put`, `Get`, `Open`, `List`, `Delete`, `StoreContent` and the envelope types (design D3, D4). Gate: as 3.1
  plus `go test -count=5 -cpu 1 -shuffle=on ./storage/objectstore`.
- [ ] 3.3 Metrics on the injected registry (design D7). Gate: as 3.1 and the metric-registry guard in `task lint`.
- [ ] 3.4 The data cache as ruled in 1.3 (design D6). Gate: as 3.1; `TestPublicSignatures` (the
  `harness-boundaries` guard) passes.

## 4. Ledger, records and docs

- [ ] 4.1 Ledger rows of design D11 (`storage/objectstore` new, `storage` to `adapt`, `internal/cache` importer count,
  `docs/adr/063-…` new). Gate: `task ledger:check`.
- [ ] 4.2 Carry ADR-063 into `docs/adr/` with its port note (design D11; closes #321). Gate: `task docs:check`.
- [ ] 4.3 `storage/doc.go` and `storage/storage.go` examples and `Put` doc (#280 item 1); package `doc.go`, `README.md`,
  `ExampleNewStoreWithConfig`; `docs/repository-map.md` row; the storage bullet in the developer and reviewer contracts
  (design D2, D11). Gate: `task docs:check`, the adapter guard in `task verify`.
- [ ] 4.4 `storage/objectstore merged` in `scripts/cover-check.sh` (design D9). Gate: `task cover:check` after
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
