# mutation-check

## Purpose

The mutation check is the on-demand command that runs the experiment of `docs/testing.md`, "Show that the test can
fail", for one wrong change to one Go source file, and classifies the outcome by rule instead of by hand. The command
itself writes nothing inside the repository. It is run by hand; it is not a step of `task verify` or of CI.

## ADDED Requirements

### Requirement: The command and its inputs

`task mutate:check -- <flags>` SHALL run the program `internal/harness/mutcheck` from the repository's root. The
program SHALL take these flags:

- `-pkg`: one package path. A pattern containing `...` SHALL be refused.
- `-test`: the name of a top-level test, or of a top-level test and one of its subtests written `Name/Sub`, each part
  matched exactly.
- `-file`: the file the wrong change is made to (the target).
- `-mutant`: the path of a copy of the target with the wrong change made in it.
- `-expect`: a location `file.go:N` as Go prints it at the start of an output line of the named test. The file may be
  a test file, a non-test file such as a helper in another package, or a file of Go's `testing` package. `-expect-text`:
  a fixed string that an output line of the named test contains. At least one of the two SHALL be given; each may be
  given more than once.
- `-seed` (default 1), `-timeout` (the `go test` timeout of each run, default two minutes) and `-runs` (the number of
  baseline runs and of mutant runs, default 3, at least 1).

Before it starts any run the program SHALL refuse, exiting non-zero and naming the input and the reason, when the target
does not exist, is outside the module, is not a Go source file, or is a `_test.go` file; when the mutant is inside the
module or has the same content as the target; when neither `-expect` nor `-expect-text` is given; when an `-expect`
value is not `file.go:N` with N a positive whole number, or an `-expect-text` value is empty; when there is no `go.mod`
at the repository's root; when the directory for temporary files (`TMPDIR`) lies inside the module, so that the
program's own files would land in the repository; when `-seed` is 0,
which Rapid takes as "choose a random seed"; when the caller's `GOFLAGS` sets `-overlay`, or sets `-cover`,
`-coverpkg`, `-covermode` or `-coverprofile`, under which Go builds the target from the file on disk and the wrong
change would not run; or when a file that git does not track exists under the package's `testdata/rapid/` directory. A
refusal because of the kind of target SHALL say that the manual procedure of `docs/testing.md` applies to it. The
caller's `GOFLAGS` is what `go env GOFLAGS` prints in the program's environment, so a value set with `go env -w`
counts.

#### Scenario: A script as the target

- **WHEN** `-file` names `scripts/merge-check.sh`
- **THEN** the program starts no run, exits non-zero, says that only a Go source file that is not a test file can be
  the target, and names the manual procedure of `docs/testing.md`

#### Scenario: A test file as the target

- **WHEN** `-file` names a `_test.go` file
- **THEN** the program starts no run, exits non-zero, and says that the test, its inputs and its expectations stay
  unchanged

#### Scenario: No expected assertion

- **WHEN** neither `-expect` nor `-expect-text` is given
- **THEN** the program starts no run, exits non-zero, and names both flags

#### Scenario: A package pattern

- **WHEN** `-pkg` is `./...`
- **THEN** the program starts no run and exits non-zero, saying that one package is named

#### Scenario: A malformed expected location

- **WHEN** `-expect` is `probe_test.go` with no line number
- **THEN** the program starts no run, exits non-zero, and names the value and the form `file.go:N`

#### Scenario: An empty expected text

- **WHEN** `-expect-text` is given an empty string
- **THEN** the program starts no run and exits non-zero, naming the flag

#### Scenario: No go.mod at the root

- **WHEN** the directory the program runs from has no `go.mod`
- **THEN** the program starts no run and exits non-zero, saying it must run from the repository's root

#### Scenario: Temporary files inside the module

- **WHEN** `TMPDIR` names a directory inside the module
- **THEN** the program starts no run, exits non-zero, and says that its files would be written into the repository

#### Scenario: Coverage in GOFLAGS

- **WHEN** `GOFLAGS` was set to `-cover` with `go env -w`, and the process environment has no `GOFLAGS`
- **THEN** the program starts no run, exits non-zero, and says that under coverage Go would build the target from the
  file on disk

#### Scenario: A failure file left by an earlier run

- **WHEN** a file that git does not track exists under the package's `testdata/rapid/` directory
- **THEN** the program starts no run, exits non-zero, and names the file

#### Scenario: A failure file that is committed

