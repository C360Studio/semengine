# component-registration Specification

## Purpose
Component registration is how a consumer's program makes SemEngine's components and their payload types known. Each
component package exports its own `Register`, and `RegisterPayloads` where it owns payload types; the consumer calls
each one it uses, and no package registers a whole family of components for it. When graph-ingest is built with
hierarchy inference enabled and the payload registry lacks the hierarchy container type, construction fails with an
error naming that type and the call that registers it (`inference.RegisterPayloads`). The capability also keeps the
`component` package, directly or through its dependencies, from importing any agentic package, `graph` or a package
under it, `internal/graphmutation`, `pkg/lifecycle`, or any `pkg/projection` package but `pkg/projection/contract`,
and requires a component ported from SemStreams to refuse a configuration key it does not define. A component
factory is registered by one `Register` call that keeps its own copy of the registration; the configuration check
reads the whole input; admission snapshots are declaration values; and a configuration schema uses one word for each
basic type.

## Requirements

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

### Requirement: A component factory is registered by one call

`component.Registry.Register(Registration)` SHALL be the only way to register a component factory, keyed by the
registration's `Name`. It SHALL refuse, with an error of the invalid class naming the factory, a `Name` that
`ValidateComponentName` refuses, a nil `Factory` or `Ports`, a `Type` that is not one of the five
`types.ComponentType` constants, and a `Name` already registered. The registry SHALL keep its own copy of the
registration, so a later change to the caller's value, its schema or its dependency list does not change what is
registered, except inside a schema default that is not a JSON value. A default that is nil, a string, a bool, a number,
a `[]string`, or a `[]any` or `map[string]any` holding these at any depth SHALL be copied; a default of any other type
is stored as is and stays shared with the caller. The registration SHALL be the one place a factory reports its
configuration schema, description and version: `Discoverable` SHALL require only `InputPorts`, `OutputPorts`,
`Health` and `DataFlow`.

#### Scenario: A misspelled component type

- **WHEN** a factory is registered with `Type` `"procesor"`
- **THEN** `Register` fails with an invalid-class error naming the factory, and nothing is registered

#### Scenario: The caller changes its value after registering

- **WHEN** a caller registers a factory and then changes the description, a schema property and the dependency list
  of the value it passed
- **THEN** `ListFactories` reports the values as they were registered

#### Scenario: A component with only ports, health and flow

- **WHEN** a type that has `InputPorts`, `OutputPorts`, `Health` and `DataFlow`, and no `Meta` or `ConfigSchema`, is
  assigned to a `component.Discoverable`
- **THEN** the program compiles

### Requirement: The configuration check reads the whole input

`component.ValidateFactoryConfig`, which `Registry.CreateComponent`, `Registry.Declare` and `component.SafeUnmarshal`
call, SHALL refuse with an invalid-class error a configuration in which anything other than whitespace follows the
first JSON value. `SafeUnmarshal` SHALL refuse a nil target, or a nil pointer, with an invalid-class error,
whether or not the configuration is empty, and SHALL NOT panic. It SHALL refuse, with an invalid-class error naming
the key, a configuration key the target struct does not define.

#### Scenario: A configuration object pasted twice

- **WHEN** a component is created from the configuration `{} {"s":"x"}`
- **THEN** creation fails with an invalid-class error, and the factory is never called

#### Scenario: A nil target

- **WHEN** `SafeUnmarshal` is called with a valid configuration and a nil target
- **THEN** it returns an invalid-class error and does not panic

#### Scenario: A key the target does not define

- **WHEN** `SafeUnmarshal` decodes `{"prot":7}` into a struct whose only field is tagged `port`
- **THEN** it returns an invalid-class error naming `prot`

#### Scenario: A nil target with an empty configuration

- **WHEN** `SafeUnmarshal` is called with an empty configuration and a nil target
- **THEN** it returns an invalid-class error

### Requirement: Admission snapshots are declaration values

`component.Registry.Snapshots()` SHALL return a `[]component.Declaration`, one for each admitted component, sorted by
instance name. Each value SHALL be a copy that shares no slice with the registry.

#### Scenario: A caller changes a snapshot

- **WHEN** a caller changes a port in a declaration that `Snapshots` returned, and then calls `Snapshots` again
- **THEN** the second result shows the port as it was admitted

### Requirement: A configuration schema uses one word for each basic type

A schema that `component.GenerateConfigSchema` produces SHALL use the type words `int`, `float` and `bool` for
integer, floating-point and boolean values: in properties, in nested objects, and in the items of arrays.

#### Scenario: An array of integers

- **WHEN** a schema is generated for a configuration struct with a `[]int` field
- **THEN** the field's item schema has type `int`, the same word an `int` field's property has
