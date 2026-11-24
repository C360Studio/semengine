# config-manager Delta

## ADDED Requirements

### Requirement: The platform identity record is a read contract

The `platform_identity` key of the configuration bucket (`semengine_config_<org>_<stem>`) SHALL hold a JSON object
with exactly the fields `org`, `stem` and `id`: the declared `platform.org`, the declared `platform.id`, and the
effective platform ID. A process that does not run `config.Manager` reads this record to learn the platform ID its
entity IDs must carry.

#### Scenario: The record a first boot writes

- **WHEN** a Manager starts on an empty bucket
- **THEN** the `platform_identity` value decodes to an object whose keys are exactly `org`, `stem` and `id`, with `id`
  the adopted effective `platform.id`
- **AND** the test that verifies this is `TestConfigManagerFirstBootMintsPlatformIdentity`
