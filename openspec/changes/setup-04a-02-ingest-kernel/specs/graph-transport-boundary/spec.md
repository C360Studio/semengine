# graph-transport-boundary

## ADDED Requirements

### Requirement: Reserved request subjects have one declaration

The request/reply subjects graph-ingest serves (`graph.ingest.query.entity`, `.batch`, `.prefix`, `.suffix`) and the
mutation subject family (`graph.mutation.>`) SHALL be declared once, in the `graph` package, and every server and
client in the module SHALL use that declaration. A contract test run by `task test:unit` SHALL fail, naming the file
and line, when a non-test Go file outside the declaring file spells one of these subjects, or the `graph.mutation.`
prefix, as a string literal; a sensitivity test SHALL plant such a literal in a temporary module and require the
failure.

#### Scenario: A literal spelling is added

- **WHEN** a non-test file outside the declaration spells `graph.ingest.query.entity` as a string literal
- **THEN** the contract test fails naming the file and line

#### Scenario: The declaration and tests are not reported

- **WHEN** the subjects appear in the declaring file, in a `_test.go` file, or in a comment
- **THEN** the contract test reports nothing for them
