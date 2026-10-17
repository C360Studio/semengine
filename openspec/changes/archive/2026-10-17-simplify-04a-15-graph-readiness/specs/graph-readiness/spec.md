# Delta: graph-readiness

## ADDED Requirements

### Requirement: The readiness gate gives one reason in a fixed order

`readiness.EvaluateReadinessGate` SHALL take one `readiness.Reading` and return one `DeferReason`, checked in this
order: `status_unknown` when the reading is not fresh; `unrecognized_state` when the state is not one of
`graph.IndexStates()`; `hard_stop` when the state is `degraded` or `reset_required`; `bootstrap_incomplete` when the
envelope's `bootstrap_complete` is false; otherwise `DeferNone`, whatever the envelope's `ready`, `lag` or
`staleness_ms`. The zero `Reading` SHALL give `status_unknown`. `readiness.DeferReasons()` SHALL return the four
non-empty reasons as a new slice on every call.

#### Scenario: A stale reading of a healthy envelope

- **WHEN** the gate reads a `Reading` with `Fresh` false and a `ready`, bootstrapped envelope
- **THEN** it returns `status_unknown`

#### Scenario: Two conditions hold

- **WHEN** the gate reads a fresh `Reading` whose state is `degraded` and whose `bootstrap_complete` is false
- **THEN** it returns `hard_stop`

#### Scenario: The reason list is a copy

- **WHEN** a caller overwrites an element of the slice `DeferReasons()` returned
- **THEN** the next call returns the four reasons unchanged

### Requirement: Readiness wiring is refused, not ignored

`readiness.NewPublisher` SHALL return an error, and no publisher, for a nil bucket or an empty key. `Watcher.Run`
SHALL return an error before any I/O for a nil context, a nil bucket source, an empty key, a heartbeat of zero or less,
or a nil logger. Neither SHALL substitute a default for a missing value.

#### Scenario: A publisher with no key

- **WHEN** `NewPublisher` is called with a bucket and an empty key
- **THEN** it returns an error and a nil publisher

#### Scenario: A watcher with no heartbeat

- **WHEN** `Run` is called on a watcher built with a heartbeat of zero
- **THEN** it returns an error without opening the bucket
