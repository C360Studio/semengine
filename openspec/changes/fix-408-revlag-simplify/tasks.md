# Tasks: fix-408-revlag-simplify

Design track: inventory, proposal, design, spec deltas for `revision-watermark` and `entity-state-follower`. Codex
reviews the design and the code. Each step ends when `task verify` passes on a clean tree at its commit (#171).

## 1. Proposal and design

- [ ] 1.1 Inventory, proposal, design and spec deltas committed. Gate: `task spec:check` and `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval
- [ ] 1.2 Independent review of `inventory.md` (INVENTORY PASS), then of `design.md` and the spec deltas (no BLOCKING
  or HIGH open), recorded on PR #548. Hold: Codex design review

## 2. Code

- [ ] 2.1 `internal/revlag`, tests first (design D1-D6; premises `internal/revlag/watermark.go:58-71`, `:101-131`,
  `:174-206`, `internal/revlag/watermark_test.go:458-525`, `:563-749`). First commit: the tests, failing on behaviour.
  `Observe` may gain its `error` result returning nil in that commit so the tests compile. The tests are: the
  time-rule tests (`TestWatermark_TimeIsOldestInFlight`, `TestWatermark_CompletionAboveInFlightKeepsTime`, updated
  expectations in the existing tests per inventory § 2); `TestWatermark_ObserveRefusesNonAscending`; the checkpointed
  `TestWatermark_ConcurrentReadersAndWriters` (D4); and `TestPropWatermarkHistory` over the test's own model, with
  `pinref_test.go` and the `len(w.pending)` reads removed (D5). Second commit: the implementation. Refusal, the time
  rule, in-flight bookkeeping meeting D3's four claims, covered estimate removed, doc comments rewritten. Gate: each
  new or changed assertion fails on the base commit (recorded); the #408 commands pass
  (`go test -race -count=1 -cpu=1 ./internal/revlag`, `go test -count=5 -cpu=1 -shuffle=on ./internal/revlag`);
  `task mutate:check` detects N1-N6; benchmark at 10,000 and 40,000 in flight and the prop seed and check counts in
  `notes/2.1.md`; `task verify` passes on a clean tree.
- [ ] 2.2 `graph/follower`, test first (design D2; premise `graph/follower/follower.go:262-264`, `:287-292`).
  `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery`, a unit test over a fake `kvcatalog.CatalogReader` delivering
  revision 5 then 3: `Stuck` names the lost feed, `QueryReady` returns `index_not_ready`. It fails on the base commit.
  Then `observe` declares the feed lost on a refused `Observe`. Doc comments updated: `follower.go:263`,
  `graph/readiness/index_status.go:23-26`, `graph/index_status.go:86-88`, `:106-108`, and
  `pkg/graphview/view.go:356-360` (after merging `origin/main` if #544 has landed). Gate: the test fails on base and
  passes after; `task mutate:check` detects F1; `go test -tags integration ./graph/follower ./processor/graph-index`
  passes; `task verify` passes on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record approves the head, with no BLOCKING or HIGH open; the PR body carries
  `implemented-by:`, `reviewed-by: codex` and `Closes #408`. Hold: Codex code review

## 4. Docs, ADR and ledger

- [ ] 4.1 ADR-083 amendment appended at the end of the record (design D7; ADR-109's line citations re-checked);
  ledger rows `pkg/revlag` and ADR-083 updated (D8); `docs/repository-map.md:98` updated; `revision-watermark` Purpose
  updated. Gate: `task docs:check`, `task spec:check`.

## 5. Archive

- [ ] 5.1 Archive as the last content commit (`openspec archive`), specs synced. Gate: `task spec:check` and
  `task docs:check`.
