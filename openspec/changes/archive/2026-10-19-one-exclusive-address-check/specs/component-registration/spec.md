# component-registration

## ADDED Requirements

### Requirement: Admission refuses a second claim on an exclusive resource

An exclusive resource is one only one port may hold; today the only exclusive port is `component.NetworkPort`, whose
resource is its protocol, host and port (`tcp:127.0.0.1:8080`). `component.Registry.CreateComponent` SHALL refuse,
with an error of the invalid class, a component that claims an exclusive resource an admitted component already
holds, naming the resource and the holder, and SHALL leave the admitted set unchanged. `Registry.CreateComponent` and
`Registry.Declare` SHALL both refuse, with an error of the invalid class, a component that claims one exclusive
resource on two of its own ports, in either direction, naming the resource and the component. A resource that is not
exclusive, such as a file path, MAY be shared.

#### Scenario: Two components on one address

- **WHEN** component `first` is admitted with a network port on `tcp:127.0.0.1:8080`, and component `second` is then
  created with a network port on the same address
- **THEN** creation fails with an invalid-class error saying `tcp:127.0.0.1:8080` is already used by component
  `first`, and `Snapshots` holds only `first`

#### Scenario: One component on one address twice

- **WHEN** component `gateway` declares an input and an output network port, both on `tcp:0.0.0.0:8080`
- **THEN** `Declare` and `CreateComponent` each fail with an invalid-class error reading
  `resource conflict: tcp:0.0.0.0:8080 already used by another port of component 'gateway'`, and nothing is
  admitted

#### Scenario: Two components on one file path

- **WHEN** two components each declare a file port on the same path
- **THEN** both are admitted
