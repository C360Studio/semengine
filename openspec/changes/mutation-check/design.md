# Design: mutation-check

Status: **revision 1, for independent pre-owner design review.** It rests on `inventory.md` in this directory:
revision 2, which has `INVENTORY PASS` in round 2 (PR #82, comment 5981915103), sha256
`774646d7ab3704c2a3498148fe414bd9149d388f6ed1aa1238d6d97965a62ed9`. That file is the accepted inventory and is not
repeated here. "Inventory 2c" means its section 2c, and "Q9" means its open question 9. Round 2's four NITs are
answered under "Inventory errata" below; the inventory's text is unchanged.

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
- **Noise came from the inputs, not from overlay.** On the first try, the prototype's verdict matched the trial
  agent's reading of the page in 10 of 12 recorded cases. Both misses were inputs: R02's record names no assertion,
  and in R11 the line named was inside a helper, so Go printed another. Given the lines Go prints, all 12 matched. That
  reading is not independent, because one agent wrote both; where the page's text alone decides (R06, a whole-run
  timeout, and R08, a killed process), both pages say inconclusive, as the prototype did. Only 3 of the 12 records
  named the assertion at all. The planted controls P1-P7 got the right verdict. The Rapid control P8 got a false
  detection from a failure file an earlier run had left, until every run set `-rapid.nofailfile` (D7).

So for Go source, overlay is the way to make the wrong change: it writes nothing, it survives an interrupt or a kill
because there is nothing to restore, and in the trial it gave the same answer every time. It is not a way to check a
change to a script.

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
present consumer; it is Q3 for the owner, recommended "not now".

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
- **The expected assertion** is required: at least one `-expect name_test.go:N`, the location as Go prints it at the
  start of a failure line, or one `-expect-text`, a fixed string the failure prints.

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
Because #79 names a script, this is Q4 for the owner.

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
| The test passed and `go test` exited zero | Baseline: pass. Mutant: survivor, subject to the reach check |
| The test passed and `go test` exited non-zero | Inconclusive |
| The test failed with a failure line at an expected location or containing an expected text | Detection; later panics and other failure lines are recorded as notes (R09, R12) |
| The test failed otherwise | Inconclusive: the report gives the locations that failed |

The final verdict is the spec's: inconclusive unless every baseline run passed, the mutant runs agree, the after-run
passed and the fingerprint did not change. A survivor needs a coverage run showing a statement on the changed lines
executed; without it the verdict is invalid. The trial's reach lines show this working for R04, R05 and P2.

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

Three baseline runs, three mutant runs and one after-run by default, with `-runs N` to change the first two counts.
A baseline failure stops the check; a mutant run ended by the timeout or a signal is the last one. That would have
saved two of R06's 60-second runs and two of R08's 82-99-second runs. Three baseline runs catch only a frequent flake:
P7 failed once in three. A rarer flake can pass all three, and the command does not read GitHub's `class:flake`
issues. Both are declared.

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
replayed in the baseline and mutant runs alike. Native fuzz targets need nothing: plain `go test` replays only their
seeds. The test of this behaviour needs a Rapid property, and Rapid enters `go.mod` with PR #48, so that task waits
for PR #48 (the owner's sequencing).

### D8 Interrupts, kills, runaway memory

- **SIGINT or SIGTERM** to the program: it stops the running `go test` and everything it started, exits non-zero and
  prints no verdict. Nothing in the tree needs restoring.
- **SIGKILL** to the program: the `go test` it started runs on until its own `-timeout`, then exits. Nothing in the
  repository changes. The temporary directory with the logs and the overlay file stays behind outside the
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
which come from the page's text, not from the program.

1. **The classifier**, on recorded event streams. `go test -json` output is captured once from go1.26.6 for each
   per-run case in D5 and kept under the program's `testdata/`. Each case's expected reading is written in the test.
   *Generated checks:* the per-run reading is a function of about seven inputs (build, selection, timeout, signal,
   result, exit status, an expected hit). The decision is an exhaustive enumeration of that small domain, checked
   against a reference table written in the test from the spec's rules. No library is needed, and every combination
   runs. The aggregate rules (agreement, early stops, after-run, fingerprint, reach) get named examples, one per
   scenario.
