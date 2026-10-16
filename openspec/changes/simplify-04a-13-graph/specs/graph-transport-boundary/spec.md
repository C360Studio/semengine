# graph-transport-boundary

## MODIFIED Requirements

### Requirement: Reserved request subjects have one declaration

The request/reply verbs graph-ingest serves (`graph.ingest.query.entity`, `.batch`, `.prefix`) SHALL be declared once,
in a verb table in the `graph` package whose entries name each verb's name, subject and responder. The entity and batch
requests SHALL be Go types declared in `graph`, used by the caller and the responder alike. The mutation subject family
(`graph.mutation.>`) SHALL be declared once, in `internal/graphmutation`. Every server and client in the module SHALL
take these subjects from their declarations. A contract test run by `task test:unit` SHALL fail, naming the file and
line, when a non-test Go file other than the two declaring files spells one of these subjects, or the
`graph.mutation.` prefix, as a string literal; a sensitivity test SHALL plant such a literal in a temporary module and
require the failure.

#### Scenario: A literal spelling is added

- **WHEN** a non-test file outside the declarations spells `graph.ingest.query.entity` as a string literal
- **THEN** the contract test fails naming the file and line

#### Scenario: The declarations and tests are not reported

- **WHEN** the subjects appear in a declaring file, in a `_test.go` file, or in a comment
- **THEN** the contract test reports nothing for them

### Requirement: Authority reads are not enveloped

Replies on `graph.ingest.query.*` are authority reads and SHALL NOT be enveloped: the entity verb's reply SHALL carry
the key-value revision it read, under the JSON key `kv_revision`. The module SHALL provide no function that decides
from a reply's content whether it is enveloped.

#### Scenario: An entity read carries its revision

- **WHEN** the entity verb's reply for entity E is a bare `graph.ExactEntity` with revision R
- **THEN** the reply's JSON holds R under `kv_revision`, and the reader returns E and revision R

## ADDED Requirements

### Requirement: Mutation replies carry four outcomes

A mutation reply's `outcome` SHALL be one of `applied`, `unchanged`, `entity_not_found` or `failed`. Every other failure
SHALL travel as a classified error, never as an outcome. graph-ingest SHALL count each canonical mutation on
`graph_mutation_outcomes_total`, with `outcome` set to the reply's outcome, or to the error's classified code, or to
`internal` when the error carries no code (ruling L, #91 comment 6062681355).

#### Scenario: A structurally invalid create

- **WHEN** a create request is refused with code `structural_invalid`
- **THEN** `graph_mutation_outcomes_total{operation="create",outcome="structural_invalid"}` increases by one
