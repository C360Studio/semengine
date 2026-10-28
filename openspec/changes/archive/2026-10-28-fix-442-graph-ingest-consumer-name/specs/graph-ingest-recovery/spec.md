# graph-ingest-recovery

## ADDED Requirements

### Requirement: A declared consumer name is refused

graph-ingest names the durable consumer of each JetStream input from the input's subject. Its configuration
validation SHALL refuse a JetStream input that declares `consumer_name`, as invalid configuration whose error names
the port and `consumer_name`, so that construction and port declaration both fail rather than drop the value.

#### Scenario: A JetStream input declaring consumer_name

- **WHEN** graph-ingest's configuration has a JetStream input that declares `consumer_name`
- **THEN** `DeclarePorts`, `CreateGraphIngest` and `Config.Validate` each fail, classified invalid, naming the port and
  `consumer_name`
- **AND** the test that verifies this is `TestConfigRefusesDeclaredConsumerName`
