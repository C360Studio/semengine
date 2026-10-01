# lifecycle-suite

## ADDED Requirements

### Requirement: Complete sensitivity matrix

The in-package failpoint double SHALL declare every failpoint in one table that gives the check it must trip, and
every sensitivity test of the suite SHALL take its cases from that table. The suite's own tests SHALL fail when a
declared failpoint has no expected check. The helper that ends a double after a check SHALL join every worker the
double started before it returns, for every failpoint, and SHALL then require that the double holds nothing.

#### Scenario: Failpoint declared without a check

- **WHEN** a failpoint is declared and the table gives it no expected check
- **THEN** a `lifecycletest` test fails naming the failpoint

#### Scenario: Worker of a double that never stops it

- **WHEN** the helper ends a double whose Stop returned nil while its worker was still running
- **THEN** the worker has exited before the helper returns, and the double reports nothing unresolved
