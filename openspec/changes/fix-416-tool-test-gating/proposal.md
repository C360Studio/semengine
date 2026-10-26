# Proposal: fix-416-tool-test-gating

## Why

`task verify` spends most of its test time on the tests of the repository's own tools, and #416 is the result. On
PR #413's head `6911630` (local Mac) `test:unit` took 169 s, of which `internal/harness/contract` took 164 s, mutcheck
67 s and pindiff 16 s. `test:integration` took 211 s: contract 99 s and mutcheck 32 s, because it re-runs every
package. `test:repeat` took 358 s, contract alone 355 s, and five rounds of contract in one test binary passed Go's
10-minute timeout when the machine was busy (#416). These tests check scripts, `Taskfile.yml` and CI, which change in
few pull requests, and they run in full on every one.

Owner, 2026-10-26: "lets just assume it was a total waste of fucking time and see what we can do to clean up our
testing dumpster fire". The owner chose the shape (PR #419 body) and ruled the inventory's open question on PR #419
(comment 6429349207): "Yes, split it". The checks over the repository's own files stay in `contract` and run in
`test:unit` on every change. The tests that drive a script move to a new package, and that package is gated with
mutcheck and pindiff. Light process: inventory, this proposal, spec deltas and tasks; no design document; Codex
reviews the code.

## What Changes

- **New package `internal/harness/toolcheck`.** The rule for which tests move is what each test reads. A test
  moves when every repository file it reads is under a trigger path (below); the rest of what it reads is planted in a
  temporary tree. A test that reads any other file of the real tree stays in `contract`. Only review enforces this
  rule. By that rule these files move out of `contract` unchanged (`git mv`): `mergecheck_test.go`,
  `specqueue_test.go`, `cover_test.go`, `portstatus_test.go`, `mergegate_test.go`, `ledgercheck_test.go` and
  `cleanuproots_test.go`. `TestLintTestPortsHonoursNoMarker` and `TestLintTestPortsSkipsIgnoredFiles` move out of
  `addresses_test.go`, because they run the script on a planted tree. `TestLintTestPortsPasses`
  (`addresses_test.go:54-59`) runs the script over the real tree, so it stays, with `TestNoFixedAddressesInTests` and
  `TestEveryAddressPortPickSaysWhy`.
  Each package keeps its own copy of the test helpers it calls (`repoRoot`, `copyScript`, `fakeBin`,
  `requireViolation`, `runLintTestPorts` and the rest). That repeats about 100 lines of test code, and a shared helper
  package would be surface for test code alone. Every other file stays: the tree guards, `agentsetup_test.go`,
  `ledger_test.go`, `docker_test.go` and `imagepin_test.go` (inventory §2).
- **`scripts/test-pkgs.sh unit|integration|tools`** prints one package list on one line, separated by spaces.
  `tools` prints the three tool packages, and the script is the only place they are named. `unit` prints
  `go list ./...` without them. `integration` reads the build lines directly:
  `git grep --untracked -l '^[[:space:]]*//go:build integration[[:space:]]*$' -- '*_test.go' ':!**/testdata/**'`,
  then each file's directory as `./<dir>`, sorted and unique. This is the same line the testtext guard accepts, after
  it trims the line (`contract/testtext_test.go:184`). `--untracked` adds new files that git does not ignore
  (`.worktrees/` is ignored). `go list` cannot be used here. Without the tag it does not list a directory whose only
  Go files are `integration`-tagged tests, and it still exits 0, so the package would be dropped. On today's tree the
  `git grep` list is the ten packages of inventory §3. The script exits non-zero on an empty list or a failing
  command.
- **`Taskfile.yml`.** `test:unit`, `test:repeat`, `test:integration` and a new `test:tools` each read their list into
  a Task variable (`vars: PKGS: sh: scripts/test-pkgs.sh <list>`). They run plain `go test ... {{.PKGS}}`, and
  `test:repeat` and `test:integration` run `{{.CLI_ARGS | default .PKGS}}`, so `-- <pkgs>` still focuses a run. No
  wrapper sits in front of `go test` and no environment variable is set (integration-test-runner,
  "The task takes no lock and sets nothing up"). `scripts/gopkgs.sh` leaves the three test lines. A new
  `test:tools:changed` runs `scripts/tools-changed.sh`. The pinning test holds each task's `PKGS` line
  (`unit` for `test:unit` and `test:repeat`, `integration` for `test:integration`, `tools` for `test:tools`), as well
  as the command lines and `test:tools:changed`'s one command.
  Probe at the pinned Task 3.51.1, a scratch Taskfile in `/tmp`. A `sh:` variable whose command exits 3 failed the
  task (`task: Command "echo partial; exit 3" failed: exit status 3`, exit 1) and ran none of its commands. A
  variable printing `a b c` reached the command as three words. `-- ./x` replaced the default. Task 2.1 repeats the
  probe and records it.
- **`scripts/tools-changed.sh`** finds the changed files, then runs `task test:tools` or prints
  `tool tests skipped: no changed file under <trigger paths>` and exits 0. In CI on a `push` it runs everything, and
  on a `pull_request` it diffs against `git merge-base origin/$GITHUB_BASE_REF HEAD`. Outside CI it diffs against
  `git merge-base HEAD origin/main` and adds untracked files that git does not ignore. Diffs use `--no-renames`, so a
  file moved out of a trigger path still counts. Any other event, a missing base or a failing git command runs
  everything and says why. A local checkout of main with no changes skips the tool tests; a push to main runs them in
  CI.
- **Trigger paths:** `scripts/`, `internal/harness/`, `.github/`, `Taskfile.yml`, `go.mod`, `go.sum`,
  `docs/admission-ledger.yaml` (portstatus copies it), and `service/base.go`, `service/service_manager.go` and
  `service/component_manager.go` (also copied by portstatus, `portstatus_test.go:26-31`). Left out, with reasons:
  `openspec/` and `package.json` (specqueue plants its own tree and a stand-in `openspec` CLI);
  `AGENTS.md`, `.agents/` and `.claude/` (their parity test stays in `contract`); and product Go code. toolcheck,
  mutcheck and pindiff import no package of this module (task 3.1 checks this with `go list -deps -test`), so only
  `go.mod`, `go.sum` and their own files can break their build.
- **CI.** The verify job's checkout gets `fetch-depth: 0`, so `origin/<base>` exists. That is one line, it is pinned
  by `TestCIWorkflowPinned`, and nothing else in CI changes.
- **`scripts/verify.sh`:** `test:tools:changed` runs after `test:unit`, and `test:repeat` stays last.
- **Pins and their failure tests.** `TestUnitInvocationsPinned` gets the new command lines, and it parses each
  test task's `vars` as well as its `cmds`. Its new sensitivity cases:
  - `test:unit` reads the `tools` list;
  - `test:repeat` has no `PKGS` and defaults to `./...`;
  - `test:integration` reads the `unit` list;
  - `test:tools` reads the `unit` list;
  - `test:tools:changed` runs something else;
  - the tool step is missing from verify;
  - the `test:tools` command drifts.

  `TestCIWorkflowPinned` gets one case: `fetch-depth: 0` dropped.

  `TestTestPkgsLists` works on planted git modules only. A directory whose only Go file is an `integration`-tagged
  test is listed. An untracked tagged file is listed. A `testdata` directory and a package with only untagged tests
  are not. A tree with no tagged file makes the script exit non-zero. The `unit` list leaves out the three tool
  packages and keeps a planted `internal/harness/contract`. The test does not compare against the real tree's list:
  that list and any oracle for it read the same build lines, and an exact check in a gated package would fail first
  on the next scripts pull request rather than on the product pull request that changed the tree.

  `TestToolsChanged` runs `scripts/tools-changed.sh` in planted git repositories, with a stand-in `task` on `PATH`
  that records whether it ran. Its local cases remove `GITHUB_ACTIONS`, `GITHUB_EVENT_NAME` and `GITHUB_BASE_REF` from
  the environment, as `mergecheck_test.go:154-158` does. Its cases are a script change, docs and product code only,
  an untracked file under a trigger path, an uncommitted `go.mod` edit, a rename out of `scripts/`, no
  `origin/main`, a CI push, a CI pull request with docs only, and an unknown event.

  `toolcheck/doc.go` states what each test reads: the real-tree files under trigger paths and nothing else.

## Capabilities

### Modified Capabilities

- `merge-gate`: "Varied and repeated unit runs" and "Verify runs both unit steps, the repeat step last" change.
  Added: "Tool tests and repository guards are separate packages", "Each test task reads its own package list",
  "Tool tests run when their files change" and "How the changed files are found".
- `integration-test-runner`: "Integration test command" changes. Added: "Only packages with integration tests".

The other "run by `task test:unit`" sentences in inventory §1 (harness-boundaries `:751` and `:862`,
graph-transport-boundary `:36` and `:91`, metric-registry `:90`) stay true: their tests stay in `contract`.

## Non-goals

- No build tags (AGENTS.md allows only `integration`), no test cache, no new CI job, and no change to
  `test:integration`'s flags, to `ledger:check`, or to `scripts/cover-check.sh` (contract, mutcheck and pindiff are
  not coverage targets; inventory §3).
- No test is rewritten or dropped. Files move, and the only new tests are the pins and gating tests above.
- A changed set that misses a dependency (a product file a tool test starts reading later, outside the list) is
  caught only in review and by the next push to main, where everything runs. The list and the files each tool test
  reads go into the `toolcheck` doc comment, and the review checks them.

## Impact

- Code: `internal/harness/contract/` (files leave), the new `internal/harness/toolcheck/`, `scripts/test-pkgs.sh`,
  `scripts/tools-changed.sh`, `scripts/verify.sh`, `Taskfile.yml` and `.github/workflows/ci.yml`.
- Docs and rules: `AGENTS.md` and `.agents/rules-index.md` (the rows in task 5.1 plus one new row, keeping the 1-to-1
  rows), `docs/testing.md`, `docs/test-infrastructure.md`, `docs/repository-map.md`, the preflight skill, and the
  `test:integration` row of `docs/admission-ledger.yaml`.
- Retires #416 (`Closes #416`). No overlap with draft #413 or #414 in gated files. #413 edits
  `service/component_manager.go`, so its next CI run will run the tool tests. This lands first (owner).
