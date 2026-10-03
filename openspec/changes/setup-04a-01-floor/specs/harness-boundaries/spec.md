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

### Requirement: No bare select

No Go file in the module, test or non-test, `package main` included, SHALL contain a `select` statement with no
cases. A goroutine parked on one has no way out, and in a re-executed test binary, which runs with no test-timeout
timer, Go's deadlock detector kills the process once every goroutine blocks. Code parks on `ctx.Done()`, a channel or
`signal.Notify`; a `main` uses `signal.NotifyContext`. The check parses each file, so it matches code only: a comment
or a string that mentions the pattern does not trip it, and every spelling of an empty `select` (spacing, newlines,
a comment inside) does. A file that does not parse fails the check. It sits beside "No sleeps in tests" and has, like
it, no baseline, no allowlist and no inline exemption.

#### Scenario: A bare select is added

- **WHEN** a test file, a non-test file or a `package main` file contains `select {}`, in any spacing
- **THEN** the contract test fails naming the file and line

#### Scenario: A select with cases or a comment

- **WHEN** a file contains a `select` with at least one case, or mentions `select {}` only in a comment or a string
- **THEN** the contract test reports nothing for that file

### Requirement: Public signatures name no internal type

A public package is a non-test, non-`main` package of this module whose import path has no `internal` element. No
exported identifier of a public package SHALL name a type declared in a package of this module with an `internal`
path element (Go's rule for what a caller outside the module cannot import), directly or through what a caller
outside the module reaches from it: the exported methods of a type it names, its exported and embedded struct fields,
an interface's method set (embedded interfaces included), type arguments, generic constraints, and alias targets. A
type an exported identifier reaches is followed whether it is exported or not. Function bodies and unexported
identifiers are not checked; a caller outside the module cannot reach them.

#### Scenario: A planted violation fails

- **WHEN** a fixture module's public package declares an exported identifier that names a type declared in a package
  with an `internal` path element directly (a function result, an exported variable or constant, an exported struct
  field), or only through an exported method, an embedded field, an interface method set, a type argument, a generic
  constraint on a function or a type, an alias, or an unexported type that an exported function returns
- **THEN** the contract test fails naming each identifier and the internal type it reaches

#### Scenario: A clean tree passes

- **WHEN** a fixture module's public package uses a type declared under its `internal/` only in unexported
  identifiers and function bodies
- **THEN** the contract test reports nothing for that package

#### Scenario: The floor passes

- **WHEN** the contract test runs over this module with the floor's eleven public packages ported
- **THEN** it passes: no exported identifier names a type under `internal/` (the pin's one,
  `natsclient.TemporalResolver.GetStats`, is dropped with `TemporalResolver`; owner ruling, #9 comment 5969522395,
  item 1)

### Requirement: No second spelling of deployment authority

A deployment's authority (`org`, `platform`) becomes the first two positions of an identity only through the
entity-ID family (#72 ruling A as extended, comment 5969505488). No exported name in a non-test package of this
module, internal and `main` packages included, SHALL contain `Federation`, `GlobalID` or `EntityIRI`. This covers
package-level identifiers, and the exported methods, struct fields and interface methods of package-level types,
whether the type is exported or not. A type alias is a package-level type: its own name is checked, and so are the
members of the type it stands for; members of a type this module declares are reported once, at that declaration.
A failure names the file, line, qualified identifier and rule. Unexported names and test files are not checked. The
check that a struct field named `Org` or `Platform` appears only where ruling C allows it is not part of this
requirement.

#### Scenario: A deployment-authority name is exported

- **WHEN** a fixture module declares an exported name containing `Federation`, `GlobalID` or `EntityIRI` in a
  public, an internal or a `main` package: a type, function, variable or constant, a method (of an unexported type
  too), a struct field or an interface method, or a type alias, or a member of the struct or interface an alias
  stands for
- **THEN** the contract test fails naming each one once, with its file, line and qualified identifier and the rule

#### Scenario: The words appear where the rule does not apply

- **WHEN** the same words appear only in unexported names, in a name such as `Federated` that does not contain one
  of them, or in a test file
- **THEN** the contract test reports nothing for them