- **WHEN** the only files under the package's `testdata/rapid/` directory are tracked by git
- **THEN** the program does not refuse on their account

### Requirement: The program writes nothing inside the repository

The program itself SHALL NOT write, create or remove any file inside the repository, however it ends, SIGKILL
included. The changed copy, the logs and every file the program makes SHALL be outside the repository. Reading the
tree's state SHALL NOT refresh git's index. Every Go build that a mutant run makes SHALL see the wrong change: the test
binary, and a `go build` or `go run` that the test itself starts; this holds also when the repository is reached
through a symbolic link. A file that the test reads while it runs is not
replaced; that is why a target that is not Go source is refused.

The program SHALL take the fingerprint of the tree that `scripts/tree-state.sh` prints before its first run and after
its last run. When the two differ, whoever changed the tree, the verdict SHALL be inconclusive and the report SHALL say
that the tree changed during the check.

On SIGINT or SIGTERM the program SHALL stop the process group of the run in progress before it exits, SHALL exit
non-zero, and SHALL print no verdict. Every run is started in a process group of its own; a process that the run starts
in yet another process group is not stopped.

#### Scenario: A read-only tree

- **WHEN** the program checks a wrong change that is detected, against a planted module whose every directory is
  read-only
- **THEN** the check reaches its verdict, and the content hash of every file in the module is the same afterwards

#### Scenario: A build the test starts

- **WHEN** the named test builds a command of the same module with `go build` and runs it, and the wrong change is
  in that command's source
- **THEN** the command the test built shows the wrong change

#### Scenario: A repository reached through a symbolic link

- **WHEN** the program runs in a directory reached through a symbolic link to the repository, with `PWD` naming the
  link, and the wrong change is detected when the repository is reached directly
- **THEN** the verdict is detection

#### Scenario: Interrupted

- **WHEN** the program receives SIGTERM while the test of a run waits, and the run has started a further process in
  its process group
- **THEN** no process of the run's process group is running when the program exits, it exits non-zero, it prints no
  verdict, and the tree is unchanged

#### Scenario: The tree changes during the check

- **WHEN** the named test writes a file into its package directory that git does not ignore
- **THEN** the verdict is inconclusive and the report says the tree changed during the check

### Requirement: The runs

Every run of one check SHALL use the same command line and environment, apart from what this requirement adds for a
mutant run and for the reach run. That command line is `go test -json -count=1 -cpu 1 -race -timeout <timeout> -run
<the name, anchored> <pkg>`. The environment sets `GOFLAGS` explicitly to the caller's `GOFLAGS`, read once, sets
`RAPID_SEED` to the seed, and sets `RAPID_NOFAILFILE` to `true`. A mutant run appends the overlay that maps the target
to the mutant to `GOFLAGS`. The reach run adds coverage of the target's package, and has no overlay.

The program SHALL make, in this order: `-runs` baseline runs on the unchanged code; `-runs` mutant runs with the wrong
change; one after-run on the unchanged code; and, only when every mutant run passed the named test, one reach run on
the unchanged code. A baseline run that does not pass SHALL end the check before any mutant run. A mutant run that ends
by the timeout, by a signal or by the bound below SHALL be the last mutant run. A run that has not ended by twice its
timeout SHALL be stopped, with its process group; the report states that bound.

#### Scenario: A baseline run fails

- **WHEN** the second of three baseline runs fails
- **THEN** no mutant run is made, and the verdict is inconclusive and names that run

#### Scenario: A mutant run times out

- **WHEN** the first mutant run ends by the timeout
- **THEN** no second mutant run is made and the verdict is inconclusive

#### Scenario: A run that does not end

- **WHEN** the `go` command of a run does not exit by twice the timeout
- **THEN** the run is stopped with its process group, and the verdict is inconclusive and names the bound

#### Scenario: The environment is the same in every run

- **WHEN** `-seed 7` is given and the caller's `GOFLAGS` is `-tags=planted`
- **THEN** every run's environment has `RAPID_SEED=7`, `RAPID_NOFAILFILE=true` and `-tags=planted` in `GOFLAGS`, and
  only the mutant runs' `GOFLAGS` also carries the overlay

#### Scenario: Flags set with go env -w

- **WHEN** `GOFLAGS` was set to `-tags=planted` with `go env -w`, and the process environment has no `GOFLAGS`
- **THEN** every run's `GOFLAGS` has `-tags=planted`, the mutant runs' with the overlay appended

