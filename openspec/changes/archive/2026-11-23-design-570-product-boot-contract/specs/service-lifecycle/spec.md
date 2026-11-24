# service-lifecycle Delta

## ADDED Requirements

### Requirement: A platform the configuration manager did not set is refused

`NewComponentManager` SHALL refuse a `deps.Platform` that has either field set and whose pair (`Org`, `Platform`)
differs from the (`platform.org`, `platform.id`) of the configuration `deps.Manager` holds. A pair with one field
empty differs. The refusal SHALL be classified invalid, SHALL name both pairs, and SHALL come before any component
factory runs. `ConfigureFromServices` SHALL return it and compose nothing. A `deps.Platform` with both fields empty
SHALL NOT be refused for this reason.

#### Scenario: The declared platform ID is passed after a mint

- **WHEN** a started `config.Manager` minted `s-<hex>` from the declared `s`, and `deps.Platform` names `s`
- **THEN** `ConfigureFromServices` returns an invalid error naming both pairs, no component factory has run, and
  nothing is composed
- **AND** the test that verifies this is `TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne`

#### Scenario: Another org is passed

- **WHEN** `deps.Platform` names the minted `s-<hex>` under another org
- **THEN** it is refused the same way
- **AND** the test that verifies this is `TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne`

#### Scenario: Half of the pair is passed

- **WHEN** `deps.Platform` sets only `Org`, or only `Platform`, to the effective value
- **THEN** it is refused the same way
- **AND** the test that verifies this is `TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne`

#### Scenario: The established platform is passed

- **WHEN** `deps.Platform` is built from that manager's `GetConfig().Get()` after `Start`
- **THEN** the component manager is constructed
- **AND** the test that verifies this is `TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne`

#### Scenario: No platform is passed

- **WHEN** `deps.Platform` has both fields empty
- **THEN** the component manager is not refused for this reason; a component that needs a platform refuses it when
  it is built
- **AND** the test that verifies this is `TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne`
