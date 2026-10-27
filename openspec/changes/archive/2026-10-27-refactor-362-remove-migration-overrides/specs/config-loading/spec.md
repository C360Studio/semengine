# config-loading delta: refactor-362-remove-migration-overrides

## MODIFIED Requirements

### Requirement: Names a deployment chooses are not checked for letter case

`LoadFile` and `LoadFromBytes` SHALL NOT check map keys a deployment chooses (component, service, stream, archival
and model registry names) or the contents of a component's or service's `config` payload for letter case.

#### Scenario: Names a deployment chooses are not checked

- **WHEN** `LoadFromBytes` reads a document with a component named `Config` whose `config` payload holds `"Port"`
- **THEN** it returns a `Config` holding that component, with the payload unchanged, and no error
