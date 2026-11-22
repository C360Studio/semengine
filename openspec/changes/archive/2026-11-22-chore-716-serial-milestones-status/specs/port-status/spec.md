# port-status

## MODIFIED Requirements

### Requirement: Issue and milestone hygiene check

When run with `--check`, `scripts/port-status.sh` SHALL query GitHub for open milestones and open issues, exit 2 if any
`gh` read fails or returns malformed JSON, and exit 1 naming each offending item if any open milestone title contains
whitespace or any open issue lacks a milestone or has zero labels; otherwise it SHALL exit 0.

#### Scenario: Clean milestones and issues pass the check

- **WHEN** `scripts/port-status.sh --check` runs against open milestones `v0.1.0-alpha.7`, `v0.1.0-alpha.8`,
  `v0.1.0-alpha.9`, `v0.1.0-alpha.10`, `tier-2`, and `backlog` and every open issue has a milestone and at least one
  label
- **THEN** it prints `hygiene: ok` and exits 0

#### Scenario: An open issue missing a milestone or label fails the check

- **WHEN** `scripts/port-status.sh --check` runs and an open issue has `milestone: null` or an empty `labels` list
- **THEN** it prints the issue number and reason and exits 1

#### Scenario: A milestone title containing whitespace fails the check

- **WHEN** `scripts/port-status.sh --check` runs and an open milestone has whitespace in its `title`
- **THEN** it prints the milestone title and exits 1
