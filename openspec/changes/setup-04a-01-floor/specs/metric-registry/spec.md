# metric-registry

## ADDED Requirements

### Requirement: One canonical collector per metric key

`metric.RegisterOrGet(r, serviceName, metricName, candidate)` registers a Prometheus collector under the key
`serviceName.metricName`, or returns the collector already registered under that key. When the key is held by a
collector of the candidate's exact concrete type with the same descriptors, `RegisterOrGet` SHALL return that
collector and no error, and writes through it SHALL be gathered. A caller uses the collector `RegisterOrGet` returns,
never its own candidate.

`RegisterOrGet` SHALL return the zero collector and a fatal error, store nothing and leave the canonical collector
untouched for each of these candidates:

- one for a held key whose concrete type, help or label names differ from the held collector's;
- a nil or typed-nil candidate, without panicking;
- one Prometheus refuses because another key, a core metric or a collector registered directly through
  `PrometheusRegistry()` already owns its descriptor. `Unregister` of the refused key SHALL return false.

#### Scenario: Same key, same type and descriptors

- **WHEN** the same key is registered twice with a collector of the same concrete type, name and help, and one write
  is made through each returned handle
- **THEN** both calls return the same collector with no error, and the gathered value is 2

#### Scenario: Same key, another type or help

- **WHEN** a key held by a gauge is registered again as a counter of the same name and help, or with a different help
- **THEN** the call returns the zero collector and a fatal error, and the first collector's gathered value is
  unchanged

#### Scenario: Nil candidate

- **WHEN** a nil collector or a typed-nil collector is registered
- **THEN** the call returns the zero collector and a fatal error, does not panic, and `Unregister` of the key returns
  false

#### Scenario: Cross-key alias

- **WHEN** a collector whose descriptor a second key, a core metric, or a collector registered directly through
  `PrometheusRegistry()` already owns is registered under a new key
- **THEN** the call returns the zero collector and a fatal error, `Unregister` of the new key returns false, and only
  the owning collector's series is gathered, carrying the owner's writes
