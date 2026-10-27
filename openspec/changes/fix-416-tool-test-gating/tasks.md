# Tasks: fix-416-tool-test-gating

Light process, owner 2026-10-26: inventory, proposal, spec deltas and tasks; no design document; Codex reviews the
code. Tests come first: 3.2 and 3.3 are written and shown failing before their code, with the failure recorded in
notes, and they are committed with the code of 4.1 and 4.2, so every commit passes `task verify` on a clean tree
(#171).

## 1. Inventory and proposal

- [x] 1.1 Inventory (`inventory.md`) committed at d61fcad; the owner ruled its open question on PR #419 (comment
  6429349207, "Yes, split it"). Gate: the comment read back.
- [x] 1.2 Proposal, tasks and the `merge-gate` and `integration-test-runner` deltas committed (Claude-side review
  PASS on `918cebc`, its two LOWs applied in `0b9d396`). Gate: `task spec:check`
  and `task docs:check`.

## 2. Probe

- [x] 2.1 Repeat the proposal's Task 3.51.1 probe (a failing `sh:` variable fails the task before its commands; the
  output is split into words; `-- <pkgs>` replaces the default) and record its Taskfile, command and output in
  `notes/2.1.md`. Gate: the output. If Task behaves otherwise, stop and report on PR #419 before task 4.2.

## 3. Tests

- [x] 3.1 The package move: `git mv` the seven files the proposal names into `internal/harness/toolcheck/`. Move
  `TestLintTestPortsHonoursNoMarker` and `TestLintTestPortsSkipsIgnoredFiles` into `toolcheck/lintports_test.go`;
  `TestLintTestPortsPasses` stays in `contract`. Copy into each package the helpers it calls and only those. Add
  `toolcheck/doc.go`, which names the trigger paths, the rule for which tests belong in the package, and the
  repository files each tool test reads. Test bodies stay as they are. Gate: `go test -count=1 ./internal/harness/contract/
  ./internal/harness/toolcheck/` passes. The test names (`go test -list '.*'`) of the two packages together equal
  `contract`'s list at the base. `go list -deps -test ./internal/harness/toolcheck/` names no
  `github.com/c360studio/semengine` package except toolcheck. `task verify` passes.
- [x] 3.2 Pins written before the code: in `TestUnitInvocationsPinned`, the new command lines, the `vars` parsing,
  and the sensitivity cases; in `TestCIWorkflowPinned`, the `fetch-depth: 0` case (proposal, "Pins and their failure
  tests"). Gate: both fail against the unchanged `Taskfile.yml`, `scripts/verify.sh` and `ci.yml`, with the output in
  `notes/3.2.md`. Committed with 4.2.
- [x] 3.3 `TestTestPkgsLists` (`toolcheck/testpkgs_test.go`, planted modules only) and `TestToolsChanged`
  (`toolcheck/toolschanged_test.go`, local cases without the `GITHUB_*` variables), with the cases the proposal lists.
  Gate: both fail with the scripts absent, with the output in `notes/3.3.md`. Committed with 4.1.

## 4. Code

- [x] 4.1 `scripts/test-pkgs.sh` and `scripts/tools-changed.sh`. Gate: 3.3 passes;
  `go test -count=5 -cpu 1 -shuffle=on ./internal/harness/toolcheck/` passes.
- [x] 4.2 `Taskfile.yml` (`test:unit`, `test:repeat`, `test:integration`, `test:tools`, `test:tools:changed`),
  `scripts/verify.sh` steps, and `fetch-depth: 0` in `ci.yml`. Gate: 3.2 passes; `task test:integration` runs
  exactly the packages `scripts/test-pkgs.sh integration` prints and `task cover:check` passes after it;
  `task verify` passes on a clean tree.

## 5. Docs and rules

- [x] 5.1 `AGENTS.md` and `.agents/rules-index.md`, row for row. Edit rows `AGENTS.md:114` and `:116`, matching
  rules-index `:59` and `:61`. In rules-index `:78`, `:101`, `:104` and `:112`, the enforcer cells name tests that
  now run under `task test:tools`. Add one new row in domain 3, in both files: tool tests run only when their trigger
  paths change, the repository guards run on every verify. Add `task test:tools` to the commands block, and
  `test:tools:changed` to its `task verify` step list (`AGENTS.md:51-52`). `AGENTS.md`
  stays under 24,000 bytes (23,082 at the base). Gate: `TestAgentSetupParity` in `task verify`, and
  `task docs:check`.
- [x] 5.2 `docs/testing.md` `:41-44` (the unit line no longer says every package, plus a "Tool tests" line),
  `docs/test-infrastructure.md:114`, `docs/repository-map.md:44` (the contract line, plus a toolcheck line),
  `.agents/skills/semengine-preflight/SKILL.md:41-47` (including the `task verify` line at `:47`, which gains
  `test:tools:changed`) and the `test:integration` row of
  `docs/admission-ledger.yaml` (`:212-217`). `.agents/README.md:138` and
  `internal/harness/contract/ledger_test.go:16` are checked and left unchanged, because both stay true after the
  split. Gate: `task docs:check`, `task ledger:check` and `task verify`.

## 6. Measure

- [x] 6.1 Run `task verify` once at the base (d61fcad) and twice at the last code commit on the same machine. One
  after-run has a scripts change, so the tool tests run; the other has a docs-only change, so they are skipped. Record
  the step timings and the load average of each run, and post them on PR #419 with commands and SHAs. Read the comment
  back. Gate: the comment.

## 7. Code review

- [ ] 7.1 Codex code review of record at a named commit with a local `task verify`; the PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`.
  Hold: Codex's review of record on PR #419, approving at a named commit with a local `task verify`.

## 8. Archive

- [ ] 8.1 Sync the `merge-gate` and `integration-test-runner` deltas into `openspec/specs/` and archive as the last
  content commit. Gate: `task spec:check` and `task docs:check` on the archive commit.
