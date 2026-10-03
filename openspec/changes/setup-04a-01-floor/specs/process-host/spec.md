# process-host

## ADDED Requirements

### Requirement: Helper process from the test binary

`internal/harness/prochost` SHALL start a child process by re-executing the running test binary with a named helper
selected through an environment marker, in its own process group, with stdout and stderr captured under the
evidence directory when `SEMENGINE_EVIDENCE_DIR` is set (the integration lane) and under the test's temporary
directory otherwise (the unit lane); a helper SHALL run only when its marker is set and SHALL be a no-op in a
normal test run. No runtime binary SHALL be required.

#### Scenario: Helper runs only under its marker

- **WHEN** the test binary runs without the marker
- **THEN** the helper test returns immediately and starts nothing

#### Scenario: Helper is started

- **WHEN** a test starts the helper named by the marker
- **THEN** a child process exists in its own process group and its output is written under the evidence directory
  or, when none is set, under the test's temporary directory

#### Scenario: A test reads what the helper wrote to stderr

- **WHEN** the helper writes to stderr, for example a runtime panic as it ends, and `Wait` has returned
- **THEN** the file at the process's stderr path holds what it wrote

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
