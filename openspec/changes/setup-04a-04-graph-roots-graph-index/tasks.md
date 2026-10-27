# Tasks: setup-04a-04-graph-roots-graph-index

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171).

## 1. Design and measurement

- [x] 1.1 nats.go v1.54.0 / nats-server v2.15.1 restart probe, run before the owner saw question 2: a broker restart
  does not close a KV watcher's `Updates()`; it stalls and resumes (design D3). Posted on PR #414, comment
  6425769680.
- [x] 1.2 Owner rulings on questions 1-3 and the size waiver: PR #414 comment 6426254115 (one PR with a waiver by count;
  refuse queries while the feed is lost; keep the alias behaviour, #415).
- [x] 1.3 Delete and recreate probe (design D3, Fable read H2): program and output posted on PR #414, comment
  6426443564. Gate: the comment read back.
- [x] 1.4 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review on the
  ruled text. Gate: `task spec:check` and `task docs:check`. Claude reviewer INVENTORY PASS and DESIGN PASS at
  `ebd77d9` (PR #414 comment recording it); Codex accepted the corrections at `58814ca` (comment 6426969384).
- [ ] 1.5 Pin probe re-run with `-v` to record the shuffle seed; result in `notes/1.5.md`. Gate: the note.
- [ ] 1.6 #108's number: the pin's `owner_filter_load_integration_test.go` load shape at the pin, then on the port,
  recording entities, KV writes and the `index_not_ready` window, posted on #108 and in `notes/1.6.md` (design D13).
  Gate: the comment read back.

## 2. Tests first

- [x] 2.1 `graph`: `TestQueryResponseCarriesIndexedRevisionAndProducer` and the empty-producer refusal; `graph/readiness`:
  the ported `ComputeIndexStatus` tests and the revision gauges' names against the registry. Gate: in 3.1.
- [x] 2.2 Follower tests of design D11 (`TestFollowerRunReturnsAndLeavesNothing`, `TestFollowerPublishesOnEveryHeartbeat`,
  `TestFollowerSnapshotEndSetsBootstrapTarget`, `TestFollowerLatchesResetOnUnreadableSnapshotValue`,
  `TestFollowerDeclaresLostFeed`), the lost-feed test shown failing against the pin's loop shape first (`notes/2.2.md`).
  Gate: in 3.2.
- [x] 2.3 graph-index tests of design D11 (with `TestLatchBootstrap_ContinuousWrite`, `notes/3.2.md`), the ported pin
  tests rewritten without sleeps, skips, `t.Parallel` in integration files, the six writer back doors and
  `subscribeForRequests`; each dropped pin test named with its reason in `notes/2.3.md`. Gate: in 4.x.

## 3. Shared pieces

- [x] 3.1 `graph/query_response.go`, `graph.QueryVerbs()` entries and `ResponderGraphIndex`; `ComputeIndexStatus`,
  `IndexStatusInputs` and the revision gauges in `graph/readiness` (design D2, D6, D10). Gate: `go test -race
  -count=1 ./graph/...`, `TestGraphImportsNoTransport`, `TestReservedSubjectsDeclaredOnce`.
- [x] 3.2 `graph/follower` (design D3). Gate: `go test -race -count=1 ./graph/follower`, `task test:integration --
  ./graph/follower`, `go test -count=5 -cpu 1 -shuffle=on ./graph/follower`.

## 4. graph-index

- [x] 4.1 Configuration, factory, registration, metrics (design D9, D10). Gate: `go test -race -count=1
  ./processor/graph-index`, the metric-registry guard in `task lint`.
- [x] 4.2 Index write path on the keyed pool, coalescer, repair loop, owner reconciliation (design D7, D8). Gate:
  as 4.1 plus `task test:integration -- ./processor/graph-index`.
- [x] 4.3 Lifecycle on `component.Lifecycle` with the follower; Health and DataFlow (design D4). Gate: as 4.2 and
  `TestGraphIndexLifecycleSuite`.
- [x] 4.4 Query handlers from the verb table, `QueryReady`, the reply envelope (design D5, D6). Gate: as 4.2 and
  `TestQueryDuringStartupReplayIsIndexNotReady`, `TestGraphIndexServesExactlyTheDeclaredVerbs`.
- [ ] 4.5 The alias index as ruled (comment 6426254115, item 3): no fix here; #415 linked from the PR without a
  closing keyword, and the alias read's exception in the spec names it (design D12). Gate: the link read back.

## 5. Ledger, records and docs

- [ ] 5.1 Ledger rows and edits of design D14; ADR-074, ADR-077, ADR-078 in `docs/adr/` with port notes and rows.
  Gate: `task ledger:check`.
- [ ] 5.2 Docs and records of design D16 (README, doc comments, repository map, contract lines, `query-pattern` skill,
  ADR-108 note). Gate: `task docs:check`, the adapter guard in `task verify`.

## 6. Evidence and review

- [ ] 6.1 `task mutate:check` for M1-M6 (design D11) and coverage of `processor/graph-index` and `graph/follower`, in
  `notes/6.1.md`; `task verify` passes on a clean tree at the last commit before the archive commit.
- [ ] 6.2 Cross-agent review stop; `reviewed-by:` in the PR body.
- [ ] 6.3 Comments on #107, #109, #110, #103, #106, #68, #195, #196 and #16 (the widened capture check, design
  D17), each "implemented in PR #414 at `<SHA>`, pending merge", read back.

## 7. Close

- [ ] 7.1 Archive (`openspec archive setup-04a-04-graph-roots-graph-index`) as the last content commit, syncing the
  four capabilities into `openspec/specs/`. Gate: `task spec:check` and `task docs:check` on the archive commit.
