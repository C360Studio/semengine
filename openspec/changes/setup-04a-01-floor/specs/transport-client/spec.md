# transport-client

## ADDED Requirements

### Requirement: Acknowledged is not durable on a memory stream

A publish acknowledgement from the client SHALL mean only that the server accepted the message into the stream's
storage class; the client SHALL expose the stream's storage class in its stream configuration, and a consumer of a
memory-backed stream SHALL NOT be promised the message across a server restart.

#### Scenario: Memory stream after restart

- **WHEN** a message is published to a memory-backed stream and acknowledged, and the server restarts
- **THEN** the message is absent, and a file-backed stream's message published the same way is present

### Requirement: Settlement follows the decision

A message consumed through the delivery helpers SHALL be acknowledged, negatively acknowledged, or terminated only
after the delivery work returned its decision; a panic in the work SHALL be settled as the result reports and SHALL
not acknowledge the message; a work item that outlasts the acknowledgement wait SHALL be kept from redelivery by the
heartbeat policy while the work runs, and SHALL be redelivered when the heartbeat stops.

#### Scenario: Long work with heartbeat

- **WHEN** the work runs past the consumer's acknowledgement wait with a healthy heartbeat
- **THEN** the message is not redelivered to a second handler while the first runs

#### Scenario: Semantic retry

- **WHEN** the work returns a retry decision
- **THEN** the server redelivers the message durably and the delivery count increases

#### Scenario: Work panics

- **WHEN** the work panics
- **THEN** the result's decision is quarantine with the panic as its cause, the message is not acknowledged, and the
  result reports that the owner must stop

### Requirement: Close and Drain are bounded by the caller's context

`Subscription.Drain(ctx)` SHALL refuse a nil context (kept from the pin). `Client.Close(ctx)` and `Client.Connect(ctx)`
SHALL refuse a nil context without touching the connection (changed behaviour: the pin reaches the context's
deadline with a live connection). `Close` SHALL bound its drain by the caller's context deadline and return within
it, reporting the drain outcome, and a second `Close` SHALL return nil without a server call; none of these SHALL be
repeated or retried by the client on the caller's behalf.

#### Scenario: Close under an ended context

- **WHEN** Close is called with a deadline shorter than the drain needs
- **THEN** Close returns within the deadline, the connection is closed, and a second Close returns nil

#### Scenario: Nil Close context

- **WHEN** Close is called with a nil context on a connected client
- **THEN** Close returns an error naming the nil context, the connection stays open, and no drain is attempted

#### Scenario: Nil Connect context

- **WHEN** Connect is called with a nil context
- **THEN** Connect returns an error naming the nil context and no dial is attempted

#### Scenario: Nil Drain context

- **WHEN** Drain is called with a nil context
- **THEN** Drain returns an error naming the nil context and makes no server call
