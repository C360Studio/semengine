# operator-surface Delta

## MODIFIED Requirements

### Requirement: Health and service listings are served as at the pin

`/health` SHALL answer the aggregate (`health.Aggregate`) of every service's `Health()` plus a `nats` sub-status when
the manager has a NATS client and a `config` sub-status (the config manager's `Health()`) when it has a config
manager, with status 503 when the aggregate is unhealthy and 200 otherwise. The `nats` sub-status
SHALL be healthy with the message `Connected (RTT: <rtt>)` while connected, and unhealthy with a message beginning
`Disconnected:` otherwise.

#### Scenario: NATS disconnected

- **WHEN** `/health` is requested from a manager whose NATS client is not connected
- **THEN** it answers 503, and its `nats` sub-status is unhealthy with a message beginning `Disconnected:`
- **AND** the test that verifies this is `TestSystemHealthReportsNATSDisconnected`

#### Scenario: A config watch lost

- **WHEN** `/health` is requested from a manager whose config manager reports a lost watch
- **THEN** it answers 503, and its `config` sub-status is unhealthy with a message containing `restart the process`
