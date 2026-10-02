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

## ADDED Requirements

### Requirement: Public signatures name no internal type

A public package is a non-test, non-`main` package of this module whose import path has no `internal` element. No
exported identifier of a public package SHALL name a type declared in a package of this module under `internal/`,
directly or through what a caller outside the module reaches from it: the exported methods of a type it names, its
exported and embedded struct fields, an interface's method set (embedded interfaces included), type arguments,
generic constraints, and alias targets. A type an exported identifier reaches is followed whether it is exported or
not. Function bodies and unexported identifiers are not checked; a caller outside the module cannot reach them.

#### Scenario: A planted violation fails

- **WHEN** a fixture module's public package declares an exported identifier that names a type declared under its
  `internal/` directly (a function result, an exported variable or constant, an exported struct field), or only
  through an exported method, an embedded field, an interface method set, a type argument, a generic constraint on a
  function or a type, an alias, or an unexported type that an exported function returns
- **THEN** the contract test fails naming each identifier and the internal type it reaches

#### Scenario: A clean tree passes

- **WHEN** a fixture module's public package uses a type declared under its `internal/` only in unexported
  identifiers and function bodies
- **THEN** the contract test reports nothing for that package

#### Scenario: The floor passes

- **WHEN** the contract test runs over this module with the floor's eleven public packages ported
- **THEN** it passes, `natsclient.TemporalResolver`'s cache statistics being reached only through the unexported
  `cacheStats`
