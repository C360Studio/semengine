# Tasks: fix-369-subject-grammar

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit, or which mutant it detects. Each code task ends only when `task verify` passes on a clean
tree at its commit (#171). Tasks 2.1 and 2.2 land in one commit: the tests name `TypeMalformedSubject`, so they do
not compile without it; the failing run on the base is recorded in `notes/2.1.md`.

## 1. Design

- [x] 1.1 Inventory (`inventory.md`) passes independent inventory review (`INVENTORY PASS`, round 2), committed at
  807c367. Gate: the reviewer's verdict.
- [x] 1.2 Design (`design.md`), proposal and the `composition-validation` delta under `specs/` written, following the
  owner's ruling on #369 (2026-10-23). Gate: `task spec:check` and `task docs:check`.
- [x] 1.3 Claude design review. A Claude `semengine-reviewer` passes `design.md` and the delta; its verdict is
  linked from a PR comment.
- [x] 1.4 Codex design review. Codex passes `design.md` and the delta; its verdict is linked from a PR comment.
- [x] 1.5 Owner design acceptance. The owner accepts the design on #369 or PR #380.

## 2. Code

- [ ] 2.1 Tests first: T1–T3 and the T4 vocabulary case, with the "fourteen" comment above
  `TestValidateFindingsVocabularyIsClosed` made "fifteen" (`design.md`, "Proving tests"), against the hand-written
  oracle. Gate: each fails on the base (`notes/2.1.md`).
- [ ] 2.2 `TypeMalformedSubject` and the check in `composition.Analyze`, before the early returns. No change to
  `component/`, `config/` or `service/`. Gate: `task verify` green.
- [ ] 2.3 Sensitivity: mutants M1–M7 run with `task mutate:check`, each reported as detected or as a survivor
  (`notes/2.3.md`). Gate: the notes file.
- [ ] 2.4 Hold: Codex code review. Codex passes the code at a named commit with a local `task verify`; the PR body
  carries `implemented-by:` and `reviewed-by:`.

## 3. Docs and archive

- [ ] 3.1 `composition/doc.go` names `malformed_subject`; the `composition` row's `contract` in
  `docs/admission-ledger.yaml` gains the `malformed_subject` clause. Gate: `task verify` green.
- [ ] 3.2 Delta synced into `openspec/specs/composition-validation/spec.md`, with the Purpose line's "fourteen" made
  "fifteen"; archived as the last content commit. Gate: `task spec:check` and `task docs:check`.
