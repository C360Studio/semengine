# component-registration

## ADDED Requirements

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
