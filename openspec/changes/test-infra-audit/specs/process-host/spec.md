# process-host

## MODIFIED Requirements

### Requirement: Helper process from the test binary

`internal/harness/prochost` SHALL start a child process by re-executing the running test binary with a named helper
selected through an environment marker, in its own process group, with stdout and stderr captured in files under the
test's temporary directory; when the test has failed by the time its cleanup reaps the child, prochost SHALL write the
child's stdout and stderr to the test log. A helper SHALL run only when its marker is set and SHALL be a no-op in a
normal test run. No runtime binary SHALL be required.

#### Scenario: Helper runs only under its marker

- **WHEN** the test binary runs without the marker
- **THEN** the helper test returns immediately and starts nothing

#### Scenario: Helper is started

- **WHEN** a test starts the helper named by the marker
- **THEN** a child process exists in its own process group and its output is written under the test's temporary
  directory, and into the test log when the test fails
