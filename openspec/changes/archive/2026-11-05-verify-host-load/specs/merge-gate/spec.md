# merge-gate

## MODIFIED Requirements

### Requirement: The flake label is for failures a pull request can end

The label `class:flake` is for a failure that a pull request in this repository can end: a test or check that passes and
fails on the same tree. A failed network fetch SHALL NOT be filed under it. A red from a local run whose `task verify`
log shows a one-minute load average above twice the host's core count SHALL be rerun on an idle host before it is filed,
and its issue SHALL quote the log's `host:` lines. No command sorts these, so this part of the rule is checked in
review only.

#### Scenario: Failed network fetch

- **WHEN** a network fetch fails
- **THEN** it is not filed under `class:flake`; review checks this, as no command sorts the two

#### Scenario: Local red under host load

- **WHEN** a test or check fails in a local `task verify` whose `host:` line shows a one-minute load average above twice
  the core count
- **THEN** it is rerun on an idle host before any issue is filed, and an issue that is filed quotes the `host:` lines;
  review checks this, as no command sorts a load-induced red from a flake
