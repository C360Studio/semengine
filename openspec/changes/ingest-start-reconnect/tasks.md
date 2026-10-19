# Tasks: ingest-start-reconnect

Each task names its outcome and the gate that proves it. Tests come first. Each stop's task ends only when
`task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`, round 1 at `c0299fd`); proposal and design pass pre-owner review
  (`DESIGN REVIEW PASS`, round 2 at `a5d7d52`) and are accepted by the owner
  (2026-10-19, Enjoy conversation: "Accept as designed; no spec requirement"). Gate: `task spec:check` and `task docs:check`.

## 2. Implementation

- [x] 2.1 Tests first: the relay helper, T1 and T2 (D2); the fail-first runs on today's code recorded in
  `notes/2.1.md`. Gate: both fail as D2 states; `go vet -tags integration ./processor/graph-ingest/`.
- [x] 2.2 The fix and the doc comment (D1); mutants M1-M3 (D3) recorded in `notes/2.2.md`. Gate: `task verify` on a
  clean tree.
- [ ] 2.3 Review stop (Codex) on 2.1-2.2; `reviewed-by:` in the PR body.

## 3. Archive

- [ ] 3.1 Archive the change (`skip_specs`). Gate: `task spec:check` and `task docs:check`. `task merge:check -- 316`
  is recorded on PR #316 after the archive; no task claims it.
