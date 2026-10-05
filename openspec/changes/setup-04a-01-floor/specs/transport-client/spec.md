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

#### Scenario: Stopped heartbeat

- **WHEN** the heartbeat stops while the work runs past the acknowledgement wait
- **THEN** the server redelivers the message

#### Scenario: Semantic retry

- **WHEN** the work returns a retry decision
- **THEN** the server redelivers the message durably and the delivery count increases

#### Scenario: Work panics

- **WHEN** the work panics
- **THEN** the result's decision is quarantine with the panic as its cause, the message is not acknowledged, and the
  result reports that the owner must stop

### Requirement: Close and Drain are bounded by the caller's context

`Subscription.Drain(ctx)` SHALL refuse a nil context (kept from the pin). `Client.Close(ctx)` and `Client.Connect(ctx)`
SHALL refuse a nil context without touching the connection (changed behaviour: at the pin `Close(nil)` panics on a
connected client, and `Connect(nil)` dials, then panics, leaking the dialled connection; design P13). `Close` SHALL
bound its drain by the caller's context deadline and return within it, reporting the drain outcome, and a later
`Close` SHALL make no server call, and SHALL return nil once the first
`Close`'s cleanup and the join are complete; none of these SHALL be
repeated or retried by the client on the caller's behalf.

#### Scenario: Close under an ended context

- **WHEN** Close is called with a deadline shorter than the drain needs
- **THEN** Close returns within the deadline, the connection is closed, and a second Close with a live context returns
  nil once no handler of the client is still running (see "A nil Close means every handler returned")

#### Scenario: Nil Close context

- **WHEN** Close is called with a nil context on a connected client
- **THEN** Close returns an error naming the nil context, the connection stays open, and no drain is attempted

#### Scenario: Nil Connect context

- **WHEN** Connect is called with a nil context
- **THEN** Connect returns an error naming the nil context and no dial is attempted

#### Scenario: Nil Drain context

- **WHEN** Drain is called with a nil context
- **THEN** Drain returns an error naming the nil context and makes no server call

### Requirement: A nil Close means everything the client owns has finished

`Client.Close` SHALL return nil only after every piece of work the client owns has finished: its background work,
every running invocation of a message handler passed to `Subscribe`, `SubscribeForRequests` or a Consume method,
the event handlers of the connection it closes when this client dialled it, and its async publish error handler as
far as nats.go lets the client observe it (D3 declares the one residual). This SHALL hold on every Close path, a
drain that runs out of time included, which Close SHALL report as a transient error wrapping
`nats.ErrDrainTimeout` whichever timer ran out first. Each Close SHALL return within its own context and SHALL NOT
wait on another caller's drain. A Close called from inside one of the client's callbacks SHALL return its context's
error and never nil. A client-owned subscription left on a connection that `SetConnection` replaced SHALL be
unsubscribed and its running handler joined; the replaced connection itself belongs to the `SetConnection` caller.
A client-created consumer whose connection, the one its JetStream handle was made on, was replaced through
`SetConnection` SHALL be stopped when Close begins and its handlers joined, and its claim SHALL be released only after
they return.

#### Scenario: A nil Close means every handler returned

- **WHEN** a message handler passed to Subscribe or a Consume method is running and Close is called, first with an
  ended context and then with a live one
- **THEN** the first Close returns the context's error, and the second returns nil only after the handler returned

#### Scenario: A drain that runs out of time is reported

- **WHEN** a message handler is still running when the drain timeout passes, and Close's context is live
- **THEN** the first Close returns an error wrapping the drain timeout, after the handler has returned

#### Scenario: Each Close observes its own context

- **WHEN** one Close is draining and a second Close is called with an ended context
- **THEN** the second Close returns the context's error without waiting for the drain

#### Scenario: Close from inside a client callback

- **WHEN** a client callback calls Close with a bounded context
- **THEN** Close returns that context's error and never nil

#### Scenario: A subscription on a replaced connection

- **WHEN** a client subscription's connection has been replaced through SetConnection and its handler is running
  when Close is called with a live context
- **THEN** Close unsubscribes it and returns nil only after the handler returned, and the replaced connection stays
  open

#### Scenario: A consumer on a replaced connection

- **WHEN** a client-created consumer's connection, the one its JetStream handle was made on, has been replaced through
  SetConnection, and Close is called with a live context
- **THEN** Close stops the consumer, returns nil only after its handlers returned and its claim was released, and the
  replaced connection stays open

### Requirement: Close refuses new work

