# graph-readiness Delta

## MODIFIED Requirements

### Requirement: The readiness gate gives one reason in a fixed order

`readiness.EvaluateReadinessGate` SHALL take one `readiness.Reading` and return one `DeferReason`, checked in this
order: `status_unknown` when the reading is not fresh; `unrecognized_state` when the state is not one of
`graph.IndexStates()`, which are `building`, `ready` and `degraded`; `hard_stop` when the state is `degraded`;
`bootstrap_incomplete` when the envelope's `bootstrap_complete` is false; otherwise `DeferNone`, whatever the
envelope's `ready`, `lag` or `staleness_ms`.

#### Scenario: A stale reading of a healthy envelope

- **WHEN** the gate reads a `Reading` with `Fresh` false and a `ready`, bootstrapped envelope
- **THEN** it returns `status_unknown`

#### Scenario: Two conditions hold

- **WHEN** the gate reads a fresh `Reading` whose state is `degraded` and whose `bootstrap_complete` is false
- **THEN** it returns `hard_stop`

#### Scenario: A state no producer writes any more

- **WHEN** the gate reads a fresh `Reading` whose state is `reset_required`
- **THEN** it returns `unrecognized_state` (`TestEvaluateReadinessGate_UnrecognizedStateFailsClosed`, extended)