#### Scenario: A property test

- **WHEN** the named test is a Rapid property that fails on the wrong change for the given seed
- **THEN** the verdict is detection, and no file is written under the package's `testdata/rapid/` directory

#### Scenario: A property test whose seed misses the wrong change

- **WHEN** the named test is a Rapid property that passes on the wrong change for the given seed
- **THEN** the verdict is survivor, and its reason says that the result holds for that seed

### Requirement: The changed region and reach

The program SHALL take the wrong change's hunks from a line diff of the target against the mutant with no context lines,
which no external diff program, text conversion or hunk-merging setting that the caller configures can change. A hunk
that removes or replaces lines has those lines of the target as its region. A hunk that only inserts lines after target
line k has, as its region, the place between lines k and k+1; when that place lies outside every function body of the
target, the region is not measurable. A mutant with more than one hunk SHALL be accepted, and the report SHALL list
every hunk and say that there is more than one.

The reach run measures the unchanged code; Go builds a file it covers from the file on disk, so an overlay would not
apply in that run anyway. A region of removed or replaced lines is reached when the reach run's profile shows an
executed block that overlaps those lines. An insertion inside a function body belongs to the innermost statement list
that holds its place, as Go's parser reads the target: a function body, a block, a branch of an `if`, or a clause of a
`switch` or `select`. It is reached when the block holding the first statement of that list after the place executed;
when no statement of that list follows the place, when the block holding the last statement before it executed. A block
of a sibling branch or clause never decides it. A region is not measurable when no block decides it: no block overlaps
the lines, the list has no statement, or no block holds the statement that would decide. The wrong change is reached
when any of its regions is reached; not reached when every region is measurable and none is reached; and not measurable
otherwise.

#### Scenario: A deletion that was reached

- **WHEN** the wrong change deletes a line that the reach run shows executed, and every mutant run passes
- **THEN** the verdict is survivor

#### Scenario: A deletion in a branch that did not run

- **WHEN** the wrong change deletes a line in a branch that the reach run shows did not execute, and every mutant run
  passes
- **THEN** the verdict is invalid, and says the test did not reach the wrong change

#### Scenario: Regions are read in the target's line numbers

- **WHEN** the wrong change deletes three lines that executed, and the lines with the same numbers in the mutant lie in
  a block of the target that did not execute
- **THEN** the wrong change is reached

#### Scenario: An insertion in a branch that did not run

- **WHEN** the wrong change only inserts a line into a branch that the reach run shows did not execute, and every
  mutant run passes
- **THEN** the verdict is invalid

#### Scenario: An insertion at the end of an if-branch whose else ran

- **WHEN** the wrong change only inserts a line after the last statement of an `if` branch that did not execute, the
  `else` branch that begins on the next line executed, and every mutant run passes
- **THEN** the wrong change is not reached, and the verdict is invalid

#### Scenario: An insertion at the end of a switch case whose next case ran

- **WHEN** the wrong change only inserts a line after the last statement of a `case` clause that did not execute, the
  next `case` clause executed, and every mutant run passes
- **THEN** the wrong change is not reached, and the verdict is invalid

#### Scenario: Hunks are not merged

- **WHEN** the caller's git configuration sets `diff.interHunkContext` to 5, and the wrong change replaces two lines
  three lines apart
- **THEN** the report lists two hunks, and the unchanged lines between them are in neither region

#### Scenario: A method added before a function that did not run

- **WHEN** the wrong change only inserts a new method between two top-level declarations, the function right after it
  did not execute, and every mutant run passes
- **THEN** the verdict is survivor, and the report says reach could not be measured

#### Scenario: Two hunks

- **WHEN** the wrong change has two hunks, an import removed and a call removed, and the call's line executed
- **THEN** the wrong change is reached, and the report lists both hunks and says there is more than one

#### Scenario: A change to a declaration

- **WHEN** the wrong change only changes a package-level constant, and every mutant run passes
- **THEN** the verdict is survivor, and the report says reach could not be measured

### Requirement: Outcome classification

The program SHALL give exactly one verdict: detection, survivor, invalid or inconclusive. An expected line is an output
line of the named test or of its subtests that begins with an expected location or contains an expected text.

- **Detection:** every baseline run passed; every mutant run failed the named test and printed at least one expected
  line; the after-run passed; and the tree did not change.
