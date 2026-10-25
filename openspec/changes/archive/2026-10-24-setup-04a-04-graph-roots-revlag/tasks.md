# Tasks: setup-04a-04-graph-roots-revlag

Each task names its outcome and the gate that proves it. Tests come first. Each stop's task ends only when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass pre-owner review and are accepted by the
  owner, Q1 answered. Gate: `task spec:check` and `task docs:check`.

## 2. Tests

- [x] 2.1 `internal/revlag/watermark_test.go` (design D7, D8): the pin's 13 tests with each `Indexed()` read taken
  from `IndexedAt()`; `TestWatermark_ObservedIsHighestDelivered`, `TestWatermark_DrainAtFloorIsCovered`,
  `TestWatermark_CompleteWithNothingInFlight`, `TestWatermark_ConcurrentReadersAndWriters`; `TestPropWatermarkHistory`
  against `internal/revlag/pinref_test.go`, the pin's code. Written before the package source. Gate: in 3.1.

## 3. Port

- [x] 3.1 `internal/revlag/watermark.go` as design D2-D4: the pin's code without `Indexed`, with the doc lines that
  name it reworded. Gate: `go test -race -count=1 ./internal/revlag`, `go test -count=5 -cpu 1 -shuffle=on
  ./internal/revlag`, `go list -deps ./internal/revlag` naming only standard-library packages; seed and checks
  completed for `TestPropWatermarkHistory` in `notes/3.1.md`.
- [x] 3.2 Design D5 (Q1 yes): `Complete` and the floor read no longer scan every in-flight revision; the 2.1 tests pass
  unchanged; new `TestWatermark_RetentionTracksInFlight` (design D7) passes; benchmark at 10,000 and 40,000 in flight
  in `notes/3.2.md`. Gate: as 3.1.

## 4. Ledger, status, docs

- [x] 4.1 The `adapt` row (design D10); the `internal/revlag/` map row and the corrected `docs/repository-map.md`
  lines 12-19; the ADR-066 port note (design D11). Gate: `task ledger:check`, `task docs:check`, `task status --
  --offline` showing change 4 at 1/8.

## 5. Evidence and review

- [x] 5.1 `task mutate:check` for M1-M6 (detections) and S1 (survivor recorded), and coverage, in `notes/5.1.md`
  (verdicts from `notes/3.2.md:169-178`, run on the file the code stop approved).
- [x] 5.1a Coverage (100%, `notes/5.1.md`) recorded on the PR; `task verify` passes on a clean tree at the final head.
- [x] 5.2 Cross-agent review stop (Codex); `reviewed-by:` in the PR body.

## 6. Close

- [x] 6.1 Archive (`openspec archive setup-04a-04-graph-roots-revlag`) as the last content commit, syncing
  `revision-watermark` into `openspec/specs/` with the Purpose design D6 gives. Gate: `task spec:check` and
  `task docs:check`.
