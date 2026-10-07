# merge-gate

## ADDED Requirements

### Requirement: Pinned runner image

Every job in the CI workflow SHALL run on the GitHub-hosted image `ubuntu-24.04`, written as the one label
`runs-on: ubuntu-24.04`: not `ubuntu-latest`, not another label, not a list of labels, not a runner group and not an
expression. `ubuntu-latest` moves to a new Ubuntu release on GitHub's schedule, with no change in this repository, and
a run of the same commit can then pass on one release and fail on the other. The label `ubuntu-24.04` still moves
between GitHub's builds of Ubuntu 24.04, and that is accepted. A move to another image is a change to this
requirement, made in its own pull request, whose CI run is the `task verify` run on the new image. A contract test
SHALL fail, naming the job and what it runs on, when any job in the workflow has no `runs-on`, or a `runs-on` other
than the one label `ubuntu-24.04`.

#### Scenario: A job on the moving label

- **WHEN** a job in `.github/workflows/ci.yml` has `runs-on: ubuntu-latest`, whether it is `verify`, `merge-check`,
  `required` or a job added beside them
- **THEN** the contract test fails naming the job, `ubuntu-latest` and `ubuntu-24.04`

#### Scenario: A job with no runner

- **WHEN** a job in `.github/workflows/ci.yml` has no `runs-on`
- **THEN** the contract test fails naming the job and `runs-on`

#### Scenario: Another label

- **WHEN** a job in `.github/workflows/ci.yml` has `runs-on: ubuntu-26.04` or `runs-on: ubuntu-24.04-arm`
- **THEN** the contract test fails naming the job and that label

#### Scenario: Not one label

- **WHEN** a job's `runs-on` is a list of labels that includes `ubuntu-24.04`, a runner group, or an expression
- **THEN** the contract test fails naming the job and what it runs on
