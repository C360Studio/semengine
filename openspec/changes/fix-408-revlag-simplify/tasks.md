# Tasks: fix-408-revlag-simplify

Design track: inventory, proposal, design, spec deltas for `revision-watermark` and `entity-state-follower`. Codex
reviews the design and the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal and design

- [x] 1.1 Inventory, proposal, design and spec deltas committed. Gate: `task spec:check` and `task docs:check`.
- [x] 1.2 Independent review of `inventory.md` (INVENTORY PASS), then of `design.md` and the spec deltas (no BLOCKING
  or HIGH open), recorded on PR #548.

## 2. Code

- [x] 2.1 `internal/revlag`, tests first (design D1-D6; premises `internal/revlag/watermark.go:58-71`, `:101-131`,
  `:174-206`, `internal/revlag/watermark_test.go:290-325`, `:458-561`, `:563-749`).
  - First commit: the tests, failing on behaviour. `Observe` may gain its `error` result returning nil in that
    commit, so the tests compile. The tests:
    - the time-rule tests: `TestWatermark_TimeIsOldestInFlight`, `TestWatermark_CompletionAboveInFlightKeepsTime`, and
      updated expectations in the existing tests (inventory § 2);
    - `TestWatermark_ObserveRefusesNonAscending`;
    - the checkpointed `TestWatermark_ConcurrentReadersAndWriters`, with keys of their own for 1,500 and 2,000 (D4);
    - `TestWatermark_RetentionTracksInFlight` with a no-stuck case added. `TestWatermark_MemoryDoesNotGrowWithCompletions`
      and its `len(w.pending)` reads go (D5);
    - `TestPropWatermarkHistory` over the test's own model, with `pinref_test.go` deleted (D5).
  - Second commit: the implementation: the refusal, the time rule, in-flight bookkeeping meeting D3's four claims, the
    covered estimate removed, and the package's doc comments rewritten.
  - Gate:
    - each assertion of changed behaviour (the time rule, the refusal) fails on the base commit (recorded); retained
      guarantees (checkpoint A, the floor, retention) are shown by the D6 mutants instead;
    - the #408 commands pass: `go test -race -count=1 -cpu=1 ./internal/revlag` and
      `go test -count=5 -cpu=1 -shuffle=on ./internal/revlag`;
    - `task mutate:check` trips N1-N6 at the assertions D6 names;
    - `notes/2.1.md` records the benchmark at 10,000 and 40,000 in flight, and the prop seed and check counts;
    - `task verify` passes on a clean tree.
- [x] 2.2 `graph/follower`, test first (design D2; premise `graph/follower/follower.go:206-241`, `:262-264`, `:287-292`).
  - The test: `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery` is an in-package unit test. It builds a `Follower`
    with a fake `kvcatalog.CatalogReader` (only `WatchAll` implemented, returning a watcher whose channel delivers
    revision 5 then revision 3) and drives `follow`. After the second delivery, `Stuck` names the lost feed and
    `QueryReady` returns `index_not_ready`. It fails on the base commit.
  - The fix: `observe` declares the feed lost on a refused `Observe`.
  - The rename: `readiness.IndexStatusInputs.IndexedAt` becomes `OldestUnappliedAt` with its callers and doc mentions
    (design, owner rulings).
  - Doc comments: `follower.go:263`, `graph/readiness/index_status.go:23-26` and `:38`,
    `graph/index_status.go:86-88`, `:94-96` and `:106-108`, and `pkg/graphview/view.go:356-360` (after merging
    `origin/main` if #544 has landed).
  - Gate: the test fails on base and passes after; `task mutate:check` trips F1;
    `go test -tags integration ./graph/follower ./processor/graph-index` passes; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open; the PR body carries
  `implemented-by:`, `reviewed-by: codex` and `Closes #408`. Hold: Codex code review

## 4. Docs, ADR and ledger

- [ ] 4.1 ADR and docs edits (design D7, D8):
  - ADR-083 amendment appended at the end of the record;
  - ADR-083 `:7`, ADR-083 `:200` and ADR-109 `:200` updated in place, each keeping its line count;
  - ADR-109's citations into ADR-083 re-checked; if #482 landed first, merge `origin/main` and recheck D7's lines;
  - ledger rows `pkg/revlag` and ADR-083 updated;
  - `docs/repository-map.md:98` and the `revision-watermark` Purpose updated.

  Gate: `task docs:check`, `task spec:check`.

## 5. Archive

- [ ] 5.1 Archive as the last content commit (`openspec archive`), specs synced. Gate: `task spec:check` and
  `task docs:check`.
