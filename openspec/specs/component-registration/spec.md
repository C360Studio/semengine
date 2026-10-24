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
`pkg/projection/contract`.

#### Scenario: Dependency listing

- **WHEN** a contract test lists the dependencies of `./component`
- **THEN** no listed path contains an `agentic` element, and none is `graph`, under `graph/`,
  `internal/graphmutation`, `pkg/lifecycle`, or `pkg/projection` other than `pkg/projection/contract`

#### Scenario: A graph import reaches the component model

- **WHEN** a file of `component` imports a package that imports `pkg/lifecycle`
- **THEN** the contract test fails naming the path it found

### Requirement: Component dependencies carry no tool registry or lifecycle manager

A `component.Dependencies` SHALL carry no tool registry and no lifecycle manager; a component that needs the lifecycle
manager receives it from the host when it is registered or constructed.

#### Scenario: Dependencies hold neither

- **WHEN** the fields of `component.Dependencies` are listed
- **THEN** none is a tool registry or a lifecycle manager

### Requirement: The import contract test is shown able to fail

A sensitivity test SHALL require the contract test's failure for a temporary module whose package reaches a forbidden
path.

#### Scenario: A temporary module reaches a forbidden path

- **WHEN** the sensitivity test builds a temporary module whose package reaches a forbidden path
- **THEN** the contract test fails

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
`types.ComponentType` constants, and a `Name` already registered.

#### Scenario: A misspelled component type

- **WHEN** a factory is registered with `Type` `"procesor"`
- **THEN** `Register` fails with an invalid-class error naming the factory, and nothing is registered

### Requirement: The registry keeps its own copy of a registration

The registry SHALL keep its own copy of the registration, so a later change to the caller's value, its schema or its
dependency list does not change what is registered, except inside a schema default that is not a JSON value. A default
that is nil, a string, a bool, a number, a `[]string`, or a `[]any` or `map[string]any` holding these at any depth SHALL
be copied; a default of any other type is stored as is and stays shared with the caller.

#### Scenario: The caller changes its value after registering

- **WHEN** a caller registers a factory and then changes the description, a schema property and the dependency list
  of the value it passed
- **THEN** `ListFactories` reports the values as they were registered

### Requirement: The registration is the one place a factory reports its schema

The registration SHALL be the one place a factory reports its configuration schema, description and version:
`Discoverable` SHALL require only `InputPorts`, `OutputPorts`, `Health` and `DataFlow`.

#### Scenario: A component with only ports, health and flow

- **WHEN** a type that has `InputPorts`, `OutputPorts`, `Health` and `DataFlow`, and no `Meta` or `ConfigSchema`, is
  assigned to a `component.Discoverable`
- **THEN** the program compiles

### Requirement: The configuration check reads the whole input

`component.ValidateFactoryConfig`, which `Registry.CreateComponent`, `Registry.Declare` and `component.SafeUnmarshal`
call, SHALL refuse with an invalid-class error a configuration in which anything other than whitespace follows the
first JSON value.

#### Scenario: A configuration object pasted twice

- **WHEN** a component is created from the configuration `{} {"s":"x"}`
- **THEN** creation fails with an invalid-class error, and the factory is never called

### Requirement: SafeUnmarshal refuses a nil target

`component.SafeUnmarshal` SHALL refuse a nil target, or a nil pointer, with an invalid-class error, whether or not the
configuration is empty, and SHALL NOT panic.

#### Scenario: A nil target

- **WHEN** `SafeUnmarshal` is called with a valid configuration and a nil target
- **THEN** it returns an invalid-class error and does not panic

#### Scenario: A nil target with an empty configuration

- **WHEN** `SafeUnmarshal` is called with an empty configuration and a nil target
- **THEN** it returns an invalid-class error

### Requirement: SafeUnmarshal refuses a key the target does not define

`component.SafeUnmarshal` SHALL refuse, with an invalid-class error naming the key, a configuration key the target
struct does not define.

#### Scenario: A key the target does not define

- **WHEN** `SafeUnmarshal` decodes `{"prot":7}` into a struct whose only field is tagged `port`
- **THEN** it returns an invalid-class error naming `prot`

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

### Requirement: Admission refuses a second claim on an exclusive resource

