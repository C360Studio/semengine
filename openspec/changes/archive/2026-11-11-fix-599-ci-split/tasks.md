# Tasks: fix-599-ci-split

Each step ends when `task verify` passes on a clean tree at its commit (#171). One PR, closing #599.

## 1. Two CI jobs

- [x] 1.1 `scripts/verify.sh` takes an optional group (`checks`, `integration`) and walks the one step list, skipping
  what the group leaves out (design D1); `Taskfile.yml` `verify` runs `scripts/verify.sh {{.CLI_ARGS}}`. Add
  `TestVerifyGroups` to `internal/harness/toolcheck/mergegate_test.go`, failing first, shown able to fail by a planted
  `verify.sh` whose `integration` group drops `cover:check`.
- [x] 1.2 Split `.github/workflows/ci.yml` `verify` into `checks` (limit 15, `task verify -- checks`, checkout with
  `fetch-depth: 0`) and `integration` (limit 20, `task verify -- integration`), the setup steps in each, and make
  `required` need both (design D2, D5, D6). Update `ciWorkflowViolations`, its constants and the
  `TestCIWorkflowPinnedSensitivity` fixture and cases (both jobs' limits, run lines, `required`'s needs). The PR
  records each job's wall time from its own CI run.
- [x] 1.3 Say there are two CI jobs in `AGENTS.md` (the "CI:" line), `.agents/protocol.md` § Verification,
  `.agents/rules-index.md` (the "A merge needs `Required`" row's spec citation), `.agents/skills/preflight/SKILL.md`
  (the `task verify` row and the CI paragraph), `docs/repository-map.md` (the `.github/` row) and the
  `scripts/tools-changed.sh` comment that names the verify job.

## 2. Spec

- [x] 2.1 Apply the delta to `openspec/specs/merge-gate/spec.md` and archive the change.
