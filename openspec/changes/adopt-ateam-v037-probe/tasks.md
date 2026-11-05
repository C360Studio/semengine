# Tasks: adopt-ateam-v037-probe

Each step ends when `task verify` passes on a clean tree at its commit (#171), with ateam v0.3.7 first on `PATH`. One PR.

## 1. Pin and probe

- [x] 1.1 Pin `v0.3.7` in both workflows, the merge-gate tool tests and `.agents/README.md`; `merge-check.yml` gains the
  `Merge check probe` job and the `merge-check` job's `needs`/`if`/`name` as ateam v0.3.7's workflow has them; the
  tool tests pin the probe's shape (base-commit checkout, the job skipped on `awaiting=true`, the full check on no
  answer) and are shown to fail when it is removed.
- [x] 1.2 `.agents/contracts/reviewer.md` takes v0.3.7's template paragraph; `ateam doctor --check` at v0.3.7 passes.

## 2. Prose and spec

- [x] 2.1 `AGENTS.md` Merge gate line; both spec deltas applied to `openspec/specs/`.
- [ ] 2.2 On this pull request once ready: `Merge check probe` answers `awaiting=true` and `Merge check` is absent before
  the review record, and runs once the record and `reviewed-by:` line exist; run links in the PR.
