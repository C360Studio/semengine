# Tasks: fix-262-stale-days-operand

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit. Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory (`inventory.md`) passes independent inventory review (`INVENTORY PASS`), recorded on PR #381.
  Gate: the reviewer's verdict.
- [x] 1.2 Claude design review. A Claude `semengine-reviewer` passes `design.md`, this file and the `spec-queue`
  delta; its verdict is linked from a PR comment.
- [x] 1.3 Codex design review and owner design acceptance on #262 or PR #381. Q1 is answered: the range check
  (2026-10-23). Gate: the comments. Codex passed with one MEDIUM wording fix (termination promised for argument
  processing only), applied; the owner accepted the design on 2026-10-23.

## 2. Tests and code

- [x] 2.1 Tests first. `runSpecQueue` bounds each run under a deadline derived from `t.Context()`, and a run that has
  not ended fails, saying so. `TestSpecQueueArguments` covers cases A1–A6 of `design.md` with expected values written
  by hand. The file comment's "all four requirements" becomes "all six". Gate: A1 fails at the bound and A2, A3 and
  A6 fail with exit 0 on the base, recorded in `notes/2.1.md`.
- [ ] 2.2 The operand check at `scripts/openspec-queue.sh:63` plus the range check the owner chose for Q1. The usage
  line `:50` and the exit-2 sentence `:41` are edited in place, so the header keeps its line count and the `--help`
  range at `:64` stays right. Gate: `task verify` green.
- [ ] 2.3 Sensitivity by hand: mutants M1–M4 of `design.md`, each run against `TestSpecQueueArguments`, with baseline,
  mutant and restored runs reported as detected or survived (`notes/2.3.md`). Gate: the notes file.

## 3. Code review

- [ ] 3.1 Claude code review. A Claude `semengine-reviewer` passes the code at a named commit with a local
  `task verify`.
  Hold: waits for the Claude reviewer's PASS, recorded as a PR comment naming the commit.
- [ ] 3.2 Codex code review of record. Codex passes the code at a named commit with a local `task verify`; the PR body
  carries `implemented-by: claude (opus)` and `reviewed-by: codex`.
  Hold: waits for Codex's PASS, recorded as a PR comment naming the commit.

## 4. Docs and archive

- [ ] 4.1 `docs/testing.md:371-372` names `TestSpecQueueArguments` beside the other spec-queue tests. Gate:
  `task docs:check`.
- [ ] 4.2 Delta synced into `openspec/specs/spec-queue/spec.md`, with the Purpose's "exit 2 means it could not read
  the changes" made "exit 2 means it could not read the changes or refused its arguments". `docs/repository-map.md`
  `:91` adds "and the `fix-262-stale-days-operand` archive", plus an archived-change row like `:100-101`. Archived as
  the last content commit. Gate: `task spec:check` and `task docs:check`.
