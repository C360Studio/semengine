# Tasks: fix-258-errs-zero-class

Light process, owner 2026-10-26: inventory, tests and code, then Codex's code review; no separate design document.
Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Inventory

- [x] 1.1 Inventory (`inventory.md`) recorded on the PR. Gate: the Claude inventory review's findings applied (one
  HIGH, three LOW). Applied at 56e1c29; the corrected text was not re-reviewed (light process, owner 2026-10-26).

## 2. Tests and code

- [x] 2.1 Tests first: `TestClassifiedErrorWithoutClass` in `pkg/errs/errs_test.go` fails on the base, recorded in
  `notes/2.1.md`. Gate: the failing output.
- [x] 2.2 `ErrorTransient ErrorClass = iota + 1`, plus a `contract` and `known_risks` line in the `pkg/errs` row of
  `docs/admission-ledger.yaml`. Gate: `task verify` green on a clean tree. Done at 7640c34: `task verify` printed
  `verify: ok` on a clean tree.

## 3. Code review

- [ ] 3.1 Codex code review of record at a named commit with a local `task verify`; the PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`. Gate: Codex's comment.
  Hold: Codex code review (owner starts Codex).

## 4. Archive

- [ ] 4.1 The `error-classification` delta synced into `openspec/specs/error-classification/spec.md`; archived as the
  last content commit. Gate: `task spec:check` and `task docs:check`.
