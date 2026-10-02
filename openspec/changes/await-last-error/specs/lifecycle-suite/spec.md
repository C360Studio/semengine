# Lifecycle suite delta

## MODIFIED Requirements

### Requirement: Probes retain observable state

`probe.Callback` SHALL expose entered, release, and joined channels; `probe.ObservedContext` SHALL signal the first
Done() observation and optionally hold it; `probe.Await` SHALL poll under the caller's context and return the last
observation's value and its error, if it had one, on failure.

#### Scenario: Await reports the last observation

- **WHEN** the observed condition never holds
- **THEN** Await returns after the context ends with the last observation's value and its error, if it had one,
  in the message
