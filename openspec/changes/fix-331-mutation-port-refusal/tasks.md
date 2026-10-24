# Tasks: fix-331-mutation-port-refusal

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit. Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory (`inventory.md`) passes independent inventory review (`INVENTORY PASS`), recorded on the PR.
  Gate: the reviewer's verdict. Passed at ef6848b with two NITs (spec line numbers, #393's file count), both corrected
  (PR #397 comment 6384552461).
- [x] 1.2 Claude design review. A Claude `semengine-reviewer` passes `design.md`, this file and the spec delta; its
  verdict is linked from a PR comment. Gate: the verdict. Passed at ef6848b with four MEDIUM findings (holds for the
  review stops, task lines for the ledger and #398 steps, file the ambiguous-refusal defect, a case where the malformed
  input is not last) and one NIT, all applied (PR #397 comment 6384552461).
- [ ] 1.3 Codex design review on the PR. Gate: Codex's comment.
  Hold: Codex design review (the owner starts Codex).
- [ ] 1.4 Owner design acceptance on #331 or the PR. Gate: the owner's comment.
  Hold: owner design acceptance.
- [x] 1.5 The ambiguous refusal the design leaves alone is filed as #399 (`status:blocked`, linked from the PR without a
  closing keyword). Gate: the issue.

## 2. Tests and code

- [ ] 2.1 Tests first (design "Proving tests"). Gate: the new assertions fail on the base, recorded in
  `notes/2.1.md` with the failing output.
- [ ] 2.2 The fix (design D1), plus D3's clause in item (8) of the `processor/graph-ingest` row's `contract` in
  `docs/admission-ledger.yaml`. Gate: `task verify` green, plus the design's mutants each reporting `verdict: detection`
  under `task mutate:check`, recorded in `notes/2.2.md`.

## 3. Code review

- [ ] 3.1 Claude code review. A Claude `semengine-reviewer` passes the code at a named commit with a local
  `task verify`.
- [ ] 3.2 Codex code review of record at a named commit with a local `task verify`; the PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`.

## 4. Docs and archive

- [ ] 4.0 Before archive, merge `origin/main`. If #398 (#305) has merged, re-copy the delta from whichever
  requirement on main carries graph-ingest's refusal sentence after the split, and apply D2's clause to it. If the
  openspec 1.14.1 bump has landed, keep each requirement under 500 characters. Gate: `task spec:check`.
- [ ] 4.1 The `graph-transport-boundary` delta synced into `openspec/specs/graph-transport-boundary/spec.md`;
  archived as the last content commit. Gate: `task spec:check` and `task docs:check`.
