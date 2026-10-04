# Tasks: mutation-check

Each task names the outcome and the gate that proves it. An unticked task whose first words are `Hold:` waits for
what it names, and `task spec:queue` reports it as blocked until then. "This pull request" is PR #82; evidence is
recorded there as a comment. Every outcome below is reached on the branch, in or before the archive commit. (D) is
the developer, (W) the technical writer.

"Written first" means the test is run and seen to fail for the stated reason before the code that makes it pass, and
that output is recorded on this pull request. Expected verdicts, exits and words are written out in each test, from
the scenarios of the `mutation-check` delta, never from running the program.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 82` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS` in round 2 of at most three, recorded on this
      pull request (comment 5981915103). The file reviewed and the file committed are the same, sha256
      `774646d7…62ed9`.
- [ ] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the `mutation-check` delta:
      `DESIGN REVIEW PASS` in at most three rounds, recorded on this pull request, with the reviewed and the
      committed checksums.
- [ ] 1.3 Hold: the owner's ruling on #79, questions Q1 to Q4 of `design.md`. The ruling is quoted under "Owner's
      rulings" with its comment link, and the design, the delta and these tasks follow it.

## 2. The program and its tests

- [ ] 2.1 (D) The classifier. `go test -json` output captured from go1.26.6 for each per-run case of `design.md`, D5,
      is kept under `internal/harness/mutcheck/testdata/`. A test enumerates every combination of the per-run inputs
      and compares the program's reading with a reference table written in the test from the spec's rules. One named
      example for each aggregate rule: baselines, agreement, early stops, after-run, fingerprint, reach. Written
      first. Gate: `task test:unit`.
- [ ] 2.2 (D) The refusals: one case for each scenario of "The command and its inputs", including the untracked and
      the tracked file under `testdata/rapid/`, and one each for `-seed 0`, `GOFLAGS` that already sets `-overlay`, a
      mutant inside the module and a mutant identical to its target. Each case shows that no run started. Written
      first. Gate: `task test:unit`.
- [ ] 2.3 (D) End to end with the real toolchain, on planted modules in temporary directories, `-runs 1` where
      repetition is not under test: detection; survivor reached; survivor not reached (invalid); a wrong change that
      does not build (invalid); a `go build` started by the test sees the wrong change; every run's environment has
      `RAPID_SEED` and `RAPID_NOFAILFILE=true`; a test that writes into its package directory makes the check
      inconclusive; and the planted module, made read-only, has the same content hashes afterwards. Written first.
      Gate: `task test:unit`.
- [ ] 2.4 (D) SIGTERM to the program while the named test waits on a channel: no process it started is running when
      it exits, it exits non-zero, it prints no verdict, and the planted module is unchanged. Written first. Gate:
      `task test:unit`.
- [ ] 2.5 (D) The report and the exit status: each scenario of "Report and exit status", and the report's fields as
      that requirement lists them. Written first. Gate: `task test:unit`.
- [ ] 2.6 (D) `Taskfile.yml` gains `mutate:check`, which runs `go run ./internal/harness/mutcheck {{.CLI_ARGS}}`; its
      description says it is run by hand, writes nothing in the repository and fails unless the verdict is
      detection. `task --list` shows it. Gate: `task verify`.
- [ ] 2.7 The time the new tests add to the `test:unit` and `test:repeat` steps, from CI runs on `main` and on this
      branch, against the budget of 60 s in `design.md`, "Declared costs". The numbers are recorded on this pull
      request and in "Declared costs". Over budget, the end-to-end set is cut as that section says, before this task
      is ticked.
- [ ] 2.8 Hold: PR #48 merged into `main`, which brings Rapid into `go.mod`. (D) A planted Rapid property in a
      temporary module that reads Rapid from the module cache with `GOPROXY=off`: the scenarios "A property test"
      and "A property test whose seed misses the wrong change", with no file under `testdata/rapid/` afterwards.
      Written first. Gate: `task test:unit`.

## 3. Shown able to fail

- [ ] 3.1 (D) The experiment of `docs/testing.md`, "Show that the test can fail", for each wrong change to the
      program listed in `design.md`, D11, "Shown able to fail". For each, the change and the baseline, wrong-change
      and after-runs are recorded on this pull request, with the program's own report where it can check its own
      package. A wrong change the tests let through is reported as a survivor and closed with a new case, or listed
      under what is not covered.
- [ ] 3.2 (D) The program run on the three recorded cases whose code is on `main`, R01 to R03 of the trial (PR #59's
      wrong changes to `internal/harness/probe/await.go`), with the assertion locations the trial used. The expected
      verdict for each is detection, as the trial found. The three reports are recorded on this pull request.

## 4. Documents

- [ ] 4.1 (W) `.agents/skills/semengine-preflight/SKILL.md`: the command table gains `task mutate:check`. Gate:
      `task docs:check`.
- [ ] 4.2 Hold: PR #48 merged into `main`. (W) `.agents/contracts/semengine-developer.md` and `semengine-reviewer.md`:
      the mutation passages name `task mutate:check` for a Go source target, keep the `cp` procedure for the rest, and
      give the checksum as `shasum -a 256`. Gate: `task docs:check`.
- [ ] 4.3 Hold: PR #48 merged into `main`, then PR #73. (W) `docs/testing.md`, "Show that the test can fail": the
      command and what it checks, the manual procedure for what it refuses, the outcome invalid, the sentence on
      bounded assertions, equivalence as a reviewer's assessment, and SHA-256 for the restore check (`design.md`,
      D5 and D12). Gate: `task docs:check`.
- [ ] 4.4 Hold: PR #48 merged into `main`, then PR #73. (W) `docs/repository-map.md`: the program's row and the
      `mutation-check` spec. Gate: `task docs:check`.
- [ ] 4.5 Hold: the owner's ruling on #79, question Q1, then PR #48 and PR #73 merged into `main`. (W) `AGENTS.md`:
      the Commands block gains `task mutate:check`, and the row on mutation outcomes says what the ruling says.
      Gate: `task docs:check`.

## 5. Review

- [ ] 5.1 Codex's implementation review of this pull request: a review record that approves, at the head that
      carries every code commit, recorded on this pull request. A later content commit gets a re-review.
- [ ] 5.2 The description has one line `implemented-by:` that names `claude` and one line `reviewed-by:` that names
      `codex`, written once Codex's newest review record approves.

## 6. In the archive commit

- [ ] 6.1 `openspec archive mutation-check` creates `openspec/specs/mutation-check/spec.md`, in the same commit as
      the spec sync and the ticks this commit makes. Gate: `task spec:check` on that commit.
