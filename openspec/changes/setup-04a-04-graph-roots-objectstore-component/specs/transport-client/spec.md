# transport-client

## ADDED Requirements

### Requirement: A caller can wait for one consumer's handlers

`Client.WaitConsumerHandlers(ctx, handle)` SHALL return nil once delivery on that consumer has ended and every handler
invocation of it has returned, and SHALL NOT stop delivery itself. If `ctx` ends first it SHALL return an error matching
`ctx.Err()` and change nothing. It SHALL refuse a nil `ctx`, and a handle this client did not return, as invalid input
without waiting or panicking. It SHALL never close the client.

#### Scenario: A handler still runs after the consumer's end

- **WHEN** a handler is held mid-invocation and the consumer's handle is stopped, or the broker is gone for good
- **THEN** the wait returns only after the handler returns, then nil
- **AND** the tests that verify this are `TestWaitConsumerHandlersWaitsForRunningHandler` and
  `TestWaitConsumerHandlersAfterConnectionLoss`

#### Scenario: The context ends first

- **WHEN** the context ends while a handler still runs
- **THEN** the call returns an error matching the context's error, and a later call with a fresh context returns nil
  once the handler has returned
- **AND** the test that verifies this is `TestWaitConsumerHandlersContextEndsFirst`

#### Scenario: A handle the client did not return

- **WHEN** the handle is nil, another client's, or a value nats.go never made
- **THEN** the call returns an invalid-input error at once
- **AND** the test that verifies this is `TestWaitConsumerHandlersRefusesUnknownHandle`

#### Scenario: After Close

- **WHEN** the client's `Close` has returned nil
- **THEN** a call with one of its handles returns nil at once
- **AND** the test that verifies this is `TestWaitConsumerHandlersAfterClose`
