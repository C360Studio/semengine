# port-status

## ADDED Requirements

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
