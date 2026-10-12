# process-host

## MODIFIED Requirements

### Requirement: Helper process from the test binary

`internal/harness/prochost` SHALL start a child process by re-executing the running test binary with a named helper
selected through an environment marker, in its own process group, with stdout and stderr captured under the directory
`SEMENGINE_EVIDENCE_DIR` names when a developer sets it, and under the test's temporary directory otherwise; a helper
SHALL run only when its marker is set and SHALL be a no-op in a normal test run. No runtime binary SHALL be required.

#### Scenario: Helper runs only under its marker

- **WHEN** the test binary runs without the marker
- **THEN** the helper test returns immediately and starts nothing

#### Scenario: Helper is started

- **WHEN** a test starts the helper named by the marker
- **THEN** a child process exists in its own process group and its output is written under the directory
  `SEMENGINE_EVIDENCE_DIR` names or, when it is unset, under the test's temporary directory
