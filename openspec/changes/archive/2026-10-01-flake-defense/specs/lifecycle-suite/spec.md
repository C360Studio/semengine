# lifecycle-suite

## ADDED Requirements

### Requirement: Complete sensitivity matrix

The in-package failpoint double SHALL declare every failpoint in one table that gives the check it must trip, and
every sensitivity test of the suite SHALL take its cases from that table. The suite's own tests SHALL fail when a
declared failpoint has no expected check.

A worker is signalled when a Stop has told it to exit. The helper that ends a double after a check SHALL call Stop as
Run does after each check and SHALL then require that the double holds nothing, for every failpoint. It SHALL NOT wait
for a signalled worker before it checks that requirement, so that a Stop that returned nil ahead of its worker's exit
is reported and not hidden by the helper's own wait. A worker that no Stop has signalled SHALL be ended through its
Start context and joined before the requirement is checked. The helper SHALL join every worker the double started
before it returns, whether or not the requirement held.

#### Scenario: Failpoint declared without a check

- **WHEN** a failpoint is declared and the table gives it no expected check
- **THEN** a `lifecycletest` test fails naming the failpoint

#### Scenario: Worker of a double that never stops it

- **WHEN** the helper ends a double whose Stop returned nil without signalling its worker
- **THEN** the helper ends and joins that worker, the double then reports nothing unresolved, and the worker has
  exited before the helper returns

#### Scenario: Stop returned ahead of its worker's exit

- **WHEN** the helper ends a double whose Stop signalled its worker and returned nil, and the worker has not exited
- **THEN** the helper fails the test naming what the double still holds, and the worker has exited before the helper
  returns
