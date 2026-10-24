# port-status Specification

## Purpose

`scripts/port-status.sh` (run as `task status`) reports the Slice 04A 65-package port scoreboard from
`docs/admission-ledger.yaml` (`--offline` requires no network access), checks GitHub milestone and issue hygiene
(`--check`), and synchronizes open issues and pull requests into the `SemEngine Port Tracker` GitHub Project v2 board
(`--sync-project`).

## Requirements

### Requirement: Port scoreboard from the admission ledger

`scripts/port-status.sh` (invoked by `task status`) SHALL read `docs/admission-ledger.yaml` and report the Slice 04A
65-package port status across the seven changes (`setup-04a-01-floor` through `setup-04a-07-substrate-seam`), plus the
per-owner split of `service`, without requiring network access when run with `--offline`.

#### Scenario: Offline scoreboard run

- **WHEN** `scripts/port-status.sh --offline` is run from the repository root
- **THEN** it prints the package counts for all seven Slice 04A changes, the `service` PR 1–7 status lines, and the
  65-package summary, and exits 0

### Requirement: Issue and milestone hygiene check

When run with `--check`, `scripts/port-status.sh` SHALL query GitHub for open milestones and open issues, exit 2 if any
`gh` read fails or returns malformed JSON, and exit 1 naming each offending item if any open milestone title contains
whitespace or any open issue lacks a milestone or has zero labels; otherwise it SHALL exit 0.

#### Scenario: Clean milestones and issues pass the check

- **WHEN** `scripts/port-status.sh --check` runs against open milestones `setup`, `slice-04a`, `slice-04b`, and
  `slice-04c` and every open issue has a milestone and at least one label
- **THEN** it prints `hygiene: ok` and exits 0

#### Scenario: An open issue missing a milestone or label fails the check

- **WHEN** `scripts/port-status.sh --check` runs and an open issue has `milestone: null` or an empty `labels` list
- **THEN** it prints the issue number and reason and exits 1

#### Scenario: A milestone title containing whitespace fails the check

- **WHEN** `scripts/port-status.sh --check` runs and an open milestone has whitespace in its `title`
- **THEN** it prints the milestone title and exits 1

### Requirement: GitHub Project v2 sync

When run with `--sync-project [<number>]`, `scripts/port-status.sh` SHALL resolve the exact `SemEngine Port Tracker`
project (or explicit `<number>`), refuse malformed or ambiguous project discovery with exit code 2 before any write,
and create and link the project when absent.

#### Scenario: Exact project matching and multi-page sync

- **WHEN** `scripts/port-status.sh --sync-project` runs with multiple organization projects or more than 100 open issues
- **THEN** it selects only the exact `SemEngine Port Tracker` title (creating and linking the project first only when
  absent), paginates every open issue and pull request, and exits 0; if discovery is malformed/ambiguous it exits 2
  before any write, and if a GraphQL connection is truncated it exits 2 before any item or field mutation

### Requirement: GitHub Project v2 sync refuses truncated collections and pages through all items

When run with `--sync-project`, `scripts/port-status.sh` SHALL refuse truncated GraphQL collections with exit code 2
before any item or field mutation, and SHALL paginate open issues and pull requests beyond the first 100.

#### Scenario: A truncated collection or more than 100 open items

- **WHEN** `scripts/port-status.sh --sync-project` runs and a GraphQL connection is truncated, or more than 100 issues
  are open
- **THEN** it exits 2 before any item or field mutation for a truncated connection, and paginates every open issue and
  pull request otherwise

### Requirement: GitHub Project v2 sync writes open items and field values

When run with `--sync-project`, `scripts/port-status.sh` SHALL sync open items and field values (`Status`, `Stage`,
`Start Date`, `Target Date`).

#### Scenario: Open items and their fields

- **WHEN** `scripts/port-status.sh --sync-project` runs against a project with open issues and pull requests
- **THEN** each open item is synced with its `Status`, `Stage`, `Start Date` and `Target Date` field values
