# Tasks: carry-check

Each task names the outcome and the gate that proves it. An unchecked task that says "hold" is one `task spec:queue`
reports as blocked until the named review or ruling exists. "This pull request" is PR #54; evidence is recorded
there as a comment. Every outcome below is reached on the branch, before this pull request merges. (D) is the
developer, (W) the technical writer.

"Written first" means the test is run and seen to fail for the stated reason before the code that makes it pass,
and that output is recorded on this pull request. The tests run offline against a local git repository that stands
in for SemStreams; their git calls ignore the user's git configuration; their expected texts are written out in the
test (`design.md`, "Tests").

## 1. Design

- [x] 1.1 Independent review of `inventory.md`, `design.md` and the `harness-boundaries` delta: DESIGN PASS in
      round 2 of at most three, recorded on this pull request (comment 5952034096) with the reviewed and committed
      checksums.
- [x] 1.2 Owner acceptance of the reviewed design: #52 comment 5952476906. Design question Q1 (the fetch inside
      `task verify`) is answered: fetch, #52 comment 5951926749.

## 2. The comparison

- [x] 2.1 (D) `TestRewriteModulePath` and `TestRewriteMovedPackage`, written first, show the module path rewritten
      in an import and a comment and left alone in a string literal; a package path in a doc link ending at `]`; a
      moved package's import path taken from a `carry` or `adapt` directory entry at the same pin; and re-sorted
      imports equal. Gate: `task test:unit`.
- [x] 2.2 (D) `TestCoveredFiles`, written first, shows `README.md` and a sub-directory left out, `testdata` taken
      in, a note after the destination path ignored, and a file-against-directory entry reported as not compared.
      Gate: `task test:unit`.
- [x] 2.3 (D) `TestDiffOutput` and `TestDiffExitCodes`, written first, show the outputs and exit codes of design D4:
      nothing for an unchanged file, a unified diff for a changed line, the two one-sided lines, the per-entry lines,
      exit 2 for an unknown argument and for a named entry not compared. Gate: `task test:unit`.

## 3. The check

- [ ] 3.1 (D) `TestCheckSensitivity`, written first, plants each violation in a temporary tree and requires exit 1
      with the entry, the file and the kind named: a one-line edit in a non-test file, in a test file and in a
      `testdata` file; a file removed; a file added; a changed string literal that holds the module path; an import
      redirected to the destination of a `defer-exclude` entry; a destination that does not exist, one that starts
      with `/` and one with a `..` segment. `TestCheckScope` shows a carried entry passing, an `adapt` entry never
      failed, and a ledger with no `carry` entry passing with an unreachable remote. Gate: `task test:unit`.
- [ ] 3.2 (D) The experiment of `docs/testing.md`, "Show that the test can fail", is run once on the comparison:
      with test files left out of the covered files, `TestCheckSensitivity` fails on its test-file case. The change
      and the three outputs are recorded on this pull request.
- [ ] 3.3 (D) `TestCheckPinUnreadable`, written first, shows exit 2 and the message of design D5 for a remote that
      refuses, and for a remote that never answers with the fetch bound set under a second. The bound is one for
      the whole run, not one per SHA. Gate: `task test:unit`; the test's time in `task test:repeat` is recorded.
- [ ] 3.4 (D) `TestCommandExitStatus`, written first, runs the program as a separate process, the way
      `task ledger:check` runs it, in a temporary tree with a planted `carry` violation and a local repository as
      the remote: it requires a non-zero exit and the violation's message, and exit 0 for the same tree with no
      `carry` entry. Shown to fail with the program's exit status dropped. Gate: `task test:unit`.
- [ ] 3.5 (D) `task ledger:check` runs the schema tests and then the program; `task ledger:diff` exists and its
      description says it reads GitHub. `TestLedgerCheckWiring` and its `Sensitivity` pair in
      `internal/harness/contract`, written first, fail when `Taskfile.yml` drops the program's command or adds
      anything that discards its exit status. Gates: `task ledger:check` and `task test:unit`.
- [ ] 3.6 (D) The new package passes `task test:repeat -- ./internal/harness/pindiff`, `task lint`, `task vuln` and
      `task tidy:check`, the last with no new module in `go.mod`.

## 4. Evidence against the real pin

- [ ] 4.1 (D) `task ledger:diff` with no argument, run on this branch against the twelve rows on `main`: standard
      output's SHA-256 and the per-entry lines are recorded on this pull request.
- [ ] 4.2 (D) In a scratch copy of this branch that is not committed, `pkg/timestamp` is copied from the pin with the
      rewrite and given a `carry` row. `task ledger:check` passes; with one line of `timestamp.go` edited it fails
      naming the entry and the file; `task ledger:diff -- pkg/timestamp` prints that line. The three outputs are
      recorded on this pull request, with the wall time of the passing run: it is the one run here that fetches the
      pin, so it is the measure of what the check adds to `task verify`.

## 5. Documents

- [ ] 5.1 (W) `docs/provenance.md`: rule 5 names `task ledger:check` as what holds a `carry` row to the pin; a
      paragraph says how a port is reviewed with `task ledger:diff` and what the verdict records (design D7); the
      Status paragraph is current. Gate: `task docs:check`.
- [ ] 5.2 (W) The header comment of `docs/admission-ledger.yaml` says a `carry` row is compared with the pin, and
      that `destination` starts with a path inside the repository and may carry a note after it. Gate: `task
      ledger:check`.
- [ ] 5.3 (W) Written on top of PR #56 once it has merged: `docs/testing.md` (structural guards and helper
      packages), `docs/repository-map.md` and the `AGENTS.md` rule index (the ledger row, and `task ledger:diff` in
      the command list) name the check and say it fetches the pin. Gate: `task docs:check`.

## 6. Review and landing

- [ ] 6.1 Hold: independent change review. The verdict on the full diff is a pass recorded on this pull request with
      the reviewed commit.
- [ ] 6.2 (D) `task verify` passes on the branch's last commit before the archive commit, with the branch up to
      date with `main`; its step timings are recorded on this pull request, and the body of this pull request
      carries `implemented-by:`.
- [ ] 6.3 (W) The change is archived and `openspec/specs/harness-boundaries/spec.md` synced as the last content
      commit; `task spec:check` passes and `task spec:queue` shows no open hold.
