# mutation-check

## Purpose

The mutation check is the on-demand command that runs the experiment of `docs/testing.md`, "Show that the test can
fail", for one wrong change to one Go source file, and classifies the outcome by rule instead of by hand. It writes
nothing inside the repository. It is run by hand; it is not a step of `task verify` or of CI.

## ADDED Requirements

### Requirement: The command and its inputs

`task mutate:check -- <flags>` SHALL run the program `internal/harness/mutcheck` from the repository's root. The
program SHALL take these flags:

- `-pkg`: one package path. A pattern containing `...` SHALL be refused.
- `-test`: the name of a top-level test, or of a top-level test and one of its subtests written `Name/Sub`, each part
  matched exactly.
- `-file`: the file the wrong change is made to.
- `-mutant`: the path of a copy of that file with the wrong change made in it.
- `-expect`: a location written as Go prints it at the start of a failure line, `name_test.go:N`; and `-expect-text`:
  a fixed string that a failure of the named test prints. At least one of the two SHALL be given; each may be given
  more than once.
- `-seed` (default 1, never 0, which Rapid takes as "choose a random seed"), `-timeout` (the `go test` timeout of
  each run, default two minutes) and `-runs` (the number of baseline runs and of mutant runs, default 3, at least 1).

Before it starts any run the program SHALL refuse, exiting non-zero and naming the input and the reason, when the target
does not exist, is outside the module, is not a Go source file, or is a `_test.go` file; when the mutant is inside the
module or has the same content as the target; when neither `-expect` nor `-expect-text` is given; when `-seed` is 0;
when `GOFLAGS` in its environment already sets `-overlay`; or when a file that git does not track exists under the
package's `testdata/rapid/` directory. A refusal because of the kind of target SHALL say that the manual procedure of
`docs/testing.md` applies to it.

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

#### Scenario: A failure file left by an earlier run

- **WHEN** a file that git does not track exists under the package's `testdata/rapid/` directory
- **THEN** the program starts no run, exits non-zero, and names the file

#### Scenario: A failure file that is committed

- **WHEN** the only files under the package's `testdata/rapid/` directory are tracked by git
- **THEN** the program does not refuse on their account

### Requirement: Nothing inside the repository is written

The program SHALL apply the wrong change without writing, creating or removing any file inside the repository. The
changed copy, the logs and every file the program makes SHALL be outside it. Every Go build that a run of the named test
makes SHALL see the wrong change: the test binary, and a `go build` or `go run` that the test itself starts. A file that
the test reads while it runs is not replaced; that is why a target that is not Go source is refused.

The program SHALL take the fingerprint of the tree that `scripts/tree-state.sh` prints before its first run and
after its last run. When the two differ, the verdict SHALL be inconclusive and the report SHALL say that the tree
changed during the check.

On SIGINT or SIGTERM the program SHALL stop every process it started before it exits, SHALL exit non-zero, and SHALL
print no verdict. However the program ends, SIGKILL included, every file in the repository SHALL be as it was before
the program started.

#### Scenario: A read-only tree

- **WHEN** the program checks one wrong change that is detected and one that survives, against a planted module
  whose every directory is read-only
- **THEN** both checks reach their verdict, and the content hash of every file in the module is the same afterwards

#### Scenario: A build the test starts

- **WHEN** the named test builds a command of the same module with `go build` and runs it, and the wrong change is
  in that command's source
- **THEN** the command the test built shows the wrong change

#### Scenario: Interrupted

- **WHEN** the program receives SIGTERM while the test of a run is blocked
- **THEN** no process the program started is running when it exits, it exits non-zero, it prints no verdict, and
  the tree is unchanged

#### Scenario: The tree changes during the check

- **WHEN** the named test writes a file into its package directory that git does not ignore
- **THEN** the verdict is inconclusive and the report says the tree changed during the check

### Requirement: The runs

Each run SHALL be `go test -json -count=1 -cpu 1 -race -timeout <timeout> -run <the name, anchored> <pkg>`, with
`RAPID_SEED` set to the seed and `RAPID_NOFAILFILE` set to `true` in its environment. The program SHALL make, in this
order, `-runs` baseline runs on the unchanged tree, `-runs` mutant runs with the wrong change, and one after-run on
the unchanged tree. A baseline run that does not pass SHALL end the check before any mutant run. A mutant run that
ends by the timeout or by a signal SHALL be the last mutant run. Each run SHALL end, with every process it started,
within its timeout plus a build allowance that the report states.

#### Scenario: A baseline run fails

- **WHEN** the second of three baseline runs fails
- **THEN** no mutant run is made, and the verdict is inconclusive and names that run

#### Scenario: A mutant run times out

