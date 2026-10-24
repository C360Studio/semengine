# Tasks: fix-331-mutation-port-refusal

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit. Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory (`inventory.md`) passes independent inventory review (`INVENTORY PASS`), recorded on the PR.
  Gate: the reviewer's verdict.
- [ ] 1.2 Claude design review. A Claude `semengine-reviewer` passes `design.md`, this file and the spec delta; its
  verdict is linked from a PR comment. Gate: the verdict.
- [ ] 1.3 Codex design review and owner design acceptance on #331 or the PR. Gate: the comments.

## 2. Tests and code

- [ ] 2.1 Tests first (design "Proving tests"). Gate: the new assertions fail on the base, recorded in
  `notes/2.1.md` with the failing output.
- [ ] 2.2 The fix (design). Gate: `task verify` green, plus the design's mutants each reporting `verdict: detection`
  under `task mutate:check`, recorded in `notes/2.2.md`.

## 3. Code review

- [ ] 3.1 Claude code review. A Claude `semengine-reviewer` passes the code at a named commit with a local
  `task verify`.
- [ ] 3.2 Codex code review of record at a named commit with a local `task verify`; the PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`.

## 4. Docs and archive

- [ ] 4.1 The `graph-transport-boundary` delta synced into `openspec/specs/graph-transport-boundary/spec.md`;
  archived as the last content commit. Gate: `task spec:check` and `task docs:check`.
