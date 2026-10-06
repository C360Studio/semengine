# Design: mutation-check

Status: **revision 6: the accepted design, corrected after implementation review rounds 1 to 3.** Round 3
(`impl-review-r3.md`, sha256 `7ee140a5c236bdca736e31644d1c3b817074fffd73c25668513bf0c66351ef6e`, at code head
`390ddcf`) approved with one MEDIUM finding open, which went to the owner under the three-round rule; the owner ruled
"fix now" (#79, comment 5994738791). "Implementation review round 3 correction" below says what changed. Revision 5
was the accepted design, corrected after implementation review rounds 1 and 2. Round 2
(`impl-review-r2.md`, sha256 `2fdc930b6cd61035fefa89ffcdbf721aa830a88bbbffa726a82e0fca729b769d`, at code head
`e936326`) found one more place where the reach rule decides by the wrong statement, and two NITs; "Implementation
review round 2 corrections" below says what changed. The owner accepted
revision 3 and answered Q1 to Q5 as recommended ("Owner's rulings"). Implementation review round 1
(`impl-review-r1.md`, sha256 `c3edf5cab729cc09e717c01acb16118d974481da8efb23a0b7b595ae843e4659`, at code head
`8bee3ea`) found the delta wrong or silent in six places; "Implementation review round 1 corrections" below says what
changed. No decision changed. The history of the pre-owner rounds follows. Revision 2 (commit
`9773da9`) got DESIGN REVIEW PASS in round 2 (`design-review-r2.md`, sha256
`48703ca562121a5b9f2c3ef54b0e82b4f83f1e0b9c898cea678745ff6978c9ac`), with two MEDIUM findings and five NITs that change
no decision; "Round 2 review findings" below says where each is answered. Revision 1 (commit `ba7f440`) got
DESIGN CHANGES REQUESTED in round 1 (`design-review-r1.md`, sha256
`aea028aac0b394e5bcd605ee5e06fa3d7992620841619f70abdee01bc63e94ee`): one BLOCKING finding, one HIGH, eight MEDIUM and
three NIT. "Round 1 review findings" below says where each is answered. The design rests on `inventory.md` in this
directory: revision 2, which has `INVENTORY PASS` in round 2 (PR #82, comment 5981915103), sha256
`774646d7ab3704c2a3498148fe414bd9149d388f6ed1aa1238d6d97965a62ed9`. That file is the accepted inventory and is not
repeated here. "Inventory 2c" means its section 2c, and "Q9" means its open question 9. Round 2's four NITs on the
inventory are answered under "Inventory errata"; the inventory's text is unchanged. New decisions in this revision are
numbered D13 to D15, so that the review's references to D1-D12 stay valid.

## Context

Issue #79 asks for `task mutate:check -- <package> <TestName> <mutant>`. It would run the experiment of
`docs/testing.md`, "Show that the test can fail", and classify the outcome as detection, survivor or inconclusive by
command instead of by hand. #79 names `scripts/mutation-check.sh`, a fixture test, a `Taskfile.yml` entry, a change
to the `AGENTS.md` row for mutation outcomes, and an edit to `docs/testing.md`.

The owner's words since the inventory, verbatim:

- On the trial, before it ran: "my answer depends on how realistic it is to expect a command to return solid results
  without a bunch of noise", then "yes go that way, run the trial first". The sequencing agreed with him is #79
  comment 5981621215. The program, its tests and the `Taskfile.yml` entry proceed now. The `AGENTS.md` row and the
  `docs/testing.md` section wait until #48 and then #73 merge. Seed replay waits for #48.
- On overlay: "that go test -overlay technique looks very promising. please give me a summaary of how that goes as
  it seems like a huge win for us around mutation testing". The summary is the next section.
- The scope ruling that let the inventory read the pin's testing page: #22 comment 5981684453.

His standing rule for the work is "Write the minimum code that solves the stated problem".

Terms used below.

- A *wrong change* (a *mutant*) is the one deliberate, plausible change to the implementation that the test is meant
  to catch (`docs/testing.md:118-119`). The *target* is the file it changes.
- An *overlay* is Go's `-overlay` build option. It takes a JSON file mapping a file's path to another file's path,
  and builds as if the second file's content were at the first path. Nothing on disk changes.
- A *run* is one `go test` of the named test. A *baseline run* uses the unchanged tree, a *mutant run* uses the wrong
  change, and the *after-run* uses the unchanged tree again once the mutant runs are over.
- A *verdict* is the command's one-word result: detection, survivor, invalid or inconclusive. Section D5 defines
  invalid.
- The tree's *fingerprint* is what `scripts/tree-state.sh` prints: a hash of HEAD, the status listing, the tracked
  diff and every untracked, unignored file's content (inventory 2c).

## How overlay went: a summary for the owner

The trial ran `go test -overlay` on 12 mutants recorded on PR #59 and PR #48, and on 9 planted controls.

- **No file was edited, in any case.** All 12 recorded cases left the tree byte-identical, checked by a SHA-256 of
  the whole tree before and after (`trial/cases/R*/out/report.txt`, line `tree before=... identical=true`; recounted
  for this design).
- **The results did not wobble.** Each recorded case ran its mutant three times, and the three runs gave the same
  answer in all 12 cases: 36 runs, no disagreement. Every baseline run and every after-run passed (recounted from the
  same logs).
- **It reached what the tests build, including in a child process.** Two tests that re-run their own test binary as a
  child process (R09, R11) saw the wrong change. This design measured two more cases. A file embedded with
  `//go:embed` sees the overlay, which refutes the trial's guess that it would not. A `go build` that the test starts
  itself does not see an overlay given as the `-overlay` flag, but does see it when the overlay is given through
  `GOFLAGS=-overlay=<file>`. "Premises" P3 and P4 have both measurements. Nothing in this repository embeds a file
  today, so the command does not use the first fact; it matters for Q3.
- **It does not reach a file the test reads while it runs.** Go's own documentation says so ("overlays will not
  appear when binaries and tests are run through go run and go test"), and it was measured (inventory 2a, 11). In
  this repository that means a shell script tested by a Go test, a YAML file, test data, and source text that a
  contract test scans. Of the mutants recorded so far, the seven in change `review-gate-check` were all to
  `scripts/merge-check.sh` (its `design.md:276-281`). Overlay cannot reach those.
- **Go drops the overlay for a file it builds with coverage.** The round-1 review measured this, and this revision
  measured it again (P16): a test that fails on the wrong change passes when `-cover` or `-coverpkg` covers the
  target's package, through the flag and through `GOFLAGS` (go1.26.6 here; go1.26.6 and go1.26.4 in the review).
  Go's build hands the coverage tool the file's path on disk (`cmd/go/internal/work/exec.go:679-683` and `:2074` in
  go1.26.6), so the original is what runs.
  So the trial's "reach" runs, which checked survivors with `-overlay` plus `-coverpkg`, measured the unchanged code,
  not the wrong change, and the prototype's note that the reach run "also proves the overlay reached the build" is
  wrong. The design now measures reach on the unchanged code on purpose, and says why that answers the question for one
  wrong change (D13). Re-read that way, R04's and R05's survivors were still reached (D13 gives the blocks); the
  trial's evidence for it was not what it said.
- **Noise came from the inputs, not from overlay.** On the first try, the prototype's verdict matched the trial
  agent's reading of the page in 10 of 12 recorded cases. Both misses were inputs: R02's record names no assertion,
  and in R11 the line named was inside a helper, so Go printed another. Given the lines Go prints, all 12 matched. That
  reading is not independent, because one agent wrote both; where the page's text alone decides (R06, a whole-run
  timeout, and R08, a killed process), both pages say inconclusive, as the prototype did. Only 3 of the 12 records
  named the assertion at all. The planted controls P1-P7 got the right verdict. The Rapid control P8 got a false
  detection from a failure file an earlier run had left, until every run set `-rapid.nofailfile` (D7).

So for Go source, overlay is the way to make the wrong change: it writes nothing, it survives an interrupt or a kill
because there is nothing to restore, and in the trial it gave the same answer every time. It is not a way to check a
change to a script, and it does not work together with coverage, which is why no mutant run measures coverage and the
command refuses a `GOFLAGS` that turns coverage on (P17).

## Inventory errata

Answers to round 2's four NITs (`inventory-review-r2.md`, sha256 `52b42c4b3f413b89073b509d63ff6b0390ead67021ca1e87acd1416a56aae816`).

1. **Inventory 2g and 2c, the pin's scripts.** Three `.sh.txt` scripts are listed, and all three mutate
   `component/lifecycle_test_suite.go` (`accepted-start-order-mutant.sh.txt:3` included). `run_mutations.sh`'s
   `EXIT` trap copies the backup back without checking it (`:10-15`); its `md5` checks are on the main path (`:64`,
   `:113`). A SIGKILL to `run_mutations.sh` leaves its mutant in its disposable copy (`:4-5`), not in the tree it was
   copied from.
2. **Inventory 6, the Catalogs cell.** The search `git grep -n -i -E 'backup|\.bak'` also matches
   `internal/harness/runner/runner_test.go:114-115`, where a fake tool runs `sed -i.bak` and removes the `.bak` file
   on the next line, and the prose lines `semengine-developer.md:58` and `semengine-reviewer.md:49`. No catalog of
   in-flight mutations exists; the conclusion stands.
3. **Inventory 7, item 2.** "`git diff` shows it" holds only for a tracked target. For an untracked target, neither
   `git diff` nor `scripts/verify.sh`'s tracked-state check (`:18`) shows a mutant left by a killed run, and the
   backup is the only copy (as inventory 6, Recovery, says). This design makes no backup, because it writes nothing.
4. **Inventory 12, numbering.** There is no Q12: round 1's Q12, the ordering against #80, was merged into Q11.
   References here use the inventory's numbers as written; none refers to Q12.

## Round 1 review findings, and where each is answered

| Finding | Answer |
| --- | --- |
| BLOCKING: the reach run covers the original code | Re-measured (P16, P17). D13: reach is measured on the unchanged code by design, without an overlay, with the argument for why that shows reach for one wrong change and its limits. P14 corrected. A `GOFLAGS` with coverage is refused. The proposal no longer says "shown reached by coverage" without saying of what |
| HIGH: "the changed lines" for a deletion and for several hunks | D13 and the spec's "The changed region and reach": regions in the target's line numbers for removal, replacement and insertion; several hunks accepted and reported; scenarios for a deletion survivor, a deletion and an insertion not reached, two hunks, a declaration, and line numbers |
| MEDIUM: a log line looks like a failure line | D5 and the spec: what is matched is stated (an output line of the named test), the report prints each matched line, and the limit is under "What the command does not see" (P19) |
| MEDIUM: a race report has no rule | D14, owner question Q5; spec scenarios for a race report expected and not expected (P18) |
| MEDIUM: the caller's other `GOFLAGS` | D15 and "The runs": every run has the same flags and environment apart from the overlay and the reach run's coverage; the report prints each run's `GOFLAGS` |
| MEDIUM: the self-check meets the `GOFLAGS` refusal | D11 and D15: the program builds each child's environment from the one it is given, and its tests build the program under the outer overlay but run it with the overlay removed. The self-check claim is narrowed, and task 3.1 records which wrong changes it covered |
| MEDIUM: "every file SHALL be as it was" | The spec now says the program itself writes nothing, however it ends; the fingerprint check covers what the test or another writer changes |
| MEDIUM: tasks stopped by the ruling carry no `Hold:` | Every task that waits for the owner's acceptance now says `Hold:` (tasks.md) |
| MEDIUM: the test-time budget | "Declared costs": the real-toolchain set is cut now to three cases; the budget is the sum of the package's own `ok` times in `test:unit` and `test:repeat`, read from the verify log (task 2.9) |
| MEDIUM: "no path ends in a wrong verdict" | Corrected in "What a session has to know": four facts with the seed, and the paths that can still end in a wrong verdict, recorded as a finding |
| NIT: the `-expect` form | Any `file.go:N` that Go prints, test file or not (spec, D2) |
| NIT: undefined build allowance; no scenario for a signal; where the reach run sits | The bound is twice the timeout and has a scenario; a scenario for a run killed by a signal; the reach run comes after the after-run (D6) |
| NIT: #80's ruling words | Quoted as recorded, with "Option A" marked as the session's reading (ordering section) |
| Q1: the reviewer agrees with the design's reading | Kept; the orchestrator's split proposal and its withdrawal are recorded in Q1 |

## Round 2 review findings, and where each is answered

| Finding | Answer |
| --- | --- |
| MEDIUM 1: a `GOFLAGS` set with `go env -w` escapes the refusal, and an environment `GOFLAGS` replaces it in the mutant runs only | Re-measured (P22). D1, D15 and the spec: the caller's flags are what `go env GOFLAGS` prints in the program's environment; the refusal reads that value; every run gets it explicitly as `GOFLAGS`, a mutant run with the overlay appended. Scenarios for a `go env -w` value in the refusal and in "The runs" |
| MEDIUM 2: an insertion between top-level declarations borrows reach from the next function | Re-measured (P23). D13 and the spec: an insertion whose position lies outside every function body, as Go's parser reads the target, is not measurable; inside a body, only that body's blocks count. Scenario "A method added before a function that did not run" |
| NIT: a block counts its entry, not each line | Re-measured (P23); added to D13's declared limits |
| NIT: `git diff --no-index` honours an external diff | Re-measured (P24); the diff is taken with `--no-ext-diff --no-textconv` (D4, P20), and the spec says no external diff can change it |
| NIT: the bound case costs 2 s a pass | The case uses `-timeout 200ms` (bound 400 ms), counted in "Declared costs" |
| NIT: the SIGTERM stand-in should start a grandchild | D11 and task 2.6: the stand-in `go` starts a helper that writes its own pid and its parent's, as `TestFetchLeavesNoProcess` does, and both must be gone |
| NIT: Q1 and Q5 lack "what this costs you"; Q5's "often" and "many" | Both lines added; "often" and "many" removed, and Q5 and D14 say that none of the trial's 196 runs had a race report |

## Implementation review round 3 correction

| Finding | Correction |
| --- | --- |
| MEDIUM: an insertion after a labeled statement that is the last of its list reads not reached when the label is reached by `goto`: the last-before decider uses the statement's start, the label, which sits on the inclusive end of the block before it, and `goto` skips that block (measured by the review: `Lab2(-1)`, profile `9.2,10.1 2 0` and `11.2,11.11 1 1`, verdict invalid where survivor is right) | Re-measured (P29). The owner ruled, verbatim, "fix now" (#79, comment 5994738791), choosing: when the deciding statement is labeled, the region is not measurable, so survivor with the note. As the review advised, the rule applies to the last-before decider only. The first-after decider keeps a labeled statement: the place right before a label is reached only by falling through, which the block before the label measures. D13 and the spec say so; scenario "An insertion after a labeled last statement" |

## Implementation review round 2 corrections

| Finding | Correction |
| --- | --- |
| HIGH-2: an insertion whose place lies inside a statement of the list is decided by the wrong statement: inside a multi-line `if` condition, the next statement decides (block count 0, invalid, though the inserted condition ran); right after a label reached by `goto`, the block before the label decides, because the label's start is that block's inclusive end (invalid, though the label was reached) | Re-measured (P28). D13 and the spec: when the place lies inside a statement of its list (the statement begins on or before line k and ends on or after line k+1), the region is not measurable, so both shapes give survivor with the report saying reach could not be measured. The review's other fix, deciding by the containing statement's own block (for a label, its inner statement's), was weighed and not taken; D13 gives the reasons. Scenarios for a multi-line condition and for a label |
| NIT-A: a line inserted after a `return`, `panic`, `break`, `continue` or `goto` is dead code but reads reached | Declared in D13 beside the block-entry limit |
| NIT-B: a `TMPDIR` that does not exist is refused but not listed | D2 and the spec's refusal list, with a scenario |

## Implementation review round 1 corrections

| Finding | Correction |
| --- | --- |
| HIGH-1: an insertion at the end of a branch that did not run reads reached when the next line opens a sibling block that ran | **Erratum.** D13's insertion sentence (revisions 1 to 3: "an executed block of that body contains the position, or begins on line k+1") counted the block that begins on the next line even when it belongs to a sibling `else` branch or `case` clause. Design-review rounds 2 and 3 passed that sentence. The review's proposed fix (decide by the blocks that contain the place, and use the next line's block only when none does) mends the `if`/`else` case but not the `case` clause: a clause's block ends at its last statement, so no block contains the place after it, and the next line's block is the next clause's (measured, P25). D13 and the spec now decide an insertion by its own statement list, as Go's parser reads it; scenarios for an `if`/`else` and a `switch` case |
| MEDIUM: "every process it started" claims more than a process-group kill delivers | D8, D6 and the spec: the program stops the run's process group; a process the run starts in a process group of its own is not stopped, declared |
| MEDIUM: "invalid, does not build" skipped the after-run (declared departure 5) | D13 and the spec: invalid needs every baseline run and the after-run to pass and the tree unchanged, like survivor; scenario "The build breaks after the baselines" |
| NIT: the extra refusals are not in the delta (declared departure 1) | D2 and the spec's refusal list: a malformed `-expect`, an empty `-expect-text`, no `go.mod` at the root, `TMPDIR` inside the module; one scenario each |
| MEDIUM: children inherit the caller's `PWD`, so through a symbolic link the overlay never matches | Measured with `go` itself (P26). D15: every child's `PWD` is the resolved root. The spec's "every Go build ... SHALL see the wrong change" now says it holds through a symbolic link, with a scenario |
| MEDIUM: a caller's `diff.interHunkContext` merges hunks | Measured (P27). D4: the diff passes `--inter-hunk-context=0`; the spec says no hunk-merging setting can change the hunks, with a scenario |
| NIT: "selected no test" is read before a signal | Code only: the spec's "Killed by a signal" scenario already requires the reason to name the signal (task 2.11) |
| QUESTION: host git configuration (`core.fsmonitor`) reaching the program's git calls | D15 declares it; the tests isolate host git configuration (task 2.11) |

## What the inventory found, and what this design does with each

| Finding | This design |
| --- | --- |
| No command runs or classifies a mutation check, here or at the pin (inventory 1, Q14) | A new program, `internal/harness/mutcheck`, behind `task mutate:check` (D1-D4). No ledger row: the pin has no mutation command (Q14, closed) |
| Four spellings of the restore check: none, SHA-1, SHA-256, MD5 (2c, Q7) | Nothing is restored, because nothing is written. The target's SHA-256 before and after, and the tree's fingerprint before and after, go in the report. The documents move to SHA-256 (D12) |
| A three-way verdict does not survive `task`, which returns 201 for any non-zero exit (3.2, 11, Q10) | The verdict is the last line of the report; the exit status is zero only for detection (D3) |
| `harness-boundaries` binds on-demand harness commands to write nothing in the repository (3.2, Q9) | The program writes nothing in the repository and has its own write-nothing test, like `pindiff`'s (D1, D11) |
| A killed bash restore leaves the mutant; a SIGTERM restore waits for the child (2c, 6, 11) | No restore exists to fail. The program stops its processes on SIGINT and SIGTERM (D8) |
| The records name the assertion 3 times in 12; a helper moves the printed line; Rapid reports at `rapid.Check` (trial) | `-expect` takes a location as Go prints it, and `-expect-text` a fixed string. At least one is required (D2) |
| Hand classification counted a timeout, a kill, a stack overflow and a deadlock panic as detections (2b) | These verdicts are given by rule: timeout and kill are inconclusive; a panic counts only after an expected failure line (D5) |
| The pin's testing page has rules #46 dropped (2b) | Each is adopted, declined or put to the owner (D5) |
| Rapid replays failure files and writes new ones (2e; trial P8) | Every run sets `RAPID_NOFAILFILE=true` and a fixed `RAPID_SEED`; untracked failure files are refused (D7) |
| Integration tests cannot run under plain `go test` (2g, Q4) | Not supported; a test that selects nothing is inconclusive, and the message says integration tests run only through `task test:integration` (D9) |
| #60 and #80 overlap or touch the same rule (3.4, Q9, Q11) | Ordering section |
| Collision class: temporary ownership of source files while `go test` runs (6) | Overlay removes the ownership: the program never holds a file in the tree. The tree fingerprint shows another writer (D1) |

The inventory's open questions, and where each is answered:

| Inventory question | Answered in |
| --- | --- |
| Q1 How the intended assertion is named | D2 |
| Q2 Timeout and bounded assertions | D5, and owner question Q2 |
| Q3 Equivalence; Q3b Invalid | D5 |
| Q4 Integration tests | D9 |
| Q5 No `-timeout` on #79's command line | The spec: a two-minute default per run; D6's early stop |
| Q6 A fixture test needs the real `go test` | D11 |
| Q7 Four checksum spellings | D5, D12 |
| Q8 Where the backup lives | No backup: D1 |
| Q9 Writes nothing inside the repository | D1, D10; ordering against #60 |
| Q10 A three-way verdict through `task`; the `AGENTS.md` row | D3; owner question Q1 |
| Q11 Ordering against #48, #73, #60 and #80 | Ordering section |
| Q13 Which spec | D10 |
| Q14 (closed) No ledger row | "New surfaces" |
| Q15 "Lesson 9" has no source | Nothing in this design rests on it |
| Q16 Interrupts and kills | D8 |

## What the trial shows, and what was checked

The trial's report (`trial/report.md`, sha256 `b2b744e9ccc0be602f6062ce1ea2935b34ff7305e0cf408c18afcfaf61030546`)
was written by the trial agent and saved unedited. The design rests only on what is checked below.

| Trial claim | Checked how | Result |
| --- | --- | --- |
| 12 recorded cases, verdicts as listed | Each case's `report.txt` `VERDICT` line | Holds for all 12 and for the extra runs of R02 and R11 |
| 36 mutant runs, no flips; baselines and after-runs passed; trees identical | Per-run lines counted from each `report.txt` | Holds |
| Planted controls P1-P7 right | `VERDICT` lines | Holds (P1 in both overlay and edit mode) |
| Rapid: a failure file contaminates later runs; `-rapid.nofailfile` stops it | P8's six run directories, mapped by their command lines; Rapid v1.3.0 source | Holds. Rapid's source adds two facts: `-rapid.nofailfile` stops only the writing, and existing files under `testdata/rapid/<Test>/` are replayed on every run (P6) |
| The location a helper prints is the caller's (R11) | `R11/out-callsite` and `out-helperline` | Holds |
| Rapid reports at the `rapid.Check` line | `trial/trees/planted/internal/plantedmut/plant_test.go:63` is the `rapid.Check` call; the verdict names `:63` | Holds |
| Overlay would not reach `//go:embed` | Measured (P3) | **Refuted**: an embedded file sees the overlay |
| Overlay would not reach a subprocess build | Measured (P4) | **Holds for the flag; refuted for `GOFLAGS=-overlay=`** |
| A script cannot cap memory on macOS (R08) | `ulimit -v` under `/bin/bash` 3.2; `go doc runtime/debug.SetMemoryLimit` | Holds: the limit cannot be set ("Invalid argument"), and Go's memory limit is "soft" |
| The reach run shows the changed lines executed, and "also proves the overlay reached the build" (`proto/mutcheck/main.go:388-433`; P14 in revision 1) | Re-measured (P16) | **Refuted**: Go drops the overlay under coverage, so the reach runs measured the unchanged code. R04's line 591 lies in the original block `586.2,595.28`, which also holds the deleted line 592; R05's lines 207-208 matched the original blocks `207.36,208.46` and `208.46,210.4`, which hold the deleted lines 208-210. Both survivors are reached under D13's rule; the trial reached the right answer by a coincidence of line numbers |
| "Mine": the agent's own reading of the page | Not independent of the prototype: one agent wrote both | Used only where the page's text decides alone (R06, R08: both pages call a timeout and a kill inconclusive) |

## Goals and non-goals

Goals: a command that runs the experiment for one wrong change to one Go source file and gives its verdict by the
page's rules; writes nothing in the repository however it ends; and prints a record the pull request quotes.

Non-goals: generating wrong changes (#79 excludes `gremlins` and `go-mutesting`); judging whether a wrong change is
plausible or the named assertion is the intended one, which stays review only; integration tests; wrong changes to
scripts and other files read at run time (Q3 below); making the command a step of `task verify` or CI; any change to
`merge-check`.

## Options

### D1 How the wrong change is applied

| Option | What it is | Writes in the repository | Reaches | Costs |
| --- | --- | --- | --- | --- |
| **O1a Overlay only** (recommended) | Go source targets only; the overlay is passed through `GOFLAGS`; anything else is refused and pointed to the manual procedure | Nothing | The test binary, `//go:embed`, a child `go build` or `go run`, a re-run test binary | Script mutants stay manual (the seven of `review-gate-check`) |
| O1b Overlay, and edit with a backup for the rest | `cp` backup, write the mutant into the tree, restore, compare checksums | Yes, the target, for the length of the runs | Everything | Conflicts with the write-nothing requirement on `pindiff`'s commands (`harness-boundaries` `spec.md:292`, `:359`) and with `task fmt` as "the only command that writes" (`AGENTS.md:32`). A SIGKILL leaves the mutant in the tree (measured, inventory 11). Another reader of the tree sees the mutant (inventory 6) |
| O1c Overlay, and a disposable copy for the rest | Copy the tracked and untracked unignored files to a temporary directory, `git init` it, write the mutant there | Nothing | Everything | A second mode and its tests. A survivor in a script cannot be shown reached, because Go coverage does not cover scripts. Copying is cheap: 161 files in 0.11 s, and the `contract` and `probe` tests pass in such a copy with the same timing (P10) |
| O1d Disposable copy only | As O1c, for every target | Nothing | Everything | One mode, but every run is in another directory: logs show its paths, and a test that needs git history behaves differently (none found today). Gives up overlay, which the owner asked for |
| O1e Nothing | Keep the manual procedure | Yes | Everything | The class #79 is about, hand classification, stays open (inventory 2b) |

**Recommended: O1a.** It is the smallest change that covers every recorded Go mutant, it writes nothing, and a kill
leaves nothing behind. The overlay goes through `GOFLAGS`, not the `-overlay` flag, because only `GOFLAGS` reaches a
`go build` that the test starts (P4; `pindiff`'s `TestCommandExitStatus` is such a test). O1c is a real option with a
present consumer; it is Q3 for the owner, recommended "not now". Because Go drops the overlay for a file it builds with
coverage (P16), the program refuses when the caller's `GOFLAGS` sets `-cover`, `-coverpkg`, `-covermode` or
`-coverprofile`: with it, every mutant run would run the original code and pass (P17). The caller's `GOFLAGS` is what
`go env GOFLAGS` prints in the program's environment, so a value set with `go env -w` counts (P22, D15).

Reading the tree's state must not write either. `scripts/tree-state.sh` runs `git status`, which may refresh
`.git/index` as a side effect. The program runs it with `GIT_OPTIONAL_LOCKS=0`, which, in git's words, "will prevent
git status from refreshing the index as a side effect" (`git help git`, git 2.50.1). The script fingerprints the tree
it lives in (`tree-state.sh:8`, `cd "$(dirname "$0")/.."`), so each planted module in the tests carries a copy of
it; the script stays the one home of the fingerprint. It runs in a read-only tree (P15).

### D2 What the implementer gives

The command takes flags, not positional arguments, because seven inputs do not read well by position:

```text
task mutate:check -- -pkg ./internal/harness/probe -test TestAwaitClearsEarlierObservationError \
  -file internal/harness/probe/await.go -mutant /tmp/await.go -expect probe_test.go:227
```

- **The wrong change** is a copy of the target with the change made in it (`-mutant`), outside the repository. That
  is exactly what an overlay maps, and the report prints the diff, so the reviewer sees the change. A patch would have
  to be made by editing the tree first, which is what the page forbids restoring from (`docs/testing.md:124-125`). A
  search-and-replace pair (the pin's scripts, the prototype) quotes badly across lines on a command line.
- **The expected assertion** is required: at least one `-expect file.go:N`, a location as Go prints it at the start of
  an output line of the named test, or one `-expect-text`, a fixed string such a line contains. The file need not be a
  test file: Go prints the frame that reported the failure, which can be a helper in a non-test file
  (`lifecycletest.go`) or Go's own `testing.go` for a race report (P18).

Before any run the program also refuses an `-expect` that is not `file.go:N` with N a positive whole number, an empty
`-expect-text`, a directory with no `go.mod` (the program runs from the repository's root), a `TMPDIR` that does not
exist, and a `TMPDIR` inside the module, where its own files would be written into the repository. Each fails closed.

| Option for naming the assertion | Cost |
| --- | --- |
| Location only | A helper that calls `t.Helper()` moves the printed line to its caller (R11), and Rapid prints every failure at the `rapid.Check` line, so the assertions inside one property cannot be told apart by location (trial) |
| Text only | A short string can match an unrelated line; the report shows the line it matched |
| **Both, at least one required** (recommended) | Two flags to document |
| Neither: report "needs a human" (the prototype's default) | The page's rule ("the intended assertion fails") cannot be checked, and only 3 of 12 records named one |

What the command cannot do is tell whether the named assertion is the *intended* one. A location learned from a
failing run and then named is still a claim. It stays review only, and the report shows what was named and where the
runs failed.

### D3 How the verdict reaches the caller

`task` 3.51.1 returns 201 for any non-zero exit status (inventory 11), and `go run` also replaces it
(`harness-boundaries` `spec.md:293-294`).

| Option | Cost |
| --- | --- |
| A distinct exit status per verdict | Lost through `task` and `go run` |
| Exit zero whenever the command ran, as `ledger:diff` does | A survivor or an inconclusive check reads as a pass to anyone who looks only at the status; the pin warns of exactly this ("A runner's zero exit may mean its evaluation succeeded; inspect the report", `01-testing.md:222-223`) |
| **Exit zero only for detection; the verdict is the report's last line** (recommended) | A refusal and a survivor both fail; only the last line tells them apart |

### D4 A shell script or a Go program

| | `scripts/mutation-check.sh` (bash 3.2) | **`internal/harness/mutcheck`, a Go program** (recommended) |
| --- | --- | --- |
| Reading `go test -json` events | Needs `jq` (present at `/usr/bin/jq`; `merge-check.sh` already uses it) | Standard library |
| Bounding a run and stopping its process group | No `timeout` in base macOS: the one on this host is Homebrew's (`/opt/homebrew/bin/timeout`), and `scripts/doctor.sh` checks for neither tool | `exec.CommandContext` with a process-group kill, as `pindiff/pin.go:92` does |
| Signals | A SIGTERM trap waits for the foreground child (measured, inventory 11); SIGINT is ignored when started as an `&` job (`test-integration.sh:15-18`) | `signal.NotifyContext`, as `pindiff/main.go:34` does |
| Coverage profile for the reach check | `awk` over the profile | Standard library |
| Tests | Shell fixture test, which checks exit status only (inventory 5 (iii)), or Go tests driving the script | Go tests against the real toolchain, as `pindiff`'s `TestCommandExitStatus` does |
| Cost | Matches #79's named file | Changes #79's acceptance from a script and a bash fixture test to a program and Go tests. Each call compiles the program (about a second or two; not measured). The prototype is 500 lines of Go |

**Recommended: the Go program**, run as `go run ./internal/harness/mutcheck {{.CLI_ARGS}}` from `Taskfile.yml`.
Because #79 names a script, this is Q4 for the owner. Either way the line diff of the target against the mutant comes
from `git diff --no-index --no-ext-diff --no-textconv --inter-hunk-context=0 -U0` (P20): Go's standard library has no
line diff, and the program already runs git. The two `--no-` options keep a caller's external diff program or text
conversion from changing or breaking the hunks (P24), and `--inter-hunk-context=0` keeps a caller's
`diff.interHunkContext` from merging nearby hunks into one region that holds unchanged lines (P27).

### D13 How reach is shown

The page asks that "the change must compile and the test must reach it" (`docs/testing.md:119`). Reach is what tells a
survivor (the test ran the wrong change and still passed: look for a missing assertion) from an invalid wrong change
(the test never got there: look for a missing input). Revision 1 meant to take reach from a coverage run of the wrong change.
That run cannot exist under overlay: Go drops the overlay for a file it covers (P16).

| Option | What it measures | Costs |
| --- | --- | --- |
| **R1 Coverage of the unchanged code over the changed region** (recommended) | One more run of the unchanged code, with coverage of the target's package and no overlay; each hunk's region is read in the target's line numbers | Shows reach only when the test takes the same path in every run; a region with no statement that Go counts (a declaration, an import, a comment) is not measurable |
| R2 Coverage of the wrong change in a disposable copy | The mutant's own coverage, from a copy of the module with the mutant written into it | The copy machinery of O1c enters this change for one run. A deletion leaves no statement in the mutant to cover, so it needs the prototype's neighbouring-lines rule (`proto/mutcheck/main.go:104-112`), which counts the wrong block when the neighbours sit in other branches |
| R3 No reach check | Nothing | Invalid only for a wrong change that does not build. Step 2's "the test must reach it" stays review only, and an unreached wrong change reads as a survivor |

**Recommended: R1.** Why it answers the question: until a run first enters a changed region, the unchanged code and the
mutant run the same statements on the same inputs, because the code is the same there. So when the test takes the same
path in every run (the same inputs, the same seed, and no branch on scheduling or time before the region), a mutant run
enters a changed region if and only if the unchanged code executes one. That holds for any number of hunks ("any
region executed"), and it answers a deletion exactly: the deleted statements executed in the unchanged code, so the
mutant run arrived where they were. R2 would measure the mutant directly, but has no statement to measure for a
deletion, a common wrong change in the record (R04, R05, R08) and the page's own example (`docs/testing.md:145`,
"remove the emit").

The regions, from the hunks of the line diff (P20):

- A hunk that removes or replaces lines: those lines of the target. The region is reached when an executed block of
  the reach run's profile overlaps them.
- A hunk that only inserts lines after target line k: the place between lines k and k+1. When that place lies outside
  every function body of the target, as Go's parser (`go/parser`) reads it, the region is not measurable, like any
  other declaration: a new method or type placed before a function says nothing about whether that function ran. A
  function's first block begins on its `func` line (P23), so without this rule such an insertion would borrow the next
  function's reach. Inside a body, the place belongs to the innermost statement list that holds it: a function body, a
  block, a branch of an `if`, or a clause of a `switch` or `select`. The block holding the first statement of that list
  after the place decides; when no statement of the list follows the place, the block holding the last statement before
  it decides. A block of a sibling branch or clause never decides, which is the error revisions 1 to 3 made (see
  "Implementation review round 1 corrections"). Measured shapes (P21, P25): after `x = 0` on line 9 of `Clamp`, the
  next statement `return Limit` lies in block `8.15,11.3`; after the `if` that ends on line 11, the next statement lies
  in block `12.2,12.14`; at the end of `Sign`'s `if` branch, the last statement `x = -1` lies in `5.11,7.3`, not in the
  `else` block `7.8,9.3` that begins on the next line; at the end of `Name`'s `case 1:`, the last statement lies in
  `17.9,18.12`, which ends at that statement, while the next clause's block `19.9,20.12` begins on the next line.
  When the place lies inside a statement of its list, so that the statement begins on or before line k and ends on or
  after line k+1, the region is not measurable. That covers a condition, call or literal written over several lines,
  and the place between a label and its statement. Go's coverage counts statements, and an insertion inside one changes
  an expression or the statement's shape, which no block measures. The review also offered deciding by the containing
  statement's own block (for a label, its inner statement's). It gives the right answer for both measured shapes, but
  it is a proxy with errors of its own: an inserted operand after `&&` or `||` runs only when the operands before it
  allow, so the statement's block can show executed while the inserted part never ran; and a label's start sits on the
  inclusive end of the block before it (P28), the very slip that made round 2's finding. Not measurable gives survivor
  with a note, so an insertion that truly never ran reads survivor, and the reader is told reach could not be
  measured; it never gives a false invalid, which would send the implementer looking for a missing input while a
  missing assertion goes unseen.
  When no statement of the list follows the place and the last statement before it is labeled, the region is not
  measurable either, by the owner's ruling (#79, comment 5994738791). The decider would be the block holding the
  label, and a label reached by `goto` sits on the inclusive end of the block before it, which `goto` skips (P29). The
  first-after decider keeps a labeled statement: the place right before a label is reached only by falling through,
  which that same block measures. The cost, declared: a labeled loop as the last statement of a list, whose label
  lies inside an ordinary block and decides correctly (P29), now reads not measurable too. No Go file of this
  repository has a label today (implementation review round 3).
- A region no block decides is not measurable: no block overlaps the lines, the list has no statement, or no block holds
  the deciding statement. The wrong change is reached when any region is reached, not
  reached when every region is measurable and none is reached, and not measurable otherwise.

Verdicts: reached gives survivor; not reached gives invalid; not measurable gives survivor, with the report saying that
reach could not be measured. Invalid, whether by reach or because the wrong change does not build, needs what survivor
needs of the other runs: every baseline run and the after-run passed, and the tree did not change. A build that breaks
for every run after the baselines (a cache, a disk, a module download) then reads inconclusive, not "the wrong change
does not build". Not measurable is not invalid, because nothing shows the test missed the change, and it is
not inconclusive, because running again would not change it. A reach run that fails is inconclusive. The reach run comes
after the after-run, and only when every mutant run passed.

Several hunks are accepted, listed and flagged in the report. Go's unused-import rule makes a common wrong change two
hunks: removing the only call into a package means removing its import too. Refusing them would send those wrong
changes back to the manual procedure. Cost: a copy taken before a later edit of the target reverts that edit in every
mutant run; the list of hunks in the report is where that shows.

What R1 cannot show, declared: a region reached on some schedules and not others; a declaration's reach; the code the
wrong change makes newly run, which is not measured itself but lies after a changed region that is; and a line after a
call that does not return in that run. A block counts its entry, not each line (P23): a deletion after a call that
panics and is recovered, calls `runtime.Goexit`, or blocks for good reads as reached. Likewise a line inserted right
after a `return`, `panic`, `break`, `continue` or `goto` is dead code, yet the block of the statement before it decides
and reads reached, so the verdict is survivor; whether that survivor is equivalent is the reviewer's assessment.

R04 and R05 re-read: R04 deletes target line 592, inside the original block `586.2,595.28`, executed once; R05 deletes
lines 208-210, overlapped by `207.36,208.46` (12 times) and `208.46,210.4` (once). Both are reached under R1.

### D14 A report from the race detector

Every run uses `-race`, as `task test:unit` does. A wrong change that removes a lock can fail only with the race
detector's line `testing.go:1712: race detected during execution of test` (P18), not with an assertion of the test.
How often that happens is not measured: none of the trial's 196 runs had a race report (`race=false` on every run line
of `trial/cases/*/out*/report.txt`).

| Option | Cost |
| --- | --- |
| **The race report counts only when the implementer names it** (`-expect-text "race detected during execution of test"`), and the report shows the line (recommended) | The implementer has to know to name it. A race elsewhere in the package, set off by the wrong change's timing, counts as well; the report shows where the race was |
| A race report always counts as detection | Readmits "a different failure" as detection whenever the test races for an unrelated reason |
| A race report never counts | A lock-removal wrong change can only be detected by a test that observes a wrong value, which a race need not produce on cue |

This is owner question Q5, because whether the race detector is "the intended assertion" is a policy of the page, like
Q2. Races also depend on the schedule: three mutant runs that do not all report it disagree, and the check is
inconclusive.

## Decisions

These hold if the owner accepts the recommendations.

### D5 Verdicts, and the rules the pin had that #46 dropped

Per run, from `go test -json` events of the named test and its subtests:

| What the run shows | Per-run reading |
| --- | --- |
| The build failed (`build-fail`, `[build failed]`, `[setup failed]`) | Baseline: inconclusive. Mutant: **invalid** |
| No `run` event for the named test | Inconclusive: the name selected no test |
| `panic: test timed out after` | Inconclusive, even when an expected line was printed first |
| No result for the test and the process ended by a signal (`signal: killed`, R08) | Inconclusive |
| The run did not end by twice its timeout and was stopped | Inconclusive (D6) |
| The test passed and `go test` exited zero | Baseline: pass. Mutant: survivor, subject to the reach check |
| The test passed and `go test` exited non-zero | Inconclusive |
| The test failed with a failure line at an expected location or containing an expected text | Detection; later panics and other failure lines are recorded as notes (R09, R12) |
| The test failed and its only failure is a race report that no expected location or text matches | Inconclusive (D14) |
| The test failed otherwise | Inconclusive: the report gives the locations that failed |

What is matched is an output line of the named test or of its subtests that begins with an expected location or
contains an expected text. Go's JSON output says which test printed a line, but not whether `t.Log` or `t.Error`
printed it (P19), so a log line at a named location gives a detection although a different assertion failed. The report
prints every matched line in full, so the reader sees which line it was; the limit is declared.

The final verdict is the spec's: inconclusive unless every baseline run passed, the mutant runs agree, the after-run
passed and the fingerprint did not change. Reach decides between survivor and invalid (D13).

The pin's rules that #46 did not carry (inventory 2b, the carry table):

| Pin rule (`01-testing.md`) | Decision | Reason |
| --- | --- | --- |
| **Invalid**, the fourth outcome (`:215`) | **Adopt**, for a mutant that does not build or is not reached | "Fix your wrong change" and "fix the run" call for different actions. The page already asks that "the change must compile and the test must reach it" (`docs/testing.md:119`), and the reach check is the only way the command can show reach |
| A bounded assertion that observes a required termination can detect; an outer timeout alone cannot (`:216-218`) | **Adopt the rule.** Whether to make an exception for tests built to fail only by the binary's timeout is **Q2** | The command already behaves this way: a test's own bound is an assertion line (R07), and the whole-run panic is inconclusive (R06) |
| Equivalence is a reviewer's assessment, with its contract, input domain, reasoning and reviewer (`:220-222`) | **Adopt** | The command reports a survivor as a survivor; "equivalent" is never a verdict. PR #48 recorded one without the parts the pin asks for (inventory 2b) |
| Restore the original bytes and verify checksums (`:188`) | **Adopt in substance; the mechanism changes.** Nothing is written, so nothing is restored; the target's SHA-256 and the tree fingerprint before and after are recorded. The manual procedure keeps the checksum, as SHA-256 (D12) | One spelling of the check, not four (Q7) |
| A disposable copy (`:180`) | **Decline for now**; Q3 | Overlay already writes nothing for Go source; the copy's only extra reach is scripts and data |
| Do not restore over concurrent edits (`:190-191`) | **Adopt** as the fingerprint check: a tree that changes during the check makes it inconclusive | Overlay overwrites nothing, so the remaining risk is runs that read a moving tree |
| Not a new CI gate (`:174-175`); no runner is selected (`:71-72`) | **Adopt "not a gate."** The command is run by hand. The pin's "no runner" is superseded by #79 itself | The owner's sequencing keeps the command out of `task verify` |

### D6 Repetition, flaky baselines, early stops

Three baseline runs, three mutant runs and one after-run by default, with `-runs N` to change the first two counts,
then the reach run when every mutant run passed (D13). A baseline failure stops the check; a mutant run ended by the
timeout, a signal or the bound is the last one. That would have saved two of R06's 60-second runs and two of R08's
82-99-second runs.

The bound: a run that has not ended by twice its `-timeout` is stopped with its process group, and is inconclusive.
`go test`'s own timeout covers only the test binary, not the build or a process the test leaves holding its output;
twice the timeout gives the build as long as the test. The report states the bound.

Three baseline runs catch only a frequent flake: P7 failed once in three. A rarer flake can pass all three, and the
command does not read GitHub's `class:flake` issues. Both are declared.

### D7 Generated inputs (Rapid)

Every run's environment sets `RAPID_SEED` (from `-seed`, default 1) and `RAPID_NOFAILFILE=true`. Rapid reads both from
the environment (v1.3.0 `engine.go:91-93`). A package that does not import Rapid ignores them, so the command does not
need to know which packages use it. `docs/testing.md:259-260` already asks for `-rapid.nofailfile` during a mutation
check, and the pin's page asks that a generated failure be replayed with the same seed against the wrong change and the
original (`01-testing.md:203-205`). A seed of 0 is refused, because Rapid takes it as "choose a random seed"
(`engine.go:70`), and the runs of one check would then differ. The program refuses to start when a file that git does
not track exists under the package's `testdata/rapid/`. Rapid replays every file there on every run
(`engine.go:338-340`), and P8 showed an earlier run's file turning a survivor into a detection. Tracked files are not
refused: PR #48 commits curated seeds under `pkg/types/testdata/rapid/` on purpose, and its `README.md` there says "Run
mutation checks with `-rapid.nofailfile` so they never write here in the first place". A tracked seed is a fixed input,
replayed in the baseline and mutant runs alike. A seed that misses the wrong change gives a survivor for that seed
only, so a survivor's reason always names the seed and says that, for a test that generates its inputs, the result
holds for it. Native fuzz targets need nothing: plain `go test` replays only their
seeds. The test of this behaviour needs a Rapid property, and Rapid enters `go.mod` with PR #48, so that task waits
for PR #48 (the owner's sequencing).

### D8 Interrupts, kills, runaway memory

- **SIGINT or SIGTERM** to the program: it stops the process group of the run in progress, exits non-zero and prints no
  verdict. Nothing in the tree needs restoring. Every run starts in a process group of its own, and the stop (and the
  bound of D6) reaches every process that stays in that group. A process the run starts in yet another group is not
  stopped: `pindiff`'s fetch does that (`pin.go:91`), and so does this program for each of its own runs, so a check of
  the program's own end-to-end tests leaves the inner program's `go test` to its own timeout. Declared.
- **SIGKILL** to the program: the `go test` it started runs on until its own `-timeout`, then exits. The program itself
  has written nothing in the repository; what the orphaned test does is the test's, and the next check's fingerprint
  starts from the tree as it is then. The temporary directory with the logs and the overlay file stays behind outside the
  repository. Declared.
- **Runaway memory (R08):** an unbounded recursion under `-race` grew until the operating system killed it, three
  times, at 82-99 s. Memory cannot be capped from the program on macOS (P8). The early stop in D6 limits it to one
  mutant run. A killed run is inconclusive and its reason says the process was killed. Declared; not solved.

### D9 What the command does not take

- **Integration tests.** A test in a file tagged `integration` is not built by plain `go test`, so its baseline
  selects nothing and the check is inconclusive. The message adds that integration tests run only through
  `task test:integration`, which this command does not drive (Q4 of the inventory).
- **Scripts, YAML, test data, scanned source text.** Refused by file type (D1), except Go source read as text at run
  time, which the refusal cannot see. A wrong change there either shows as not reached (invalid), or the test never
  sees it and passes. The second case is a false survivor when the target is a non-test file of a package the test
  both builds and scans. No such test exists today: the contract guards scan the tree but plant their cases in
  temporary trees. Declared.
- **Test files.** Refused: the page keeps the test and its expectations unchanged, and Go coverage does not cover
  test files, so a survivor could never be shown reached. The pin's scripts mutated `_test.go` test-support files.
  In this repository test support lives in non-test files under `internal/harness/`.

### D10 Where the behaviour is specified

A new capability, `mutation-check` (inventory Q13). `harness-boundaries` already binds `pindiff`'s on-demand commands,
but its Purpose is keeping test machinery out of production code, keeping SemEngine's Docker footprint apart from
SemStreams' cleanup, and keeping ported files traceable (`spec.md:3-6`); a mutation check is none of these. The new spec
takes over the write-nothing guarantee in its own words, so `pindiff`'s requirement is unchanged. Nothing in
`merge-gate` changes.

### D11 Tests

The developer writes each test first and records it failing. The expected verdicts come from the spec's scenarios,
which come from the page's text, never from running the program.

1. **The classifier**, on recorded event streams. `go test -json` output captured from go1.26.6 for each per-run case
   in D5 is kept under the program's `testdata/`: among them a race report (P18), a log line at the expected location
   (P19), a run killed by a signal and a build failure. Each case's expected reading is written in the test.
   *Generated checks:* the per-run reading is a function of about eight inputs (build, selection, timeout, signal,
   result, exit status, an expected hit, a race report). The decision is an exhaustive enumeration of that small domain,
   checked against a reference table written in the test from the spec's rules. No library is needed, and every
   combination runs. The aggregate rules (agreement, early stops, after-run, fingerprint, reach) get named examples.
2. **Regions and reach**, on recorded line diffs and coverage profiles: each scenario of "The changed region and
   reach". Two planted cases make the region rules able to fail. In one, three deleted lines executed while the lines
   with the same numbers in the mutant lie in a block of the target that did not: a program that reads regions in the
   mutant's numbers says not reached, and the right answer is reached. In the other, a line is inserted right after an
   `if` line whose branch did not run: the prototype's neighbouring-lines rule says reached, and the right answer is
   not reached.
3. **Refusals**, each scenario of "The command and its inputs".
4. **With a stand-in `go`**, a script first on `PATH` that prints a recorded event stream, as `mergecheck_test.go` does
   with its stand-in `gh`: a run that does not end by its bound (with `-timeout 200ms`, so the bound is 400 ms), and a
   test that writes into its package directory. Neither needs a build.
5. **End to end with the real toolchain**, on planted modules in temporary directories, as `pindiff`'s
   `TestCommandExitStatus` does (inventory 5 (iii); Q6: "never invoke the real go" is a comment in one package, not a
   rule). Three cases, cut now (see "Declared costs"):
   - a detection in a planted module whose every directory is read-only, where the test detects the wrong change
     through a command it builds with `go build`. One check shows the overlay reaching a child build through `GOFLAGS`,
     the environment of every run (seed, `RAPID_NOFAILFILE`, a caller's `-tags` set with `go env -w` in a temporary
     `GOENV` file), and that the content hash of every
     file is unchanged, as `TestWritesNothingInTree` does;
   - a deletion survivor that was reached, so a real reach run, its profile and its regions are read end to end.
     Measured over budget with item 6 added (task 2.10), it moved to a recorded profile and a stand-in `go`
     (`TestReportSurvivor`), as "Declared costs" provides; item 6's survivor now reads the real reach run;
   - SIGTERM to the built program while its stand-in `go` waits, having started a helper that writes its own pid and
     its parent's, as `TestFetchLeavesNoProcess`'s helper does: neither process is left.

   Each planted module is a git repository with a copy of `scripts/tree-state.sh`.
6. **Rapid** (waits for #48): a planted property in a temporary module that resolves Rapid from the module cache with
   `GOPROXY=off`: detection for a seed that finds the wrong change, survivor for one that does not, and no file under
   `testdata/rapid/` afterwards.

Planted test sources inside Go test files split the literals the text guards match, as `testtext_test.go:14` does.
A planted test that has to wait waits on a channel, never on `select {}` (PR #48's new rule) or a sleep.

**The self-check.** The program builds each child's environment from the environment it is given (D15). Its tests give
it one whose `GOFLAGS` they choose, and the SIGTERM test builds the program with the inherited environment (so an outer
check's overlay reaches the built program) but runs it with `-overlay` removed from `GOFLAGS`. An outer check of the
program's own package is then not refused by the program under test. Measured in task 3.1: the command checked its own
package, 18 checks and 18 detections (PR #82, comment 5983606233).

**Shown able to fail.** Each of these wrong changes to the program must be detected by its own tests, run as the page
asks and recorded on the pull request: a non-zero exit read as detection; a survivor reported without a reach run; the
fingerprint compare dropped; a timeout read as detection; a failing baseline ignored; a location outside the expected
set accepted; an unexpected race report accepted; the overlay passed as a flag instead of through `GOFLAGS` (the child
build then sees the original); a caller's `GOFLAGS` dropped from the mutant runs; a `GOFLAGS` with `-cover` not refused;
regions read in the mutant's line numbers; an insertion's region taken from its neighbouring lines, or decided by a
sibling branch's block; an insertion inside a statement decided by a neighbouring statement's block; an insertion after
a labeled last statement decided by the label's block; hunks merged by a caller's git setting; a child's `PWD` left as
the caller's; a region that is not measurable read as not reached; exit zero for a survivor; and `RAPID_NOFAILFILE` not
set.

### D12 Documents

The `docs/testing.md`, `AGENTS.md` and `docs/repository-map.md` edits wait until #48 and then #73 merge: the owner's
sequencing names the first two, and both pull requests also change the map. The contract edits wait for #48, the only
open pull request that changes the contracts. The preflight skill, which neither touches, does not wait.

- `docs/testing.md`, "Show that the test can fail": names the command and what it checks; keeps the manual
  procedure for what the command refuses; adds invalid, the bounded-assertion sentence and equivalence as an
  assessment (D5); and gives the restore checksum as SHA-256.
- `.agents/contracts/semengine-developer.md` and `semengine-reviewer.md`: the mutation passages (inventory 2a, 2c)
  name the command and give the checksum as `shasum -a 256`.
- `AGENTS.md`: the Commands block gains `task mutate:check`. The rules row changes as the owner rules on Q1 (also
  held on that ruling).
- `docs/repository-map.md`: the program's row and the new spec.
- `.agents/skills/semengine-preflight/SKILL.md`: the task table gains the command. Not held.

### D15 The environment of the runs

Every run of one check gets the same command line and environment, apart from the overlay of a mutant run and the
coverage flags of the reach run. The caller's flags are what `go env GOFLAGS` prints in the program's environment.
That includes a value set with `go env -w`, which lives in Go's environment file and not in the process environment;
and a `GOFLAGS` in the process environment replaces the file's value entirely (P22). So the program reads that value
once, refuses on it (D1), and sets it explicitly as `GOFLAGS` in every run, a mutant run with `-overlay=<file>`
appended. Without this, a file-set flag would apply to the baseline runs and vanish from the mutant runs, whose
`GOFLAGS` carries the overlay. `GOFLAGS` is the only setting the program overrides, so every other setting of Go's
environment file applies to every run alike. The coverage flags of the reach run go on its command line. The report
prints each run's command line and `GOFLAGS`.

The program builds each child's environment from the environment it is given, setting `GOFLAGS`, `RAPID_SEED`,
`RAPID_NOFAILFILE`, `PWD` and, for git, `GIT_OPTIONAL_LOCKS`, rather than letting children inherit its process
environment unread; that is what lets its tests, and a check of its own package, decide what a child sees. `PWD` is the
root with symbolic links resolved, the same root the overlay's paths are built from. Go takes its working directory from
`PWD` when `PWD` names the current directory, and looks overlay paths up under it without resolving links, so a caller
whose `PWD` reaches the repository through a symbolic link would otherwise get mutant runs that build the unchanged code
(P26). `os/exec` sets `PWD` only when a command's environment is left empty, which the program never does.

Host git configuration reaches the program's git commands, as it reaches the caller's own. A setting such as
`core.fsmonitor=true` can make the `git status` the program runs start git's file-system monitor, which keeps its own
files under `.git`. The program does not override host git configuration; that is the user's setting acting, declared
and not measured. The program's tests isolate host git configuration (`GIT_CONFIG_GLOBAL=/dev/null`,
`GIT_CONFIG_NOSYSTEM=1`), so their snapshots of planted modules cannot see it.

## Ordering against #48, #73, #60 and #80

- **#48, then #73:** ruled by the owner (#79 comment 5981621215). The document tasks hold on both, the Rapid test on
  #48, and PR #82 lands after both. PR #48 was at `82e979b` when this revision was written. Its 279-file list and
  its hunk headers for `AGENTS.md`, `docs/testing.md`, `docs/provenance.md`, `docs/repository-map.md` and the three
  contracts are the same as at `a2f975a`, the head inventory review round 2 checked, and at `4e44422`, the head
  revision 1 checked (read through the pulls API). PR #73 is unchanged at `f612ec5`.
- **#60** (a command that writes a ported package's files into the tree): no order is needed. This change writes
  nothing in the repository, so it adds no exception to `pindiff`'s write-nothing requirement or to `AGENTS.md:32`.
  It touches none of #60's files: its spec delta is a new capability, and #60's question is about `harness-boundaries`
  and `pindiff`. #60 stays the only proposed writer.
- **#80** (tests name the requirement they prove): the owner's words, as #80 comment 5981307449 records them, are "agree
  on 80". The session that relayed them reads them as choosing Option A, a guard over product packages only; that
  reading is the session's. The same comment says `main` has no product package today, so this change's tests, under
  `internal/harness/`, are outside the guard either way. Both changes edit `docs/testing.md`, in different sections, and
  both add to the `AGENTS.md` rules table, in different rows. Neither depends on the other. Whichever merges second
  merges `main` into its branch and keeps both texts. The ordering is not ruled; this is the design's reading, and the
  owner may rule otherwise.

## What the command does not see

- Whether a pull request used it at all. Only a check that reads the pull request could see that (Q1).
- Whether the wrong change is plausible, and whether the named assertion is the intended one (D2).
- Whether a matched line was a log line or a failure: Go prints both the same way (D5, P19).
- A flake rarer than one failure in three baseline runs (D6).
- A wrong change in a file the test reads while it runs (D1, D9).
- Memory: a runaway mutant is stopped only by the operating system, the timeout or the bound (D6, D8).
- A process the run starts in a process group of its own: neither the bound nor an interrupt stops it (D8).
- Reach on some schedules only, or inside a child process. The reach run shows one path of the unchanged code, and a
  child process (a command the test builds, or the test binary run again) writes no coverage, so a region reached only
  there reads as not reached and the verdict is invalid, not survivor. A detection there is unaffected (R09, R11).
- The reach of a change to a declaration (D13: not measurable).
- A test that builds with coverage itself: that build compiles the original (P16).

## Invariants and their spec homes

| Invariant | Spec home |
| --- | --- |
| Exit status zero if and only if the verdict is detection | "Report and exit status", first two scenarios |
| The program itself writes, creates or removes no file in the repository, whatever ends it | "The program writes nothing inside the repository", "A read-only tree" and "Interrupted" |
| A failing exit status alone never gives detection | "Outcome classification", "A failure that names no expected location" |
| A survivor is never reported when the reach run shows the wrong change was not reached | "The changed region and reach", "A deletion in a branch that did not run" |
| Regions are read in the target's line numbers | "The changed region and reach", "Regions are read in the target's line numbers" |
| Every run of one check has the same flags and environment, apart from the overlay and the reach run's coverage | "The runs", "The environment is the same in every run" |
| A whole-run timeout is never a detection | "Outcome classification", "Whole-run timeout" |

## What a session has to know after this change

The inventory (section 7) listed ten facts a session must hold to run the experiment by hand. With the command, four:

1. Make the wrong change in a copy of the file, outside the repository.
2. Name the assertion you expect to fail, by the location Go prints or by its message (and, if Q5 is answered as
   recommended, name the race detector's line when it is the intended check).
3. The command handles a non-test Go source file in a unit-tested package. For anything else it refuses and says so,
   and the page's manual procedure applies.
4. For a test that generates its inputs, the seed decides whether the wrong change is found: choose one that finds it.

Four facts is two more than the architect contract's limit of two, so it is a design finding, recorded here. If a
session knows none of them: the first three are checked before any run, and the session gets a refusal that names the
flag or the file and, for the third, the manual procedure. The fourth shows in the survivor's reason, which names the
seed. Revision 1 said no path ends in a wrong verdict; that was wrong. Four still can, and are declared: a log line
named as the expected location gives a detection while another assertion failed (the report prints the line); a region
reached only on some schedules or only in a child process is judged from one path of the unchanged code; a deletion
after a call that does not return reads as reached (D13); and a seed that misses gives a survivor that holds for that
seed only. A difference in `GOFLAGS` between runs no longer can, including one set with `go env -w` (D15). The gap to
"nothing": the second fact is the page's own rule, because only the implementer can say which assertion the wrong change
is meant to trip; the first and third follow from overlay, and Q3 is the choice that would remove the third; the fourth
belongs to generated tests.

## Declared costs

- **Test time.** The real toolchain runs in three cases only (D11, item 5), cut now rather than after a measurement.
  A warm `go test -race -json` of a planted one-file module took 0.3-0.4 s here, with or without the overlay and with
  `-coverpkg`; the first runs after a toolchain switch took 1.3-3.2 s (scratch module `arch-ov2`, go1.26.6). The
  detection case makes three runs, each building a child command; the survivor case four; the SIGTERM case one build of
  the program. That is about 6-8 s per pass. The stand-in `go` cases add about half a second, most of it the bound case's
  400 ms wait. `task test:unit` runs the package once and `task test:repeat` five times (`-count=5`), so about 40-55 s
  summed. The budget is 60 s, measured
  as the sum of the two `ok .../internal/harness/mutcheck <time>` lines that the `test:unit` and `test:repeat` steps
  print in a CI verify log (task 2.9). Over budget, the survivor case moves to a recorded profile and a stand-in `go`,
  leaving the detection and SIGTERM cases.
  Measured (task 2.9): CI run 37228603260 at `76aa263` printed 6.727 s for `test:unit` and 24.880 s for
  `test:repeat`, 31.607 s in all, under the budget; the survivor case stays on the real toolchain. The earlier run
  37226655576 at `9c1b3e1` printed 5.576 s and 20.860 s (26.436 s).
  Measured (task 2.10), with the two Rapid cases of D11 item 6 added: CI run 37477518090 at `85b6984` printed 11.715 s
  and 54.941 s, 66.656 s in all, over the budget. In that run's `test:repeat`, `contract`, `metric`, `natsclient`,
  `tlsutil` and `pindiff` took 1.5 to 2.0 times as long as in run 37474423694 at `cf74191` (this package: 5.016 s and
  20.933 s) and `runner` the same, so part of the rise may be the runner. Over budget, the survivor case moved to a
  recorded profile and a stand-in `go`.
- **Run time of a check.** With the defaults a check is seven runs, plus one reach run for a would-be survivor: 6-43 s
  for the trial's cases other than R06 and R08, and about 60 s plus the baselines for a whole-run timeout of 60 s.
- **The program itself:** about the prototype's size (500 lines), plus tests. Each call compiles the program first.
- **Owner time:** five questions below.

## New surfaces and who uses them

| Surface | Present consumer |
| --- | --- |
| `task mutate:check` and `internal/harness/mutcheck` | The implementing agent under `semengine-developer.md:58-63`, `:203-205`; the reviewer reproducing evidence (`semengine-reviewer.md:47-49`, `:196-198`), as PR #59's reviewer did with overlay; the trial's 12 recorded mutants |
| The report block | The record line `docs/testing.md:312` and the reviewer's question at `:321-322` |
| The `mutation-check` capability spec | This change's tests |

New surface, not ported: no ledger row (Q14).

## The adoption sweep

This change does not establish a primitive for reuse across packages: the program is one tool, and its classifier
is internal to it. The places that spell the manual procedure adopt the command in this change (D12). Nothing else
should adopt it. A tracking issue is not owed.

## Questions for the owner

Each question is in plain words: the recommendation first, then why, then what the other answer costs.

**Q1. Once the command exists, what should the `AGENTS.md` rule on mutation outcomes (row `:99`) say under
"Enforced by"?**

Recommendation: keep "review only", and name the command inside that phrase, the way the spec-queue row names
`task spec:queue` (`AGENTS.md:93`). For example: "review only; `task mutate:check` runs the experiment for a Go source
file and prints a record a pull request can quote; nothing fails when a pull request does not use it." The row's other
two rules (fuzz seed replay reported apart from exploration, and recording what was not covered) stay review only
whatever wording is chosen.

Why: the table says "review only" means that no command fails when the rule is broken (`AGENTS.md:60-61`). This rule
is broken when a pull request calls a survivor or an inconclusive run a detection, and nothing fails then: the command
runs only when someone runs it, and it is in neither `task verify` nor CI. Every "Enforced by" entry that is not review
only names a check that runs on every change. The table's two on-demand commands, `task ledger:diff` (`:77`) and
`task spec:queue` (`:93`), appear only inside "review only".

A proposal that was withdrawn: the orchestrator first proposed splitting the row like the ledger row, "when used, the
command does the classification, the seed replay and the run that writes nothing; whether a pull request used it, and
whether the mutant and the named assertion are the right ones, stay review only". The pre-owner review showed why it
does not fit (`design-review-r1.md`, "Owner question Q1"): the ledger row is split because `task ledger:check` fails
in `task verify` when a carry row is broken, and this row has no such half; "when used" is the mark of a check that
fails nothing when skipped. The orchestrator withdrew the proposal.

If you choose otherwise: putting the command in the enforced column makes it the table's only optional command counted
as enforcement, which hides the kind of drift the table exists to show. The only way to make the rule truly fail when
broken is a later `merge-check` rule that fails a code pull request claiming a detection without the command's report.
That costs a new marker in pull request text, a new rule with its tests, and false failures on pull requests where no
mutation check applies; it is worth ordering only if hand-written detection claims keep getting past review. What
"review only" costs you: a pull request that writes its detection by hand, without the command, still passes unless a
reviewer reads its record, as today.

**Q2. Should a test that can fail only by `go test`'s overall timeout ever count as catching the wrong change?**

Recommendation: no. A whole-run timeout stays inconclusive, as both our page (`docs/testing.md:133`) and the pin's
(`01-testing.md:216-218`) say. When a detection is needed from such a test, the test gets its own time limit.

Why: a timeout says the run never finished, not which assertion caught the wrong change. The trial's R06 (PR #48's
`LatestNeverWaitsOnCollection`) is this case, and PR #48's records count it as detected (inventory 2b).
`probe_test.go:26-27` bounds 8 call sites only by the binary's timeout.

If you choose yes (a flag that lets a timeout count when the named test is the one running): it readmits the outcome
both pages reject, and every report would have to be read to see whether the flag was used. What "no" costs you: R06's
record is PR #48's to correct, and wrong changes like R06 stay inconclusive until their test gets its own limit.

**Q3. Should the command also handle wrong changes to scripts and other files a test reads while it runs?**

Recommendation: not now. They stay with the page's manual procedure, and the command says so when given one.

Why: overlay cannot reach those files, so the command would need a second way of applying a wrong change, a
disposable copy of the tree (O1c). Go's coverage does not cover scripts, so a script survivor could never be shown
reached and would need a rule of its own.

If you choose yes: the copy mode, its tests and that rule come into this change; copying itself is cheap (P10). What
"not now" costs you: the next wrong change to a script guard (`merge-check.sh`, `cover-check.sh`,
`cleanup-roots-check.sh`) is checked by hand, as `review-gate-check`'s seven were.

**Q4. Should the command be a Go program instead of the shell script #79 names?**

Recommendation: yes: `internal/harness/mutcheck`, run by the same `task mutate:check`.

Why (D4): the command reads `go test`'s JSON output and a coverage profile, gives each run a time limit and stops all
its processes, and handles Ctrl-C and SIGTERM. The macOS shell does the last two badly: a measured SIGTERM waited for
the running child, and macOS has no `timeout` command without Homebrew. `pindiff` is the precedent: an on-demand
harness program tested end to end against the real toolchain.

If you choose the script: `jq` and Homebrew's `timeout` become tools `scripts/doctor.sh` does not check, and signal
handling rests on shell traps the inventory measured failing. What "yes" costs you: #79's acceptance changes from a
script and a shell fixture test to a program and Go tests, and each run of the command compiles it first.

**Q5. Should a report from Go's race detector count as the test catching the wrong change?**

Recommendation: only when the implementer names it in advance, with `-expect-text "race detected during execution of
test"`; otherwise the check is inconclusive.

Why: the page counts only the intended assertion. Every unit test here runs under the race detector, and for a wrong
change that removes a lock the race detector can be the only thing that notices, so it can be the intended check, but
only the implementer can say that it is (D14). Requiring the name keeps an unrelated failure from counting by accident,
and the report shows the line. How often a wrong change shows only as a race report is not measured: none of the
trial's 196 runs had one.

If you choose "always counts": a race that the wrong change only sets off somewhere else counts as catching it. If you
choose "never counts": a lock-removal wrong change can be caught only by a test that sees a wrong value, which a race
need not give on cue, so such wrong changes would read inconclusive. What the recommendation costs you: the implementer
has to know to name the race line, and once it is named, a race elsewhere in the package that the wrong change sets off
counts as well; the report shows where the race was.

## Owner's rulings

The owner accepted this design and answered Q1 to Q5 as recommended on 2026-10-04, verbatim: "Accept, agree with all
five" (#79, comment 5982900883; asked in comment 5982520123, on the design at `daca356`). So: Q1, the `AGENTS.md` row
stays "review only" and names `task mutate:check` inside that clause; Q2, a whole-run timeout is never a detection;
Q3, scripts and files read at run time stay with the manual procedure; Q4, a Go program at `internal/harness/mutcheck`;
Q5, a race report counts only when named in advance with `-expect-text`.

### Conformance to the rulings

Each binding ruling, where the code carries it out and which test shows it, or the held task where it waits. Paths
under `internal/harness/mutcheck/` are given by file name. No deviation from a ruling was found, so there is no
DEVIATION row.

| Ruling | Carried out at | Shown by | State |
| --- | --- | --- | --- |
| Q1 (#79, comment 5982900883): the `AGENTS.md` mutation-outcome row stays "review only" and names `task mutate:check` inside that clause | Not yet: `tasks.md:179`, task 4.5, held on #48 and then #73 by the sequencing ruling | None yet | Held |
| Q2 (same comment): a whole-run timeout is never a detection; the check is inconclusive | `classify.go:182`, read before the detection case at `classify.go:192` | `classify_test.go:130` (a recorded timeout after an expected line); `classify_test.go:205`, the reference reading of `TestReadEveryCombination` (`classify_test.go:221`) over all 2048 combinations; `classify_test.go:339` (the check's verdict); task 3.1, W04 | Carried out |
| Q3 (same comment): wrong changes to scripts and other files read at run time stay with the manual procedure, and the command says so | `inputs.go:39` (the manual-procedure text); `inputs.go:130` and `inputs.go:132` (a test file, or a target that is not Go source, is refused with that text) | `refuse_test.go:56` (a script as the target); `refuse_test.go:61` (a test file as the target) | Carried out |
| Q4 (same comment): a Go program at `internal/harness/mutcheck`, run by `task mutate:check` | `main.go:1` and `main.go:29`; `Taskfile.yml:73-76` | The package's tests, end to end at `e2e_test.go:48`; tasks 3.1 to 3.5 and 2.14 ran every check through the command | Carried out |
| Q5 (same comment): a race report counts as a detection only when named in advance with `-expect-text`; otherwise inconclusive | `classify.go:192` (detection needs an expected line); `classify.go:194` (a race report with none is inconclusive) | `classify_test.go:122` and `classify_test.go:125` (a recorded race, not named and named); `classify_test.go:354` and `classify_test.go:356` (the check's verdicts); task 3.1, W07. The sentence in `docs/testing.md` is `tasks.md:172`, task 4.3, held | Carried out; documents held |
| Sequencing (#79, comment 5981621215): the trial first; the program, its tests and the `Taskfile.yml` entry now; the `AGENTS.md` row and `docs/testing.md` after #48 and then #73; seed replay after #48 | The trial: `design.md:217`. Now: the package and `Taskfile.yml:73`. Held: `tasks.md:172`, `tasks.md:177` and `tasks.md:179` (tasks 4.3 to 4.5); `tasks.md:79` (task 2.10, the Rapid property). Every run already sets `RAPID_SEED` and `RAPID_NOFAILFILE` (`main.go:118`) | `e2e_test.go:48`, whose planted test fails unless both variables reach the run; task 3.1, W15 | Carried out; held parts as listed |
| Labeled last statement (#79, comment 5994738791): "fix now": when the last statement before an insertion is labeled, the region is not measurable | `reach.go:177` | `reach_test.go:125` (written first at `efbd685`); `reach_test.go:130` (the first statement after the place still decides when labeled); task 3.5, V08 | Carried out |

## Premises

| # | Premise | Measurement |
| --- | --- | --- |
| P1 | `task` 3.51.1 returns 201 for a command exiting 1, 2 or 3; `task -x` keeps the status | Inventory 11 (scratch `Taskfile.yml`); reproduced in inventory review round 2 |
| P2 | `go test` exits 1 for an assertion failure, a timeout, a panic and a build failure, and 0 when `-run` selects nothing | Inventory 11 (go1.26.6); the reviewer reproduced it on the local default toolchain |
| P3 | An overlay reaches a file embedded with `//go:embed` | Scratch module `arch-ov2`: `TestEmbed` with an overlay of `data.txt` printed `e_test.go:12: embedded "mutant\n"`, with the flag and with `GOFLAGS`, on go1.26.6 and go1.26.4; both files' SHA-256 unchanged afterwards |
| P4 | A `go build` started by the test sees an overlay given through `GOFLAGS=-overlay=<file>`, and not one given as the `-overlay` flag | Same module: `TestSubprocessBuild` passed (original seen) with the flag; failed with `e_test.go:26: tool printed "mutant\n"` with `GOFLAGS`; go1.26.6 and go1.26.4 |
| P5 | An overlay does not reach a file the test reads at run time | Inventory 2a and 11; reviewer reproduced (round 2) |
| P6 | `-rapid.nofailfile` only stops Rapid writing a failure file; Rapid replays every file under `testdata/rapid/<Test>/` on every run; `RAPID_SEED` and `RAPID_NOFAILFILE` set the same values from the environment | `pgregory.net/rapid@v1.3.0` `engine.go:69` ("do not write fail files"), `:280`, `:290-293`, `:329-340`, `:91-93`, `:122-131`; `persist.go:62-66` |
| P7 | PR #48 commits curated failure files under `pkg/types/testdata/rapid/` and asks for `-rapid.nofailfile` | `gh api repos/C360Studio/semengine/contents/pkg/types/testdata/rapid/README.md?ref=a2f975a`, line 8; the file list at `4e44422` still has the `.fail` file |
| P8 | Memory cannot be capped from a script on macOS; Go's memory limit is soft | `/bin/bash -c 'ulimit -v 1000000'`: "cannot modify limit: Invalid argument"; `go doc runtime/debug.SetMemoryLimit`: "a soft memory limit" |
| P9 | `timeout` is not part of base macOS on this host | `command -v timeout`: `/opt/homebrew/bin/timeout`; macOS 26.5.2 |
| P10 | A disposable copy is cheap, and the `contract` and `probe` tests pass in one | 161 files copied in 0.11 s (2.1 MB); `go test -count=1 ./internal/harness/contract/ ./internal/harness/probe/`: 16.2 s in the worktree, 16.5 s in a `git init`-ed copy, both passing |
| P11 | The trial's verdicts, agreement, baselines, after-runs and tree checks are as reported | Per-run lines recounted from every `trial/cases/*/out*/report.txt` |
| P12 | `harness-boundaries` binds `pindiff` to write nothing and to promise only pass or fail through `task` | `spec.md:292`, `:293-294`, `:358-359` (inventory 3.2) |
| P13 | "Review only" means no command fails when the rule is broken; the spec-queue row names an on-demand command under review only | `AGENTS.md:60-61`, `:93` |
| P14 | (Corrected.) The trial's reach runs measured the unchanged code, because Go dropped the overlay under coverage; read in the target's line numbers, R04's and R05's deleted lines lie in executed blocks of those runs | `R04/out/reach-cover.out`: `client.go:586.2,595.28 4 1` holds deleted line 592; `R05/out/reach-cover.out`: `generic_json.go:207.36,208.46 1 12` and `208.46,210.4 1 1` overlap deleted lines 208-210 |
| P16 | Go drops the overlay for a file in a package it builds with coverage, through the flag and through `GOFLAGS`, and the profile has the original's line numbers | Scratch module `arch-cov` (go1.26.6): with `return 10` deleted by overlay, `TestClampBig` fails without coverage (`e_test.go:7: Clamp(20) = 0`, flag and `GOFLAGS`) and passes with `-coverpkg` (`ok ... coverage: 75.0%`) and with `-cover`; the profile block `5.12,8.3` counts 2 statements, the original's. Reproduces the round-1 review's `rv3-ov` (go1.26.6 and go1.26.4). Cause: go1.26.6 `src/cmd/go/internal/work/exec.go:679-683` collects each file's path on disk, and `:2074` passes those paths to the coverage tool with no overlay lookup |
| P17 | A caller's `GOFLAGS` with `-cover` silently undoes the overlay | Same module: `GOFLAGS="-overlay=... -cover"`: `ok ... coverage: 50.0%`; the same without `-cover`: `e_test.go:7: Clamp(20) = 0`, FAIL |
| P18 | A race the race detector finds fails the test with `testing.go:1712: race detected during execution of test` and exit 1 | Scratch module `arch-log`, `TestRace`, go1.26.6, `go test -json -race`; reproduces the review's `rv3-race` |
| P19 | In `go test -json`, a `t.Logf` line and a `t.Errorf` or `t.Fatalf` line are the same kind of event with the same shape | `arch-log`, `TestLog`: `"Action":"output","Test":"TestLog","Output":"    l_test.go:13: checking the value\n"` and the same for `:14` and `:15`; no field tells them apart (go1.26.6) |
| P20 | `git diff --no-index --no-ext-diff --no-textconv --no-color -U0` gives one hunk header per change: `@@ -27 +26,0 @@` for a deleted line 27, `@@ -30,0 +31 @@` for a line inserted after line 30 | Run against `internal/harness/probe/await.go` and copies outside the worktree with `GIT_OPTIONAL_LOCKS=0`; `git status --porcelain` empty afterwards |
| P21 | A coverage profile has no block for a package-level constant; an `if` line's block ends at its `{`, and its branch's block starts there | `arch-cov` unchanged code: blocks `7.23,8.15`, `8.15,11.3`, `12.2,12.14`, `12.14,14.3`, `15.2,15.10`; none overlaps line 4, `const Limit = 10` |
| P22 | A `GOFLAGS` set with `go env -w` is not in the process environment; `go env GOFLAGS` prints it; a process `GOFLAGS` replaces it entirely, so a mutant run with only the overlay in its `GOFLAGS` drops the file's flags | Scratch `arch-goenv`, `GOENV` set to a scratch file, go1.26.6: after `go env -w GOFLAGS=-cover`, the process `GOFLAGS` is empty and `go env GOFLAGS` prints `-cover`; with `GOFLAGS=-overlay=/x.json` it prints `-overlay=/x.json`. In `arch-cov`, the baseline built with the file's `-cover` (`ok ... coverage: 50.0%`) while the run with `GOFLAGS=-overlay=...` built without it (`e_test.go:7: Clamp(20) = 0`). Reproduces the review's `rv4-goenv` |
| P23 | A function's first coverage block begins on its `func` line, and a block counts its entry: it reads executed even when a call in it did not return | Scratch `arch-blk`, go1.26.6: `func F() {` on line 7 gives block `b.go:7.10,10.2 2 1`, counted executed although `F`'s call to `boom()` panicked (recovered by the test) before line 9 ran; `func G() int { return 1 }` on line 12 gives `12.14,12.26`. Reproduces the review's `rv4-blk` |
| P24 | `git diff --no-index` runs a caller's external diff unless told not to | With `GIT_EXTERNAL_DIFF=/usr/bin/false`: exit 128 without `--no-ext-diff`; with `--no-ext-diff --no-textconv`, the hunk header `@@ -27 +26,0 @@` (git 2.50.1; worktree `git status --porcelain` empty afterwards) |
| P25 | An `if` branch's block ends at its closing brace, so the place after its last statement lies inside it; a `case` clause's block ends at its last statement, so the place after it lies in no block, and the next clause's block begins on the next line | Scratch module `arch-r4`, go1.26.6, `TestR` calling `Sign(5)`, `Name(2)`, `Pick(-1)`: `r.go:5.11,7.3 1 0` (the `if` branch), `7.8,9.3 1 1` (the `else`), `17.9,18.12 1 0` (`case 1:`), `19.9,20.12 1 1` (`case 2:`), `21.10,22.13 1 0` (`default:`) |
| P26 | Through a symbolic link, with `PWD` naming the link, an overlay keyed by the resolved path is not applied; with `PWD` set to the resolved path it is | `arch-cov` reached through a symbolic link, go1.26.6, overlay keyed by the resolved path: `ok ... 0.263s` (the original ran); with `PWD=<resolved>`: `e_test.go:7: Clamp(20) = 0` |
| P27 | `git -c diff.interHunkContext=5 diff --no-index -U0` merges two one-line changes three lines apart into one hunk; `--inter-hunk-context=0` keeps them apart | Scratch `arch-hunk`, git 2.50.1: `@@ -2 +2 @@` and `@@ -5 +5 @@` by default; `@@ -2,4 +2,4 @@` with the setting; the two hunks again with `--inter-hunk-context=0` |
| P28 | A multi-line `if` condition lies in the block that ends at the condition's `{`; a label's start is the inclusive end of the block before it | Scratch module `arch-r5`, go1.26.6, `TestS` calling `Pick(5)` and `Lab(-1)`: `s.go:3.22,5.11 1 1` holds `if x > 0 &&` (line 4) and `x < 100 {` (line 5); `16.2,17.1 2 0` ends at `done:` (line 17, column 1), skipped by `goto done`; `18.2,18.10 1 1` is the labeled `return x`. Reproduces the review's `rv8` profiles |
| P29 | A simple labeled statement's label is the inclusive end of the block before it, which `goto` skips; a labeled loop's label lies inside the block that runs up to the loop's `{` | Scratch module `arch-r6`, go1.26.6, `TestL` calling `Lab2(-1)` and `Loop(2)`: `l.go:9.2,10.1 2 0` ends at `done:` (line 10, column 1) and `11.2,11.11 1 1` is the labeled `hits += 2`, reproducing the review's `rv9`; `14.18,17.25 2 1` holds `outer:` (line 16) and the loop header |
| P15 | `scripts/tree-state.sh` prints a fingerprint in a tree whose every directory, `.git` included, is read-only, and leaves `.git/index` unchanged there | Scratch repository `arch-ro`: exit 0, the same fingerprint as before, the same SHA-256 of `.git/index` |

## Not measured

- The time `go run` takes to compile the program before each check.
- Whether running without `-race` would let Go's own stack limit stop R08 before the operating system does.
- The program itself run from a directory reached through a symbolic link. P26 measures `go`'s side; the program's
  test is task 2.11.
- What git's file-system monitor writes under `.git` when a host sets `core.fsmonitor=true` (D15).
- A test of this repository whose path to a changed region depends on scheduling, where D13's reach could differ from
  run to run. None was looked for beyond the trial's survivors, which are consistent (P14).
- Whether `git status` would ever rewrite `.git/index` in this repository without `GIT_OPTIONAL_LOCKS=0`. In the
  scratch repository it did not even after a file's time changed, so no test here can show the variable matters; it
  rests on git's documentation.
- That after a SIGKILL to the program, its `go test` runs on until its own `-timeout`. It follows from the process
  group the program gives each run, but it was not measured.
- Behaviour under PR #48's packages beyond the trial's 12 cases.