- **Survivor:** every baseline run passed; every mutant run passed the named test and exited zero; the after-run passed;
  the tree did not change; the reach run passed; and the wrong change is reached or not measurable.
- **Invalid:** every baseline run passed, the after-run passed and the tree did not change; and either the wrong
  change does not build, or every other condition for survivor holds and the wrong change is not reached.
- **Inconclusive:** every other case, including a baseline run that did not pass, a run that selected no test, a run
  that ended by the timeout (even when an expected line was printed before it), a run ended by a signal, a run stopped
  by the bound, a failure with no expected line, mutant runs that disagree, an after-run or a reach run that did not
  pass, and a tree that changed.

A failing exit status alone SHALL never give detection. A report of the race detector counts as an expected line only
when an expected location or text matches it. A panic or a further failure line printed after an expected line SHALL be
recorded in the report and SHALL NOT change a detection. Go prints a log line of a test the same way as a failure line,
so the report SHALL print every expected line in full. No verdict SHALL be named "equivalent": whether a survivor is
equivalent is a reviewer's assessment.

#### Scenario: Detected

- **WHEN** each mutant run fails the named test and prints a line at the expected location
- **THEN** the verdict is detection, and the report prints that line

#### Scenario: A different assertion fails

- **WHEN** each mutant run fails the named test only at a location that is not expected
- **THEN** the verdict is inconclusive, and the report gives the locations that failed

#### Scenario: Whole-run timeout

- **WHEN** a mutant run ends with `go test`'s timeout panic
- **THEN** the verdict is inconclusive

#### Scenario: Killed by a signal

- **WHEN** a mutant run's process is killed by a signal before the named test reports a result
- **THEN** the verdict is inconclusive, and its reason names the signal

#### Scenario: A race report that is not expected

- **WHEN** each mutant run fails only with `race detected during execution of test` and no expected text or location
  matches that line
- **THEN** the verdict is inconclusive

#### Scenario: A race report that is expected

- **WHEN** each mutant run fails with `race detected during execution of test` and `-expect-text` names that text
- **THEN** the verdict is detection, and the report prints the line

#### Scenario: Nothing selected

- **WHEN** `-test` names no test of the package
- **THEN** the verdict is inconclusive and says the name selected no test, and that a test in a file built only with
  the `integration` tag is not run by this command

#### Scenario: The wrong change does not build

- **WHEN** the mutant does not compile
- **THEN** the verdict is invalid

#### Scenario: The build breaks after the baselines

- **WHEN** every baseline run passed, and the mutant runs and the after-run all fail to build
- **THEN** the verdict is inconclusive, not invalid

#### Scenario: A flaky baseline

- **WHEN** one of three baseline runs fails and the other two pass
- **THEN** the verdict is inconclusive

#### Scenario: Mutant runs disagree

- **WHEN** one mutant run is detected and another passes
- **THEN** the verdict is inconclusive

#### Scenario: A failure that names no expected location

- **WHEN** each mutant run exits non-zero with no expected line
- **THEN** the verdict is not detection

#### Scenario: A panic after the expected failure

- **WHEN** each mutant run prints an expected line and then panics
- **THEN** the verdict is detection, and the report records the panic

### Requirement: Report and exit status

The program SHALL print a report block that a pull request can quote. It SHALL contain: the command as given; the
commit, the tree's fingerprint, and whether the tree has uncommitted changes; the Go version; the package and the test;
the target's path and its SHA-256 before and after; the wrong change as a unified diff from the target to the mutant,
with its hunks and their regions; the expected locations and texts; the seed; each run's command line, `GOFLAGS`,
result, expected lines, failure locations, wall time and verdict; for a reach run, whether the wrong change was
reached, not reached or not measurable, with the blocks that decided it; the directory holding the logs, said to be on
this machine only; and, as its last line, the verdict and its reason. A survivor's reason SHALL say that, for a test
that generates its inputs, the result holds for the given seed. The program SHALL exit zero only for detection.
Through `task`, only pass or fail and the report are promised.

#### Scenario: A detection passes

- **WHEN** the verdict is detection
- **THEN** the program exits zero and the last line of its output names the verdict

#### Scenario: A survivor fails

- **WHEN** the verdict is survivor
- **THEN** the program exits non-zero and the last line of its output names the verdict, its reason and the seed

#### Scenario: Logs are named as local

- **WHEN** any verdict is reached
- **THEN** the report names the directory holding the logs and says it is on this machine only
