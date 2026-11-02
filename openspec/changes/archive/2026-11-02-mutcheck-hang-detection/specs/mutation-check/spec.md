# mutation-check

## MODIFIED Requirements

### Requirement: A run is bounded

A mutant run that ends by the timeout, by a signal or by the bound below SHALL be the last mutant run. A run that has
not ended by twice its timeout SHALL be stopped, with its process group; the report states that bound.

#### Scenario: A mutant run times out

- **WHEN** the first mutant run ends by the timeout
- **THEN** no second mutant run is made, the after-run is made, and the verdict follows "Detection by hang"

#### Scenario: A run that does not end

- **WHEN** the `go` command of a run does not exit by twice the timeout
- **THEN** the run is stopped with its process group, and the verdict is inconclusive and names the bound

### Requirement: Detection

The verdict SHALL be detection when every baseline run passed; every mutant run failed the named test and printed at
least one expected line, and no mutant run ended by the timeout; the after-run passed; and the tree did not change.
"Detection by hang" gives the same verdict under its own conditions.

#### Scenario: Detected

- **WHEN** each mutant run fails the named test and prints a line at the expected location
- **THEN** the verdict is detection, and the report prints that line

#### Scenario: A failure that names no expected location

- **WHEN** each mutant run exits non-zero with no expected line and no `go test` timeout panic
- **THEN** the verdict is not detection

### Requirement: Inconclusive

The verdict SHALL be inconclusive in every other case, including a baseline run that did not pass, a run that selected
no test, a run that ended by the timeout and is not a detection by hang, a run ended by a signal, a run stopped by the
bound, a failure with no expected line, mutant runs that disagree, an after-run or a reach run that did not pass, and a
tree that changed.

#### Scenario: A different assertion fails

- **WHEN** each mutant run fails the named test only at a location that is not expected
- **THEN** the verdict is inconclusive, and the report gives the locations that failed

#### Scenario: Whole-run timeout

- **WHEN** every mutant run fails with an expected line and the after-run ends with `go test`'s timeout panic
- **THEN** the verdict is inconclusive, and its reason names the after-run

#### Scenario: Killed by a signal

- **WHEN** a mutant run's process is killed by a signal before the named test reports a result
- **THEN** the verdict is inconclusive, and its reason names the signal

#### Scenario: Nothing selected

- **WHEN** `-test` names no test of the package
- **THEN** the verdict is inconclusive and says the name selected no test, and that a test in a file built only with
  the `integration` tag is not run by this command

#### Scenario: The build breaks after the baselines

- **WHEN** every baseline run passed, and the mutant runs and the after-run all fail to build
- **THEN** the verdict is inconclusive, not invalid

#### Scenario: A flaky baseline

- **WHEN** one of three baseline runs fails and the other two pass
- **THEN** the verdict is inconclusive

#### Scenario: Mutant runs disagree

- **WHEN** one mutant run is detected and another passes
- **THEN** the verdict is inconclusive

### Requirement: The report describes each run

The report SHALL contain each run's command line, `GOFLAGS`, result, expected lines, failure locations, wall time, the
package's elapsed time as `go test -json` reported it (or that none was reported) and verdict; and, for a reach run,
whether the wrong change was reached, not reached or not measurable, with the blocks that decided it.

#### Scenario: A reach run in the report

- **WHEN** a check made a reach run
- **THEN** the report says whether the wrong change was reached, not reached or not measurable, and names the blocks
  that decided it

#### Scenario: Elapsed time in the report

- **WHEN** a baseline run passes and `go test -json` reports the package's elapsed time as 2.4 seconds
- **THEN** the report's entry for that run gives the elapsed time 2.4s beside its wall time

## ADDED Requirements

### Requirement: Detection by hang

The verdict SHALL be detection, by hang, when every baseline run passed; the first mutant run ran the named test and
ended by `go test`'s own timeout, printing `panic: test timed out after`, with no result for the named test, no
signal and before the bound; the after-run passed; the elapsed time `go test -json` reported for each baseline run
and for the after-run was at most a third of the timeout; and the tree did not change. No expected line is needed.

#### Scenario: Detected by hang

- **WHEN** `-timeout 45s` and `-runs 3` are given, every baseline run and the after-run pass with an elapsed time of 3
  seconds, and mutant run 1 runs the named test and ends with `panic: test timed out after 45s` with no expected line
- **THEN** one mutant run is made, the verdict is detection, the program exits zero, and the last line says the
  detection came by hang

#### Scenario: A slow baseline

- **WHEN** `-timeout 45s` is given, baseline run 2 passes with an elapsed time of 20 seconds, and mutant run 1 ends with
  `go test`'s timeout panic
- **THEN** the verdict is inconclusive, and its reason names baseline run 2 and its elapsed time

#### Scenario: A slow after-run

- **WHEN** `-timeout 45s` is given, every baseline run passes in 3 seconds, mutant run 1 ends with `go test`'s timeout
  panic, and the after-run passes with an elapsed time of 20 seconds
- **THEN** the verdict is inconclusive, and its reason names the after-run and its elapsed time

#### Scenario: Stopped at the bound

- **WHEN** every baseline run and the after-run pass within a third of the timeout, and the first mutant run's `go`
  command has not exited by twice the timeout and is stopped
- **THEN** the verdict is inconclusive and names the bound

#### Scenario: A timeout before the named test runs

- **WHEN** mutant run 1 ends with `go test`'s timeout panic and the stream has no run event for the named test
- **THEN** the verdict is inconclusive

#### Scenario: A mutant run before the hang

- **WHEN** `-runs 3` is given, mutant run 1 passes or fails with an expected line, and mutant run 2 ends with `go
  test`'s timeout panic
- **THEN** only mutant runs 1 and 2 are made, and the verdict is inconclusive and says the mutant runs disagree

#### Scenario: A test that reported a result

- **WHEN** mutant run 1's named test passes after printing a line that begins `panic: test timed out after`
- **THEN** the verdict is not detection by hang

### Requirement: What a detection by hang reports

The last line's reason for a detection by hang SHALL say that the detection came by hang, not by an assertion, and
SHALL name the timeout, a third of it, and the slowest baseline run's and the after-run's elapsed times. The margin
SHALL be checked only when mutant run 1 is a hang; no other verdict depends on an elapsed time. A control run over
the margin SHALL be named, with its elapsed time and the margin, in the inconclusive verdict's reason.

#### Scenario: The kind of evidence

- **WHEN** the verdict is detection by hang
- **THEN** the last line begins `verdict: detection: by hang` and names the timeout, the margin and the control runs'
  elapsed times