- **WHEN** the first mutant run ends by the timeout
- **THEN** no second mutant run is made and the verdict is inconclusive

#### Scenario: The seed reaches every run

- **WHEN** `-seed 7` is given
- **THEN** every run's environment has `RAPID_SEED=7` and `RAPID_NOFAILFILE=true`

#### Scenario: A property test

- **WHEN** the named test is a Rapid property that fails on the wrong change for the given seed
- **THEN** the verdict is detection, and no file is written under the package's `testdata/rapid/` directory

#### Scenario: A property test whose seed misses the wrong change

- **WHEN** the named test is a Rapid property that passes on the wrong change for the given seed
- **THEN** the verdict is survivor, with the seed in the report

### Requirement: Outcome classification

The program SHALL give exactly one verdict: detection, survivor, invalid or inconclusive.

- **Detection:** every baseline run passed; every mutant run failed the named test with at least one failure line at
  an expected location or containing an expected text; the after-run passed; and the tree did not change.
- **Survivor:** every baseline run passed; every mutant run passed the named test and exited zero; the after-run
  passed; the tree did not change; and one more mutant run, measuring the coverage of the target's package, shows a
  statement on the changed lines executed.
- **Invalid:** the wrong change does not build; or every other condition for survivor holds and the coverage run does
  not show a statement on the changed lines executed.
- **Inconclusive:** every other case, including a baseline run that did not pass, a run that selected no test, a run
  that ended by the timeout (even when an expected failure line was printed before it), a run ended by a signal, a
  failure with no line at an expected location or containing an expected text, mutant runs that disagree, an
  after-run that did not pass, and a tree that changed.

A failing exit status alone SHALL never give detection. A panic or a further failure line printed after an expected
failure line SHALL be recorded in the report and SHALL NOT change a detection. No verdict SHALL be named
"equivalent": whether a survivor is equivalent is a reviewer's assessment.

#### Scenario: Detected

- **WHEN** each mutant run fails the named test with a failure line at the expected location
- **THEN** the verdict is detection

#### Scenario: A different assertion fails

- **WHEN** each mutant run fails the named test only at a location that is not expected
- **THEN** the verdict is inconclusive, and the report gives the locations that failed

#### Scenario: Whole-run timeout

- **WHEN** a mutant run ends with `go test`'s timeout panic
- **THEN** the verdict is inconclusive

#### Scenario: Nothing selected

- **WHEN** `-test` names no test of the package
- **THEN** the verdict is inconclusive and says the name selected no test, and that a test in a file built only with
  the `integration` tag is not run by this command

#### Scenario: The wrong change does not build

- **WHEN** the mutant does not compile
- **THEN** the verdict is invalid

#### Scenario: A survivor that was reached

- **WHEN** each mutant run passes and the coverage run shows a statement on the changed lines executed
- **THEN** the verdict is survivor

#### Scenario: A survivor that was not reached

- **WHEN** each mutant run passes and the coverage run shows no statement on the changed lines executed
- **THEN** the verdict is invalid, and says the test did not reach the wrong change

#### Scenario: A flaky baseline

- **WHEN** one of three baseline runs fails and the other two pass
- **THEN** the verdict is inconclusive

#### Scenario: Mutant runs disagree

- **WHEN** one mutant run is detected and another passes
- **THEN** the verdict is inconclusive

#### Scenario: A failure that names no expected location

- **WHEN** each mutant run exits non-zero with no failure line of the named test at an expected location or
  containing an expected text
- **THEN** the verdict is not detection

#### Scenario: A panic after the expected failure

- **WHEN** each mutant run prints a failure line at the expected location and then panics
- **THEN** the verdict is detection, and the report records the panic

### Requirement: Report and exit status

The program SHALL print a report block that a pull request can quote. It SHALL contain: the command as given; the
commit, the tree's fingerprint, and whether the tree has uncommitted changes; the Go version; the package and the
test; the target's path and its SHA-256 before and after; the wrong change as a unified diff from the target to the
mutant; the expected locations and texts; the seed; each run's command line, result, failure locations, wall time and
verdict; for a would-be survivor, the coverage result; the directory holding the logs, said to be on this machine
only; and, as its last line, the verdict and its reason. The program SHALL exit zero only for detection. Through
`task`, only pass or fail and the report are promised.

#### Scenario: A detection passes

- **WHEN** the verdict is detection
- **THEN** the program exits zero and the last line of its output names the verdict

#### Scenario: A survivor fails

- **WHEN** the verdict is survivor
- **THEN** the program exits non-zero and the last line of its output names the verdict and its reason

#### Scenario: Logs are named as local

- **WHEN** any verdict is reached
- **THEN** the report names the directory holding the logs and says it is on this machine only
