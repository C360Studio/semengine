# Tasks: setup-04a-04-graph-roots-embedding

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171). One task is one developer's unit.

## 1. Design and measurement

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review; owner
  acceptance. Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 Post the nats.go v1.54.0 / nats-server v2.15.1 KV probe (inventory 7, P1-P14) on PR #458. Posted as PR #458
  comment 6472856197.
- [x] 1.3 Owner rulings on #35 and #107. Ruled at PR #458 comment 6473224284.
- [ ] 1.4 File the `internal/kvwatch` adoption issue listing design D11's sites, linked from the PR without a closing
  keyword. Gate: the issue read back.

## 2. Tests first

- [ ] 2.1 Port the pin's unit tests for `bm25_embedder.go`, `dedup.go`, `vector.go` and the truncation and identity
  helpers, without the two HTTP test files and without `time.Sleep`; add `TestEmbeddingImportsNoProviderModule` in
  `internal/harness/contract` (design D9). Pin tests dropped or rewritten are named in `notes/2.1.md`. Gate: in 3.1.
- [ ] 2.2 Storage tests of design D9: `TestRevisionGuardHistoryMatchesModel` (sequential),
  `TestConcurrentSavesEndAtHighestRevision` and `TestSaveDedupKeepsSameSpaceReplacesOther` (integration,
  `natsfixture`, #195), the pin's storage unit tests on fakes behind `NewKVStore`, each changed behaviour shown
  failing against the pin's shape; seed and run count in
  `notes/2.2.md`. Gate: in 3.2.
- [ ] 2.3 Worker and cache tests of design D9: the unit `Run` tests (synctest on fakes, nil, second `Run` for both
  types), the integration `Run` tests (`TestWorkerRunLeavesNoWatchDelivery`, `TestVectorCacheRunLeavesNoWatchDelivery`,
  both lost-watch tests), `TestVectorCacheNotAuthoritative`, `TestFindSimilarOrderAndLimit`,
  `TestTerminalCallbackOncePerOutcome`, `TestPanickingEntryStaysPending`, `TestCASExhaustionIsNotTerminal`,
  `TestFailureWithCanceledTextIsAFailure`, `TestBothLanesEmbedIdenticalBytes`, `TestWorkerSupersededRevisionIsDropped`
  (forced interleaving), and the ported `TestWorkerResolvesLiveRegistryStatePerFetch` on `natsfixture`. Evidence in
  `notes/2.3.md`. Gate: in 3.3 and 3.4.

## 3. The package

- [ ] 3.0 Move `graph/follower`'s `stopWatch`, `watchDrainBudget` and `errWatchDeliveryOpen` into `internal/kvwatch`
  and the follower test's `nativeDeliveryStacks` and `awaitNativeDeliveryBlocked` into `internal/harness/natsfixture`
  (doc naming the backlog and #454), with the follower calling both; behaviour unchanged (design D1, D4). Gate: the
  follower's tests, `TestStopWatchGivesUpOnADeliveryThatNeverCloses` (moved) and
  `TestFollowerRunReturnsAndLeavesNothing` pass unchanged in what they assert; `task verify`.

- [ ] 3.1 `embedder.go`, `bm25_embedder.go`, `dedup.go`, `vector.go` (dead helpers not carried), `derived_reasons.go`
  (design D1, D2, D7 docs). Gate: `go test -race -count=1 ./graph/embedding`; `go mod tidy` leaves
  `golang.org/x/sync` direct and adds no `go-openai`.
- [ ] 3.2 `storage.go` on two `*natsclient.KVStore`, every read-modify-write through `UpdateWithRetryRead`, the dead
  surface not carried (design D2, D3; ADR-110). Gate: as 3.1 plus `task test:integration -- ./graph/embedding`.
- [ ] 3.3 `vector_cache.go`: `VectorCache` with `Run` and `FindSimilar` (design D5). Gate: as 3.2.
- [ ] 3.4 `worker.go`: `Run(ctx)` in place of `Start`/`Stop`, `NewWorker` without the bucket, `WorkerMetrics` edits,
  shutdown classified by the error, one text-length default, unordered-callback doc (design D4, D6, D7). Gate: as 3.2
  plus `go test -count=5 -cpu 1 -shuffle=on ./graph/embedding`.
- [ ] 3.5 The mutation checks of design D9 (`task mutate:check`, the revision comparison removed from `SaveGenerated`,
  then from `SaveFailed`), recorded in `notes/2.2.md` with survivors reported as survivors. If a D6 proof cannot pass
  or a mutant survives, stop: the finding fails, the design is amended, and the Worker adopts
  `internal/dispatch.KeyedPool` per the owner's ruling. Gate: each named test fails on its mutant.

## 4. Ledger, records and docs

- [ ] 4.1 Ledger rows of design D8: `graph/embedding` (`adapt`, its `known_risks` naming change 5's required reaction
  to a lost-watch error) and `graph/embedding/http_embedder.go` (`defer-exclude`, naming #32). Gate:
  `task ledger:check`.
- [ ] 4.2 Package `doc.go` and `README.md` (design D7), the ADR-066 and ADR-063 port notes, the `docs/repository-map.md`
  rows for `graph/embedding` and `internal/kvwatch` (design D1, D8). Gate: `task docs:check`.
- [ ] 4.3 `graph/embedding merged` in `scripts/cover-check.sh` (design D8). Gate: `task cover:check` after
  `task test:integration`.

## 5. Evidence and review

- [ ] 5.1 `task verify` passes on a clean tree at the last commit before the archive commit; notes linked from a PR
  comment.
- [ ] 5.2 Cross-agent review stop; `reviewed-by:` in the PR body. The body retires #35 and #107 (design D10).
- [ ] 5.3 Comments on #141 and #195 ("implemented in PR #458 at `<SHA>`, pending merge"), read back.

## 6. Close

- [ ] 6.1 Archive (`openspec archive setup-04a-04-graph-roots-embedding`) as the last content commit, syncing
  `embedding-index` into `openspec/specs/`. Gate: `task spec:check` and `task docs:check` on the archive commit.