2. **End to end, with the real toolchain**, on planted modules in temporary directories, as `pindiff`'s
   `TestCommandExitStatus` does (inventory 5 (iii); Q6: "never invoke the real go" is a comment in one package, not a
   rule). Only the cases that need real `go`, with `-runs 1` where repetition is not under test (tasks 2.3 and 2.4):
   detection; survivor reached; survivor not reached (invalid); a wrong change that does not build (invalid); a child
   `go build` sees the wrong change; every run's environment carries the seed and `RAPID_NOFAILFILE=true`; a test that
   writes into its package directory makes the check inconclusive; the tree read-only throughout and compared by
   SHA-256, as `TestWritesNothingInTree` does; and SIGTERM while the test waits leaves no process running, as
   `TestFetchLeavesNoProcess` does. Each planted module is a git repository with a copy of `scripts/tree-state.sh`.
3. **Refusals**, each scenario of "The command and its inputs".
4. **Rapid** (held on #48): a planted property in a temporary module that resolves Rapid from the module cache with
   `GOPROXY=off`: detection for a seed that finds the wrong change, survivor for one that does not, and no file under
   `testdata/rapid/` afterwards.

Planted test sources inside Go test files split the literals the text guards match, as `testtext_test.go:14` does.
A blocking planted test waits on a channel, never on `select {}` (PR #48's new rule) or a sleep.

**Shown able to fail.** Each of these wrong changes to the program must be detected by its own tests, run as the
page asks and recorded on the pull request: a non-zero exit read as detection; a survivor without the reach check;
the fingerprint compare dropped; a timeout read as detection; a failing baseline ignored; a location outside the
expected set accepted; the overlay passed as a flag instead of through `GOFLAGS` (the child build then sees the
original); exit zero for a survivor; and `RAPID_NOFAILFILE` not set (the planted test that prints its environment, in
task 2.3, sees it; no Rapid property is needed). The command can run most of these checks on its own package.

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

## Ordering against #48, #73, #60 and #80

- **#48, then #73:** ruled by the owner (#79 comment 5981621215). The document tasks hold on both, the Rapid test on
  #48, and PR #82 lands after both. PR #48 was at `4e44422` when this design was written. Its 279-file list and
  its hunk headers for `AGENTS.md`, `docs/testing.md`, `docs/provenance.md`, `docs/repository-map.md` and the three
  contracts are the same as at `a2f975a`, the head inventory review round 2 checked (read through the pulls API).
- **#60** (a command that writes a ported package's files into the tree): no order is needed. This change writes
  nothing in the repository, so it adds no exception to `pindiff`'s write-nothing requirement or to `AGENTS.md:32`.
  It touches none of #60's files: its spec delta is a new capability, and #60's question is about `harness-boundaries`
  and `pindiff`. #60 stays the only proposed writer.
- **#80** (tests name the requirement they prove): the owner chose its Option A (#80 comment 5981307449): the guard
  covers product packages only, so this change's tests, under `internal/harness/`, are outside it. Both changes edit
  `docs/testing.md`, in different sections, and both add to the `AGENTS.md` rules table, in different rows. Neither
  depends on the other. Whichever merges second merges `main` into its branch and keeps both texts. The ordering is not
  ruled; this is the design's reading, and the owner may rule otherwise.

## What the command does not see

- Whether a pull request used it at all. Only a check that reads the pull request could see that (Q1).
- Whether the wrong change is plausible, and whether the named assertion is the intended one (D2).
- A flake rarer than one failure in three baseline runs (D6).
- A wrong change in a file the test reads while it runs (D1, D9).
- Memory: a runaway mutant is stopped only by the operating system or the timeout (D8).
- Reach inside a child process. A wrong change that only a child process runs (a command the test builds, or the test
  binary run again) carries no coverage, so if it survives, the verdict is invalid, not survivor. A detection there is
  unaffected (R09, R11).

## Invariants and their spec homes

| Invariant | Spec home |
| --- | --- |
| Exit status zero if and only if the verdict is detection | "Report and exit status", first two scenarios |
| No file in the repository is written, created or removed, whatever ends the program | "Nothing inside the repository is written", "A read-only tree" and "Interrupted" |
| A failing exit status alone never gives detection | "Outcome classification", "A failure that names no expected location" |
| A survivor is never reported without coverage showing the changed lines executed | "Outcome classification", "A survivor that was not reached" |
| Every run of one check uses the same seed | "The runs", "The seed reaches every run" |
| A whole-run timeout is never a detection | "Outcome classification", "Whole-run timeout" |

## What a session has to know after this change

The inventory (section 7) listed ten facts a session must hold to run the experiment by hand. With the command:

1. Make the wrong change in a copy of the file, outside the repository.
2. Name the assertion you expect to fail, as Go prints it or by its message.
3. The command handles a non-test Go source file in a unit-tested package. For anything else it refuses and says
   so, and the page's manual procedure applies.

Everything else (seed, failure files, timeout, `-v` and the name check, the exit-status trap, restore) is the
command's. Three facts is one more than the architect contract's limit of two, so it is a design finding, recorded
here. What happens if a session knows none of them: each one is checked before any run, and the session gets a refusal
that names the flag or the file and, for the third, the manual procedure. No path ends in a wrong verdict. The gap to
"nothing": the second fact is the page's own rule, because only the implementer can say which assertion the wrong
change is meant to trip; the first and third follow from overlay, and Q3 is the choice that would remove the third.

## Declared costs

- **Test time.** The end-to-end tests (tasks 2.3 and 2.4, nine cases) run the real `go test` about four times per
  case, once under `task test:unit` and five more times under `task test:repeat`. A warm `go test -race -json` of a
  planted one-file module took 0.3-0.4 s here, with or without the overlay and with `-coverpkg`; the first runs after
  a toolchain switch took 1.3-3.2 s (scratch module `arch-ov2`, go1.26.6). That gives about 15 s per pass and about
  90 s summed over the six passes, before packages running side by side shorten the wall time. `task verify` takes
  about 3.3 minutes on CI today, against a pinned 15-minute limit (inventory 3.2). The design proposes a budget of
  60 s of added CI time, measured in task 2.7. Over budget, the end-to-end set shrinks to the four cases only real
  `go` can show (the read-only tree with a detection, the child build, a survivor's reach, and SIGTERM), and the rest
  move to recorded event streams.
- **Run time of a check.** With the defaults a check is seven runs plus one coverage run for a survivor: 6-43 s for
  the trial's cases other than R06 and R08, about 60 s plus the baselines for a whole-run timeout of 60 s.
- **The program itself:** about the prototype's size (500 lines), plus tests. Each call compiles the program first.
- **Owner time:** four questions below.

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

Each is in plain words: the recommendation first, then the reasons, then what the other answer costs.

**Q1. What should the `AGENTS.md` row for mutation outcomes say once the command exists?**

The row is `AGENTS.md:99`. Recommendation: keep it "review only", and name the command the way the spec-queue row names
`task spec:queue` (`AGENTS.md:93`): "review only; `task mutate:check` classifies a wrong change's runs by rule and
prints the record a pull request quotes; it fails nothing when it is not used." Reasons: the table defines "review only"
as "no command fails when the rule is broken" (`AGENTS.md:60-61`). This command fails nothing when a pull request skips
it or writes its verdict by hand, which is how the rule gets broken. The row you would copy for the split, the ledger
row (`:77`), is enforced by a command that always runs in `task verify`; its on-demand half, `task ledger:diff`, is the
part it calls review only.

- The orchestrator recommends splitting the row: "when used, the command does the classification, the seed replay
  and the run that writes nothing; whether a pull request used it, and whether the mutant and the named assertion are
  the right ones, stay review only." Cost: it puts an optional command in the "Enforced by" column. A reader could
  take the rule as backed by a command when nothing fails if it is skipped, which is the drift this table exists to
  show.
- Calling it fully enforced: contradicts the table's own definition.
- A later, separate change to `merge-check`, failing a code pull request that claims a detection without the command's
  report block. This is the only option that makes the rule fail when broken. Cost: a new marker in pull request text,
  another `merge-check` rule with its tests, and false failures on pull requests where no mutation check applies. Worth
  ordering only if hand-written detection claims keep slipping past review once the command exists.

**Q2. Should a test built to fail only by `go test`'s timeout ever count as a detection?**

Recommendation: no. A whole-run timeout stays inconclusive, as both our page (`docs/testing.md:133`) and the pin's
(`01-testing.md:216-218`) say. When a detection is needed from such a test, the test gets its own bound. Reasons:
a timeout says the run never finished, not which assertion caught the wrong change. The trial's R06 (PR #48's
`LatestNeverWaitsOnCollection`) is this case, and PR #48's records count it as detected (inventory 2b).
`probe_test.go:26-27` bounds 8 call sites only by the binary's timeout. Cost of "yes" (a flag that lets a timeout
count when the named test is the one running): it readmits the class both pages reject, and every report would need
reading to see whether the flag was used. Cost of "no" to you: R06-like mutants stay inconclusive until someone adds a
bound to the test. PR #48's own record is that pull request's to correct.

**Q3. Should the command also check wrong changes to scripts and other files a test reads while it runs?**

Recommendation: not now. They stay with the manual procedure, and the command says so when given one. Reasons:
overlay cannot reach them, so it needs the second mode O1c, a disposable copy of the tree. A survivor in a script
cannot be shown reached, because Go's coverage does not cover scripts, so script survivors would need their own rule.
Cost of "not now": the next change to a script guard (`merge-check.sh`, `cover-check.sh`, `cleanup-roots-check.sh`)
is checked by hand, as `review-gate-check`'s seven were. Cost of "yes": the copy mode, its tests and a rule for
unprovable survivors in this change. Copying itself is cheap (P10).

**Q4. A Go program instead of the shell script #79 names?**

Recommendation: yes. `internal/harness/mutcheck`, behind the same `task mutate:check`. Reasons (D4): the command
reads `go test`'s JSON events and a coverage profile, bounds each run and stops its process group, and handles
signals. bash 3.2 does the last two poorly: a measured SIGTERM waited for the child, and macOS has no `timeout`
command without Homebrew. `pindiff` is the precedent: an on-demand harness program tested end to end against the
real toolchain. Cost: #79's acceptance changes from a script and a bash fixture test to a program and Go tests, and
each call compiles the program first. Cost of "no": `jq` and a Homebrew `timeout` become dependencies that
`scripts/doctor.sh` does not check, and signal handling rests on traps that the inventory measured failing.

## Owner's rulings

None yet. The four questions above go to the owner on #79 after the pre-owner design review.

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
| P14 | The prototype's reach check showed the changed lines executed for every survivor in the trial | `R04`, `R05`, `P2` `report.txt` `reach` lines |
| P15 | `scripts/tree-state.sh` prints a fingerprint in a tree whose every directory, `.git` included, is read-only, and leaves `.git/index` unchanged there | Scratch repository `arch-ro`: exit 0, the same fingerprint as before, the same SHA-256 of `.git/index` |

## Not measured

- The test time the end-to-end tests add (budget proposed; task 2.7).
- The time `go run` takes to compile the program before each check.
- Whether running without `-race` would let Go's own stack limit stop R08 before the operating system does.
- Coverage of an overlaid statement when the overlay comes through `GOFLAGS`. `GOFLAGS=-overlay` with `-json`,
  `-race`, `-cpu 1` and `-coverpkg` builds and runs the overlaid file (measured in `arch-ov2`), but that module has no
  statement to cover; the trial's reach check used the flag. Task 2.3's end-to-end survivor case covers it.
- Whether `git status` would ever rewrite `.git/index` in this repository without `GIT_OPTIONAL_LOCKS=0`. In the
  scratch repository it did not even after a file's time changed, so no test here can show the variable matters; it
  rests on git's documentation.
- That after a SIGKILL to the program, its `go test` runs on until its own `-timeout`. It follows from the process
  group the program gives each run, but it was not measured.
- Behaviour under PR #48's packages beyond the trial's 12 cases.
