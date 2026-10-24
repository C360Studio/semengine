# metric-registry Specification

## Purpose
The metric registry in `metric` keeps one Prometheus collector per metric name, keyed by service name and metric
name. `RegisterOrGet` hands back the collector already registered under a key when the new one matches it, so every
caller writes to the same series, and refuses with an error, storing nothing, any collector that would clash with
one already registered. No production code registers a collector on Prometheus' process-global registry: a component
registers only on the `metric.MetricsRegistry` it is given, and nowhere when it is given none.

## Requirements

### Requirement: One canonical collector per metric key

`metric.RegisterOrGet(r, serviceName, metricName, candidate)` registers a Prometheus collector under the key
`serviceName.metricName`, or returns the collector already registered under that key. When the key is held by a
collector of the candidate's exact concrete type with the same descriptors, `RegisterOrGet` SHALL return that
collector and no error, and writes through it SHALL be gathered. A caller uses the collector `RegisterOrGet` returns,
never its own candidate.

#### Scenario: Same key, same type and descriptors

- **WHEN** the same key is registered twice with a collector of the same concrete type, name and help, and one write
  is made through each returned handle
- **THEN** both calls return the same collector with no error, and the gathered value is 2

### Requirement: A mismatching candidate for a held key is refused

`RegisterOrGet` SHALL return the zero collector and a fatal error, store nothing and leave the canonical collector
untouched for a candidate for a held key whose concrete type, help or label names differ from the held collector's.

#### Scenario: Same key, another type, help or label names

- **WHEN** a key held by a gauge is registered again as a counter of the same name and help, or with a different help,
  or with the same type, name and help but other label names
- **THEN** the call returns the zero collector and a fatal error, and the first collector's gathered value is
  unchanged

### Requirement: A nil candidate is refused

`RegisterOrGet` SHALL return the zero collector and a fatal error, store nothing and leave the canonical collector
untouched for a nil or typed-nil candidate, without panicking.

#### Scenario: Nil candidate

- **WHEN** a nil collector or a typed-nil collector is registered
- **THEN** the call returns the zero collector and a fatal error, does not panic, and `Unregister` of the key returns
  false

### Requirement: A candidate whose descriptor another owner holds is refused

`RegisterOrGet` SHALL return the zero collector and a fatal error, store nothing and leave the canonical collector
untouched for a candidate Prometheus refuses because another key, a core metric or a collector registered directly
through `PrometheusRegistry()` already owns its descriptor. `Unregister` of the refused key SHALL return false.

#### Scenario: Cross-key alias

- **WHEN** a collector whose descriptor a second key, a core metric, or a collector registered directly through
  `PrometheusRegistry()` already owns is registered under a new key
- **THEN** the call returns the zero collector and a fatal error, `Unregister` of the new key returns false, and only
  the owning collector's series is gathered, carrying the owner's writes

### Requirement: No process-global registration

No non-test Go file of this module SHALL register a collector on Prometheus' process-global registry
(`prometheus.DefaultRegisterer`, `prometheus.MustRegister`, `prometheus.Register`, or `promauto` without a registry).

#### Scenario: No global registration in the module

- **WHEN** the non-test Go files of this module are searched for a call that registers on Prometheus' process-global
  registry
- **THEN** none is found

### Requirement: A component registers only on the registry it is given

A component SHALL register its collectors only on the `metric.MetricsRegistry` it is given; given none, it SHALL
register them nowhere.

#### Scenario: A component built without a registry

- **WHEN** graph-ingest is built with a nil metrics registry and processes a message
- **THEN** Prometheus' process-global registry gathers none of graph-ingest's series

#### Scenario: Two registries, two components

- **WHEN** two graph-ingest components are built on two different registries
- **THEN** each registry gathers its own component's series and not the other's

### Requirement: A contract test fails on a global registration

A contract test run by `task test:unit` SHALL fail, naming the file and line, on such a call, and a sensitivity test
SHALL plant one in a temporary module and require the failure.

#### Scenario: A planted global registration

- **WHEN** a non-test file of a fixture module calls `prometheus.MustRegister`
- **THEN** the contract test fails naming the file and line

