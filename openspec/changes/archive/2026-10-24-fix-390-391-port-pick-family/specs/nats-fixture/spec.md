# nats-fixture

## MODIFIED Requirements

### Requirement: Start takes its host port from the operating system

The create and start phase SHALL run the once-per-process Docker preparation before taking its host port, SHALL
publish the client port on a host port the fixture took from the operating system with a port-0 listener on every IPv4
address (network `tcp4`) and closed, and SHALL NOT leave the host port to Docker to choose; a failure to take a port
SHALL fail that phase.

#### Scenario: Explicit host-port binding alongside held ephemeral ports

- **WHEN** ephemeral host ports are held by listeners on all addresses and two fixtures start
- **THEN** both fixtures' Start calls return nil, Docker's inspect for each container reports its `URL()` port as the
  explicit `HostPort` binding on `4222/tcp`, and JetStream answers on each `URL()`
