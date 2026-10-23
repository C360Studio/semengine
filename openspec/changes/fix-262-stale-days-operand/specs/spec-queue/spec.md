# spec-queue

## ADDED Requirements

### Requirement: --stale-days takes a whole number of days

`scripts/openspec-queue.sh` SHALL accept `--stale-days` only when the next argument is one or more ASCII digits and
nothing else, read as a decimal number of days. Otherwise it SHALL print `--stale-days needs a whole number of days`
on standard error, print nothing on standard output, and exit 2, before it reads the change list and whatever else
is given. It SHALL never read another option as the operand, and SHALL end on every argument list.

#### Scenario: --stale-days is the last argument

- **WHEN** `scripts/openspec-queue.sh --stale-days` is run
- **THEN** it ends, prints `--stale-days needs a whole number of days` as a line on standard error, prints nothing
  on standard output, and exits 2

#### Scenario: The operand is not a whole number

- **WHEN** the script is run with `--stale-days abc`, `--stale-days -1`, `--stale-days 7d`, or `--stale-days ''`
- **THEN** it prints `--stale-days needs a whole number of days` on standard error, prints nothing on standard
  output, and exits 2

#### Scenario: An option in the operand's place

- **WHEN** the script is run with `--stale-days --strict`, or with `--strict --stale-days abc`
- **THEN** it prints `--stale-days needs a whole number of days` on standard error, prints nothing on standard
  output, and exits 2

#### Scenario: Zero and leading zeros are whole numbers

- **WHEN** the script is run with `--stale-days 0` or `--stale-days 08` on a change whose open tasks carry no caveat
- **THEN** it prints the queue with that change's `ok` line, prints nothing on standard error, and exits 0

### Requirement: An unknown argument is refused

For an argument it does not know, `scripts/openspec-queue.sh` SHALL print `unknown argument: <arg>` on standard error
and exit 2.

#### Scenario: An unknown argument

- **WHEN** the script is run with `--bogus`
- **THEN** it prints `unknown argument: --bogus` on standard error and exits 2