An exclusive resource is one only one port may hold. Today the only exclusive port is `component.NetworkPort`, a
listener whose claim is its protocol, host and port, shown as its resource text (`tcp:127.0.0.1:8080`).

`component.Registry.CreateComponent` SHALL refuse, with an error of the invalid class, a component with a claim that
clashes with a claim an admitted component holds. The error SHALL name the refused component, its claim, the holder
and the holder's claim.

#### Scenario: Two components on one address

- **WHEN** component `first` is admitted with a network port on `tcp:127.0.0.1:8080`, and component `second` is then
  created with a network port on the same address
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'second' claims tcp:127.0.0.1:8080, held by component 'first' as tcp:127.0.0.1:8080`,
  and `Snapshots` holds only `first`

### Requirement: A refusal names the first holder and changes nothing

When several admitted components hold clashing claims, the refusal SHALL name the first holder in instance-name order,
and the admitted set SHALL be left unchanged.

#### Scenario: Several holders clash

- **WHEN** components `beta` on `tcp:127.0.0.1:8080` and `alpha` on `tcp:::1:8080` are admitted, and component
  `gamma` is then created with a network port on `tcp:0.0.0.0:8080`
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'gamma' claims tcp:0.0.0.0:8080, held by component 'alpha' as tcp:::1:8080`, and
  `Snapshots` holds `alpha` and `beta`

### Requirement: Two network claims clash by transport, port and host

Two network claims SHALL clash when they have the same transport and the same port, and either host is a wildcard (an
unspecified address such as `0.0.0.0`, `::` or `::ffff:0.0.0.0`), or both hosts are IP addresses naming one address (an
IPv4-mapped IPv6 address names its IPv4 address, and an IPv6 zone is ignored), or both hosts are the same text. The
transport is UDP when the protocol is `udp` in any letter case and TCP for every other protocol.

#### Scenario: Two protocol labels on one TCP port

- **WHEN** component `first` is admitted with a network port on `http:0.0.0.0:8080`, and component `second` is then
  created with a network port on `websocket:0.0.0.0:8080`
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'second' claims websocket:0.0.0.0:8080, held by component 'first' as http:0.0.0.0:8080`

#### Scenario: A protocol label in another letter case

- **WHEN** component `first` is admitted with a network port on `UDP:0.0.0.0:8080`, and component `second` is then
  created with a network port on `udp:0.0.0.0:8080`
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'second' claims udp:0.0.0.0:8080, held by component 'first' as UDP:0.0.0.0:8080`

#### Scenario: A wildcard host and a specific host on one port

- **WHEN** component `first` is admitted with a network port on `tcp:0.0.0.0:8080`, and component `second` is then
  created with a network port on `tcp:127.0.0.1:8080`
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'second' claims tcp:127.0.0.1:8080, held by component 'first' as tcp:0.0.0.0:8080`;
  and when `first` is admitted on `tcp:127.0.0.1:8080` and `second` is then created on `tcp:0.0.0.0:8080`, the error
  reads
  `resource conflict: component 'second' claims tcp:0.0.0.0:8080, held by component 'first' as tcp:127.0.0.1:8080`

#### Scenario: TCP and UDP on one port

- **WHEN** component `first` is admitted with a network port on `tcp:0.0.0.0:8080`, and component `second` is then
  created with a network port on `udp:0.0.0.0:8080`
- **THEN** both are admitted, and `Snapshots` holds `first` and `second`

#### Scenario: Two loopback addresses on one port

- **WHEN** component `first` is admitted with a network port on `tcp:127.0.0.1:8080`, and component `second` is then
  created with a network port on `tcp:::1:8080`
- **THEN** both are admitted, and `Snapshots` holds `first` and `second`

#### Scenario: A zone on an IPv6 address is ignored

- **WHEN** component `first` is admitted with a network port on `tcp:::1:8080`, and component `second` is then
  created with a network port on `tcp:::1%lo0:8080`
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'second' claims tcp:::1%lo0:8080, held by component 'first' as tcp:::1:8080`

#### Scenario: One link-local address on two interfaces

