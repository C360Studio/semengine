# transport-client

## ADDED Requirements

### Requirement: A failed call refuses no other call

No natsclient call SHALL refuse because other calls failed. A call that fails SHALL return its own error and SHALL
leave `Status()` as `Connect`, the connection's events, the health monitor and `Close` set it (changed: at the pin,
fifteen failed calls or dials with no success between them opened a circuit breaker; until its backoff ended,
JetStream and request calls returned `ErrCircuitOpen`, as did `Connect` while no connection was installed, and the
status then read `Disconnected` on a live connection until the health monitor's next tick; the breaker is
removed, #222).

#### Scenario: Failures on one subject leave other calls working

- **WHEN** a connected client whose health monitor is off makes twenty `PublishToStream` calls to a subject no stream
  captures, and each fails
- **THEN** each error is that publish's own failure, a `PublishToStream` to a subject a stream captures then
  succeeds, and `Status()` reports `Connected`

### Requirement: The not-connected refusal carries the transient class

`ErrNotConnected` SHALL be transient by `errs.IsTransient` and by `errs.Classify`.

#### Scenario: Not connected

- **WHEN** a call returns `ErrNotConnected`
- **THEN** `errs.IsTransient` reports true and `errs.Classify` returns `ErrorTransient`

## MODIFIED Requirements

### Requirement: Only Close changes the status once Close begins

While a connection is installed and Close has not begun, a Connect that does not install its own connection SHALL NOT
change `Status()`. Once Close has begun, no writer but Close SHALL change `Status()`. It keeps its value through the
drain and is `Disconnected` for good once Close's cleanup has finished.

#### Scenario: Only Close changes the status once Close begins

- **WHEN** Close has begun and the health monitor, a connection handler or a Connect would change the status
- **THEN** Status keeps the value it had when Close began until Close's cleanup has finished, and reports
  Disconnected from then on

#### Scenario: A losing Connect leaves the winner's status

- **WHEN** two Connect calls overlap, one installs its connection and the other fails or is cancelled
- **THEN** Status is that of the installed connection

### Requirement: A retrying request refuses what it cannot attempt

`RequestWithRetryClassified` SHALL take a `retry.Config` and make its attempts as `retry.Do` does: `MaxAttempts` counts
every attempt, a count below one makes one attempt (a negative count is no longer refused), and a config `retry.Do`
refuses SHALL be returned before anything is sent (changed: `natsclient.RetryConfig`, whose `MaxRetries` counted the
attempts after the first and was refused below zero, is removed, #222; at the pin a negative count returned success with
no reply, Codex F31). Only an attempt that received no reply SHALL be retried; a reply, an error reply included, ends
the call. It SHALL check the context before every attempt, the first included, and SHALL return a caller's
cancellation as an error that matches `context.Canceled` and that `errs.IsTransient` reports transient (Codex F32).
`RequestWithHeaders`, `RequestClassified` and the retrying call SHALL return a cancellation that arrives while an
attempt waits for its reply as an error that matches `context.Canceled` (early check E1).

#### Scenario: Negative retry count

- **WHEN** `RequestWithRetryClassified` is called with `MaxAttempts` 0 or -1 and a responder that answers
- **THEN** the responder's handler runs once and the call returns its reply

#### Scenario: A config retry.Do refuses

- **WHEN** `RequestWithRetryClassified` is called with a negative `InitialDelay`
- **THEN** it returns an error, whose class is not promised, and the responder's handler never runs

#### Scenario: An error reply is not retried

- **WHEN** the responder's handler returns an error and `MaxAttempts` is 3
- **THEN** the handler runs once and the call returns the classified error

#### Scenario: Ended context

- **WHEN** a retrying request is called with a context that has already ended
- **THEN** it returns an error matching `context.Canceled` and the responder's handler never runs

#### Scenario: Cancelled while an attempt is in flight

- **WHEN** a caller cancels `RequestWithHeaders`, `RequestClassified` or `RequestWithRetryClassified` while the
  responder holds the request
- **THEN** the call returns an error matching `context.Canceled`

## REMOVED Requirements

### Requirement: The client's refusals carry the transient class

**Reason**: `natsclient.ErrCircuitOpen` is removed with the circuit breaker (#222). OpenSpec's archive refuses a
modified requirement that drops a scenario, so the requirement is re-added above as "The not-connected refusal carries
the transient class", with its "Not connected" scenario and without "Circuit open".

**Migration**: callers that matched `natsclient.ErrCircuitOpen` drop that term; `ErrNotConnected` is unchanged.
