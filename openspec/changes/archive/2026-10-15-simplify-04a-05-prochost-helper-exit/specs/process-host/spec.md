# process-host

## ADDED Requirements

### Requirement: A helper ends when its test process ends

A helper that `prochost.Start` started SHALL exit on its own once the test process that started it has ended, however
that process ended:

- by returning or exiting without running its cleanups;
- by a signal sent to its process group or to it alone, SIGKILL included;
- by a panic on a goroutine other than the test's own, the `-timeout` alarm included.

On that exit the helper SHALL write a line naming the reason to its stderr and SHALL exit with a non-zero status. While
the test process runs, prochost SHALL NOT end a helper except by the test's own calls and its cleanup. A helper's
standard input is reserved for prochost, and a helper function SHALL NOT read it. A helper stopped by `Pause` is not
covered until it is resumed.

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

#### Scenario: Helper keeps running while its test process runs

- **WHEN** a test starts a helper and then sends it SIGTERM
- **THEN** the helper was running until then, and it exits with status 0
