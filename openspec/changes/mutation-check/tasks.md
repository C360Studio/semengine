# Tasks: mutation-check

Each task names the outcome and the gate that proves it. An unticked task whose first words are `Hold:` waits for
what it names, and `task spec:queue` reports it as blocked until then. "This pull request" is PR #82; evidence is
recorded there as a comment. Every outcome below is reached on the branch, in or before the archive commit. (D) is
the developer, (W) the technical writer. "The owner's acceptance" is task 1.3: no implementation starts before it.

"Written first" means the test is run and seen to fail for the stated reason before the code that makes it pass, and
that output is recorded on this pull request. Expected verdicts, exits and words are written out in each test, from
the scenarios of the `mutation-check` delta, never from running the program.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 82` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS` in round 2 of at most three, recorded on this
      pull request (comment 5981915103). The file reviewed and the file committed are the same, sha256
      `774646d7…62ed9`.
- [x] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the `mutation-check` delta:
      `DESIGN REVIEW PASS` in round 3 of three, recorded on this pull request (comment 5982517389) with the reviewed
      checksums. Round 1 asked for changes (`design-review-r1.md`, sha256 `aea028aa…94ee`); round 2 passed with two
      MEDIUM findings and five NITs (`design-review-r2.md`, sha256 `48703ca5…c9ac`); round 3 passed at `a960bce` with
      one NIT (`design-review-r3.md`, sha256 `e718b18e…28c6`), fixed in `daca356` (one path in `design.md`; the word
      diff is in the record).
- [x] 1.3 The owner's ruling on #79, questions Q1 to Q5 of `design.md`: accepted, all five as recommended, verbatim
      "Accept, agree with all five" (comment 5982900883). It is quoted under "Owner's rulings" in `design.md`; the
      design, the delta and these tasks already follow the recommendations.

## 2. The program and its tests

- [x] 2.1 (D) The classifier: `go test -json` output captured from go1.26.6 for
      each per-run case of `design.md`, D5, including a race report, a log line at the expected location, a run killed
      by a signal and a build failure, kept under `internal/harness/mutcheck/testdata/`. A test enumerates every
      combination of the per-run inputs and compares the program's reading with a reference table written in the test
      from the spec's rules. One named example for each aggregate rule. Written first. Gate: `task test:unit`.
      Evidence: comment 5983603895.
