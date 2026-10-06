# metric-registry

## ADDED Requirements

### Requirement: No process-global registration

No non-test Go file of this module SHALL register a collector on Prometheus' process-global registry
(`prometheus.DefaultRegisterer`, `prometheus.MustRegister`, `prometheus.Register`, or `promauto` without a registry).
A component SHALL register its collectors only on the `metric.MetricsRegistry` it is given; given none, it SHALL
register them nowhere. A contract test run by `task test:unit` SHALL fail, naming the file and line, on such a call,
and a sensitivity test SHALL plant one in a temporary module and require the failure.

#### Scenario: A component built without a registry

- **WHEN** graph-ingest is built with a nil metrics registry and processes a message
- **THEN** Prometheus' process-global registry gathers none of graph-ingest's series

#### Scenario: Two registries, two components

- **WHEN** two graph-ingest components are built on two different registries
- **THEN** each registry gathers its own component's series and not the other's

#### Scenario: A planted global registration

- **WHEN** a non-test file of a fixture module calls `prometheus.MustRegister`
- **THEN** the contract test fails naming the file and line
