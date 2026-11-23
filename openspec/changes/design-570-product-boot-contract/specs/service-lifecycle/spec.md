# service-lifecycle Delta

## ADDED Requirements

### Requirement: A platform the configuration manager did not set is refused

`NewComponentManager` SHALL refuse a `deps.Platform` whose `Org` or `Platform` is not empty and differs from the
`platform.org` or `platform.id` of the configuration `deps.Manager` holds, naming both pairs. `ConfigureFromServices`
SHALL return that error and compose nothing. An empty `deps.Platform` SHALL NOT be refused for this reason.

#### Scenario: The declared platform ID is passed after a mint

- **WHEN** a started `config.Manager` minted `s-<hex>` from the declared `s`, and `deps.Platform` names `s`
- **THEN** `ConfigureFromServices` returns an error naming both pairs and composes nothing
- **AND** the test that verifies this is `TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne`

#### Scenario: The established platform is passed

- **WHEN** `deps.Platform` is built from that manager's `GetConfig().Get()` after `Start`
- **THEN** the component manager is constructed
- **AND** the test that verifies this is `TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne`
