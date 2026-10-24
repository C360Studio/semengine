# Tasks: fix-390-391-port-pick-family

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit. Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [ ] 1.1 Inventory (`inventory.md`) passes independent inventory review (`INVENTORY PASS`), recorded on PR #404.
  Gate: the reviewer's verdict.
- [ ] 1.2 Claude design review. A Claude `semengine-reviewer` passes `design.md`, this file and both spec deltas; its
  verdict is linked from a PR comment.
- [ ] 1.3 Codex design review and owner design acceptance on #404. Gate: the comments.
  Hold: Codex design review of record and the owner's acceptance, on PR #404.

## 2. Tests and code

- [ ] 2.0 Post PR #389's port-pick walk on #404: program source, command, go and macOS versions, output (20,000 binds
  each for `tcp` `:0`, `tcp4` `:0`, `tcp` `127.0.0.1:0`, beside the daemon on `127.0.0.1:63150`). Time-box 15 minutes;
  if it does not reproduce, record that and the docs say "not promised" only. Gate: the PR comment link.
- [ ] 2.1 Tests first: `internal/harness/contract/portpick_test.go` with `TestEveryAddressPortPickSaysWhy` and its
  sensitivity test (design "Proving tests"). Gate: the first fails on the base naming the nine sites; the sensitivity
  test passes; output in `notes/2.1.md`. The red test lands in task 2.4's commit.
- [ ] 2.2 `internal/harness/natsfixture/fixture.go` (`freePort` picks `tcp4`; comments `:36-37`, `:313-321` per D1)
  and `fixture_integration_test.go` (`deadPort` returns `freePort()`). Gate: `task verify`.
- [ ] 2.3 `internal/harness/natsfixture/start_port_integration_test.go`: `// Every address:` reasons at `:37`, `:57`,
  `:64`. Gate: `task test:integration` for the package.
- [ ] 2.4 `metric/lifecycle_test.go:70`, `service/pprof_test.go:93`, `service/metrics_owner_test.go:52,112`:
  reasons (D4). Gate: `TestEveryAddressPortPickSaysWhy` passes; `task verify` green.
- [ ] 2.5 `scripts/lint-test-ports.sh` header (D6). Gate: `task lint` (the script and its fixture test unchanged in
  behaviour).

## 3. Code review

- [ ] 3.1 Claude code review. A Claude `semengine-reviewer` passes the code at a named commit with a local
  `task verify`; recorded on PR #404.
- [ ] 3.2 Codex code review of record at a named commit with a local `task verify`; the PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`.
  Hold: Codex code review of record on PR #404.

## 4. Docs and archive

- [ ] 4.1 `docs/testing.md:498-500` (the family to pick, the reason comment, the release-check line, D5) and
  `docs/test-infrastructure.md:139-148` (the `tcp4` pick, measured not promised; the retry covers the gap).
  Gate: `task docs:check`.
- [ ] 4.2 `AGENTS.md` § 3 and `.agents/rules-index.md` § 3: one new row each (spec delta text). Gate: `task verify`
  (row-count check) and `AGENTS.md` under 24,000 bytes.
- [ ] 4.3 `docs/admission-ledger.yaml:248-265`: the `lint-test-ports.sh` destination notes the header corrected by
  this change (#391). Gate: `task ledger:check`.
- [ ] 4.4 Both deltas synced into `openspec/specs/`; archived as the last content commit. Gate: `task spec:check` and
  `task docs:check`.
