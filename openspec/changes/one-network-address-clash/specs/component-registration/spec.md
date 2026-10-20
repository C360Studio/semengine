# component-registration

## MODIFIED Requirements

### Requirement: Admission refuses a second claim on an exclusive resource

An exclusive resource is one only one port may hold. Today the only exclusive port is `component.NetworkPort`, a
listener whose claim is its protocol, host and port, shown as its resource text (`tcp:127.0.0.1:8080`).

Two network claims clash when they have the same transport and the same port, and either host is a wildcard (an
unspecified address such as `0.0.0.0`, `::` or `::ffff:0.0.0.0`), or both hosts are IP addresses naming one address (an
IPv4-mapped IPv6 address names its IPv4 address; an IPv6 zone is ignored except on a link-local address, where two
zone texts are two addresses), or both hosts are the same text. The transport is UDP when the protocol is `udp` in
any letter case and TCP for every other protocol. A host name or an interface name is not resolved at admission, so
a name beside an address, or an interface's name beside its index, that the system takes as one is not refused here;
the bind at `Start` decides it (not promised).

`component.Registry.CreateComponent` SHALL refuse, with an error of the invalid class, a component with a claim that
clashes with a claim an admitted component holds. The error SHALL name the refused component, its claim, the holder
and the holder's claim, naming the first holder in instance-name order when several clash, and the admitted set SHALL
be left unchanged.

`Registry.CreateComponent` and `Registry.Declare` SHALL both refuse, with an error of the invalid class, a component
two of whose own claims clash, in either direction, naming the component and both claims.

Claims that do not clash MAY both be admitted, and a resource that is not exclusive, such as a file path, MAY be
shared.

#### Scenario: Two components on one address

- **WHEN** component `first` is admitted with a network port on `tcp:127.0.0.1:8080`, and component `second` is then
  created with a network port on the same address
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'second' claims tcp:127.0.0.1:8080, held by component 'first' as tcp:127.0.0.1:8080`,
  and `Snapshots` holds only `first`

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

#### Scenario: A host name is not resolved

- **WHEN** component `first` is admitted with a network port on `tcp:localhost:8080`, and component `second` is then
  created with a network port on `tcp:127.0.0.1:8080`
- **THEN** both are admitted, and `Snapshots` holds `first` and `second`

#### Scenario: A zone on an address that is not link-local

- **WHEN** component `first` is admitted with a network port on `tcp:::1:8080`, and component `second` is then
  created with a network port on `tcp:::1%lo0:8080`
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'second' claims tcp:::1%lo0:8080, held by component 'first' as tcp:::1:8080`

#### Scenario: Link-local addresses on two interfaces

- **WHEN** component `first` is admitted with a network port on `tcp:fe80::1%en0:8080`, and component `second` is
  then created with a network port on `tcp:fe80::1%en1:8080`
- **THEN** both are admitted, and `Snapshots` holds `first` and `second`

#### Scenario: Several holders clash

- **WHEN** components `beta` on `tcp:127.0.0.1:8080` and `alpha` on `tcp:::1:8080` are admitted, and component
  `gamma` is then created with a network port on `tcp:0.0.0.0:8080`
- **THEN** creation fails with an invalid-class error reading
  `resource conflict: component 'gamma' claims tcp:0.0.0.0:8080, held by component 'alpha' as tcp:::1:8080`, and
  `Snapshots` holds `alpha` and `beta`

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

#### Scenario: Two components on one file path

- **WHEN** two components each declare a file port on the same path
- **THEN** both are admitted
