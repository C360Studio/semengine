# Tasks: setup-04a-04-graph-roots-graphview

Each task names its outcome and the gate that proves it. Tests come first. Each step ends only when `task verify`
passes on a clean tree at its commit (#171). One task is one developer's unit.

## 1. Design and measurement

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); proposal and design pass the independent pre-owner review; owner
  acceptance, including the proposal's three questions. Gate: `task spec:check` and `task docs:check`.
  Owner accepted 2026-10-30 (proposal `Approved-by:`). Codex approved the design at `4526573` (PR #476 comment
  6522948775; its BLOCKING `Hooks.OnTick` finding, comment 6522851399, is fixed in `4526573`).
- [x] 1.2 Post the nats.go KV watch probes (inventory 7: G1-G5 `/tmp/gv-probe`, G6 `/tmp/gv-probe-g6`) on PR #476
  with source, command, versions and output; re-run them at the new version if #464 has merged first. Gate: the
  comment read back. Posted as PR #476 comment 6522730160 (nats.go v1.54.0; #464 still open).
- [x] 1.3 Comment on #461 that `pkg/graphview` is a new `kvwatch.Stop` caller (design D7), no closing keyword. Gate:
  the comment read back. Posted as #461 comment 6522988165.
- [x] 1.4 File the adoption issue listing design D7's sites, linked from the PR without a closing keyword. Gate: the
  issue read back. Filed as #477, linked in PR #476 comment 6522999612.

## 2. Tests first

- [x] 2.1 Port the pin's unit tests onto `Run` in `synctest` (design D6): the fake source closes `Updates()` when the
  watch context ends (a change: the pin fake ignores it, `harness_test.go:56`); the 50 ms windows become
  `synctest.Wait`; `time.After(testWait)` bounds go; the concurrent seam test takes its expected state from the
  writes; the pin harness's `OnTick` waits (`harness_test.go:128`, `hooks_test.go:44`) become `synctest.Wait` after
  the tick. Tests of dropped surface are rewritten on kept surface or dropped, each named in `notes/2.1.md`. Gate:
  in 3.2. Done at `7aa3a21`: unit gates of 3.2 green, `task verify` ok.
- [x] 2.2 The new unit tests of design D6 (`TestRunRefusesNilContext`, `TestRunOnce`, `TestRunLeavesNothingBehind`,
  `TestRunContextEndIsAStop` with its constructed both-ready loop, `TestRunLostWatchFailsClosed`, `TestRunOpenFailure`,
  `TestDecodeOncePerWrite`) and the product-shaped tests of
  each kept symbol with no reader (design D3), each shown failing against the pin's shape. Gate: in 3.2. Done:
  unit gates green, each test's wrong change in `notes/2.2.md`.
- [x] 2.3 `TestViewHistoryMatchesModel` (design D6): seed, run count and completed checks in `notes/2.3.md`. Gate:
  in 3.2. Done: unit gates green; three wrong changes detected, recorded in `notes/2.3.md`.
- [ ] 2.4 Integration tests of design D6 on `natsfixture.Open`: the pin's real-bucket test,
  `TestRunLeavesNoWatchDelivery`, `TestRunReportsLostWatch`. Gate: in 3.2.

## 3. The package

- [x] 3.1 `doc.go`, `errors.go`, `delta.go` with the dropped fields gone and the docs of design D4 (S10). Gate:
  `go build ./pkg/graphview`.
- [ ] 3.2 `view.go` and `subscription.go`: `Run(ctx)` in place of `Start`/`Stop` (design D2), `kvwatch.Stop`, the
  dropped surface (design D3), and the optional removals the owner accepted at 1.1 (design D4). Gate:
  `go test -race -count=1 ./pkg/graphview`, `go test -count=5 -cpu 1 -shuffle=on ./pkg/graphview`,
  `task test:integration -- ./pkg/graphview`. Unit gates green at `7aa3a21`; ticks with 2.4, which adds the
  integration tests.
- [ ] 3.3 The mutation checks of design D6 (`task mutate:check`), recorded in `notes/3.3.md`, survivors reported as
  survivors; a survivor is a gap fixed before 5.1. Gate: each named test fails on its mutant.

## 4. Ledger, records and docs

- [ ] 4.1 Ledger rows: `pkg/graphview` (`adapt`: `Run`, the stop/loss split, `kvwatch.Stop`, dropped surface; the
  contract field names the kept symbols' contract lines) and `docs/adr/081-graph-view-subscription.md`. Gate:
  `task ledger:check`.
- [ ] 4.2 `pkg/graphview/README.md`; ADR-081 ported with its port note (design D5); the ADR-085 port note; the
  `docs/repository-map.md` row and the `docs/adr/` row; the `kv-or-stream` skill line (design D1). Gate:
  `task docs:check`.
- [ ] 4.3 `pkg/graphview unit` in `scripts/cover-check.sh` and `d11Targets` in
  `internal/harness/toolcheck/cover_test.go`; the measured figure on the PR. Gate: `task cover:check` after
  `task test:integration`, and `task test:tools`.

## 5. Evidence and review

- [ ] 5.1 `task verify` passes on a clean tree at the last commit before the archive commit; notes linked from a PR
  comment.
- [ ] 5.2 Cross-agent review stop; `reviewed-by:` in the PR body. The body advances #195 and addresses #9, with no
  closing keyword for either.
- [ ] 5.3 Comment on #195 ("implemented in PR #476 at `<SHA>`, pending merge", this package's site), read back.

## 6. Close

- [ ] 6.1 Archive (`openspec archive setup-04a-04-graph-roots-graphview`) as the last content commit, syncing
  `graph-view-subscription` into `openspec/specs/`. Gate: `task spec:check` and `task docs:check` on the archive
  commit.
