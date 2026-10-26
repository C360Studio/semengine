# Tasks: setup-04a-04-graph-roots-graph-index

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171).

## 1. Design and measurement

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner, with owner questions 1-3 answered. Gate: `task spec:check` and `task docs:check`.
  Hold: owner acceptance and the size waiver (design D1).
- [ ] 1.2 Pin probe re-run with `-v` to record the shuffle seed; result in `notes/1.2.md`. Gate: the note.
- [ ] 1.3 #108's number: the pin's `owner_filter_load_integration_test.go` load shape at the pin, then on the port,
  recording entities, KV writes and the `index_not_ready` window, posted on #108 and in `notes/1.3.md` (design D13).
  Gate: the comment read back.
- [ ] 1.4 nats.go v1.54.0 probe: does a broker restart close a KV watcher's `Updates()`? Program, command, versions and
  output on PR #414 (inventory E2). Gate: the comment read back.

## 2. Tests first

- [ ] 2.1 `graph`: `TestQueryResponseCarriesIndexedRevisionAndProducer` and the empty-producer refusal; `graph/readiness`:
  the ported `ComputeIndexStatus` tests and the revision gauges' names against the registry. Gate: in 3.1.
- [ ] 2.2 Follower tests of design D11 (`TestFollowerRunReturnsAndLeavesNothing`, `TestFollowerPublishesOnEveryHeartbeat`,
  `TestFollowerSnapshotEndSetsBootstrapTarget`, `TestFollowerLatchesResetOnUnreadableSnapshotValue`,
  `TestFollowerDeclaresLostFeed`), the lost-feed test shown failing against the pin's loop shape first (`notes/2.2.md`).
  Gate: in 3.2.
- [ ] 2.3 graph-index tests of design D11, the ported pin tests rewritten without sleeps, skips, `t.Parallel` in
  integration files, the six writer back doors and `subscribeForRequests`; each dropped pin test named with its reason
  in `notes/2.3.md`. Gate: in 4.x.

## 3. Shared pieces

- [ ] 3.1 `graph/query_response.go`, `graph.QueryVerbs()` entries and `ResponderGraphIndex`; `ComputeIndexStatus`,
  `IndexStatusInputs` and the revision gauges in `graph/readiness` (design D2, D6, D10). Gate: `go test -race
  -count=1 ./graph/...`, `TestGraphImportsNoTransport`, `TestReservedSubjectsDeclaredOnce`.
- [ ] 3.2 `graph/follower` (design D3). Gate: `go test -race -count=1 ./graph/follower`, `task test:integration --
  ./graph/follower`, `go test -count=5 -cpu 1 -shuffle=on ./graph/follower`.

## 4. graph-index

- [ ] 4.1 Configuration, factory, registration, metrics (design D9, D10). Gate: `go test -race -count=1
  ./processor/graph-index`, the metric-registry guard in `task lint`.
- [ ] 4.2 Index write path on the keyed pool, coalescer, repair loop, owner reconciliation (design D7, D8). Gate:
  as 4.1 plus `task test:integration -- ./processor/graph-index`.
- [ ] 4.3 Lifecycle on `component.Lifecycle` with the follower; Health and DataFlow (design D4). Gate: as 4.2 and
  `TestGraphIndexLifecycleSuite`.
- [ ] 4.4 Query handlers from the verb table, `QueryReady`, the reply envelope (design D5, D6). Gate: as 4.2 and
  `TestQueryDuringStartupReplayIsIndexNotReady`, `TestGraphIndexServesExactlyTheDeclaredVerbs`.
- [ ] 4.5 The alias index as owner question 3 is answered: the fix and its `[A] -> [B] -> []` test, or the filed
  issue linked from the PR without a closing keyword (design D12). Gate: the test, or the issue read back.

## 5. Ledger, records and docs

- [ ] 5.1 Ledger rows and edits of design D14; ADR-074, ADR-077, ADR-078 in `docs/adr/` with port notes and rows.
  Gate: `task ledger:check`.
- [ ] 5.2 Docs and records of design D16 (README, doc comments, repository map, contract lines, `query-pattern` skill,
  ADR-108 note). Gate: `task docs:check`, the adapter guard in `task verify`.

## 6. Evidence and review

- [ ] 6.1 `task mutate:check` for M1-M7 (design D11) and coverage of `processor/graph-index` and `graph/follower`, in
  `notes/6.1.md`; `task verify` passes on a clean tree at the last commit before the archive commit.
- [ ] 6.2 Cross-agent review stop; `reviewed-by:` in the PR body.
- [ ] 6.3 Comments on #107, #109, #110, #103, #106, #68, #195, #196, each "implemented in PR #414 at `<SHA>`, pending
  merge", read back.

## 7. Close

- [ ] 7.1 Archive (`openspec archive setup-04a-04-graph-roots-graph-index`) as the last content commit, syncing the
  four capabilities into `openspec/specs/`. Gate: `task spec:check` and `task docs:check` on the archive commit.