- [x] 2.2 (D) Regions and reach, on recorded line diffs and coverage profiles: each
      scenario of "The changed region and reach", with the two planted cases of `design.md`, D11, item 2 (line numbers
      of the target against the mutant's; an insertion after an `if` line whose branch did not run). Written first.
      Gate: `task test:unit`.
      Evidence: comment 5983604024.
- [x] 2.3 (D) The refusals: one case for each scenario of "The command and its
      inputs", and one each for `-seed 0`, `GOFLAGS` that sets `-overlay`, a mutant inside the module and a mutant
      identical to its target. Each case shows that no run started. Written first. Gate: `task test:unit`.
      Evidence: comment 5983604171.
- [x] 2.4 (D) With a stand-in `go` first on `PATH`: a run that does not end by twice
      its timeout (`-timeout 200ms`) is stopped with its processes and is inconclusive; a test that writes into its
      package directory makes the check inconclusive. Written first. Gate: `task test:unit`.
      Evidence: comment 5983604328.
- [x] 2.5 (D) End to end with the real toolchain, on planted modules that are git
      repositories with a copy of `scripts/tree-state.sh`, `-runs 1`: a detection through a command the test builds, in
      a module whose every directory is read-only, with a caller's `-tags` set with `go env -w` in a temporary `GOENV`
      file and the seed in every run's environment, and the content hashes unchanged; and a deletion survivor that was
      reached. Written first. Gate: `task test:unit`.
      Evidence: comment 5983604483.
- [x] 2.6 (D) SIGTERM to the built program while its stand-in `go` waits, having
      started a helper that writes its own pid and its parent's, as `TestFetchLeavesNoProcess`'s helper does: neither
      process is running when the program exits, it exits non-zero, it prints no verdict, and the planted module is
      unchanged. The program is built with the inherited environment and run with `-overlay` removed from `GOFLAGS`.
      Written first. Gate: `task test:unit`.
      A CI failure in the setup (a detached `git maintenance`) was fixed in `76aa263`; see the comment.
      Evidence: comment 5983636217.
- [x] 2.7 (D) The report and the exit status: each scenario of "Report and exit
      status", and the report's fields as that requirement lists them, the hunks, each run's `GOFLAGS` and the
      survivor's seed among them. Written first. Gate: `task test:unit`.
      Evidence: comment 5983604625.
- [x] 2.8 (D) `Taskfile.yml` gains `mutate:check`, which runs
      `go run ./internal/harness/mutcheck {{.CLI_ARGS}}`; its description says it is run by hand, writes nothing in
      the repository and fails unless the verdict is detection. `task --list` shows it. Gate: `task verify`.
      Evidence: comment 5983604783.
- [x] 2.9 The package's test time: the sum of the two
      `ok .../internal/harness/mutcheck <time>` lines that the `test:unit` and `test:repeat` steps print in a CI verify
      log of this branch, against the budget of 60 s in `design.md`, "Declared costs". The numbers and the run are
      recorded on this pull request and in "Declared costs". Over budget, the survivor case of 2.5 moves to a recorded
      profile and a stand-in `go` before this task is ticked.
      CI run 37228603260 at `76aa263`: 6.727 s + 24.880 s = 31.607 s, under the 60 s budget.
      Evidence: comment 5983636400.
- [x] 2.10 (D) (PR #48 merged as `deaafd4`, which brings Rapid into `go.mod`.) A
      planted Rapid property in a temporary module that reads Rapid from the module cache with `GOPROXY=off`: the
      scenarios "A property test" and "A property test whose seed misses the wrong change", with no file under
      `testdata/rapid/` afterwards. Written first. Gate: `task test:unit`.
      Seeds 1 (finds the wrong change) and 3 (misses it), calibrated on the planted module. Both cases passed against
      the code as it stood, so they are named in the record, not seen to fail. V10 (`RAPID_NOFAILFILE` not set):
      detection. With these cases the package went over its budget (CI run 37477518090 at `85b6984`: 66.656 s), so
      the survivor case of 2.5 moved to a recorded profile and a stand-in `go` (`6ccceea`). CI run 37480956128 at
      `6ccceea`: 9.945 s + 46.406 s = 56.351 s, under the 60 s budget.
      Evidence: comment 6018825761.
- [x] 2.11 (D) The corrections of implementation review round 1 (`impl-review-r1.md`, sha256 `c3edf5ca…4659`;
      `design.md`, "Implementation review round 1 corrections"), each with a test written first and seen to fail against
      the code at `8bee3ea`, or, where a case already exists, named in the record: (a) an insertion is decided by its
      own statement list: recorded cases for the scenarios "An insertion at the end of an if-branch whose else ran" and
      "An insertion at the end of a switch case whose next case ran"; (b) invalid needs every baseline run and the
      after-run to pass: the scenario "The build breaks after the baselines"; (c) the four added refusals, one case
      each; (d) every child's `PWD` is the resolved root: the scenario "A repository reached through a symbolic link",
      with the environment's `PWD` naming a link to the planted module; (e) the hunk diff passes
      `--inter-hunk-context=0`: the scenario "Hunks are not merged", with a temporary global git configuration; (f) a
      run killed before its test starts names the signal, as "Killed by a signal" says; (g) the report's bound line and
      the messages say "process group", and the tests set `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`.
      Gate: `task test:unit`.
      Tests at `64307fe` (11 failing against the code of `8bee3ea`), fixes at `f30d841`; (c)'s malformed `-expect`
      and empty `-expect-text` cases could not fail first and are shown able to fail in 3.3 (V04, V05).
      Evidence: comment 5984462581.
- [x] 2.12 (D) The corrections of implementation review round 2 (`impl-review-r2.md`, sha256 `2fdc930b…769d`;
      `design.md`, "Implementation review round 2 corrections"), each with a test written first and seen to fail against
      the code at `e936326`: (a) an insertion whose place lies inside a statement of its list is not measurable:
      recorded cases for the scenarios "An insertion inside a multi-line condition" and "An insertion right after a
      label", the second with the label reached by `goto`; (b) a `TMPDIR` that does not exist is refused: the scenario
      "Temporary files in a directory that does not exist", or, if a case already exists, named in the record. Gate:
      `task test:unit`.
      Tests at `41b73da` (both reach cases failing against the code of `e936326`), fix at `10b9746`; (b)'s case
      could not fail first and is shown able to fail in 3.4 (V07).
      Evidence: comment 5984734456.
- [x] 2.13 (D) The correction of implementation review round 3 (`impl-review-r3.md`, sha256 `7ee140a5…ef6e`; the owner's
      ruling "fix now", #79, comment 5994738791; `design.md`, "Implementation review round 3 correction"): an insertion
      after a labeled statement that is the last of its list is not measurable, while the first-after decider keeps a
      labeled statement. A recorded case for the scenario "An insertion after a labeled last statement", with the label
      reached by `goto`, written first and seen to fail against the code at `390ddcf`; the existing case for a place
      right before a label still passes. Gate: `task test:unit`.
      Test at `efbd685` (failing against the code of `390ddcf`), fix at `4a5feb2`; no case for a place right
      before a label existed, so one was added at `efbd685`, passing before and after the fix.
      Evidence: comment 5994937391.
- [x] 2.14 (D) The corrections of Codex's implementation review of record (PR #82, comment 5995872776: APPROVE at
      `35fa1d9` with two MEDIUM findings). (a) The report prints in full, as a note, every further line of the named
      test and its subtests that begins with a location, and the reading does not change: a report assertion on the
      `log-then-fail` recording, written first and seen to fail against the code at `29f8abe`, and one mutation check
      through `task mutate:check` with a wrong change that drops those lines again. (b) `design.md` gains the
      per-ruling conformance table (`.agents/contracts/semengine-reviewer.md:113-117`) for Q1 to Q5, the sequencing
      and the labeled-last-statement ruling, each with its `file:line` evidence or its held task. Gate:
      `task test:unit`.
      Test at `714197f` (failing against the code of `29f8abe`), fix at `7630ad9`, V09 a detection; the table at `a3d27f7`.
      Evidence: comment 5996159559.

## 3. Shown able to fail

- [x] 3.1 (D) The experiment of `docs/testing.md`, "Show that the test can fail",
      for each wrong change to the program listed in `design.md`, D11, "Shown able to fail". It starts with one check
      of the program's own package by the command, recording whether that works; each wrong change the command cannot
      check is checked by hand as the page describes. For each, the change and the baseline, wrong-change and
      after-runs are recorded on this pull request. A wrong change the tests let through is reported as a survivor and
      closed with a new case, or listed under what is not covered.
      18 checks, 18 detections, 0 survivors, 0 inconclusive, 0 invalid; reports in comments 5983605892
      and 5983606064. What is not covered is listed in the comment.
      Evidence: comment 5983606233.
- [x] 3.2 (D) The program run on the three recorded cases whose code is on `main`,
      R01 to R03 of the trial (PR #59's wrong changes to `internal/harness/probe/await.go`), with the assertion
      locations the trial used. The expected verdict for each is detection, as the trial found. The three reports are
      recorded on this pull request.
      R01, R02 and R03: detection, as the trial found.
      Evidence: comment 5983606427.
- [x] 3.3 (D) The experiment of `docs/testing.md`, "Show that the test can fail", for the wrong changes `design.md`,
      D11, adds in revision 4: an insertion decided by a sibling branch's block, hunks merged by a caller's git setting,
      and a child's `PWD` left as the caller's. Each with its change and its baseline, wrong-change and after-runs
      recorded on this pull request; a survivor is reported as such.
      V01 to V03: 3 detections, 0 survivors, 0 inconclusive, 0 invalid; V04 and V05 (extra): 2 detections.
      Evidence: comment 5984462715.
- [x] 3.4 (D) The experiment of `docs/testing.md`, "Show that the test can fail", for the wrong change that would undo
      2.12 (a): an insertion inside a statement decided by a neighbouring statement's block, as `design.md`, D11, lists
      it. Its change and its baseline, wrong-change and after-runs are recorded on this pull request; a survivor is
      reported as such.
      V06: 1 detection, 0 survivors, 0 inconclusive, 0 invalid; V07 (extra): 1 detection.
      Evidence: comment 5984734642.
- [x] 3.5 (D) The experiment of `docs/testing.md`, "Show that the test can fail", for the wrong change that would undo
      2.13: an insertion after a labeled last statement decided by the label's block, as `design.md`, D11, lists it. Its
      change and its baseline, wrong-change and after-runs are recorded on this pull request; a survivor is reported as
      such.
      V08: 1 detection; a first attempt that did not build: 1 invalid.
      Evidence: comment 5994937779.

## 4. Documents

- [x] 4.1 (W) `.agents/skills/semengine-preflight/SKILL.md`: the command table
      gains `task mutate:check`. Gate: `task docs:check`. Done in `87329fe`; evidence comment 5995720079.
- [x] 4.2 (W) (PR #48 merged as `deaafd4`.)
      `.agents/contracts/semengine-developer.md` and `semengine-reviewer.md`: the mutation passages name
      `task mutate:check` for a Go source target, keep the `cp` procedure for the rest, and give the checksum as
      `shasum -a 256`. Gate: `task docs:check`. Done in `36f1c35`; evidence comment 6019183479.
- [x] 4.3 (W) (PR #48 merged as `deaafd4`; PR #73 as `955fc83`.) `docs/testing.md`, "Show
      that the test can fail": the command and what it checks, the manual procedure for what it refuses, the outcome
      invalid and how reach is judged, the sentence on bounded assertions, the race detector as the owner rules on Q5,
      equivalence as a reviewer's assessment, and SHA-256 for the restore check (`design.md`, D5, D12, D13 and D14).
      Gate: `task docs:check`. Done in `e1961ab`.
- [x] 4.4 (W) (PR #48 merged as `deaafd4`; PR #73 as `955fc83`.) `docs/repository-map.md`:
      the program's row and the `mutation-check` spec. Gate: `task docs:check`. Done in `1492c32`; the spec is listed
      under "Not yet present" until the archive (6.1) moves it to `openspec/specs/`.
- [x] 4.5 (W) (PR #48 merged as `deaafd4`; PR #73 as `955fc83`.) `AGENTS.md`:
      the Commands block gains `task mutate:check`, and the row on mutation outcomes says what the ruling says. Gate:
      `task docs:check`. Done in `c4ed889`.

## 5. Review

- [ ] 5.1 Codex's implementation review of this pull request: a review record that approves, at the head that
      carries every code commit, recorded on this pull request. A later content commit gets a re-review.
- [ ] 5.2 The description has one line `implemented-by:` that names `claude` and one line `reviewed-by:` that names
      `codex`, written once Codex's newest review record approves.

## 6. In the archive commit

- [ ] 6.1 `openspec archive mutation-check` creates `openspec/specs/mutation-check/spec.md`, in the same commit as
      the spec sync and the ticks this commit makes. Gate: `task spec:check` on that commit.