- **WHEN** component `first` is admitted with a network port on `tcp:fe80::1%en0:8080`, and component `second` is
  then created with a network port on `tcp:fe80::1%en1:8080`
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'second' claims tcp:fe80::1%en1:8080, held by component 'first' as tcp:fe80::1%en0:8080`

### Requirement: A host name is not resolved at admission

Admission SHALL NOT resolve a host name, so a name beside an address that resolves to the same address is not refused
here; the bind at `Start` decides it (not promised).

#### Scenario: A host name is not resolved

- **WHEN** component `first` is admitted with a network port on `tcp:localhost:8080`, and component `second` is then
  created with a network port on `tcp:127.0.0.1:8080`
- **THEN** both are admitted, and `Snapshots` holds `first` and `second`

### Requirement: A component's own claims may not clash

`Registry.CreateComponent` and `Registry.Declare` SHALL both refuse, with an error of the invalid class, a component
two of whose own claims clash, in either direction, naming the component and both claims.

#### Scenario: One component on one address twice

- **WHEN** component `gateway` declares an input and an output network port, both on `tcp:0.0.0.0:8080`
- **THEN** `Declare` and `CreateComponent` each fail with an invalid-class error reading
  `resource conflict: component 'gateway' claims tcp:0.0.0.0:8080, held by another of its ports as tcp:0.0.0.0:8080`,
  and nothing is admitted

#### Scenario: One component, two protocol labels on one TCP port

- **WHEN** component `gateway` declares an input network port on `http:0.0.0.0:8080` and an output network port on
  `ws:0.0.0.0:8080`
- **THEN** `Declare` and `CreateComponent` each fail with an invalid-class error reading
  `resource conflict: component 'gateway' claims ws:0.0.0.0:8080, held by another of its ports as http:0.0.0.0:8080`

### Requirement: Claims that do not clash are admitted

Admission SHALL NOT refuse claims that do not clash, which MAY both be admitted, and a resource that is not
exclusive, such as a file path, MAY be shared.

#### Scenario: Two components on one file path

- **WHEN** two components each declare a file port on the same path
- **THEN** both are admitted

### Requirement: A declaration names its first clash with another

`component.Declaration.ConflictWith(held)` SHALL return nil when no exclusive claim of the declaration clashes with an
exclusive claim of `held`, under the clash rule of "Two network claims clash by transport, port and host". When one
does, it SHALL return an unwrapped error whose text is `resource conflict: component '<declaration>' claims <claim>,
held by component '<held>' as <held claim>` for the first clash found.

#### Scenario: The method and admission agree

- **WHEN** component `first` is admitted on `http:0.0.0.0:8080` and component `second` declares
  `websocket:0.0.0.0:8080`
- **THEN** `second`'s declaration `ConflictWith` `first`'s returns an error whose text is
  `resource conflict: component 'second' claims websocket:0.0.0.0:8080, held by component 'first' as http:0.0.0.0:8080`,
  and `CreateComponent` refuses `second` with an invalid-class error containing that text
- **AND** when each side declares two claims and only the candidate's second clashes with the holder's first, the
  error names those two claims (`TestDeclarationConflictWithMatchesAdmission`)

#### Scenario: No clash

- **WHEN** the two declarations claim `udp:0.0.0.0:8080` and `tcp:0.0.0.0:8080`
- **THEN** `ConflictWith` returns nil (`TestDeclarationConflictWithAdmitsNonClashingClaims`)

### Requirement: The first clash is found in a fixed order

The search for the first clash SHALL visit `held`'s claims and, for each, the declaration's claims, each side inputs
first and then outputs in declaration order.

#### Scenario: Only the candidate's second claim clashes

- **WHEN** each side declares two claims and only the candidate's second clashes with the holder's first
- **THEN** the error names those two claims (`TestDeclarationConflictWithMatchesAdmission`)

### Requirement: Admission wraps the declaration's clash error

`Registry.CreateComponent` SHALL refuse a clashing candidate, naming the first admitted holder in instance-name order,
with an invalid-class error that wraps the error `ConflictWith` returns, so the refusal's text contains it.

#### Scenario: The refusal contains the method's text

- **WHEN** `CreateComponent` refuses a candidate that clashes with an admitted component
- **THEN** the refusal is an invalid-class error whose text contains the text `ConflictWith` returns for the same pair

### Requirement: A declaration carries no separate exclusive-resource list

A declaration SHALL carry no separate list of exclusive-resource text.

#### Scenario: Claims come from the ports

- **WHEN** a `Declaration` is inspected
- **THEN** it has no field listing exclusive-resource text apart from its ports
