# harness-boundaries

## MODIFIED Requirements

### Requirement: Import graph

No non-test Go file outside `internal/harness/` SHALL import `internal/harness/...`, `testcontainers-go`, `testing`,
or `gopkg.in/yaml.v3`. No production package SHALL import the component factories of more than one component family;
the consumer's composition root is the only place factories and payload registrations are aggregated. No Go file in
the module, test or non-test, SHALL import `github.com/c360studio/semstreams` or any path under it, and `go.mod`
SHALL NOT require it. `internal/harness/natsfixture` SHALL import no package of this module outside
`internal/harness/`; any other `internal/harness/` package MAY import a package of this module only when that package
starts no goroutine and holds no resource beyond a call, and no test of any package in the helper's dependency
closure imports the helper.

#### Scenario: Production package imports the fixture

- **WHEN** a non-test file outside internal/harness imports internal/harness/natsfixture
- **THEN** the contract test fails naming the file

#### Scenario: An aggregator package appears

- **WHEN** a production package imports `Register` from two or more component packages
- **THEN** the contract test fails naming the package

#### Scenario: A file imports SemStreams

- **WHEN** any Go file in the module, test or non-test, imports `github.com/c360studio/semstreams/...`, or `go.mod`
  requires it
- **THEN** the contract test fails naming the file and the import

#### Scenario: A test helper lives in the harness

- **WHEN** a non-test file under `internal/harness/semantictest` imports `testing` and `pkg/types`
- **THEN** the contract test reports no violation

#### Scenario: The fixture imports a ported package

- **WHEN** a file under `internal/harness/natsfixture` imports `natsclient`
- **THEN** the contract test fails naming the file
