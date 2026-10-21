# operator-surface

## ADDED Requirements

### Requirement: The component manager's routes are served under /components

The component manager SHALL serve, under `/components/`, `health`, `list`, `types`, `types/{id}`, `status/{name}`
(with `?debug=true` adding a component's `DebugStatus`), `config/{name}`, `flowgraph` (JSON, or Mermaid with
`format=mermaid`), `validate` and `paths`. Each route SHALL answer only `GET` and SHALL answer any other method with
405. A name that is empty, `.`, `..` or decodes to a path separator SHALL be answered with 400, and an unknown name
with 404. Before boot commits only `health`, `list` and `status/` SHALL be served. `RegisterAll` SHALL register the
component manager's OpenAPI spec, so that every `$ref` in `/openapi.json` names a schema it contains; importing the
package SHALL register nothing.

#### Scenario: A malicious component name

- **WHEN** `/components/status/` is asked for `..` or a name that decodes to a path separator
- **THEN** the answer is 400
- **AND** the test that verifies this is `TestAttack_HandleComponentStatus_MaliciousComponentName`

#### Scenario: A method other than GET

- **WHEN** a route is asked with `POST`
- **THEN** the answer is 405
- **AND** the test that verifies this is `TestAttack_HandleComponentStatus_InvalidHTTPMethods`

#### Scenario: Every schema reference resolves

- **WHEN** a composition built through `RegisterAll` is started with `StartAll`
- **THEN** every `$ref` in `/openapi.json` names a schema in that document
- **AND** the test that verifies this is `TestOpenAPIDocumentResolvesEverySchemaRef`

### Requirement: The composition projections serve the result kept at construction

`/components/flowgraph`, `/components/validate` and `/components/paths` SHALL project the composition result the
component manager kept at construction and SHALL compute nothing. The flowgraph answer for a given composition SHALL be
stable: its JSON and Mermaid forms are a tested contract.

#### Scenario: The flowgraph for a fixture composition

- **WHEN** `/components/flowgraph` is asked for a fixture composition, as JSON and with `format=mermaid`
- **THEN** both answers equal the checked-in files under `service/testdata/`
- **AND** the test that verifies this is `TestFlowGraphMatchesGoldenForFixtureComposition`

#### Scenario: Validation projects the kept result

- **WHEN** `/components/validate` is asked after construction
- **THEN** it returns the findings the construction-time analysis produced
- **AND** the test that verifies this is `TestFlowValidationHandlerProjectsLibraryResult`
