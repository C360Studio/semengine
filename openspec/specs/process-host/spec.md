# process-host Specification

## Purpose
The process host, `internal/harness/prochost`, lets a test run part of itself as a separate child process by
re-running the test binary, so a test can kill, pause or signal that process at a chosen point and check what
survived. The child's output is kept in files and written to the test log when the test fails, and no child outlives
the test that started it.

## Requirements

### Requirement: Helper process from the test binary

`internal/harness/prochost` SHALL start a child process by re-executing the running test binary with a named helper
selected through an environment marker, in its own process group, with stdout and stderr captured in files under the
test's temporary directory.

#### Scenario: Helper is started

- **WHEN** a test starts the helper named by the marker
- **THEN** a child process exists in its own process group and its output is written under the test's temporary
  directory, and into the test log when the test fails

### Requirement: A failed test gets the child's output in its log

When the test has failed by the time its cleanup reaps the child, prochost SHALL write the child's stdout and stderr to
the test log.

#### Scenario: Output of a failed test

- **WHEN** a test fails while its helper has written to stdout and stderr
- **THEN** the child's stdout and stderr are written to the test log when cleanup reaps it

### Requirement: A helper runs only under its marker

A helper SHALL run only when its marker is set and SHALL be a no-op in a normal test run. No runtime binary SHALL be
required.

#### Scenario: Helper runs only under its marker

- **WHEN** the test binary runs without the marker
- **THEN** the helper test returns immediately and starts nothing

### Requirement: Signals and bounded wait

The host SHALL offer signal, pause and resume, kill, a wait bounded by the caller's context, and an alive probe;
`Wait` SHALL return the exit status when the process ended and the context error with the last observed state when
it did not.

#### Scenario: Kill between two observed points

- **WHEN** the helper reports a checkpoint and the test kills it before the next
- **THEN** Wait returns a killed status within the bound and the first checkpoint's side effect is observable while
  the second's is not

#### Scenario: Paused process does not progress

- **WHEN** the test pauses the helper
- **THEN** the process is observed stopped (process state `T`), the helper's checkpoint count read while it is
  stopped does not change after the test sends its next request, and after resume the next checkpoint arrives

### Requirement: No orphan survives the test

At test end the host SHALL terminate the helper's process group and wait under a fresh bounded context; the
helper's pid SHALL be observed gone before the test completes.

#### Scenario: Test ends with the helper running

- **WHEN** a test returns without stopping its helper
- **THEN** cleanup kills the group and the pid is gone before the next test starts

### Requirement: A helper ends when its test process ends

A helper that `prochost.Start` started SHALL exit on its own once the test process that started it has ended, however
that process ended:

- by returning or exiting without running its cleanups;
- by a signal sent to its process group or to it alone, SIGKILL included;
- by a panic on a goroutine other than the test's own, the `-timeout` alarm included.

A helper stopped by `Pause` is not covered until it is resumed.

#### Scenario: Test process killed alone

- **WHEN** the test process is sent SIGKILL while its helper is running in the helper's own process group
- **THEN** the helper's pid is gone within the test's failure bound, and its stderr names the reason

#### Scenario: Test process interrupted

- **WHEN** the test process ends on a SIGINT it does not catch, and the helper, in its own process group, receives no
  signal
- **THEN** the helper's pid is gone within the test's failure bound, and its stderr names the reason

#### Scenario: Test process exits without cleanup

- **WHEN** the test process exits through `os.Exit`, or panics on a goroutine other than the test's own as the
  `-timeout` alarm does, so its cleanups do not run
- **THEN** the helper's pid is gone within the test's failure bound, and its stderr names the reason

#### Scenario: A helper stopped by Pause

- **WHEN** a helper is stopped by `Pause` and the test process ends
- **THEN** the helper is not promised to exit until it is resumed

### Requirement: A helper that exits on its own says why

When a helper exits because its test process ended, it SHALL write a line naming the reason to its
stderr and SHALL exit with a non-zero status.

#### Scenario: The exit line

- **WHEN** the test process ends and its helper exits on its own
- **THEN** the helper's stderr holds a line naming the reason and its exit status is non-zero

### Requirement: A helper is not ended while its test process runs

While the test process runs, prochost SHALL NOT end a helper except by the test's own calls and its cleanup.

#### Scenario: Helper keeps running while its test process runs

- **WHEN** a test starts a helper and then sends it SIGTERM
- **THEN** the helper was running until then, and it exits with status 0

### Requirement: A helper's standard input is reserved for prochost

A helper's standard input is reserved for prochost, and a helper function SHALL NOT read it.

#### Scenario: A helper function does not read standard input

- **WHEN** a helper function runs
- **THEN** it does not read its standard input