Once `Close` has begun, `Connect`, `Subscribe`, `SubscribeForRequests` and the three Consume methods SHALL return
`nats.ErrConnectionClosed`, decided by Close having begun, not by `Status()`. `Connect` SHALL refuse before it
dials. A consumer setup admitted before Close began SHALL keep its claim until its handler invocations have
returned. `SetConnection` SHALL change nothing once Close has begun.

#### Scenario: Connect after Close

- **WHEN** Connect is called after Close has begun
- **THEN** it returns ErrConnectionClosed, no dial is attempted, and Status is unchanged

#### Scenario: Subscribe during the drain

- **WHEN** Subscribe or a Consume method is called while Close is draining
- **THEN** it returns ErrConnectionClosed and no handler of it ever runs

#### Scenario: A consumer refused during Close keeps its claim

- **WHEN** a consumer setup admitted before Close began has started native delivery
- **THEN** it returns ErrConnectionClosed with no handle only after its handler invocations returned, or, if its
  setup context ends first, that context's error at that point; either way its claim is held, and Close does not
  return nil, until the handlers have returned

### Requirement: Only Close changes the status once Close begins

While a connection is installed and Close has not begun, a Connect that does not install its own connection SHALL
change neither `Status()`, nor `Failures()`, nor the circuit. Once Close has begun, no writer but Close SHALL change
`Status()`. It keeps its value through the drain and is `Disconnected` for good once Close's cleanup has finished.

#### Scenario: Only Close changes the status once Close begins

- **WHEN** Close has begun and the health monitor, a connection handler, a failing operation or a Connect would
  change the status
- **THEN** Status keeps the value it had when Close began until Close's cleanup has finished, and reports
  Disconnected from then on

#### Scenario: A losing Connect leaves the winner's status

- **WHEN** two Connect calls overlap, one installs its connection and the other fails or is cancelled
- **THEN** Status, Failures and the circuit are those of the installed connection

### Requirement: A delivery after the recorded end is refused, and counted

A delivery that nats.go hands over after the client has recorded its subscription's or consumer's end SHALL NOT run
the caller's handler. Each refusal SHALL be logged at warn level and counted as `late_delivery_refused` on the
JetStream error metric when the client has JetStream metrics configured. With acknowledgements the message is
redelivered. With `AckPolicy: "none"`, or on a core subscription, it is lost; the owner accepted this loss
(#9 comment 5980769459). A graceful drain SHALL NOT refuse a delivery this way.

#### Scenario: Late delivery on an acknowledged consumer

- **WHEN** a delivery reaches the client after it recorded the consumer's end
- **THEN** the handler does not run, the message is not settled, a warn record names the subject and ack policy,
  and the `late_delivery_refused` count rises by one

### Requirement: A message handler's panic is recovered

A panic in a message handler passed to a Consume method SHALL be recovered by the client, which SHALL Nak the
message, log the panic at error level with the message's subject, and count it as `handler_panic` on the JetStream
error metric when the client has JetStream metrics configured (owner ruling 3, #9 comment 5985697767: only a root
process lets a panic end it). A Nak that fails SHALL be logged with the panic, not discarded.

#### Scenario: A handler panics

- **WHEN** a consumer's message handler panics
- **THEN** the panic does not escape, the message is Nak'd, an error record names the panic and the subject, and
  the `handler_panic` count rises by one

#### Scenario: The Nak after a panic fails

- **WHEN** a handler panics and the Nak that follows returns an error
- **THEN** the error record names the panic, the subject and the Nak error

### Requirement: A consumer's setup context does not parent its handlers

`ConsumeStreamWithConfig` and `ConsumeInternalStreamWithConfig` SHALL use their context to bound setup only and SHALL
NOT retain it. Each handler's context SHALL descend from a context the client owns for that consumer, which the
client SHALL cancel when `Close` begins; `Close` SHALL still join every handler invocation (owner ruling, #9 comment
5994720412 item 2). `ConsumeStreamWithConfigContexts` keeps the caller's `handlerCtx` as the handlers' parent.

#### Scenario: A handler after the setup deadline

- **GIVEN** a consumer started with a setup context whose deadline then passes
- **WHEN** a message is delivered after that deadline
- **THEN** its handler receives a context that has not ended

#### Scenario: Close begins while a handler runs

- **GIVEN** a consumer whose handler is running
- **WHEN** `Close` begins
- **THEN** the handler's context ends, and `Close` returns nil only after the handler has returned
