# component-registration

## ADDED Requirements

### Requirement: Each component package registers itself

Every component package SHALL export `Register(*component.Registry) error`, and every package that owns payload types
SHALL export `RegisterPayloads(*payloadregistry.Registry) error`. No package of this module SHALL aggregate the
registrations of more than one component family; a consumer's composition root calls each package it uses.

#### Scenario: graph-ingest is registered on its own

- **WHEN** a test creates a `component.Registry`, calls `graphingest.Register` on it, and creates graph-ingest from it
  with a payload registry filled by the per-package `RegisterPayloads` calls it needs
- **THEN** the component is created and starts against the fixture

### Requirement: A missing registration names the call that fixes it

When graph-ingest is built with hierarchy inference enabled and the payload registry lacks the hierarchy container
type, construction SHALL fail with an error naming the missing type and the per-package call that registers it
(`inference.RegisterPayloads`), and SHALL NOT name a symbol this module does not have.

#### Scenario: Hierarchy enabled without the container type

- **WHEN** graph-ingest is built with `enable_hierarchy` and a payload registry that lacks the container type
- **THEN** construction fails naming the type and `inference.RegisterPayloads`

### Requirement: The component model reaches no agentic or graph package

`component` SHALL NOT import, directly or through its dependencies, any package of the agentic domain, the `graph`
package or a package under it, `internal/graphmutation`, `pkg/lifecycle`, or `pkg/projection` other than
`pkg/projection/contract`. A `component.Dependencies` SHALL carry no tool registry and no lifecycle manager; a
component that needs the lifecycle manager receives it from the host when it is registered or constructed. A
sensitivity test SHALL require the contract test's failure for a temporary module whose package reaches a
forbidden path.

#### Scenario: Dependency listing

- **WHEN** a contract test lists the dependencies of `./component`
- **THEN** no listed path contains an `agentic` element, and none is `graph`, under `graph/`,
  `internal/graphmutation`, `pkg/lifecycle`, or `pkg/projection` other than `pkg/projection/contract`

#### Scenario: A graph import reaches the component model

- **WHEN** a file of `component` imports a package that imports `pkg/lifecycle`
- **THEN** the contract test fails naming the path it found

### Requirement: Unknown configuration keys are refused

A component ported from the pin SHALL refuse, at construction, a configuration key it does not define, with an error
naming the key.

#### Scenario: Misspelled key

- **WHEN** graph-ingest is built from a configuration containing `ingest_lane` instead of `ingest_lanes`
- **THEN** construction fails naming `ingest_lane`
