# config-loading

## ADDED Requirements

### Requirement: The default services are the heartbeat alone

`LoadFile` and `LoadFromBytes` SHALL default `Services` to one entry, `heartbeat`, enabled, and SHALL default no other
service, so a loaded document holds the services it names and the heartbeat.

#### Scenario: A document that names no services

- **WHEN** `LoadFile` or `LoadFromBytes` reads `{"platform":{"org":"acme","id":"p1","type":"vessel"}}`
- **THEN** the returned `Config.Services` has exactly one key, `heartbeat`, and that entry is enabled
